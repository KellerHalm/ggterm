package ui

import (
	"errors"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"ggterm/internal/pty"
	"ggterm/internal/terminal"
)

// Options controls the terminal window.
type Options struct {
	FontSize float64         // <=0 => 15
	Done     <-chan struct{} // closes the window when the channel closes
}

const (
	padding      = 8
	minWindowW   = 320
	minWindowH   = 200
	cursorBlink  = 530 * time.Millisecond
	defaultFont  = 15.0
	tickRate     = 60
	wheelPerStep = 3
)

type game struct {
	term *terminal.Terminal
	sess pty.Session
	face *text.GoTextFace
	done <-chan struct{}

	cellW, cellH float64
	ascent       float64

	width, height int
	snapshot      terminal.Snapshot
	scrollOffset  int
	wheelAccum    float64
	lastTitle     string

	chars []rune
	keys  []ebiten.Key
}

// Run opens the terminal window and blocks until it is closed or the session
// fails.
func Run(term *terminal.Terminal, sess pty.Session, opts Options) error {
	if term == nil {
		return errors.New("ui: nil terminal")
	}
	if sess == nil {
		return errors.New("ui: nil session")
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
		term:   term,
		sess:   sess,
		face:   face,
		done:   opts.Done,
		ascent: metrics.HAscent,
	}
	g.cellW = text.Advance("M", face)
	if g.cellW <= 0 {
		g.cellW = size * 0.6
	}
	g.cellH = math.Ceil(metrics.HAscent + metrics.HDescent)
	if g.cellH < size {
		g.cellH = math.Ceil(size * 1.2)
	}

	g.snapshot = term.Snapshot(0)
	g.lastTitle = g.snapshot.Title
	width := int(math.Ceil(float64(g.snapshot.Cols)*g.cellW)) + 2*padding
	height := int(math.Ceil(float64(g.snapshot.Rows)*g.cellH)) + 2*padding
	ebiten.SetWindowTitle(windowTitle(g.snapshot.Title))
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
	rows := int((float64(g.height) - 2*padding) / g.cellH)
	cols = clampInt(cols, 1, 1000)
	rows = clampInt(rows, 1, 1000)

	snap := g.term.Snapshot(g.scrollOffset)
	if cols != snap.Cols || rows != snap.Rows {
		g.term.Resize(cols, rows)
		if err := g.sess.Resize(pty.Size{Cols: cols, Rows: rows}); err != nil {
			return err
		}
		snap = g.term.Snapshot(g.scrollOffset)
	}

	if err := g.handleInput(snap); err != nil {
		return err
	}
	if err := g.handleWheel(); err != nil {
		return err
	}

	g.snapshot = g.term.Snapshot(g.scrollOffset)
	g.scrollOffset = g.snapshot.ScrollOffset
	if g.snapshot.Title != g.lastTitle {
		g.lastTitle = g.snapshot.Title
		ebiten.SetWindowTitle(windowTitle(g.lastTitle))
	}
	return nil
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.width = maxInt(outsideWidth, 1)
	g.height = maxInt(outsideHeight, 1)
	return g.width, g.height
}

func (g *game) write(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	_, err := g.sess.Write(data)
	return err
}

func (g *game) handleInput(snap terminal.Snapshot) error {
	mods := currentModifiers()
	typed := false

	chars := ebiten.AppendInputChars(g.chars[:0])
	g.chars = chars
	if !mods.ctrl && !mods.alt {
		for _, r := range chars {
			if r == 0 {
				continue
			}
			if err := g.write([]byte(string(r))); err != nil {
				return err
			}
			typed = true
		}
	}

	keys := inpututil.AppendJustPressedKeys(g.keys[:0])
	g.keys = keys
	for _, key := range keys {
		switch key {
		case ebiten.KeyControl, ebiten.KeyControlLeft, ebiten.KeyControlRight,
			ebiten.KeyAlt, ebiten.KeyAltLeft, ebiten.KeyAltRight,
			ebiten.KeyShift, ebiten.KeyShiftLeft, ebiten.KeyShiftRight:
			continue
		case ebiten.KeyPageUp:
			if mods.shift || !snap.AltScreen {
				g.scrollOffset = clampOffset(g.scrollOffset+pageStep(snap.Rows), snap.ScrollbackLen)
				continue
			}
			if err := g.write([]byte("\x1b[5~")); err != nil {
				return err
			}
			typed = true
			continue
		case ebiten.KeyPageDown:
			if mods.shift || !snap.AltScreen {
				g.scrollOffset = clampOffset(g.scrollOffset-pageStep(snap.Rows), snap.ScrollbackLen)
				continue
			}
			if err := g.write([]byte("\x1b[6~")); err != nil {
				return err
			}
			typed = true
			continue
		}
		sequence, ok := keySequence(key, mods)
		if !ok {
			continue
		}
		if err := g.write(sequence); err != nil {
			return err
		}
		typed = true
	}
	if typed {
		g.scrollOffset = 0
		g.wheelAccum = 0
	}
	return nil
}

func (g *game) handleWheel() error {
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

	if g.snapshot.AltScreen {
		sequence := []byte("\x1b[A")
		if steps < 0 {
			sequence = []byte("\x1b[B")
			steps = -steps
		}
		for i := 0; i < steps; i++ {
			if err := g.write(sequence); err != nil {
				return err
			}
		}
		return nil
	}

	g.scrollOffset = clampOffset(g.scrollOffset+steps*wheelPerStep, g.snapshot.ScrollbackLen)
	return nil
}
