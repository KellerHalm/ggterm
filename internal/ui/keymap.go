package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"ggterm/internal/config"
)

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
	actionFileManager
	actionHelp
	actionFullscreen
	actionDetach
	actionQuit
)

type keyBinding struct {
	key    ebiten.Key
	mods   modifiers
	label  string
	action action
	arg    int
	desc   string
}

var actionNames = map[string]action{
	"split_vertical":    actionSplitVertical,
	"split_h":           actionSplitHorizontal,
	"split_horizontal":  actionSplitHorizontal,
	"close_pane":        actionClosePane,
	"new_tab":           actionNewTab,
	"close_tab":         actionCloseTab,
	"next_tab":          actionNextTab,
	"prev_tab":          actionPrevTab,
	"select_tab":        actionSelectTab,
	"focus_next":        actionFocusNext,
	"focus_left":        actionFocusLeft,
	"focus_right":       actionFocusRight,
	"focus_up":          actionFocusUp,
	"focus_down":        actionFocusDown,
	"toggle_broadcast":  actionToggleBroadcast,
	"broadcast":         actionToggleBroadcast,
	"file_manager":      actionFileManager,
	"fm":                actionFileManager,
	"yazi":              actionFileManager,
	"help":              actionHelp,
	"fullscreen":        actionFullscreen,
	"toggle_fullscreen": actionFullscreen,
	"detach":            actionDetach,
	"quit":              actionQuit,
}

var actionDescriptions = map[action]string{
	actionSplitVertical:   "split vertical (side by side)",
	actionSplitHorizontal: "split horizontal (stacked)",
	actionClosePane:       "close pane",
	actionNewTab:          "new tab",
	actionCloseTab:        "close tab",
	actionNextTab:         "next tab",
	actionPrevTab:         "previous tab",
	actionSelectTab:       "select tab",
	actionFocusNext:       "focus next pane",
	actionFocusLeft:       "focus pane left",
	actionFocusRight:      "focus pane right",
	actionFocusUp:         "focus pane up",
	actionFocusDown:       "focus pane down",
	actionToggleBroadcast: "toggle input broadcast",
	actionFileManager:     "open the file manager",
	actionHelp:            "toggle this help",
	actionFullscreen:      "toggle fullscreen",
	actionDetach:          "detach (session keeps running)",
	actionQuit:            "quit and terminate the session",
}

var defaultBindingSpecs = []config.Binding{
	{Key: "%", Action: "split_vertical"},
	{Key: "\\", Action: "split_vertical"},
	{Key: "|", Action: "split_vertical"},
	{Key: "\"", Action: "split_horizontal"},
	{Key: "-", Action: "split_horizontal"},
	{Key: "c", Action: "new_tab"},
	{Key: "n", Action: "next_tab"},
	{Key: "p", Action: "prev_tab"},
	{Key: "x", Action: "close_pane"},
	{Key: "&", Action: "close_tab"},
	{Key: "o", Action: "focus_next"},
	{Key: "h", Action: "focus_left"},
	{Key: "j", Action: "focus_down"},
	{Key: "k", Action: "focus_up"},
	{Key: "l", Action: "focus_right"},
	{Key: "left", Action: "focus_left"},
	{Key: "down", Action: "focus_down"},
	{Key: "up", Action: "focus_up"},
	{Key: "right", Action: "focus_right"},
	{Key: "b", Action: "toggle_broadcast"},
	{Key: "y", Action: "file_manager"},
	{Key: "?", Action: "help"},
	{Key: "f", Action: "fullscreen"},
	{Key: "d", Action: "detach"},
	{Key: "q", Action: "quit"},
	{Key: "1", Action: "select_tab", Arg: 1},
	{Key: "2", Action: "select_tab", Arg: 2},
	{Key: "3", Action: "select_tab", Arg: 3},
	{Key: "4", Action: "select_tab", Arg: 4},
	{Key: "5", Action: "select_tab", Arg: 5},
	{Key: "6", Action: "select_tab", Arg: 6},
	{Key: "7", Action: "select_tab", Arg: 7},
	{Key: "8", Action: "select_tab", Arg: 8},
	{Key: "9", Action: "select_tab", Arg: 9},
}

var defaultBindings = buildDefaultBindings()

func buildDefaultBindings() []keyBinding {
	bindings := make([]keyBinding, 0, len(defaultBindingSpecs))
	for _, spec := range defaultBindingSpecs {
		binding, err := bindingFromConfig(spec)
		if err != nil {
			panic(err)
		}
		bindings = append(bindings, binding)
	}
	return bindings
}

func buildKeymap(keys config.Keys) ([]keyBinding, keyBinding, error) {
	prefix, err := parseKeySpec(keys.Prefix)
	if err != nil {
		return nil, keyBinding{}, fmt.Errorf("prefix: %w", err)
	}
	prefix.action = actionNone
	prefix.label = prettySpec(keys.Prefix)

	var bindings []keyBinding
	if !keys.Replace {
		bindings = append(bindings, defaultBindings...)
	}
	for _, spec := range keys.Unbind {
		chord, err := parseKeySpec(spec)
		if err != nil {
			return nil, keyBinding{}, fmt.Errorf("unbind %q: %w", spec, err)
		}
		bindings = removeBinding(bindings, chord.key, chord.mods)
	}
	for _, spec := range keys.Bindings {
		binding, err := bindingFromConfig(spec)
		if err != nil {
			return nil, keyBinding{}, err
		}
		bindings = removeBinding(bindings, binding.key, binding.mods)
		if binding.action != actionNone {
			bindings = append(bindings, binding)
		}
	}
	return bindings, prefix, nil
}

func bindingFromConfig(spec config.Binding) (keyBinding, error) {
	chord, err := parseKeySpec(spec.Key)
	if err != nil {
		return keyBinding{}, fmt.Errorf("binding %q: %w", spec.Key, err)
	}
	name := strings.ToLower(strings.TrimSpace(spec.Action))
	if name == "" || name == "none" || name == "unbind" {
		chord.action = actionNone
		return chord, nil
	}
	bindingAction, ok := actionNames[name]
	if !ok {
		return keyBinding{}, fmt.Errorf("binding %q: unknown action %q", spec.Key, spec.Action)
	}
	arg := spec.Arg
	if bindingAction == actionSelectTab && arg > 0 {
		arg--
	}
	desc := spec.Desc
	if desc == "" {
		desc = actionDescriptions[bindingAction]
	}
	chord.action = bindingAction
	chord.arg = arg
	chord.desc = desc
	return chord, nil
}

func removeBinding(bindings []keyBinding, key ebiten.Key, mods modifiers) []keyBinding {
	filtered := bindings[:0]
	for _, binding := range bindings {
		if binding.key == key && binding.mods == mods {
			continue
		}
		filtered = append(filtered, binding)
	}
	return filtered
}

func parseKeySpec(spec string) (keyBinding, error) {
	original := strings.TrimSpace(spec)
	if original == "" {
		return keyBinding{}, fmt.Errorf("empty key")
	}
	value := original
	if len(value) > len("prefix+") && strings.EqualFold(value[:len("prefix+")], "prefix+") {
		value = value[len("prefix+"):]
	}
	parts := strings.Split(value, "+")
	if value == "+" {
		parts = []string{"plus"}
	}
	name := strings.TrimSpace(parts[len(parts)-1])
	if name == "" {
		return keyBinding{}, fmt.Errorf("invalid key %q", spec)
	}
	var mods modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "ctrl", "control":
			mods.ctrl = true
		case "alt", "option":
			mods.alt = true
		case "shift":
			mods.shift = true
		case "super", "cmd", "command", "win", "meta":
			return keyBinding{}, fmt.Errorf("unsupported modifier %q", part)
		default:
			return keyBinding{}, fmt.Errorf("unknown modifier %q", part)
		}
	}
	key, inherentShift, err := keyForName(name)
	if err != nil {
		return keyBinding{}, err
	}
	if inherentShift {
		mods.shift = true
	}
	return keyBinding{key: key, mods: mods, label: prettySpec(value)}, nil
}

func keyForName(name string) (ebiten.Key, bool, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if entry, ok := namedKeys[lower]; ok {
		return entry.key, entry.shift, nil
	}
	runes := []rune(name)
	if len(runes) != 1 {
		return 0, false, fmt.Errorf("unknown key %q", name)
	}
	r := runes[0]
	switch {
	case r >= 'a' && r <= 'z':
		return ebiten.KeyA + ebiten.Key(r-'a'), false, nil
	case r >= 'A' && r <= 'Z':
		return ebiten.KeyA + ebiten.Key(r-'A'), true, nil
	case r >= '0' && r <= '9':
		return ebiten.KeyDigit0 + ebiten.Key(r-'0'), false, nil
	}
	if entry, ok := punctuationKeys[r]; ok {
		return entry.key, entry.shift, nil
	}
	return 0, false, fmt.Errorf("unknown key %q", name)
}

var namedKeys = map[string]struct {
	key   ebiten.Key
	shift bool
}{
	"up":           {ebiten.KeyArrowUp, false},
	"down":         {ebiten.KeyArrowDown, false},
	"left":         {ebiten.KeyArrowLeft, false},
	"right":        {ebiten.KeyArrowRight, false},
	"arrowup":      {ebiten.KeyArrowUp, false},
	"arrowdown":    {ebiten.KeyArrowDown, false},
	"arrowleft":    {ebiten.KeyArrowLeft, false},
	"arrowright":   {ebiten.KeyArrowRight, false},
	"enter":        {ebiten.KeyEnter, false},
	"return":       {ebiten.KeyEnter, false},
	"tab":          {ebiten.KeyTab, false},
	"space":        {ebiten.KeySpace, false},
	"escape":       {ebiten.KeyEscape, false},
	"esc":          {ebiten.KeyEscape, false},
	"backspace":    {ebiten.KeyBackspace, false},
	"delete":       {ebiten.KeyDelete, false},
	"del":          {ebiten.KeyDelete, false},
	"insert":       {ebiten.KeyInsert, false},
	"ins":          {ebiten.KeyInsert, false},
	"home":         {ebiten.KeyHome, false},
	"end":          {ebiten.KeyEnd, false},
	"pageup":       {ebiten.KeyPageUp, false},
	"pgup":         {ebiten.KeyPageUp, false},
	"pagedown":     {ebiten.KeyPageDown, false},
	"pgdn":         {ebiten.KeyPageDown, false},
	"slash":        {ebiten.KeySlash, false},
	"backslash":    {ebiten.KeyBackslash, false},
	"minus":        {ebiten.KeyMinus, false},
	"equal":        {ebiten.KeyEqual, false},
	"plus":         {ebiten.KeyEqual, true},
	"comma":        {ebiten.KeyComma, false},
	"period":       {ebiten.KeyPeriod, false},
	"dot":          {ebiten.KeyPeriod, false},
	"semicolon":    {ebiten.KeySemicolon, false},
	"quote":        {ebiten.KeyQuote, false},
	"apostrophe":   {ebiten.KeyQuote, false},
	"backquote":    {ebiten.KeyBackquote, false},
	"grave":        {ebiten.KeyBackquote, false},
	"leftbracket":  {ebiten.KeyLeftBracket, false},
	"rightbracket": {ebiten.KeyRightBracket, false},
	"f1":           {ebiten.KeyF1, false},
	"f2":           {ebiten.KeyF2, false},
	"f3":           {ebiten.KeyF3, false},
	"f4":           {ebiten.KeyF4, false},
	"f5":           {ebiten.KeyF5, false},
	"f6":           {ebiten.KeyF6, false},
	"f7":           {ebiten.KeyF7, false},
	"f8":           {ebiten.KeyF8, false},
	"f9":           {ebiten.KeyF9, false},
	"f10":          {ebiten.KeyF10, false},
	"f11":          {ebiten.KeyF11, false},
	"f12":          {ebiten.KeyF12, false},
}

var punctuationKeys = map[rune]struct {
	key   ebiten.Key
	shift bool
}{
	'!':  {ebiten.KeyDigit1, true},
	'@':  {ebiten.KeyDigit2, true},
	'#':  {ebiten.KeyDigit3, true},
	'$':  {ebiten.KeyDigit4, true},
	'%':  {ebiten.KeyDigit5, true},
	'^':  {ebiten.KeyDigit6, true},
	'&':  {ebiten.KeyDigit7, true},
	'*':  {ebiten.KeyDigit8, true},
	'(':  {ebiten.KeyDigit9, true},
	')':  {ebiten.KeyDigit0, true},
	'-':  {ebiten.KeyMinus, false},
	'_':  {ebiten.KeyMinus, true},
	'=':  {ebiten.KeyEqual, false},
	'+':  {ebiten.KeyEqual, true},
	'[':  {ebiten.KeyLeftBracket, false},
	'{':  {ebiten.KeyLeftBracket, true},
	']':  {ebiten.KeyRightBracket, false},
	'}':  {ebiten.KeyRightBracket, true},
	';':  {ebiten.KeySemicolon, false},
	':':  {ebiten.KeySemicolon, true},
	'\'': {ebiten.KeyQuote, false},
	'"':  {ebiten.KeyQuote, true},
	',':  {ebiten.KeyComma, false},
	'<':  {ebiten.KeyComma, true},
	'.':  {ebiten.KeyPeriod, false},
	'>':  {ebiten.KeyPeriod, true},
	'/':  {ebiten.KeySlash, false},
	'?':  {ebiten.KeySlash, true},
	'\\': {ebiten.KeyBackslash, false},
	'|':  {ebiten.KeyBackslash, true},
	'`':  {ebiten.KeyBackquote, false},
	'~':  {ebiten.KeyBackquote, true},
	' ':  {ebiten.KeySpace, false},
}

func prettySpec(spec string) string {
	spec = strings.TrimSpace(spec)
	if len(spec) > len("prefix+") && strings.EqualFold(spec[:len("prefix+")], "prefix+") {
		spec = spec[len("prefix+"):]
	}
	parts := strings.Split(spec, "+")
	for i := range parts {
		switch strings.ToLower(parts[i]) {
		case "ctrl", "control":
			parts[i] = "Ctrl"
		case "alt", "option":
			parts[i] = "Alt"
		case "shift":
			parts[i] = "Shift"
		default:
			if runes := []rune(parts[i]); len(runes) == 1 && unicode.IsLower(runes[0]) {
				parts[i] = string(unicode.ToUpper(runes[0]))
			} else if len(parts[i]) > 0 {
				parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
			}
		}
	}
	return strings.Join(parts, "+")
}

func (g *game) isPrefixKey(key ebiten.Key, mods modifiers) bool {
	return g.prefix.key == key && g.prefix.mods == mods
}

func lookupBinding(bindings []keyBinding, key ebiten.Key, mods modifiers) (keyBinding, bool) {
	for _, binding := range bindings {
		if binding.key == key && binding.mods == mods {
			return binding, true
		}
	}
	return keyBinding{}, false
}

func (g *game) helpLines() []string {
	lines := []string{g.prefix.label + " is the prefix key.", ""}
	type group struct {
		action action
		keys   []string
		desc   string
	}
	var groups []group
	index := make(map[action]int)
	for _, binding := range g.bindings {
		position, ok := index[binding.action]
		if !ok {
			position = len(groups)
			index[binding.action] = position
			groups = append(groups, group{action: binding.action, desc: binding.desc})
		}
		groups[position].keys = append(groups[position].keys, binding.label)
	}
	for _, item := range groups {
		keys := strings.Join(item.keys, " / ")
		if item.action == actionSelectTab && len(item.keys) > 2 {
			keys = item.keys[0] + ".." + item.keys[len(item.keys)-1]
		}
		lines = append(lines, fmt.Sprintf("%-14s %s", keys, item.desc))
	}
	if len(groups) == 0 {
		lines = append(lines, "(no keybindings configured)")
	}
	lines = append(lines, "", "Mouse: click a pane or tab to focus it.")
	return lines
}
