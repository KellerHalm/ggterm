package terminal

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 30, B: 40, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func kittySequence(control string, payload []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("\x1b_G")
	buf.WriteString(control)
	buf.WriteByte(';')
	buf.WriteString(base64.StdEncoding.EncodeToString(payload))
	buf.WriteString("\x1b\\")
	return buf.Bytes()
}

func Test_Terminal_kitty_transmit_and_display(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5, CellWidth: 2, CellHeight: 2})
	_, _ = term.Write(kittySequence("a=T,f=100,i=7,c=2,r=1", testPNG(t, 4, 2)))

	snap := term.Snapshot(0)
	if len(snap.Images) != 1 {
		t.Fatalf("expected one image, got %d", len(snap.Images))
	}
	placement := snap.Images[0]
	if placement.Graphic.ID != 7 {
		t.Errorf("expected image id 7, got %d", placement.Graphic.ID)
	}
	if placement.Graphic.Width != 4 || placement.Graphic.Height != 2 {
		t.Errorf("unexpected image size %dx%d", placement.Graphic.Width, placement.Graphic.Height)
	}
	if placement.Row != 0 || placement.Col != 0 || placement.Cols != 2 || placement.Rows != 1 {
		t.Errorf("unexpected placement %+v", placement)
	}
	if snap.Cursor.Row != 1 || snap.Cursor.Col != 2 {
		t.Errorf("expected cursor (1,2), got (%d,%d)", snap.Cursor.Row, snap.Cursor.Col)
	}
}

func Test_Terminal_kitty_reassembles_chunks(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	data := testPNG(t, 3, 3)
	encoded := base64.StdEncoding.EncodeToString(data)
	split := len(encoded) / 2

	first := "\x1b_Ga=T,f=100,i=11,c=1,r=1,m=1;" + encoded[:split] + "\x1b\\"
	second := "\x1b_Gm=0;" + encoded[split:] + "\x1b\\"
	_, _ = term.Write([]byte(first))
	if got := len(term.Snapshot(0).Images); got != 0 {
		t.Fatalf("expected no placement before final chunk, got %d", got)
	}
	_, _ = term.Write([]byte(second))

	snap := term.Snapshot(0)
	if len(snap.Images) != 1 {
		t.Fatalf("expected one image, got %d", len(snap.Images))
	}
	if snap.Images[0].Graphic.ID != 11 {
		t.Errorf("expected image id 11, got %d", snap.Images[0].Graphic.ID)
	}
}

func Test_Terminal_kitty_scrolls_with_screen(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 3, CellWidth: 1, CellHeight: 1})
	_, _ = term.Write(kittySequence("a=T,f=100,i=3,c=1,r=1", testPNG(t, 1, 1)))
	if got := len(term.Snapshot(0).Images); got != 1 {
		t.Fatalf("expected image before scroll, got %d", got)
	}

	_, _ = term.Write([]byte("\x1b[3;1H\n"))
	if got := len(term.Snapshot(0).Images); got != 0 {
		t.Fatalf("expected image to scroll out, got %d", got)
	}
	if got := len(term.Snapshot(1).Images); got != 1 {
		t.Fatalf("expected image in scrollback, got %d", got)
	}
}

func Test_Terminal_kitty_alt_screen_clears_placements(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write([]byte("\x1b[?1049h"))
	_, _ = term.Write(kittySequence("a=T,f=100,i=5,c=1,r=1", testPNG(t, 1, 1)))
	if got := len(term.Snapshot(0).Images); got != 1 {
		t.Fatalf("expected image in alt screen, got %d", got)
	}
	_, _ = term.Write([]byte("\x1b[?1049l"))
	if got := len(term.Snapshot(0).Images); got != 0 {
		t.Fatalf("expected alt image to be cleared, got %d", got)
	}
}

func Test_Terminal_ggterm_display(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	payload := base64.StdEncoding.EncodeToString(testPNG(t, 2, 2))
	seq := "\x1b_ggterm;action=display,id=4,cols=2,rows=1;" + payload + "\x1b\\"
	_, _ = term.Write([]byte(seq))

	snap := term.Snapshot(0)
	if len(snap.Images) != 1 {
		t.Fatalf("expected one image, got %d", len(snap.Images))
	}
	placement := snap.Images[0]
	if placement.Graphic.ID != 4 || placement.Cols != 2 || placement.Rows != 1 {
		t.Errorf("unexpected placement %+v", placement)
	}
}

func Test_Terminal_kitty_rejects_invalid_payload(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write([]byte("\x1b_Ga=T,f=100,i=1;not-base64!\x1b\\"))
	if got := len(term.Snapshot(0).Images); got != 0 {
		t.Fatalf("expected no image, got %d", got)
	}
}

func Test_Terminal_kitty_transmit_then_place(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write(kittySequence("a=t,f=100,i=21,I=9,p=2", testPNG(t, 2, 2)))
	if got := len(term.Snapshot(0).Images); got != 0 {
		t.Fatalf("transmit must not display, got %d", got)
	}

	_, _ = term.Write([]byte("\x1b_Ga=p,i=21,p=2,c=2,r=1\x1b\\"))
	snap := term.Snapshot(0)
	if len(snap.Images) != 1 {
		t.Fatalf("expected placed image, got %d", len(snap.Images))
	}
	if snap.Images[0].PlacementID != 2 {
		t.Errorf("expected placement 2, got %d", snap.Images[0].PlacementID)
	}

	_, _ = term.Write([]byte("\x1b_Ga=d,d=i,i=21\x1b\\"))
	if got := len(term.Snapshot(0).Images); got != 0 {
		t.Fatalf("expected deletion to remove placement, got %d", got)
	}
}

func Test_Terminal_kitty_place_by_image_number(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write(kittySequence("a=t,f=100,i=31,I=7", testPNG(t, 2, 2)))
	_, _ = term.Write([]byte("\x1b_Ga=p,I=7,c=1,r=1\x1b\\"))
	snap := term.Snapshot(0)
	if len(snap.Images) != 1 || snap.Images[0].Graphic.ID != 31 {
		t.Fatalf("expected image 31 by number, got %+v", snap.Images)
	}
}

func Test_Terminal_kitty_main_image_survives_alt_screen(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	_, _ = term.Write(kittySequence("a=T,f=100,i=41,c=1,r=1,C=1", testPNG(t, 1, 1)))
	_, _ = term.Write([]byte("\x1b[?1049h"))
	_, _ = term.Write([]byte("\x1b[?1049l"))

	snap := term.Snapshot(0)
	if len(snap.Images) != 1 || snap.Images[0].Graphic.ID != 41 {
		t.Fatalf("expected main image after alt screen, got %+v", snap.Images)
	}
}

func Test_Terminal_ggterm_transmit_then_display(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5})
	payload := base64.StdEncoding.EncodeToString(testPNG(t, 2, 2))
	transmit := "\x1b_ggterm;action=transmit,id=8;" + payload + "\x1b\\"
	display := "\x1b_ggterm;action=display,id=8,cols=1,rows=1\x1b\\"
	_, _ = term.Write([]byte(transmit + display))

	snap := term.Snapshot(0)
	if len(snap.Images) != 1 || snap.Images[0].Graphic.ID != 8 {
		t.Fatalf("expected stored image to be displayed, got %+v", snap.Images)
	}
}

func Test_Terminal_cell_size_rescales_auto_placements(t *testing.T) {
	term := New(Config{Cols: 10, Rows: 5, CellWidth: 8, CellHeight: 16})
	_, _ = term.Write(kittySequence("a=T,f=100,i=51", testPNG(t, 16, 16)))
	if got := term.Snapshot(0).Images[0]; got.Cols != 2 || got.Rows != 1 {
		t.Fatalf("unexpected initial size %dx%d", got.Cols, got.Rows)
	}
	term.SetCellSize(16, 16)
	if got := term.Snapshot(0).Images[0]; got.Cols != 1 || got.Rows != 1 {
		t.Fatalf("unexpected rescaled size %dx%d", got.Cols, got.Rows)
	}
}
