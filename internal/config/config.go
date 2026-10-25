// Package config loads ggterm settings from TOML and Lua.
//
// Configuration is layered: built-in defaults, the TOML file, the optional Lua
// file and finally explicit command-line overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// DefaultSession is used when neither the config file nor the command line
// selects a session.
const DefaultSession = "default"

const (
	DefaultConfigName = "config.toml"
	DefaultLuaName    = "init.lua"
)

// Font controls the terminal face. Paths for Bold, Italic and BoldItalic are
// optional: when empty, ggterm looks for matching faces next to Path or in the
// system font directories.
type Font struct {
	Family     string
	Path       string
	Bold       string
	Italic     string
	BoldItalic string
	Size       float64
	Fallback   []string
	Ligatures  bool
}

// Window controls window-level options that are applied before the graphics
// loop starts.
type Window struct {
	// Transparent enables a translucent window. Combine it with a theme
	// background alpha (for example "#0f1117d0") to control the opacity.
	Transparent bool
}

// Binding maps a key chord (parsed after the prefix) to an action.
type Binding struct {
	Key    string
	Action string
	Arg    int
	Desc   string
}

// Keys holds keybinding overrides. Bindings are merged with the built-in
// defaults unless Replace is set.
type Keys struct {
	Prefix   string
	Bindings []Binding
	Unbind   []string
	Replace  bool
}

// Settings is the fully resolved configuration.
type Settings struct {
	Session string
	Shell   string
	Cols    int
	Rows    int

	Scrollback int
	Restore    bool
	Padding    int
	Decorated  bool

	Font    Font
	Window  Window
	Theme   Theme
	Keys    Keys
	Plugins Plugins
}

// Default returns a fresh copy of the built-in configuration.
func Default() Settings {
	return Settings{
		Session:    DefaultSession,
		Cols:       120,
		Rows:       36,
		Scrollback: 10000,
		Restore:    true,
		Padding:    8,
		Decorated:  true,
		Font: Font{
			Family:    "Go Mono",
			Size:      15,
			Ligatures: true,
		},
		Theme: defaultTheme,
		Keys: Keys{
			Prefix: "ctrl+b",
		},
	}
}

// Overrides carries command-line values that must win over the config file.
// Nil fields were not specified explicitly.
type Overrides struct {
	Session    *string
	Shell      *string
	Cols       *int
	Rows       *int
	Scrollback *int
	Restore    *bool
	FontSize   *float64
	PluginsDir *string
	NoPlugins  *bool
}

// Result is the resolved configuration plus every file that was read. The
// file list is used for hot reloading.
type Result struct {
	Settings Settings
	Files    []string
}

// Loader reads configuration from a TOML file and an optional Lua file.
type Loader struct {
	// Path selects an explicit config file. A .lua path loads only Lua;
	// otherwise Lua is read from init.lua next to the TOML file. An empty
	// path discovers config.toml and init.lua in the user config directory.
	Path string

	// Disable skips file loading and uses defaults plus overrides.
	Disable bool

	Overrides Overrides
}

type generalTable struct {
	Session    *string  `toml:"session"`
	Shell      *string  `toml:"shell"`
	Cols       *int     `toml:"cols"`
	Rows       *int     `toml:"rows"`
	Scrollback *int     `toml:"scrollback"`
	Restore    *bool    `toml:"restore"`
	Padding    *int     `toml:"padding"`
	Decorated  *bool    `toml:"decorated"`
	FontSize   *float64 `toml:"font_size"`
	FontFamily *string  `toml:"font_family"`
}

type fontTable struct {
	Family     *string  `toml:"family"`
	Path       *string  `toml:"path"`
	Bold       *string  `toml:"bold"`
	Italic     *string  `toml:"italic"`
	BoldItalic *string  `toml:"bold_italic"`
	Size       *float64 `toml:"size"`
	Fallback   []string `toml:"fallback"`
	Ligatures  *bool    `toml:"ligatures"`
}

type windowTable struct {
	Transparent *bool `toml:"transparent"`
}

type themeTable struct {
	Name string `toml:"name"`
	File string `toml:"file"`
	themeFields
}

type bindingTable struct {
	Key    string `toml:"key"`
	Action string `toml:"action"`
	Arg    int    `toml:"arg"`
	Desc   string `toml:"desc"`
}

type keysTable struct {
	Prefix   *string        `toml:"prefix"`
	Replace  *bool          `toml:"replace_defaults"`
	Bindings []bindingTable `toml:"bindings"`
	Unbind   []string       `toml:"unbind"`
}

type pluginsTable struct {
	Dir      *string  `toml:"dir"`
	Enabled  []string `toml:"enabled"`
	Disabled []string `toml:"disabled"`
}

type fileConfig struct {
	General generalTable `toml:"general"`
	Font    fontTable    `toml:"font"`
	Window  windowTable  `toml:"window"`
	Theme   themeTable   `toml:"theme"`
	Keys    keysTable    `toml:"keys"`
	Plugins pluginsTable `toml:"plugins"`
}

// ConfigDir returns the directory that stores ggterm configuration.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ggterm"), nil
}

// DefaultPath returns the default TOML configuration file path.
func DefaultPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultConfigName), nil
}

// Discover returns the config file that should be loaded, preferring TOML and
// falling back to init.lua. It returns an empty path when neither exists.
func Discover() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	tomlPath := filepath.Join(dir, DefaultConfigName)
	if fileExists(tomlPath) {
		return tomlPath, nil
	}
	luaPath := filepath.Join(dir, DefaultLuaName)
	if fileExists(luaPath) {
		return luaPath, nil
	}
	return "", nil
}

// Load resolves the settings.
func (l Loader) Load() (Result, error) {
	settings := Default()
	result := Result{Settings: settings}
	if l.Disable {
		resolvePlugins(&settings, "")
		l.applyOverrides(&settings)
		if err := validate(settings); err != nil {
			return Result{Settings: settings}, err
		}
		result.Settings = settings
		return result, nil
	}

	tomlPath, luaPath, explicit := l.paths()
	if explicit {
		required := tomlPath
		if required == "" {
			required = luaPath
		}
		if _, err := os.Stat(required); err != nil {
			return result, fmt.Errorf("config file: %w", err)
		}
	}

	tomlDir := ""
	if tomlPath != "" {
		tomlDir = filepath.Dir(tomlPath)
	} else if l.Path != "" {
		tomlDir = filepath.Dir(luaPath)
	}

	if tomlPath != "" && fileExists(tomlPath) {
		if err := applyTOMLFile(&settings, tomlPath, tomlDir, &result.Files); err != nil {
			return result, err
		}
	}
	if luaPath != "" && fileExists(luaPath) {
		files, err := applyLuaFile(&settings, luaPath, filepath.Dir(luaPath), tomlDir)
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, files...)
	}

	l.applyOverrides(&settings)
	resolvePlugins(&settings, tomlDir)
	if err := validate(settings); err != nil {
		return Result{Settings: settings}, err
	}
	result.Settings = settings
	return result, nil
}

func (l Loader) paths() (tomlPath, luaPath string, explicit bool) {
	if l.Disable {
		return "", "", false
	}
	if l.Path != "" {
		abs, err := filepath.Abs(l.Path)
		if err != nil {
			abs = l.Path
		}
		if strings.EqualFold(filepath.Ext(abs), ".lua") {
			return "", abs, true
		}
		return abs, filepath.Join(filepath.Dir(abs), DefaultLuaName), true
	}
	dir, err := ConfigDir()
	if err != nil {
		return "", "", false
	}
	tomlPath = filepath.Join(dir, DefaultConfigName)
	luaPath = filepath.Join(dir, DefaultLuaName)
	if !fileExists(tomlPath) && !fileExists(luaPath) {
		legacy := filepath.Join(dir, "config.lua")
		if fileExists(legacy) {
			luaPath = legacy
		}
	}
	return tomlPath, luaPath, false
}

func (l Loader) applyOverrides(settings *Settings) {
	o := l.Overrides
	if o.Session != nil {
		settings.Session = *o.Session
	}
	if o.Shell != nil {
		settings.Shell = *o.Shell
	}
	if o.Cols != nil {
		settings.Cols = *o.Cols
	}
	if o.Rows != nil {
		settings.Rows = *o.Rows
	}
	if o.Scrollback != nil {
		settings.Scrollback = *o.Scrollback
	}
	if o.Restore != nil {
		settings.Restore = *o.Restore
	}
	if o.FontSize != nil {
		settings.Font.Size = *o.FontSize
	}
	if o.PluginsDir != nil {
		settings.Plugins.Dir = *o.PluginsDir
	}
	if o.NoPlugins != nil && *o.NoPlugins {
		settings.Plugins = Plugins{Off: true}
	}
}

// ConfigPath returns the file that Load would read (or an empty string when no
// file exists). It is intended for diagnostics and hot reloading.
func (l Loader) ConfigPath() string {
	tomlPath, luaPath, _ := l.paths()
	if tomlPath != "" && fileExists(tomlPath) {
		return tomlPath
	}
	if luaPath != "" && fileExists(luaPath) {
		return luaPath
	}
	if tomlPath != "" {
		return tomlPath
	}
	return luaPath
}

func applyTOMLFile(settings *Settings, path, dir string, files *[]string) error {
	var file fileConfig
	meta, err := toml.DecodeFile(path, &file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		names := make([]string, len(undecoded))
		for i, key := range undecoded {
			names[i] = key.String()
		}
		return fmt.Errorf("%s: unknown option(s): %s", path, strings.Join(names, ", "))
	}
	*files = append(*files, path)

	general := file.General
	if general.Session != nil {
		settings.Session = *general.Session
	}
	if general.Shell != nil {
		settings.Shell = *general.Shell
	}
	if general.Cols != nil {
		settings.Cols = *general.Cols
	}
	if general.Rows != nil {
		settings.Rows = *general.Rows
	}
	if general.Scrollback != nil {
		settings.Scrollback = *general.Scrollback
	}
	if general.Restore != nil {
		settings.Restore = *general.Restore
	}
	if general.Padding != nil {
		settings.Padding = *general.Padding
	}
	if general.Decorated != nil {
		settings.Decorated = *general.Decorated
	}
	if general.FontSize != nil {
		settings.Font.Size = *general.FontSize
	}
	if general.FontFamily != nil {
		settings.Font.Family = *general.FontFamily
	}

	if file.Font.Family != nil {
		settings.Font.Family = *file.Font.Family
	}
	if file.Font.Path != nil {
		settings.Font.Path = resolvePath(dir, *file.Font.Path)
	}
	if file.Font.Bold != nil {
		settings.Font.Bold = resolvePath(dir, *file.Font.Bold)
	}
	if file.Font.Italic != nil {
		settings.Font.Italic = resolvePath(dir, *file.Font.Italic)
	}
	if file.Font.BoldItalic != nil {
		settings.Font.BoldItalic = resolvePath(dir, *file.Font.BoldItalic)
	}
	if file.Font.Size != nil {
		settings.Font.Size = *file.Font.Size
	}
	if file.Font.Fallback != nil {
		settings.Font.Fallback = resolvePaths(dir, file.Font.Fallback)
	}
	if file.Font.Ligatures != nil {
		settings.Font.Ligatures = *file.Font.Ligatures
	}

	if file.Window.Transparent != nil {
		settings.Window.Transparent = *file.Window.Transparent
	}

	if file.Theme.Name != "" && file.Theme.File != "" {
		return fmt.Errorf("%s: theme.name and theme.file are mutually exclusive", path)
	}
	if file.Theme.Name != "" {
		theme, ok := BuiltinTheme(file.Theme.Name)
		if !ok {
			return fmt.Errorf("%s: unknown theme %q (available: %s)", path, file.Theme.Name, strings.Join(BuiltinThemeNames(), ", "))
		}
		settings.Theme = theme
	}
	if file.Theme.File != "" {
		themePath := resolvePath(dir, file.Theme.File)
		theme, err := LoadThemeFile(themePath)
		if err != nil {
			return fmt.Errorf("%s: %w", themePath, err)
		}
		settings.Theme = theme
		*files = append(*files, themePath)
	}
	if err := applyThemeFields(&settings.Theme, file.Theme.themeFields); err != nil {
		return fmt.Errorf("%s: theme: %w", path, err)
	}

	if file.Keys.Prefix != nil {
		settings.Keys.Prefix = *file.Keys.Prefix
	}
	if file.Keys.Replace != nil {
		settings.Keys.Replace = *file.Keys.Replace
	}
	for _, binding := range file.Keys.Bindings {
		settings.Keys.Bindings = append(settings.Keys.Bindings, Binding{
			Key:    binding.Key,
			Action: binding.Action,
			Arg:    binding.Arg,
			Desc:   binding.Desc,
		})
	}
	settings.Keys.Unbind = append(settings.Keys.Unbind, file.Keys.Unbind...)

	if file.Plugins.Dir != nil {
		settings.Plugins.Dir = resolvePath(dir, *file.Plugins.Dir)
	}
	settings.Plugins.Enabled = append(settings.Plugins.Enabled, file.Plugins.Enabled...)
	settings.Plugins.Disabled = append(settings.Plugins.Disabled, file.Plugins.Disabled...)
	return nil
}

func validate(settings Settings) error {
	if strings.TrimSpace(settings.Session) == "" {
		return errors.New("config: session name must not be empty")
	}
	if settings.Cols < 1 || settings.Cols > 10000 {
		return fmt.Errorf("config: cols must be between 1 and 10000, got %d", settings.Cols)
	}
	if settings.Rows < 1 || settings.Rows > 10000 {
		return fmt.Errorf("config: rows must be between 1 and 10000, got %d", settings.Rows)
	}
	if settings.Scrollback < 0 {
		return fmt.Errorf("config: scrollback must be non-negative, got %d", settings.Scrollback)
	}
	if settings.Padding < 0 || settings.Padding > 64 {
		return fmt.Errorf("config: padding must be between 0 and 64, got %d", settings.Padding)
	}
	if settings.Font.Size < 4 || settings.Font.Size > 200 {
		return fmt.Errorf("config: font.size must be between 4 and 200, got %g", settings.Font.Size)
	}
	if strings.TrimSpace(settings.Keys.Prefix) == "" {
		return errors.New("config: keys.prefix must not be empty")
	}
	for _, binding := range settings.Keys.Bindings {
		if strings.TrimSpace(binding.Key) == "" {
			return errors.New("config: keybinding is missing a key")
		}
		if strings.TrimSpace(binding.Action) == "" {
			return fmt.Errorf("config: keybinding %q is missing an action", binding.Key)
		}
	}
	return nil
}

func resolvePath(dir, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) || dir == "" {
		return value
	}
	return filepath.Join(dir, value)
}

func resolvePaths(dir string, values []string) []string {
	resolved := make([]string, len(values))
	for i, value := range values {
		resolved[i] = resolvePath(dir, value)
	}
	return resolved
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
