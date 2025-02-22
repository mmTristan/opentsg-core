package colourgen

import (
	"fmt"
	"image/color"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGoodHex(t *testing.T) {

	goodNoAlpha := []string{"#C06090", "#C69", "rgb(192,96,144)"}

	for i := range goodNoAlpha {
		genC := HexToColour(goodNoAlpha[i])
		Convey("Checking a known string input", t, func() {
			Convey(fmt.Sprintf("using a %s as the hex colour", goodNoAlpha[i]), func() {
				Convey("A purple colour is returned, of R 192, of G 96 of B144", func() {
					So(genC, ShouldResemble, color.NRGBA{R: 192, G: 96, B: 144, A: 255})
				})
			})
		})
	}

}

func TestGoodHexAlpha(t *testing.T) {

	goodAlpha := []string{"#C06090f0", "#C69f", "rgba(192,96,144,240)", "rgb12(4095,1023,255)", "rgba12(4095,1023,255,4095)"}
	expect := []color.Color{color.NRGBA{R: 192, G: 96, B: 144, A: 240}, color.NRGBA{R: 192, G: 96, B: 144, A: 240},
		color.NRGBA{R: 192, G: 96, B: 144, A: 240}, color.NRGBA64{65520, 16368, 4080, 0xffff}, color.NRGBA64{65520, 16368, 4080, 65520}}
	for i := range goodAlpha {
		genC := HexToColour(goodAlpha[i])
		Convey("Checking a known string input", t, func() {
			Convey(fmt.Sprintf("using a %s as the hex colour", goodAlpha[i]), func() {
				Convey("A purple colour is returned, a R of 192, a G of 96, a B of 144 and an A of 240", func() {
					So(genC, ShouldResemble, expect[i])
				})
			})
		})
	}
}

func TestBadHex(t *testing.T) {

	badIn := []string{"#CgA649", "realbad", "rgba(243,56,78)", "rgb(20,20,20,20)", "rgba12(20,20,20,4096)"}

	for i := range badIn {
		// these check if they somehow make it through the initial json regex that no value is returned
		genC := HexToColour(badIn[i])
		Convey("Checking an invalid hex code is fenced by regex", t, func() {
			Convey(fmt.Sprintf("using a %s as the hex colour", badIn[i]), func() {
				Convey("No Colour is returned as g is an invalid hex code", func() {
					So(genC, ShouldResemble, color.NRGBA{R: 0, G: 0, B: 0, A: 0})
				})
			})
		})
	}
}

func TestAssign(t *testing.T) {

	colourCheck := []string{"grey", "black", "white", "red", "green", "blue"}
	expectedOutput := [][3]int{
		{200, 200, 200},
		{0, 0, 0},
		{4095, 4095, 4095},
		{200, 0, 0},
		{0, 200, 0},
		{0, 0, 200},
	}

	for i := range colourCheck {
		// these check if they somehow make it through the initial json regex that no value is returned
		testRGB, err := AssignRGBValues(colourCheck[i], 200, 0, 4095)
		Convey("Checking an invalid hex code is fenced by regex", t, func() {
			Convey(fmt.Sprintf("using a %s as the hex colour", "R"), func() {
				Convey("No Colour is returned as g is an invalid hex code", func() {
					So(testRGB, ShouldResemble, expectedOutput[i])
					So(err, ShouldBeNil)
				})
			})
		})
	}
}

func TestConvert(t *testing.T) { 

	cToCheck := []color.NRGBA{{100, 88, 66, 240}, {100, 88, 66, 255}, {R: 194, G: 166, B: 73, A: 255}}
	expec := []color.NRGBA64{{R: 25600, G: 22528, B: 16896, A: 61440}, {R: 25600, G: 22528, B: 16896, A: 65535}, {R: 49664, G: 42496, B: 18688, A: 65535}} 
	// {25600, 22528, 16896, 62464}, {25600, 22528, 16896, 65535}}
	for i, c := range cToCheck {
		// these check if they somehow make it through the initial json regex that no value is returned
		genC := ConvertNRGBA64(c)
		fmt.Println(c)
		Convey("Checking rgba are converted to nrgba64", t, func() {
			Convey(fmt.Sprintf("using a %v as the input colour", c), func() {
				Convey("A converted colour is returned", func() {
					So(genC, ShouldResemble, expec[i])
				})
			})
		})
	}
}
