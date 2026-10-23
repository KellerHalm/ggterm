package ui

import (
	"bytes"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gomono"

	"ggterm/internal/terminal"
)

var (
	themeBG     = color.RGBA{R: 0x0f, G: 0x11, B: 0x17, A: 0xff}
	themeFG     = color.RGBA{R: 0xc9, G: 0xd1, B: 0xd9, A: 0xff}
	themeCursor = color.RGBA{R: 0x58, G: 0xa6, B: 0xff, A: 0xc0}
)

var ansiPalette = [16]color.RGBA{
	{R: 0x48, G: 0x4f, B: 0x58, A: 0xff},
	{R: 0xff, G: 0x7b, B: 0x72, A: 0xff},
	{R: 0x3f, G: 0xb9, B: 0x50, A: 0xff},
	{R: 0xd2, G: 0x99, B: 0x22, A: 0xff},
	{R: 0x58, G: 0xa6, B: 0xff, A: 0xff},
	{R: 0xbc, G: 0x8c, B: 0xff, A: 0xff},
	{R: 0x39, G: 0xc5, B: 0xcf, A: 0xff},
	{R: 0xb1, G: 0xba, B: 0xc4, A: 0xff},
	{R: 0x6e, G: 0x76, B: 0x81, A: 0xff},
	{R: 0xff, G: 0xa1, B: 0x98, A: 0xff},
	{R: 0x56, G: 0xd3, B: 0x64, A: 0xff},
	{R: 0xe3, G: 0xb3, B: 0x41, A: 0xff},
	{R: 0x79, G: 0xc0, B: 0xff, A: 0xff},
	{R: 0xd2, G: 0xa8, B: 0xff, A: 0xff},
	{R: 0x56, G: 0xd4, B: 0xdd, A: 0xff},
	{R: 0xf0, G: 0xf6, B: 0xfc, A: 0xff},
}

func newFace(size float64) (*text.GoTextFace, text.Metrics, error) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(gomono.TTF))
	if err != nil {
		return nil, text.Metrics{}, err
	}
	face := &text.GoTextFace{Source: source, Size: size}
	return face, face.Metrics(), nil
}

func cellColors(style terminal.Style) (color.RGBA, color.RGBA) {
	fg := resolveColor(style.FG, themeFG)
	bg := resolveColor(style.BG, themeBG)
	if style.Bold && style.FG.Mode == terminal.ColorIndexed && style.FG.Index < 8 {
		fg = ansiPalette[style.FG.Index+8]
	}
	if style.Reverse {
		fg, bg = bg, fg
	}
	if style.Faint {
		fg.R = uint8(int(fg.R) * 0xa0 / 0xff)
		fg.G = uint8(int(fg.G) * 0xa0 / 0xff)
		fg.B = uint8(int(fg.B) * 0xa0 / 0xff)
		fg.A = 0xa0
	}
	return fg, bg
}

func resolveColor(value terminal.Color, fallback color.RGBA) color.RGBA {
	switch value.Mode {
	case terminal.ColorIndexed:
		return indexedColor(value.Index)
	case terminal.ColorRGB:
		return color.RGBA{R: value.R, G: value.G, B: value.B, A: 0xff}
	default:
		return fallback
	}
}

func indexedColor(index uint8) color.RGBA {
	if index < 16 {
		return ansiPalette[index]
	}
	if index < 232 {
		levels := [6]uint8{0, 95, 135, 175, 215, 255}
		n := int(index) - 16
		return color.RGBA{R: levels[n/36], G: levels[(n/6)%6], B: levels[n%6], A: 0xff}
	}
	gray := uint8(8 + (int(index)-232)*10)
	return color.RGBA{R: gray, G: gray, B: gray, A: 0xff}
}
