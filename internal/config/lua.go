package config

import (
	"fmt"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

func applyLuaFile(settings *Settings, path, dir, fallbackDir string) ([]string, error) {
	var files []string

	L := lua.NewState()
	defer L.Close()
	L.OpenLibs()

	module := L.NewTable()
	L.SetGlobal("ggterm", module)
	L.SetField(module, "set_option", L.NewFunction(func(L *lua.LState) int {
		if err := luaSetOption(L, settings, dir, fallbackDir); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}))
	L.SetField(module, "set_prefix", L.NewFunction(func(L *lua.LState) int {
		settings.Keys.Prefix = L.CheckString(1)
		return 0
	}))
	L.SetField(module, "bind", L.NewFunction(func(L *lua.LState) int {
		binding := Binding{
			Key:    L.CheckString(1),
			Action: L.CheckString(2),
			Arg:    L.OptInt(3, 0),
		}
		if L.GetTop() >= 4 {
			binding.Desc = L.OptString(4, "")
		}
		settings.Keys.Bindings = append(settings.Keys.Bindings, binding)
		return 0
	}))
	L.SetField(module, "unbind", L.NewFunction(func(L *lua.LState) int {
		settings.Keys.Unbind = append(settings.Keys.Unbind, L.CheckString(1))
		return 0
	}))
	L.SetField(module, "clear_bindings", L.NewFunction(func(L *lua.LState) int {
		settings.Keys.Replace = true
		settings.Keys.Bindings = nil
		settings.Keys.Unbind = nil
		return 0
	}))
	L.SetField(module, "plugin_dir", L.NewFunction(func(L *lua.LState) int {
		settings.Plugins.Dir = resolveRelative(L.CheckString(1), dir, fallbackDir)
		return 0
	}))
	L.SetField(module, "enable_plugin", L.NewFunction(func(L *lua.LState) int {
		settings.Plugins.Enabled = append(settings.Plugins.Enabled, L.CheckString(1))
		return 0
	}))
	L.SetField(module, "disable_plugin", L.NewFunction(func(L *lua.LState) int {
		settings.Plugins.Disabled = append(settings.Plugins.Disabled, L.CheckString(1))
		return 0
	}))
	L.SetField(module, "theme", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		theme, ok := BuiltinTheme(name)
		if !ok {
			L.RaiseError("unknown theme %q (available: %s)", name, strings.Join(BuiltinThemeNames(), ", "))
		}
		settings.Theme = theme
		return 0
	}))
	L.SetField(module, "theme_file", L.NewFunction(func(L *lua.LState) int {
		raw := L.CheckString(1)
		themePath := resolveRelative(raw, dir, fallbackDir)
		theme, err := LoadThemeFile(themePath)
		if err != nil {
			L.RaiseError("%v", err)
		}
		settings.Theme = theme
		files = append(files, themePath)
		return 0
	}))
	L.SetField(module, "set_theme", L.NewFunction(func(L *lua.LState) int {
		table := L.CheckTable(1)
		if err := luaThemeFields(L, table, settings); err != nil {
			L.RaiseError("%v", err)
		}
		return 0
	}))

	if err := L.DoFile(path); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return files, nil
}

func luaSetOption(L *lua.LState, settings *Settings, dir, fallbackDir string) error {
	key := L.CheckString(1)
	value := L.Get(2)
	asString := func() (string, error) {
		if value.Type() != lua.LTString {
			return "", fmt.Errorf("option %q expects a string", key)
		}
		return value.String(), nil
	}
	asInt := func() (int, error) {
		if value.Type() != lua.LTNumber {
			return 0, fmt.Errorf("option %q expects a number", key)
		}
		return int(lua.LVAsNumber(value)), nil
	}
	asFloat := func() (float64, error) {
		if value.Type() != lua.LTNumber {
			return 0, fmt.Errorf("option %q expects a number", key)
		}
		return float64(lua.LVAsNumber(value)), nil
	}
	asBool := func() (bool, error) {
		if value.Type() != lua.LTBool {
			return false, fmt.Errorf("option %q expects a boolean", key)
		}
		return lua.LVAsBool(value), nil
	}

	switch key {
	case "session":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Session = v
	case "shell":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Shell = v
	case "cols":
		v, err := asInt()
		if err != nil {
			return err
		}
		settings.Cols = v
	case "rows":
		v, err := asInt()
		if err != nil {
			return err
		}
		settings.Rows = v
	case "scrollback":
		v, err := asInt()
		if err != nil {
			return err
		}
		settings.Scrollback = v
	case "padding":
		v, err := asInt()
		if err != nil {
			return err
		}
		settings.Padding = v
	case "restore":
		v, err := asBool()
		if err != nil {
			return err
		}
		settings.Restore = v
	case "decorated":
		v, err := asBool()
		if err != nil {
			return err
		}
		settings.Decorated = v
	case "font.family":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Font.Family = v
	case "font.path":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Font.Path = resolveRelative(v, dir, fallbackDir)
	case "font.bold":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Font.Bold = resolveRelative(v, dir, fallbackDir)
	case "font.italic":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Font.Italic = resolveRelative(v, dir, fallbackDir)
	case "font.bold_italic":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Font.BoldItalic = resolveRelative(v, dir, fallbackDir)
	case "font.size":
		v, err := asFloat()
		if err != nil {
			return err
		}
		settings.Font.Size = v
	case "font.fallback":
		table := L.CheckTable(2)
		settings.Font.Fallback = luaStringList(table)
	case "font.ligatures":
		v, err := asBool()
		if err != nil {
			return err
		}
		settings.Font.Ligatures = v
	case "window.transparent":
		v, err := asBool()
		if err != nil {
			return err
		}
		settings.Window.Transparent = v
	case "theme":
		v, err := asString()
		if err != nil {
			return err
		}
		theme, ok := BuiltinTheme(v)
		if !ok {
			return fmt.Errorf("unknown theme %q (available: %s)", v, strings.Join(BuiltinThemeNames(), ", "))
		}
		settings.Theme = theme
	case "keys.prefix":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Keys.Prefix = v
	case "plugins.dir":
		v, err := asString()
		if err != nil {
			return err
		}
		settings.Plugins.Dir = resolveRelative(v, dir, fallbackDir)
	case "plugins.enabled":
		table := L.CheckTable(2)
		settings.Plugins.Enabled = luaStringList(table)
	case "plugins.disabled":
		table := L.CheckTable(2)
		settings.Plugins.Disabled = luaStringList(table)
	default:
		return fmt.Errorf("unknown option %q", key)
	}
	return nil
}

func luaStringList(table *lua.LTable) []string {
	var values []string
	table.ForEach(func(_, value lua.LValue) {
		if value.Type() == lua.LTString {
			values = append(values, value.String())
		}
	})
	return values
}

func luaThemeFields(L *lua.LState, table *lua.LTable, settings *Settings) error {
	var fields themeFields
	var themeErr error
	set := func(key string, dst **string, value lua.LValue) {
		if value.Type() == lua.LTString {
			text := value.String()
			*dst = &text
		}
	}
	table.ForEach(func(key, value lua.LValue) {
		if themeErr != nil {
			return
		}
		name := strings.ToLower(key.String())
		switch name {
		case "name":
			if value.Type() == lua.LTString {
				theme, ok := BuiltinTheme(value.String())
				if !ok {
					themeErr = fmt.Errorf("unknown theme %q (available: %s)", value.String(), strings.Join(BuiltinThemeNames(), ", "))
					return
				}
				settings.Theme = theme
			}
		case "background":
			set(name, &fields.Background, value)
		case "foreground":
			set(name, &fields.Foreground, value)
		case "cursor":
			set(name, &fields.Cursor, value)
		case "bar_background":
			set(name, &fields.BarBackground, value)
		case "bar_foreground":
			set(name, &fields.BarForeground, value)
		case "tab_active_background":
			set(name, &fields.TabActiveBackground, value)
		case "tab_active_foreground":
			set(name, &fields.TabActiveForeground, value)
		case "border":
			set(name, &fields.Border, value)
		case "border_active":
			set(name, &fields.BorderActive, value)
		case "overlay_background":
			set(name, &fields.OverlayBackground, value)
		case "palette":
			if palette, ok := value.(*lua.LTable); ok {
				fields.Palette = luaStringList(palette)
			}
		}
	})
	if themeErr != nil {
		return themeErr
	}
	if err := applyThemeFields(&settings.Theme, fields); err != nil {
		return err
	}
	return nil
}

func resolveRelative(value, dir, fallbackDir string) string {
	value = strings.TrimSpace(value)
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	if dir != "" {
		return filepath.Join(dir, value)
	}
	if fallbackDir != "" {
		return filepath.Join(fallbackDir, value)
	}
	return value
}
