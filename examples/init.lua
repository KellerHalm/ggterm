-- GoGoTerminal Lua configuration example.
-- Copy to <os.UserConfigDir()>/ggterm/init.lua; it is loaded after config.toml.

ggterm.set_option("font.size", 16)
ggterm.set_option("padding", 6)
ggterm.set_prefix("ctrl+b")

ggterm.bind("c", "new_tab")
ggterm.bind("1", "select_tab", 1)
ggterm.bind("2", "select_tab", 2)
ggterm.bind("q", "none") -- remove a default binding

-- Select a built-in theme or load a theme file.
-- ggterm.theme("nord")
-- ggterm.theme_file("themes/my-theme.toml")

-- Override individual colours.
-- ggterm.set_theme({ cursor = "#ffcc66" })
