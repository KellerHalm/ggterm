// Package pty provides cross-platform pseudoterminal session management.
//
// On Windows it wraps ConPTY (requires Windows 10 1809+).
// On Unix systems it uses creack/pty.
package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrClosed is returned when an operation is attempted on a closed session.
var ErrClosed = errors.New("pty: session closed")

// ErrInvalidSize is returned when terminal dimensions are not positive or exceed the supported maximum.
var ErrInvalidSize = errors.New("pty: invalid size")

// Size represents terminal dimensions in columns and rows.
type Size struct {
	Cols int
	Rows int
}

// Options configures a new pseudoterminal session.
type Options struct {
	// Command is the command line to execute.
	// Empty string launches the platform default shell.
	// May include arguments.
	Command string

	// Cols and Rows specify terminal dimensions.
	// Values <= 0 default to 80x24.
	Cols int
	Rows int

	// Env specifies environment variables for the child process.
	// nil inherits os.Environ() plus terminal-specific variables.
	// Terminal variables (TERM, COLORTERM) are always set.
	Env []string

	// Dir specifies the working directory.
	// Empty string inherits the current working directory.
	Dir string
}

// Session represents an active pseudoterminal session.
//
// Methods are safe for concurrent use from multiple goroutines.
// A session must be closed exactly once via Close.
type Session interface {
	// Read reads output from the pseudoterminal.
	// Returns io.EOF when the session terminates.
	Read(p []byte) (int, error)

	// Write writes input to the pseudoterminal.
	Write(p []byte) (int, error)

	// Resize changes the terminal dimensions.
	// Returns an error if the size is invalid or the session is closed.
	Resize(size Size) error

	// Wait blocks until the process exits or ctx is cancelled.
	// Returns the exit code (0 for success, -1 if unavailable).
	// Returns ctx.Err() if the context is cancelled.
	Wait(ctx context.Context) (int, error)

	// Close terminates the session and releases resources.
	// Safe to call multiple times; subsequent calls return nil.
	// Does not wait for the process to exit - use Wait for that.
	Close() error
}

// DefaultShell returns the platform-appropriate default shell command.
//
// The shell is determined by checking in order:
//   - GGTERM_SHELL environment variable
//   - Platform-specific shell (COMSPEC on Windows, $SHELL on Unix)
//   - Fallback (cmd.exe on Windows, /bin/sh on Unix)
func DefaultShell() string {
	return defaultShell()
}

// Start creates and starts a new pseudoterminal session.
//
// The process is started immediately. Use Wait to retrieve the exit code.
// The session must be closed via Close when no longer needed.
//
// Returns an error if:
//   - The platform does not support pseudoterminals (e.g., Windows < 1809)
//   - The command cannot be started
//   - Terminal dimensions are invalid
func Start(opts Options) (Session, error) {
	// Normalize dimensions
	cols := opts.Cols
	rows := opts.Rows
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}

	// Validate dimensions
	if cols > 10000 || rows > 10000 {
		return nil, fmt.Errorf("%w: %dx%d (too large)", ErrInvalidSize, cols, rows)
	}

	// Determine command
	cmd := opts.Command
	if cmd == "" {
		cmd = DefaultShell()
	}

	// Merge environment
	env := mergeEnv(opts.Env)

	return start(cmd, cols, rows, env, opts.Dir)
}

// mergeEnv merges user-provided environment variables with required terminal variables.
// Always sets TERM=xterm-256color and COLORTERM=truecolor.
// Preserves all other variables from opts if provided, otherwise inherits os.Environ().
func mergeEnv(userEnv []string) []string {
	var base []string
	if userEnv != nil {
		base = make([]string, 0, len(userEnv)+2)
		// Copy user env, filtering out TERM/COLORTERM which we'll add
		for _, kv := range userEnv {
			if !strings.HasPrefix(kv, "TERM=") && !strings.HasPrefix(kv, "COLORTERM=") {
				base = append(base, kv)
			}
		}
	} else {
		baseEnv := os.Environ()
		base = make([]string, 0, len(baseEnv)+2)
		// Copy inherited env, filtering out TERM/COLORTERM
		for _, kv := range baseEnv {
			if !strings.HasPrefix(kv, "TERM=") && !strings.HasPrefix(kv, "COLORTERM=") {
				base = append(base, kv)
			}
		}
	}

	// Append required terminal variables
	base = append(base, "TERM=xterm-256color", "COLORTERM=truecolor")
	return base
}
