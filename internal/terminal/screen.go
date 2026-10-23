package terminal

func (t *Terminal) saveCursor() {
	if t.altScreen {
		t.altSavedCursor = t.cursor
		t.altSavedStyle = t.style
	} else {
		t.savedCursor = t.cursor
		t.savedStyle = t.style
	}
}

func (t *Terminal) softReset() {
	t.style = Style{}
	t.autowrap = true
	t.pendingWrap = false
	t.insertMode = false
	t.newlineMode = false
	t.originMode = false
	t.cursor.Visible = true
	t.curStyle = CursorBlock
	t.scrollTop = 0
	t.scrollBottom = t.cfg.Rows - 1
}
