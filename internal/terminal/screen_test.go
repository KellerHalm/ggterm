package terminal

import "testing"

func mustWrite(t *testing.T, term *Terminal, input string) {
	t.Helper()
	if _, err := term.Write([]byte(input)); err != nil {
		t.Fatalf("Write(%q) failed: %v", input, err)
	}
}

func Test_Terminal_scroll_region_confines_scrolling(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 5})
	mustWrite(t, term, "L0\r\nL1\r\nL2\r\nL3\r\nL4")
	mustWrite(t, term, "\x1b[2;4r\x1b[4;1H\n")

	snap := term.Snapshot(0)
	want := []rune{'L', 'L', 'L', ' ', 'L'}
	for row, expected := range want {
		if got := snap.Lines[row][0].Rune; got != expected {
			t.Errorf("row %d first rune = %q, want %q", row, got, expected)
		}
	}
	if snap.ScrollbackLen != 0 {
		t.Errorf("scrollback len = %d, want 0 for a region below the top", snap.ScrollbackLen)
	}
}

func Test_Terminal_insert_lines_stays_in_scroll_region(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 5})
	mustWrite(t, term, "L0\r\nL1\r\nL2\r\nL3\r\nL4")
	mustWrite(t, term, "\x1b[2;4r\x1b[2;1H\x1b[L")

	snap := term.Snapshot(0)
	got := []rune{
		snap.Lines[0][0].Rune,
		snap.Lines[1][0].Rune,
		snap.Lines[2][0].Rune,
		snap.Lines[3][0].Rune,
		snap.Lines[4][0].Rune,
	}
	want := []rune{'L', ' ', 'L', 'L', 'L'}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d first rune = %q, want %q", i, got[i], want[i])
		}
	}
}

func Test_Terminal_origin_mode_offsets_cursor(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 5})
	mustWrite(t, term, "\x1b[2;4r\x1b[?6h")

	if snap := term.Snapshot(0); snap.Cursor.Row != 1 {
		t.Fatalf("after enabling origin mode cursor row = %d, want 1", snap.Cursor.Row)
	}

	mustWrite(t, term, "\x1b[3;1H")
	if snap := term.Snapshot(0); snap.Cursor.Row != 3 {
		t.Errorf("CUP 3;1 in origin mode row = %d, want 3", snap.Cursor.Row)
	}

	mustWrite(t, term, "\x1b[?6l")
	if snap := term.Snapshot(0); snap.Cursor.Row != 0 {
		t.Errorf("after disabling origin mode cursor row = %d, want 0", snap.Cursor.Row)
	}
}

func Test_Terminal_set_scroll_region_homes_cursor(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 5})
	mustWrite(t, term, "\x1b[5;5H\x1b[2;4r")

	snap := term.Snapshot(0)
	if snap.Cursor.Row != 0 || snap.Cursor.Col != 0 {
		t.Errorf("cursor = (%d,%d), want (0,0)", snap.Cursor.Row, snap.Cursor.Col)
	}
}

func Test_Terminal_wide_rune_wraps_when_it_does_not_fit(t *testing.T) {
	term := New(Config{Cols: 3, Rows: 2})
	mustWrite(t, term, "ab日")

	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'a' || snap.Lines[0][1].Rune != 'b' {
		t.Errorf("first row = %q%q", snap.Lines[0][0].Rune, snap.Lines[0][1].Rune)
	}
	if snap.Lines[1][0].Rune != '日' || snap.Lines[1][0].Width != 2 {
		t.Errorf("wrapped rune = %q width %d, want 日 width 2", snap.Lines[1][0].Rune, snap.Lines[1][0].Width)
	}
}

func Test_Terminal_overwriting_wide_rune_clears_continuation(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 2})
	mustWrite(t, term, "日\x1b[1;1Hx")

	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'x' {
		t.Errorf("cell [0,0] = %q, want x", snap.Lines[0][0].Rune)
	}
	if snap.Lines[0][1].Rune != ' ' || snap.Lines[0][1].Width != 1 {
		t.Errorf("cell [0,1] = %q width %d, want blank width 1", snap.Lines[0][1].Rune, snap.Lines[0][1].Width)
	}
}

func Test_Terminal_pending_wrap_is_cancelled_by_carriage_return(t *testing.T) {
	term := New(Config{Cols: 3, Rows: 2})
	mustWrite(t, term, "ABC\rD")

	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'D' {
		t.Errorf("cell [0,0] = %q, want D", snap.Lines[0][0].Rune)
	}
	if snap.Cursor.Row != 0 {
		t.Errorf("cursor row = %d, want 0", snap.Cursor.Row)
	}
}

func Test_Terminal_scrollback_respects_limit(t *testing.T) {
	term := New(Config{Cols: 2, Rows: 2, MaxScrollback: 3})
	mustWrite(t, term, "A\r\nB\r\nC\r\nD\r\nE\r\nF")

	snap := term.Snapshot(0)
	if snap.ScrollbackLen != 3 {
		t.Fatalf("scrollback len = %d, want 3", snap.ScrollbackLen)
	}
	oldest := term.Snapshot(3)
	if oldest.Lines[0][0].Rune != 'B' {
		t.Errorf("oldest retained line = %q, want B", oldest.Lines[0][0].Rune)
	}
}

func Test_Terminal_alt_screen_has_no_scrollback_and_clears_on_reentry(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	mustWrite(t, term, "MAIN")
	mustWrite(t, term, "\x1b[?1049hA\r\nB\r\nC\r\nD")

	if snap := term.Snapshot(0); snap.ScrollbackLen != 0 {
		t.Errorf("alt screen scrollback len = %d, want 0", snap.ScrollbackLen)
	}

	mustWrite(t, term, "\x1b[?1049l")
	if snap := term.Snapshot(0); snap.Lines[0][0].Rune != 'M' {
		t.Errorf("main screen first rune = %q, want M", snap.Lines[0][0].Rune)
	}

	mustWrite(t, term, "\x1b[?1049h")
	snap := term.Snapshot(0)
	if !snap.AltScreen || snap.Lines[0][0].Rune != ' ' {
		t.Errorf("re-entered alt screen = active:%v first:%q, want blank", snap.AltScreen, snap.Lines[0][0].Rune)
	}
}

func Test_Terminal_reset_while_in_alt_screen_restores_main(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	mustWrite(t, term, "MAIN\x1b[?1049hALT\x1bc")

	snap := term.Snapshot(0)
	if snap.AltScreen {
		t.Error("alt screen still active after reset")
	}
	if snap.Lines[0][0].Rune != ' ' {
		t.Errorf("main screen was not cleared, first rune = %q", snap.Lines[0][0].Rune)
	}
	if snap.ScrollbackLen != 0 {
		t.Errorf("scrollback len = %d, want 0", snap.ScrollbackLen)
	}
}

func Test_Terminal_resize_preserves_scrollback(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3, MaxScrollback: 10})
	mustWrite(t, term, "A\r\nB\r\nC\r\nD\r\nE")
	term.Resize(3, 2)

	snap := term.Snapshot(0)
	if snap.Cols != 3 || snap.Rows != 2 {
		t.Fatalf("dimensions = %dx%d, want 3x2", snap.Cols, snap.Rows)
	}
	if snap.ScrollbackLen != 2 {
		t.Errorf("scrollback len = %d, want 2", snap.ScrollbackLen)
	}
	if snap.Lines[0][0].Rune != 'C' || snap.Lines[1][0].Rune != 'D' {
		t.Errorf("visible lines = %q,%q, want C,D", snap.Lines[0][0].Rune, snap.Lines[1][0].Rune)
	}
	if snap.Cursor.Row >= snap.Rows || snap.Cursor.Col >= snap.Cols {
		t.Errorf("cursor (%d,%d) outside resized screen", snap.Cursor.Row, snap.Cursor.Col)
	}
}

func Test_Terminal_insert_mode_shifts_existing_text(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 2})
	mustWrite(t, term, "abc\x1b[1;1H\x1b[4hX")

	snap := term.Snapshot(0)
	got := []rune{
		snap.Lines[0][0].Rune,
		snap.Lines[0][1].Rune,
		snap.Lines[0][2].Rune,
		snap.Lines[0][3].Rune,
	}
	want := []rune{'X', 'a', 'b', 'c'}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row = %q, want %q", string(got), string(want))
		}
	}

	mustWrite(t, term, "\x1b[4lY")
	if got := term.Snapshot(0).Lines[0][1].Rune; got != 'Y' {
		t.Errorf("after disabling insert mode cell [0,1] = %q, want Y", got)
	}
}

func Test_Terminal_newline_mode_moves_to_column_zero(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	mustWrite(t, term, "\x1b[20hA\nB")

	if got := term.Snapshot(0).Lines[1][0].Rune; got != 'B' {
		t.Errorf("cell [1,0] = %q, want B", got)
	}
}

func Test_Terminal_soft_reset_keeps_screen(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	mustWrite(t, term, "\x1b[31;1m\x1b[4h\x1b[?25l\x1b[2;3rX\x1b[!pZ")

	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'X' {
		t.Errorf("screen was cleared by soft reset: [0,0] = %q", snap.Lines[0][0].Rune)
	}
	cell := snap.Lines[0][1]
	if cell.Rune != 'Z' || cell.Style != (Style{}) {
		t.Errorf("cell after soft reset = %q style %+v, want plain Z", cell.Rune, cell.Style)
	}
	if !snap.Cursor.Visible {
		t.Error("cursor remains hidden after soft reset")
	}
	if snap.Lines[0][2].Rune != ' ' {
		t.Errorf("insert mode survived soft reset: [0,2] = %q", snap.Lines[0][2].Rune)
	}
}

func Test_Terminal_full_reset_clears_modes(t *testing.T) {
	term := New(Config{Cols: 5, Rows: 3})
	mustWrite(t, term, "\x1b[4h\x1b[20h\x1b[31mA\x1bcB")

	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'B' {
		t.Errorf("cell [0,0] = %q, want B", snap.Lines[0][0].Rune)
	}
	if snap.Lines[0][1].Rune != ' ' {
		t.Errorf("insert mode survived reset: [0,1] = %q", snap.Lines[0][1].Rune)
	}
	if snap.Lines[1][0].Rune != ' ' {
		t.Errorf("newline mode survived reset: [1,0] = %q", snap.Lines[1][0].Rune)
	}
	if cell := snap.Lines[0][0]; cell.Style != (Style{}) {
		t.Errorf("style survived reset: %+v", cell.Style)
	}
}
