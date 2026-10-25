package config

import (
	"image/color"
	"path/filepath"
	"testing"
)

const base16JSON = `{
  "scheme": "Test Scheme",
  "base00": "101010",
  "base01": "202020",
  "base02": "303030",
  "base03": "404040",
  "base04": "505050",
  "base05": "e0e0e0",
  "base06": "f0f0f0",
  "base07": "ffffff",
  "base08": "ff0000",
  "base09": "ff8800",
  "base0A": "ffff00",
  "base0B": "00ff00",
  "base0C": "00ffff",
  "base0D": "0000ff",
  "base0E": "ff00ff",
  "base0F": "880000"
}`

const base16YAML = `scheme: "Test Scheme"
base00: "101010"
base01: "202020"
base02: "303030"
base03: "404040"
base04: "505050"
base05: "e0e0e0"
base06: "f0f0f0"
base07: "ffffff"
base08: "ff0000"
base09: "ff8800"
base0A: "ffff00"
base0B: "00ff00"
base0C: "00ffff"
base0D: "0000ff"
base0E: "ff00ff"
base0F: "880000"
`

const iTermTheme = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Background Color</key>
  <dict>
    <key>Alpha Component</key><real>1</real>
    <key>Blue Component</key><real>0.1</real>
    <key>Green Component</key><real>0.2</real>
    <key>Red Component</key><real>0.3</real>
  </dict>
  <key>Foreground Color</key>
  <dict>
    <key>Blue Component</key><real>0.9</real>
    <key>Green Component</key><real>0.8</real>
    <key>Red Component</key><real>0.7</real>
  </dict>
  <key>Cursor Color</key>
  <dict>
    <key>Blue Component</key><real>1</real>
    <key>Green Component</key><real>1</real>
    <key>Red Component</key><real>1</real>
  </dict>
  <key>Ansi 0 Color</key><dict><key>Red Component</key><real>0</real><key>Green Component</key><real>0</real><key>Blue Component</key><real>0</real></dict>
  <key>Ansi 1 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>0</real><key>Blue Component</key><real>0</real></dict>
  <key>Ansi 2 Color</key><dict><key>Red Component</key><real>0</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>0</real></dict>
  <key>Ansi 3 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>0</real></dict>
  <key>Ansi 4 Color</key><dict><key>Red Component</key><real>0</real><key>Green Component</key><real>0</real><key>Blue Component</key><real>1</real></dict>
  <key>Ansi 5 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>0</real><key>Blue Component</key><real>1</real></dict>
  <key>Ansi 6 Color</key><dict><key>Red Component</key><real>0</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>1</real></dict>
  <key>Ansi 7 Color</key><dict><key>Red Component</key><real>0.8</real><key>Green Component</key><real>0.8</real><key>Blue Component</key><real>0.8</real></dict>
  <key>Ansi 8 Color</key><dict><key>Red Component</key><real>0.4</real><key>Green Component</key><real>0.4</real><key>Blue Component</key><real>0.4</real></dict>
  <key>Ansi 9 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>0.5</real><key>Blue Component</key><real>0.5</real></dict>
  <key>Ansi 10 Color</key><dict><key>Red Component</key><real>0.5</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>0.5</real></dict>
  <key>Ansi 11 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>0.5</real></dict>
  <key>Ansi 12 Color</key><dict><key>Red Component</key><real>0.5</real><key>Green Component</key><real>0.5</real><key>Blue Component</key><real>1</real></dict>
  <key>Ansi 13 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>0.5</real><key>Blue Component</key><real>1</real></dict>
  <key>Ansi 14 Color</key><dict><key>Red Component</key><real>0.5</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>1</real></dict>
  <key>Ansi 15 Color</key><dict><key>Red Component</key><real>1</real><key>Green Component</key><real>1</real><key>Blue Component</key><real>1</real></dict>
</dict>
</plist>`

func TestParseColor(t *testing.T) {
	tests := []struct {
		in      string
		want    color.RGBA
		wantErr bool
	}{
		{"#fff", color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, false},
		{"123456", color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}, false},
		{"#12345678", color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0x78}, false},
		{"nope", color.RGBA{}, true},
		{"#12345", color.RGBA{}, true},
	}
	for _, test := range tests {
		got, err := ParseColor(test.in)
		if (err != nil) != test.wantErr {
			t.Errorf("%q: err = %v", test.in, err)
			continue
		}
		if !test.wantErr && got != test.want {
			t.Errorf("%q = %+v, want %+v", test.in, got, test.want)
		}
	}
}

func TestBuiltinTheme(t *testing.T) {
	for _, name := range BuiltinThemeNames() {
		theme, ok := BuiltinTheme(name)
		if !ok {
			t.Errorf("missing built-in theme %q", name)
			continue
		}
		if theme.Name == "" || theme.Background.A != 0xff {
			t.Errorf("theme %q is incomplete: %+v", name, theme)
		}
	}
	if _, ok := BuiltinTheme("does-not-exist"); ok {
		t.Error("unknown theme resolved")
	}
}

func TestLoadBase16JSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheme.json")
	writeFile(t, path, base16JSON)
	theme, err := LoadThemeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if theme.Background.R != 0x10 {
		t.Errorf("background = %+v", theme.Background)
	}
	if theme.Foreground.R != 0xe0 {
		t.Errorf("foreground = %+v", theme.Foreground)
	}
	if theme.Palette[1] != (color.RGBA{R: 0xff, G: 0, B: 0, A: 0xff}) {
		t.Errorf("palette 1 = %+v", theme.Palette[1])
	}
	if theme.Palette[4].B != 0xff {
		t.Errorf("palette 4 = %+v", theme.Palette[4])
	}
}

func TestLoadBase16YAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheme.yaml")
	writeFile(t, path, base16YAML)
	theme, err := LoadThemeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if theme.Background.G != 0x10 || theme.Foreground.G != 0xe0 {
		t.Errorf("theme = %+v", theme)
	}
}

func TestLoadITermTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheme.itermcolors")
	writeFile(t, path, iTermTheme)
	theme, err := LoadThemeFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if theme.Background.R != 77 || theme.Background.G != 51 || theme.Background.B != 26 {
		t.Errorf("background = %+v", theme.Background)
	}
	if theme.Foreground.R != 179 || theme.Foreground.G != 204 || theme.Foreground.B != 230 {
		t.Errorf("foreground = %+v", theme.Foreground)
	}
	if theme.Palette[1].R != 0xff || theme.Palette[1].G != 0 {
		t.Errorf("palette 1 = %+v", theme.Palette[1])
	}
	if theme.Palette[15].R != 0xff || theme.Palette[15].G != 0xff || theme.Palette[15].B != 0xff {
		t.Errorf("palette 15 = %+v", theme.Palette[15])
	}
}

func TestThemePaletteValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.toml")
	writeFile(t, path, "palette = [\"#000000\"]\n")
	if _, err := LoadThemeFile(path); err == nil {
		t.Fatal("expected a palette length error")
	}
}
