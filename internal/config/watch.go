package config

import (
	"os"
	"path/filepath"
	"time"
)

type fileStamp struct {
	modTime time.Time
	size    int64
}

// Watcher polls the configuration files and reloads them when they change.
// Polling is used instead of platform-specific file notifications so that the
// same code works on Windows, Linux and macOS.
type Watcher struct {
	loader   Loader
	settings Settings
	files    []string
	stamps   map[string]fileStamp
}

// NewWatcher creates a watcher for a loader and its initial result.
func NewWatcher(loader Loader, initial Result) *Watcher {
	w := &Watcher{
		loader:   loader,
		settings: initial.Settings,
	}
	w.files = loader.watchPaths(initial.Files)
	w.capture()
	return w
}

// Settings returns the last successfully loaded settings.
func (w *Watcher) Settings() Settings {
	return w.settings
}

// Poll reloads the configuration when any watched file changed. The second
// return value reports whether the settings were replaced.
func (w *Watcher) Poll() (Settings, bool, error) {
	if !w.changed() {
		return w.settings, false, nil
	}
	// Capture first so a broken file is not retried on every tick.
	w.capture()

	result, err := w.loader.Load()
	if err != nil {
		return w.settings, false, err
	}
	w.settings = result.Settings
	w.files = w.loader.watchPaths(result.Files)
	w.capture()
	return w.settings, true, nil
}

func (w *Watcher) changed() bool {
	for _, path := range w.files {
		if w.stamps[path] != stampOf(path) {
			return true
		}
	}
	return false
}

func (w *Watcher) capture() {
	w.stamps = make(map[string]fileStamp, len(w.files))
	for _, path := range w.files {
		w.stamps[path] = stampOf(path)
	}
}

func stampOf(path string) fileStamp {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{modTime: info.ModTime(), size: info.Size()}
}

func (l Loader) watchPaths(loaded []string) []string {
	seen := make(map[string]bool)
	var paths []string
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	if l.Disable {
		return paths
	}
	if l.Path != "" {
		abs, err := filepath.Abs(l.Path)
		if err != nil {
			abs = l.Path
		}
		add(abs)
		if !IsLuaPath(abs) {
			add(filepath.Join(filepath.Dir(abs), DefaultLuaName))
		}
	} else if dir, err := ConfigDir(); err == nil {
		add(filepath.Join(dir, DefaultConfigName))
		add(filepath.Join(dir, DefaultLuaName))
		add(filepath.Join(dir, "config.lua"))
	}
	for _, path := range loaded {
		add(path)
	}
	return paths
}

// IsLuaPath reports whether a config path selects a Lua file.
func IsLuaPath(path string) bool {
	return filepath.Ext(path) == ".lua"
}
