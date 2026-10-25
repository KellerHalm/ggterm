package ui

import "github.com/hajimehoshi/ebiten/v2"

type action uint8

const (
	actionNone action = iota
	actionSplitVertical
	actionSplitHorizontal
	actionClosePane
	actionNewTab
	actionCloseTab
	actionNextTab
	actionPrevTab
	actionSelectTab
	actionFocusNext
	actionFocusLeft
	actionFocusRight
	actionFocusUp
	actionFocusDown
	actionToggleBroadcast
	actionHelp
	actionDetach
	actionQuit
)

type keyBinding struct {
	key    ebiten.Key
	shift  bool
	action action
	arg    int
	desc   string
}

const prefixKey = ebiten.KeyB

var defaultBindings = []keyBinding{
	{key: ebiten.KeyBackslash, action: actionSplitVertical, desc: "split vertical (side by side)"},
	{key: ebiten.KeyBackslash, shift: true, action: actionSplitVertical, desc: "split vertical (side by side)"},
	{key: ebiten.KeyDigit5, shift: true, action: actionSplitVertical, desc: "split vertical (side by side)"},
	{key: ebiten.KeyMinus, action: actionSplitHorizontal, desc: "split horizontal (stacked)"},
	{key: ebiten.KeyQuote, shift: true, action: actionSplitHorizontal, desc: "split horizontal (stacked)"},
	{key: ebiten.KeyC, action: actionNewTab, desc: "new tab"},
	{key: ebiten.KeyN, action: actionNextTab, desc: "next tab"},
	{key: ebiten.KeyP, action: actionPrevTab, desc: "previous tab"},
	{key: ebiten.KeyX, action: actionClosePane, desc: "close pane"},
	{key: ebiten.KeyDigit7, shift: true, action: actionCloseTab, desc: "close tab"},
	{key: ebiten.KeyO, action: actionFocusNext, desc: "focus next pane"},
	{key: ebiten.KeyH, action: actionFocusLeft, desc: "focus pane left"},
	{key: ebiten.KeyJ, action: actionFocusDown, desc: "focus pane down"},
	{key: ebiten.KeyK, action: actionFocusUp, desc: "focus pane up"},
	{key: ebiten.KeyL, action: actionFocusRight, desc: "focus pane right"},
	{key: ebiten.KeyArrowLeft, action: actionFocusLeft, desc: "focus pane left"},
	{key: ebiten.KeyArrowDown, action: actionFocusDown, desc: "focus pane down"},
	{key: ebiten.KeyArrowUp, action: actionFocusUp, desc: "focus pane up"},
	{key: ebiten.KeyArrowRight, action: actionFocusRight, desc: "focus pane right"},
	{key: ebiten.KeyB, action: actionToggleBroadcast, desc: "toggle input broadcast"},
	{key: ebiten.KeySlash, shift: true, action: actionHelp, desc: "show help"},
	{key: ebiten.KeyD, action: actionDetach, desc: "detach, keep session running"},
	{key: ebiten.KeyQ, action: actionQuit, desc: "quit and terminate the session"},
}

func init() {
	digits := []ebiten.Key{
		ebiten.KeyDigit1, ebiten.KeyDigit2, ebiten.KeyDigit3,
		ebiten.KeyDigit4, ebiten.KeyDigit5, ebiten.KeyDigit6,
		ebiten.KeyDigit7, ebiten.KeyDigit8, ebiten.KeyDigit9,
	}
	for i, key := range digits {
		defaultBindings = append(defaultBindings, keyBinding{
			key:    key,
			action: actionSelectTab,
			arg:    i,
			desc:   "select tab",
		})
	}
}

func isPrefixKey(key ebiten.Key, mods modifiers) bool {
	return key == prefixKey && mods.ctrl && !mods.alt
}

func lookupBinding(key ebiten.Key, mods modifiers) (keyBinding, bool) {
	if mods.ctrl || mods.alt {
		return keyBinding{}, false
	}
	for _, binding := range defaultBindings {
		if binding.key == key && binding.shift == mods.shift {
			return binding, true
		}
	}
	return keyBinding{}, false
}

var helpLines = []string{
	"Ctrl+B is the prefix key.",
	"",
	"%  or \\      split vertical (side by side)",
	"\"  or -      split horizontal (stacked)",
	"h j k l      focus pane left/down/up/right",
	"arrows       focus pane in direction",
	"o            focus next pane",
	"c            new tab",
	"n / p        next / previous tab",
	"1..9         select tab",
	"x            close pane",
	"&            close tab",
	"b            toggle broadcast input",
	"?            toggle this help",
	"d            detach (session keeps running)",
	"q            quit and terminate the session",
	"",
	"Mouse: click a pane or tab to focus it.",
}
