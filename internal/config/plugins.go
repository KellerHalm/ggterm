package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Plugins controls discovery of Lua plugins.
type Plugins struct {
	// Dir is the directory scanned for *.lua files. An empty value resolves to
	// <config dir>/plugins.
	Dir string

	// Enabled limits loading to the named plugins (file name without .lua).
	// An empty list loads every plugin in Dir.
	Enabled []string

	// Disabled excludes the named plugins even when they are present.
	Disabled []string

	// Off disables plugin loading entirely (set by --no-plugins).
	Off bool
}

// DefaultPluginDir returns the user plugin directory.
func DefaultPluginDir() string {
	dir, err := ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "plugins")
}

// DiscoverPluginFiles returns the plugin files selected by p, sorted by name.
// Missing directories yield no files and no error; discovery is best effort so
// that startup is not blocked by file system problems.
func DiscoverPluginFiles(p Plugins) []string {
	if p.Off {
		return nil
	}
	dir := p.Dir
	if dir == "" {
		dir = DefaultPluginDir()
	}
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	enabled := make(map[string]bool, len(p.Enabled))
	for _, name := range p.Enabled {
		enabled[strings.TrimSuffix(name, filepath.Ext(name))] = true
	}
	disabled := make(map[string]bool, len(p.Disabled))
	for _, name := range p.Disabled {
		disabled[strings.TrimSuffix(name, filepath.Ext(name))] = true
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".lua") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if len(enabled) > 0 && !enabled[name] {
			continue
		}
		if disabled[name] {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files
}

func resolvePlugins(settings *Settings, dir string) {
	if strings.TrimSpace(settings.Plugins.Dir) == "" {
		settings.Plugins.Dir = DefaultPluginDir()
		return
	}
	settings.Plugins.Dir = resolvePath(dir, settings.Plugins.Dir)
}
