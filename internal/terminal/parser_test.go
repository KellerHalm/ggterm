package terminal

import "testing"

func Test_Terminal_OSC_title_with_UTF8(t *testing.T) {
	term := New(Config{})
	mustWrite(t, term, "\x1b]2;Термин\x07")

	if got := term.Title(); got != "Термин" {
		t.Errorf("title = %q, want Термин", got)
	}
}

func Test_Terminal_ignores_DCS_APC_PM_SOS_until_ST(t *testing.T) {
	for _, introducer := range []string{"\x1bP", "\x1b_", "\x1b^", "\x1bX"} {
		t.Run(introducer, func(t *testing.T) {
			term := New(Config{Cols: 10, Rows: 2})
			mustWrite(t, term, introducer+"hidden\x1b\\OK")

			snap := term.Snapshot(0)
			if snap.Lines[0][0].Rune != 'O' || snap.Lines[0][1].Rune != 'K' {
				t.Errorf("text after ST = %q%q, want OK", snap.Lines[0][0].Rune, snap.Lines[0][1].Rune)
			}
		})
	}
}

func Test_Terminal_consumes_character_set_designation(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 2})
	mustWrite(t, term, "\x1b(BA\x1b)0q")

	snap := term.Snapshot(0)
	if snap.Lines[0][0].Rune != 'A' || snap.Lines[0][1].Rune != 'q' {
		t.Errorf("text = %q%q, want Aq", snap.Lines[0][0].Rune, snap.Lines[0][1].Rune)
	}
}

func Test_Terminal_SGR_underline_color_does_not_change_attributes(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 2})
	mustWrite(t, term, "\x1b[58;2;1;2;3m\x1b[31;1mA")

	cell := term.Snapshot(0).Lines[0][0]
	if cell.Style.FG != IndexedColor(1) || !cell.Style.Bold {
		t.Errorf("style = %+v, want red bold", cell.Style)
	}
	if cell.Style.Underline || cell.Style.Blink || cell.Style.Faint {
		t.Errorf("underline color leaked into attributes: %+v", cell.Style)
	}
}

func Test_Terminal_malformed_extended_SGR_is_ignored(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 2})
	mustWrite(t, term, "\x1b[38;5mA")

	cell := term.Snapshot(0).Lines[0][0]
	if cell.Style != (Style{}) {
		t.Errorf("style = %+v, want zero", cell.Style)
	}
}

func Test_Terminal_large_scroll_parameter_is_clamped(t *testing.T) {
	term := New(Config{Cols: 3, Rows: 3})
	mustWrite(t, term, "A\r\nB\r\nC\x1b[999999S")

	snap := term.Snapshot(0)
	for row := range snap.Lines {
		if snap.Lines[row][0].Rune != ' ' {
			t.Fatalf("row %d not blank after huge scroll: %q", row, snap.Lines[row][0].Rune)
		}
	}
}

func Test_Terminal_never_panics_on_malformed_input(t *testing.T) {
	inputs := []string{
		"\x1b[",
		"\x1b[31;",
		"\x1b]0;unterminated",
		"\x1bPunterminated",
		"\x1b[999999999999999999999D",
		"\xff\xfe\x80",
		"\x1b[38;2;300;400m",
		"\x1b[?99999h",
	}
	for _, input := range inputs {
		term := New(Config{Cols: 8, Rows: 3})
		mustWrite(t, term, input)
		mustWrite(t, term, "OK")
		if snap := term.Snapshot(0); snap.Cols != 8 || snap.Rows != 3 {
			t.Errorf("input %q changed dimensions to %dx%d", input, snap.Cols, snap.Rows)
		}
	}
}

func Test_Terminal_device_status_report(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	mustWrite(t, term, "AB\x1b[6n\x1b[?6n\x1b[5n")

	responses := term.TakeResponses()
	want := []string{"\x1b[1;3R", "\x1b[?1;3R", "\x1b[0n"}
	if len(responses) != len(want) {
		t.Fatalf("responses = %d, want %d", len(responses), len(want))
	}
	for i := range want {
		if string(responses[i]) != want[i] {
			t.Errorf("response %d = %q, want %q", i, responses[i], want[i])
		}
	}
	if again := term.TakeResponses(); again != nil {
		t.Errorf("second TakeResponses = %q, want nil", again)
	}
}

func Test_Terminal_device_attributes(t *testing.T) {
	term := New(Config{})
	mustWrite(t, term, "\x1b[c\x1b[>c")

	responses := term.TakeResponses()
	if len(responses) != 1 || string(responses[0]) != "\x1b[?6c" {
		t.Errorf("responses = %q, want one VT102 DA response", responses)
	}
}

func Test_Terminal_response_queue_is_bounded(t *testing.T) {
	term := New(Config{})
	for i := 0; i < maxPendingResponses+10; i++ {
		mustWrite(t, term, "\x1b[5n")
	}
	if got := len(term.TakeResponses()); got != maxPendingResponses {
		t.Errorf("queued responses = %d, want %d", got, maxPendingResponses)
	}
}
