package pty

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

const readTimeout = 5 * time.Second

func startSession(t *testing.T, opts Options) Session {
	t.Helper()
	sess, err := Start(opts)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	t.Cleanup(func() {
		if err := sess.Close(); err != nil && !errors.Is(err, ErrClosed) {
			t.Errorf("Close failed: %v", err)
		}
	})
	return sess
}

func markerCommand(marker string) string {
	return "echo " + marker + "\r\n"
}

func Test_Start_default_shell_echoes_marker(t *testing.T) {
	sess := startSession(t, Options{})

	if _, err := sess.Write([]byte(markerCommand("ggterm-pty-ok"))); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	output := readUntilMarker(ctx, sess, "ggterm-pty-ok")
	if !strings.Contains(output, "ggterm-pty-ok") {
		t.Fatalf("output did not contain marker, got %q", output)
	}

	if err := sess.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(context.Background(), readTimeout)
	defer waitCancel()
	if _, err := sess.Wait(waitCtx); err != nil && !errors.Is(err, ErrClosed) {
		t.Errorf("Wait failed after close: %v", err)
	}
}

func Test_Resize_keeps_session_functional(t *testing.T) {
	sess := startSession(t, Options{Cols: 80, Rows: 24})

	if err := sess.Resize(Size{Cols: 120, Rows: 40}); err != nil {
		t.Fatalf("Resize failed: %v", err)
	}
	if _, err := sess.Write([]byte(markerCommand("resize-ok"))); err != nil {
		t.Fatalf("Write after resize failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	if output := readUntilMarker(ctx, sess, "resize-ok"); !strings.Contains(output, "resize-ok") {
		t.Errorf("output after resize did not contain marker, got %q", output)
	}
}

func Test_Resize_rejects_invalid_dimensions(t *testing.T) {
	sess := startSession(t, Options{})

	for _, size := range []Size{
		{Cols: 0, Rows: 24},
		{Cols: 80, Rows: 0},
		{Cols: -1, Rows: 24},
		{Cols: 80, Rows: -1},
		{Cols: 10001, Rows: 24},
	} {
		if err := sess.Resize(size); !errors.Is(err, ErrInvalidSize) {
			t.Errorf("Resize(%+v) error = %v, want ErrInvalidSize", size, err)
		}
	}
}

func Test_Start_rejects_oversized_dimensions(t *testing.T) {
	sess, err := Start(Options{Cols: 10001, Rows: 24})
	if sess != nil {
		if closeErr := sess.Close(); closeErr != nil && !errors.Is(closeErr, ErrClosed) {
			t.Errorf("Close after rejected Start failed: %v", closeErr)
		}
	}
	if !errors.Is(err, ErrInvalidSize) {
		t.Fatalf("Start error = %v, want ErrInvalidSize", err)
	}
}

func Test_Close_is_idempotent_and_does_not_hang(t *testing.T) {
	sess := startSession(t, Options{})

	firstClose := make(chan error, 1)
	go func() {
		firstClose <- sess.Close()
	}()

	select {
	case err := <-firstClose:
		if err != nil {
			t.Errorf("first Close failed: %v", err)
		}
	case <-time.After(readTimeout):
		t.Fatal("Close hung")
	}

	if err := sess.Close(); err != nil {
		t.Errorf("second Close failed: %v", err)
	}
}

func Test_DefaultShell_is_not_empty(t *testing.T) {
	if shell := DefaultShell(); shell == "" {
		t.Error("DefaultShell returned empty string")
	}
}

func readUntilMarker(ctx context.Context, sess Session, marker string) string {
	chunks := make(chan []byte, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := sess.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case chunks <- chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	var output strings.Builder
	for {
		select {
		case chunk := <-chunks:
			output.Write(chunk)
			if strings.Contains(output.String(), marker) {
				return output.String()
			}
		case <-ctx.Done():
			return output.String()
		}
	}
}

func Test_Wait_returns_process_exit_code(t *testing.T) {
	command := "exit 3"
	if runtime.GOOS == "windows" {
		command = "cmd /c exit 3"
	}
	sess := startSession(t, Options{Command: command})

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	code, err := sess.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}
