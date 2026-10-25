-- Show the current time in the right side of the status bar.
-- Copy to <os.UserConfigDir()>/ggterm/plugins/clock.lua and press prefix + t.

ggterm.plugin({
  name = "clock",
  version = "1.0.0",
  description = "clock in the status bar",
})

local enabled = true

local function render()
  if enabled then
    ggterm.status(os.date("%H:%M:%S"))
  else
    ggterm.status("")
  end
end

ggterm.on("start", render)
ggterm.on("tick", render)
ggterm.on("shutdown", function() ggterm.status("") end)

ggterm.command("clock_toggle", "toggle the status bar clock", function()
  enabled = not enabled
  render()
end)

ggterm.bind("t", "plugin:clock_toggle")
