package terminal

import "github.com/mattn/go-runewidth"

func runeWidth(r rune) int {
	width := runewidth.RuneWidth(r)
	if width < 0 {
		return 0
	}
	return width
}

func (t *Terminal) handleGround(b byte) {
	switch b {
	case 0x07:
	case 0x08:
		t.backspace()
	case 0x09:
		t.tab()
	case 0x0A, 0x0B, 0x0C:
		t.linefeed()
	case 0x0D:
		t.carriageReturn()
	case 0x1B:
		t.parser.state = stateEscape
	default:
		if b >= 0x20 && b < 0x7F {
			t.printRune(rune(b))
		}
	}
}

func (t *Terminal) handleEscape(b byte) {
	switch b {
	case '[':
		t.parser.state = stateCSI
		t.parser.params = t.parser.params[:0]
		t.parser.inter = t.parser.inter[:0]
	case ']':
		t.parser.state = stateOSC
		t.parser.oscBuf = t.parser.oscBuf[:0]
	case '7':
		t.saveCursor()
		t.parser.reset()
	case '8':
		t.restoreCursor()
		t.parser.reset()
	case 'D':
		t.index()
		t.parser.reset()
	case 'M':
		t.reverseIndex()
		t.parser.reset()
	case 'E':
		t.nextLine()
		t.parser.reset()
	case 'c':
		t.reset()
		t.parser.reset()
	case '(', ')', '*', '+', '#', '%', ' ', 'N', 'O':
		t.parser.state = stateEscapeCharset
	case '_':
		t.parser.state = stateAPC
		t.parser.apcBuf = t.parser.apcBuf[:0]
		t.parser.apcEsc = false
		t.parser.apcOverflow = false
	case 'P', 'X', '^':
		t.parser.state = stateIgnore
		t.parser.oscBuf = t.parser.oscBuf[:0]
	default:
		t.parser.reset()
	}
}

func (t *Terminal) handleCSI(b byte) {
	if b >= '0' && b <= '9' {
		t.parser.oscBuf = append(t.parser.oscBuf, b)
		return
	}
	if b == ';' {
		t.parser.oscBuf = append(t.parser.oscBuf, b)
		return
	}
	if b == '?' || b == '>' || b == '!' || b == '$' || b == '"' || b == ' ' || b == '\'' {
		t.parser.inter = append(t.parser.inter, b)
		return
	}
	if b >= 0x40 && b <= 0x7E {
		t.executeCSI(b, t.parser.parseParams(t.parser.oscBuf))
		t.parser.reset()
		return
	}
	t.parser.reset()
}

func (t *Terminal) handleOSC(b byte) {
	if b == 0x07 || b == 0x9C {
		t.executeOSC(t.parser.oscBuf)
		t.parser.reset()
		return
	}
	if b == 0x1B {
		t.parser.oscBuf = append(t.parser.oscBuf, b)
		return
	}
	if len(t.parser.oscBuf) > 0 && t.parser.oscBuf[len(t.parser.oscBuf)-1] == 0x1B && b == '\\' {
		t.executeOSC(t.parser.oscBuf[:len(t.parser.oscBuf)-1])
		t.parser.reset()
		return
	}
	t.parser.oscBuf = append(t.parser.oscBuf, b)
	if len(t.parser.oscBuf) > 4096 {
		t.parser.reset()
	}
}

func (t *Terminal) handleAPC(b byte) {
	p := t.parser
	if p.apcEsc {
		if b == '\\' {
			data := append([]byte(nil), p.apcBuf...)
			overflow := p.apcOverflow
			p.reset()
			if !overflow {
				t.executeAPC(data)
			}
			return
		}
		if b == 0x18 || b == 0x1A {
			p.reset()
			return
		}
		p.apcEsc = false
		if !p.apcOverflow {
			p.apcBuf = append(p.apcBuf, 0x1B)
		}
	}
	switch {
	case b == 0x1B:
		p.apcEsc = true
		return
	case b == 0x18 || b == 0x1A || b == 0x9C:
		p.reset()
		return
	case p.apcOverflow:
		return
	case len(p.apcBuf) >= maxGraphicPayload:
		p.apcOverflow = true
		p.apcBuf = p.apcBuf[:0]
		return
	}
	p.apcBuf = append(p.apcBuf, b)
}

func (t *Terminal) printRune(r rune) {
	width := runeWidth(r)
	if width == 0 {
		return
	}
	if t.pendingWrap {
		t.wrapLine()
	}
	if width == 2 && t.cursor.Col+1 >= t.cfg.Cols {
		if t.autowrap {
			t.wrapLine()
		} else {
			t.grid.setCell(t.cursor.Row, t.cursor.Col, ' ', t.style)
			return
		}
	}
	if t.insertMode {
		t.grid.insertCells(t.cursor.Row, t.cursor.Col, width)
	}
	t.grid.setCell(t.cursor.Row, t.cursor.Col, r, t.style)
	t.cursor.Col += width
	if t.cursor.Col >= t.cfg.Cols {
		if t.autowrap {
			t.pendingWrap = true
		}
		t.cursor.Col = t.cfg.Cols - 1
	}
}

func (t *Terminal) wrapLine() {
	t.cursor.Col = 0
	t.pendingWrap = false
	t.index()
}

func (t *Terminal) backspace() {
	t.pendingWrap = false
	if t.cursor.Col > 0 {
		t.cursor.Col--
	}
}

func (t *Terminal) tab() {
	t.pendingWrap = false
	next := ((t.cursor.Col / t.cfg.TabWidth) + 1) * t.cfg.TabWidth
	if next >= t.cfg.Cols {
		t.cursor.Col = t.cfg.Cols - 1
	} else {
		t.cursor.Col = next
	}
}

func (t *Terminal) linefeed() {
	t.pendingWrap = false
	if t.newlineMode {
		t.cursor.Col = 0
	}
	t.index()
}

func (t *Terminal) carriageReturn() {
	t.pendingWrap = false
	t.cursor.Col = 0
}

func (t *Terminal) index() {
	if t.cursor.Row == t.scrollBottom {
		t.scrollRegionUp(1)
		return
	}
	if t.cursor.Row < t.cfg.Rows-1 {
		t.cursor.Row++
	}
}

func (t *Terminal) reverseIndex() {
	t.pendingWrap = false
	if t.cursor.Row == t.scrollTop {
		t.scrollRegionDown(1)
		return
	}
	if t.cursor.Row > 0 {
		t.cursor.Row--
	}
}

func (t *Terminal) nextLine() {
	t.linefeed()
	t.carriageReturn()
}

func (t *Terminal) scrollRegionUp(n int) {
	if n <= 0 {
		return
	}
	if n > t.scrollBottom-t.scrollTop+1 {
		n = t.scrollBottom - t.scrollTop + 1
	}
	if t.scrollTop == 0 && !t.altScreen {
		t.scrollBase += n
	}
	t.graphicsScrollUpLocked(t.scrollTop, t.scrollBottom, n)
	for i := 0; i < n; i++ {
		if t.scrollTop == 0 && !t.altScreen && t.cfg.MaxScrollback > 0 {
			line := make([]Cell, t.cfg.Cols)
			copy(line, t.grid.cells[0])
			t.scrollback = append(t.scrollback, line)
			if len(t.scrollback) > t.cfg.MaxScrollback {
				t.scrollback = t.scrollback[len(t.scrollback)-t.cfg.MaxScrollback:]
			}
		}
		t.grid.scrollUp(t.scrollTop, t.scrollBottom, 1)
	}
	t.pruneScrollbackGraphicsLocked()
}

func (t *Terminal) scrollRegionDown(n int) {
	if n <= 0 {
		return
	}
	if n > t.scrollBottom-t.scrollTop+1 {
		n = t.scrollBottom - t.scrollTop + 1
	}
	t.graphicsScrollDownLocked(t.scrollTop, t.scrollBottom, n)
	t.grid.scrollDown(t.scrollTop, t.scrollBottom, n)
}

func (t *Terminal) restoreCursor() {
	if t.altScreen {
		t.cursor = t.altSavedCursor
		t.style = t.altSavedStyle
	} else {
		t.cursor = t.savedCursor
		t.style = t.savedStyle
	}
	t.pendingWrap = false
}

func (t *Terminal) reset() {
	if t.altScreen {
		t.grid, t.altGrid = t.altGrid, t.grid
		t.altScreen = false
	}
	t.grid.clear()
	t.scrollback = t.scrollback[:0]
	t.responses = nil
	t.cursor = Cursor{Visible: true}
	t.style = Style{}
	t.title = ""
	t.autowrap = true
	t.pendingWrap = false
	t.originMode = false
	t.insertMode = false
	t.newlineMode = false
	t.curStyle = CursorBlock
	t.scrollTop = 0
	t.scrollBottom = t.cfg.Rows - 1
	t.altGrid = nil
	t.clearGraphicsLocked()
}
