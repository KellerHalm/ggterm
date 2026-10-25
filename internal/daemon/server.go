package daemon

import (
	"encoding/gob"
	"errors"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"ggterm/internal/mux"
)

// ErrAlreadyRunning is returned when a daemon is already listening on the
// requested socket.
var ErrAlreadyRunning = errors.New("daemon: session already running")

const (
	defaultSaveInterval  = 2 * time.Second
	defaultPruneInterval = 500 * time.Millisecond
)

// Options configures a session daemon.
type Options struct {
	Session       string
	Mux           *mux.Mux
	LayoutPath    string
	FrameInterval time.Duration
	SaveInterval  time.Duration
	Logger        *log.Logger
}

// Server owns a multiplexer and serves it to one attached client at a time.
type Server struct {
	opts Options

	mu     sync.Mutex
	ln     net.Listener
	client *clientConn

	stopOnce sync.Once
	stopped  chan struct{}
	logger   *log.Logger
}

// New creates a server for an already constructed multiplexer.
func New(opts Options) (*Server, error) {
	if opts.Mux == nil {
		return nil, errors.New("daemon: nil multiplexer")
	}
	if opts.FrameInterval <= 0 {
		opts.FrameInterval = defaultFrameInterval
	}
	if opts.SaveInterval <= 0 {
		opts.SaveInterval = defaultSaveInterval
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "ggterm daemon: ", log.LstdFlags)
	}
	return &Server{
		opts:    opts,
		stopped: make(chan struct{}),
		logger:  logger,
	}, nil
}

// Listen creates the session endpoint, replacing a stale unix socket. It
// fails with ErrAlreadyRunning when another daemon is alive.
func Listen(socket string) (net.Listener, error) {
	return listenEndpoint(socket)
}

// ListenAndServe listens on socket and serves until the session ends.
func (s *Server) ListenAndServe(socket string) error {
	ln, err := Listen(socket)
	if err != nil {
		return err
	}
	defer removeEndpoint(socket)
	registerSession(s.opts.Session)
	defer unregisterSession(s.opts.Session)
	// Persist the restored state right away so a crash keeps the layout.
	if s.opts.LayoutPath != "" {
		if err := mux.Save(s.opts.LayoutPath, s.opts.Mux); err != nil {
			s.logger.Printf("save layout: %v", err)
		}
	}
	return s.Serve(ln)
}

// Serve accepts clients on ln until the server is stopped.
func (s *Server) Serve(ln net.Listener) error {
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()

	go s.watchEmpty()
	go s.saveLoop()
	go s.pruneLoop()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopped:
				return nil
			default:
				return err
			}
		}
		go s.handle(conn)
	}
}

// Stop terminates the session, persists its layout and shuts down all panes.
func (s *Server) Stop() {
	s.stop(false)
}

func (s *Server) stop(discardLayout bool) {
	s.stopOnce.Do(func() {
		close(s.stopped)

		s.mu.Lock()
		ln := s.ln
		client := s.client
		s.client = nil
		s.mu.Unlock()

		if ln != nil {
			_ = ln.Close()
		}
		if client != nil {
			s.sendFrame(client)
			client.close()
		}

		if s.opts.LayoutPath != "" {
			var err error
			if discardLayout {
				err = mux.Remove(s.opts.LayoutPath)
			} else {
				err = mux.Save(s.opts.LayoutPath, s.opts.Mux)
			}
			if err != nil {
				s.logger.Printf("save layout: %v", err)
			}
		}
		s.opts.Mux.Shutdown()
	})
}

func (s *Server) isStopped() bool {
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}

func (s *Server) watchEmpty() {
	select {
	case <-s.opts.Mux.EmptyChan():
		s.mu.Lock()
		client := s.client
		s.mu.Unlock()
		if client != nil {
			s.sendFrame(client)
		}
		time.Sleep(100 * time.Millisecond)
		s.stop(false)
	case <-s.stopped:
	}
}

func (s *Server) saveLoop() {
	ticker := time.NewTicker(s.opts.SaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			if s.opts.LayoutPath == "" {
				continue
			}
			if err := mux.Save(s.opts.LayoutPath, s.opts.Mux); err != nil {
				s.logger.Printf("save layout: %v", err)
			}
		}
	}
}

func (s *Server) pruneLoop() {
	ticker := time.NewTicker(defaultPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			s.opts.Mux.Prune()
		}
	}
}

func (s *Server) handle(conn net.Conn) {
	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	var greeting hello
	if err := dec.Decode(&greeting); err != nil {
		_ = conn.Close()
		return
	}
	if greeting.Version != ProtocolVersion {
		_ = enc.Encode(statusReply{Version: ProtocolVersion})
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})

	switch greeting.Action {
	case actionStatus:
		_ = enc.Encode(s.status())
		_ = conn.Close()
		return
	case actionKill:
		s.stop(true)
		_ = conn.Close()
		return
	case actionAttach:
	default:
		_ = conn.Close()
		return
	}

	client := newClientConn(s, conn, enc, greeting.Cols, greeting.Rows)
	s.mu.Lock()
	previous := s.client
	s.client = client
	s.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	client.start()
	go s.frameLoop(client)
	s.readLoop(client, dec)
}

func (s *Server) status() statusReply {
	tabs, panes := s.opts.Mux.Stats()
	cols, rows := s.opts.Mux.Cells()
	s.mu.Lock()
	attached := s.client != nil
	s.mu.Unlock()
	return statusReply{
		Version:  ProtocolVersion,
		Session:  s.opts.Session,
		PID:      os.Getpid(),
		Cols:     cols,
		Rows:     rows,
		Tabs:     tabs,
		Panes:    panes,
		Attached: attached,
	}
}

func (s *Server) readLoop(client *clientConn, dec *gob.Decoder) {
	for {
		var req request
		if err := dec.Decode(&req); err != nil {
			client.close()
			return
		}
		if s.dispatch(client, req) {
			return
		}
	}
}

// dispatch applies one client request. It reports whether the connection
// should stop reading.
func (s *Server) dispatch(client *clientConn, req request) bool {
	m := s.opts.Mux
	switch req.Kind {
	case requestInput:
		m.WriteInput(req.Data)
	case requestResize:
		client.setSize(req.Cols, req.Rows)
	case requestCellSize:
		m.SetCellSize(req.CellW, req.CellH)
	case requestScroll:
		client.setOffset(req.PaneID, req.Offset)
	case requestSplit:
		_ = m.Split(mux.Direction(req.Dir))
	case requestClosePane:
		m.CloseActivePane()
	case requestNewTab:
		_ = m.NewTab()
	case requestCloseTab:
		m.CloseTab()
	case requestNextTab:
		m.NextTab()
	case requestPrevTab:
		m.PrevTab()
	case requestSelectTab:
		m.SelectTab(req.Index)
	case requestFocusNext:
		m.FocusNext()
	case requestFocusDir:
		m.FocusDirection(mux.MoveDirection(req.Dir))
	case requestFocusPane:
		m.FocusPaneID(req.PaneID)
	case requestBroadcast:
		m.ToggleBroadcast()
	case requestDetach:
		client.close()
		return true
	case requestQuit:
		s.stop(true)
		return true
	}
	return false
}

func (s *Server) frameLoop(client *clientConn) {
	ticker := time.NewTicker(s.opts.FrameInterval)
	defer ticker.Stop()
	s.sendFrame(client)
	for {
		select {
		case <-client.done:
			return
		case <-ticker.C:
			s.sendFrame(client)
		}
	}
}

func (s *Server) sendFrame(client *clientConn) {
	select {
	case <-client.done:
		return
	default:
	}

	cols, rows, forceAll := client.frameState()
	if cols <= 0 || rows <= 0 {
		cols, rows = s.opts.Mux.Cells()
	}
	view := s.opts.Mux.Viewport(cols, rows)

	f := frame{
		Cols:      view.Cols,
		Rows:      view.Rows,
		ActiveTab: view.ActiveTab,
		Tabs:      view.Tabs,
		Broadcast: view.Broadcast,
		Empty:     s.opts.Mux.Empty(),
	}
	seen := make(map[int]bool, len(view.Panes))
	for _, layout := range view.Panes {
		id := layout.Pane.ID()
		seen[id] = true
		client.applyOffset(layout.Pane)
		revision := layout.Pane.Revision()
		if forceAll || client.needsUpdate(id, revision) {
			f.Updates = append(f.Updates, paneUpdate{
				ID:       id,
				Revision: revision,
				Snapshot: layout.Pane.Snapshot(),
			})
			client.markSent(id, revision)
		}
		f.Boxes = append(f.Boxes, paneBox{
			ID:      id,
			Rect:    layout.Rect,
			Content: layout.Content,
			Active:  layout.Active,
		})
	}
	client.retain(seen)
	client.enqueue(f)
}

func (s *Server) clientClosed(client *clientConn) {
	s.mu.Lock()
	if s.client == client {
		s.client = nil
	}
	s.mu.Unlock()
}

type clientConn struct {
	server *Server
	conn   net.Conn
	enc    *gob.Encoder
	frames chan frame
	done   chan struct{}

	once sync.Once

	mu       sync.Mutex
	cols     int
	rows     int
	offsets  map[int]int
	sent     map[int]uint64
	dirty    map[int]bool
	forceAll bool
}

func newClientConn(server *Server, conn net.Conn, enc *gob.Encoder, cols, rows int) *clientConn {
	if cols <= 0 || rows <= 0 {
		cols, rows = server.opts.Mux.Cells()
	}
	return &clientConn{
		server:  server,
		conn:    conn,
		enc:     enc,
		frames:  make(chan frame, 4),
		done:    make(chan struct{}),
		cols:    cols,
		rows:    rows,
		offsets: make(map[int]int),
		sent:    make(map[int]uint64),
		dirty:   make(map[int]bool),
	}
}

func (c *clientConn) start() {
	go c.writeLoop()
}

func (c *clientConn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case f := <-c.frames:
			if err := c.enc.Encode(&f); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *clientConn) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
		c.server.clientClosed(c)
	})
}

func (c *clientConn) enqueue(f frame) {
	select {
	case c.frames <- f:
		return
	case <-c.done:
		return
	default:
	}
	select {
	case <-c.frames:
	default:
	}
	select {
	case c.frames <- f:
	case <-c.done:
	default:
	}
	c.mu.Lock()
	c.forceAll = true
	c.mu.Unlock()
}

func (c *clientConn) setSize(cols, rows int) {
	c.mu.Lock()
	c.cols, c.rows = cols, rows
	c.mu.Unlock()
}

func (c *clientConn) setOffset(paneID, offset int) {
	c.mu.Lock()
	if offset < 0 {
		offset = 0
	}
	c.offsets[paneID] = offset
	c.dirty[paneID] = true
	c.mu.Unlock()
}

func (c *clientConn) frameState() (cols, rows int, forceAll bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	forceAll = c.forceAll
	c.forceAll = false
	return c.cols, c.rows, forceAll
}

func (c *clientConn) applyOffset(pane mux.PaneView) {
	c.mu.Lock()
	offset, ok := c.offsets[pane.ID()]
	c.mu.Unlock()
	if ok {
		pane.ScrollTo(offset)
	}
}

func (c *clientConn) needsUpdate(id int, revision uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	sent, ok := c.sent[id]
	return c.dirty[id] || !ok || sent != revision
}

func (c *clientConn) markSent(id int, revision uint64) {
	c.mu.Lock()
	c.sent[id] = revision
	delete(c.dirty, id)
	c.mu.Unlock()
}

func (c *clientConn) retain(seen map[int]bool) {
	c.mu.Lock()
	for id := range c.sent {
		if !seen[id] {
			delete(c.sent, id)
			delete(c.offsets, id)
			delete(c.dirty, id)
		}
	}
	c.mu.Unlock()
}
