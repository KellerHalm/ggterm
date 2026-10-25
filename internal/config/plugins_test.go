package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoverPluginFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "alpha.lua"), "")
	writeFile(t, filepath.Join(dir, "beta.lua"), "")
	writeFile(t, filepath.Join(dir, "notes.txt"), "")
	writeFile(t, filepath.Join(dir, "nested", "gamma.lua"), "")

	names := func(files []string) []string {
		var result []string
		for _, file := range files {
			result = append(result, filepath.Base(file))
		}
		return result
	}

	files := DiscoverPluginFiles(Plugins{Dir: dir})
	if got := names(files); len(got) != 2 || got[0] != "alpha.lua" || got[1] != "beta.lua" {
		t.Errorf("files = %v", got)
	}

	files = DiscoverPluginFiles(Plugins{Dir: dir, Enabled: []string{"beta"}})
	if got := names(files); len(got) != 1 || got[0] != "beta.lua" {
		t.Errorf("enabled files = %v", got)
	}

	files = DiscoverPluginFiles(Plugins{Dir: dir, Disabled: []string{"alpha.lua"}})
	if got := names(files); len(got) != 1 || got[0] != "beta.lua" {
		t.Errorf("disabled files = %v", got)
	}

	if files := DiscoverPluginFiles(Plugins{Dir: dir, Off: true}); len(files) != 0 {
		t.Errorf("off files = %v", names(files))
	}
	if files := DiscoverPluginFiles(Plugins{Dir: filepath.Join(dir, "missing")}); len(files) != 0 {
		t.Errorf("missing dir files = %v", names(files))
	}
}

func TestLoadTOMLPlugins(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	writeFile(t, configPath, `
[plugins]
dir = "myplugins"
enabled = ["clock"]
disabled = ["noisy"]
`)

	result, err := Loader{Path: configPath}.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(dir, "myplugins")
	if result.Settings.Plugins.Dir != wantDir {
		t.Errorf("dir = %q, want %q", result.Settings.Plugins.Dir, wantDir)
	}
	if len(result.Settings.Plugins.Enabled) != 1 || result.Settings.Plugins.Enabled[0] != "clock" {
		t.Errorf("enabled = %v", result.Settings.Plugins.Enabled)
	}
	if len(result.Settings.Plugins.Disabled) != 1 || result.Settings.Plugins.Disabled[0] != "noisy" {
		t.Errorf("disabled = %v", result.Settings.Plugins.Disabled)
	}
}

func TestLoadLuaPlugins(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "init.lua")
	writeFile(t, configPath, `
ggterm.plugin_dir("lua-plugins")
ggterm.enable_plugin("clock")
ggterm.disable_plugin("noisy")
`)

	result, err := Loader{Path: configPath}.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(dir, "lua-plugins")
	if result.Settings.Plugins.Dir != wantDir {
		t.Errorf("dir = %q, want %q", result.Settings.Plugins.Dir, wantDir)
	}
	if len(result.Settings.Plugins.Enabled) != 1 || result.Settings.Plugins.Enabled[0] != "clock" {
		t.Errorf("enabled = %v", result.Settings.Plugins.Enabled)
	}
	if len(result.Settings.Plugins.Disabled) != 1 || result.Settings.Plugins.Disabled[0] != "noisy" {
		t.Errorf("disabled = %v", result.Settings.Plugins.Disabled)
	}
}

func TestPluginOverrides(t *testing.T) {
	noPlugins := true
	result, err := Loader{Disable: true, Overrides: Overrides{NoPlugins: &noPlugins}}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Settings.Plugins.Off {
		t.Error("plugins are not disabled")
	}

	customDir := filepath.Join(t.TempDir(), "custom")
	result, err = Loader{Disable: true, Overrides: Overrides{PluginsDir: &customDir}}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.Settings.Plugins.Dir != customDir {
		t.Errorf("dir = %q", result.Settings.Plugins.Dir)
	}
}

func TestWatcherSeesPluginChanges(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	pluginDir := filepath.Join(dir, "plugins")
	writeFile(t, configPath, "[plugins]\ndir = \"plugins\"\n")
	writeFile(t, filepath.Join(pluginDir, "one.lua"), "ggterm.command(\"one\", function() end)")

	loader := Loader{Path: configPath}
	result, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	watcher := NewWatcher(loader, result)

	time.Sleep(10 * time.Millisecond)
	writeFile(t, filepath.Join(pluginDir, "two.lua"), "ggterm.command(\"two\", function() end)")
	if _, changed, err := watcher.Poll(); err != nil || !changed {
		t.Fatalf("poll = %v %v, want a change", changed, err)
	}
}
