//go:build !windows

package daemon

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// endpointDir uses a short per-user temporary directory so the socket path
// stays below the platform limit for sun_path (104 bytes on macOS).
func endpointDir() (string, error) {
	return filepath.Join(os.TempDir(), fmt.Sprintf("ggterm-%d", os.Getuid())), nil
}

func endpoint(name string) (string, error) {
	if err := ValidateSessionName(name); err != nil {
		return "", err
	}
	dir, err := endpointDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".sock"), nil
}

func listenEndpoint(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create session directory: %w", err)
	}
	ln, err := net.Listen("unix", path)
	if err == nil {
		_ = os.Chmod(path, 0o600)
		return ln, nil
	}
	// The path already exists: either a live daemon owns it or a previous
	// process left a stale socket behind.
	if conn, dialErr := net.DialTimeout("unix", path, 250*time.Millisecond); dialErr == nil {
		_ = conn.Close()
		return nil, ErrAlreadyRunning
	}
	if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
		return nil, fmt.Errorf("remove stale socket: %w", removeErr)
	}
	ln, err = net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return ln, nil
}

func dialEndpoint(path string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", path, timeout)
}

func removeEndpoint(path string) {
	_ = os.Remove(path)
}

// registerSession and unregisterSession are no-ops on Unix: the socket itself
// is the registry.
func registerSession(string) {}

func unregisterSession(string) {}

func sessionNames() ([]string, error) {
	dir, err := endpointDir()
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sock") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".sock")
		if ValidateSessionName(name) == nil {
			names = append(names, name)
		}
	}
	return names, nil
}
