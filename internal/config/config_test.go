package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSettings(t *testing.T) {
	settings := Default()
	if settings.Session != "default" {
		t.Errorf("session = %q", settings.Session)
	}
	if settings.Font.Size != 15 {
		t.Errorf("font size = %g", settings.Font.Size)
	}
	if settings.Theme.Name != "ggterm-dark" {
		t.Errorf("theme = %q", settings.Theme.Name)
	}
	if settings.Keys.Prefix != "ctrl+b" {
		t.Errorf("prefix = %q", settings.Keys.Prefix)
	}
}

func TestLoaderOverrides(t *testing.T) {
	session := "work"
	cols := 90
	font := 21.0
	overrides := Overrides{Session: &session, Cols: &cols, FontSize: &font}
	result, err := Loader{Disable: true, Overrides: overrides}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.Settings.Session != "work" || result.Settings.Cols != 90 || result.Settings.Font.Size != 21 {
		t.Errorf("settings = %+v", result.Settings)
	}
	if result.Settings.Rows != 36 {
		t.Errorf("rows should keep the default, got %d", result.Settings.Rows)
	}
}

func TestLoadTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, `
[general]
session = "work"
shell = "pwsh -NoLogo"
cols = 100
rows = 30
scrollback = 500
restore = false
padding = 4
decorated = false
font_size = 19

[font]
family = "Go Mono"
fallback = ["Go Mono"]

[theme]
name = "nord"
background = "#101010"

[keys]
prefix = "ctrl+a"
replace_defaults = true
unbind = ["q"]

[[keys.bindings]]
key = "%"
action = "split_vertical"

[[keys.bindings]]
key = "1"
action = "select_tab"
arg = 3
desc = "go to third tab"
`)
	result, err := Loader{Path: path}.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings := result.Settings
	if settings.Session != "work" || settings.Shell != "pwsh -NoLogo" {
		t.Errorf("general = %+v", settings)
	}
	if settings.Cols != 100 || settings.Rows != 30 || settings.Scrollback != 500 {
		t.Errorf("size = %dx%d scrollback %d", settings.Cols, settings.Rows, settings.Scrollback)
	}
	if settings.Restore || settings.Decorated || settings.Padding != 4 {
		t.Errorf("flags = restore %v decorated %v padding %d", settings.Restore, settings.Decorated, settings.Padding)
	}
	if settings.Font.Size != 19 || len(settings.Font.Fallback) != 1 {
		t.Errorf("font = %+v", settings.Font)
	}
	if settings.Theme.Name != "nord" || settings.Theme.Background.R != 0x10 {
		t.Errorf("theme = %+v", settings.Theme)
	}
	if settings.Keys.Prefix != "ctrl+a" || !settings.Keys.Replace {
		t.Errorf("keys = %+v", settings.Keys)
	}
	if len(settings.Keys.Bindings) != 2 || settings.Keys.Bindings[1].Arg != 3 {
		t.Errorf("bindings = %+v", settings.Keys.Bindings)
	}
	if len(result.Files) != 1 || result.Files[0] != path {
		t.Errorf("files = %v", result.Files)
	}
}

func TestLoadTOMLUnknownOption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, "[general]\ncolrs = 90\n")
	if _, err := (Loader{Path: path}).Load(); err == nil {
		t.Fatal("expected an error for an unknown option")
	}
}

func TestLoadTOMLThemeFile(t *testing.T) {
	dir := t.TempDir()
	themePath := filepath.Join(dir, "theme.toml")
	writeFile(t, themePath, `
name = "custom"
background = "#010203"
foreground = "#f0f0f0"
palette = [
  "#000000", "#111111", "#222222", "#333333",
  "#444444", "#555555", "#666666", "#777777",
  "#888888", "#999999", "#aaaaaa", "#bbbbbb",
  "#cccccc", "#dddddd", "#eeeeee", "#ffffff",
]
`)
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, "[theme]\nfile = \"theme.toml\"\nborder = \"#ff0000\"\n")
	result, err := Loader{Path: path}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.Settings.Theme.Background.B != 0x03 || result.Settings.Theme.Border.R != 0xff {
		t.Errorf("theme = %+v", result.Settings.Theme)
	}
	if len(result.Files) != 2 {
		t.Errorf("files = %v", result.Files)
	}
}

func TestLoadLua(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "config.toml")
	writeFile(t, tomlPath, "[font]\nsize = 16\n")
	writeFile(t, filepath.Join(dir, "init.lua"), `
ggterm.set_option("font.size", 22)
ggterm.set_option("padding", 3)
ggterm.set_prefix("ctrl+a")
ggterm.bind("c", "new_tab", 0, "new tab from lua")
ggterm.bind("9", "select_tab", 9)
ggterm.unbind("q")
ggterm.theme("dracula")
ggterm.set_theme({ cursor = "#ff0000" })
`)
	result, err := Loader{Path: tomlPath}.Load()
	if err != nil {
		t.Fatal(err)
	}
	settings := result.Settings
	if settings.Font.Size != 22 || settings.Padding != 3 {
		t.Errorf("options = size %g padding %d", settings.Font.Size, settings.Padding)
	}
	if settings.Keys.Prefix != "ctrl+a" {
		t.Errorf("prefix = %q", settings.Keys.Prefix)
	}
	if len(settings.Keys.Bindings) != 2 || settings.Keys.Bindings[1].Arg != 9 {
		t.Errorf("bindings = %+v", settings.Keys.Bindings)
	}
	if len(settings.Keys.Unbind) != 1 || settings.Keys.Unbind[0] != "q" {
		t.Errorf("unbind = %v", settings.Keys.Unbind)
	}
	if settings.Theme.Name != "dracula" {
		t.Errorf("theme = %q", settings.Theme.Name)
	}
	if settings.Theme.Cursor.R != 0xff || settings.Theme.Cursor.G != 0 {
		t.Errorf("cursor = %+v", settings.Theme.Cursor)
	}
}

func TestLoadLuaThemeFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "theme.toml"), "background = \"#112233\"\n")
	path := filepath.Join(dir, "init.lua")
	writeFile(t, path, `ggterm.theme_file("theme.toml")`)
	result, err := Loader{Path: path}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.Settings.Theme.Background.R != 0x11 {
		t.Errorf("background = %+v", result.Settings.Theme.Background)
	}
	if len(result.Files) != 1 {
		t.Errorf("files = %v", result.Files)
	}
}

func TestLoadLuaOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.lua")
	writeFile(t, path, `ggterm.set_option("rows", 44)`)
	result, err := Loader{Path: path}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.Settings.Rows != 44 {
		t.Errorf("rows = %d", result.Settings.Rows)
	}
}

func TestLoaderMissingExplicitFile(t *testing.T) {
	_, err := Loader{Path: filepath.Join(t.TempDir(), "missing.toml")}.Load()
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestWatcherReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, "[font]\nsize = 14\n")

	loader := Loader{Path: path}
	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	watcher := NewWatcher(loader, result)
	if _, changed, err := watcher.Poll(); err != nil || changed {
		t.Fatalf("unexpected initial poll: changed=%v err=%v", changed, err)
	}

	writeFile(t, path, "[font]\nsize = 25\n")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	settings, changed, err := watcher.Poll()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected the watcher to report a change")
	}
	if settings.Font.Size != 25 {
		t.Errorf("font size = %g", settings.Font.Size)
	}
}

func TestWatcherReportsBrokenConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeFile(t, path, "[font]\nsize = 14\n")

	loader := Loader{Path: path}
	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	watcher := NewWatcher(loader, result)

	writeFile(t, path, "[general]\ncols = 0\n")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	settings, changed, err := watcher.Poll()
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if changed {
		t.Error("settings must not change on error")
	}
	if settings.Font.Size != 14 {
		t.Errorf("settings were replaced: %+v", settings)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
