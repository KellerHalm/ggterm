package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ggterm/internal/mux"
	"ggterm/internal/pty"
)

type fakeSession struct {
	output chan []byte
	closed chan struct{}

	mu      sync.Mutex
	written bytes.Buffer
	exited  sync.Once
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

func (s *fakeSession) Resize(size pty.Size) error { return nil }

func (s *fakeSession) Wait(ctx context.Context) (int, error) {
	select {
	case <-s.closed:
		return 0, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

func (s *fakeSession) Close() error {
	s.exited.Do(func() { close(s.closed) })
	return nil
}

func (s *fakeSession) writes() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written.String()
}

type fakeStarter struct {
	mu       sync.Mutex
	sessions []*fakeSession
}

func (s *fakeStarter) start(mux.StartOptions) (pty.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := newFakeSession()
	s.sessions = append(s.sessions, session)
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

func shortSocket(t *testing.T) string {
	t.Helper()
	path, err := endpoint(fmt.Sprintf("test-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	t.Cleanup(func() { removeEndpoint(path) })
	return path
}

func startTestServer(t *testing.T, saveInterval time.Duration) (*fakeStarter, string, string, <-chan struct{}) {
	t.Helper()
	starter := &fakeStarter{}
	cfg := mux.Config{
		Cols:  80,
		Rows:  24,
		Dir:   t.TempDir(),
		Start: starter.start,
	}
	workspace, err := mux.New(cfg)
	if err != nil {
		t.Fatalf("mux.New: %v", err)
	}
	socket := shortSocket(t)
	layout := filepath.Join(t.TempDir(), "layout.json")
	server, err := New(Options{
		Session:      "test",
		Mux:          workspace,
		LayoutPath:   layout,
		SaveInterval: saveInterval,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	listener, err := Listen(socket)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("server did not stop")
		}
	})
	return starter, socket, layout, done
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestAttachInputOutputDetachReattach(t *testing.T) {
	starter, socket, _, _ := startTestServer(t, time.Hour)

	client, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	view := client.Viewport(80, 24)
	if len(view.Panes) != 1 || len(view.Tabs) != 1 {
		t.Fatalf("initial view = %d panes %d tabs, want 1/1", len(view.Panes), len(view.Tabs))
	}
	if starter.count() != 1 {
		t.Fatalf("started sessions = %d, want 1", starter.count())
	}

	client.WriteInput([]byte("echo hi\r"))
	waitFor(t, "input to reach the pane", func() bool {
		return starter.session(0).writes() == "echo hi\r"
	})

	starter.session(0).output <- []byte("hello from the pane")
	waitFor(t, "output in the snapshot", func() bool {
		return paneText(client.Viewport(80, 24)) == "hello from the pane"
	})

	if err := client.Split(mux.Vertical); err != nil {
		t.Fatalf("Split: %v", err)
	}
	waitFor(t, "split layout", func() bool {
		return len(client.Viewport(80, 24).Panes) == 2
	})

	if !client.Detach() {
		t.Fatal("Detach returned false")
	}
	_ = client.Close()

	status, err := FetchStatus(socket)
	if err != nil {
		t.Fatalf("FetchStatus after detach: %v", err)
	}
	if status.Panes != 2 || status.Attached {
		t.Errorf("status after detach = %+v, want 2 panes and not attached", status)
	}

	again, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	defer again.Close()
	view = again.Viewport(80, 24)
	if len(view.Panes) != 2 {
		t.Fatalf("panes after reattach = %d, want 2", len(view.Panes))
	}
	if got := paneText(view); got != "hello from the pane" {
		t.Errorf("scrollback lost after reattach, pane text = %q", got)
	}

	again.Quit()
	waitFor(t, "session to stop", func() bool {
		return !SocketAlive(socket)
	})
	if err := fetchErr(socket); err == nil {
		t.Error("session still responds after quit")
	}
}

func fetchErr(socket string) error {
	_, err := FetchStatus(socket)
	return err
}

func paneText(view mux.View) string {
	var sb strings.Builder
	for _, layout := range view.Panes {
		snap := layout.Pane.Snapshot()
		for row, line := range snap.Lines {
			for _, cell := range line {
				if cell.Width == 0 {
					continue
				}
				if cell.Rune == 0 {
					sb.WriteRune(' ')
				} else {
					sb.WriteRune(cell.Rune)
				}
			}
			if row < len(snap.Lines)-1 {
				sb.WriteByte('\n')
			}
		}
	}
	return strings.TrimRight(strings.TrimSpace(sb.String()), "\n")
}

func TestAbruptDisconnectKeepsSessionRunning(t *testing.T) {
	starter, socket, _, _ := startTestServer(t, time.Hour)

	client, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	starter.session(0).output <- []byte("still here")
	waitFor(t, "output in the snapshot", func() bool {
		return strings.Contains(paneText(client.Viewport(80, 24)), "still here")
	})

	// Simulate a closed window: drop the connection without sending detach.
	_ = client.Close()
	waitFor(t, "daemon to notice the disconnect", func() bool {
		status, err := FetchStatus(socket)
		return err == nil && !status.Attached && status.Panes == 1
	})

	again, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("reattach: %v", err)
	}
	defer again.Close()
	if got := paneText(again.Viewport(80, 24)); !strings.Contains(got, "still here") {
		t.Errorf("session content lost after disconnect, got %q", got)
	}
	again.Quit()
}

func TestStatusAndKill(t *testing.T) {
	_, socket, layout, done := startTestServer(t, time.Hour)

	status, err := FetchStatus(socket)
	if err != nil {
		t.Fatalf("FetchStatus: %v", err)
	}
	if status.Session != "test" || status.Panes != 1 || status.Tabs != 1 {
		t.Errorf("status = %+v", status)
	}

	client, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer client.Close()

	status, err = FetchStatus(socket)
	if err != nil {
		t.Fatalf("FetchStatus attached: %v", err)
	}
	if !status.Attached {
		t.Error("status did not report an attached client")
	}

	if err := Kill(socket); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop after Kill")
	}
	waitFor(t, "layout removal", func() bool {
		_, err := os.Stat(layout)
		return os.IsNotExist(err)
	})
}

func TestDetachedSessionKeepsLayout(t *testing.T) {
	_, socket, layout, _ := startTestServer(t, 20*time.Millisecond)

	client, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := client.Split(mux.Horizontal); err != nil {
		t.Fatalf("Split: %v", err)
	}
	if err := client.NewTab(); err != nil {
		t.Fatalf("NewTab: %v", err)
	}
	waitFor(t, "layout to be saved", func() bool {
		data, err := os.ReadFile(layout)
		return err == nil && bytes.Count(data, []byte("\"pane\"")) == 3
	})
	_ = client.Close()

	waitFor(t, "layout file to remain after detach", func() bool {
		_, err := os.Stat(layout)
		return err == nil
	})
}

func TestScrollPositionSynchronizes(t *testing.T) {
	starter, socket, _, _ := startTestServer(t, time.Hour)

	client, err := Attach(socket, 80, 24)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer client.Close()

	var output strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&output, "line-%03d\r\n", i)
	}
	starter.session(0).output <- []byte(output.String())
	waitFor(t, "output to fill the scrollback", func() bool {
		return strings.Contains(paneText(client.Viewport(80, 24)), "line-099")
	})

	active := client.Viewport(80, 24).ActivePane()
	if active == nil {
		t.Fatal("no active pane")
	}
	active.Scroll(5)
	waitFor(t, "server to apply the scroll offset", func() bool {
		return active.Snapshot().ScrollOffset == 5
	})
	text := paneText(client.Viewport(80, 24))
	if strings.Contains(text, "line-099") {
		t.Errorf("scrolled view still shows the newest line:\n%s", text)
	}
}

func TestListenReportsAlreadyRunning(t *testing.T) {
	_, socket, _, _ := startTestServer(t, time.Hour)
	if _, err := Listen(socket); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Listen on a live socket = %v, want ErrAlreadyRunning", err)
	}
}
