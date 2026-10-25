// Package session defines the interface the UI uses to drive a multiplexer.
//
// The same interface is implemented by an in-process mux (Local) and by a
// remote client attached to a background session daemon.
package session

import "ggterm/internal/mux"

// Session is a controllable multiplexer.
type Session interface {
	Cells() (cols, rows int)
	Viewport(cols, rows int) mux.View
	Empty() bool

	WriteInput(data []byte)

	Split(dir mux.Direction) error
	CloseActivePane()
	NewTab() error
	CloseTab()
	NextTab()
	PrevTab()
	SelectTab(index int)
	FocusNext()
	FocusDirection(dir mux.MoveDirection)
	FocusPane(id int)
	ToggleBroadcast() bool

	// Detach closes the UI but leaves a background daemon running. It reports
	// whether detaching is supported by this session.
	Detach() bool

	// Quit terminates the session and all of its panes.
	Quit()
}

// Local adapts an in-process multiplexer to Session. It does not support
// detaching because the processes die with the process.
type Local struct {
	*mux.Mux
}

// NewLocal wraps a multiplexer.
func NewLocal(m *mux.Mux) Local {
	return Local{Mux: m}
}

func (l Local) FocusPane(id int) { l.Mux.FocusPaneID(id) }

func (l Local) Detach() bool { return false }

func (l Local) Quit() {}
