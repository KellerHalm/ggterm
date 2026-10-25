package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

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
	fg, bg := cellColors(style)
	if fg != ansiPalette[4] {
		t.Errorf("reverse foreground = %+v, want palette 4", fg)
	}
	if bg != ansiPalette[9] {
		t.Errorf("reverse background = %+v, want bright red palette 9", bg)
	}
}

func Test_indexedColor_xterm_256(t *testing.T) {
	if got := indexedColor(196); got != (struct{ R, G, B, A uint8 }{255, 0, 0, 255}) {
		t.Errorf("color 196 = %+v, want red", got)
	}
	if got := indexedColor(232); got.R != 8 || got.G != 8 || got.B != 8 {
		t.Errorf("color 232 = %+v, want gray 8", got)
	}
	if got := indexedColor(255); got.R != 238 {
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binding, ok := lookupBinding(test.key, test.mods)
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
	if !isPrefixKey(ebiten.KeyB, modifiers{ctrl: true}) {
		t.Error("Ctrl+B is not the prefix key")
	}
	if isPrefixKey(ebiten.KeyB, modifiers{ctrl: true, alt: true}) {
		t.Error("Ctrl+Alt+B must not be the prefix key")
	}
	if isPrefixKey(ebiten.KeyB, modifiers{}) {
		t.Error("plain B must not be the prefix key")
	}
}

func Test_fitText(t *testing.T) {
	face, _, err := newFace(15)
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
