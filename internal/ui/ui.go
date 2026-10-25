package ui

import (
	"errors"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"ggterm/internal/mux"
)

// Options controls the terminal window.
type Options struct {
	FontSize float64         // <=0 => 15
	Done     <-chan struct{} // closes the window when the channel closes
}

const (
	padding      = 8
	minWindowW   = 420
	minWindowH   = 240
	cursorBlink  = 530 * time.Millisecond
	defaultFont  = 15.0
	tickRate     = 60
	wheelPerStep = 3
)

type paneHit struct {
	pane   *mux.Pane
	x0, y0 int
	x1, y1 int
}

type tabHit struct {
	index int
	x0    int
	x1    int
}

type game struct {
	mux  *mux.Mux
	face *text.GoTextFace
	done <-chan struct{}

	cellW, cellH float64
	ascent       float64

	width, height int
	view          mux.View
	contentRows   int
	showBars      bool

	scroll     map[int]int
	wheelAccum float64
	lastTitle  string

	prefix   bool
	showHelp bool

	chars []rune
	keys  []ebiten.Key

	paneHits []paneHit
	tabHits  []tabHit
}

// Run opens the multiplexer window and blocks until it is closed or all panes
// have exited.
func Run(m *mux.Mux, opts Options) error {
	if m == nil {
		return errors.New("ui: nil multiplexer")
	}
	size := opts.FontSize
	if size <= 0 {
		size = defaultFont
	}
	face, metrics, err := newFace(size)
	if err != nil {
		return err
	}
	g := &game{
		mux:    m,
		face:   face,
		done:   opts.Done,
		ascent: metrics.HAscent,
		scroll: make(map[int]int),
	}
	g.cellW = text.Advance("M", face)
	if g.cellW <= 0 {
		g.cellW = size * 0.6
	}
	g.cellH = math.Ceil(metrics.HAscent + metrics.HDescent)
	if g.cellH < size {
		g.cellH = math.Ceil(size * 1.2)
	}

	cols, rows := m.Cells()
	g.view = m.Viewport(cols, rows)
	g.lastTitle = viewTitle(g.view)
	width := int(math.Ceil(float64(cols)*g.cellW)) + 2*padding
	height := int(math.Ceil(float64(rows+2)*g.cellH)) + 2*padding
	ebiten.SetWindowTitle(windowTitle(g.lastTitle))
	ebiten.SetWindowSize(clampInt(width, minWindowW, 4096), clampInt(height, minWindowH, 4096))
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(tickRate)
	return ebiten.RunGame(g)
}

func (g *game) Update() error {
	if g.done != nil {
		select {
		case <-g.done:
			return ebiten.Termination
		default:
		}
	}
	if g.width <= 2*padding || g.height <= 2*padding {
		return nil
	}
	cols := int((float64(g.width) - 2*padding) / g.cellW)
	totalRows := int((float64(g.height) - 2*padding) / g.cellH)
	cols = clampInt(cols, 1, 1000)
	totalRows = clampInt(totalRows, 1, 1000)

	g.showBars = totalRows >= 6
	g.contentRows = totalRows
	if g.showBars {
		g.contentRows = totalRows - 2
	}
	g.contentRows = clampInt(g.contentRows, 1, 1000)

	g.view = g.mux.Viewport(cols, g.contentRows)
	if g.mux.Empty() {
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

		if g.prefix {
			if isPrefixKey(key, mods) {
				g.mux.WriteInput([]byte{0x02})
				g.prefix = false
				consumedChars = true
				typed = true
				continue
			}
			if binding, ok := lookupBinding(key, mods); ok {
				g.prefix = false
				consumedChars = true
				if g.runAction(binding, view) {
					return ebiten.Termination
				}
				continue
			}
			g.prefix = false
		}

		if isPrefixKey(key, mods) {
			g.prefix = true
			consumedChars = true
			continue
		}
		if active == nil {
			continue
		}

		switch key {
		case ebiten.KeyPageUp:
			snap := active.Snapshot(g.scrollOffset(active.ID()))
			if mods.shift || !snap.AltScreen {
				offset := g.scrollOffset(active.ID()) + pageStep(snap.Rows)
				g.setScroll(active, offset, snap.ScrollbackLen)
				continue
			}
			g.mux.WriteInput([]byte("\x1b[5~"))
			typed = true
			continue
		case ebiten.KeyPageDown:
			snap := active.Snapshot(g.scrollOffset(active.ID()))
			if mods.shift || !snap.AltScreen {
				offset := g.scrollOffset(active.ID()) - pageStep(snap.Rows)
				g.setScroll(active, offset, snap.ScrollbackLen)
				continue
			}
			g.mux.WriteInput([]byte("\x1b[6~"))
			typed = true
			continue
		}

		sequence, ok := keySequence(key, mods)
		if !ok {
			continue
		}
		g.mux.WriteInput(sequence)
		typed = true
	}

	if g.prefix {
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
				g.mux.WriteInput([]byte(string(r)))
				typed = true
			}
		}
	}

	if typed && active != nil {
		g.setScroll(active, 0, 0)
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
	snap := active.Snapshot(g.scrollOffset(active.ID()))
	if snap.AltScreen {
		sequence := []byte("\x1b[A")
		if steps < 0 {
			sequence = []byte("\x1b[B")
			steps = -steps
		}
		for i := 0; i < steps; i++ {
			g.mux.WriteInput(sequence)
		}
		return nil
	}
	offset := g.scrollOffset(active.ID()) + steps*wheelPerStep
	g.setScroll(active, offset, snap.ScrollbackLen)
	return nil
}

func (g *game) handleMouse(view mux.View) error {
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return nil
	}
	x, y := ebiten.CursorPosition()
	for _, hit := range g.tabHits {
		if x >= hit.x0 && x < hit.x1 && y >= padding && y < padding+int(g.cellH) {
			g.mux.SelectTab(hit.index)
			return nil
		}
	}
	for _, hit := range g.paneHits {
		if x >= hit.x0 && x < hit.x1 && y >= hit.y0 && y < hit.y1 {
			g.mux.FocusPane(hit.pane)
			return nil
		}
	}
	return nil
}

func (g *game) runAction(binding keyBinding, view mux.View) bool {
	switch binding.action {
	case actionSplitVertical:
		_ = g.mux.Split(mux.Vertical)
	case actionSplitHorizontal:
		_ = g.mux.Split(mux.Horizontal)
	case actionClosePane:
		g.mux.CloseActivePane()
	case actionNewTab:
		_ = g.mux.NewTab()
	case actionCloseTab:
		g.mux.CloseTab()
	case actionNextTab:
		g.mux.NextTab()
	case actionPrevTab:
		g.mux.PrevTab()
	case actionSelectTab:
		g.mux.SelectTab(binding.arg)
	case actionFocusNext:
		g.mux.FocusNext()
	case actionFocusLeft:
		g.mux.FocusDirection(mux.MoveLeft)
	case actionFocusRight:
		g.mux.FocusDirection(mux.MoveRight)
	case actionFocusUp:
		g.mux.FocusDirection(mux.MoveUp)
	case actionFocusDown:
		g.mux.FocusDirection(mux.MoveDown)
	case actionToggleBroadcast:
		g.mux.ToggleBroadcast()
	case actionHelp:
		g.showHelp = !g.showHelp
	case actionQuit:
		return true
	}
	return false
}

func (g *game) scrollOffset(id int) int {
	return g.scroll[id]
}

func (g *game) setScroll(pane *mux.Pane, offset, limit int) {
	offset = clampOffset(offset, limit)
	if offset == 0 {
		delete(g.scroll, pane.ID())
		return
	}
	g.scroll[pane.ID()] = offset
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
