package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ggterm/internal/config"
)

type fakeHost struct {
	notifications []string
	status        string
	input         string
	actions       []string
	terminate     bool
	state         State
}

func (h *fakeHost) SessionName() string { return h.state.Session }

func (h *fakeHost) Notify(message string) { h.notifications = append(h.notifications, message) }

func (h *fakeHost) SetStatus(text string) { h.status = text }

func (h *fakeHost) Input(text string) { h.input += text }

func (h *fakeHost) Action(name string, arg int) (bool, bool) {
	h.actions = append(h.actions, name)
	return true, h.terminate
}

func (h *fakeHost) State() State { return h.state }

func writePlugin(t *testing.T, dir, name, source string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManagerLoadsCommandsHooksAndBindings(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "demo.lua", `
ggterm.plugin({ name = "demo", version = "1.2.3", description = "demo plugin" })

ggterm.command("hello", "say hello", function(arg)
  ggterm.notify("hello " .. tostring(arg))
  ggterm.input("ls\n")
  ggterm.status("demo running")
end)

ggterm.bind("p", "plugin:hello")

ggterm.on("start", function()
  ggterm.notify("started " .. ggterm.version())
end)

ggterm.on("key", function(key)
  if key.key == "ctrl+g" then
    ggterm.notify("intercepted")
    return true
  end
  return false
end)
`)

	host := &fakeHost{state: State{Session: "work", Cols: 120, Rows: 40}}
	manager := New(host)
	defer manager.Close()

	if errs := manager.Sync(config.Plugins{Dir: dir}); len(errs) != 0 {
		t.Fatalf("Sync errors: %v", errs)
	}

	infos := manager.Infos()
	if len(infos) != 1 {
		t.Fatalf("infos = %+v", infos)
	}
	if infos[0].Name != "demo" || infos[0].Version != "1.2.3" || infos[0].Description != "demo plugin" {
		t.Errorf("info = %+v", infos[0])
	}

	commands := manager.Commands()
	if len(commands) != 1 || commands[0].Name != "hello" || commands[0].Desc != "say hello" {
		t.Errorf("commands = %+v", commands)
	}

	bindings := manager.Bindings()
	if len(bindings) != 1 || bindings[0].Key != "p" || bindings[0].Action != "plugin:hello" {
		t.Errorf("bindings = %+v", bindings)
	}

	if len(host.notifications) == 0 || !strings.HasPrefix(host.notifications[0], "started ") {
		t.Errorf("notifications = %v", host.notifications)
	}

	if handled, terminate := manager.RunCommand("hello", 42); !handled || terminate {
		t.Errorf("RunCommand = %v %v", handled, terminate)
	}
	if host.input != "ls\n" || host.status != "demo running" {
		t.Errorf("input = %q status = %q", host.input, host.status)
	}
	if len(host.notifications) < 2 || host.notifications[1] != "hello 42" {
		t.Errorf("notifications = %v", host.notifications)
	}

	if consumed, _ := manager.FireKey("ctrl+g", true, false, false, false); !consumed {
		t.Error("ctrl+g should have been consumed")
	}
	if consumed, _ := manager.FireKey("ctrl+x", true, false, false, false); consumed {
		t.Error("ctrl+x should not have been consumed")
	}
}

func TestManagerStateAPI(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "state.lua", `
ggterm.command("report", function()
  local st = ggterm.state()
  local active = ggterm.active()
  local tabs = ggterm.tabs()
  ggterm.notify(st.session .. ":" .. st.tabs .. ":" .. #tabs .. ":" .. active.title .. ":" .. st.theme)
end)
`)
	host := &fakeHost{state: State{
		Session:   "work",
		Cols:      80,
		Rows:      24,
		ActiveTab: 1,
		TabCount:  2,
		PaneCount: 3,
		Theme:     "nord",
		Tabs: []TabState{
			{Index: 0, Title: "shell"},
			{Index: 1, Title: "logs", Active: true},
		},
		PaneID:    7,
		PaneTitle: "logs",
		PaneDir:   "C:/work",
		PaneAlive: true,
	}}
	manager := New(host)
	defer manager.Close()
	if errs := manager.Sync(config.Plugins{Dir: dir}); len(errs) != 0 {
		t.Fatalf("Sync errors: %v", errs)
	}
	if _, _ = manager.RunCommand("report", 0); len(host.notifications) != 1 {
		t.Fatalf("notifications = %v", host.notifications)
	}
	if got, want := host.notifications[0], "work:2:2:logs:nord"; got != want {
		t.Errorf("report = %q, want %q", got, want)
	}
}

func TestManagerFiltersAndReportsErrors(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "good.lua", `ggterm.command("good", function() end)`)
	writePlugin(t, dir, "broken.lua", `this is not lua`)
	writePlugin(t, dir, "other.lua", `ggterm.command("other", function() end)`)

	host := &fakeHost{}
	manager := New(host)
	defer manager.Close()

	errs := manager.Sync(config.Plugins{Dir: dir})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "broken.lua") {
		t.Fatalf("errors = %v", errs)
	}
	if !manager.HasCommand("good") || !manager.HasCommand("other") {
		t.Errorf("commands = %+v", manager.Commands())
	}

	if errs := manager.Sync(config.Plugins{Dir: dir, Enabled: []string{"good"}}); len(errs) != 0 {
		t.Fatalf("enabled Sync errors: %v", errs)
	}
	if !manager.HasCommand("good") || manager.HasCommand("other") {
		t.Errorf("enabled commands = %+v", manager.Commands())
	}

	if errs := manager.Sync(config.Plugins{Dir: dir, Disabled: []string{"good", "broken"}}); len(errs) != 0 {
		t.Fatalf("disabled Sync errors: %v", errs)
	}
	if manager.HasCommand("good") {
		t.Error("good should have been disabled")
	}
	if !manager.HasCommand("other") {
		t.Error("other should still be loaded")
	}
}

func TestManagerReloadsChangedPlugins(t *testing.T) {
	dir := t.TempDir()
	path := writePlugin(t, dir, "live.lua", `ggterm.command("first", function() end)`)

	host := &fakeHost{}
	manager := New(host)
	defer manager.Close()
	if errs := manager.Sync(config.Plugins{Dir: dir}); len(errs) != 0 {
		t.Fatalf("Sync errors: %v", errs)
	}
	if !manager.HasCommand("first") {
		t.Fatal("first command missing")
	}

	if err := os.WriteFile(path, []byte(`ggterm.command("second", function() end)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if errs := manager.Sync(config.Plugins{Dir: dir}); len(errs) != 0 {
		t.Fatalf("reload errors: %v", errs)
	}
	if manager.HasCommand("first") || !manager.HasCommand("second") {
		t.Errorf("commands after reload = %+v", manager.Commands())
	}
}

func TestManagerOffAndTermination(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "quit.lua", `
ggterm.command("quit", function()
  ggterm.action("quit")
end)
`)
	host := &fakeHost{terminate: true}
	manager := New(host)
	defer manager.Close()

	if errs := manager.Sync(config.Plugins{Dir: dir}); len(errs) != 0 {
		t.Fatalf("Sync errors: %v", errs)
	}
	handled, terminate := manager.RunCommand("quit", 0)
	if !handled || !terminate {
		t.Errorf("RunCommand = %v %v", handled, terminate)
	}
	if len(host.actions) != 1 || host.actions[0] != "quit" {
		t.Errorf("actions = %v", host.actions)
	}

	if errs := manager.Sync(config.Plugins{Off: true}); len(errs) != 0 {
		t.Fatalf("off Sync errors: %v", errs)
	}
	if len(manager.Commands()) != 0 || len(manager.Infos()) != 0 {
		t.Errorf("plugins still loaded after Off")
	}
}

func TestManagerHookErrorIsReported(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "boom.lua", `
ggterm.on("tick", function() error("boom") end)
`)
	host := &fakeHost{}
	manager := New(host)
	defer manager.Close()
	if errs := manager.Sync(config.Plugins{Dir: dir}); len(errs) != 0 {
		t.Fatalf("Sync errors: %v", errs)
	}
	manager.Tick()
	if len(host.notifications) != 1 || !strings.Contains(host.notifications[0], "boom") {
		t.Errorf("notifications = %v", host.notifications)
	}
}

func TestManagerRollsBackFailedPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "partial.lua", `
ggterm.command("ghost", function() end)
ggterm.bind("g", "plugin:ghost")
error("boom")
`)
	host := &fakeHost{}
	manager := New(host)
	defer manager.Close()
	errs := manager.Sync(config.Plugins{Dir: dir})
	if len(errs) != 1 {
		t.Fatalf("errors = %v", errs)
	}
	if manager.HasCommand("ghost") {
		t.Error("command from a failed plugin was kept")
	}
	if len(manager.Bindings()) != 0 {
		t.Errorf("bindings = %+v", manager.Bindings())
	}
	if len(manager.Infos()) != 0 {
		t.Errorf("infos = %+v", manager.Infos())
	}
}
