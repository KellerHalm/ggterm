package terminal

func (t *Terminal) executeCSI(cmd byte, params []int) {
	t.pendingWrap = false
	switch cmd {
	case 'A':
		t.moveCursor(-t.parser.param(0, 1), 0)
	case 'B':
		t.moveCursor(t.parser.param(0, 1), 0)
	case 'C':
		t.moveCursor(0, t.parser.param(0, 1))
	case 'D':
		t.moveCursor(0, -t.parser.param(0, 1))
	case 'E':
		t.cursor.Col = 0
		t.moveCursor(t.parser.param(0, 1), 0)
	case 'F':
		t.cursor.Col = 0
		t.moveCursor(-t.parser.param(0, 1), 0)
	case 'G', '`':
		t.cursor.Col = clamp(t.parser.param(0, 1)-1, 0, t.cfg.Cols-1)
	case 'd':
		row := t.parser.param(0, 1) - 1
		if t.originMode {
			row += t.scrollTop
		}
		t.cursor.Row = clamp(row, t.rowMin(), t.rowMax())
	case 'H', 'f':
		t.cursorPosition(params)
	case 'J':
		t.eraseInDisplay(t.parser.param(0, 0))
	case 'K':
		t.eraseInLine(t.parser.param(0, 0))
	case 'L':
		if t.inScrollRegion() {
			count := t.parser.param(0, 1)
			t.graphicsScrollDownLocked(t.cursor.Row, t.scrollBottom, count)
			t.grid.insertLines(t.cursor.Row, t.scrollBottom, count)
		}
	case 'M':
		if t.inScrollRegion() {
			count := t.parser.param(0, 1)
			t.graphicsDeleteLinesLocked(t.cursor.Row, t.scrollBottom, count)
			t.grid.deleteLines(t.cursor.Row, t.scrollBottom, count)
		}
	case 'P':
		t.grid.deleteChars(t.cursor.Row, t.cursor.Col, t.parser.param(0, 1))
	case '@':
		t.grid.insertChars(t.cursor.Row, t.cursor.Col, t.parser.param(0, 1))
	case 'X':
		t.grid.eraseChars(t.cursor.Row, t.cursor.Col, t.parser.param(0, 1))
	case 'S':
		t.scrollRegionUp(t.parser.param(0, 1))
	case 'T':
		t.scrollRegionDown(t.parser.param(0, 1))
	case 'r':
		t.setScrollRegion(params)
	case 'n':
		t.deviceStatusReport(t.parser.param(0, 0))
	case 'c':
		t.deviceAttributes()
	case 'p':
		if len(t.parser.inter) > 0 && t.parser.inter[0] == '!' {
			t.softReset()
		}
	case 'm':
		t.setSGR(params)
	case 'h':
		t.setMode(params, true)
	case 'l':
		t.setMode(params, false)
	}
}

func (t *Terminal) rowMin() int {
	if t.originMode {
		return t.scrollTop
	}
	return 0
}

func (t *Terminal) rowMax() int {
	if t.originMode {
		return t.scrollBottom
	}
	return t.cfg.Rows - 1
}

func (t *Terminal) moveCursor(dRow, dCol int) {
	t.cursor.Row = clamp(t.cursor.Row+dRow, t.rowMin(), t.rowMax())
	t.cursor.Col = clamp(t.cursor.Col+dCol, 0, t.cfg.Cols-1)
}

func (t *Terminal) cursorPosition(params []int) {
	row := t.parser.param(0, 1) - 1
	col := t.parser.param(1, 1) - 1
	if t.originMode {
		row += t.scrollTop
	}
	t.cursor.Row = clamp(row, t.rowMin(), t.rowMax())
	t.cursor.Col = clamp(col, 0, t.cfg.Cols-1)
}

func (t *Terminal) setScrollRegion(params []int) {
	top := t.parser.param(0, 1) - 1
	bottom := t.parser.param(1, t.cfg.Rows) - 1
	if top < 0 || bottom >= t.cfg.Rows || top >= bottom {
		return
	}
	t.scrollTop = top
	t.scrollBottom = bottom
	t.cursor.Row = t.rowMin()
	t.cursor.Col = 0
}

func (t *Terminal) inScrollRegion() bool {
	return t.cursor.Row >= t.scrollTop && t.cursor.Row <= t.scrollBottom
}

func (t *Terminal) eraseInDisplay(mode int) {
	switch mode {
	case 0:
		t.grid.clearRegion(t.cursor.Row, t.cursor.Col, t.cfg.Rows-1, t.cfg.Cols-1)
	case 1:
		t.grid.clearRegion(0, 0, t.cursor.Row, t.cursor.Col)
	case 2:
		t.grid.clear()
	case 3:
		if !t.altScreen {
			t.scrollback = t.scrollback[:0]
			t.pruneScrollbackGraphicsLocked()
		}
	}
}

func (t *Terminal) eraseInLine(mode int) {
	switch mode {
	case 0:
		t.grid.clearRegion(t.cursor.Row, t.cursor.Col, t.cursor.Row, t.cfg.Cols-1)
	case 1:
		t.grid.clearRegion(t.cursor.Row, 0, t.cursor.Row, t.cursor.Col)
	case 2:
		t.grid.clearRegion(t.cursor.Row, 0, t.cursor.Row, t.cfg.Cols-1)
	}
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
