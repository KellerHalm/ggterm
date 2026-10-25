// Package plugin loads Lua plugins into the interactive client.
//
// A plugin is a single .lua file that may register commands, keybindings and
// hooks. Plugins run in the UI process next to the configuration Lua state;
// the background daemon never executes plugin code.
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"

	"ggterm/internal/config"
	"ggterm/internal/version"
)

// Host is the surface plugins use to observe and control the client.
type Host interface {
	SessionName() string
	Notify(message string)
	SetStatus(text string)
	Input(text string)
	// Action runs a built-in action. It reports whether the action exists and
	// whether the client should terminate (quit or detach).
	Action(name string, arg int) (handled, terminate bool)
	State() State
}

// State is a snapshot of the multiplexer that plugins can inspect.
type State struct {
	Session      string
	Cols         int
	Rows         int
	ActiveTab    int
	TabCount     int
	PaneCount    int
	Broadcast    bool
	Theme        string
	Tabs         []TabState
	PaneID       int
	PaneTitle    string
	PaneDir      string
	PaneAlive    bool
	ScrollOffset int
}

// TabState describes one tab for plugins.
type TabState struct {
	Index  int
	Title  string
	Active bool
}

// Info describes one loaded plugin.
type Info struct {
	Name        string
	Version     string
	Description string
	File        string
}

// Command is a command registered by a plugin.
type Command struct {
	Name string
	Desc string
	Info Info
}

type commandEntry struct {
	Command
	run *lua.LFunction
}

type hookEntry struct {
	plugin string
	fn     *lua.LFunction
}

type fileStamp struct {
	modTime time.Time
	size    int64
}

// Manager owns the Lua state of all plugins.
type Manager struct {
	host Host

	L        *lua.LState
	loaded   bool
	files    []string
	stamps   map[string]fileStamp
	infos    []Info
	commands map[string]*commandEntry
	hooks    map[string][]hookEntry
	bindings []config.Binding

	current          *Info
	currentCommands  []string
	currentHooks     map[string]int
	currentBindStart int
	terminate        bool
	lastTick         time.Time
}

// New creates a manager that forwards host calls to host.
func New(host Host) *Manager {
	return &Manager{
		host:     host,
		commands: make(map[string]*commandEntry),
		hooks:    make(map[string][]hookEntry),
	}
}

// Sync loads the plugins selected by settings. It reloads only when a plugin
// file was added, removed or changed. Errors are reported per plugin; the
// remaining plugins stay active.
func (m *Manager) Sync(settings config.Plugins) []error {
	files := config.DiscoverPluginFiles(settings)
	stamps := make(map[string]fileStamp, len(files))
	for _, file := range files {
		stamps[file] = stampOf(file)
	}
	if m.loaded && equalFiles(m.files, files) && equalStamps(m.stamps, stamps) {
		return nil
	}
	return m.load(files, stamps)
}

// Close runs the shutdown hook and releases the Lua state.
func (m *Manager) Close() {
	if !m.loaded {
		return
	}
	m.fire("shutdown")
	m.release()
}

// Infos returns the loaded plugins in load order.
func (m *Manager) Infos() []Info {
	return append([]Info(nil), m.infos...)
}

// Commands returns all plugin commands sorted by name.
func (m *Manager) Commands() []Command {
	commands := make([]Command, 0, len(m.commands))
	for _, entry := range m.commands {
		commands = append(commands, entry.Command)
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands
}

// Bindings returns the default keybindings registered by plugins.
func (m *Manager) Bindings() []config.Binding {
	return append([]config.Binding(nil), m.bindings...)
}

// HasCommand reports whether a plugin registered name.
func (m *Manager) HasCommand(name string) bool {
	_, ok := m.commands[name]
	return ok
}

// RunCommand invokes a plugin command. It reports whether the command existed
// and whether the client should terminate as a result.
func (m *Manager) RunCommand(name string, arg int) (handled, terminate bool) {
	entry, ok := m.commands[name]
	if !ok {
		return false, false
	}
	m.terminate = false
	m.call(entry.run, lua.LNumber(arg), entry.Info.Name)
	return true, m.terminate
}

// HasHook reports whether any plugin listens to event.
func (m *Manager) HasHook(event string) bool {
	return len(m.hooks[event]) > 0
}

// FireKey calls the "key" hooks. It reports whether the key was consumed and
// whether the client should terminate.
func (m *Manager) FireKey(chord string, ctrl, alt, shift, prefix bool) (consumed, terminate bool) {
	hooks := m.hooks["key"]
	if len(hooks) == 0 {
		return false, false
	}
	m.terminate = false
	table := m.L.NewTable()
	table.RawSetString("key", lua.LString(chord))
	table.RawSetString("ctrl", lua.LBool(ctrl))
	table.RawSetString("alt", lua.LBool(alt))
	table.RawSetString("shift", lua.LBool(shift))
	table.RawSetString("prefix", lua.LBool(prefix))
	for _, hook := range hooks {
		result := m.call(hook.fn, table, hook.plugin)
		if truthy(result) {
			consumed = true
			break
		}
	}
	return consumed, m.terminate
}

// Tick runs the "tick" hooks at most once per second. It reports whether the
// client should terminate as a result.
func (m *Manager) Tick() (terminate bool) {
	if len(m.hooks["tick"]) == 0 {
		return false
	}
	now := time.Now()
	if !m.lastTick.IsZero() && now.Sub(m.lastTick) < time.Second {
		return false
	}
	m.lastTick = now
	m.terminate = false
	m.fire("tick")
	return m.terminate
}

func (m *Manager) load(files []string, stamps map[string]fileStamp) []error {
	if m.loaded {
		m.fire("shutdown")
		m.release()
		m.host.SetStatus("")
	}
	if len(files) == 0 {
		m.files = nil
		m.stamps = stamps
		return nil
	}

	L := lua.NewState()
	L.OpenLibs()
	m.L = L
	m.loaded = true
	m.files = append([]string(nil), files...)
	m.stamps = stamps
	m.commands = make(map[string]*commandEntry)
	m.hooks = make(map[string][]hookEntry)
	m.bindings = nil
	m.infos = nil
	m.lastTick = time.Time{}
	m.setupAPI()

	var errs []error
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		m.beginPlugin(&Info{Name: name, File: file})
		if err := L.DoFile(file); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			m.rollbackPlugin()
			continue
		}
		m.infos = append(m.infos, *m.current)
		m.endPlugin()
	}
	m.fire("start")
	return errs
}

func (m *Manager) beginPlugin(info *Info) {
	m.current = info
	m.currentCommands = nil
	m.currentHooks = make(map[string]int)
	m.currentBindStart = len(m.bindings)
}

func (m *Manager) endPlugin() {
	m.current = nil
	m.currentCommands = nil
	m.currentHooks = nil
}

func (m *Manager) rollbackPlugin() {
	for _, name := range m.currentCommands {
		delete(m.commands, name)
	}
	for event, count := range m.currentHooks {
		hooks := m.hooks[event]
		if count > len(hooks) {
			count = len(hooks)
		}
		m.hooks[event] = hooks[:len(hooks)-count]
	}
	if m.currentBindStart <= len(m.bindings) {
		m.bindings = m.bindings[:m.currentBindStart]
	}
	m.endPlugin()
}

func (m *Manager) release() {
	if m.L != nil {
		m.L.Close()
	}
	m.L = nil
	m.loaded = false
	m.files = nil
	m.stamps = nil
	m.infos = nil
	m.commands = make(map[string]*commandEntry)
	m.hooks = make(map[string][]hookEntry)
	m.bindings = nil
}

func (m *Manager) fire(event string) {
	hooks := m.hooks[event]
	if len(hooks) == 0 {
		return
	}
	m.terminate = false
	for _, hook := range hooks {
		m.call(hook.fn, nil, hook.plugin)
	}
}

// call invokes fn and converts Lua errors into host notifications.
func (m *Manager) call(fn *lua.LFunction, arg lua.LValue, pluginName string) lua.LValue {
	if m.L == nil || fn == nil {
		return lua.LNil
	}
	args := []lua.LValue(nil)
	if arg != nil {
		args = []lua.LValue{arg}
	}
	if err := m.L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, args...); err != nil {
		m.report(pluginName, err)
		return lua.LNil
	}
	result := m.L.Get(-1)
	m.L.Pop(1)
	return result
}

func (m *Manager) report(pluginName string, err error) {
	m.host.Notify("plugin " + pluginName + ": " + err.Error())
}

func (m *Manager) setupAPI() {
	L := m.L
	module := L.NewTable()
	L.SetGlobal("ggterm", module)

	L.SetField(module, "plugin", L.NewFunction(func(L *lua.LState) int {
		if m.current == nil {
			L.RaiseError("ggterm.plugin must be called while a plugin file is loading")
		}
		table := L.CheckTable(1)
		if value := table.RawGetString("name"); value.Type() == lua.LTString {
			m.current.Name = value.String()
		}
		if value := table.RawGetString("version"); value.Type() == lua.LTString {
			m.current.Version = value.String()
		}
		if value := table.RawGetString("description"); value.Type() == lua.LTString {
			m.current.Description = value.String()
		}
		return 0
	}))

	L.SetField(module, "command", L.NewFunction(func(L *lua.LState) int {
		if m.current == nil {
			L.RaiseError("ggterm.command must be called while a plugin file is loading")
		}
		name := strings.TrimSpace(L.CheckString(1))
		if name == "" {
			L.RaiseError("command name must not be empty")
		}
		desc := ""
		fnIndex := 3
		if L.Get(2).Type() == lua.LTFunction {
			fnIndex = 2
		} else {
			desc = L.OptString(2, "")
		}
		fn := L.CheckFunction(fnIndex)
		if _, exists := m.commands[name]; exists {
			L.RaiseError("command %q is already registered", name)
		}
		m.commands[name] = &commandEntry{
			Command: Command{Name: name, Desc: desc, Info: *m.current},
			run:     fn,
		}
		m.currentCommands = append(m.currentCommands, name)
		return 0
	}))

	L.SetField(module, "on", L.NewFunction(func(L *lua.LState) int {
		if m.current == nil {
			L.RaiseError("ggterm.on must be called while a plugin file is loading")
		}
		event := strings.ToLower(strings.TrimSpace(L.CheckString(1)))
		switch event {
		case "start", "key", "tick", "shutdown":
		default:
			L.RaiseError("unknown event %q (available: start, key, tick, shutdown)", event)
		}
		fn := L.CheckFunction(2)
		m.hooks[event] = append(m.hooks[event], hookEntry{plugin: m.current.Name, fn: fn})
		m.currentHooks[event]++
		return 0
	}))

	L.SetField(module, "bind", L.NewFunction(func(L *lua.LState) int {
		if m.current == nil {
			L.RaiseError("ggterm.bind must be called while a plugin file is loading")
		}
		binding := config.Binding{
			Key:    L.CheckString(1),
			Action: L.CheckString(2),
			Arg:    L.OptInt(3, 0),
		}
		if L.GetTop() >= 4 {
			binding.Desc = L.OptString(4, "")
		}
		m.bindings = append(m.bindings, binding)
		return 0
	}))

	L.SetField(module, "notify", L.NewFunction(func(L *lua.LState) int {
		m.host.Notify(L.CheckString(1))
		return 0
	}))

	L.SetField(module, "status", L.NewFunction(func(L *lua.LState) int {
		m.host.SetStatus(L.OptString(1, ""))
		return 0
	}))

	L.SetField(module, "input", L.NewFunction(func(L *lua.LState) int {
		m.host.Input(L.CheckString(1))
		return 0
	}))

	L.SetField(module, "action", L.NewFunction(func(L *lua.LState) int {
		name := L.CheckString(1)
		arg := L.OptInt(2, 0)
		handled, terminate := m.host.Action(name, arg)
		if terminate {
			m.terminate = true
		}
		L.Push(lua.LBool(handled))
		return 1
	}))

	L.SetField(module, "state", L.NewFunction(func(L *lua.LState) int {
		state := m.host.State()
		table := L.NewTable()
		table.RawSetString("session", lua.LString(state.Session))
		table.RawSetString("cols", lua.LNumber(state.Cols))
		table.RawSetString("rows", lua.LNumber(state.Rows))
		table.RawSetString("active_tab", lua.LNumber(state.ActiveTab+1))
		table.RawSetString("tabs", lua.LNumber(state.TabCount))
		table.RawSetString("panes", lua.LNumber(state.PaneCount))
		table.RawSetString("broadcast", lua.LBool(state.Broadcast))
		table.RawSetString("theme", lua.LString(state.Theme))
		L.Push(table)
		return 1
	}))

	L.SetField(module, "active", L.NewFunction(func(L *lua.LState) int {
		state := m.host.State()
		if state.PaneID == 0 {
			L.Push(lua.LNil)
			return 1
		}
		table := L.NewTable()
		table.RawSetString("id", lua.LNumber(state.PaneID))
		table.RawSetString("title", lua.LString(state.PaneTitle))
		table.RawSetString("dir", lua.LString(state.PaneDir))
		table.RawSetString("alive", lua.LBool(state.PaneAlive))
		table.RawSetString("scroll_offset", lua.LNumber(state.ScrollOffset))
		L.Push(table)
		return 1
	}))

	L.SetField(module, "tabs", L.NewFunction(func(L *lua.LState) int {
		state := m.host.State()
		table := L.NewTable()
		for _, tab := range state.Tabs {
			item := L.NewTable()
			item.RawSetString("index", lua.LNumber(tab.Index+1))
			item.RawSetString("title", lua.LString(tab.Title))
			item.RawSetString("active", lua.LBool(tab.Active))
			table.Append(item)
		}
		L.Push(table)
		return 1
	}))

	L.SetField(module, "version", L.NewFunction(func(L *lua.LState) int {
		L.Push(lua.LString(version.String()))
		return 1
	}))
}

func stampOf(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{modTime: info.ModTime(), size: info.Size()}
}

func equalFiles(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStamps(a, b map[string]fileStamp) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func truthy(value lua.LValue) bool {
	if value == nil || value == lua.LNil {
		return false
	}
	switch value.Type() {
	case lua.LTBool:
		return lua.LVAsBool(value)
	case lua.LTNil:
		return false
	}
	return true
}
