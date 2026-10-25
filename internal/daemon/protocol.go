package daemon

import (
	"time"

	"ggterm/internal/mux"
	"ggterm/internal/terminal"
)

// ProtocolVersion is bumped whenever the wire format or message semantics
// change in an incompatible way.
const ProtocolVersion = 3

const defaultFrameInterval = time.Second / 30

const (
	actionAttach = "attach"
	actionStatus = "status"
	actionKill   = "kill"

	requestInput     = "input"
	requestResize    = "resize"
	requestCellSize  = "cell_size"
	requestScroll    = "scroll"
	requestSplit     = "split"
	requestClosePane = "close_pane"
	requestNewTab    = "new_tab"
	requestCloseTab  = "close_tab"
	requestNextTab   = "next_tab"
	requestPrevTab   = "prev_tab"
	requestSelectTab = "select_tab"
	requestFocusNext = "focus_next"
	requestFocusDir  = "focus_dir"
	requestFocusPane = "focus_pane"
	requestBroadcast = "broadcast"
	requestDetach    = "detach"
	requestQuit      = "quit"
)

type hello struct {
	Version int
	Action  string
	Cols    int
	Rows    int
}

type request struct {
	Kind   string
	Data   []byte
	Cols   int
	Rows   int
	CellW  int
	CellH  int
	Dir    int
	Index  int
	PaneID int
	Offset int
}

type paneBox struct {
	ID      int
	Rect    mux.Rect
	Content mux.Rect
	Active  bool
	Dir     string
}

type paneUpdate struct {
	ID       int
	Revision uint64
	Snapshot terminal.Snapshot
}

type frame struct {
	Cols      int
	Rows      int
	ActiveTab int
	Tabs      []mux.TabInfo
	Boxes     []paneBox
	Updates   []paneUpdate
	Broadcast bool
	Empty     bool
}

type statusReply struct {
	Version  int
	Session  string
	PID      int
	Cols     int
	Rows     int
	Tabs     int
	Panes    int
	Attached bool
}

// Status is a user-facing description of one running session.
type Status struct {
	Session  string
	PID      int
	Cols     int
	Rows     int
	Tabs     int
	Panes    int
	Attached bool
}
