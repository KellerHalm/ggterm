package mux

import (
	"context"
	"errors"
	"os"
	"sync"

	"ggterm/internal/pty"
	"ggterm/internal/terminal"
)

// StartOptions describes a process that should be attached to a new pane.
type StartOptions struct {
	Command string
	Dir     string
	Cols    int
	Rows    int
}

// Pane owns a terminal buffer and the pseudoterminal process behind it.
//
// The pane is safe for concurrent use: output is pumped in a background
// goroutine while the UI reads snapshots and sends input.
type Pane struct {
	id      int
	command string
	dir     string

	mu       sync.Mutex
	term     *terminal.Terminal
	sess     pty.Session
	cols     int
	rows     int
	scroll   int
	alive    bool
	exitCode int
	exitErr  error

	done      chan struct{}
	closeOnce sync.Once
}

type paneOptions struct {
	id         int
	command    string
	dir        string
	cols       int
	rows       int
	scrollback int
	start      func(StartOptions) (pty.Session, error)
}

func newPane(opts paneOptions) (*Pane, error) {
	if opts.cols <= 0 {
		opts.cols = 80
	}
	if opts.rows <= 0 {
		opts.rows = 24
	}
	if opts.dir == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.dir = cwd
		}
	}

	term := terminal.New(terminal.Config{
		Cols:          opts.cols,
		Rows:          opts.rows,
		MaxScrollback: opts.scrollback,
	})
	sess, err := opts.start(StartOptions{
		Command: opts.command,
		Dir:     opts.dir,
		Cols:    opts.cols,
		Rows:    opts.rows,
	})
	if err != nil {
		return nil, err
	}

	p := &Pane{
		id:      opts.id,
		command: opts.command,
		dir:     opts.dir,
		term:    term,
		sess:    sess,
		cols:    opts.cols,
		rows:    opts.rows,
		alive:   true,
		done:    make(chan struct{}),
	}
	go p.pump()
	go p.wait()
	return p, nil
}

func (p *Pane) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := p.sess.Read(buf)
		if n > 0 {
			if _, writeErr := p.term.Write(buf[:n]); writeErr != nil {
				return
			}
			for _, response := range p.term.TakeResponses() {
				if _, writeErr := p.sess.Write(response); writeErr != nil {
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *Pane) wait() {
	code, err := p.sess.Wait(context.Background())
	p.mu.Lock()
	p.alive = false
	p.exitCode = code
	p.exitErr = err
	p.mu.Unlock()
	close(p.done)
}

// ID returns the stable numeric identifier of the pane.
func (p *Pane) ID() int { return p.id }

// Dir returns the working directory the pane was started in.
func (p *Pane) Dir() string { return p.dir }

// Command returns the command the pane was started with (empty for the default shell).
func (p *Pane) Command() string { return p.command }

// Done is closed after the underlying process exits.
func (p *Pane) Done() <-chan struct{} { return p.done }

// Alive reports whether the underlying process is still running.
func (p *Pane) Alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.alive
}

// ExitCode returns the exit status once the process has finished.
func (p *Pane) ExitCode() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode, p.exitErr
}

// Size returns the current terminal dimensions of the pane content.
func (p *Pane) Size() (cols, rows int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cols, p.rows
}

// Resize applies new content dimensions to both the buffer and the process.
func (p *Pane) Resize(cols, rows int) error {
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}

	p.mu.Lock()
	if p.cols == cols && p.rows == rows {
		p.mu.Unlock()
		return nil
	}
	p.cols = cols
	p.rows = rows
	p.mu.Unlock()

	p.term.Resize(cols, rows)
	err := p.sess.Resize(pty.Size{Cols: cols, Rows: rows})
	if err != nil && errors.Is(err, pty.ErrClosed) {
		return nil
	}
	return err
}

// Write sends input to the pane process.
func (p *Pane) Write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if _, err := p.sess.Write(data); err != nil && !errors.Is(err, pty.ErrClosed) {
		return err
	}
	return nil
}

// Snapshot returns a consistent copy of the visible terminal buffer at the
// pane's current scroll offset.
func (p *Pane) Snapshot() terminal.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.term.Snapshot(p.scroll)
}

// Scroll moves the viewport by delta lines into the scrollback history.
// Positive values scroll up (towards older output).
func (p *Pane) Scroll(delta int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scroll = p.clampScroll(p.scroll + delta)
}

// ScrollTo positions the viewport at an absolute scrollback offset.
func (p *Pane) ScrollTo(offset int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scroll = p.clampScroll(offset)
}

func (p *Pane) clampScroll(offset int) int {
	limit, alt := p.term.ScrollInfo()
	if alt || offset < 0 {
		return 0
	}
	if offset > limit {
		return limit
	}
	return offset
}

// ResetScroll returns the viewport to the live output.
func (p *Pane) ResetScroll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.scroll = 0
}

// Revision reports how many times the terminal content may have changed.
func (p *Pane) Revision() uint64 {
	return p.term.Revision()
}

// ScrollOffset returns the current scrollback offset of the pane viewport.
func (p *Pane) ScrollOffset() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scroll
}

// Title returns the most recent window title reported by the application.
func (p *Pane) Title() string {
	return p.term.Title()
}

// Close terminates the pane process. It is safe to call multiple times.
func (p *Pane) Close() error {
	var err error
	p.closeOnce.Do(func() {
		err = p.sess.Close()
	})
	return err
}
