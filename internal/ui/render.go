package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggterm/internal/mux"
	"ggterm/internal/terminal"
)

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(themeBG)
	g.paneHits = g.paneHits[:0]
	g.tabHits = g.tabHits[:0]

	view := g.view
	topRows := 0
	if g.showBars {
		g.drawTabBar(screen, view)
		topRows = 1
	}
	contentY := float64(padding) + float64(topRows)*g.cellH
	for _, layout := range view.Panes {
		g.drawPane(screen, layout, contentY)
	}
	if g.showBars {
		g.drawStatusBar(screen, view)
	}
	if g.showHelp {
		g.drawHelp(screen)
	}
}

func (g *game) drawPane(screen *ebiten.Image, layout mux.PaneLayout, contentY float64) {
	x0 := float64(padding) + float64(layout.Rect.X)*g.cellW
	y0 := contentY + float64(layout.Rect.Y)*g.cellH
	width := float64(layout.Rect.W) * g.cellW
	height := float64(layout.Rect.H) * g.cellH

	bordered := layout.Rect.W >= 3 && layout.Rect.H >= 3
	contentX, contentYPos := x0, y0
	if bordered {
		borderColor := themeBorder
		thickness := float32(1)
		if layout.Active {
			borderColor = themeBorderActive
			thickness = 2
		}
		drawFrame(screen, x0, y0, width, height, float64(thickness), borderColor)
		contentX = x0 + g.cellW
		contentYPos = y0 + g.cellH
	}

	g.paneHits = append(g.paneHits, paneHit{
		pane: layout.Pane,
		x0:   int(math.Floor(x0)),
		y0:   int(math.Floor(y0)),
		x1:   int(math.Ceil(x0 + width)),
		y1:   int(math.Ceil(y0 + height)),
	})

	snap := layout.Pane.Snapshot(g.scrollOffset(layout.Pane.ID()))
	for row := 0; row < layout.Content.H && row < len(snap.Lines); row++ {
		g.drawLine(screen, contentX, contentYPos, row, snap.Lines[row], layout.Content.W)
	}
	if layout.Active && !g.showHelp {
		g.drawCursor(screen, snap, contentX, contentYPos, layout.Content)
	}
}

func (g *game) drawLine(screen *ebiten.Image, baseX, baseY float64, row int, line []terminal.Cell, limit int) {
	if limit > len(line) {
		limit = len(line)
	}
	start := -1
	var style terminal.Style
	var run strings.Builder

	flush := func(end int) {
		if start < 0 {
			return
		}
		if end > limit {
			end = limit
		}
		if end > start {
			g.drawCells(screen, baseX, baseY, row, start, end, run.String(), style)
		}
		start = -1
		run.Reset()
	}

	for col := 0; col < limit; {
		cell := line[col]
		width := int(cell.Width)
		if width == 0 {
			col++
			continue
		}
		if width > 2 {
			width = 1
		}
		if width == 2 && col+1 < limit {
			flush(col)
			g.drawCells(screen, baseX, baseY, row, col, col+2, string(cell.Rune), cell.Style)
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
	flush(limit)
}

func (g *game) drawCells(screen *ebiten.Image, baseX, baseY float64, row, startCol, endCol int, value string, style terminal.Style) {
	fg, bg := cellColors(style)
	if style.Blink && !blinkVisible() {
		fg = bg
	}
	x := baseX + float64(startCol)*g.cellW
	y := baseY + float64(row)*g.cellH
	width := float32(float64(endCol-startCol) * g.cellW)
	height := float32(g.cellH)

	if bg != themeBG {
		vector.FillRect(screen, float32(x), float32(y), width, height, bg, false)
	}
	if strings.TrimSpace(value) != "" {
		drawText(screen, g.face, value, float32(x), float32(y), g.cellH, fg)
		if style.Bold {
			drawText(screen, g.face, value, float32(x)+0.6, float32(y), g.cellH, fg)
		}
	}
	if style.Underline {
		vector.FillRect(screen, float32(x), float32(y)+float32(g.ascent)+1, width, 1, fg, false)
	}
	if style.Strikethrough {
		vector.FillRect(screen, float32(x), float32(y)+height/2, width, 1, fg, false)
	}
}

func drawText(screen *ebiten.Image, face *text.GoTextFace, value string, x, y float32, lineSpacing float64, clr color.Color) {
	opts := &text.DrawOptions{}
	opts.LayoutOptions.LineSpacing = lineSpacing
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(screen, value, face, opts)
}

func (g *game) drawCursor(screen *ebiten.Image, snap terminal.Snapshot, baseX, baseY float64, content mux.Rect) {
	if !snap.Cursor.Visible || !ebiten.IsFocused() || !blinkVisible() {
		return
	}
	row, col := snap.Cursor.Row, snap.Cursor.Col
	if row < 0 || row >= content.H || col < 0 || col >= content.W {
		return
	}
	width := g.cellW
	if row < len(snap.Lines) && col < len(snap.Lines[row]) && snap.Lines[row][col].Width == 2 {
		width *= 2
	}
	x := baseX + float64(col)*g.cellW
	y := baseY + float64(row)*g.cellH
	vector.FillRect(screen, float32(x), float32(y), float32(width), float32(g.cellH), themeCursor, false)
}

func (g *game) drawTabBar(screen *ebiten.Image, view mux.View) {
	width := float64(view.Cols) * g.cellW
	if width <= 0 {
		return
	}
	vector.FillRect(screen, padding, padding, float32(width), float32(g.cellH), themeBarBG, false)

	x := float64(padding)
	maxX := float64(padding) + width
	for index, tab := range view.Tabs {
		if x >= maxX {
			break
		}
		label := fmt.Sprintf(" %d %s ", index+1, tab.Title)
		label = fitText(label, maxX-x, g.face)
		if label == "" {
			break
		}
		labelWidth := text.Advance(label, g.face)
		fg := themeBarFG
		if tab.Active {
			vector.FillRect(screen, float32(x), padding, float32(labelWidth), float32(g.cellH), themeTabActiveBG, false)
			fg = themeTabActiveFG
		}
		drawText(screen, g.face, label, float32(x), padding, g.cellH, fg)
		g.tabHits = append(g.tabHits, tabHit{
			index: index,
			x0:    int(math.Floor(x)),
			x1:    int(math.Ceil(x + labelWidth)),
		})
		x += labelWidth
	}
}

func (g *game) drawStatusBar(screen *ebiten.Image, view mux.View) {
	width := float64(view.Cols) * g.cellW
	if width <= 0 {
		return
	}
	y := float64(padding) + float64(1+view.Rows)*g.cellH
	vector.FillRect(screen, padding, float32(y), float32(width), float32(g.cellH), themeBarBG, false)

	left := fmt.Sprintf(" %d/%d  Ctrl+B prefix   ? help", view.ActiveTab+1, len(view.Tabs))
	if g.prefix {
		left = " PREFIX  %/\\ split V   \"/- split H   c tab   x close   ? help"
	}
	right := ""
	if view.Broadcast {
		right = "BROADCAST "
	}
	rightWidth := text.Advance(right, g.face)
	maxLeft := width - rightWidth
	left = fitText(left, maxLeft, g.face)
	drawText(screen, g.face, left, padding, float32(y), g.cellH, themeBarFG)
	if right != "" {
		rightX := float64(padding) + width - rightWidth
		drawText(screen, g.face, right, float32(rightX), float32(y), g.cellH, themeBorderActive)
	}
}

func (g *game) drawHelp(screen *ebiten.Image) {
	maxWidth := 0.0
	for _, line := range helpLines {
		if width := text.Advance(line, g.face); width > maxWidth {
			maxWidth = width
		}
	}
	boxW := maxWidth + 3*g.cellW
	boxH := float64(len(helpLines)+2) * g.cellH
	x := (float64(g.width) - boxW) / 2
	y := (float64(g.height) - boxH) / 2
	if x < padding {
		x = padding
	}
	if y < padding {
		y = padding
	}
	vector.FillRect(screen, float32(x), float32(y), float32(boxW), float32(boxH), themeOverlayBG, false)
	drawFrame(screen, x, y, boxW, boxH, 1, themeBorderActive)
	for i, line := range helpLines {
		drawText(screen, g.face, line, float32(x+g.cellW), float32(y+g.cellH*float64(i+1)), g.cellH, themeFG)
	}
}

func drawFrame(screen *ebiten.Image, x, y, width, height float64, thickness float64, clr color.Color) {
	th := float32(thickness)
	vector.FillRect(screen, float32(x), float32(y), float32(width), th, clr, false)
	vector.FillRect(screen, float32(x), float32(y+height-thickness), float32(width), th, clr, false)
	vector.FillRect(screen, float32(x), float32(y), th, float32(height), clr, false)
	vector.FillRect(screen, float32(x+width-thickness), float32(y), th, float32(height), clr, false)
}

func fitText(value string, maxWidth float64, face *text.GoTextFace) string {
	if maxWidth <= 0 {
		return ""
	}
	if text.Advance(value, face) <= maxWidth {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		candidate := strings.TrimRight(string(runes), " ") + "..."
		if text.Advance(candidate, face) <= maxWidth {
			return candidate
		}
	}
	return ""
}

func blinkVisible() bool {
	return (time.Now().UnixMilli()/cursorBlink.Milliseconds())%2 == 0
}
