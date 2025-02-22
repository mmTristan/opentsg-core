// Package Core is used for handling factory objects for imports and frame generation
package core

import (
	"context"
	"encoding/json"
	"os"
	"sync"

	_ "embed"

	"github.com/mrmxf/opentsg-core/config/validator"
	"github.com/mrmxf/opentsg-core/credentials"
)

////////////////
// This is a base of useful things that are used across the config files. Will be designed to be something used by each one

type testKey struct {
	tag   string
	added int
}

var (
	updates = testKey{"update key for the array of objects", 0}
	baseKey = testKey{"base key for widgets", 1}
	//	widgetbases  = testKey{"widget bases", 2}
	frameHolders    = testKey{"The key for holding all the generated json", 3}
	aliasKey        = testKey{"base for aliases to run through out the program", 4}
	lines           = testKey{"the holder of the hashes of the name+content for line numbers and files", 5}
	addedWidgets    = testKey{"the key to access the list of added widgets to find missed aliases", 6}
	factoryDir      = testKey{"the directory of the main widget factory and everything is relative to", 7}
	credentialsAuth = testKey{"the holder of all the auth information provided by the user for accessing http sources", 7}
)

// then the rest is additions to the alias

type factory struct {
	Include  []factoryarr                `json:"include,omitempty" yaml:"include,omitempty"`
	Args     []arguments                 `json:"args" yaml:"args"`
	Create   []map[string]map[string]any `json:"create" yaml:"create"`
	Generate []generate                  `json:"generate" yaml:"generate"`
	// ADD a middleawre section here, may be difficult to keep tabs on
}

type arguments struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

// getArgs returns the list of arguments from a factory
// we only need the list of argument names and no their description.
func (f factory) getArgs() []string {
	results := make([]string, len(f.Args))
	for i, k := range f.Args {
		results[i] = k.Name
	}

	return results
}

type factoryarr struct {
	URI  string `json:"uri,omitempty" yaml:"uri,omitempty"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type generate struct {
	Name   []map[string]string            `json:"name" yaml:"name"` // map[string]any add the error handling later
	Range  []string                       `json:"range" yaml:"range"`
	Action map[string]map[string][]string `json:"action" yaml:"action"` // target(s)  data  updates
}

//go:embed jsonschema/includeschema.json
var incschema []byte

// Processing structs
type base struct {
	authBody              credentials.Decoder
	jsonFileLines         validator.JSONLines
	importedFactories     map[string]factory
	importedWidgets       map[string]json.RawMessage
	generatedFrameWidgets map[string]widgetContents
}

// widgetContents is the content of each widget within a frame and its position in the run order
type widgetContents struct {
	Data     json.RawMessage
	Pos      int
	arrayPos []int
	Tag      string
}

// AliasIdentity is the name and zposition of a widget. Where zposition is the widgets poisition in the global array of widgets
type AliasIdentity struct {
	Alias string
	ZPos  int
}

// SyncMap  is a map with a sync.Mutex to prevent concurrent writes.
type SyncMap struct {
	Data map[string]string
	Mu   *sync.Mutex
}

// Get alias returns a map of the locations alias and their grid positions.
func GetAlias(c context.Context) SyncMap {
	Alias := c.Value(aliasKey)
	if Alias != nil {

		return Alias.(SyncMap)
	}
	//else return an empty map
	var newmu sync.Mutex

	return SyncMap{Mu: &newmu, Data: make(map[string]string)}
}

// GetFrameWidgets returns a map of the alias
func GetFrameWidgets(c context.Context) map[string]widgetContents {

	return c.Value(baseKey).(map[string]widgetContents)
}

// GetApplied widgets returns a syncMap that contains all the widget names that have been assigned an alias
func GetAppliedWidgets(c context.Context) SyncMap {

	return c.Value(addedWidgets).(SyncMap)
}

// GetJSONLines returns the hash map of all the imported files and their lines.
// This is for use in conjunction with the validator package
func GetJSONLines(c context.Context) validator.JSONLines {

	return c.Value(lines).(validator.JSONLines)
}

// PutAlias inits a map of the alias in a context
func PutAlias(c context.Context) context.Context {
	n := SyncMap{make(map[string]string), &sync.Mutex{}}

	return context.WithValue(c, aliasKey, n)
}

// GetDir returns the directory that the base factory resides in.
func GetDir(c context.Context) string {
	s, ok := c.Value(factoryDir).(string)
	if !ok {
		s, _ := os.Getwd()

		return s
	}

	return s
}

// GetWebBytes is a wrapper of `credentials` where the configuration body is stored in config.
// This is to prevent several intialisations of the authbody or the data being passed around.
func GetWebBytes(c *context.Context, URI string) ([]byte, error) {

	auth, ok := (*c).Value(credentialsAuth).(credentials.Decoder)
	//if there is not an authorisation body make a new one with no credentials
	if !ok {
		var err error
		auth, err = credentials.AuthInit("")
		if err != nil {
			return nil, err
		}
	}

	return auth.Decode(URI)
}
