//go:build windows

package daemon

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
)

// endpoint returns the named pipe used by a session daemon. The user name is
// part of the pipe so different accounts never share a session.
func endpoint(name string) (string, error) {
	if err := ValidateSessionName(name); err != nil {
		return "", err
	}
	return `\\.\pipe\ggterm-` + pipeUser() + `-` + name, nil
}

func pipeUser() string {
	current, err := user.Current()
	if err != nil || current.Username == "" {
		return "user"
	}
	var b strings.Builder
	for _, r := range current.Username {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 32 {
			break
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

func listenEndpoint(path string) (net.Listener, error) {
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{
		InputBufferSize:  64 * 1024,
		OutputBufferSize: 64 * 1024,
	})
	if err == nil {
		return ln, nil
	}
	// The pipe name is taken by another instance: report a conflict instead
	// of a confusing access error.
	if conn, dialErr := dialEndpoint(path, 250*time.Millisecond); dialErr == nil {
		_ = conn.Close()
		return nil, ErrAlreadyRunning
	}
	return nil, fmt.Errorf("listen on %s: %w", path, err)
}

func dialEndpoint(path string, timeout time.Duration) (net.Conn, error) {
	return winio.DialPipe(path, &timeout)
}

func removeEndpoint(string) {}

// Named pipes do not appear on disk, so running sessions announce themselves
// with a small marker file. Stale markers are harmless: ListStatuses probes
// the pipe and skips sessions that no longer respond.
func registerSession(name string) {
	if ValidateSessionName(name) != nil {
		return
	}
	dir, err := SessionsDir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name+".session"), nil, 0o600)
}

func unregisterSession(name string) {
	if ValidateSessionName(name) != nil {
		return
	}
	dir, err := SessionsDir()
	if err != nil {
		return
	}
	_ = os.Remove(filepath.Join(dir, name+".session"))
}

func sessionNames() ([]string, error) {
	dir, err := SessionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".session") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".session")
		if err := ValidateSessionName(name); err == nil {
			names = append(names, name)
		}
	}
	return names, nil
}
