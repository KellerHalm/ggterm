package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"ggterm/internal/mux"
	"ggterm/internal/ui"
)

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
	restore := flag.Bool("restore", true, "restore tabs and split layout from the previous session")
	layoutFile := flag.String("layout-file", "", "layout file path (default: user config directory)")
	flag.Parse()

	if *cols <= 0 || *rows <= 0 {
		return fmt.Errorf("cols and rows must be positive, got %dx%d", *cols, *rows)
	}
	if *scrollback < 0 {
		return fmt.Errorf("scrollback must be non-negative, got %d", *scrollback)
	}

	path := *layoutFile
	if path == "" {
		if defaultPath, err := mux.DefaultLayoutPath(); err == nil {
			path = defaultPath
		}
	}

	cfg := mux.Config{
		Cols:          *cols,
		Rows:          *rows,
		MaxScrollback: *scrollback,
		Command:       *shell,
	}

	var workspace *mux.Mux
	var err error
	if *restore && path != "" {
		workspace, err = mux.Restore(path, cfg)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "ggterm: restore layout: %v\n", err)
			}
			workspace = nil
		}
	}
	if workspace == nil {
		workspace, err = mux.New(cfg)
		if err != nil {
			return fmt.Errorf("start terminal session: %w", err)
		}
	}

	runErr := ui.Run(workspace, ui.Options{FontSize: *fontSize})

	var saveErr error
	if path != "" {
		if err := mux.Save(path, workspace); err != nil {
			saveErr = fmt.Errorf("save layout: %w", err)
		}
	}
	workspace.Shutdown()

	return errors.Join(runErr, saveErr)
}
