// package canvaswidget contains all the methods for extracting canvas properties to be used by other values
package canvaswidget

import (
	"context"
	"fmt"
	"image"
	"image/draw"

	_ "embed"

	"github.com/mrmxf/opentsg-core/config"
	"github.com/mrmxf/opentsg-core/config/widgets"
)

type key struct {
	k string
}

var (
	generatedConfig = key{"the key for holding all the configuration values"}
)

// ConfigVals is the go struct of all the configuration values that may be called by an input.
type ConfigVals struct {
	Type        string           `json:"type" yaml:"type"`
	Name        []string         `json:"name,omitempty" yaml:"name,omitempty"`
	Framesize   config.Framesize `json:"frameSize,omitempty" yaml:"frameSize,omitempty"`
	LineWidth   float64          `json:"linewidth,omitempty" yaml:"linewidth,omitempty"`
	FileDepth   int              `json:"filedepth,omitempty" yaml:"filedepth,omitempty"`
	GridRows    int              `json:"gridRows,omitempty" yaml:"gridRows,omitempty"`
	GridColumns int              `json:"gridColumns,omitempty" yaml:"gridColumns,omitempty"`
	BaseImage   string           `json:"baseImage,omitempty" yaml:"baseImage,omitempty"`
	Geometry    string           `json:"geometry,omitempty" yaml:"geometry,omitempty"`
	LineColor   string           `json:"lineColor,omitempty" yaml:"lineColor,omitempty"`
	Background  string           `json:"backgroundFillColor,omitempty" yaml:"backgroundFillColor,omitempty"`
	ImageType   string           `json:"imageType,omitempty" yaml:"imageType,omitempty"`
	Analytics   analytics        `json:"frame analytics" yaml:"frame analytics"`
}

type analytics struct {
	Configs enabled `json:"configuration" yaml:"configuration"`
	Average enabled `json:"average color" yaml:"average color"`
	PHash   enabled `json:"phash" yaml:"phash"`
}

type enabled struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

/*The following functions allow it to be mocked as a widget*/

// Alias is used for mocking the canvas as a widget To make it a widgethandler.Generator
func (c ConfigVals) Alias() string {
	return ""
}

// Location is used for mocking the canvas as a widget. To make it a widgethandler.Generator
func (c ConfigVals) Location() string {
	return "A0" // the grid that is always guaranteed to be there
}

// Generate is used for mocking the canvas as a widget, it just returns nil and
// is designed to always successfully run.
func (c ConfigVals) Generate(canvas draw.Image, opts ...any) error {
	return nil
}

//go:embed jsonschema/baseschema.json
var baseschema []byte

// Loop init extracts and applies the canvas properties for each frame.
// This is to be run as the first step after generating the frame widgets,
// because other modules rely on this information for generating their own structs.
func LoopInit(frameContext *context.Context) []error {
	conf, errs := widgets.ExtractWidgetStructs[ConfigVals]("builtin.canvasoptions", baseschema, frameContext)

	if errs != nil {
		return errs
	}
	globParams := ConfigVals{}

	if len(conf) == 1 {
		for _, v := range conf {
			globParams = v
		}
	} else {

		return []error{fmt.Errorf("0061 %v configs have been assigned, only assign one config", len(conf))}
	}

	midC := context.WithValue(*frameContext, generatedConfig, globParams)
	*frameContext = midC // update the context pointer

	return []error{}
}

// GetLWidth returns the width of the gridlines
func GetLWidth(c context.Context) float64 {
	g := contToConf(c)

	return g.LineWidth
}

// GetFileDepth returns the bitdepth for the image to be saved at.
// This only interacts with the dpx file, all other file types are saved 16 bit
func GetFileDepth(c context.Context) int {
	g := contToConf(c)

	return g.FileDepth
}

func contToConf(c context.Context) ConfigVals {
	val := c.Value(generatedConfig)
	if val != nil {
		g := val.(ConfigVals)

		return g
	}
	// else return an empty struct which may cause breakages down the line
	return ConfigVals{}
}

// GetFileType returns the file name for the image to be saved.
// e.g. "multiramp-4b-pc-hd"
func GetFileName(c context.Context) []string {
	g := contToConf(c)

	return g.Name
}

// GetGridRows returns the number of rows required, the minimum returned value is 1
func GetGridRows(c context.Context) int {
	g := contToConf(c)
	if g.GridRows == 0 {
		return 1
	}

	return g.GridRows
}

// GetGridColumns returns the number of columns required, the minimum returned value is 1
func GetGridColumns(c context.Context) int {
	g := contToConf(c)
	if g.GridColumns == 0 {

		return 1
	}

	return g.GridColumns
}

// GetBaseImage returns the string of the image location to be used as a background
func GetBaseImage(c context.Context) string {
	g := contToConf(c)

	return g.BaseImage
}

// GetGeometry returns the string of the geometry location to be used as the structure
func GetGeometry(c context.Context) string {
	g := contToConf(c)

	return g.Geometry
}

// GetFillColour returns the colour string of the background
func GetFillColour(c context.Context) string {
	g := contToConf(c)

	return g.Background
}

// GetLineColour returns the user defined colour string sof the grid lines
func GetLineColour(c context.Context) string {
	g := contToConf(c)

	return g.LineColor
}

// GetPictureSize returns the image size as an image.Point so it can be used without
// manipulation for generating the canvas
func GetPictureSize(c context.Context) image.Point {
	g := contToConf(c)

	return image.Point{g.Framesize.W, g.Framesize.H}
}

// GetCanvasType returns the type of image to be used for the testcard.
// With either "ACES" or "NRGBA64" as available strings
func GetCanvasType(c context.Context) string {
	g := contToConf(c)

	return g.ImageType
}

// GetCanvasSchema exports the schema for canvas widget
func GetCanvasSchema() []byte {

	return baseschema
}

// GetMetaConfiguration exports if the metadata feature has been enabled
func GetMetaConfiguration(c context.Context) bool {
	g := contToConf(c)

	return g.Analytics.Configs.Enabled
}

// GetMetaPhash exports if the phash has been calculated
func GetMetaPhash(c context.Context) bool {
	g := contToConf(c)

	return g.Analytics.PHash.Enabled
}

// GetMetaAverage exports if the metadata feature has been enabled for average colour
func GetMetaAverage(c context.Context) bool {
	g := contToConf(c)

	return g.Analytics.Average.Enabled
}
