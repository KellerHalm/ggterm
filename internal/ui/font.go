package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gomono"

	"ggterm/internal/config"
	"ggterm/internal/terminal"
)

func loadFace(font config.Font) (text.Face, text.Metrics, error) {
	primary, err := faceSource(font.Path, font.Family)
	if err != nil {
		return nil, text.Metrics{}, err
	}
	faces := []text.Face{&text.GoTextFace{Source: primary, Size: font.Size}}
	for _, fallback := range font.Fallback {
		source, err := faceSource(fallback, fallback)
		if err != nil {
			return nil, text.Metrics{}, fmt.Errorf("fallback font: %w", err)
		}
		faces = append(faces, &text.GoTextFace{Source: source, Size: font.Size})
	}
	if len(faces) == 1 {
		return faces[0], faces[0].Metrics(), nil
	}
	multi, err := text.NewMultiFace(faces...)
	if err != nil {
		return nil, text.Metrics{}, err
	}
	return multi, multi.Metrics(), nil
}

func faceSource(path, family string) (*text.GoTextFaceSource, error) {
	entry := strings.TrimSpace(path)
	if entry == "" {
		entry = strings.TrimSpace(family)
	}
	if entry == "" {
		return sourceFromBytes(gomono.TTF)
	}
	switch strings.ToLower(entry) {
	case "go mono", "gomono", "go-mono", "default", "monospace", "mono":
		return sourceFromBytes(gomono.TTF)
	}
	data, err := os.ReadFile(entry)
	if err != nil {
		if path == "" {
			return nil, fmt.Errorf("font %q: only the bundled Go Mono face is available by name; set font.path to a .ttf/.otf file", entry)
		}
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(entry)) {
	case ".ttc", ".otc":
		sources, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("font collection %q is empty", entry)
		}
		return sources[0], nil
	}
	return sourceFromBytes(data)
}

func sourceFromBytes(data []byte) (*text.GoTextFaceSource, error) {
	return text.NewGoTextFaceSource(bytes.NewReader(data))
}

func themeColors(theme config.Theme, style terminal.Style) (color.RGBA, color.RGBA) {
	fg := resolveColor(style.FG, theme.Foreground, theme.Palette)
	bg := resolveColor(style.BG, theme.Background, theme.Palette)
	if style.Bold && style.FG.Mode == terminal.ColorIndexed && style.FG.Index < 8 {
		fg = theme.Palette[style.FG.Index+8]
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

func resolveColor(value terminal.Color, fallback color.RGBA, palette [16]color.RGBA) color.RGBA {
	switch value.Mode {
	case terminal.ColorIndexed:
		return indexedColor(palette, value.Index)
	case terminal.ColorRGB:
		return color.RGBA{R: value.R, G: value.G, B: value.B, A: 0xff}
	default:
		return fallback
	}
}

func indexedColor(palette [16]color.RGBA, index uint8) color.RGBA {
	if index < 16 {
		return palette[index]
	}
	if index < 232 {
		levels := [6]uint8{0, 95, 135, 175, 215, 255}
		n := int(index) - 16
		return color.RGBA{R: levels[n/36], G: levels[(n/6)%6], B: levels[n%6], A: 0xff}
	}
	gray := uint8(8 + (int(index)-232)*10)
	return color.RGBA{R: gray, G: gray, B: gray, A: 0xff}
}
