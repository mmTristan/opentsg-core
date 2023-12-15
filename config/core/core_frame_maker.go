package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/cbroglie/mustache"
	gonanoid "github.com/matoous/go-nanoid"
	"github.com/mrmxf/opentsg-core/config/internal/get"
	"github.com/peterbourgon/mergemap"
	"gopkg.in/yaml.v3"
)

type mustacheKey struct {
	key      string
	min, max int
}

type data struct {
	Dimensions []int            `json:"dimensions" yaml:"dimensions"`
	Data       []map[string]any `json:"data" yaml:"data"`
}

// FrameWidgetsGenerator runs the create frame for the given position. Applying any updates required and generating any
// extra json from data. It returns an initial context with all the frame and configuration information.
/*
The widgets are found by depth first tree traversal, where properties are passed by property and not value.

This means each include statement is searched when it is found and the order widgets
are declared in this format, is the order they are run.

*/
func FrameWidgetsGenerator(c context.Context, pos int, debug bool) (context.Context, []error) {
	var allError []error

	// generate a frameContext context to be returned
	frameContext := context.Background()

	// extract the base info
	all := c.Value(updates).(factory)
	mainBase := c.Value(frameHolders).(base)

	// create a clean map for each frame to prevent overwrite errors. Line holder is only to be read from
	bases := base{importedFactories: make(map[string]factory),
		importedWidgets: make(map[string]json.RawMessage),
		jsonFileLines:   mainBase.jsonFileLines,
		metadataParams:  mainBase.metadataParams,
		metadataBucket:  make(map[string]map[string]any)}
	for k, v := range mainBase.importedFactories {
		bases.importedFactories[k] = v
	}
	for k, v := range mainBase.importedWidgets {
		bases.importedWidgets[k] = v
	}

	bases.generatedFrameWidgets = make(map[string]widgetContents)
	rawUpdate := all.Create[pos]

	z := 0
	// generate all the rawjson and their name/relarive position
	errs, delayUpdates := bases.createWidgets(rawUpdate, "", []int{}, 0, &z)
	if len(errs) != 0 {
		// append the errors to be handled by the core/draw
		allError = append(allError, errs...)
	}

	// update the array paths
	for _, delayUpdate := range delayUpdates {

		parent := regexp.MustCompile(`^[\w\.]{1,255}(\[[\d]{1,3}:{0,1}[\d]{0,3}\]){1,}$`)
		var updates []string
		var err error
		if parent.MatchString(delayUpdate.target) {
			updates, err = arrayGetter(delayUpdate.target, bases.generatedFrameWidgets)
		} else {
			updates, err = dotGetter(delayUpdate.target, bases.generatedFrameWidgets)
		}
		if err != nil {
			allError = append(allError, err)
		}

		for _, update := range updates {
			frameBase := bases.generatedFrameWidgets[update]
			// if it has data then update, the headers do not contain any json data
			if len(frameBase.Data) != 0 {
				ibody, _ := yaml.Marshal(delayUpdate.body)
				frameBase.Data, err = jsonCombiner(frameBase.Data, ibody)
				if err != nil {

					allError = append(allError, fmt.Errorf("0035 when updating the widget %v from the %s : %v ", update, delayUpdate, err))
				}
				bases.generatedFrameWidgets[update] = frameBase
			}
		}
	}

	// Metadata update of the base widget
	for k, wc := range bases.generatedFrameWidgets {
		// if it is widget to be updated
		if wc.Tag != "" {
			metadata, _ := bases.metadataGetter(map[string]any{}, k)
			var widget map[string]any
			yaml.Unmarshal(wc.Data, &widget)

			updatedWidget, err := objectMustacheUpdater(widget, metadata, k, "", "")
			if err != nil {
				allError = append(allError, err)
			} else {
				updatedByte, _ := yaml.Marshal(updatedWidget)
				wc.Data = updatedByte
				bases.generatedFrameWidgets[k] = wc
			}
		}
	}

	// after getting all the updates apply the metadata to the base widgets.
	/*

		pseudo code

		loop through every update. Get the relevant metadata, from parents to child.
		apply the updates with the object

		then carry on

	*/

	// fmt.Println(bases.metadataBucket)

	parentsOfWidgetsMap := SyncMap{make(map[string]string), &sync.Mutex{}}
	// addedWidgets holds all the widgets that are assigned a widget so missed ones can be found
	frameContext = context.WithValue(frameContext, addedWidgets, parentsOfWidgetsMap)
	// base key gives the frame for each run
	frameContext = context.WithValue(frameContext, baseKey, bases.generatedFrameWidgets)
	// alias key contains the global section for all the aliases in gridgen
	frameContext = context.WithValue(frameContext, aliasKey, c.Value(aliasKey))
	// factorydir has the factory the open bit was called in, so far for use in the add iamge widget
	frameContext = context.WithValue(frameContext, factoryDir, c.Value(factoryDir))
	// lines holds the hash of the lines of al the json values
	frameContext = context.WithValue(frameContext, lines, mainBase.jsonFileLines)
	// mmReaderAuth holds any auth information used in the system
	frameContext = context.WithValue(frameContext, credentialsAuth, mainBase.authBody)

	if debug {
		//tree(bases.generatedFrameWidgets)
	}
	// add an ability to just dump the frame and let the user see
	return frameContext, allError
}

type jsonUpdate struct {
	target string
	body   map[string]any
}

// each json has factory has a tag that defines the widget it represents
type widgetID struct {
	ID string `json:"type,omitempty" yaml:"type,omitempty"`
}

// createWidgets loops through the create functions of all the factories and generates
// the widgets to be used on a frame by frame basis.
func (b *base) createWidgets(createTargets map[string]map[string]any, parent string,
	positions []int, start int, zPos *int) ([]error, []jsonUpdate) {

	var genErrs []error
	var arrayUpdates []jsonUpdate

	// tag the parent factory position for array positions calculations
	if len(parent) != 0 { // check it isn't the main parent factory
		if _, ok := b.generatedFrameWidgets[parent[:len(parent)-1]]; !ok {
			b.generatedFrameWidgets[parent[:len(parent)-1]] = widgetContents{arrayPos: positions}
		}
	}

	// extract the runOrder to use the keys in
	runOrder := keyOrder(createTargets)

	creatCount := start
	// run the updates in the set order
	for _, runKey := range runOrder {
		createUpdate := createTargets[runKey]
		dotExt := "" // treat like there's not dot extenions unless specified otherwise

		if strings.Contains(runKey, ".") {
			children := strings.SplitN(runKey, ".", 2) // split in two for the remaining dot extensions
			dotExt = children[1]
			runKey = children[0] // update k to just the first section of the dot path
		}
		dotPath := parent + runKey

		// is it targeting a factory?
		childFactory, ok := b.importedFactories[dotPath]

		if !ok {

			// check if the updates are in a predeclared widget or an array/dotpath update
			update, err := b.widgetHandler(createUpdate, dotPath, dotExt, positions, creatCount, zPos)

			arrayUpdates = append(arrayUpdates, update...)

			if err != nil {
				genErrs = append(genErrs, err)
			}

		} else { // run the factory

			fullname := parent + runKey

			if dotExt != "" {

				fullname += "." + dotExt
			}

			//args := childFactory.getArgs()

			//metadata2 := make(map[string]any)

			//	parents := strings.Split(fullname, ".")
			//var base string

			/*
				use run key to argument paramters

				at the moment this takes the arguments and applies them to any thing that may be passed on.
				whereas the level its decalred should be edited?
			*/
			//	fmt.Println(b.metadataParams, "PARENT", parent, "run", parent+runKey, b.metadataParams[parent+runKey], dotExt)
			//	fmt.Println(createUpdate)

			/*
				newArgs := b.metadataParams[fullname]
				for i, p := range parents {
					//fmt.Println(p)
					if i != 0 {
						base += "." + p
					} else {
						base = p
					}

					// only update with the metadata from the bucket
					// and with metadata that fits the arguments
					for k, v := range b.metadataBucket[base] {
						if stringMatcher(k, newArgs) {
							metadata2[k] = v
						}
					}

					// mapOverWriter(metadata2, b.metadataBucket[base])

				}

				//	args2 = append(args2, b.metadataParams[parent+runKey]...)
				// switch between the methods
				if len(newArgs) > 0 {
					args = newArgs
				}

				// metadata bucket can be run here
				// can give parents and overwriting here


				additions := make(map[string]any)
				// newChild is the generated creates to passed along
				// split the data into additions and metadata.
				// metadata is used to update everything within the child
				for k, v := range createUpdate {
					if stringMatcher(k, args) {
						metadata[k] = v
					} else { // if it doesn't match then it is an addition
						additions[k] = v
					}
				}*/

			metadata, additions := b.metadataGetter(createUpdate, fullname)

			//	fmt.Println(args, newArgs, fullname)
			//	fmt.Println(metadata, additions, metadata2)
			// assign the metadata before additional metadata is used

			// b.metadataBucket[fullname] = metadata

			//mapOverWriter(metadata2, metadata)
			//metadata = metadata2

			//		fmt.Println(b.metadataBucket, metadata2, base, "metadata")
			// fmt.Println(b.metadataBucket, runKey, parents, dotExt, "md", dotPath)

			newChild, err := getChildren(childFactory, dotPath, dotExt, additions, metadata)
			//	fmt.Println("CHILDREN:", newChild, additions, metadata, childFactory)
			if err != nil { // quit this run after finding the errors
				genErrs = append(genErrs, err)

				continue
			}

			// fmt.Println(newChild)

			// run the generate first if there is any
			// do not run if the dot path is targeting its children
			// as the children are updated afterwards
			if len(childFactory.Generate) > 0 && dotExt == "" {

				//	fmt.Println(b.metadataBucket)
				errs := b.factoryGenerateWidgets(childFactory.Generate, dotPath+".", metadata, append(positions, creatCount), zPos)
				genErrs = append(genErrs, errs...)
			}

			// then run any other creates
			for i, r := range newChild {

				// run the other creates to pass on their arguments
				errs, arrs := b.createWidgets(r, dotPath+".", append(positions, []int{creatCount}...), i, zPos)
				arrayUpdates = append(arrayUpdates, arrs...)
				genErrs = append(genErrs, errs...)
			}
		}
		creatCount++
	}

	return genErrs, arrayUpdates
}

func (b *base) metadataGetter(update map[string]any, fullname string) (metadata, updaters map[string]any) {

	metadata = make(map[string]any)
	updaters = make(map[string]any)

	parents := strings.Split(fullname, ".")
	var base string

	newArgs := b.metadataParams[fullname]
	for i, p := range parents {
		//fmt.Println(p)
		if i != 0 {
			base += "." + p
		} else {
			base = p
		}

		// only update with the metadata from the bucket
		// and with metadata that fits the arguments
		for k, v := range b.metadataBucket[base] {
			if stringMatcher(k, newArgs) {
				metadata[k] = v
			}
		}
	}

	// mapOverWriter(metadata2, b.metadataBucket[base])

	//	additions := make(map[string]any)
	// newChild is the generated creates to passed along

	layerMetadata := make(map[string]any)
	// split the data into additions and metadata.
	// metadata is used to update everything within the child
	for k, v := range update {
		if stringMatcher(k, newArgs) {
			metadata[k] = v
			layerMetadata[k] = v
		} else { // if it doesn't match then it is an addition
			updaters[k] = v
		}
	}
	//	fmt.Println(args, newArgs, fullname)
	//	fmt.Println(metadata, additions, metadata2)
	// assign the metadata before additional metadata is used

	// only assign the metadata if it is the first pass and a base widget

	if _, ok := b.metadataBucket[fullname]; !ok {

		// set the base as empty if there is no
		b.metadataBucket[fullname] = layerMetadata

	}

	/*	fmt.Println(fullname, b.metadataBucket, "BEFORE")
		b.metadataBucket[fullname] = metadata
		fmt.Println(fullname, b.metadataBucket, "AFTER")*/
	// b.metadataBucket[fullname] = metadata

	return
}

func mapOverWriter[C comparable, T any](dest, new map[C]T) {

	for k, v := range new {
		dest[k] = v
	}
}

// getChildren extracts the children of a factory. If it contains a dotpath extension then the child is just the single
// object of that addition.
func getChildren(childFactory factory, dotPath, dotExt string, additions, metadata map[string]any) ([]map[string]map[string]any, error) {
	var newChild []map[string]map[string]any

	if dotExt != "" {
		// apply the metadata and mock the create function for passing data on
		newChild = make([]map[string]map[string]any, 1)
		updated, err := objectMustacheUpdater(additions, metadata, dotPath, dotExt, "")
		if err != nil {
			return newChild, err
		}
		//	newChild[0] = map[string]map[string]any{dotExt: createUpdate}
		newChild[0] = map[string]map[string]any{dotExt: updated}
		//newChild[0] = map[string]map[string]any{dotExt: additions}
	} else {
		//this is where the problems happen
		// as the child takes the metadata from the parent instead of forming its won
		// @ TODO
		newChild = make([]map[string]map[string]any, len(childFactory.Create))
		for i, action := range childFactory.Create {
			// update the map for each create function with
			// the separated metadata and map aditions

			newAction := make(map[string]map[string]any)

			for k, v := range action {
				subAction := make(map[string]any)
				mergemap.Merge(subAction, v)
				mergemap.Merge(subAction, additions)

				newAction[k] = subAction
			}

			newChild[i] = newAction

			//	var err error
			/*	newChild[i], err = mapMustacheUpdater(action, metadata, additions, dotPath, dotExt)
				if err != nil {
					return newChild, err
				}*/
		}
	}

	return newChild, nil
}

// widgetHandler checks if an update is targeting a widget that has been generated and updates it. Or creates a new widget instance
// Else it returns the path as an update to be made later
func (b *base) widgetHandler(createUpdate map[string]any, dotPath, dotExt string, positions []int, createCount int, zPos *int) ([]jsonUpdate, error) {
	widgBase, ok := b.importedWidgets[dotPath]

	if !ok {
		// assume its a base widget array or dot path
		if dotExt != "" {
			dotPath += "." + dotExt
		}

		return []jsonUpdate{{dotPath, createUpdate}}, nil
	}

	//update createUpdate here

	// @ ADDED

	md, ud := b.metadataGetter(createUpdate, dotPath)
	res, err := objectMustacheUpdater(ud, md, dotPath, dotExt, "")

	if err != nil {
		return []jsonUpdate{}, err
	}

	err = b.frameBytesAdder(res, widgBase, dotPath, append(positions, createCount), zPos)

	// fmt.Println(string(widgBase), ok)
	if err != nil {
		return []jsonUpdate{}, fmt.Errorf("0009 %v when parsing the widget %s", err, dotPath)
	}

	return []jsonUpdate{}, nil
}

// frameBytesAdder checks for a widget in a frame and applies the updates. If there is already a widget it updated with the create map
func (b *base) frameBytesAdder(createUpdate map[string]any, widgetBase json.RawMessage, dotPath string, positions []int, zPos *int) error {

	// if it's already been made update based on the previous data
	createBytes, _ := yaml.Marshal(createUpdate)
	if holder, ok := b.generatedFrameWidgets[dotPath]; ok {
		raw, err := jsonCombiner(holder.Data, createBytes)
		if err != nil {

			return fmt.Errorf("0036 error when updating the widget %v: %v", dotPath, err)
		}

		holder.Data = raw
		b.generatedFrameWidgets[dotPath] = holder
	} else {
		// create a new entry

		raw, err := jsonCombiner(widgetBase, createBytes)
		if err != nil {

			return fmt.Errorf("0036 error when updating the widget %v: %v", dotPath, err)
		}

		var id widgetID
		err = yaml.Unmarshal(raw, &id)
		if err != nil {

			return fmt.Errorf("0009 %v when parsing the widget %s", err, dotPath)
		}

		if id.ID == "" {
			id.ID, _ = gonanoid.Nanoid() // generate a random id so widgets can't pick it up with delibrate names
		}
		b.generatedFrameWidgets[dotPath] = widgetContents{raw, *zPos, positions, id.ID}
		*zPos++
	}

	return nil
}

// keyOrder gets the dot and array path of a create target in order of the number of dots and arrays called.
// if there are the same amount of dot paths and arrays then the dotpath is first in the order.
func keyOrder(createTargets map[string]map[string]any) []string {
	// Get the run order of create sorting by dotpath depth
	keyOrder := make(map[int]map[int][]string)
	max := 0

	// sort the keys passed on how deep is the array and how many dotpaths does it contain
	for createTarget := range createTargets {
		arrs := strings.Count(createTarget, ".")
		pos := strings.Count(createTarget, "[")
		if _, ok := keyOrder[pos]; !ok {
			keyOrder[pos] = make(map[int][]string)
		}

		keyOrder[pos][arrs] = append(keyOrder[pos][arrs], createTarget)
		if pos+arrs > max {
			max = pos + arrs
		}
	}

	var order []string
	for i := 0; i <= max; i++ {
		// sort the order in number of arrays length of dot[ath]
		dotParents := make([]int, i+1)
		for base := 0; base <= i; base++ {
			dotParents[base] = i - base
		}
		for k, v := range dotParents {
			order = append(order, keyOrder[k][v]...)
		}
	}

	return order
}

// mapMustacheupdater updates the contents of the map with the mustache properties
func mapMustacheUpdater(baseCreate map[string]map[string]any, metadata, updates map[string]any, path, ext string) (map[string]map[string]any, error) {
	mustache.AllowMissingVariables = false
	action := make(map[string]map[string]any)

	extext := "" // the estension of the extension
	if ext != "" {
		if strings.Contains(ext, ".") {
			children := strings.SplitN(ext, ".", 2) // split in two for the remaining dot extensions
			extext = "." + children[1]
			ext = children[0] // update k to just the first section of the dot path
		}
	}

	for targetWidg, baseUpdate := range baseCreate {
		// update the target widget with the metadata here. The metadata is self contained per child
		//@ Added removed the metadata bots
		target, err := mustacheErrorWrap(targetWidg, path+ext, metadata)
		if err != nil {

			return action, err
		}

		if target != ext && ext != "" {
			// if there is an extension and it doesn't match the target then continue
			// this stops dot paths causing chaos and being used where they aren't intended

			continue
		} else {
			target += extext
		}

		if len(baseUpdate) == 0 {
			// allow for empty updates by producing an empty map to pass on
			action[target] = make(map[string]any)
		}
		// update copyMap with the updates to not change the original base
		copyMap := make(map[string]any)
		for k, v := range baseUpdate {
			copyMap[k] = v
		}
		mergemap.Merge(copyMap, updates)

		action[target], err = objectMustacheUpdater(copyMap, metadata, path, ext, extext)
		if err != nil {

			return action, err
		}
	}

	return action, nil
}

// objectMustacheUpdater updates a map[string]any with all the metadata required
func objectMustacheUpdater(updates, metadata map[string]any, path, ext, extext string) (map[string]any, error) {
	action := make(map[string]any)
	for k, vals := range updates {
		var err error
		metadata["framenumber"] = "{{framenumber}}" // self fufilling mustache substition
		action[k], err = typeExtractor(vals, path+ext, metadata)
		if err != nil {

			return nil, err
		}
	}

	return action, nil
}

// type extractor recusrively searches through type any to update all the values of an object
// with the mustached metadata
func typeExtractor(value any, location string, metadata map[string]any) (any, error) {
	switch val := value.(type) {
	case string:
		// update the target widget with the metadata here. The metadata is self contained per child
		update, err := mustacheErrorWrap(val, location, metadata)

		return update, err

	case []any:
		old := val
		newArray := make([]any, len(old))
		for i, s := range old {
			extracted, err := typeExtractor(s, location, metadata)
			if err != nil {
				return val, err
			}
			newArray[i] = extracted
		}

		return newArray, nil
	case map[string]any:
		// get a new map to update the base of
		updatedMap := make(map[string]any)
		v, vKeys := get.Get(val, []string{}, false)

		for i := range v {
			target := v[i]

			// follow the extract chain
			extracted, err := typeExtractor(target, location, metadata)
			if err != nil {
				return updatedMap, err
			}

			// loop through the results and mustache every internal value
			err = set(updatedMap, vKeys[i], extracted, location)
			if err != nil {

				return updatedMap, err
			}

		}

		return updatedMap, nil

	default:

		return value, nil
	}
}

// factoryGenerateWidgets generates dotpaths and widgets from json data. It overwrites a base json with the data points
func (b *base) factoryGenerateWidgets(generateActions []generate, parent string, meta map[string]any, aPositions []int, zPos *int) []error {
	var errs []error
	count := 0

	// assign the base array position
	b.generatedFrameWidgets[parent[:len(parent)-1]] = widgetContents{arrayPos: aPositions}

	for _, generator := range generateActions {

		/* do some generation */
		for targetName, actions := range generator.Action {
			// place the parent without the dot

			// make each new bases
			for action, updateFields := range actions {

				// run the action and append errors
				genErrs := b.generateAction(generator.Name, action, targetName, parent, meta, updateFields, aPositions, zPos)
				errs = append(errs, genErrs...)

			}
		}
		count++
	}

	return errs
}

// generateAction runs the generate sequence for a single action, updating the base with the generated frame widgets.
// It returns an array of errors as some functions generate several
func (b *base) generateAction(genName []map[string]string, action, targetName, parent string, meta map[string]any, updateFields []string, aPositions []int, zPos *int) []error {
	dataAndExt := strings.SplitN(action, ".", 2) // split in two for the remaining dot extensions

	updateData, ok := b.importedWidgets[parent+dataAndExt[0]] // extract the data
	if !ok {
		return []error{fmt.Errorf("0010 no data was found for %s", parent+dataAndExt[0])}

	}
	jsonBase, ok := b.importedWidgets[parent+targetName]
	if !ok {
		return []error{fmt.Errorf("0011 no widgets were found for %s", parent+targetName)}

	}

	// get the data then add all the dimensions
	toAdd := make(map[string]data)

	// @TODO push it through with the schema so we know the error doesn't need checking.
	if err := yaml.Unmarshal(updateData, &toAdd); err != nil {
		return []error{fmt.Errorf("0033 extracting data for %s : ", parent+targetName)}
	}

	// get the data field, presuming its mustached in
	dataField, err := mustacheErrorWrap(dataAndExt[1], parent+dataAndExt[1], meta)
	if err != nil {
		return []error{err}
	}
	// no error checking as it will run later with error checking
	depths := toAdd[dataField].Dimensions

	// Assign the keys here
	mk, newerrs := getLimits(genName, parent+dataAndExt[0]) // make([]mustacheKey, len(g.Name))
	if newerrs != nil {
		return []error{newerrs}
	}
	aliasError := parent + dataAndExt[0] + "." + dataField // the alias to be used in error messages

	// check the keys dimensions
	mk, err = dimensionCheck(mk, depths, toAdd, dataField, aliasError)
	if err != nil {
		return []error{err}
	}

	data := toAdd[dataField].Data
	results, err := getName(mk, depths, dataField)

	if err != nil {
		return []error{fmt.Errorf("0014 %v at %s", err, aliasError)}
	}

	args := b.metadataParams[parent[:len(parent)-1]]
	// assign the argument of the parent to the generate widgets
	// so any metadata updates can still be applied
	for _, keys := range results {
		b.metadataParams[parent+keys.name[1:]] = args
		// figure out metadata declaration. BEcause the metadata may be comprised of
		// several layers
		b.metadataBucket[parent+keys.name[1:]] = meta
	}

	genErrs := b.dataToFrame(jsonBase, results, data, updateFields, parent, aPositions, zPos)

	return genErrs
}

// Data to frame combines the extracted data with a base widget for all the generated widgets.
func (b base) dataToFrame(jsonBase json.RawMessage, results []arrayValues, data []map[string]any, updateFields []string, parent string, aPositions []int, zPos *int) []error {
	var errs []error
	// go in the range of data or results = list of data points
	for _, result := range results {
		// extract the data at the results position as they aren't linear
		rd := data[result.arrayPos]

		addJSON, err := dataExtract(rd, updateFields, parent[:len(parent)-1]+result.name)
		if err != nil {
			errs = append(errs, err)
		} else { // only update if there is no error

			genBases := strings.Split(result.name, ".") // add bases here

			//	pos := append(aPositions, count)
			pos := aPositions
			// update the parents with array positions for later
			for g := 1; g < len(genBases)-1; g++ {
				// can make an any.join function to replicate my strings and array pos idea
				newb := strings.Join(genBases[:g+1], ".")
				b.generatedFrameWidgets[parent[:len(parent)-1]+newb] = widgetContents{arrayPos: append(pos, result.effectiveArray[:g]...)}

			}
			// run the update of the base with the new data
			newbase, err := jsonCombiner(jsonBase, addJSON)
			if err != nil {
				// if there's an error move onto the next one
				errs = append(errs, fmt.Errorf("0037 error when generating the widget %v: %v", parent[:len(parent)-1]+result.name, err))

				continue
			}

			var id widgetID
			err = yaml.Unmarshal(newbase, &id)
			if err != nil {
				// if there's an error move onto the next one
				errs = append(errs, fmt.Errorf("0034 Unable to extract widget Id for %s", parent[:len(parent)-1]+result.name))

				continue
			}

			if _, ok := b.generatedFrameWidgets[parent[:len(parent)-1]+result.name]; ok {
				// or do I break here?
				errs = append(errs, fmt.Errorf("0015 %s has already been generated for the parent %s", parent[:len(parent)-1]+result.name, parent[:len(parent)-1]))
			} else {
				b.generatedFrameWidgets[parent[:len(parent)-1]+result.name] = widgetContents{newbase, *zPos, append(pos, result.effectiveArray...), id.ID}
				*zPos++
			}
		}

	}

	return errs
}

// DimensionCheck checks if the mustache key values are within the range of depths for a data.
// It updates the mustache key depths to be the maximum when values are unknown.
func dimensionCheck(mk []mustacheKey, depths []int, toAdd map[string]data, dataField, aliasError string) ([]mustacheKey, error) {
	if len(depths) == len(mk) {
		// check the dimensions match as well
		dim := 1
		for i, m := range mk {
			dim *= depths[i] // multiply the depths to get the expected length of data
			if m.max == -1 { // else the max has been decided elsewhere
				mk[i] = mustacheKey{m.key, m.min, depths[i]}
			} else if m.max > depths[i] {
				// TODO emit an error? or just sandwich it down for the moment
				m.max = depths[i]
			}
		}

		if dim != len(toAdd[dataField].Data) {
			return mk, fmt.Errorf("0012 at %s the number of data points %v does not match the n dimensions of the data matrix %v", aliasError, len(mk), len(depths))

		}
	} else { // GET specific line with validator
		return mk, fmt.Errorf("0013 at %s the number of keys %v does not match the n dimensions of the data matrix %v", aliasError, len(mk), len(depths))
	}

	return mk, nil
}

// get limits get the array range for generated data
func getLimits(targets []map[string]string, path string) ([]mustacheKey, error) {

	mk := make([]mustacheKey, len(targets))
	for i, target := range targets {
		if len(target) != 1 {

			return mk, fmt.Errorf("0016 more than one key has been used for the %v dimension of the data at %s, received %v keys", i, path, len(target))
		}

		for targetKey, targetVal := range target {
			min, max := 0, 0

			ranger := regexp.MustCompile(`^\[[\d]{0,4}:[\d]{0,4}\]$`)
			rangerEnd := regexp.MustCompile(`^\[:[\d]{0,4}\]$`)
			switch {
			case targetVal == "[:]":
				max = -1
			case rangerEnd.MatchString(targetVal):
				fmt.Sscanf(targetVal, "[:%v]", &max)
			case ranger.MatchString(targetVal):
				n, _ := fmt.Sscanf(targetVal, "[%v:%v]", &min, &max)
				if n == 1 {
					max = -1
					// only one item was written
					// this means that it was only min -> max
					// if max == -1 then it means the max value
				}
			default:
				fmt.Sscanf(targetVal, "[%v]", &min) // else one d version
				max = min + 1
			}

			if min > max && max >= 0 {

				return mk, fmt.Errorf("0017 the minimum position %v is greater than the maxmimum of %v for the key %s", min, max, targetKey)
			}
			mk[i] = mustacheKey{targetKey, min, max}
		}
	}

	return mk, nil
}

type recurseData struct {
	key       string
	positions map[string]int
	results   []arrayValues
	mustaches []mustacheKey
	offsets   []int
}

type arrayValues struct {
	name           string
	arrayPos       int
	effectiveArray []int
}

// add all the recurisve values for the n dimensional arrays
func (r *recurseData) recurseDataArray(arrayPos int) {

	// on the mustaches minimum, not 0
	start := r.mustaches[arrayPos].min
	for i := start; i < r.mustaches[arrayPos].max; i++ {
		r.positions[r.mustaches[arrayPos].key] = i
		//	generate the title

		if len(r.mustaches)-1 > arrayPos {
			//	positions is a map of integers and metadata of each ones current position which is updated regardless
			r.recurseDataArray(arrayPos + 1)
		} else {
			//	get yo data add to parent(ofc)
			arrayPosition := 0 // update the dm at each position
			pos := make([]int, len(r.mustaches))
			for j, n := range r.mustaches {
				arrayPosition += (r.positions[n.key]) * r.offsets[j]
				pos[j] = r.positions[n.key]
			}
			// no error handling here as the keys are made within the function
			// so they never are missed out
			st, _ := mustache.Render(r.key, r.positions)
			r.results = append(r.results, arrayValues{st, arrayPosition, pos})
		}
	}
}

// dotgetter retruns the aliases associated with each dotpath
func dotGetter(updateKey string, bases map[string]widgetContents) ([]string, error) {
	parent := regexp.MustCompile("^" + strings.ReplaceAll(updateKey, ".", "\\."))
	var matches []string
	for widget := range bases {
		if parent.MatchString(widget) {
			matches = append(matches, widget)
		}
	}
	if len(matches) == 0 {

		return matches, fmt.Errorf("0018 no map values found for the dot path of %s", updateKey)
	}

	return matches, nil
}

// arrayGetter returns the alias asscoiated with each array path
func arrayGetter(arrayTarget string, aliasLocations map[string]widgetContents) ([]string, error) {

	parent := regexp.MustCompile(`^[\w\.]{1,255}(\[[\d]{1,3}:{0,1}[\d]{0,3}\]){1,}$`)
	counter := make(map[int][]string)
	minmaxes := make([][2]int, 0)

	max := 0
	// repeat this section for every string that matches and do the updates here
	// these are then overwritten by the more targeted ones
	switch {
	case parent.MatchString(arrayTarget):
		end := 0
		// loop through every set of cordinates
		for end+1 < len(arrayTarget) {
			start := end + strings.IndexRune(arrayTarget[end:], '[')
			end += strings.IndexRune(arrayTarget[end+1:], ']') + 1 // add one to prevent it finding itself
			min, max := 0, 0
			// scan the digits from the text
			n, _ := fmt.Sscanf(arrayTarget[start:end+1], "[%04d:%04d]", &min, &max)
			if n == 1 { // if only one was written then "min" is both values
				minmaxes = append(minmaxes, [2]int{min, min})
			} else {
				minmaxes = append(minmaxes, [2]int{min, max})
			}

		}

		// update the array to include the parent position
		title := strings.SplitN(arrayTarget, "[", 2)

		foundations, ok := aliasLocations[title[0]]
		if !ok { // if not OK ask why
			return nil, fmt.Errorf("0019 could not find array postions for %s for %s", title[0], arrayTarget)
		}

		for i := len(foundations.arrayPos) - 1; i >= 0; i-- {
			minmaxes = append([][2]int{{foundations.arrayPos[i], foundations.arrayPos[i]}}, minmaxes...)
		}

	default:

		return nil, fmt.Errorf("0020 %s is not a valid array string", arrayTarget)
		// return an error
	}
	// update these to also be ByteAndOrigin at a later date

	for k, v := range aliasLocations {
		// the array positions circumvent the name checking
		// and return the names of the alaises at those positions
		if arrayPositionFence(v.arrayPos, minmaxes) {
			depth := strings.Count(k, ".")
			counter[depth] = append(counter[depth], k)
			if depth > max {
				max = depth
			}
		}
	}

	if len(counter) == 0 {

		return nil, fmt.Errorf("0021 no matches found for %s", arrayTarget)
	}

	var order []string
	for i := 0; i <= max; i++ {
		order = append(order, counter[i]...)
	}

	return order, nil
}

func arrayPositionFence(targets []int, ranges [][2]int) bool {
	// if they aren;t the same length then they are referencing different dimensions
	if len(targets) < len(ranges) {

		return false
	}
	for i, point := range targets {
		if !(point >= ranges[i][0] && point <= ranges[i][1]) {

			return false
		}
		// return if there are more dimensions that ranges isn't checking
		if i+1 >= len(ranges) {

			return true
		}
	}

	return true
}

// get name produce the array of names for a data set
func getName(mk []mustacheKey, depths []int, param string) ([]arrayValues, error) {
	var base string
	dataArrayOffset := 1
	if len(mk) == 0 {
		return []arrayValues{}, fmt.Errorf("0022 no keys declared for %s", param)
	}

	offsets := make([]int, len(mk))
	for i := len(mk) - 1; i >= 0; i-- {
		offsets[i] = dataArrayOffset
		dataArrayOffset *= depths[i]
	}

	for _, m := range mk {
		base += "." + m.key + "{{" + m.key + "}}"
	}

	if dataArrayOffset == 0 {
		return []arrayValues{}, fmt.Errorf("0023 one of the array depths in  %v is 0 at %s", depths, param)
	}
	// dm /= mk[0].max

	// replace the first point with the parameter
	base = strings.Replace(base, ".", "."+param, 1)

	r := recurseData{base, make(map[string]int), []arrayValues{}, mk, offsets}
	r.recurseDataArray(0)

	return r.results, nil
}

// stringMatcher checks if a string matches any in an array
func stringMatcher(base string, targets []string) bool {
	for _, match := range targets {
		if match == base {

			return true
		}
	}

	return false
}

// data extract recursivley searches through a map to return the data as marshalled Json.
func dataExtract(base map[string]interface{}, keys []string, name string) ([]byte, error) {

	newData := make(map[string]interface{})
	// loop through the keys
	for _, key := range keys {
		layers := strings.Split(key, ".")
		if len(layers) > 1 { // if it is a dot bath follow the data path as a map
			// recurse etc
			baseValue := base[layers[0]]
			// find the value at the end of the chain
			for _, layerKey := range layers[1:] {
				nestedMap, ok := baseValue.(map[string]any)
				if !ok {

					return nil, fmt.Errorf("0024 the keys %s do not lead to a value for the object %s", keys, name)
				}

				base := nestedMap[layerKey]
				baseValue = base
			}
			updater := make(map[string]any)
			// recursively set the value
			err := set(updater, layers, baseValue, name)
			if err != nil {

				return []byte{}, err // the error is already in the opentsg format
			}

			newData[layers[0]] = updater[layers[0]]
		} else { // set the map straight away with the base
			newData[layers[0]] = base[layers[0]]
		}
	}

	return yaml.Marshal(newData)
}

// Set generates a new map from the keys, where each key in the array is a new layer in the map.
// The value is assigned at the end of the key path.
func set(setMap map[string]any, keys []string, value interface{}, path string) error {
	if len(keys) == 1 {
		setMap[keys[0]] = value

		return nil
	}

	v, ok := setMap[keys[0]]
	if !ok {
		v = make(map[string]any)
		setMap[keys[0]] = v
	}
	newM, ok := v.(map[string]any)
	if !ok {

		return fmt.Errorf("0008 at %s the key %s does not produce a map values for the keys %v", path, keys[0], keys[1:])
	}

	return set(newM, keys[1:], value, path)
}

// jsoncombiner overwrites basejson with addjson using mergemap
func jsonCombiner(baseJSON, addJSON json.RawMessage) ([]byte, error) {
	// make the base from the previous additons
	base := make(map[string]interface{})
	err := yaml.Unmarshal(baseJSON, &base)
	if err != nil {

		return nil, fmt.Errorf("%v", err)
	}

	// get the map of the update
	add := make(map[string]interface{})
	err = yaml.Unmarshal(addJSON, &add)
	if err != nil {

		return nil, fmt.Errorf("%v", err)
	}

	// merge the maps and return the combined bytes
	combinedMap := mergemap.Merge(base, add)

	return yaml.Marshal(combinedMap)
}

// tree produces a tree showing the run order of all the widgets
func tree(bases map[string]widgetContents) {

	type stringArray struct {
		name  string
		array []int
	}
	// get the order they run in
	order := make([]stringArray, len(bases))
	for key, conts := range bases {
		if conts.Data != nil {
			order[conts.Pos] = stringArray{key, conts.arrayPos}
		}
	} // or based on their array depthsand positions
	depth := 0
	prev := []int{}
	for _, i := range order {
		position := bases[i.name]

		if i.name != "" {
			newDepth := len(position.arrayPos)
			// compare with the previous array
			// if they arent the same length then reassert to the depth
			// compare positions to find the point they change
			if len(prev) != len(i.array) {
				// change depth to be the shortest
				if len(prev) < len(i.array) {
					depth = len(prev)
				} else {
					depth = len(i.array)
				}

			} else {
				// find where the array changes position from the previous one
				for j, pos := range prev {
					if i.array[j] != pos {
						depth = j

						break
					}
				}
			}
			// generate any headers
			for newDepth-depth > 1 {
				depth++
				newdent := strings.SplitN(i.name, ".", depth+1)
				indent := strings.Repeat(" ", depth)
				header := strings.Join(newdent[:depth], ".")
				fmt.Println(indent + "└" + header)

			}
			indent := strings.Repeat(" ", newDepth)
			fmt.Println(indent + "├" + i.name)

			depth = newDepth
			prev = i.array
		}
	}
}

func mustacheErrorWrap(input, location string, metadata map[string]any) (string, error) {
	// ensure we don't allow missing varaibles
	mustache.AllowMissingVariables = false
	sUp, err := mustache.Render(input, metadata)

	if err != nil {
		/*
			what information is useful here
			location the error happened, the offending string
			err, path, ext, sUp

			I want to wrap the error which tells the main information and I want to locate it

			dot path won't really cut it but will for the mo
		*/
		err = fmt.Errorf("0007 %v in %s at %s", err, input, location)
	}

	return sUp, err
}
