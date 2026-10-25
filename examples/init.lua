-- GoGoTerminal Lua configuration example.
-- Copy to <os.UserConfigDir()>/ggterm/init.lua; it is loaded after config.toml.

ggterm.set_option("font.size", 16)
ggterm.set_option("font.ligatures", true)
-- ggterm.set_option("font.bold", "fonts/Bold.ttf")
-- ggterm.set_option("font.fallback", { "C:/Windows/Fonts/msyh.ttc" })
-- ggterm.set_option("window.transparent", true)
ggterm.set_option("padding", 6)
ggterm.set_prefix("ctrl+b")

ggterm.bind("c", "new_tab")
ggterm.bind("1", "select_tab", 1)
ggterm.bind("2", "select_tab", 2)
ggterm.bind("q", "none") -- remove a default binding
-- ggterm.bind("y", "file_manager") -- already the default: prefix + y

-- Select a built-in theme or load a theme file.
-- ggterm.theme("nord")
-- ggterm.theme_file("themes/my-theme.toml")

-- Override individual colours.
-- ggterm.set_theme({ cursor = "#ffcc66" })

-- Plugins are loaded from <config dir>/plugins unless [plugins] says otherwise.
-- ggterm.plugin_dir("plugins")
-- ggterm.enable_plugin("clock")
-- ggterm.disable_plugin("scratch")

-- Bind a key to a plugin command (the plugin can register one too).
-- ggterm.bind("z", "plugin:clock_toggle")
