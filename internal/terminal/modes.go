package terminal

func (t *Terminal) setMode(params []int, enable bool) {
	private := t.parser.hasPrivate()
	for _, p := range params {
		if !private {
			switch p {
			case 4:
				t.insertMode = enable
			case 20:
				t.newlineMode = enable
			}
			continue
		}
		switch p {
		case 6:
			t.originMode = enable
			t.cursor.Row = t.rowMin()
			t.cursor.Col = 0
		case 7:
			t.autowrap = enable
		case 25:
			t.cursor.Visible = enable
		case 47, 1047, 1049:
			t.switchAltScreen(enable)
		case 2004:
		}
	}
}

func (t *Terminal) switchAltScreen(enable bool) {
	if enable == t.altScreen {
		return
	}
	t.clearAltGraphicsLocked()
	if enable {
		if t.altGrid == nil {
			t.altGrid = newGrid(t.cfg.Rows, t.cfg.Cols)
		}
		t.altGrid.clear()
		t.grid, t.altGrid = t.altGrid, t.grid
		t.cursor, t.altCursor = t.altCursor, t.cursor
		t.cursor = Cursor{Row: 0, Col: 0, Visible: t.cursor.Visible}
		t.altScreen = true
		return
	}
	t.grid, t.altGrid = t.altGrid, t.grid
	t.cursor, t.altCursor = t.altCursor, t.cursor
	t.altScreen = false
}

func (t *Terminal) executeOSC(data []byte) {
	if len(data) == 0 {
		return
	}
	cmd := 0
	separator := -1
	for i, b := range data {
		if b == ';' {
			separator = i
			break
		}
		if b >= '0' && b <= '9' {
			cmd = cmd*10 + int(b-'0')
		}
	}
	if separator < 0 {
		return
	}
	if cmd == 0 || cmd == 2 {
		t.title = string(data[separator+1:])
	}
}
