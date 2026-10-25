package ui

import (
	"errors"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"ggterm/internal/config"
	"ggterm/internal/mux"
	"ggterm/internal/session"
)

// ErrDetached is returned by Run when the user detaches from a background
// session instead of quitting it.
var ErrDetached = errors.New("ui: detached")

// Options controls the terminal window.
type Options struct {
	Settings config.Settings
	Reload   func() (config.Settings, bool, error)
	Done     <-chan struct{} // closes the window when the channel closes
}

const (
	minWindowW   = 420
	minWindowH   = 240
	cursorBlink  = 530 * time.Millisecond
	tickRate     = 60
	wheelPerStep = 3
)

type paneHit struct {
	pane   mux.PaneView
	x0, y0 int
	x1, y1 int
}

type tabHit struct {
	index int
	x0    int
	x1    int
}

type game struct {
	session session.Session
	face    text.Face
	fonts   *fontSet
	done    <-chan struct{}
	reload  func() (config.Settings, bool, error)

	settings config.Settings
	theme    config.Theme
	bindings []keyBinding
	prefix   keyBinding
	padding  int

	cellW, cellH float64
	ascent       float64

	width, height int
	view          mux.View
	contentRows   int
	showBars      bool

	wheelAccum float64
	lastTitle  string
	lastPoll   time.Time
	notice     string
	noticeTill time.Time

	prefixing bool
	showHelp  bool
	detached  bool

	chars []rune
	keys  []ebiten.Key

	paneHits []paneHit
	tabHits  []tabHit

	rects         rectBatch
	paneCache     map[int]*paneCacheEntry
	seenPanes     map[int]bool
	graphicCache  map[uint64]*graphicCacheEntry
	renderVersion uint64
}

// Run opens the multiplexer window and blocks until it is closed or all panes
// have exited. It returns ErrDetached when the user asked to leave the session
// running in the background.
func Run(s session.Session, opts Options) error {
	if s == nil {
		return errors.New("ui: nil session")
	}
	settings := opts.Settings
	if settings.Font.Size <= 0 {
		settings = config.Default()
	}
	g := &game{
		session: s,
		done:    opts.Done,
		reload:  opts.Reload,
	}
	if err := g.applySettings(settings, true); err != nil {
		return err
	}
	ebiten.SetWindowDecorated(settings.Decorated)

	cols, rows := s.Cells()
	g.view = s.Viewport(cols, rows)
	g.lastTitle = viewTitle(g.view)
	width := int(math.Ceil(float64(cols)*g.cellW)) + 2*g.padding
	height := int(math.Ceil(float64(rows+2)*g.cellH)) + 2*g.padding
	ebiten.SetWindowTitle(windowTitle(g.lastTitle))
	ebiten.SetWindowSize(clampInt(width, minWindowW, 4096), clampInt(height, minWindowH, 4096))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(tickRate)
	err := ebiten.RunGameWithOptions(g, &ebiten.RunGameOptions{
		ScreenTransparent: settings.Window.Transparent,
	})
	if g.detached && err == nil {
		return ErrDetached
	}
	return err
}

func (g *game) applySettings(settings config.Settings, initial bool) error {
	fonts, err := loadFontSet(settings.Font)
	if err != nil {
		return err
	}
	bindings, prefix, err := buildKeymap(settings.Keys)
	if err != nil {
		return err
	}
	padding := clampInt(settings.Padding, 0, 64)

	face := fonts.variants[fontRegular].face
	cellW := text.Advance("M", face)
	if cellW <= 0 {
		cellW = settings.Font.Size * 0.6
	}
	cellH := math.Ceil(fonts.metrics.HAscent + fonts.metrics.HDescent)
	if cellH < settings.Font.Size {
		cellH = math.Ceil(settings.Font.Size * 1.2)
	}

	g.clearPaneCache()
	g.renderVersion++
	g.settings = settings
	g.fonts = fonts
	g.face = face
	g.theme = settings.Theme
	g.bindings = bindings
	g.prefix = prefix
	g.padding = padding
	g.cellW = cellW
	g.cellH = cellH
	g.ascent = fonts.metrics.HAscent
	if g.session != nil {
		g.session.SetCellSize(
			clampInt(int(math.Ceil(cellW)), 1, 256),
			clampInt(int(math.Ceil(cellH)), 1, 256),
		)
	}
	if !initial {
		ebiten.SetWindowDecorated(settings.Decorated)
	}
	return nil
}

func (g *game) maybeReload() {
	if g.reload == nil {
		return
	}
	if time.Since(g.lastPoll) < 500*time.Millisecond {
		return
	}
	g.lastPoll = time.Now()

	settings, changed, err := g.reload()
	if err != nil {
		g.notify("config: "+err.Error(), 5*time.Second)
		return
	}
	if !changed {
		return
	}
	if err := g.applySettings(settings, false); err != nil {
		g.notify("config: "+err.Error(), 5*time.Second)
		return
	}
	g.notify("config reloaded", 2*time.Second)
}

func (g *game) notify(message string, duration time.Duration) {
	g.notice = message
	g.noticeTill = time.Now().Add(duration)
}

func (g *game) currentNotice() string {
	if g.notice == "" || time.Now().After(g.noticeTill) {
		return ""
	}
	return g.notice
}

func (g *game) Update() error {
	g.maybeReload()
	if g.done != nil {
		select {
		case <-g.done:
			return ebiten.Termination
		default:
		}
	}
	if g.width <= 2*g.padding || g.height <= 2*g.padding {
		return nil
	}
	cols := int((float64(g.width) - 2*float64(g.padding)) / g.cellW)
	totalRows := int((float64(g.height) - 2*float64(g.padding)) / g.cellH)
	cols = clampInt(cols, 1, 1000)
	totalRows = clampInt(totalRows, 1, 1000)

	g.showBars = totalRows >= 6
	g.contentRows = totalRows
	if g.showBars {
		g.contentRows = totalRows - 2
	}
	g.contentRows = clampInt(g.contentRows, 1, 1000)

	g.view = g.session.Viewport(cols, g.contentRows)
	if g.session.Empty() {
		return ebiten.Termination
	}

	if err := g.handleMouse(g.view); err != nil {
		return err
	}
	if err := g.handleInput(g.view); err != nil {
		return err
	}
	if err := g.handleWheel(g.view); err != nil {
		return err
	}

	title := viewTitle(g.view)
	if title != g.lastTitle {
		g.lastTitle = title
		ebiten.SetWindowTitle(windowTitle(title))
	}
	return nil
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.width = maxInt(outsideWidth, 1)
	g.height = maxInt(outsideHeight, 1)
	return g.width, g.height
}

func viewTitle(view mux.View) string {
	if view.ActiveTab < 0 || view.ActiveTab >= len(view.Tabs) {
		return ""
	}
	return view.Tabs[view.ActiveTab].Title
}

func (g *game) handleInput(view mux.View) error {
	mods := currentModifiers()
	consumedChars := false
	typed := false

	active := view.ActivePane()

	keys := inpututil.AppendJustPressedKeys(g.keys[:0])
	g.keys = keys
	for _, key := range keys {
		if isModifierKey(key) {
			continue
		}

		if g.prefixing {
			if g.isPrefixKey(key, mods) {
				g.session.WriteInput([]byte{0x02})
				g.prefixing = false
				consumedChars = true
				typed = true
				continue
			}
			if binding, ok := lookupBinding(g.bindings, key, mods); ok {
				g.prefixing = false
				consumedChars = true
				if g.runAction(binding, view) {
					return ebiten.Termination
				}
				continue
			}
			g.prefixing = false
		}

		if g.isPrefixKey(key, mods) {
			g.prefixing = true
			consumedChars = true
			continue
		}
		if active == nil {
			continue
		}

		switch key {
		case ebiten.KeyPageUp:
			snap := active.Snapshot()
			if mods.shift || !snap.AltScreen {
				active.Scroll(pageStep(snap.Rows))
				continue
			}
			g.session.WriteInput([]byte("\x1b[5~"))
			typed = true
			continue
		case ebiten.KeyPageDown:
			snap := active.Snapshot()
			if mods.shift || !snap.AltScreen {
				active.Scroll(-pageStep(snap.Rows))
				continue
			}
			g.session.WriteInput([]byte("\x1b[6~"))
			typed = true
			continue
		}

		sequence, ok := keySequence(key, mods)
		if !ok {
			continue
		}
		g.session.WriteInput(sequence)
		typed = true
	}

	if g.prefixing {
		consumedChars = true
	}
	if !consumedChars && active != nil {
		chars := ebiten.AppendInputChars(g.chars[:0])
		g.chars = chars
		if !mods.ctrl && !mods.alt {
			for _, r := range chars {
				if r == 0 {
					continue
				}
				g.session.WriteInput([]byte(string(r)))
				typed = true
			}
		}
	}

	if typed && active != nil {
		active.ResetScroll()
		g.wheelAccum = 0
	}
	return nil
}

func (g *game) handleWheel(view mux.View) error {
	_, y := ebiten.Wheel()
	if y == 0 {
		return nil
	}
	g.wheelAccum += y
	steps := int(g.wheelAccum)
	if steps == 0 {
		return nil
	}
	g.wheelAccum -= float64(steps)

	active := view.ActivePane()
	if active == nil {
		return nil
	}
	snap := active.Snapshot()
	if snap.AltScreen {
		sequence := []byte("\x1b[A")
		if steps < 0 {
			sequence = []byte("\x1b[B")
			steps = -steps
		}
		for i := 0; i < steps; i++ {
			g.session.WriteInput(sequence)
		}
		return nil
	}
	active.Scroll(steps * wheelPerStep)
	return nil
}

func (g *game) handleMouse(view mux.View) error {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	x, y := ebiten.CursorPosition()
	for _, hit := range g.tabHits {
		if x >= hit.x0 && x < hit.x1 && y >= g.padding && y < g.padding+int(g.cellH) {
			g.session.SelectTab(hit.index)
			return nil
		}
	}
	for _, hit := range g.paneHits {
		if x >= hit.x0 && x < hit.x1 && y >= hit.y0 && y < hit.y1 {
			g.session.FocusPane(hit.pane.ID())
			return nil
		}
	}
	return nil
}

func (g *game) runAction(binding keyBinding, view mux.View) bool {
	switch binding.action {
	case actionSplitVertical:
		_ = g.session.Split(mux.Vertical)
	case actionSplitHorizontal:
		_ = g.session.Split(mux.Horizontal)
	case actionClosePane:
		g.session.CloseActivePane()
	case actionNewTab:
		_ = g.session.NewTab()
	case actionCloseTab:
		g.session.CloseTab()
	case actionNextTab:
		g.session.NextTab()
	case actionPrevTab:
		g.session.PrevTab()
	case actionSelectTab:
		g.session.SelectTab(binding.arg)
	case actionFocusNext:
		g.session.FocusNext()
	case actionFocusLeft:
		g.session.FocusDirection(mux.MoveLeft)
	case actionFocusRight:
		g.session.FocusDirection(mux.MoveRight)
	case actionFocusUp:
		g.session.FocusDirection(mux.MoveUp)
	case actionFocusDown:
		g.session.FocusDirection(mux.MoveDown)
	case actionToggleBroadcast:
		g.session.ToggleBroadcast()
	case actionHelp:
		g.showHelp = !g.showHelp
	case actionFullscreen:
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	case actionDetach:
		if g.session.Detach() {
			g.detached = true
			return true
		}
	case actionQuit:
		g.session.Quit()
		return true
	}
	return false
}

func isModifierKey(key ebiten.Key) bool {
	switch key {
	case ebiten.KeyControl, ebiten.KeyControlLeft, ebiten.KeyControlRight,
		ebiten.KeyAlt, ebiten.KeyAltLeft, ebiten.KeyAltRight,
		ebiten.KeyShift, ebiten.KeyShiftLeft, ebiten.KeyShiftRight:
		return true
	}
	return false
}
