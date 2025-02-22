package gridgen

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mrmxf/opentsg-core/config/core"
)

type TPIGCompile struct {
	TPIGS      []Imports  `json:"tpig include"`
	Dimensions Dimensions `json:"Dimensions"`
}

type Imports struct {
	Include string `json:"uri"`
	Alias   string `json:"alias"`
}

type TPIG struct {
	Tilelayout []Tilelayout    `json:"Tile layout"`
	Dimensions Dimensions      `json:"Dimensions"`
	Carve      map[string]XY2D `json:"Carve"`
	// NAme?
}

type Tilelayout struct {
	Name       string    `json:"Name"`
	Tags       []string  `json:"Tags"`
	Neighbours []string  `json:"Neighbours"`
	Layout     Positions `json:"Layout"`
}

type Positions struct {
	Carve XY `json:"Carve"`
	Flat  XY `json:"Flat"`
	Size  XY `json:"XY"`
}

type Dimensions struct {
	Carve XY2D `json:"Carve"`
	Flat  XY2D `json:"Flat"`
}

type XY struct {
	Destination string `json:"Destination,omitempty"`
	X           int    `json:"X"`
	Y           int    `json:"Y"`
}

type XY2D struct {
	X0 int `json:"X0"`
	Y0 int `json:"Y0"`
	X1 int `json:"X1"`
	Y1 int `json:"Y1"`
}

func flatmap(c *context.Context, tpigpath string) (canvasAndMask, error) {

	//update the path getting to be localised
	basePath := core.GetDir(*c)
	file, err := core.GetWebBytes(c, tpigpath)
	if err != nil {
		fullpath := filepath.Join(basePath, tpigpath)
		file, err = os.ReadFile(fullpath)
		if err != nil {
			return canvasAndMask{}, fmt.Errorf("0DEV error accessing the tpig file %v", err)
		}
	}

	var segmentLayout TPIG
	err = json.Unmarshal(file, &segmentLayout)
	if err != nil {
		return canvasAndMask{}, fmt.Errorf("0DEV error extracting the tpig file %v", err)
	}
	// remove the need for the map of art grid as this is more of a layer
	// keep carve as a map for naming convetions

	if len(segmentLayout.Tilelayout) == 0 {
		return canvasAndMask{}, fmt.Errorf("0DEV No geometry postions have been declared")
	}

	carveSegements := make(map[string]carvedImageLayout)
	// map[string]locationsandneighbours for other things to call it

	// TODO add streamlined error checking
	// take each area to start at 0,0 and cmpile them?
	flatbase := imageGenerator(*c, image.Rect(segmentLayout.Dimensions.Flat.X0, segmentLayout.Dimensions.Flat.Y0, segmentLayout.Dimensions.Flat.X1, segmentLayout.Dimensions.Flat.Y1))
	basemask := imageGenerator(*c, image.Rect(segmentLayout.Dimensions.Flat.X0, segmentLayout.Dimensions.Flat.Y0, segmentLayout.Dimensions.Flat.X1, segmentLayout.Dimensions.Flat.Y1))
	// create the empty mask here. Keep it as empty as we want only bits that match the
	// geometry layout.

	// TPIGS layout will just be one deep for the moment

	// Extract all the tile information
	utilitySegements := make([]Segmenter, len(segmentLayout.Tilelayout))

	locs := make([]image.Rectangle, len(segmentLayout.Tilelayout))

	// add the number to segements as an id for maintaing unqiueness
	for i, t := range segmentLayout.Tilelayout {
		// update the carve map here so the shape is etc
		// carve is an array of the original and the destination

		//colour in flat at the same time
		locs[i] = image.Rect(t.Layout.Flat.X, t.Layout.Flat.Y, t.Layout.Flat.X+t.Layout.Size.X, t.Layout.Flat.Y+t.Layout.Size.Y)

		utilitySegements[i] = Segmenter{
			Shape: locs[i],
			Tags:  t.Tags,
			Name:  t.Name, importPosition: i}

		// figure out the optimisation here, or error handling as not everything will be carved
		carves := image.Rect(t.Layout.Carve.X, t.Layout.Carve.Y, t.Layout.Carve.X+t.Layout.Size.X, t.Layout.Carve.Y+t.Layout.Size.Y)
		// extract the carve for each area, appending it to the carve map
		carved := carveSegements[t.Layout.Carve.Destination]
		carved.Layout = append(carveSegements[t.Layout.Carve.Destination].Layout, carveshift{destination: carves, target: locs[i]})
		carveSegements[t.Layout.Carve.Destination] = carved
		// fill in the global base mask
		draw.Draw(basemask, utilitySegements[i].Shape, &image.Uniform{color.NRGBA64{A: 0xffff}}, image.Point{}, draw.Src)
	}

	for k, v := range carveSegements {
		carveDimensions, ok := segmentLayout.Carve[k]
		if !ok {
			return canvasAndMask{}, fmt.Errorf("000DEV the key %v was declared as a carve location, but no dimensions were given", k)
		}
		// don't bother making the offset
		v.carveSize = image.Rect(carveDimensions.X0, carveDimensions.Y0, carveDimensions.X1, carveDimensions.Y1)
		carveSegements[k] = v
	}

	cmid := context.WithValue(*c, carvekey, carveSegements)
	cmid = context.WithValue(cmid, tilekey, utilitySegements)
	cmid = context.WithValue(cmid, tilemaskkey, basemask)
	*c = cmid

	return canvasAndMask{canvas: flatbase, mask: basemask}, nil
}

// Carve checks if the resulting image needs to be carved.
// Returning the carved image and amended target names for each carve
// , as well as the original image
func Carve(c *context.Context, canvas draw.Image, target []string) []ImageLocation {
	// take in the flat image and generate filename(tpigname).extension using string manipulation
	// save the flat and carved images at the moment

	/*
		get segments and carve information from a map
		create a mask of the complete thing to run over the whole bit

	*/
	carveTargets := (*c).Value(carvekey)
	//.(map[string]carver)

	if carveTargets != nil {
		carveTargets := carveTargets.(map[string]carvedImageLayout)
		carvedTargets := make([]ImageLocation, len(carveTargets)+1)

		count := 0
		for name, ct := range carveTargets {
			carved := imageGenerator(*c, ct.carveSize)

			for _, carve := range ct.Layout {
				draw.Draw(carved, carve.destination, canvas, carve.target.Min.Add(ct.offset), draw.Src)
			}

			names := make([]string, len(target))
			for i, t := range target {
				parts := strings.Split(t, ".")
				//double the last bit to substitute it
				parts[len(parts)-2] = strings.Join([]string{parts[len(parts)-2], name}, "")
				names[i] = strings.Join(parts, ".")
			}

			carvedTargets[count] = ImageLocation{Image: carved, Location: names}
			count++
		}
		//add the full image at the end
		carvedTargets[count] = ImageLocation{Image: canvas, Location: target}
		return carvedTargets
	}

	// return the original image if there's nothing to carve
	return []ImageLocation{{Image: canvas, Location: target}}

}

func splice(c *context.Context, x, y int, size image.Point) {

	//get the poistions here []segemnter
	geometryHolder := (*c).Value(tilekey) //, utilitySegements)

	//List the geometry per grid section
	var sections map[string][]Segmenter
	if geometryHolder != nil {
		geometry := geometryHolder.([]Segmenter)
		sections = splicetpig(geometry, x, y, size)
	} else {
		sections = splicegrid(x, y, size)
	}

	cmid := context.WithValue(*c, gridkey, sections)
	*c = cmid
}

func splicetpig(segments []Segmenter, x, y int, size image.Point) map[string][]Segmenter {
	sections := make(map[string][]Segmenter)
	for xpos := 0; xpos < size.X; xpos += x {

		for ypos := 0; ypos < size.Y; ypos += y {

			// generate the name for both methods of grid coordinates
			gridCoord := fmt.Sprintf("%v%v", gridToScale(xpos/x), ypos/y)
			gridRCCoord := fmt.Sprintf("R%vC%v", xpos/x+1, ypos/y+1)

			matches := []Segmenter{}
			bounding := image.Rect(xpos, ypos, xpos+x, ypos+y)

			// check every segment to see where if it is within the grid
			for _, g := range segments {
				if bounding.Min.X < g.Shape.Max.X && bounding.Max.X > g.Shape.Min.X &&
					bounding.Min.Y < g.Shape.Max.Y && bounding.Max.Y > g.Shape.Min.Y {
					matches = append(matches, g)
				}
			}
			sections[gridCoord] = matches
			sections[gridRCCoord] = matches

		}
	}
	return sections
}

func splicegrid(x, y int, size image.Point) map[string][]Segmenter {
	sections := make(map[string][]Segmenter)
	count := 0
	for xpos := 0; xpos < size.X; xpos += x {

		for ypos := 0; ypos < size.Y; ypos += y {
			bounding := image.Rect(xpos, ypos, xpos+x, ypos+y)
			// switch for neighbours. 4 if statememts if x != x then go back 1 etc
			gridCoord := fmt.Sprintf("%v%v", gridToScale(xpos/x), ypos/y)
			gridRCCoord := fmt.Sprintf("R%vC%v", xpos/x+1, ypos/y+1)

			tagsRC, tagsC := []string{}, []string{}

			// generate the neighbours using simple if statements for each position

			if xpos != 0 {
				tagsC = append(tagsRC, fmt.Sprintf("neighbour:%v%v", gridToScale((xpos/x)-1), ypos/y))
				tagsRC = append(tagsC, fmt.Sprintf("neighbour:R%vC%v", (xpos/x+1)-1, ypos/y+1))
			}

			if xpos+x < size.X {
				tagsC = append(tagsRC, fmt.Sprintf("neighbour:%v%v", gridToScale((xpos/x)+1), ypos/y))
				tagsRC = append(tagsC, fmt.Sprintf("neighbour:R%vC%v", (xpos/x+1)+1, ypos/y+1))
			}

			if ypos != 0 {
				tagsC = append(tagsRC, fmt.Sprintf("neighbour:%v%v", gridToScale((xpos/x)), (ypos/y)-1))
				tagsRC = append(tagsC, fmt.Sprintf("neighbour:R%vC%v", (xpos/x+1), (ypos/y)-1))
			}

			if ypos+y < size.Y {
				tagsC = append(tagsRC, fmt.Sprintf("neighbour:%v%v", gridToScale((xpos/x)), (ypos/y)+1))
				tagsRC = append(tagsC, fmt.Sprintf("neighbour:R%vC%v", (xpos/x+1), (ypos/y)+1))
			}

			sections[gridCoord] = []Segmenter{{Name: gridCoord, Shape: bounding, Tags: tagsC, importPosition: count}}
			sections[gridRCCoord] = []Segmenter{{Name: gridRCCoord, Shape: bounding, Tags: tagsRC, importPosition: count}}

			count++
		}
	}
	return sections
}

// gridToScale converts an x coordiante to excel letter notation.
// Where 0 is A, 1 is B etc
func gridToScale(x int) string {

	// results is the x value as excel coordinates
	results := make([]rune, 0)

	if x == 0 {
		results = append(results, 'A')
	} else {
		input := x
		for input > 0 {
			// generate mod with custom function to account for the excel style
			off, remainder := divMod(input, 26)
			input = off
			results = append(results, rune(96+remainder))
		}
	}

	//reverse the results
	for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
		results[i], results[j] = results[j], results[i]
	}
	return string(results)

}

// Get GridGeometry returns the geometry of a grid coordiante, localised to those grid coordinates.
// This is for use with widgets that utilise geometry.
func GetGridGeometry(c *context.Context, coordinate string) ([]Segmenter, error) {
	positions, err := getGridGeometry(c, coordinate)

	if err != nil {
		return []Segmenter{}, err
	}

	// cleanse by adding a number to the base
	// cleanse positions of duplicate entries

	cleanorder := make(map[int]Segmenter)
	for _, pos := range positions {
		cleanorder[pos.importPosition] = pos
	}

	// get the positions of all the ones called
	// then order them
	keys := make([]int, len(cleanorder))
	i := 0
	for k := range cleanorder {
		keys[i] = k
		i++
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	//add teh values in order they were declared
	cleanSegments := make([]Segmenter, len(cleanorder))
	for i, pos := range keys {
		cleanSegments[i] = cleanorder[pos]
	}

	return cleanSegments, err

}

// getGridGeometry breaks the location into every grid location it covers.
// And extracts the results from the map of coordiantes and their geometry.
func getGridGeometry(c *context.Context, coordinate string) ([]Segmenter, error) {
	coordinate = strings.ToUpper(coordinate)
	sections := (*c).Value(gridkey).(map[string][]Segmenter)

	// get all the sections
	//if they are 1 grid return sections[coordinate]

	//utilising the regex
	regSing := regexp.MustCompile("^[a-zA-Z]{1,3}[0-9]{1,3}$")
	regArea := regexp.MustCompile("^[a-zA-Z]{1,3}[0-9]{1,3}:[a-zA-Z]{1,3}[0-9]{1,3}$")
	regAlias := regexp.MustCompile(`^[\w\W]{1,30}$`)
	squareX := (*c).Value(xkey).(int)
	squareY := (*c).Value(ykey).(int)
	regRC := regexp.MustCompile(`^[Rr]([\d]{2,}|[1-9]{1})[Cc]([\d]{2,}|[1-9]{1})$`)
	regRCArea := regexp.MustCompile(`^[Rr]([\d]{2,}|[1-9]{1})[Cc]([\d]{2,}|[1-9]{1}):[Rr]([\d]{2,}|[1-9]{1})[Cc]([\d]{2,}|[1-9]{1})$`)

	aliasMap := core.GetAlias(*c)

	// check what the location is
	switch {
	case regSing.MatchString(coordinate):
		coordinateLow := strings.ToLower(coordinate)
		x, y, err := gridSplit(coordinateLow)
		if err != nil {

			return []Segmenter{}, err
		}

		offseted := segementWithOffset(image.Point{-x * squareX, -y * squareY}, sections[coordinate])
		return offseted, nil
	case regArea.MatchString(coordinate):
		// gridSplit(gridString) //split it around :
		coordinateLow := strings.ToLower(coordinate)
		grids := strings.Split(coordinateLow, ":")
		x, y, err := gridSplit(grids[0])
		if err != nil {

			return []Segmenter{}, err
		}
		xend, yend, err := gridSplit(grids[1])
		if err != nil {

			return []Segmenter{}, err
		}

		// make sure the coordinates are in a valid direction
		var segements []Segmenter

		if xend < x || yend < y {

			return segements, fmt.Errorf(invalidCoordinates, coordinate, x, y, xend, yend)

		}

		for xpos := x; xpos <= xend; xpos++ {
			for ypos := y; ypos <= yend; ypos++ {
				grid := fmt.Sprintf("%v%v", gridToScale(xpos), ypos)
				segements = append(segements, segementWithOffset(image.Point{-x * squareX, -y * squareY}, sections[grid])...)
			}
		}

		return segements, nil

	case regRC.MatchString(coordinate):

		x, y := 0, 0
		fmt.Sscanf(coordinate, "R%dC%d", &x, &y)
		offseted := segementWithOffset(image.Point{-(x - 1) * squareX, -(y - 1) * squareY}, sections[coordinate])
		return offseted, nil

	case regRCArea.MatchString(coordinate):

		xs, ys, xe, ye := 0, 0, 0, 0
		fmt.Sscanf(coordinate, "R%dC%d:R%dC%d", &xs, &ys, &xe, &ye)

		var segements []Segmenter
		if xe < xs || ye < ys {

			return segements, fmt.Errorf(invalidCoordinates, coordinate, xs, ys, xe, ye)
		}
		// get square locations
		for xpos := xs; xpos <= xe; xpos++ {
			for ypos := ys; ypos <= ye; ypos++ {
				grid := fmt.Sprintf("R%vC%v", xpos, ypos)
				segements = append(segements, segementWithOffset(image.Point{-(xs - 1) * squareX, -(ys - 1) * squareY}, sections[grid])...)
			}
		}
		return segements, nil

	case regAlias.MatchString(coordinate):
		loc := aliasMap.Data[coordinate]
		if loc != "" {
			// call the function again but with the required coordinates
			return getGridGeometry(c, coordinate)
		} else {

			return nil, fmt.Errorf(invalidAlias, coordinate)
		}

	default:

		return []Segmenter{}, fmt.Errorf(invalidGrid, coordinate)
	}

}

// segment with offset applies an offset to a slice of Segmenter
func segementWithOffset(offset image.Point, input []Segmenter) []Segmenter {
	output := make([]Segmenter, len(input))

	for i, seg := range input {
		seg.Shape = seg.Shape.Add(offset)
		output[i] = seg
	}

	return output
}

type ImageLocation struct {
	Image    draw.Image
	Location []string
}

type carvedImageLayout struct {
	Layout    []carveshift
	offset    image.Point
	carveSize image.Rectangle
}

type carveshift struct {
	target, destination image.Rectangle
}

type Segmenter struct {
	Name           string
	Shape          image.Rectangle
	Tags           []string // neighbours will be included in a string? match to neighbours then
	importPosition int
}
