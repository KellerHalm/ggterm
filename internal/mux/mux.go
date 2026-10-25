// Package mux implements the multiplexer layer: panes, tabs and their layout.
//
// The package owns the terminal buffers and pseudoterminal processes of all
// panes. It has no dependency on the UI, which makes the layout logic testable
// in isolation.
package mux

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ggterm/internal/pty"
	"ggterm/internal/terminal"
)

// ErrTooSmall is returned when the active pane cannot be split any further.
var ErrTooSmall = errors.New("mux: pane too small to split")

const (
	minSplitCols = 8
	minSplitRows = 6

	shutdownWait = 2 * time.Second
)

// Config controls how new panes are created.
type Config struct {
	Cols          int
	Rows          int
	MaxScrollback int
	Command       string
	Dir           string

	// Start overrides process creation. It is used by tests; nil starts real
	// pseudoterminal sessions.
	Start func(StartOptions) (pty.Session, error)
}

type starter func(StartOptions) (pty.Session, error)

func defaultStart(opts StartOptions) (pty.Session, error) {
	return pty.Start(pty.Options{
		Command: opts.Command,
		Dir:     opts.Dir,
		Cols:    opts.Cols,
		Rows:    opts.Rows,
	})
}

// PaneView is the read-only pane surface rendered by the UI. It is implemented
// by local panes and by remote panes attached to a session daemon.
type PaneView interface {
	ID() int
	Snapshot() terminal.Snapshot
	Scroll(delta int)
	ScrollTo(offset int)
	ResetScroll()
	Title() string
	Dir() string
	Alive() bool
	Revision() uint64
	ScrollOffset() int
	Resize(cols, rows int) error
}

// Tab is a named group of panes with its own layout tree.
type Tab struct {
	ID     int
	Root   *Node
	Active *Pane
}

func (t *Tab) title() string {
	if t == nil {
		return ""
	}
	if t.Active != nil {
		if title := t.Active.Title(); title != "" {
			return title
		}
	}
	if t.Root != nil {
		for _, pane := range t.Root.panes(nil) {
			if title := pane.Title(); title != "" {
				return title
			}
		}
	}
	if t.Active != nil {
		return commandLabel(t.Active.Command())
	}
	return "shell"
}

// TabInfo is a UI-facing summary of one tab.
type TabInfo struct {
	ID     int
	Index  int
	Title  string
	Active bool
}

// View is a consistent description of the multiplexer for one frame.
type View struct {
	Cols, Rows int
	ActiveTab  int
	Tabs       []TabInfo
	Panes      []PaneLayout
	Broadcast  bool
}

// ActivePane returns the currently focused pane, or nil.
func (v View) ActivePane() PaneView {
	for _, layout := range v.Panes {
		if layout.Active {
			return layout.Pane
		}
	}
	return nil
}

// Mux owns tabs and panes.
type Mux struct {
	mu     sync.Mutex
	cfg    Config
	start  starter
	tabs   []*Tab
	active int

	cols, rows int
	broadcast  bool
	nextPaneID int
	nextTabID  int

	emptyCh   chan struct{}
	emptyOnce sync.Once
	closed    bool
}

// New creates a multiplexer with a single tab and pane.
func New(cfg Config) (*Mux, error) {
	m := newMux(cfg)
	if _, err := m.newTabLocked(m.cfg.Dir, m.cfg.Command); err != nil {
		return nil, err
	}
	return m, nil
}

func newMux(cfg Config) *Mux {
	if cfg.Cols <= 0 {
		cfg.Cols = 80
	}
	if cfg.Rows <= 0 {
		cfg.Rows = 24
	}
	if cfg.Dir == "" {
		if cwd, err := os.Getwd(); err == nil {
			cfg.Dir = cwd
		}
	}
	start := cfg.Start
	if start == nil {
		start = defaultStart
	}
	return &Mux{
		cfg:     cfg,
		start:   start,
		cols:    cfg.Cols,
		rows:    cfg.Rows,
		emptyCh: make(chan struct{}),
	}
}

// Empty reports whether all panes and tabs have been closed.
func (m *Mux) Empty() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tabs) == 0
}

// Stats returns the number of tabs and the total number of live panes.
func (m *Mux) Stats() (tabs, panes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tab := range m.tabs {
		if tab.Root != nil {
			panes += len(tab.Root.panes(nil))
		}
	}
	return len(m.tabs), panes
}

// Prune removes panes whose process has exited and drops empty tabs.
func (m *Mux) Prune() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneLocked()
}

// EmptyChan is closed when the last pane of the last tab goes away.
func (m *Mux) EmptyChan() <-chan struct{} {
	return m.emptyCh
}

// Cells returns the last known content viewport size in cells.
func (m *Mux) Cells() (cols, rows int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cols, m.rows
}

func (m *Mux) newPaneLocked(dir, command string, cols, rows int) (*Pane, error) {
	m.nextPaneID++
	return newPane(paneOptions{
		id:         m.nextPaneID,
		command:    command,
		dir:        dir,
		cols:       cols,
		rows:       rows,
		scrollback: m.cfg.MaxScrollback,
		start:      m.start,
	})
}

func (m *Mux) newTabLocked(dir, command string) (*Tab, error) {
	cols, rows := m.cols, m.rows
	if cols <= 0 {
		cols = m.cfg.Cols
	}
	if rows <= 0 {
		rows = m.cfg.Rows
	}
	pane, err := m.newPaneLocked(dir, command, cols, rows)
	if err != nil {
		return nil, err
	}
	m.nextTabID++
	tab := &Tab{ID: m.nextTabID, Root: leaf(pane), Active: pane}
	m.tabs = append(m.tabs, tab)
	m.active = len(m.tabs) - 1
	return tab, nil
}

// NewTab appends a tab and makes it active. New panes inherit the working
// directory of the current pane.
func (m *Mux) NewTab() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	dir := m.cfg.Dir
	if current := m.activePaneLocked(); current != nil {
		dir = current.Dir()
	}
	_, err := m.newTabLocked(dir, m.cfg.Command)
	return err
}

// CloseTab closes every pane in the active tab and removes the tab.
func (m *Mux) CloseTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil {
		return
	}
	m.removeTabLocked(tab)
}

// SelectTab activates a tab by zero-based index.
func (m *Mux) SelectTab(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if index < 0 || index >= len(m.tabs) {
		return
	}
	m.active = index
	tab := m.tabs[index]
	if tab.Active == nil {
		tab.Active = firstPane(tab.Root)
	}
}

// NextTab activates the next tab, wrapping around.
func (m *Mux) NextTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tabs) == 0 {
		return
	}
	m.active = (m.active + 1) % len(m.tabs)
}

// PrevTab activates the previous tab, wrapping around.
func (m *Mux) PrevTab() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tabs) == 0 {
		return
	}
	m.active--
	if m.active < 0 {
		m.active = len(m.tabs) - 1
	}
}

// Split divides the active pane in the given direction and focuses the new
// pane.
func (m *Mux) Split(dir Direction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	tab := m.activeTabLocked()
	if tab == nil || tab.Active == nil {
		return nil
	}
	active := tab.Active
	cols, rows := active.Size()
	if dir == Vertical && cols < minSplitCols {
		return ErrTooSmall
	}
	if dir == Horizontal && rows < minSplitRows {
		return ErrTooSmall
	}

	newCols, newRows := cols, rows
	if dir == Vertical {
		newCols = maxInt(1, cols/2)
	} else {
		newRows = maxInt(1, rows/2)
	}
	pane, err := m.newPaneLocked(active.Dir(), m.cfg.Command, newCols, newRows)
	if err != nil {
		return err
	}
	if !splitPane(tab.Root, active, pane, dir) {
		_ = pane.Close()
		return nil
	}
	tab.Active = pane
	return nil
}

// CloseActivePane terminates the focused pane, collapsing the layout.
func (m *Mux) CloseActivePane() {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil || tab.Active == nil {
		return
	}
	m.closePaneLocked(tab, tab.Active)
}

// FocusNext moves focus to the next pane in layout order, wrapping around.
func (m *Mux) FocusNext() {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil || tab.Root == nil {
		return
	}
	panes := tab.Root.panes(nil)
	if len(panes) < 2 {
		return
	}
	for i, pane := range panes {
		if pane == tab.Active {
			tab.Active = panes[(i+1)%len(panes)]
			return
		}
	}
	tab.Active = panes[0]
}

// FocusDirection moves focus to the nearest pane on the given side.
func (m *Mux) FocusDirection(dir MoveDirection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil || tab.Active == nil {
		return
	}
	panes := tab.Root.layout(Rect{W: m.cols, H: m.rows}, tab.Active, nil)
	next := findDirectional(panes, tab.Active, dir)
	if next == nil {
		return
	}
	for _, candidate := range tab.Root.panes(nil) {
		if candidate.ID() == next.ID() {
			tab.Active = candidate
			return
		}
	}
}

// FocusPaneID focuses the pane with the given ID if it belongs to the active
// tab.
func (m *Mux) FocusPaneID(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tab := m.activeTabLocked()
	if tab == nil || tab.Root == nil {
		return
	}
	for _, candidate := range tab.Root.panes(nil) {
		if candidate.ID() == id {
			tab.Active = candidate
			return
		}
	}
}

// Broadcast reports whether input is sent to every pane in the active tab.
func (m *Mux) Broadcast() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.broadcast
}

// SetBroadcast enables or disables input broadcasting.
func (m *Mux) SetBroadcast(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcast = enabled
}

// ToggleBroadcast flips input broadcasting and returns the new state.
func (m *Mux) ToggleBroadcast() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcast = !m.broadcast
	return m.broadcast
}

// WriteInput sends bytes to the active pane, or to every pane in the active
// tab when broadcasting is enabled.
func (m *Mux) WriteInput(data []byte) {
	if len(data) == 0 {
		return
	}
	m.mu.Lock()
	tab := m.activeTabLocked()
	if tab == nil {
		m.mu.Unlock()
		return
	}
	if m.broadcast {
		panes := tab.Root.panes(nil)
		m.mu.Unlock()
		for _, pane := range panes {
			_ = pane.Write(data)
		}
		return
	}
	pane := tab.Active
	m.mu.Unlock()
	if pane != nil {
		_ = pane.Write(data)
	}
}

// Viewport computes the current layout for the given content area and resizes
// panes whose content dimensions changed.
func (m *Mux) Viewport(cols, rows int) View {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cols, m.rows = cols, rows
	m.pruneLocked()

	view := View{Cols: cols, Rows: rows, Broadcast: m.broadcast, ActiveTab: m.active}
	if len(m.tabs) == 0 {
		return view
	}

	for i, tab := range m.tabs {
		view.Tabs = append(view.Tabs, TabInfo{
			ID:     tab.ID,
			Index:  i,
			Title:  tab.title(),
			Active: i == m.active,
		})
	}

	tab := m.activeTabLocked()
	if tab == nil || tab.Root == nil {
		return view
	}
	view.Panes = tab.Root.layout(Rect{W: cols, H: rows}, tab.Active, nil)
	for i := range view.Panes {
		content := view.Panes[i].Content
		if content.W < 1 {
			content.W = 1
		}
		if content.H < 1 {
			content.H = 1
		}
		view.Panes[i].Content = content
		_ = view.Panes[i].Pane.Resize(content.W, content.H)
	}
	return view
}

// SetCellSize propagates the frontend's character cell size to every pane.
// Images without an explicit size in cells use it for placement.
func (m *Mux) SetCellSize(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	m.mu.Lock()
	panes := m.allPanesLocked()
	m.mu.Unlock()
	for _, pane := range panes {
		pane.SetCellSize(width, height)
	}
}

func (m *Mux) allPanesLocked() []*Pane {
	var panes []*Pane
	for _, tab := range m.tabs {
		if tab.Root != nil {
			panes = tab.Root.panes(panes)
		}
	}
	return panes
}

// Shutdown terminates all panes and waits briefly for them to exit.
func (m *Mux) Shutdown() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	var panes []*Pane
	for _, tab := range m.tabs {
		if tab.Root != nil {
			panes = tab.Root.panes(panes)
		}
	}
	m.tabs = nil
	m.active = 0
	m.mu.Unlock()

	for _, pane := range panes {
		_ = pane.Close()
	}
	deadline := time.After(shutdownWait)
	for _, pane := range panes {
		select {
		case <-pane.Done():
		case <-deadline:
			return
		}
	}
}

func (m *Mux) activeTabLocked() *Tab {
	if m.active < 0 || m.active >= len(m.tabs) {
		return nil
	}
	return m.tabs[m.active]
}

func (m *Mux) activePaneLocked() *Pane {
	tab := m.activeTabLocked()
	if tab == nil {
		return nil
	}
	if tab.Active == nil {
		tab.Active = firstPane(tab.Root)
	}
	return tab.Active
}

func (m *Mux) closePaneLocked(tab *Tab, pane *Pane) {
	_ = pane.Close()
	root, removed := removePane(tab.Root, pane)
	if !removed {
		return
	}
	tab.Root = root
	if root == nil {
		m.removeTabLocked(tab)
		return
	}
	if tab.Active == pane {
		tab.Active = firstPane(root)
	}
}

func (m *Mux) removeTabLocked(tab *Tab) {
	index := -1
	for i, candidate := range m.tabs {
		if candidate == tab {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	if tab.Root != nil {
		for _, pane := range tab.Root.panes(nil) {
			_ = pane.Close()
		}
	}
	tab.Root = nil
	tab.Active = nil
	m.tabs = append(m.tabs[:index], m.tabs[index+1:]...)
	if m.active >= index {
		m.active--
	}
	if m.active < 0 {
		m.active = 0
	}
	if m.active >= len(m.tabs) {
		m.active = len(m.tabs) - 1
	}
	if len(m.tabs) == 0 {
		m.emptyOnce.Do(func() { close(m.emptyCh) })
	}
}

func (m *Mux) pruneLocked() {
	if len(m.tabs) == 0 {
		return
	}
	tabs := append([]*Tab(nil), m.tabs...)
	for _, tab := range tabs {
		if tab.Root == nil {
			m.removeTabLocked(tab)
			continue
		}
		var dead []*Pane
		for _, pane := range tab.Root.panes(nil) {
			if !pane.Alive() {
				dead = append(dead, pane)
			}
		}
		for _, pane := range dead {
			if tab.Root == nil {
				break
			}
			m.closePaneLocked(tab, pane)
		}
	}
}

func commandLabel(command string) string {
	if strings.TrimSpace(command) == "" {
		command = pty.DefaultShell()
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "shell"
	}
	name := filepath.Base(fields[0])
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "shell"
	}
	return name
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
