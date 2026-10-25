package mux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ggterm/internal/pty"
)

type fakeSession struct {
	output chan []byte
	closed chan struct{}

	mu       sync.Mutex
	written  bytes.Buffer
	resizes  []pty.Size
	exitCode int
	closeOne sync.Once
}

func newFakeSession() *fakeSession {
	return &fakeSession{
		output: make(chan []byte, 16),
		closed: make(chan struct{}),
	}
}

func (s *fakeSession) Read(p []byte) (int, error) {
	select {
	case chunk := <-s.output:
		return copy(p, chunk), nil
	case <-s.closed:
		return 0, io.EOF
	}
}

func (s *fakeSession) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written.Write(p)
}

func (s *fakeSession) Resize(size pty.Size) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resizes = append(s.resizes, size)
	return nil
}

func (s *fakeSession) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.closed:
		return s.exitCode, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (s *fakeSession) Close() error {
	s.exit(0)
	return nil
}

func (s *fakeSession) exit(code int) {
	s.closeOne.Do(func() {
		s.exitCode = code
		close(s.closed)
	})
}

func (s *fakeSession) writes() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written.String()
}

func (s *fakeSession) resizedTo() []pty.Size {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]pty.Size(nil), s.resizes...)
}

type fakeStarter struct {
	mu       sync.Mutex
	sessions []*fakeSession
	options  []StartOptions
	failAt   int
}

func newFakeStarter() *fakeStarter {
	return &fakeStarter{failAt: -1}
}

func (s *fakeStarter) start(opts StartOptions) (pty.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failAt >= 0 && len(s.sessions) == s.failAt {
		return nil, errors.New("fake start failure")
	}
	session := newFakeSession()
	s.sessions = append(s.sessions, session)
	s.options = append(s.options, opts)
	return session, nil
}

func (s *fakeStarter) session(index int) *fakeSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[index]
}

func (s *fakeStarter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func (s *fakeStarter) option(index int) StartOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.options[index]
}

func newTestMux(t *testing.T, cfg Config) (*Mux, *fakeStarter) {
	t.Helper()
	starter := newFakeStarter()
	if cfg.Start == nil {
		cfg.Start = starter.start
	}
	if cfg.Cols == 0 {
		cfg.Cols = 80
	}
	if cfg.Rows == 0 {
		cfg.Rows = 24
	}
	if cfg.Dir == "" {
		cfg.Dir = t.TempDir()
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	t.Cleanup(m.Shutdown)
	return m, starter
}

func TestViewportResizesSplitPanes(t *testing.T) {
	m, _ := newTestMux(t, Config{})

	view := m.Viewport(80, 24)
	if len(view.Panes) != 1 {
		t.Fatalf("initial panes = %d, want 1", len(view.Panes))
	}
	if view.Panes[0].Content.W != 78 || view.Panes[0].Content.H != 22 {
		t.Errorf("initial content = %+v, want 78x22", view.Panes[0].Content)
	}

	if err := m.Split(Vertical); err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	view = m.Viewport(80, 24)
	if len(view.Panes) != 2 {
		t.Fatalf("panes after split = %d, want 2", len(view.Panes))
	}
	wantRects := []Rect{
		{X: 0, Y: 0, W: 40, H: 24},
		{X: 40, Y: 0, W: 40, H: 24},
	}
	for i, layout := range view.Panes {
		if layout.Rect != wantRects[i] {
			t.Errorf("pane %d rect = %+v, want %+v", i, layout.Rect, wantRects[i])
		}
		if layout.Content.W != 38 || layout.Content.H != 22 {
			t.Errorf("pane %d content = %+v, want 38x22", i, layout.Content)
		}
	}
	if view.ActivePane() != view.Panes[1].Pane {
		t.Error("new pane was not focused after split")
	}
}

func TestSplitHorizontalLayout(t *testing.T) {
	m, _ := newTestMux(t, Config{})
	if err := m.Split(Horizontal); err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	view := m.Viewport(80, 24)
	if len(view.Panes) != 2 {
		t.Fatalf("panes = %d, want 2", len(view.Panes))
	}
	if got, want := view.Panes[0].Rect, (Rect{X: 0, Y: 0, W: 80, H: 12}); got != want {
		t.Errorf("first rect = %+v, want %+v", got, want)
	}
	if got, want := view.Panes[1].Rect, (Rect{X: 0, Y: 12, W: 80, H: 12}); got != want {
		t.Errorf("second rect = %+v, want %+v", got, want)
	}
}

func TestSplitRejectsTinyPane(t *testing.T) {
	m, _ := newTestMux(t, Config{Cols: 9, Rows: 9})
	m.Viewport(9, 9)
	if err := m.Split(Vertical); !errors.Is(err, ErrTooSmall) {
		t.Fatalf("Split error = %v, want ErrTooSmall", err)
	}
}

func TestFocusDirectionAndClosePane(t *testing.T) {
	m, _ := newTestMux(t, Config{})
	m.Viewport(80, 24)
	if err := m.Split(Vertical); err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	view := m.Viewport(80, 24)
	left, right := view.Panes[0].Pane, view.Panes[1].Pane

	m.FocusDirection(MoveLeft)
	view = m.Viewport(80, 24)
	if view.ActivePane() != left {
		t.Error("focus did not move left")
	}

	m.CloseActivePane()
	view = m.Viewport(80, 24)
	if len(view.Panes) != 1 || view.ActivePane() != right {
		t.Errorf("after close panes = %d active = %v, want 1 focused right pane", len(view.Panes), view.ActivePane() == right)
	}
}

func TestTabsAndEmptySignal(t *testing.T) {
	m, _ := newTestMux(t, Config{})
	m.Viewport(80, 24)

	if err := m.NewTab(); err != nil {
		t.Fatalf("NewTab failed: %v", err)
	}
	view := m.Viewport(80, 24)
	if len(view.Tabs) != 2 || view.ActiveTab != 1 {
		t.Fatalf("tabs = %d active = %d, want 2 and 1", len(view.Tabs), view.ActiveTab)
	}

	m.PrevTab()
	view = m.Viewport(80, 24)
	if view.ActiveTab != 0 {
		t.Errorf("active tab = %d, want 0", view.ActiveTab)
	}

	m.CloseTab()
	view = m.Viewport(80, 24)
	if len(view.Tabs) != 1 {
		t.Fatalf("tabs after close = %d, want 1", len(view.Tabs))
	}

	m.CloseActivePane()
	if !m.Empty() {
		t.Fatal("mux should be empty after closing the last pane")
	}
	select {
	case <-m.EmptyChan():
	default:
		t.Fatal("EmptyChan was not closed")
	}
}

func TestBroadcastWritesToEveryPaneInTab(t *testing.T) {
	m, starter := newTestMux(t, Config{})
	m.Viewport(80, 24)
	if err := m.Split(Vertical); err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	m.SetBroadcast(true)
	m.WriteInput([]byte("echo hi\r"))

	if got := starter.session(0).writes(); got != "echo hi\r" {
		t.Errorf("first pane writes = %q", got)
	}
	if got := starter.session(1).writes(); got != "echo hi\r" {
		t.Errorf("second pane writes = %q", got)
	}
}

func TestPanePumpFeedsTerminalAndTitle(t *testing.T) {
	m, starter := newTestMux(t, Config{})
	m.Viewport(80, 24)
	session := starter.session(0)

	session.output <- []byte("hello")
	waitFor(t, func() bool {
		snap := m.tabs[0].Active.Snapshot()
		return len(snap.Lines) > 0 && len(snap.Lines[0]) >= 5 &&
			snap.Lines[0][0].Rune == 'h' &&
			snap.Lines[0][1].Rune == 'e' &&
			snap.Lines[0][4].Rune == 'o'
	})

	session.output <- []byte("\x1b]0;my-title\x07")
	waitFor(t, func() bool {
		return m.tabs[0].Active.Title() == "my-title"
	})
}

func TestRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m, _ := newTestMux(t, Config{Dir: dir})
	m.Viewport(80, 24)
	if err := m.Split(Vertical); err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	if err := m.Split(Horizontal); err != nil {
		t.Fatalf("Split failed: %v", err)
	}
	if err := m.NewTab(); err != nil {
		t.Fatalf("NewTab failed: %v", err)
	}
	m.Viewport(80, 24)

	path := filepath.Join(t.TempDir(), "layout.json")
	if err := Save(path, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("layout file missing: %v", err)
	}

	starter := newFakeStarter()
	restored, err := Restore(path, Config{Cols: 80, Rows: 24, Dir: dir, Start: starter.start})
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	t.Cleanup(restored.Shutdown)

	view := restored.Viewport(80, 24)
	if len(view.Tabs) != 2 {
		t.Fatalf("restored tabs = %d, want 2", len(view.Tabs))
	}
	if view.ActiveTab != 1 {
		t.Errorf("restored active tab = %d, want 1", view.ActiveTab)
	}
	if starter.count() != 4 {
		t.Errorf("started panes = %d, want 4", starter.count())
	}
	for i := 0; i < starter.count(); i++ {
		if got := starter.option(i).Dir; got != dir {
			t.Errorf("pane %d dir = %q, want %q", i, got, dir)
		}
	}

	restored.SelectTab(0)
	view = restored.Viewport(80, 24)
	if len(view.Panes) != 3 {
		t.Fatalf("restored split panes = %d, want 3", len(view.Panes))
	}
	wantRects := []Rect{
		{X: 0, Y: 0, W: 40, H: 24},
		{X: 40, Y: 0, W: 40, H: 12},
		{X: 40, Y: 12, W: 40, H: 12},
	}
	for i, layout := range view.Panes {
		if layout.Rect != wantRects[i] {
			t.Errorf("restored pane %d rect = %+v, want %+v", i, layout.Rect, wantRects[i])
		}
	}
	if !view.Panes[2].Active {
		t.Error("restored active pane was not the last leaf")
	}
}

func TestSaveEmptyLayoutRemovesFile(t *testing.T) {
	m, _ := newTestMux(t, Config{})
	m.Viewport(80, 24)
	m.CloseActivePane()
	if !m.Empty() {
		t.Fatal("mux should be empty")
	}

	path := filepath.Join(t.TempDir(), "layout.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("layout file still exists, stat err = %v", err)
	}
}

func TestRestoreRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "layout.json")
	data := `{"version":99,"tabs":[{"root":{"pane":{}}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(path, Config{Start: newFakeStarter().start}); err == nil {
		t.Fatal("Restore accepted an unknown layout version")
	}
}

func TestRestoreFallsBackToConfigDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	m, _ := newTestMux(t, Config{Dir: missing})
	m.Viewport(80, 24)
	path := filepath.Join(t.TempDir(), "layout.json")
	if err := Save(path, m); err != nil {
		t.Fatal(err)
	}

	fallback := t.TempDir()
	starter := newFakeStarter()
	restored, err := Restore(path, Config{Dir: fallback, Start: starter.start})
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	t.Cleanup(restored.Shutdown)
	if got := starter.option(0).Dir; got != fallback {
		t.Errorf("restored dir = %q, want fallback %q", got, fallback)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
