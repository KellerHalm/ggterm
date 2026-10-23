package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"ggterm/internal/pty"
	"ggterm/internal/terminal"
	"ggterm/internal/ui"
)

const shutdownTimeout = 2 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ggterm: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	shell := flag.String("shell", "", "shell command to run (default: platform shell)")
	cols := flag.Int("cols", 120, "initial terminal columns")
	rows := flag.Int("rows", 36, "initial terminal rows")
	scrollback := flag.Int("scrollback", 10000, "scrollback buffer size in lines")
	fontSize := flag.Float64("font-size", 15, "font size in points")
	flag.Parse()

	if *cols <= 0 || *rows <= 0 {
		return fmt.Errorf("cols and rows must be positive, got %dx%d", *cols, *rows)
	}
	if *scrollback < 0 {
		return fmt.Errorf("scrollback must be non-negative, got %d", *scrollback)
	}

	term := terminal.New(terminal.Config{
		Cols:          *cols,
		Rows:          *rows,
		MaxScrollback: *scrollback,
	})
	sess, err := pty.Start(pty.Options{
		Command: *shell,
		Cols:    *cols,
		Rows:    *rows,
	})
	if err != nil {
		return fmt.Errorf("start terminal session: %w", err)
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		pumpOutput(term, sess)
	}()

	exitDone := make(chan struct{})
	waitCh := make(chan struct{}, 1)
	go func() {
		_, _ = sess.Wait(context.Background())
		waitCh <- struct{}{}
		close(exitDone)
	}()

	runErr := ui.Run(term, sess, ui.Options{FontSize: *fontSize, Done: exitDone})
	closeErr := sess.Close()

	select {
	case <-readDone:
	case <-time.After(shutdownTimeout):
	}

	select {
	case <-waitCh:
	case <-time.After(shutdownTimeout):
	}

	return errors.Join(runErr, closeErr)
}

func pumpOutput(term *terminal.Terminal, sess pty.Session) {
	buf := make([]byte, 32*1024)
	for {
		n, err := sess.Read(buf)
		if n > 0 {
			if _, writeErr := term.Write(buf[:n]); writeErr != nil {
				return
			}
			if replyErr := forwardResponses(term, sess); replyErr != nil {
				return
			}
		}
		if err == nil {
			continue
		}
		if !errors.Is(err, io.EOF) && !errors.Is(err, pty.ErrClosed) {
			if _, writeErr := term.Write([]byte("\r\n[ggterm: session closed]\r\n")); writeErr != nil {
				return
			}
		}
		return
	}
}

func forwardResponses(term *terminal.Terminal, sess pty.Session) error {
	for _, response := range term.TakeResponses() {
		if _, err := sess.Write(response); err != nil {
			return fmt.Errorf("write terminal response: %w", err)
		}
	}
	return nil
}
