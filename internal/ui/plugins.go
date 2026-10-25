package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ggterm/internal/plugin"
)

func (g *game) SessionName() string { return g.settings.Session }

func (g *game) Notify(message string) { g.notify(message, 4*time.Second) }

func (g *game) SetStatus(text string) { g.pluginStatus = strings.TrimSpace(text) }

func (g *game) Input(text string) {
	if text == "" || g.session == nil {
		return
	}
	g.session.WriteInput([]byte(text))
	if active := g.view.ActivePane(); active != nil {
		active.ResetScroll()
	}
}

func (g *game) Action(name string, arg int) (bool, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if command, ok := pluginCommandName(lower); ok {
		if g.plugins == nil {
			return false, false
		}
		return g.plugins.RunCommand(command, arg)
	}
	bindingAction, ok := actionNames[lower]
	if !ok || bindingAction == actionCommand {
		return false, false
	}
	if bindingAction == actionSelectTab && arg > 0 {
		arg--
	}
	return true, g.runAction(keyBinding{action: bindingAction, arg: arg}, g.view)
}

func (g *game) State() plugin.State {
	state := plugin.State{
		Session:   g.settings.Session,
		Cols:      g.view.Cols,
		Rows:      g.view.Rows,
		ActiveTab: g.view.ActiveTab,
		TabCount:  len(g.view.Tabs),
		PaneCount: len(g.view.Panes),
		Broadcast: g.view.Broadcast,
		Theme:     g.theme.Name,
	}
	for _, tab := range g.view.Tabs {
		state.Tabs = append(state.Tabs, plugin.TabState{
			Index:  tab.Index,
			Title:  tab.Title,
			Active: tab.Active,
		})
	}
	if active := g.view.ActivePane(); active != nil {
		state.PaneID = active.ID()
		state.PaneTitle = active.Title()
		state.PaneDir = active.Dir()
		state.PaneAlive = active.Alive()
		state.ScrollOffset = active.ScrollOffset()
	}
	return state
}

func pluginErrors(errs []error) string {
	parts := make([]string, 0, 3)
	for i, err := range errs {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("and %d more", len(errs)-3))
			break
		}
		parts = append(parts, err.Error())
	}
	return "plugins: " + strings.Join(parts, "; ")
}

func pluginsLabel(infos []plugin.Info) string {
	names := make([]string, 0, len(infos))
	for _, info := range infos {
		name := info.Name
		if info.Version != "" {
			name += " " + info.Version
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

func keyChordName(key ebiten.Key, mods modifiers) string {
	parts := make([]string, 0, 4)
	if mods.ctrl {
		parts = append(parts, "ctrl")
	}
	if mods.alt {
		parts = append(parts, "alt")
	}
	if mods.shift {
		parts = append(parts, "shift")
	}
	parts = append(parts, keyDisplayName(key))
	return strings.Join(parts, "+")
}

var displayKeyNames = map[ebiten.Key]string{
	ebiten.KeyEnter:        "enter",
	ebiten.KeyTab:          "tab",
	ebiten.KeySpace:        "space",
	ebiten.KeyEscape:       "escape",
	ebiten.KeyBackspace:    "backspace",
	ebiten.KeyDelete:       "delete",
	ebiten.KeyInsert:       "insert",
	ebiten.KeyHome:         "home",
	ebiten.KeyEnd:          "end",
	ebiten.KeyPageUp:       "pageup",
	ebiten.KeyPageDown:     "pagedown",
	ebiten.KeyArrowUp:      "up",
	ebiten.KeyArrowDown:    "down",
	ebiten.KeyArrowLeft:    "left",
	ebiten.KeyArrowRight:   "right",
	ebiten.KeySlash:        "slash",
	ebiten.KeyBackslash:    "backslash",
	ebiten.KeyMinus:        "minus",
	ebiten.KeyEqual:        "equal",
	ebiten.KeyComma:        "comma",
	ebiten.KeyPeriod:       "period",
	ebiten.KeySemicolon:    "semicolon",
	ebiten.KeyQuote:        "quote",
	ebiten.KeyBackquote:    "backquote",
	ebiten.KeyLeftBracket:  "leftbracket",
	ebiten.KeyRightBracket: "rightbracket",
	ebiten.KeyF1:           "f1",
	ebiten.KeyF2:           "f2",
	ebiten.KeyF3:           "f3",
	ebiten.KeyF4:           "f4",
	ebiten.KeyF5:           "f5",
	ebiten.KeyF6:           "f6",
	ebiten.KeyF7:           "f7",
	ebiten.KeyF8:           "f8",
	ebiten.KeyF9:           "f9",
	ebiten.KeyF10:          "f10",
	ebiten.KeyF11:          "f11",
	ebiten.KeyF12:          "f12",
}

func keyDisplayName(key ebiten.Key) string {
	if name, ok := displayKeyNames[key]; ok {
		return name
	}
	switch {
	case key >= ebiten.KeyA && key <= ebiten.KeyZ:
		return string(rune('a' + int(key-ebiten.KeyA)))
	case key >= ebiten.KeyDigit0 && key <= ebiten.KeyDigit9:
		return string(rune('0' + int(key-ebiten.KeyDigit0)))
	}
	return strings.ToLower(key.String())
}
