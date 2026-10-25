package config

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Theme is the complete colour scheme used by the renderer.
type Theme struct {
	Name string

	Background          color.RGBA
	Foreground          color.RGBA
	Cursor              color.RGBA
	BarBackground       color.RGBA
	BarForeground       color.RGBA
	TabActiveBackground color.RGBA
	TabActiveForeground color.RGBA
	Border              color.RGBA
	BorderActive        color.RGBA
	OverlayBackground   color.RGBA

	Palette [16]color.RGBA
}

type themeFields struct {
	Background          *string  `toml:"background" json:"background" yaml:"background"`
	Foreground          *string  `toml:"foreground" json:"foreground" yaml:"foreground"`
	Cursor              *string  `toml:"cursor" json:"cursor" yaml:"cursor"`
	BarBackground       *string  `toml:"bar_background" json:"bar_background" yaml:"bar_background"`
	BarForeground       *string  `toml:"bar_foreground" json:"bar_foreground" yaml:"bar_foreground"`
	TabActiveBackground *string  `toml:"tab_active_background" json:"tab_active_background" yaml:"tab_active_background"`
	TabActiveForeground *string  `toml:"tab_active_foreground" json:"tab_active_foreground" yaml:"tab_active_foreground"`
	Border              *string  `toml:"border" json:"border" yaml:"border"`
	BorderActive        *string  `toml:"border_active" json:"border_active" yaml:"border_active"`
	OverlayBackground   *string  `toml:"overlay_background" json:"overlay_background" yaml:"overlay_background"`
	Palette             []string `toml:"palette" json:"palette" yaml:"palette"`
}

type themeDoc struct {
	themeFields
	Name  string `toml:"name" json:"name" yaml:"name"`
	Theme *themeFields
}

var defaultTheme = Theme{
	Name:                "ggterm-dark",
	Background:          color.RGBA{R: 0x0f, G: 0x11, B: 0x17, A: 0xff},
	Foreground:          color.RGBA{R: 0xc9, G: 0xd1, B: 0xd9, A: 0xff},
	Cursor:              color.RGBA{R: 0x58, G: 0xa6, B: 0xff, A: 0xc0},
	BarBackground:       color.RGBA{R: 0x16, G: 0x1b, B: 0x26, A: 0xff},
	BarForeground:       color.RGBA{R: 0x8b, G: 0x98, B: 0xa9, A: 0xff},
	TabActiveBackground: color.RGBA{R: 0x2b, G: 0x4b, B: 0x7a, A: 0xff},
	TabActiveForeground: color.RGBA{R: 0xea, G: 0xf2, B: 0xff, A: 0xff},
	Border:              color.RGBA{R: 0x2a, G: 0x32, B: 0x3f, A: 0xff},
	BorderActive:        color.RGBA{R: 0x58, G: 0xa6, B: 0xff, A: 0xff},
	OverlayBackground:   color.RGBA{R: 0x0b, G: 0x0e, B: 0x14, A: 0xee},
	Palette: [16]color.RGBA{
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
	},
}

var builtinThemes = map[string]Theme{
	"ggterm-dark":      defaultTheme,
	"default":          defaultTheme,
	"dark":             defaultTheme,
	"dracula":          mustTheme("dracula", "282a36", "f8f8f2", "f8f8f2c0", "21222c", "bd93f9", "44475a", "f8f8f2", "44475a", "bd93f9", "21222cee", []string{"21222c", "ff5555", "50fa7b", "f1fa8c", "bd93f9", "ff79c6", "8be9fd", "f8f8f2", "6272a4", "ff6e6e", "69ff94", "ffffa4", "d6acff", "ff92df", "a4ffff", "ffffff"}),
	"nord":             mustTheme("nord", "2e3440", "d8dee9", "88c0d0c0", "3b4252", "d8dee9", "4c566a", "eceff4", "434c5e", "88c0d0", "2e3440ee", []string{"3b4252", "bf616a", "a3be8c", "ebcb8b", "81a1c1", "b48ead", "88c0d0", "e5e9f0", "4c566a", "bf616a", "a3be8c", "ebcb8b", "81a1c1", "b48ead", "8fbcbb", "eceff4"}),
	"gruvbox-dark":     mustTheme("gruvbox-dark", "282828", "ebdbb2", "fabd2fc0", "32302f", "a89984", "504945", "ebdbb2", "3c3836", "fabd2f", "282828ee", []string{"282828", "cc241d", "98971a", "d79921", "458588", "b16286", "689d6a", "a89984", "928374", "fb4934", "b8bb26", "fabd2f", "83a598", "d3869b", "8ec07c", "ebdbb2"}),
	"solarized-dark":   mustTheme("solarized-dark", "002b36", "839496", "93a1a1c0", "073642", "93a1a1", "073642", "fdf6e3", "073642", "268bd2", "002b36ee", []string{"073642", "dc322f", "859900", "b58900", "268bd2", "d33682", "2aa198", "eee8d5", "002b36", "cb4b16", "586e75", "657b83", "839496", "6c71c4", "93a1a1", "fdf6e3"}),
	"tokyo-night":      mustTheme("tokyo-night", "1a1b26", "c0caf5", "c0caf5c0", "16161e", "a9b1d6", "292e42", "c0caf5", "292e42", "7aa2f7", "1a1b26ee", []string{"15161e", "f7768e", "9ece6a", "e0af68", "7aa2f7", "bb9af7", "7dcfff", "a9b1d6", "414868", "f7768e", "9ece6a", "e0af68", "7aa2f7", "bb9af7", "7dcfff", "c0caf5"}),
	"catppuccin-mocha": mustTheme("catppuccin-mocha", "1e1e2e", "cdd6f4", "f5e0dcc0", "181825", "a6adc8", "313244", "cdd6f4", "313244", "cba6f7", "1e1e2eee", []string{"45475a", "f38ba8", "a6e3a1", "f9e2af", "89b4fa", "f5c2e7", "94e2d5", "bac2de", "585b70", "f38ba8", "a6e3a1", "f9e2af", "89b4fa", "f5c2e7", "94e2d5", "a6adc8"}),
}

// BuiltinTheme returns a named colour scheme.
func BuiltinTheme(name string) (Theme, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	theme, ok := builtinThemes[key]
	return theme, ok
}

// BuiltinThemeNames lists the available built-in schemes.
func BuiltinThemeNames() []string {
	return []string{"ggterm-dark", "dracula", "nord", "gruvbox-dark", "solarized-dark", "tokyo-night", "catppuccin-mocha"}
}

// ParseColor accepts #rgb, #rrggbb and #rrggbbaa (the leading # is optional).
func ParseColor(value string) (color.RGBA, error) {
	s := strings.TrimSpace(value)
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 && len(s) != 8 {
		return color.RGBA{}, fmt.Errorf("invalid colour %q: expected 3, 6 or 8 hex digits", value)
	}
	parsed, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid colour %q: %w", value, err)
	}
	if len(s) == 6 {
		return color.RGBA{R: uint8(parsed >> 16), G: uint8(parsed >> 8), B: uint8(parsed), A: 0xff}, nil
	}
	return color.RGBA{R: uint8(parsed >> 24), G: uint8(parsed >> 16), B: uint8(parsed >> 8), A: uint8(parsed)}, nil
}

func hexColor(c color.RGBA) string {
	if c.A == 0xff {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

// LoadThemeFile loads a theme from TOML, JSON, YAML or an iTerm2 .itermcolors
// file. Base16 schemes in JSON or YAML are detected automatically.
func LoadThemeFile(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Theme{}, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".itermcolors":
		return loadITermTheme(data)
	case ".json":
		return loadJSONTheme(data)
	case ".yaml", ".yml":
		return loadYAMLTheme(data)
	case ".toml":
		return loadTOMLTheme(data)
	}
	trimmed := bytes.TrimSpace(data)
	switch {
	case len(trimmed) > 0 && trimmed[0] == '<':
		return loadITermTheme(data)
	case len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '['):
		return loadJSONTheme(data)
	}
	return loadTOMLTheme(data)
}

func loadTOMLTheme(data []byte) (Theme, error) {
	var doc themeDoc
	if err := toml.Unmarshal(data, &doc); err != nil {
		return Theme{}, err
	}
	if doc.Theme != nil {
		doc.themeFields = *doc.Theme
	}
	return themeFromFields(doc.Name, doc.themeFields)
}

func loadJSONTheme(data []byte) (Theme, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Theme{}, err
	}
	if scheme, ok := base16FromMap(raw); ok {
		return base16Theme(scheme)
	}
	var doc themeDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return Theme{}, err
	}
	return themeFromFields(doc.Name, doc.themeFields)
}

func loadYAMLTheme(data []byte) (Theme, error) {
	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err == nil {
		if scheme, ok := base16FromAny(raw); ok {
			return base16Theme(scheme)
		}
	}
	var doc themeDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Theme{}, err
	}
	return themeFromFields(doc.Name, doc.themeFields)
}

func themeFromFields(name string, fields themeFields) (Theme, error) {
	theme := defaultTheme
	if name != "" {
		theme.Name = name
	}
	if err := applyThemeFields(&theme, fields); err != nil {
		return Theme{}, err
	}
	return theme, nil
}

func applyThemeFields(theme *Theme, fields themeFields) error {
	type target struct {
		value **string
		dst   *color.RGBA
	}
	targets := []target{
		{&fields.Background, &theme.Background},
		{&fields.Foreground, &theme.Foreground},
		{&fields.Cursor, &theme.Cursor},
		{&fields.BarBackground, &theme.BarBackground},
		{&fields.BarForeground, &theme.BarForeground},
		{&fields.TabActiveBackground, &theme.TabActiveBackground},
		{&fields.TabActiveForeground, &theme.TabActiveForeground},
		{&fields.Border, &theme.Border},
		{&fields.BorderActive, &theme.BorderActive},
		{&fields.OverlayBackground, &theme.OverlayBackground},
	}
	for _, item := range targets {
		if item.value == nil || *item.value == nil || **item.value == "" {
			continue
		}
		parsed, err := ParseColor(**item.value)
		if err != nil {
			return err
		}
		*item.dst = parsed
	}
	if fields.Palette != nil {
		if len(fields.Palette) != 16 {
			return fmt.Errorf("theme palette must have 16 colours, got %d", len(fields.Palette))
		}
		for i, value := range fields.Palette {
			parsed, err := ParseColor(value)
			if err != nil {
				return err
			}
			theme.Palette[i] = parsed
		}
	}
	return nil
}

func base16FromMap(raw map[string]any) (map[string]string, bool) {
	scheme := make(map[string]string, len(raw))
	for key, value := range raw {
		if text, ok := value.(string); ok {
			scheme[strings.ToLower(key)] = text
		}
	}
	return base16FromAny(scheme)
}

func base16FromAny(raw map[string]string) (map[string]string, bool) {
	scheme := make(map[string]string, 16)
	for key, value := range raw {
		lower := strings.ToLower(key)
		if len(lower) == 6 && strings.HasPrefix(lower, "base0") {
			scheme[lower] = value
		}
	}
	if _, ok := scheme["base00"]; !ok {
		return nil, false
	}
	return scheme, true
}

func base16Theme(scheme map[string]string) (Theme, error) {
	lookup := func(name string) (color.RGBA, error) {
		value, ok := scheme[strings.ToLower(name)]
		if !ok {
			return color.RGBA{}, fmt.Errorf("base16 scheme is missing %s", name)
		}
		return ParseColor(value)
	}
	base := func(name string, fallback color.RGBA) color.RGBA {
		if value, err := lookup(name); err == nil {
			return value
		}
		return fallback
	}
	theme := defaultTheme
	theme.Name = "base16"
	theme.Background = base("base00", theme.Background)
	theme.Foreground = base("base05", theme.Foreground)
	theme.Cursor = base("base05", theme.Foreground)
	theme.Cursor.A = 0xc0
	theme.BarBackground = base("base01", theme.BarBackground)
	theme.BarForeground = base("base04", theme.BarForeground)
	theme.TabActiveBackground = base("base02", theme.TabActiveBackground)
	theme.TabActiveForeground = base("base06", theme.TabActiveForeground)
	theme.Border = base("base02", theme.Border)
	theme.BorderActive = base("base0D", theme.BorderActive)
	theme.OverlayBackground = theme.Background
	theme.OverlayBackground.A = 0xee

	normal := []string{"base00", "base08", "base0B", "base0A", "base0D", "base0E", "base0C", "base05"}
	bright := []string{"base03", "base09", "base0B", "base0A", "base0D", "base0E", "base0C", "base07"}
	for i := 0; i < 8; i++ {
		value, err := lookup(normal[i])
		if err != nil {
			return Theme{}, err
		}
		theme.Palette[i] = value
		value, err = lookup(bright[i])
		if err != nil {
			return Theme{}, err
		}
		theme.Palette[i+8] = value
	}
	return theme, nil
}

func mustTheme(name, bg, fg, cursor, barBG, barFG, tabBG, tabFG, border, borderActive, overlay string, palette []string) Theme {
	parse := func(value string) color.RGBA {
		parsed, err := ParseColor(value)
		if err != nil {
			panic(err)
		}
		return parsed
	}
	theme := Theme{
		Name:                name,
		Background:          parse(bg),
		Foreground:          parse(fg),
		Cursor:              parse(cursor),
		BarBackground:       parse(barBG),
		BarForeground:       parse(barFG),
		TabActiveBackground: parse(tabBG),
		TabActiveForeground: parse(tabFG),
		Border:              parse(border),
		BorderActive:        parse(borderActive),
		OverlayBackground:   parse(overlay),
	}
	if len(palette) != 16 {
		panic("theme palette must have 16 colours")
	}
	for i, value := range palette {
		theme.Palette[i] = parse(value)
	}
	return theme
}

func loadITermTheme(data []byte) (Theme, error) {
	root, err := decodePlist(data)
	if err != nil {
		return Theme{}, err
	}
	dict, ok := root.(map[string]any)
	if !ok {
		return Theme{}, errors.New("iterm theme: root is not a dictionary")
	}
	theme := defaultTheme
	theme.Name = "iterm"
	assign := func(key string, dst *color.RGBA) {
		if value, ok := plistColor(dict[key]); ok {
			*dst = value
		}
	}
	assign("Background Color", &theme.Background)
	assign("Foreground Color", &theme.Foreground)
	assign("Cursor Color", &theme.Cursor)
	for i := 0; i < 16; i++ {
		assign(fmt.Sprintf("Ansi %d Color", i), &theme.Palette[i])
	}
	theme.BarBackground = blendColor(theme.Background, theme.Foreground, 0.08)
	theme.BarForeground = blendColor(theme.Foreground, theme.Background, 0.15)
	theme.TabActiveBackground = theme.Palette[4]
	theme.TabActiveForeground = theme.Background
	theme.Border = blendColor(theme.Background, theme.Foreground, 0.16)
	theme.BorderActive = theme.Palette[4]
	theme.OverlayBackground = theme.Background
	theme.OverlayBackground.A = 0xee
	return theme, nil
}

func blendColor(from, to color.RGBA, amount float64) color.RGBA {
	mix := func(a, b uint8) uint8 {
		return uint8(float64(a)*(1-amount) + float64(b)*amount)
	}
	return color.RGBA{R: mix(from.R, to.R), G: mix(from.G, to.G), B: mix(from.B, to.B), A: 0xff}
}

func plistColor(value any) (color.RGBA, bool) {
	dict, ok := value.(map[string]any)
	if !ok {
		return color.RGBA{}, false
	}
	component := func(key string, fallback float64) float64 {
		raw, ok := dict[key]
		if !ok {
			return fallback
		}
		switch v := raw.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case string:
			parsed, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return parsed
			}
		}
		return fallback
	}
	toByte := func(value float64) uint8 {
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}
		return uint8(value*255 + 0.5)
	}
	return color.RGBA{
		R: toByte(component("Red Component", 0)),
		G: toByte(component("Green Component", 0)),
		B: toByte(component("Blue Component", 0)),
		A: toByte(component("Alpha Component", 1)),
	}, true
}

func decodePlist(data []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "plist" {
			continue
		}
		return parsePlistValue(dec, start)
	}
}

func parsePlistValue(dec *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		result := make(map[string]any)
		var key string
		for {
			token, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := token.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					var text string
					if err := dec.DecodeElement(&text, &t); err != nil {
						return nil, err
					}
					key = text
					continue
				}
				value, err := parsePlistValue(dec, t)
				if err != nil {
					return nil, err
				}
				if key != "" {
					result[key] = value
					key = ""
				}
			case xml.EndElement:
				if t.Name.Local == "dict" {
					return result, nil
				}
			}
		}
	case "array":
		var result []any
		for {
			token, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := token.(type) {
			case xml.StartElement:
				value, err := parsePlistValue(dec, t)
				if err != nil {
					return nil, err
				}
				result = append(result, value)
			case xml.EndElement:
				if t.Name.Local == "array" {
					return result, nil
				}
			}
		}
	case "string", "real", "integer", "date", "data":
		var text string
		if err := dec.DecodeElement(&text, &start); err != nil {
			return nil, err
		}
		text = strings.TrimSpace(text)
		switch start.Name.Local {
		case "integer":
			if parsed, err := strconv.Atoi(text); err == nil {
				return parsed, nil
			}
		case "real":
			if parsed, err := strconv.ParseFloat(text, 64); err == nil {
				return parsed, nil
			}
		}
		return text, nil
	case "true":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return true, nil
	case "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return false, nil
	default:
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return nil, nil
	}
}
