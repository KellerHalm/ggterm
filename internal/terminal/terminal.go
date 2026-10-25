package terminal

import (
	"sync"
	"unicode/utf8"
)

type Terminal struct {
	mu sync.Mutex

	cfg Config

	grid       *grid
	scrollback [][]Cell
	cursor     Cursor
	curStyle   CursorStyle
	style      Style

	title string
	cwd   string

	altGrid   *grid
	altCursor Cursor
	altScreen bool

	autowrap    bool
	pendingWrap bool
	originMode  bool
	insertMode  bool
	newlineMode bool

	scrollTop    int
	scrollBottom int

	savedCursor    Cursor
	savedStyle     Style
	altSavedCursor Cursor
	altSavedStyle  Style

	parser     *parser
	utf8buf    []byte
	utf8expect int
	responses  [][]byte

	revision uint64

	cellW, cellH int
	scrollBase   int

	graphics       map[uint32]*Graphic
	placements     []graphicPlacement
	graphicBytes   int
	imageNumbers   map[uint32]uint32
	nextGraphicID  uint32
	pendingGraphic *kittyPending

	videos map[uint32]*videoStream
}

func New(cfg Config) *Terminal {
	if cfg.Cols <= 0 {
		cfg.Cols = 80
	}
	if cfg.Rows <= 0 {
		cfg.Rows = 24
	}
	if cfg.MaxScrollback < 0 {
		cfg.MaxScrollback = 0
	}
	if cfg.MaxScrollback == 0 {
		cfg.MaxScrollback = 1000
	}
	if cfg.TabWidth <= 0 {
		cfg.TabWidth = 8
	}
	cellW, cellH := cfg.CellWidth, cfg.CellHeight
	if cellW <= 0 {
		cellW = defaultGraphicCellW
	}
	if cellH <= 0 {
		cellH = defaultGraphicCellH
	}
	t := &Terminal{
		cfg:          cfg,
		grid:         newGrid(cfg.Rows, cfg.Cols),
		cursor:       Cursor{Visible: true},
		curStyle:     CursorBlock,
		autowrap:     true,
		scrollBottom: cfg.Rows - 1,
		parser:       newParser(),
		cellW:        cellW,
		cellH:        cellH,
	}
	return t
}

// SetCellSize tells the emulator the current pixel size of one character
// cell. Images that do not specify their size in cells are scaled with it, and
// the cursor movement after a Kitty placement follows the same metrics.
func (t *Terminal) SetCellSize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cellW == width && t.cellH == height {
		return
	}
	t.cellW, t.cellH = width, height
	t.recomputeGraphicCellsLocked()
	t.revision++
}

func (t *Terminal) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(p) > 0 {
		t.revision++
	}
	for _, b := range p {
		t.processByte(b)
	}
	return len(p), nil
}

// Revision increases whenever the terminal content or title may have changed.
// It allows remote frontends to request only the panes that need redrawing.
func (t *Terminal) Revision() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.revision
}

func (t *Terminal) Resize(cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}
	if cols == t.cfg.Cols && rows == t.cfg.Rows {
		return
	}
	t.cfg.Cols = cols
	t.cfg.Rows = rows
	t.revision++
	t.grid.resize(rows, cols)
	if t.altGrid != nil {
		t.altGrid.resize(rows, cols)
	}
	if t.scrollBottom >= rows {
		t.scrollBottom = rows - 1
	}
	if t.scrollTop >= t.scrollBottom {
		t.scrollTop = 0
	}
	if t.cursor.Row >= rows {
		t.cursor.Row = rows - 1
	}
	if t.cursor.Col >= cols {
		t.cursor.Col = cols - 1
	}
	if t.altCursor.Row >= rows {
		t.altCursor.Row = rows - 1
	}
	if t.altCursor.Col >= cols {
		t.altCursor.Col = cols - 1
	}
}

func (t *Terminal) Snapshot(scrollOffset int) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	sbLen := len(t.scrollback)
	if t.altScreen {
		sbLen = 0
	}
	if scrollOffset > sbLen {
		scrollOffset = sbLen
	}
	lines := make([][]Cell, t.cfg.Rows)
	if scrollOffset == 0 {
		cloned := t.grid.clone()
		for i := 0; i < t.cfg.Rows; i++ {
			lines[i] = cloned[i]
		}
	} else {
		sbStart := sbLen - scrollOffset
		for i := 0; i < t.cfg.Rows; i++ {
			sbIdx := sbStart + i
			if sbIdx >= 0 && sbIdx < sbLen {
				lines[i] = make([]Cell, t.cfg.Cols)
				src := t.scrollback[sbIdx]
				copy(lines[i], src)
				if len(src) < t.cfg.Cols {
					for j := len(src); j < t.cfg.Cols; j++ {
						lines[i][j] = Cell{Rune: ' ', Width: 1}
					}
				}
			} else {
				lines[i] = make([]Cell, t.cfg.Cols)
				for j := range lines[i] {
					lines[i][j] = Cell{Rune: ' ', Width: 1}
				}
			}
		}
	}
	cursor := t.cursor
	if scrollOffset > 0 {
		cursor.Visible = false
	}
	return Snapshot{
		Cols:          t.cfg.Cols,
		Rows:          t.cfg.Rows,
		Lines:         lines,
		Cursor:        cursor,
		Title:         t.title,
		Cwd:           t.cwd,
		ScrollbackLen: sbLen,
		ScrollOffset:  scrollOffset,
		AltScreen:     t.altScreen,
		Images:        t.snapshotImagesLocked(scrollOffset),
	}
}

func (t *Terminal) Title() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.title
}

// Cwd returns the working directory most recently reported by the shell via
// OSC 7, or an empty string when the application has not reported one.
func (t *Terminal) Cwd() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cwd
}

// ScrollInfo returns the number of scrollback lines and whether the primary
// screen is hidden by the alternate screen.
func (t *Terminal) ScrollInfo() (scrollbackLen int, altScreen bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.scrollback), t.altScreen
}

func (t *Terminal) TakeResponses() [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.responses) == 0 {
		return nil
	}
	out := t.responses
	t.responses = nil
	return out
}

func (t *Terminal) processByte(b byte) {
	if t.parser.state != stateGround {
		t.processSequenceByte(b)
		return
	}

	if t.utf8expect > 0 {
		if b&0xC0 == 0x80 {
			t.utf8buf = append(t.utf8buf, b)
			t.utf8expect--
			if t.utf8expect == 0 {
				r, _ := utf8.DecodeRune(t.utf8buf)
				if r != utf8.RuneError {
					t.printRune(r)
				}
				t.utf8buf = t.utf8buf[:0]
			}
			return
		}
		t.utf8buf = t.utf8buf[:0]
		t.utf8expect = 0
	}

	if b&0x80 != 0 {
		if b&0xE0 == 0xC0 {
			t.utf8buf = append(t.utf8buf[:0], b)
			t.utf8expect = 1
		} else if b&0xF0 == 0xE0 {
			t.utf8buf = append(t.utf8buf[:0], b)
			t.utf8expect = 2
		} else if b&0xF8 == 0xF0 {
			t.utf8buf = append(t.utf8buf[:0], b)
			t.utf8expect = 3
		}
		return
	}

	t.handleGround(b)
}

func (t *Terminal) processSequenceByte(b byte) {
	switch t.parser.state {
	case stateEscape:
		t.handleEscape(b)
	case stateEscapeCharset:
		t.parser.reset()
	case stateCSI:
		t.handleCSI(b)
	case stateOSC:
		t.handleOSC(b)
	case stateAPC:
		t.handleAPC(b)
	case stateIgnore:
		t.ignoreSequenceByte(b)
	}
}

func (t *Terminal) ignoreSequenceByte(b byte) {
	p := t.parser
	switch {
	case b == 0x18 || b == 0x1A || b == 0x9C:
		p.reset()
	case len(p.oscBuf) > 0 && p.oscBuf[len(p.oscBuf)-1] == 0x1B:
		if b == '\\' {
			p.reset()
			return
		}
		p.oscBuf = p.oscBuf[:0]
		if b == 0x1B {
			p.oscBuf = append(p.oscBuf, b)
		}
	case b == 0x1B:
		p.oscBuf = append(p.oscBuf[:0], b)
	}
}
