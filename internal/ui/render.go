package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggterm/internal/mux"
	"ggterm/internal/terminal"
)

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(g.theme.Background)
	g.paneHits = g.paneHits[:0]
	g.tabHits = g.tabHits[:0]

	view := g.view
	topRows := 0
	if g.showBars {
		g.drawTabBar(screen, view)
		topRows = 1
	}
	contentY := float64(g.padding) + float64(topRows)*g.cellH
	if g.seenPanes == nil {
		g.seenPanes = make(map[int]bool, len(view.Panes))
	}
	clear(g.seenPanes)
	for _, layout := range view.Panes {
		g.seenPanes[layout.Pane.ID()] = true
		g.drawPane(screen, layout, contentY)
	}
	g.prunePaneCache(g.seenPanes)
	g.pruneGraphicCache()
	if g.showBars {
		g.drawStatusBar(screen, view)
	}
	if g.files != nil {
		g.drawFileManager(screen, view, contentY)
	}
	if g.showHelp && g.files == nil {
		g.drawHelp(screen)
	}
}

func (g *game) drawPane(screen *ebiten.Image, layout mux.PaneLayout, contentY float64) {
	x0 := float64(g.padding) + float64(layout.Rect.X)*g.cellW
	y0 := contentY + float64(layout.Rect.Y)*g.cellH
	width := float64(layout.Rect.W) * g.cellW
	height := float64(layout.Rect.H) * g.cellH

	bordered := layout.Rect.W >= 3 && layout.Rect.H >= 3
	contentX, contentYPos := x0, y0
	if bordered {
		borderColor := g.theme.Border
		thickness := float64(1)
		if layout.Active {
			borderColor = g.theme.BorderActive
			thickness = 2
		}
		drawFrame(screen, x0, y0, width, height, thickness, borderColor)
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

	entry := g.cachedPane(layout)
	if entry == nil || entry.image == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(contentX, contentYPos)
	screen.DrawImage(entry.image, op)

	if layout.Active && !g.showHelp {
		g.drawCursor(screen, entry, contentX, contentYPos, layout.Content)
	}
}

// drawPaneGraphics renders either the placements behind the text (negative
// z) or in front of it (zero and positive z). Drawing into the pane's own
// offscreen image keeps neighbouring panes from being overdrawn.
func (g *game) drawPaneGraphics(dst *ebiten.Image, placements []terminal.ImagePlacement, negative bool) {
	if len(placements) == 0 {
		return
	}
	ordered := make([]terminal.ImagePlacement, 0, len(placements))
	for _, placement := range placements {
		if (placement.Z < 0) == negative {
			ordered = append(ordered, placement)
		}
	}
	slices.SortStableFunc(ordered, func(a, b terminal.ImagePlacement) int {
		return a.Z - b.Z
	})
	for _, placement := range ordered {
		texture := g.graphicImage(placement.Graphic)
		if texture == nil {
			continue
		}
		srcW, srcH := placement.SourceW, placement.SourceH
		if srcW <= 0 || srcH <= 0 {
			continue
		}
		srcX, srcY := placement.SourceX, placement.SourceY
		if srcX < 0 {
			srcX = 0
		}
		if srcY < 0 {
			srcY = 0
		}
		if srcX+srcW > placement.Graphic.Width {
			srcW = placement.Graphic.Width - srcX
		}
		if srcY+srcH > placement.Graphic.Height {
			srcH = placement.Graphic.Height - srcY
		}
		if srcW <= 0 || srcH <= 0 {
			continue
		}
		source, ok := texture.SubImage(image.Rect(srcX, srcY, srcX+srcW, srcY+srcH)).(*ebiten.Image)
		if !ok {
			continue
		}
		width := float64(placement.Cols) * g.cellW
		height := float64(placement.Rows) * g.cellH
		if width <= 0 || height <= 0 {
			continue
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(width/float64(srcW), height/float64(srcH))
		op.GeoM.Translate(
			float64(placement.Col)*g.cellW+float64(placement.X),
			float64(placement.Row)*g.cellH+float64(placement.Y),
		)
		dst.DrawImage(source, op)
	}
}

// drawSnapshot renders terminal cells into a pane-sized offscreen image. The
// image is cached by revision so idle panes cost a single DrawImage per frame.
func (g *game) drawSnapshot(dst *ebiten.Image, snap terminal.Snapshot, content mux.Rect) {
	limit := content.W
	rows := content.H
	if rows > len(snap.Lines) {
		rows = len(snap.Lines)
	}

	g.rects.reset()
	for row := 0; row < rows; row++ {
		g.collectLineBackground(0, 0, row, snap.Lines[row], limit)
	}
	g.rects.flush(dst)

	g.drawPaneGraphics(dst, snap.Images, true)

	for row := 0; row < rows; row++ {
		g.drawLine(dst, 0, 0, row, snap.Lines[row], limit)
	}
}

func (g *game) collectLineBackground(baseX, baseY float64, row int, line []terminal.Cell, limit int) {
	if limit > len(line) {
		limit = len(line)
	}
	rowY := baseY + float64(row)*g.cellH
	for col := 0; col < limit; {
		cell := line[col]
		width := int(cell.Width)
		if width <= 0 {
			col++
			continue
		}
		if width > 2 {
			width = 1
		}
		if width > limit-col {
			width = limit - col
		}
		_, bg := themeColors(g.theme, cell.Style)
		if bg != g.theme.Background {
			g.rects.add(
				baseX+float64(col)*g.cellW,
				rowY,
				float64(width)*g.cellW,
				g.cellH,
				bg,
			)
		}
		col += width
	}
}

func (g *game) drawLine(dst *ebiten.Image, baseX, baseY float64, row int, line []terminal.Cell, limit int) {
	if limit > len(line) {
		limit = len(line)
	}
	start := -1
	var style terminal.Style
	var run strings.Builder
	visible := false

	flush := func(end int) {
		if start < 0 {
			return
		}
		if end > limit {
			end = limit
		}
		if end > start {
			g.drawCells(dst, baseX, baseY, row, start, end, run.String(), style, visible)
		}
		start = -1
		run.Reset()
		visible = false
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
			g.drawCells(dst, baseX, baseY, row, col, col+2, string(cell.Rune), cell.Style, cell.Rune != 0 && cell.Rune != ' ')
			col += 2
			continue
		}
		if start < 0 {
			start, style = col, cell.Style
			run.Reset()
			visible = false
		} else if cell.Style != style {
			flush(col)
			start, style = col, cell.Style
		}
		if cell.Rune == 0 {
			run.WriteRune(' ')
		} else {
			run.WriteRune(cell.Rune)
			if cell.Rune != ' ' {
				visible = true
			}
		}
		col++
	}
	flush(limit)
}

func (g *game) drawCells(dst *ebiten.Image, baseX, baseY float64, row, startCol, endCol int, value string, style terminal.Style, visible bool) {
	fg, bg := themeColors(g.theme, style)
	blinkHidden := style.Blink && !blinkVisible()
	if blinkHidden {
		fg = bg
	}
	x := baseX + float64(startCol)*g.cellW
	y := baseY + float64(row)*g.cellH
	width := float32(float64(endCol-startCol) * g.cellW)
	height := float32(g.cellH)

	variant := g.fonts.selectFace(style)
	if visible && fg != bg {
		if g.fonts.ligatures {
			drawVariantText(dst, variant, value, x, y, g.cellH, fg)
		} else {
			offset := x
			for _, r := range value {
				drawVariantText(dst, variant, string(r), offset, y, g.cellH, fg)
				offset += g.cellW
			}
		}
	}

	underlineColor := fg
	if style.UnderlineColor.Mode != terminal.ColorDefault && !blinkHidden {
		underlineColor = resolveColor(style.UnderlineColor, fg, g.theme.Palette)
	}
	if style.Underline {
		if style.DoubleUnderline {
			vector.FillRect(dst, float32(x), float32(y)+float32(g.ascent)+1, width, 1, underlineColor, false)
			vector.FillRect(dst, float32(x), float32(y)+float32(g.ascent)+3, width, 1, underlineColor, false)
		} else {
			vector.FillRect(dst, float32(x), float32(y)+float32(g.ascent)+1, width, 1, underlineColor, false)
		}
	}
	if style.Strikethrough {
		vector.FillRect(dst, float32(x), float32(y)+height/2, width, 1, fg, false)
	}
}

func drawVariantText(dst *ebiten.Image, variant faceVariant, value string, x, y float64, lineSpacing float64, clr color.Color) {
	draw := func(offset float64) {
		opts := &text.DrawOptions{}
		opts.LayoutOptions.LineSpacing = lineSpacing
		if variant.syntheticItalic {
			opts.GeoM.Skew(-0.2, 0)
		}
		opts.GeoM.Translate(x+offset, y)
		opts.ColorScale.ScaleWithColor(clr)
		text.Draw(dst, value, variant.face, opts)
	}
	draw(0)
	if variant.syntheticBold {
		draw(0.6)
	}
}

func drawText(screen *ebiten.Image, face text.Face, value string, x, y float32, lineSpacing float64, clr color.Color) {
	opts := &text.DrawOptions{}
	opts.LayoutOptions.LineSpacing = lineSpacing
	opts.GeoM.Translate(float64(x), float64(y))
	opts.ColorScale.ScaleWithColor(clr)
	text.Draw(screen, value, face, opts)
}

func (g *game) drawCursor(screen *ebiten.Image, entry *paneCacheEntry, baseX, baseY float64, content mux.Rect) {
	cursor := entry.cursor
	if !cursor.Visible || !ebiten.IsFocused() || !blinkVisible() {
		return
	}
	row, col := cursor.Row, cursor.Col
	if row < 0 || row >= content.H || col < 0 || col >= content.W {
		return
	}
	cells := entry.cursorWidth
	if cells < 1 {
		cells = 1
	}
	if col+cells > content.W {
		cells = 1
	}
	x := baseX + float64(col)*g.cellW
	y := baseY + float64(row)*g.cellH
	vector.FillRect(screen, float32(x), float32(y), float32(float64(cells)*g.cellW), float32(g.cellH), g.theme.Cursor, false)
}

func (g *game) drawTabBar(screen *ebiten.Image, view mux.View) {
	width := float64(view.Cols) * g.cellW
	if width <= 0 {
		return
	}
	g.rects.reset()
	g.rects.add(float64(g.padding), float64(g.padding), width, g.cellH, g.theme.BarBackground)

	x := float64(g.padding)
	maxX := float64(g.padding) + width
	var labels []string
	var positions []float64
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
		if tab.Active {
			g.rects.add(x, float64(g.padding), labelWidth, g.cellH, g.theme.TabActiveBackground)
		}
		labels = append(labels, label)
		positions = append(positions, x)
		g.tabHits = append(g.tabHits, tabHit{
			index: index,
			x0:    int(math.Floor(x)),
			x1:    int(math.Ceil(x + labelWidth)),
		})
		x += labelWidth
	}
	g.rects.flush(screen)
	for i, label := range labels {
		fg := g.theme.BarForeground
		if view.Tabs[i].Active {
			fg = g.theme.TabActiveForeground
		}
		drawText(screen, g.face, label, float32(positions[i]), float32(g.padding), g.cellH, fg)
	}
}

func (g *game) drawStatusBar(screen *ebiten.Image, view mux.View) {
	width := float64(view.Cols) * g.cellW
	if width <= 0 {
		return
	}
	y := float64(g.padding) + float64(1+view.Rows)*g.cellH
	g.rects.reset()
	g.rects.add(float64(g.padding), y, width, g.cellH, g.theme.BarBackground)

	left := fmt.Sprintf(" %d/%d  %s prefix   y files   ? help", view.ActiveTab+1, len(view.Tabs), g.prefix.label)
	if g.prefixing {
		left = " PREFIX  %/\\ split V   \"/- split H   c tab   x close   d detach   ? help"
	}
	right := ""
	if view.Broadcast {
		right = "BROADCAST "
	}
	if g.pluginStatus != "" {
		right += g.pluginStatus + " "
	}
	right += g.currentNotice()
	rightWidth := text.Advance(right, g.face)
	maxLeft := width - rightWidth
	left = fitText(left, maxLeft, g.face)
	g.rects.flush(screen)
	drawText(screen, g.face, left, float32(g.padding), float32(y), g.cellH, g.theme.BarForeground)
	if right != "" {
		rightX := float64(g.padding) + width - rightWidth
		drawText(screen, g.face, right, float32(rightX), float32(y), g.cellH, g.theme.BorderActive)
	}
}

func (g *game) drawHelp(screen *ebiten.Image) {
	lines := g.helpLines()
	maxWidth := 0.0
	for _, line := range lines {
		if width := text.Advance(line, g.face); width > maxWidth {
			maxWidth = width
		}
	}
	boxW := maxWidth + 3*g.cellW
	boxH := float64(len(lines)+2) * g.cellH
	x := (float64(g.width) - boxW) / 2
	y := (float64(g.height) - boxH) / 2
	if x < float64(g.padding) {
		x = float64(g.padding)
	}
	if y < float64(g.padding) {
		y = float64(g.padding)
	}
	vector.FillRect(screen, float32(x), float32(y), float32(boxW), float32(boxH), g.theme.OverlayBackground, false)
	drawFrame(screen, x, y, boxW, boxH, 1, g.theme.BorderActive)
	for i, line := range lines {
		drawText(screen, g.face, line, float32(x+g.cellW), float32(y+g.cellH*float64(i+1)), g.cellH, g.theme.Foreground)
	}
}

func drawFrame(screen *ebiten.Image, x, y, width, height float64, thickness float64, clr color.Color) {
	th := float32(thickness)
	vector.FillRect(screen, float32(x), float32(y), float32(width), th, clr, false)
	vector.FillRect(screen, float32(x), float32(y+height-thickness), float32(width), th, clr, false)
	vector.FillRect(screen, float32(x), float32(y), th, float32(height), clr, false)
	vector.FillRect(screen, float32(x+width-thickness), float32(y), th, float32(height), clr, false)
}

func fitText(value string, maxWidth float64, face text.Face) string {
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
