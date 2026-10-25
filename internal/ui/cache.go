package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"ggterm/internal/mux"
	"ggterm/internal/terminal"
)

// paneCacheKey captures everything that affects the rendered pixels of a pane.
// The terminal revision changes on every write and resize; the scroll offset
// covers local scrolling, which does not bump the revision.
type paneCacheKey struct {
	revision uint64
	offset   int
	cols     int
	rows     int
	version  uint64
	// blinkOn is only meaningful for panes that contain blinking cells. It
	// forces the cached image to be regenerated in step with the blink phase.
	blinkOn bool
}

type paneCacheEntry struct {
	key         paneCacheKey
	image       *ebiten.Image
	cursor      terminal.Cursor
	cursorWidth int
	blink       bool
	graphicKeys []uint64
}

type graphicCacheEntry struct {
	image *ebiten.Image
}

func (g *game) cachedPane(layout mux.PaneLayout) *paneCacheEntry {
	content := layout.Content
	if content.W <= 0 || content.H <= 0 {
		return nil
	}
	id := layout.Pane.ID()
	entry := g.paneCache[id]
	blinkOn := blinkVisible()
	key := paneCacheKey{
		revision: layout.Pane.Revision(),
		offset:   layout.Pane.ScrollOffset(),
		cols:     content.W,
		rows:     content.H,
		version:  g.renderVersion,
	}
	if entry != nil && entry.blink {
		key.blinkOn = blinkOn
	}
	if entry != nil && entry.key == key {
		return entry
	}

	snap := layout.Pane.Snapshot()
	width := maxInt(int(math.Ceil(float64(content.W)*g.cellW)), 1)
	height := maxInt(int(math.Ceil(float64(content.H)*g.cellH)), 1)
	var image *ebiten.Image
	if entry != nil && entry.image != nil &&
		entry.image.Bounds().Dx() == width && entry.image.Bounds().Dy() == height {
		image = entry.image
		image.Clear()
	} else {
		image = ebiten.NewImage(width, height)
		if entry != nil && entry.image != nil {
			entry.image.Deallocate()
		}
	}
	g.drawSnapshot(image, snap, content)
	g.drawPaneGraphics(image, snap.Images, false)
	blink := snapshotBlinking(snap, content)
	if blink {
		key.blinkOn = blinkOn
	}

	graphicKeys := make([]uint64, 0, len(snap.Images))
	for _, placement := range snap.Images {
		if placement.Graphic != nil {
			graphicKeys = append(graphicKeys, placement.Graphic.Key)
		}
	}
	entry = &paneCacheEntry{
		key:         key,
		image:       image,
		cursor:      snap.Cursor,
		cursorWidth: cursorWidth(snap, content),
		blink:       blink,
		graphicKeys: graphicKeys,
	}
	if g.paneCache == nil {
		g.paneCache = make(map[int]*paneCacheEntry)
	}
	g.paneCache[id] = entry
	return entry
}

// graphicImage uploads a decoded bitmap to the GPU once and returns the
// cached texture. Graphic.Key is unique per transmitted image, so panes and
// daemon frames can share textures safely.
func (g *game) graphicImage(graphic *terminal.Graphic) *ebiten.Image {
	if graphic == nil || graphic.Width <= 0 || graphic.Height <= 0 {
		return nil
	}
	if len(graphic.Pix) < graphic.Width*graphic.Height*4 {
		return nil
	}
	if g.graphicCache == nil {
		g.graphicCache = make(map[uint64]*graphicCacheEntry)
	}
	if entry := g.graphicCache[graphic.Key]; entry != nil {
		return entry.image
	}
	source := &image.NRGBA{
		Pix:    graphic.Pix,
		Stride: graphic.Width * 4,
		Rect:   image.Rect(0, 0, graphic.Width, graphic.Height),
	}
	texture := ebiten.NewImageFromImage(source)
	g.graphicCache[graphic.Key] = &graphicCacheEntry{image: texture}
	return texture
}

func (g *game) pruneGraphicCache() {
	if len(g.graphicCache) == 0 {
		return
	}
	seen := make(map[uint64]bool)
	for _, entry := range g.paneCache {
		for _, key := range entry.graphicKeys {
			seen[key] = true
		}
	}
	for key, entry := range g.graphicCache {
		if seen[key] {
			continue
		}
		if entry.image != nil {
			entry.image.Deallocate()
		}
		delete(g.graphicCache, key)
	}
}

func (g *game) clearGraphicCache() {
	for key, entry := range g.graphicCache {
		if entry.image != nil {
			entry.image.Deallocate()
		}
		delete(g.graphicCache, key)
	}
}

// cursorWidth returns the number of cells the cursor should cover: two for a
// wide glyph, one otherwise. Invalid positions yield one cell.
func cursorWidth(snap terminal.Snapshot, content mux.Rect) int {
	cursor := snap.Cursor
	if cursor.Row < 0 || cursor.Row >= content.H || cursor.Row >= len(snap.Lines) {
		return 1
	}
	line := snap.Lines[cursor.Row]
	if cursor.Col < 0 || cursor.Col >= content.W || cursor.Col >= len(line) {
		return 1
	}
	if line[cursor.Col].Width == 2 && cursor.Col+1 < content.W {
		return 2
	}
	return 1
}

// snapshotBlinking reports whether any visible cell uses SGR blink. Such panes
// cannot be reused verbatim across blink phases.
func snapshotBlinking(snap terminal.Snapshot, content mux.Rect) bool {
	rows := content.H
	if rows > len(snap.Lines) {
		rows = len(snap.Lines)
	}
	for row := 0; row < rows; row++ {
		line := snap.Lines[row]
		limit := content.W
		if limit > len(line) {
			limit = len(line)
		}
		for col := 0; col < limit; col++ {
			if line[col].Style.Blink {
				return true
			}
		}
	}
	return false
}

func (g *game) prunePaneCache(seen map[int]bool) {
	for id, entry := range g.paneCache {
		if seen[id] {
			continue
		}
		if entry.image != nil {
			entry.image.Deallocate()
		}
		delete(g.paneCache, id)
	}
}

func (g *game) clearPaneCache() {
	for id, entry := range g.paneCache {
		if entry.image != nil {
			entry.image.Deallocate()
		}
		delete(g.paneCache, id)
	}
}
