package ui

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"ggterm/internal/config"
	"ggterm/internal/terminal"
)

func Test_keySequence(t *testing.T) {
	tests := []struct {
		name string
		key  ebiten.Key
		mods modifiers
		want string
		ok   bool
	}{
		{"enter", ebiten.KeyEnter, modifiers{}, "\r", true},
		{"alt enter", ebiten.KeyEnter, modifiers{alt: true}, "\x1b\r", true},
		{"tab", ebiten.KeyTab, modifiers{}, "\t", true},
		{"shift tab", ebiten.KeyTab, modifiers{shift: true}, "\x1b[Z", true},
		{"backspace", ebiten.KeyBackspace, modifiers{}, "\x7f", true},
		{"escape", ebiten.KeyEscape, modifiers{}, "\x1b", true},
		{"ctrl space", ebiten.KeySpace, modifiers{ctrl: true}, "\x00", true},
		{"plain space", ebiten.KeySpace, modifiers{}, "", false},
		{"ctrl a", ebiten.KeyA, modifiers{ctrl: true}, "\x01", true},
		{"ctrl z", ebiten.KeyZ, modifiers{ctrl: true}, "\x1a", true},
		{"ctrl left bracket", ebiten.KeyLeftBracket, modifiers{ctrl: true}, "\x1b", true},
		{"ctrl shift 6", ebiten.KeyDigit6, modifiers{ctrl: true, shift: true}, "\x1e", true},
		{"alt x", ebiten.KeyX, modifiers{alt: true}, "\x1bx", true},
		{"alt shift x", ebiten.KeyX, modifiers{alt: true, shift: true}, "\x1bX", true},
		{"plain x", ebiten.KeyX, modifiers{}, "", false},
		{"up", ebiten.KeyArrowUp, modifiers{}, "\x1b[A", true},
		{"shift up", ebiten.KeyArrowUp, modifiers{shift: true}, "\x1b[1;2A", true},
		{"ctrl left", ebiten.KeyArrowLeft, modifiers{ctrl: true}, "\x1b[1;5D", true},
		{"home", ebiten.KeyHome, modifiers{}, "\x1b[H", true},
		{"delete", ebiten.KeyDelete, modifiers{}, "\x1b[3~", true},
		{"f5", ebiten.KeyF5, modifiers{}, "\x1b[15~", true},
		{"page up handled by game", ebiten.KeyPageUp, modifiers{}, "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := keySequence(test.key, test.mods)
			if ok != test.ok {
				t.Fatalf("ok = %v, want %v", ok, test.ok)
			}
			if string(got) != test.want {
				t.Errorf("sequence = %q, want %q", got, test.want)
			}
		})
	}
}

func Test_cellColors_reverse_and_bold(t *testing.T) {
	style := terminal.Style{
		FG:      terminal.IndexedColor(1),
		BG:      terminal.IndexedColor(4),
		Bold:    true,
		Reverse: true,
	}
	theme := config.Default().Theme
	fg, bg := themeColors(theme, style)
	if fg != theme.Palette[4] {
		t.Errorf("reverse foreground = %+v, want palette 4", fg)
	}
	if bg != theme.Palette[9] {
		t.Errorf("reverse background = %+v, want bright red palette 9", bg)
	}
}

func Test_indexedColor_xterm_256(t *testing.T) {
	palette := config.Default().Theme.Palette
	if got := indexedColor(palette, 196); got != (struct{ R, G, B, A uint8 }{255, 0, 0, 255}) {
		t.Errorf("color 196 = %+v, want red", got)
	}
	if got := indexedColor(palette, 232); got.R != 8 || got.G != 8 || got.B != 8 {
		t.Errorf("color 232 = %+v, want gray 8", got)
	}
	if got := indexedColor(palette, 255); got.R != 238 {
		t.Errorf("color 255 = %+v, want gray 238", got)
	}
}

func Test_clampOffset(t *testing.T) {
	if got := clampOffset(-1, 10); got != 0 {
		t.Errorf("negative offset = %d, want 0", got)
	}
	if got := clampOffset(11, 10); got != 10 {
		t.Errorf("oversized offset = %d, want 10", got)
	}
	if got := clampOffset(4, 10); got != 4 {
		t.Errorf("valid offset = %d, want 4", got)
	}
}

func Test_lookupBinding(t *testing.T) {
	tests := []struct {
		name   string
		key    ebiten.Key
		mods   modifiers
		want   action
		arg    int
		wantOK bool
	}{
		{"new tab", ebiten.KeyC, modifiers{}, actionNewTab, 0, true},
		{"close pane", ebiten.KeyX, modifiers{}, actionClosePane, 0, true},
		{"split vertical percent", ebiten.KeyDigit5, modifiers{shift: true}, actionSplitVertical, 0, true},
		{"split vertical slash", ebiten.KeyBackslash, modifiers{}, actionSplitVertical, 0, true},
		{"split horizontal minus", ebiten.KeyMinus, modifiers{}, actionSplitHorizontal, 0, true},
		{"split horizontal quote", ebiten.KeyQuote, modifiers{shift: true}, actionSplitHorizontal, 0, true},
		{"close tab", ebiten.KeyDigit7, modifiers{shift: true}, actionCloseTab, 0, true},
		{"select tab 1", ebiten.KeyDigit1, modifiers{}, actionSelectTab, 0, true},
		{"select tab 9", ebiten.KeyDigit9, modifiers{}, actionSelectTab, 8, true},
		{"help", ebiten.KeySlash, modifiers{shift: true}, actionHelp, 0, true},
		{"quit", ebiten.KeyQ, modifiers{}, actionQuit, 0, true},
		{"ctrl blocks", ebiten.KeyC, modifiers{ctrl: true}, actionNone, 0, false},
		{"alt blocks", ebiten.KeyC, modifiers{alt: true}, actionNone, 0, false},
		{"unbound", ebiten.KeyEscape, modifiers{}, actionNone, 0, false},
	}
	keymap, _, err := buildKeymap(config.Default().Keys)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding, ok := lookupBinding(keymap, test.key, test.mods)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if binding.action != test.want {
				t.Errorf("action = %v, want %v", binding.action, test.want)
			}
			if binding.arg != test.arg {
				t.Errorf("arg = %d, want %d", binding.arg, test.arg)
			}
		})
	}
}

func Test_isPrefixKey(t *testing.T) {
	keymap, prefix, err := buildKeymap(config.Default().Keys)
	if err != nil {
		t.Fatal(err)
	}
	_ = keymap
	g := &game{prefix: prefix}
	if !g.isPrefixKey(ebiten.KeyB, modifiers{ctrl: true}) {
		t.Error("Ctrl+B is not the prefix key")
	}
	if g.isPrefixKey(ebiten.KeyB, modifiers{ctrl: true, alt: true}) {
		t.Error("Ctrl+Alt+B must not be the prefix key")
	}
	if g.isPrefixKey(ebiten.KeyB, modifiers{}) {
		t.Error("plain B must not be the prefix key")
	}
}

func Test_parseKeySpec(t *testing.T) {
	tests := []struct {
		spec string
		key  ebiten.Key
		mods modifiers
		err  bool
	}{
		{"ctrl+b", ebiten.KeyB, modifiers{ctrl: true}, false},
		{"prefix+c", ebiten.KeyC, modifiers{}, false},
		{"%", ebiten.KeyDigit5, modifiers{shift: true}, false},
		{"?", ebiten.KeySlash, modifiers{shift: true}, false},
		{"shift+slash", ebiten.KeySlash, modifiers{shift: true}, false},
		{"alt+enter", ebiten.KeyEnter, modifiers{alt: true}, false},
		{"F5", ebiten.KeyF5, modifiers{}, false},
		{"table", 0, modifiers{}, true},
		{"ctrl+nope", 0, modifiers{}, true},
		{"super+x", 0, modifiers{}, true},
	}
	for _, test := range tests {
		binding, err := parseKeySpec(test.spec)
		if (err != nil) != test.err {
			t.Errorf("%q: err = %v", test.spec, err)
			continue
		}
		if test.err {
			continue
		}
		if binding.key != test.key || binding.mods != test.mods {
			t.Errorf("%q = %v %+v, want %v %+v", test.spec, binding.key, binding.mods, test.key, test.mods)
		}
	}
}

func Test_buildKeymapOverrides(t *testing.T) {
	keys := config.Default().Keys
	keys.Prefix = "ctrl+a"
	keys.Bindings = []config.Binding{
		{Key: "c", Action: "split_vertical"},
		{Key: "q", Action: "none"},
		{Key: "1", Action: "select_tab", Arg: 3},
	}
	bindings, prefix, err := buildKeymap(keys)
	if err != nil {
		t.Fatal(err)
	}
	if prefix.key != ebiten.KeyA || prefix.mods != (modifiers{ctrl: true}) {
		t.Errorf("prefix = %v %+v", prefix.key, prefix.mods)
	}
	if binding, ok := lookupBinding(bindings, ebiten.KeyC, modifiers{}); !ok || binding.action != actionSplitVertical {
		t.Errorf("c override = %+v %v", binding, ok)
	}
	if _, ok := lookupBinding(bindings, ebiten.KeyQ, modifiers{}); ok {
		t.Error("q should have been unbound")
	}
	binding, ok := lookupBinding(bindings, ebiten.KeyDigit1, modifiers{})
	if !ok || binding.action != actionSelectTab || binding.arg != 2 {
		t.Errorf("select tab = %+v %v", binding, ok)
	}
}

func Test_buildKeymapReplace(t *testing.T) {
	keys := config.Default().Keys
	keys.Replace = true
	keys.Bindings = []config.Binding{{Key: "x", Action: "quit"}}
	bindings, _, err := buildKeymap(keys)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].action != actionQuit {
		t.Errorf("bindings = %+v", bindings)
	}
}

func Test_buildKeymapInvalidAction(t *testing.T) {
	keys := config.Default().Keys
	keys.Bindings = []config.Binding{{Key: "x", Action: "explode"}}
	if _, _, err := buildKeymap(keys); err == nil {
		t.Fatal("expected an unknown action error")
	}
}

func Test_applySettings(t *testing.T) {
	settings := config.Default()
	settings.Theme, _ = config.BuiltinTheme("nord")
	settings.Font.Size = 18
	settings.Padding = 4
	settings.Keys.Bindings = []config.Binding{{Key: "c", Action: "split_vertical"}}

	g := &game{}
	if err := g.applySettings(settings, true); err != nil {
		t.Fatal(err)
	}
	if g.padding != 4 {
		t.Errorf("padding = %d", g.padding)
	}
	if g.theme.Name != "nord" {
		t.Errorf("theme = %q", g.theme.Name)
	}
	if g.cellW <= 0 || g.cellH < 18 {
		t.Errorf("cell = %gx%g", g.cellW, g.cellH)
	}
	binding, ok := lookupBinding(g.bindings, ebiten.KeyC, modifiers{})
	if !ok || binding.action != actionSplitVertical {
		t.Errorf("binding = %+v %v", binding, ok)
	}
	if !g.isPrefixKey(ebiten.KeyB, modifiers{ctrl: true}) {
		t.Error("default prefix was not applied")
	}
}

func Test_helpLines(t *testing.T) {
	g := &game{prefix: keyBinding{label: "Ctrl+B"}}
	keymap, _, err := buildKeymap(config.Default().Keys)
	if err != nil {
		t.Fatal(err)
	}
	g.bindings = keymap
	lines := g.helpLines()
	if len(lines) < 5 || lines[0] != "Ctrl+B is the prefix key." {
		t.Errorf("help = %v", lines)
	}
	foundRange := false
	for _, line := range lines {
		if strings.Contains(line, "1..9") && strings.Contains(line, "select tab") {
			foundRange = true
		}
	}
	if !foundRange {
		t.Errorf("help does not aggregate tab bindings: %v", lines)
	}
}

func Test_fitText(t *testing.T) {
	face, _, err := loadFace(config.Default().Font)
	if err != nil {
		t.Fatal(err)
	}
	width := text.Advance("hello world", face)
	if got := fitText("hello world", width+1, face); got != "hello world" {
		t.Errorf("fitted = %q, want original", got)
	}
	if got := fitText("hello world", width/2, face); got == "hello world" || got == "" {
		t.Errorf("fitted = %q, want truncated text", got)
	}
}
