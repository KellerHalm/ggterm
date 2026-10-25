package terminal

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

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
	switch cmd {
	case 0, 2:
		t.title = string(data[separator+1:])
	case 7:
		if cwd := parseFileURL(string(data[separator+1:])); cwd != "" {
			t.cwd = cwd
		}
	}
}

// parseFileURL converts the payload of OSC 7 (usually file://host/path) into a
// local absolute path. Shells on Windows often send file:///C:/...
func parseFileURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return ""
	}
	path := parsed.Path
	if path == "" {
		path = parsed.Opaque
	}
	if path == "" {
		return ""
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		path = "//" + parsed.Host + path
	}
	path = filepath.FromSlash(path)
	if runtime.GOOS == "windows" {
		if len(path) >= 3 && (path[0] == '\\' || path[0] == '/') &&
			((path[1] >= 'A' && path[1] <= 'Z') || (path[1] >= 'a' && path[1] <= 'z')) && path[2] == ':' {
			path = path[1:]
		}
	}
	if !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}
