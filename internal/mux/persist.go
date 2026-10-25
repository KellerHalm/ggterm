package mux

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LayoutVersion is the current on-disk layout format version.
const LayoutVersion = 1

type layoutState struct {
	Version   int        `json:"version"`
	ActiveTab int        `json:"active_tab"`
	Tabs      []tabState `json:"tabs"`
}

type tabState struct {
	Root       *nodeState `json:"root"`
	ActivePane int        `json:"active_pane"`
}

type nodeState struct {
	Pane      *paneState `json:"pane,omitempty"`
	Direction string     `json:"direction,omitempty"`
	Ratio     float64    `json:"ratio,omitempty"`
	First     *nodeState `json:"first,omitempty"`
	Second    *nodeState `json:"second,omitempty"`
}

type paneState struct {
	Command string `json:"command,omitempty"`
	Dir     string `json:"dir,omitempty"`
}

// Save writes the current tabs and split tree to path. An empty multiplexer
// removes the file instead, so a clean exit does not leave a stale layout.
func Save(path string, m *Mux) error {
	if path == "" {
		return errors.New("mux: empty layout path")
	}
	state := m.exportLayout()
	if len(state.Tabs) == 0 {
		return Remove(path)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode layout: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create layout directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write layout: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace layout: %w", err)
	}
	return nil
}

// Remove deletes a saved layout, ignoring a missing file.
func Remove(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Restore rebuilds tabs and panes from a layout file. Processes are started
// afresh; terminal contents and scrollback are not persisted.
func Restore(path string, cfg Config) (*Mux, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state layoutState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode layout: %w", err)
	}
	if state.Version != LayoutVersion {
		return nil, fmt.Errorf("unsupported layout version %d", state.Version)
	}
	if len(state.Tabs) == 0 {
		return nil, errors.New("layout contains no tabs")
	}

	m := newMux(cfg)
	builder := &restoreBuilder{mux: m}
	for _, tab := range state.Tabs {
		if tab.Root == nil {
			builder.abort()
			return nil, errors.New("layout tab has no root node")
		}
		root, err := builder.build(tab.Root, cfg)
		if err != nil {
			builder.abort()
			return nil, err
		}
		panes := root.panes(nil)
		if len(panes) == 0 {
			builder.abort()
			return nil, errors.New("layout tab contains no panes")
		}
		active := tab.ActivePane
		if active < 0 || active >= len(panes) {
			active = 0
		}
		m.nextTabID++
		m.tabs = append(m.tabs, &Tab{
			ID:     m.nextTabID,
			Root:   root,
			Active: panes[active],
		})
	}
	m.active = state.ActiveTab
	if m.active < 0 || m.active >= len(m.tabs) {
		m.active = 0
	}
	return m, nil
}

func (m *Mux) exportLayout() layoutState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := layoutState{Version: LayoutVersion, ActiveTab: m.active}
	if state.ActiveTab < 0 {
		state.ActiveTab = 0
	}
	for _, tab := range m.tabs {
		if tab.Root == nil {
			continue
		}
		panes := tab.Root.panes(nil)
		active := 0
		for i, pane := range panes {
			if pane == tab.Active {
				active = i
				break
			}
		}
		state.Tabs = append(state.Tabs, tabState{
			Root:       exportNode(tab.Root),
			ActivePane: active,
		})
	}
	return state
}

func exportNode(n *Node) *nodeState {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return &nodeState{Pane: &paneState{
			Command: n.Pane.Command(),
			Dir:     n.Pane.Dir(),
		}}
	}
	return &nodeState{
		Direction: directionName(n.Dir),
		Ratio:     n.Ratio,
		First:     exportNode(n.First),
		Second:    exportNode(n.Second),
	}
}

func directionName(dir Direction) string {
	if dir == Horizontal {
		return "horizontal"
	}
	return "vertical"
}

func parseDirection(value string) (Direction, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "vertical", "v":
		return Vertical, true
	case "horizontal", "h":
		return Horizontal, true
	}
	return Vertical, false
}

type restoreBuilder struct {
	mux   *Mux
	panes []*Pane
}

func (b *restoreBuilder) build(n *nodeState, cfg Config) (*Node, error) {
	if n == nil {
		return nil, errors.New("layout split node is missing a child")
	}
	if n.Pane != nil {
		if n.First != nil || n.Second != nil {
			return nil, errors.New("layout pane node also has children")
		}
		pane, err := b.newPane(n.Pane, cfg)
		if err != nil {
			return nil, err
		}
		b.panes = append(b.panes, pane)
		return leaf(pane), nil
	}
	if n.First == nil || n.Second == nil {
		return nil, errors.New("layout split node is missing a child")
	}
	dir, ok := parseDirection(n.Direction)
	if !ok {
		return nil, fmt.Errorf("unknown split direction %q", n.Direction)
	}
	first, err := b.build(n.First, cfg)
	if err != nil {
		return nil, err
	}
	second, err := b.build(n.Second, cfg)
	if err != nil {
		return nil, err
	}
	ratio := n.Ratio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	return &Node{Dir: dir, Ratio: ratio, First: first, Second: second}, nil
}

func (b *restoreBuilder) newPane(state *paneState, cfg Config) (*Pane, error) {
	dir := state.Dir
	if !isDirectory(dir) {
		dir = cfg.Dir
	}
	command := state.Command
	if cfg.Command != "" {
		command = cfg.Command
	}
	cols, rows := b.mux.cols, b.mux.rows
	if cols <= 0 {
		cols = cfg.Cols
	}
	if rows <= 0 {
		rows = cfg.Rows
	}
	return b.mux.newPaneLocked(dir, command, cols, rows)
}

func (b *restoreBuilder) abort() {
	for _, pane := range b.panes {
		_ = pane.Close()
	}
}

func isDirectory(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
