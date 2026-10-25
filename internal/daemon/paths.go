package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultSession is the session used when no name is given.
const DefaultSession = "default"

// ValidateSessionName rejects names that cannot be used as file names.
func ValidateSessionName(name string) error {
	if name == "" {
		return errors.New("session name must not be empty")
	}
	if len(name) > 64 {
		return errors.New("session name is too long")
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("session name %q contains invalid character %q", name, r)
		}
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("session name %q must not start with a dot", name)
	}
	return nil
}

// ConfigDir returns the ggterm directory inside the user's config directory.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ggterm"), nil
}

// SessionsDir returns the directory that stores session layouts and markers.
func SessionsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// SocketPath returns the endpoint used by a session daemon: a unix socket on
// Unix and a named pipe on Windows.
func SocketPath(name string) (string, error) {
	return endpoint(name)
}

// LayoutPath returns the file used to persist a session layout.
func LayoutPath(name string) (string, error) {
	if err := ValidateSessionName(name); err != nil {
		return "", err
	}
	dir, err := SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// LogPath returns the daemon log file path.
func LogPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.log"), nil
}

// ListSessions returns candidate session names known to the current user.
// Use ListStatuses to check which of them are actually running.
func ListSessions() ([]string, error) {
	return sessionNames()
}
