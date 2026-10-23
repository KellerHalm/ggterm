package ui

import (
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggterm/internal/terminal"
)

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(themeBG)
	snap := g.snapshot
	for row, line := range snap.Lines {
		if row >= snap.Rows {
			break
		}
		g.drawLine(screen, row, line)
	}
	g.drawCursor(screen, snap)
}

func (g *game) drawLine(screen *ebiten.Image, row int, line []terminal.Cell) {
	start := -1
	var style terminal.Style
	var run strings.Builder

	flush := func(end int) {
		if start < 0 {
			return
		}
		g.drawCells(screen, row, start, end, run.String(), style)
		start = -1
		run.Reset()
	}

	for col := 0; col < len(line); {
		cell := line[col]
		width := int(cell.Width)
		if width == 0 {
			col++
			continue
		}
		if width > 2 {
			width = 1
		}
		if width == 2 {
			flush(col)
			g.drawCells(screen, row, col, col+2, string(cell.Rune), cell.Style)
			col += 2
			continue
		}
		if start < 0 {
			start, style = col, cell.Style
		} else if cell.Style != style {
			flush(col)
			start, style = col, cell.Style
		}
		if cell.Rune == 0 {
			run.WriteRune(' ')
		} else {
			run.WriteRune(cell.Rune)
		}
		col++
	}
	flush(len(line))
}

func (g *game) drawCells(screen *ebiten.Image, row, startCol, endCol int, value string, style terminal.Style) {
	fg, bg := cellColors(style)
	if style.Blink && !blinkVisible() {
		fg = bg
	}
	x := float32(padding) + float32(startCol)*float32(g.cellW)
	y := float32(padding) + float32(row)*float32(g.cellH)
	width := float32(endCol-startCol) * float32(g.cellW)
	height := float32(g.cellH)

	if bg != themeBG {
		vector.FillRect(screen, x, y, width, height, bg, false)
	}
	if strings.TrimSpace(value) != "" {
		drawText(screen, g.face, value, x, y, g.cellH, fg)
		if style.Bold {
			drawText(screen, g.face, value, x+0.6, y, g.cellH, fg)
		}
	}
	if style.Underline {
		vector.FillRect(screen, x, y+float32(g.ascent)+1, width, 1, fg, false)
	}
	if style.Strikethrough {
		vector.FillRect(screen, x, y+height/2, width, 1, fg, false)
	}
}

func drawText(screen *ebiten.Image, face *text.GoTextFace, value string, x, y float32, lineSpacing float64, clr color.Color) {
	opts := &text.DrawOptions{}
	opts.LayoutOptions.LineSpacing = lineSpacing
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(screen, value, face, opts)
}

func (g *game) drawCursor(screen *ebiten.Image, snap terminal.Snapshot) {
	if !snap.Cursor.Visible || !ebiten.IsFocused() || !blinkVisible() {
		return
	}
	row, col := snap.Cursor.Row, snap.Cursor.Col
	if row < 0 || row >= snap.Rows || col < 0 || col >= snap.Cols {
		return
	}
	width := float32(g.cellW)
	if row < len(snap.Lines) && col < len(snap.Lines[row]) && snap.Lines[row][col].Width == 2 {
		width *= 2
	}
	x := float32(padding) + float32(col)*float32(g.cellW)
	y := float32(padding) + float32(row)*float32(g.cellH)
	vector.FillRect(screen, x, y, width, float32(g.cellH), themeCursor, false)
}

func blinkVisible() bool {
	return (time.Now().UnixMilli()/cursorBlink.Milliseconds())%2 == 0
}
