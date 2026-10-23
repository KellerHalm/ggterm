//go:build windows

package pty

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/UserExistsError/conpty"
)

// defaultShell returns the Windows default shell.
func defaultShell() string {
	if s := os.Getenv("GGTERM_SHELL"); s != "" {
		return s
	}
	if s := os.Getenv("COMSPEC"); s != "" {
		return s
	}
	return "cmd.exe"
}

// windowsSession wraps a ConPTY session.
type windowsSession struct {
	cpty   *conpty.ConPty
	closed atomic.Bool
	mu     sync.Mutex
}

// start creates a Windows ConPTY session.
func start(cmd string, cols, rows int, env []string, dir string) (Session, error) {
	opts := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(cols, rows),
	}

	if len(env) > 0 {
		opts = append(opts, conpty.ConPtyEnv(env))
	}

	if dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(dir))
	}

	cpty, err := conpty.Start(cmd, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to start ConPTY: %w", err)
	}

	return &windowsSession{
		cpty: cpty,
	}, nil
}

func (s *windowsSession) Read(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, ErrClosed
	}
	return s.cpty.Read(p)
}

func (s *windowsSession) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, ErrClosed
	}
	return s.cpty.Write(p)
}

func (s *windowsSession) Resize(size Size) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return ErrClosed
	}

	if size.Cols <= 0 || size.Rows <= 0 {
		return fmt.Errorf("%w: %dx%d (must be positive)", ErrInvalidSize, size.Cols, size.Rows)
	}

	if size.Cols > 10000 || size.Rows > 10000 {
		return fmt.Errorf("%w: %dx%d (too large)", ErrInvalidSize, size.Cols, size.Rows)
	}

	if err := s.cpty.Resize(size.Cols, size.Rows); err != nil {
		return fmt.Errorf("resize failed: %w", err)
	}

	return nil
}

func (s *windowsSession) Wait(ctx context.Context) (int, error) {
	if s.closed.Load() {
		return -1, ErrClosed
	}

	exitCode, err := s.cpty.Wait(ctx)
	if err != nil {
		return -1, fmt.Errorf("wait failed: %w", err)
	}

	return int(exitCode), nil
}

func (s *windowsSession) Close() error {
	// Idempotent close
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cpty != nil {
		return s.cpty.Close()
	}

	return nil
}
