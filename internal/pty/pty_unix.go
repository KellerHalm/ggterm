//go:build !windows

package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/creack/pty"
)

// defaultShell returns the Unix default shell.
func defaultShell() string {
	if s := os.Getenv("GGTERM_SHELL"); s != "" {
		return s
	}
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// unixSession wraps a Unix PTY session.
type unixSession struct {
	ptmx   *os.File
	cmd    *exec.Cmd
	closed atomic.Bool
	mu     sync.Mutex
}

// start creates a Unix PTY session.
func start(command string, cols, rows int, env []string, dir string) (Session, error) {
	var cmd *exec.Cmd

	// If command is empty, exec the shell directly.
	// Otherwise, run it through sh -c so quoted args work.
	shell := defaultShell()
	if command == shell && !strings.ContainsAny(command, " \t") {
		cmd = exec.Command(shell)
	} else {
		cmd = exec.Command(shell, "-c", command)
	}

	// Set environment
	cmd.Env = env

	// Set working directory
	if dir != "" {
		cmd.Dir = dir
	}

	// Start with PTY
	winsize := &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}

	ptmx, err := pty.StartWithSize(cmd, winsize)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	return &unixSession{
		ptmx: ptmx,
		cmd:  cmd,
	}, nil
}

func (s *unixSession) Read(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, ErrClosed
	}
	return s.ptmx.Read(p)
}

func (s *unixSession) Write(p []byte) (int, error) {
	if s.closed.Load() {
		return 0, ErrClosed
	}
	return s.ptmx.Write(p)
}

func (s *unixSession) Resize(size Size) error {
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

	winsize := &pty.Winsize{
		Rows: uint16(size.Rows),
		Cols: uint16(size.Cols),
	}

	if err := pty.Setsize(s.ptmx, winsize); err != nil {
		return fmt.Errorf("resize failed: %w", err)
	}

	return nil
}

func (s *unixSession) Wait(ctx context.Context) (int, error) {
	// Wait must still reap the child after Close, so it does not reject closed sessions.
	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - kill the process group
		if s.cmd.Process != nil {
			// Send SIGKILL to the process group
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		return -1, ctx.Err()

	case err := <-done:
		if err == nil {
			return 0, nil
		}

		// Extract exit code from error
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus(), nil
			}
		}

		// Exit code unavailable
		return -1, fmt.Errorf("process exited with error: %w", err)
	}
}

func (s *unixSession) Close() error {
	// Idempotent close
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error

	// Close the PTY master
	if s.ptmx != nil {
		if err := s.ptmx.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close PTY: %w", err)
		}
	}

	if s.cmd != nil && s.cmd.Process != nil {
		pid := s.cmd.Process.Pid
		if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			if killErr := s.cmd.Process.Kill(); killErr != nil && firstErr == nil &&
				killErr.Error() != "os: process already finished" {
				firstErr = fmt.Errorf("kill process: %w", killErr)
			}
		}
	}

	return firstErr
}
