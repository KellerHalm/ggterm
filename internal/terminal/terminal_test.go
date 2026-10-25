package terminal

import (
	"strings"
	"sync"
	"testing"
)

func Test_Terminal_New_applies_defaults(t *testing.T) {
	term := New(Config{})
	if term.cfg.Cols != 80 {
		t.Errorf("expected Cols 80, got %d", term.cfg.Cols)
	}
	if term.cfg.Rows != 24 {
		t.Errorf("expected Rows 24, got %d", term.cfg.Rows)
	}
	if term.cfg.MaxScrollback != 1000 {
		t.Errorf("expected MaxScrollback 1000, got %d", term.cfg.MaxScrollback)
	}
	if term.cfg.TabWidth != 8 {
		t.Errorf("expected TabWidth 8, got %d", term.cfg.TabWidth)
	}
}

func Test_Terminal_Write_prints_ASCII(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write([]byte("hello"))
	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'h' {
		t.Errorf("expected 'h' at [0,0], got %q", snap.Lines[0][0].Rune)
	}
	if snap.Lines[0][4].Rune != 'o' {
		t.Errorf("expected 'o' at [0,4], got %q", snap.Lines[0][4].Rune)
	}
	if snap.Cursor.Row != 0 || snap.Cursor.Col != 5 {
		t.Errorf("expected cursor (0,5), got (%d,%d)", snap.Cursor.Row, snap.Cursor.Col)
	}
}

func Test_Terminal_SGR_colors_and_attributes(t *testing.T) {
	tests := []struct {
		name      string
		seq       string
		checkFG   bool
		wantFG    Color
		checkBG   bool
		wantBG    Color
		checkBold bool
		wantBold  bool
	}{
		{"fg red", "\x1b[31mA", true, IndexedColor(1), false, Color{}, false, false},
		{"bg blue", "\x1b[44mA", false, Color{}, true, IndexedColor(4), false, false},
		{"bold", "\x1b[1mA", false, Color{}, false, Color{}, true, true},
		{"bright cyan", "\x1b[96mA", true, IndexedColor(14), false, Color{}, false, false},
		{"256 fg", "\x1b[38;5;123mA", true, IndexedColor(123), false, Color{}, false, false},
		{"256 bg", "\x1b[48;5;200mA", false, Color{}, true, IndexedColor(200), false, false},
		{"rgb fg", "\x1b[38;2;10;20;30mA", true, RGBColor(10, 20, 30), false, Color{}, false, false},
		{"rgb bg", "\x1b[48;2;50;60;70mA", false, Color{}, true, RGBColor(50, 60, 70), false, false},
		{"reset", "\x1b[1;31m\x1b[0mA", true, Color{}, false, Color{}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := New(Config{Cols: 10, Rows: 5})
			_, _ = term.Write([]byte(tt.seq))
			snap := term.Snapshot(0)
			cell := snap.Lines[0][0]
			if cell.Rune != 'A' {
				t.Errorf("expected 'A', got %q", cell.Rune)
			}
			if tt.checkFG && cell.Style.FG != tt.wantFG {
				t.Errorf("FG: expected %+v, got %+v", tt.wantFG, cell.Style.FG)
			}
			if tt.checkBG && cell.Style.BG != tt.wantBG {
				t.Errorf("BG: expected %+v, got %+v", tt.wantBG, cell.Style.BG)
			}
			if tt.checkBold && cell.Style.Bold != tt.wantBold {
				t.Errorf("Bold: expected %v, got %v", tt.wantBold, cell.Style.Bold)
			}
		})
	}
}

func Test_Terminal_underline_styles_and_colour(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write([]byte("\x1b[4mA\x1b[21mB\x1b[58;5;196mC\x1b[59mD\x1b[24mE"))
	snap := term.Snapshot(0)
	line := snap.Lines[0]

	if !line[0].Style.Underline || line[0].Style.DoubleUnderline {
		t.Errorf("A underline = %+v", line[0].Style)
	}
	if !line[1].Style.Underline || !line[1].Style.DoubleUnderline {
		t.Errorf("B double underline = %+v", line[1].Style)
	}
	if line[2].Style.UnderlineColor != IndexedColor(196) {
		t.Errorf("C underline colour = %+v", line[2].Style.UnderlineColor)
	}
	if line[3].Style.UnderlineColor != (Color{}) {
		t.Errorf("D underline colour = %+v", line[3].Style.UnderlineColor)
	}
	if line[4].Style.Underline || line[4].Style.DoubleUnderline {
		t.Errorf("E underline = %+v", line[4].Style)
	}
}

func Test_Terminal_cursor_movement(t *testing.T) {
	tests := []struct {
		name    string
		seq     string
		wantRow int
		wantCol int
	}{
		{"up", "A\x1b[A", 0, 1},
		{"down", "\x1b[B", 1, 0},
		{"forward", "\x1b[C", 0, 1},
		{"back", "AA\x1b[D", 0, 1},
		{"position", "\x1b[3;5H", 2, 4},
		{"next line", "\x1b[E", 1, 0},
		{"prev line", "\x1b[2B\x1b[F", 1, 0},
		{"col absolute", "AAA\x1b[2G", 0, 1},
		{"row absolute", "\x1b[3d", 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := New(Config{Cols: 10, Rows: 5})
			_, _ = term.Write([]byte(tt.seq))
			snap := term.Snapshot(0)
			if snap.Cursor.Row != tt.wantRow || snap.Cursor.Col != tt.wantCol {
				t.Errorf("expected cursor (%d,%d), got (%d,%d)", tt.wantRow, tt.wantCol, snap.Cursor.Row, snap.Cursor.Col)
			}
		})
	}
}

func Test_Terminal_erase_display_and_line(t *testing.T) {
	tests := []struct {
		name     string
		setup    string
		seq      string
		checkRow int
		checkCol int
		wantRune rune
	}{
		{"ED cursor to end", "ABC\x1b[1;2H", "\x1b[J", 0, 2, ' '},
		{"ED start to cursor", "ABC\x1b[1;2H", "\x1b[1J", 0, 1, ' '},
		{"ED whole screen", "ABC", "\x1b[2J", 0, 0, ' '},
		{"EL cursor to end", "ABC\x1b[1;2H", "\x1b[K", 0, 2, ' '},
		{"EL start to cursor", "ABC\x1b[1;2H", "\x1b[1K", 0, 1, ' '},
		{"EL whole line", "ABC", "\x1b[2K", 0, 2, ' '},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := New(Config{Cols: 10, Rows: 5})
			_, _ = term.Write([]byte(tt.setup))
			_, _ = term.Write([]byte(tt.seq))
			snap := term.Snapshot(0)
			got := snap.Lines[tt.checkRow][tt.checkCol].Rune
			if got != tt.wantRune {
				t.Errorf("expected %q at [%d,%d], got %q", tt.wantRune, tt.checkRow, tt.checkCol, got)
			}
		})
	}
}

func Test_Terminal_scrollback(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3, MaxScrollback: 10})
	for i := 0; i < 5; i++ {
		_, _ = term.Write([]byte{byte('A' + i)})
		if i < 4 {
			_, _ = term.Write([]byte("\r\n"))
		}
	}
	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'C' {
		t.Errorf("expected 'C' on screen line 0, got %q", snap.Lines[0][0].Rune)
	}
	if snap.ScrollbackLen != 2 {
		t.Errorf("expected scrollback len 2, got %d", snap.ScrollbackLen)
	}
	snapBack := term.Snapshot(2)
	if snapBack.Lines[0][0].Rune != 'A' {
		t.Errorf("expected 'A' in scrollback, got %q", snapBack.Lines[0][0].Rune)
	}
	if snapBack.Cursor.Visible {
		t.Error("cursor should be hidden when scrolled")
	}
}

func Test_Terminal_resize(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("ABCDE"))
	term.Resize(3, 2)
	snap := term.Snapshot(0)
	if snap.Cols != 3 || snap.Rows != 2 {
		t.Errorf("expected dims (3,2), got (%d,%d)", snap.Cols, snap.Rows)
	}
	if snap.Lines[0][0].Rune != 'A' {
		t.Errorf("expected 'A' preserved after resize, got %q", snap.Lines[0][0].Rune)
	}
}

func Test_Terminal_split_UTF8(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	utf8 := []byte("日")
	_, _ = term.Write(utf8[:1])
	_, _ = term.Write(utf8[1:2])
	_, _ = term.Write(utf8[2:])
	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != '日' {
		t.Errorf("expected '日', got %q", snap.Lines[0][0].Rune)
	}
	if snap.Lines[0][0].Width != 2 {
		t.Errorf("expected width 2 for CJK, got %d", snap.Lines[0][0].Width)
	}
}

func Test_Terminal_OSC_title(t *testing.T) {
	tests := []struct {
		name string
		seq  string
		want string
	}{
		{"OSC 0 BEL", "\x1b]0;MyTitle\x07", "MyTitle"},
		{"OSC 2 ST", "\x1b]2;OtherTitle\x1b\\", "OtherTitle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := New(Config{})
			_, _ = term.Write([]byte(tt.seq))
			got := term.Title()
			if got != tt.want {
				t.Errorf("expected title %q, got %q", tt.want, got)
			}
		})
	}
}

func Test_Terminal_wide_CJK(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("日本"))
	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != '日' || snap.Lines[0][0].Width != 2 {
		t.Errorf("expected '日' width 2 at [0,0], got %q width %d", snap.Lines[0][0].Rune, snap.Lines[0][0].Width)
	}
	if snap.Lines[0][1].Width != 0 {
		t.Errorf("expected continuation cell width 0 at [0,1], got %d", snap.Lines[0][1].Width)
	}
	if snap.Lines[0][2].Rune != '本' {
		t.Errorf("expected '本' at [0,2], got %q", snap.Lines[0][2].Rune)
	}
}

func Test_Terminal_autowrap(t *testing.T) {
	term := New(Config{Cols: 3, Rows: 3})
	_, _ = term.Write([]byte("ABCD"))
	snap := term.Snapshot(0)
	if snap.Lines[0][2].Rune != 'C' {
		t.Errorf("expected 'C' at end of line 0, got %q", snap.Lines[0][2].Rune)
	}
	if snap.Lines[1][0].Rune != 'D' {
		t.Errorf("expected 'D' wrapped to line 1, got %q", snap.Lines[1][0].Rune)
	}
}

func Test_Terminal_control_chars(t *testing.T) {
	tests := []struct {
		name    string
		seq     string
		wantRow int
		wantCol int
	}{
		{"CR", "AAA\r", 0, 0},
		{"LF", "A\n", 1, 1},
		{"BS", "AAA\b", 0, 2},
		{"HT", "\t", 0, 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := New(Config{Cols: 10, Rows: 5})
			_, _ = term.Write([]byte(tt.seq))
			snap := term.Snapshot(0)
			if snap.Cursor.Row != tt.wantRow || snap.Cursor.Col != tt.wantCol {
				t.Errorf("expected cursor (%d,%d), got (%d,%d)", tt.wantRow, tt.wantCol, snap.Cursor.Row, snap.Cursor.Col)
			}
		})
	}
}

func Test_Terminal_alternate_screen(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("MAIN"))
	_, _ = term.Write([]byte("\x1b[?1049h"))
	snap := term.Snapshot(0)
	if !snap.AltScreen {
		t.Error("expected alt screen active")
	}
	if snap.Lines[0][0].Rune != ' ' {
		t.Errorf("expected blank alt screen, got %q", snap.Lines[0][0].Rune)
	}
	_, _ = term.Write([]byte("ALT"))
	_, _ = term.Write([]byte("\x1b[?1049l"))
	snap = term.Snapshot(0)
	if snap.AltScreen {
		t.Error("expected main screen restored")
	}
	if snap.Lines[0][0].Rune != 'M' {
		t.Errorf("expected main screen content 'M', got %q", snap.Lines[0][0].Rune)
	}
}

func Test_Terminal_concurrent_write_snapshot(t *testing.T) {
	term := New(Config{Cols: 80, Rows: 24, MaxScrollback: 100})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_, _ = term.Write([]byte(strings.Repeat("X", 70) + "\n"))
		}
	}()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = term.Snapshot(0)
			}
		}()
	}
	wg.Wait()
	<-done
	snap := term.Snapshot(0)
	if snap.Cols != 80 || snap.Rows != 24 {
		t.Errorf("race detector should catch data races, dims (%d,%d)", snap.Cols, snap.Rows)
	}
}

func Test_Terminal_insert_delete_lines(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("A\nB\nC"))
	_, _ = term.Write([]byte("\x1b[1;1H"))
	_, _ = term.Write([]byte("\x1b[L"))
	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != ' ' {
		t.Errorf("expected blank inserted line at [0,0], got %q", snap.Lines[0][0].Rune)
	}
	if snap.Lines[1][0].Rune != 'A' {
		t.Errorf("expected 'A' pushed down to [1,0], got %q", snap.Lines[1][0].Rune)
	}
}

func Test_Terminal_insert_delete_chars(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("ABCDE"))
	_, _ = term.Write([]byte("\x1b[1;2H"))
	_, _ = term.Write([]byte("\x1b[@"))
	snap := term.Snapshot(0)
	if snap.Lines[0][1].Rune != ' ' {
		t.Errorf("expected blank inserted at [0,1], got %q", snap.Lines[0][1].Rune)
	}
	if snap.Lines[0][2].Rune != 'B' {
		t.Errorf("expected 'B' shifted to [0,2], got %q", snap.Lines[0][2].Rune)
	}
}

func Test_Terminal_erase_chars(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("ABCDE"))
	_, _ = term.Write([]byte("\x1b[1;2H"))
	_, _ = term.Write([]byte("\x1b[2X"))
	snap := term.Snapshot(0)
	if snap.Lines[0][1].Rune != ' ' {
		t.Errorf("expected blank at [0,1], got %q", snap.Lines[0][1].Rune)
	}
	if snap.Lines[0][2].Rune != ' ' {
		t.Errorf("expected blank at [0,2], got %q", snap.Lines[0][2].Rune)
	}
	if snap.Lines[0][3].Rune != 'D' {
		t.Errorf("expected 'D' preserved at [0,3], got %q", snap.Lines[0][3].Rune)
	}
}

func Test_Terminal_scroll_up_down(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	_, _ = term.Write([]byte("A\r\nB\r\nC"))
	_, _ = term.Write([]byte("\x1b[S"))
	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'B' {
		t.Errorf("expected 'B' after scroll up, got %q", snap.Lines[0][0].Rune)
	}
}

func Test_Terminal_save_restore_cursor(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write([]byte("\x1b[3;5H"))
	_, _ = term.Write([]byte("\x1b7"))
	_, _ = term.Write([]byte("\x1b[1;1H"))
	_, _ = term.Write([]byte("\x1b8"))
	snap := term.Snapshot(0)
	if snap.Cursor.Row != 2 || snap.Cursor.Col != 4 {
		t.Errorf("expected cursor restored to (2,4), got (%d,%d)", snap.Cursor.Row, snap.Cursor.Col)
	}
}

func Test_Terminal_cursor_visibility(t *testing.T) {
	term := New(Config{})
	snap := term.Snapshot(0)
	if !snap.Cursor.Visible {
		t.Error("cursor should be visible by default")
	}
	_, _ = term.Write([]byte("\x1b[?25l"))
	snap = term.Snapshot(0)
	if snap.Cursor.Visible {
		t.Error("cursor should be hidden after DECTCEM reset")
	}
	_, _ = term.Write([]byte("\x1b[?25h"))
	snap = term.Snapshot(0)
	if !snap.Cursor.Visible {
		t.Error("cursor should be visible after DECTCEM set")
	}
}
