package core

import (
	"fmt"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*




make this test a check the whole thing runs as intended

var mockSchema = []byte(`{
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"$id": "https://example.com/product.schema.json",
	"title": "Allow anything through for tests",
	"description": "An empty schema to allow custom structs to run through",
	"type": "object"
	}`)*/

func init() {
	location, _ = os.Getwd()
	sep = string(os.PathSeparator)
}

var location string
var sep string

// make a test for the json init stage
func TestFileRead(t *testing.T) {

	_, frames, realfile := FileImport("./testdata/newfactory.json", "", false)
	// z := GetBlackVal(0)
	Convey("Checking an exisiting file is read", t, func() {
		Convey("using ./testdata/newfactory.json as the input file", func() {
			Convey("No error is returned and the number of frames returned should be 2", func() {
				So(realfile, ShouldBeNil)
				So(frames, ShouldEqual, 2)
			})
		})
	})

	badFile := []string{"./testdata/fake.json", "./testdata/apitest.png", "", "./testdata/repeatalias.json", "./testdata/frame_generate/errors/sequence_recurse.json"}
	badFileErr := []string{fmt.Sprintf("0001 open %s%stestdata%sfake.json: no such file or directory", location, sep, sep),
		fmt.Sprintf("0028 yaml: invalid leading UTF-8 octet for extracting the yaml bytes from %s%stestdata%sapitest.png", location, sep, sep),
		fmt.Sprintf("0001 read %s: is a directory", location), "0006 the alias robocorner is repeated, every alias is required to be unique",
		"0004 recursive set initialisation file detected, the maximum dotpath depth of 30 has been reached",
	}

	for i := range badFile {
		fmt.Println(location)
		_, _, err := FileImport(badFile[i], "", false)
		Convey("Checking if bad files are read and the errors are returned", t, func() {
			Convey(fmt.Sprintf("using %v as the input file", badFile[i]), func() {
				Convey(fmt.Sprintf("An error of %v is returned", badFileErr[i]), func() {
					So(err.Error(), ShouldEqual, badFileErr[i])
				})
			})
		})
	}
}

func TestBadJson(t *testing.T) {
	testFolderLocation := fmt.Sprintf("%stestdata%swrong%s", sep, sep, sep)

	badFiles := []string{"./testdata/wrong/apiinval.json", "./testdata/wrong/empty.json",
		"./testdata/wrong/badinclude.json", "./testdata/wrong/badincludebase.json"}
	results := []string{fmt.Sprintf("0003 No frames declared in %s%sapiinval.json", location, testFolderLocation),
		fmt.Sprintf("0002 yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into core.factory when opening %s%sempty.json", location, testFolderLocation),
		fmt.Sprintf("0003 No frames declared in %s%sbadinclude.json", location, testFolderLocation),
		fmt.Sprintf("0002 yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into core.factory when opening %s%sbadincludebase.json", location, testFolderLocation)}

	for i, bf := range badFiles {
		_, _, realfile := FileImport(bf, "", false)

		Convey("Checking errors are caught for invalid json", t, func() {
			Convey(fmt.Sprintf("using %v as the input file", bf), func() {
				Convey(fmt.Sprintf("%v is returned", results[i]), func() {
					So(realfile.Error(), ShouldResemble, results[i])
				})
			})
		})
	}
}



func TestJsonRread(t *testing.T) {
	inputFile := "./testdata/frame_generate/sequence.json"
	c, _, _ := FileImport(inputFile, "", false)

	predictedValues := []string{"./testdata/frame_generate/results/blue.yaml", "./testdata/frame_generate/results/green.yaml"}

	for i, pv := range predictedValues {
		n, _ := FrameWidgetsGenerator(c, i, false)

		expec, got := genHash(n, pv)

		Convey("Checking arguments are parsed correctly both in the create and generate section of json factories", t, func() {
			Convey(fmt.Sprintf("Using frame %v ./testdata/sequnce.json as the input ", i), func() {
				Convey("The generated widget map as a json body matches "+pv, func() {
					So(expec.Sum(nil), ShouldResemble, got.Sum(nil))
				})
			})
		})
	}
}
func TestYamlRead(t *testing.T) {
	inputYamls := []string{"./testdata/frame_generate/yaml_test/sequence.yaml",
		"./testdata/frame_generate/yaml_test/sequence_frame.yaml", "./testdata/frame_generate/yaml_test/sequence_full.yaml"}

	predictedValuesYaml := []string{"./testdata/frame_generate/results/blue.yaml", "./testdata/frame_generate/results/green.yaml"}
	yamlMix := []string{"an input yaml file", "a mix of yaml and json files", "a complete set of yaml files"}
	for j, inputYaml := range inputYamls {
		cYaml, _, _ := FileImport(inputYaml, "", false)

		for i, pv := range predictedValuesYaml {
			n, _ := FrameWidgetsGenerator(cYaml, i, false)
			expec, got := genHash(n, pv)

			Convey("Checking arguments are parsed correctly both in the create and generate section of yaml json factories", t, func() {
				Convey(fmt.Sprintf("using frame %v of %s this contains a %s", i, inputYaml, yamlMix[j]), func() {
					Convey("The generated widget map as a json body matches "+pv, func() {
						So(expec.Sum(nil), ShouldResemble, got.Sum(nil))
					})
				})
			})
		}
	}
}
