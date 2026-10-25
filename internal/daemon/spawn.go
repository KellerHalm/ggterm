package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// SocketAlive reports whether a daemon is accepting connections on socket.
func SocketAlive(socket string) bool {
	conn, err := dialEndpoint(socket, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// WaitForSocket polls until a daemon starts listening on socket.
func WaitForSocket(socket string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if SocketAlive(socket) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not start listening on %s", socket)
}

// Spawn starts exe as a detached background process. Its output is appended to
// logPath. The new process is not waited for.
func Spawn(exe string, args []string, logPath, dir string) error {
	var logFile *os.File
	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
			return fmt.Errorf("create log directory: %w", err)
		}
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return fmt.Errorf("open daemon log: %w", err)
		}
	}

	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = sysProcAttr()
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return fmt.Errorf("start daemon: %w", err)
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	return cmd.Process.Release()
}
