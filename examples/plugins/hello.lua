-- Minimal plugin: a command, a default binding and a key hook.
-- Press prefix + g to run it, or bind "plugin:hello" to another key.

ggterm.plugin({
  name = "hello",
  version = "1.0.0",
  description = "greets the user",
})

ggterm.command("hello", "show a greeting", function()
  local state = ggterm.state()
  ggterm.notify("Hello! session=" .. state.session .. " tab=" .. state.active_tab)
end)

ggterm.bind("g", "plugin:hello")

-- Return true to consume the key before the terminal sees it.
ggterm.on("key", function(key)
  if key.key == "ctrl+shift+k" then
    ggterm.notify("key hook: " .. key.key .. " prefix=" .. tostring(key.prefix))
    return true
  end
  return false
end)
