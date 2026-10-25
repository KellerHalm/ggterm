package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gomono"

	"ggterm/internal/config"
	"ggterm/internal/mux"
	"ggterm/internal/terminal"
	"ggterm/internal/version"
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
		{"file manager", ebiten.KeyY, modifiers{}, actionFileManager, 0, true},
		{"help", ebiten.KeySlash, modifiers{shift: true}, actionHelp, 0, true},
		{"quit", ebiten.KeyQ, modifiers{}, actionQuit, 0, true},
		{"ctrl blocks", ebiten.KeyC, modifiers{ctrl: true}, actionNone, 0, false},
		{"alt blocks", ebiten.KeyC, modifiers{alt: true}, actionNone, 0, false},
		{"unbound", ebiten.KeyEscape, modifiers{}, actionNone, 0, false},
	}
	keymap, _, err := buildKeymap(config.Default().Keys, nil)
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
	keymap, prefix, err := buildKeymap(config.Default().Keys, nil)
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
	bindings, prefix, err := buildKeymap(keys, nil)
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
	bindings, _, err := buildKeymap(keys, nil)
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
	if _, _, err := buildKeymap(keys, nil); err == nil {
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
	keymap, _, err := buildKeymap(config.Default().Keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	g.bindings = keymap
	lines := g.helpLines()
	if len(lines) < 6 || lines[0] != "ggterm "+version.String() || lines[1] != "Ctrl+B is the prefix key." {
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

func Test_classifyFontFilename(t *testing.T) {
	tests := []struct {
		name   string
		family string
		style  fontStyle
	}{
		{"JetBrainsMono-Regular.ttf", "jetbrainsmono", fontRegular},
		{"JetBrainsMono-Bold.ttf", "jetbrainsmono", fontBold},
		{"JetBrainsMono-Italic.ttf", "jetbrainsmono", fontItalic},
		{"JetBrainsMono-BoldItalic.ttf", "jetbrainsmono", fontBoldItalic},
		{"SourceCodePro-Oblique.otf", "sourcecodepro", fontItalic},
		{"RobotoMono-SemiBold.ttf", "robotomono", fontBold},
		{"JetBrainsMonoBold.ttf", "jetbrainsmono", fontBold},
		{"Bookman Old Style-Bold.ttf", "bookmanoldstyle", fontBold},
		{"Arial Narrow.ttf", "arialnarrow", fontRegular},
		{"RobotoCondensed-Bold.ttf", "robotocondensed", fontBold},
		{"RobotoMono-Light.ttf", "robotomonolight", fontRegular},
		{"Times New Roman.ttf", "timesnewroman", fontRegular},
		{"Blackletter.ttf", "blackletter", fontRegular},
		{"Segoe UI.ttf", "segoeui", fontRegular},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			family, style := classifyFontFilename(test.name)
			if family != test.family || style != test.style {
				t.Errorf("got %q %v, want %q %v", family, style, test.family, test.style)
			}
		})
	}
}

func Test_fontIndexPrefersRegularBold(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"RankMono-ExtraBold.ttf", "RankMono-SemiBold.ttf",
		"RankMono-Bold.ttf", "RankMono-Light.ttf", "RankMono-Regular.ttf",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := fontIndexFor([]string{dir})
	path, exact := index.lookup("Rank Mono", fontBold)
	if !exact || filepath.Base(path) != "RankMono-Bold.ttf" {
		t.Errorf("bold = %q exact=%v", path, exact)
	}
	path, exact = index.lookup("Rank Mono", fontRegular)
	if !exact || filepath.Base(path) != "RankMono-Regular.ttf" {
		t.Errorf("regular = %q exact=%v", path, exact)
	}
}

func Test_fontSetBuiltinSynthesizesStyles(t *testing.T) {
	font := config.Default().Font
	set, err := loadFontSet(font)
	if err != nil {
		t.Fatal(err)
	}
	if !set.ligatures {
		t.Error("ligatures should be enabled by default")
	}
	if !set.variants[fontBold].syntheticBold {
		t.Error("builtin bold should be synthesised")
	}
	if !set.variants[fontItalic].syntheticItalic {
		t.Error("builtin italic should be synthesised")
	}
	if !set.variants[fontBoldItalic].syntheticBold || !set.variants[fontBoldItalic].syntheticItalic {
		t.Error("builtin bold italic should synthesise both styles")
	}
	selected := set.selectFace(terminal.Style{Italic: true})
	if selected.face != set.variants[fontItalic].face {
		t.Error("style selection did not choose the italic face")
	}
}

func Test_fontSetDiscoversSiblingVariants(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"TestMono-Regular.ttf", "TestMono-Bold.ttf", "TestMono-Italic.ttf", "TestMono-BoldItalic.ttf"} {
		if err := os.WriteFile(filepath.Join(dir, name), gomono.TTF, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	font := config.Default().Font
	font.Path = filepath.Join(dir, "TestMono-Regular.ttf")
	font.Size = 16
	set, err := loadFontSet(font)
	if err != nil {
		t.Fatal(err)
	}
	if set.variants[fontBold].syntheticBold {
		t.Error("bold sibling was not detected")
	}
	if set.variants[fontItalic].syntheticItalic {
		t.Error("italic sibling was not detected")
	}
	if set.variants[fontBoldItalic].syntheticBold || set.variants[fontBoldItalic].syntheticItalic {
		t.Error("bold italic sibling was not detected")
	}
}

func Test_fontSetFindsSystemFamily(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"SysMono-Regular.ttf", "SysMono-Bold.ttf", "SysMono-Italic.ttf"} {
		if err := os.WriteFile(filepath.Join(dir, name), gomono.TTF, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GGTERM_FONT_DIRS", dir)

	font := config.Default().Font
	font.Family = "Sys Mono"
	font.Size = 16
	set, err := loadFontSet(font)
	if err != nil {
		t.Fatal(err)
	}
	if set.variants[fontBold].syntheticBold {
		t.Error("system bold face was not found")
	}
	if set.variants[fontItalic].syntheticItalic {
		t.Error("system italic face was not found")
	}
	if !set.variants[fontBoldItalic].syntheticItalic {
		t.Error("bold italic should fall back to bold with synthetic italic")
	}
}

func Test_rectBatch(t *testing.T) {
	var batch rectBatch
	batch.add(1, 2, 3, 4, color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff})
	batch.add(5, 6, 7, 8, color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	if len(batch.vertices) != 8 || len(batch.indices) != 12 {
		t.Fatalf("batch = %d vertices, %d indices", len(batch.vertices), len(batch.indices))
	}
	if batch.vertices[0].DstX != 1 || batch.vertices[0].DstY != 2 || batch.vertices[0].ColorR != 0x10/255.0 {
		t.Errorf("first vertex = %+v", batch.vertices[0])
	}
	if batch.indices[0] != 0 || batch.indices[6] != 4 || batch.indices[11] != 6 {
		t.Errorf("indices = %v", batch.indices)
	}
	batch.reset()
	if len(batch.vertices) != 0 || len(batch.indices) != 0 {
		t.Error("reset did not clear the batch")
	}
}

func Test_cursorWidth(t *testing.T) {
	content := mux.Rect{W: 4, H: 2}
	snap := terminal.Snapshot{
		Cursor: terminal.Cursor{Row: 0, Col: 1, Visible: true},
		Lines: [][]terminal.Cell{
			{{Rune: 'a', Width: 1}, {Rune: '漢', Width: 2}, {Rune: 0, Width: 0}},
			{{Rune: 'b', Width: 1}},
		},
	}
	if got := cursorWidth(snap, content); got != 2 {
		t.Errorf("wide cursor width = %d, want 2", got)
	}
	snap.Cursor.Col = 0
	if got := cursorWidth(snap, content); got != 1 {
		t.Errorf("narrow cursor width = %d, want 1", got)
	}
	snap.Cursor.Col = 1
	if got := cursorWidth(snap, mux.Rect{W: 2, H: 2}); got != 1 {
		t.Errorf("cursor on wide glyph at edge = %d, want 1", got)
	}
}

func Test_snapshotBlinking(t *testing.T) {
	content := mux.Rect{W: 2, H: 2}
	calm := terminal.Snapshot{Lines: [][]terminal.Cell{
		{{Rune: 'a', Width: 1}, {Rune: 'b', Width: 1}},
	}}
	if snapshotBlinking(calm, content) {
		t.Error("plain snapshot reported blinking")
	}
	blinking := terminal.Snapshot{Lines: [][]terminal.Cell{
		{{Rune: 'a', Width: 1}, {Rune: 'b', Width: 1, Style: terminal.Style{Blink: true}}},
		{{Rune: 'c', Width: 1}},
	}}
	if !snapshotBlinking(blinking, content) {
		t.Error("blinking cell was not detected")
	}
	if snapshotBlinking(blinking, mux.Rect{W: 1, H: 2}) {
		t.Error("blinking cell outside the content width was detected")
	}
}

func Test_fitText(t *testing.T) {
	fonts, err := loadFontSet(config.Default().Font)
	if err != nil {
		t.Fatal(err)
	}
	face := fonts.variants[fontRegular].face
	width := text.Advance("hello world", face)
	if got := fitText("hello world", width+1, face); got != "hello world" {
		t.Errorf("fitted = %q, want original", got)
	}
	if got := fitText("hello world", width/2, face); got == "hello world" || got == "" {
		t.Errorf("fitted = %q, want truncated text", got)
	}
}

func Test_buildKeymapPluginCommand(t *testing.T) {
	keys := config.Default().Keys
	bindings, _, err := buildKeymap(keys, []config.Binding{
		{Key: "z", Action: "plugin:hello", Arg: 3, Desc: "say hello"},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := lookupBinding(bindings, ebiten.KeyZ, modifiers{})
	if !ok {
		t.Fatal("plugin binding missing")
	}
	if binding.action != actionCommand || binding.command != "hello" || binding.arg != 3 {
		t.Errorf("binding = %+v", binding)
	}
	if binding.desc != "say hello" {
		t.Errorf("desc = %q", binding.desc)
	}

	keys.Unbind = []string{"z"}
	bindings, _, err = buildKeymap(keys, []config.Binding{{Key: "z", Action: "plugin:hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lookupBinding(bindings, ebiten.KeyZ, modifiers{}); ok {
		t.Error("user unbind should remove the plugin binding")
	}
}

func Test_keyChordName(t *testing.T) {
	tests := []struct {
		key  ebiten.Key
		mods modifiers
		want string
	}{
		{ebiten.KeyA, modifiers{ctrl: true}, "ctrl+a"},
		{ebiten.KeyDigit5, modifiers{shift: true}, "shift+5"},
		{ebiten.KeyArrowLeft, modifiers{alt: true}, "alt+left"},
		{ebiten.KeyF5, modifiers{}, "f5"},
		{ebiten.KeySlash, modifiers{ctrl: true, shift: true}, "ctrl+shift+slash"},
	}
	for _, test := range tests {
		if got := keyChordName(test.key, test.mods); got != test.want {
			t.Errorf("%v %+v = %q, want %q", test.key, test.mods, got, test.want)
		}
	}
}
