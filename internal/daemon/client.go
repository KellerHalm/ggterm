package daemon

import (
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"ggterm/internal/mux"
	"ggterm/internal/terminal"
)

// ErrUnsupported is returned when the daemon speaks a different protocol.
var ErrUnsupported = errors.New("daemon: unsupported protocol version")

// Attach connects to a session daemon and waits for the first frame.
//
// The returned client owns a background goroutine that keeps the cached view
// up to date. Call Close when the client is no longer needed.
func Attach(socket string, cols, rows int) (*Client, error) {
	conn, err := dialEndpoint(socket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	c := &Client{
		conn:  conn,
		enc:   gob.NewEncoder(conn),
		dec:   gob.NewDecoder(conn),
		panes: make(map[int]*remotePane),
		done:  make(chan struct{}),
		cols:  cols,
		rows:  rows,
	}
	if err := c.write(hello{
		Version: ProtocolVersion,
		Action:  actionAttach,
		Cols:    cols,
		Rows:    rows,
	}); err != nil {
		conn.Close()
		return nil, err
	}

	ready := make(chan struct{})
	var readyOnce sync.Once
	go c.readLoop(ready, &readyOnce)

	select {
	case <-ready:
		return c, nil
	case <-c.done:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = errors.New("daemon closed the connection")
		}
		return nil, err
	case <-time.After(10 * time.Second):
		c.Close()
		return nil, errors.New("daemon did not send an initial frame")
	}
}

// Client is a remote view of a multiplexer owned by a daemon. It implements
// session.Session.
type Client struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder

	writeMu sync.Mutex
	close   sync.Once
	done    chan struct{}

	mu    sync.Mutex
	view  mux.View
	panes map[int]*remotePane
	cols  int
	rows  int
	empty bool
	err   error
}

func (c *Client) readLoop(ready chan struct{}, readyOnce *sync.Once) {
	for {
		var f frame
		if err := c.dec.Decode(&f); err != nil {
			c.fail(err)
			return
		}
		c.apply(f)
		readyOnce.Do(func() { close(ready) })
	}
}

func (c *Client) apply(f frame) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cols, c.rows = f.Cols, f.Rows
	for _, update := range f.Updates {
		pane := c.panes[update.ID]
		if pane == nil {
			pane = &remotePane{client: c, id: update.ID}
			c.panes[update.ID] = pane
		}
		pane.apply(update.Snapshot, update.Revision)
	}
	view := mux.View{
		Cols:      f.Cols,
		Rows:      f.Rows,
		ActiveTab: f.ActiveTab,
		Tabs:      f.Tabs,
		Broadcast: f.Broadcast,
	}
	seen := make(map[int]bool, len(f.Boxes))
	for _, box := range f.Boxes {
		seen[box.ID] = true
		pane := c.panes[box.ID]
		if pane == nil {
			pane = &remotePane{client: c, id: box.ID}
			c.panes[box.ID] = pane
		}
		pane.setDir(box.Dir)
		view.Panes = append(view.Panes, mux.PaneLayout{
			Pane:    pane,
			Rect:    box.Rect,
			Content: box.Content,
			Active:  box.Active,
		})
	}
	for id := range c.panes {
		if !seen[id] {
			delete(c.panes, id)
		}
	}
	c.view = view
	c.empty = f.Empty
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.empty = true
	c.mu.Unlock()
	c.closeConn()
}

func (c *Client) write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.enc.Encode(value)
}

func (c *Client) send(r request) {
	c.mu.Lock()
	closed := c.err != nil
	c.mu.Unlock()
	if closed {
		return
	}
	if err := c.write(r); err != nil {
		c.fail(err)
	}
}

// Close disconnects from the daemon. The daemon keeps the session running.
func (c *Client) Close() error {
	c.fail(net.ErrClosed)
	c.closeConn()
	return nil
}

func (c *Client) closeConn() {
	c.close.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// Cells returns the content size last reported by the UI.
func (c *Client) Cells() (cols, rows int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cols, c.rows
}

// Viewport returns the latest frame and notifies the daemon about size
// changes.
func (c *Client) Viewport(cols, rows int) mux.View {
	c.mu.Lock()
	changed := cols != c.cols || rows != c.rows
	if changed {
		c.cols, c.rows = cols, rows
	}
	view := c.view
	c.mu.Unlock()

	if changed {
		c.send(request{Kind: requestResize, Cols: cols, Rows: rows})
	}
	return view
}

// Empty reports whether the session is gone.
func (c *Client) Empty() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.empty
}

// SetCellSize tells the daemon how large one character cell is in pixels.
func (c *Client) SetCellSize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	c.send(request{Kind: requestCellSize, CellW: width, CellH: height})
}

// WriteInput sends keyboard input to the focused pane.
func (c *Client) WriteInput(data []byte) {
	if len(data) == 0 {
		return
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	c.send(request{Kind: requestInput, Data: buf})
}

func (c *Client) Split(dir mux.Direction) error {
	c.send(request{Kind: requestSplit, Dir: int(dir)})
	return nil
}

func (c *Client) CloseActivePane() {
	c.send(request{Kind: requestClosePane})
}

func (c *Client) NewTab() error {
	c.send(request{Kind: requestNewTab})
	return nil
}

func (c *Client) CloseTab() {
	c.send(request{Kind: requestCloseTab})
}

func (c *Client) NextTab() {
	c.send(request{Kind: requestNextTab})
}

func (c *Client) PrevTab() {
	c.send(request{Kind: requestPrevTab})
}

func (c *Client) SelectTab(index int) {
	c.send(request{Kind: requestSelectTab, Index: index})
}

func (c *Client) FocusNext() {
	c.send(request{Kind: requestFocusNext})
}

func (c *Client) FocusDirection(dir mux.MoveDirection) {
	c.send(request{Kind: requestFocusDir, Dir: int(dir)})
}

func (c *Client) FocusPane(id int) {
	c.send(request{Kind: requestFocusPane, PaneID: id})
}

func (c *Client) ToggleBroadcast() bool {
	c.mu.Lock()
	c.view.Broadcast = !c.view.Broadcast
	enabled := c.view.Broadcast
	c.mu.Unlock()
	c.send(request{Kind: requestBroadcast})
	return enabled
}

// Detach leaves the daemon running and reports success. It waits briefly for
// the daemon to acknowledge the request by closing the connection so that a
// following status query observes the detached state.
func (c *Client) Detach() bool {
	c.mu.Lock()
	closed := c.err != nil
	c.mu.Unlock()
	if closed {
		return false
	}
	c.send(request{Kind: requestDetach})
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
	return true
}

// Quit terminates the session and all panes. It waits briefly for the daemon
// to close the connection so the request cannot be lost on process exit.
func (c *Client) Quit() {
	c.send(request{Kind: requestQuit})
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
	}
}

type remotePane struct {
	client *Client
	id     int

	mu     sync.Mutex
	snap   terminal.Snapshot
	rev    uint64
	offset int
	dir    string
}

func (p *remotePane) apply(snap terminal.Snapshot, rev uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snap = snap
	p.rev = rev
	p.offset = snap.ScrollOffset
}

func (p *remotePane) ID() int { return p.id }

func (p *remotePane) Snapshot() terminal.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap
}

func (p *remotePane) Scroll(delta int) {
	p.mu.Lock()
	if p.snap.AltScreen {
		p.mu.Unlock()
		return
	}
	offset := p.offset + delta
	if offset < 0 {
		offset = 0
	}
	if offset > p.snap.ScrollbackLen {
		offset = p.snap.ScrollbackLen
	}
	if offset == p.offset {
		p.mu.Unlock()
		return
	}
	p.offset = offset
	p.mu.Unlock()
	p.client.send(request{Kind: requestScroll, PaneID: p.id, Offset: offset})
}

func (p *remotePane) ScrollTo(offset int) {
	p.mu.Lock()
	if offset < 0 {
		offset = 0
	}
	if offset > p.snap.ScrollbackLen {
		offset = p.snap.ScrollbackLen
	}
	p.offset = offset
	p.mu.Unlock()
}

func (p *remotePane) ResetScroll() {
	p.mu.Lock()
	changed := p.offset != 0
	p.offset = 0
	p.mu.Unlock()
	if changed {
		p.client.send(request{Kind: requestScroll, PaneID: p.id, Offset: 0})
	}
}

func (p *remotePane) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snap.Title
}

func (p *remotePane) setDir(dir string) {
	p.mu.Lock()
	p.dir = dir
	p.mu.Unlock()
}

func (p *remotePane) Dir() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dir != "" {
		return p.dir
	}
	return p.snap.Cwd
}

func (p *remotePane) Alive() bool { return true }

func (p *remotePane) Revision() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rev
}

func (p *remotePane) ScrollOffset() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.offset
}

func (p *remotePane) Resize(cols, rows int) error { return nil }

// FetchStatus queries one running session without attaching to it.
func FetchStatus(socket string) (Status, error) {
	conn, err := dialEndpoint(socket, 2*time.Second)
	if err != nil {
		return Status{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)
	if err := enc.Encode(hello{Version: ProtocolVersion, Action: actionStatus}); err != nil {
		return Status{}, err
	}
	var reply statusReply
	if err := dec.Decode(&reply); err != nil {
		return Status{}, err
	}
	if reply.Version != ProtocolVersion {
		return Status{}, ErrUnsupported
	}
	return Status{
		Session:  reply.Session,
		PID:      reply.PID,
		Cols:     reply.Cols,
		Rows:     reply.Rows,
		Tabs:     reply.Tabs,
		Panes:    reply.Panes,
		Attached: reply.Attached,
	}, nil
}

// ListStatuses returns all sessions whose daemon responds.
func ListStatuses() ([]Status, error) {
	names, err := ListSessions()
	if err != nil {
		return nil, err
	}
	var statuses []Status
	for _, name := range names {
		socket, err := SocketPath(name)
		if err != nil {
			continue
		}
		status, err := FetchStatus(socket)
		if err != nil {
			continue
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// Kill terminates a running session. It also understands daemons started by
// older protocol versions, which is useful after an upgrade: such a daemon
// rejects the current version and keeps the session socket occupied.
func Kill(socket string) error {
	if !SocketAlive(socket) {
		return fmt.Errorf("session is not running")
	}
	for version := ProtocolVersion; version >= 1; version-- {
		conn, err := dialEndpoint(socket, time.Second)
		if err != nil {
			break
		}
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		enc := gob.NewEncoder(conn)
		if err := enc.Encode(hello{Version: version, Action: actionKill}); err != nil {
			_ = conn.Close()
			continue
		}
		var discard [1]byte
		_, _ = conn.Read(discard[:])
		_ = conn.Close()
		if WaitForSocketClosed(socket, time.Second) == nil {
			return nil
		}
	}
	if !SocketAlive(socket) {
		return nil
	}
	return fmt.Errorf("session did not terminate")
}
