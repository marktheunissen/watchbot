package render

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/olekukonko/tablewriter"
	"github.com/sirupsen/logrus"
	"golang.org/x/image/font"
)

var log = logrus.WithField("component", "render")

func TableJpeg(data [][]string, header []string) (*bytes.Buffer, error) {
	jpegBytes := &bytes.Buffer{}

	// Create the text table, render it into a bytes buffer.
	textBytes := &bytes.Buffer{}
	table := tablewriter.NewWriter(textBytes)
	table.SetHeader(header)
	for _, v := range data {
		table.Append(v)
	}
	table.SetBorder(false)
	table.Render()

	// Create an image context and print the text table into it.
	dc := gg.NewContext(380, 380)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	dc.SetRGB(0, 0, 0)

	// Load the font
	face, err := loadFont()
	if err != nil {
		return jpegBytes, err
	}
	dc.SetFontFace(face)

	// Draw using the font onto a new image.
	for i, s := range strings.Split(textBytes.String(), "\n") {
		dc.DrawString(s, 0, 20+(14*float64(i)))
	}

	// Save the image as a jpeg
	pngBytes := &bytes.Buffer{}
	err = dc.EncodePNG(pngBytes)
	if err != nil {
		return jpegBytes, err
	}
	img, _, err := image.Decode(pngBytes)
	if err != nil {
		return jpegBytes, err
	}
	err = jpeg.Encode(jpegBytes, img, &jpeg.Options{Quality: 95})
	if err != nil {
		return jpegBytes, err
	}

	// log.Debug(textBytes.String())

	return jpegBytes, nil
}

func loadFont() (font.Face, error) {
	fontBytes, err := Asset("fonts/Inconsolata-Regular.ttf")
	if err != nil {
		return nil, err
	}
	f, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}
	face := truetype.NewFace(f, &truetype.Options{
		Size: 14,
	})
	return face, nil
}
