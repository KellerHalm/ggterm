package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"

	"ggterm/internal/config"
	"ggterm/internal/daemon"
	"ggterm/internal/mux"
	"ggterm/internal/session"
	"ggterm/internal/ui"
	"ggterm/internal/version"
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
	layoutFile := flag.String("layout-file", "", "layout file path (default: session file in the config directory)")

	sessionName := flag.String("session", daemon.DefaultSession, "session name")
	noDaemon := flag.Bool("no-daemon", false, "run without a background daemon (detach is unavailable)")
	listSessions := flag.Bool("list", false, "list running sessions and exit")
	killSession := flag.Bool("kill", false, "terminate the selected session and exit")
	daemonMode := flag.Bool("daemon", false, "run as a session daemon (internal)")
	socketFlag := flag.String("socket", "", "daemon endpoint (internal)")
	configPath := flag.String("config", "", "config file (default: ggterm/config.toml in the config directory)")
	noConfig := flag.Bool("no-config", false, "ignore the config file and use built-in defaults")
	pluginsDir := flag.String("plugins-dir", "", "plugin directory (default: ggterm/plugins in the config directory)")
	noPlugins := flag.Bool("no-plugins", false, "disable Lua plugins")
	printVersion := flag.Bool("version", false, "print the ggterm version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Println("ggterm " + version.String())
		return nil
	}

	setFlags := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	overrides := config.Overrides{}
	if setFlags["session"] {
		overrides.Session = sessionName
	}
	if setFlags["shell"] {
		overrides.Shell = shell
	}
	if setFlags["cols"] {
		overrides.Cols = cols
	}
	if setFlags["rows"] {
		overrides.Rows = rows
	}
	if setFlags["scrollback"] {
		overrides.Scrollback = scrollback
	}
	if setFlags["restore"] {
		overrides.Restore = restore
	}
	if setFlags["font-size"] {
		overrides.FontSize = fontSize
	}
	if setFlags["plugins-dir"] {
		overrides.PluginsDir = pluginsDir
	}
	if setFlags["no-plugins"] {
		overrides.NoPlugins = noPlugins
	}

	loader := config.Loader{Path: *configPath, Disable: *noConfig, Overrides: overrides}
	result, err := loader.Load()
	if err != nil {
		return err
	}
	settings := result.Settings

	if err := daemon.ValidateSessionName(settings.Session); err != nil {
		return err
	}

	if *listSessions {
		return printSessions()
	}

	socket := *socketFlag
	if socket == "" {
		path, err := daemon.SocketPath(settings.Session)
		if err != nil {
			return err
		}
		socket = path
	}
	path := *layoutFile
	if path == "" {
		defaultPath, err := daemon.LayoutPath(settings.Session)
		if err != nil {
			return err
		}
		path = defaultPath
	}

	if *killSession {
		return kill(settings.Session, socket, path)
	}

	cfg := mux.Config{
		Cols:          settings.Cols,
		Rows:          settings.Rows,
		MaxScrollback: settings.Scrollback,
		Command:       settings.Shell,
	}

	if *daemonMode {
		return runDaemon(settings.Session, socket, path, cfg, settings.Restore)
	}

	watcher := config.NewWatcher(loader, result)
	uiOpts := ui.Options{Settings: settings, Reload: watcher.Poll}
	if *noDaemon {
		return runLocal(cfg, path, settings.Restore, uiOpts)
	}
	return runAttached(settings.Session, socket, path, cfg, settings.Restore, uiOpts)
}

func runDaemon(name, socket, layoutPath string, cfg mux.Config, restore bool) error {
	logger := log.New(os.Stderr, "ggterm daemon "+name+": ", log.LstdFlags)
	workspace, err := restoreOrNew(layoutPath, cfg, restore, logger.Printf)
	if err != nil {
		return err
	}

	server, err := daemon.New(daemon.Options{
		Session:    name,
		Mux:        workspace,
		LayoutPath: layoutPath,
		Logger:     logger,
	})
	if err != nil {
		workspace.Shutdown()
		return err
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		server.Stop()
	}()

	err = server.ListenAndServe(socket)
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		workspace.Shutdown()
		return nil
	}
	server.Stop()
	return err
}

func runLocal(cfg mux.Config, layoutPath string, restore bool, uiOpts ui.Options) error {
	workspace, err := restoreOrNew(layoutPath, cfg, restore, func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "ggterm: "+format+"\n", args...)
	})
	if err != nil {
		return err
	}

	runErr := ui.Run(session.NewLocal(workspace), uiOpts)
	if errors.Is(runErr, ui.ErrDetached) {
		runErr = nil
	}

	var saveErr error
	if layoutPath != "" {
		if err := mux.Save(layoutPath, workspace); err != nil {
			saveErr = fmt.Errorf("save layout: %w", err)
		}
	}
	workspace.Shutdown()
	return errors.Join(runErr, saveErr)
}

func runAttached(name, socket, layoutPath string, cfg mux.Config, restore bool, uiOpts ui.Options) error {
	client, err := attachOrSpawn(name, socket, layoutPath, cfg, restore)
	if err != nil {
		return err
	}

	runErr := ui.Run(client, uiOpts)
	_ = client.Close()
	if errors.Is(runErr, ui.ErrDetached) {
		return nil
	}
	return runErr
}

func attachOrSpawn(name, socket, layoutPath string, cfg mux.Config, restore bool) (*daemon.Client, error) {
	client, err := daemon.Attach(socket, cfg.Cols, cfg.Rows)
	if err == nil {
		return client, nil
	}

	if daemon.SocketAlive(socket) {
		if _, statusErr := daemon.FetchStatus(socket); errors.Is(statusErr, daemon.ErrUnsupported) {
			if killErr := daemon.Kill(socket); killErr != nil {
				return nil, fmt.Errorf("terminate incompatible session %q: %w", name, killErr)
			}
			if err := daemon.WaitForSocketClosed(socket, 3*time.Second); err != nil {
				return nil, fmt.Errorf("session %q uses an incompatible daemon: %w", name, err)
			}
		}
	}

	if !daemon.SocketAlive(socket) {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			return nil, exeErr
		}
		logPath, _ := daemon.LogPath()
		dir, _ := os.Getwd()
		args := daemonCommand(name, socket, layoutPath, cfg, restore)
		if spawnErr := daemon.Spawn(exe, args, logPath, dir); spawnErr != nil {
			return nil, spawnErr
		}
		if err := daemon.WaitForSocket(socket, 8*time.Second); err != nil {
			return nil, fmt.Errorf("start session %q: %w", name, err)
		}
	}

	for attempt := 0; attempt < 30; attempt++ {
		client, err = daemon.Attach(socket, cfg.Cols, cfg.Rows)
		if err == nil {
			return client, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("attach to session %q: %w", name, err)
}

func daemonCommand(name, socket, layoutPath string, cfg mux.Config, restore bool) []string {
	args := []string{
		"--daemon",
		"--session", name,
		"--socket", socket,
		"--layout-file", layoutPath,
		"--cols", strconv.Itoa(cfg.Cols),
		"--rows", strconv.Itoa(cfg.Rows),
		"--scrollback", strconv.Itoa(cfg.MaxScrollback),
		"--restore=" + strconv.FormatBool(restore),
	}
	if cfg.Command != "" {
		args = append(args, "--shell", cfg.Command)
	}
	return args
}

func restoreOrNew(layoutPath string, cfg mux.Config, restore bool, logf func(string, ...any)) (*mux.Mux, error) {
	if restore && layoutPath != "" {
		workspace, err := mux.Restore(layoutPath, cfg)
		if err == nil {
			return workspace, nil
		}
		if !os.IsNotExist(err) && logf != nil {
			logf("restore layout: %v", err)
		}
	}
	workspace, err := mux.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("start terminal session: %w", err)
	}
	return workspace, nil
}

func printSessions() error {
	statuses, err := daemon.ListStatuses()
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("no running sessions")
		return nil
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tPID\tSIZE\tTABS\tPANES\tATTACHED")
	for _, status := range statuses {
		attached := "no"
		if status.Attached {
			attached = "yes"
		}
		fmt.Fprintf(writer, "%s\t%d\t%dx%d\t%d\t%d\t%s\n",
			status.Session, status.PID, status.Cols, status.Rows,
			status.Tabs, status.Panes, attached)
	}
	return writer.Flush()
}

func kill(name, socket, layoutPath string) error {
	if err := daemon.Kill(socket); err != nil {
		fmt.Printf("session %q is not running\n", name)
		return nil
	}
	if layoutPath != "" {
		_ = mux.Remove(layoutPath)
	}
	fmt.Printf("session %q terminated\n", name)
	return nil
}
