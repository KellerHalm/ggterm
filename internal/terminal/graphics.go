package terminal

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"image"
	stddraw "image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	_ "golang.org/x/image/webp"
)

const (
	defaultGraphicCellW = 8
	defaultGraphicCellH = 16

	maxGraphicPayload    = 32 << 20
	maxGraphicPixels     = 16 << 20
	maxGraphicBytes      = 128 << 20
	maxGraphicCount      = 256
	maxGraphicPlacements = 4096
)

var graphicKeySeq atomic.Uint64

// Graphic is one decoded bitmap stored by the terminal emulator.
//
// Key is unique across the whole process, including images transmitted by
// different panes. Frontends should use it as a cache key, while ID is the
// identifier used by the graphics protocol.
type Graphic struct {
	Key    uint64
	ID     uint32
	Width  int
	Height int
	Pix    []byte // non-premultiplied RGBA, row-major
}

// ImagePlacement describes where a Graphic is visible on the current screen.
// Row and Col are relative to the visible snapshot, so they already include
// the scrollback offset.
type ImagePlacement struct {
	Graphic *Graphic

	PlacementID uint32
	Row, Col    int
	Cols, Rows  int
	X, Y        int
	SourceX     int
	SourceY     int
	SourceW     int
	SourceH     int
	Z           int
}

type graphicPlacement struct {
	imageID     uint32
	placementID uint32
	alt         bool
	absRow      int
	col         int
	cols        int
	rows        int
	reqCols     int
	reqRows     int
	x           int
	y           int
	srcX        int
	srcY        int
	srcW        int
	srcH        int
	z           int
}

type graphicPixels struct {
	width  int
	height int
	pix    []byte
}

func nextGraphicKey() uint64 {
	return graphicKeySeq.Add(1)
}

func (t *Terminal) snapshotImagesLocked(scrollOffset int) []ImagePlacement {
	if len(t.placements) == 0 {
		return nil
	}
	start := 0
	if !t.altScreen {
		start = t.scrollBase - scrollOffset
		if start < 0 {
			start = 0
		}
	}
	rows := t.cfg.Rows
	images := make([]ImagePlacement, 0, len(t.placements))
	for _, placement := range t.placements {
		if placement.alt != t.altScreen {
			continue
		}
		row := placement.absRow
		if !t.altScreen {
			row -= start
		}
		if row >= rows || row+placement.rows <= 0 {
			continue
		}
		if placement.col >= t.cfg.Cols || placement.col+placement.cols <= 0 {
			continue
		}
		graphic := t.graphics[placement.imageID]
		if graphic == nil {
			continue
		}
		images = append(images, ImagePlacement{
			Graphic:     graphic,
			PlacementID: placement.placementID,
			Row:         row,
			Col:         placement.col,
			Cols:        placement.cols,
			Rows:        placement.rows,
			X:           placement.x,
			Y:           placement.y,
			SourceX:     placement.srcX,
			SourceY:     placement.srcY,
			SourceW:     placement.srcW,
			SourceH:     placement.srcH,
			Z:           placement.z,
		})
	}
	sort.SliceStable(images, func(i, j int) bool {
		if images[i].Z != images[j].Z {
			return images[i].Z < images[j].Z
		}
		if images[i].Graphic.ID != images[j].Graphic.ID {
			return images[i].Graphic.ID < images[j].Graphic.ID
		}
		return images[i].PlacementID < images[j].PlacementID
	})
	return images
}

func (t *Terminal) allocGraphicIDLocked() uint32 {
	for {
		t.nextGraphicID++
		if t.nextGraphicID == 0 {
			t.nextGraphicID = 1
		}
		if t.graphics[t.nextGraphicID] == nil {
			return t.nextGraphicID
		}
	}
}

func (t *Terminal) storeGraphicLocked(id uint32, pixels graphicPixels) (*Graphic, error) {
	if pixels.width <= 0 || pixels.height <= 0 {
		return nil, errors.New("EINVAL:image has invalid dimensions")
	}
	if int64(pixels.width)*int64(pixels.height) > maxGraphicPixels {
		return nil, errors.New("ENOSPC:image is too large")
	}
	size := len(pixels.pix)
	if size == 0 || size > maxGraphicBytes {
		return nil, errors.New("ENOSPC:image is too large")
	}
	if id == 0 {
		id = t.allocGraphicIDLocked()
	}
	if existing := t.graphics[id]; existing != nil {
		t.removePlacementsLocked(func(p graphicPlacement) bool { return p.imageID == id })
		t.graphicBytes -= len(existing.Pix)
		delete(t.graphics, id)
	}
	if t.graphicBytes+size > maxGraphicBytes {
		t.evictUnreferencedLocked(size)
	}
	if t.graphicBytes+size > maxGraphicBytes {
		return nil, errors.New("ENOSPC:image storage full")
	}
	if t.graphics == nil {
		t.graphics = make(map[uint32]*Graphic)
	}
	if len(t.graphics) >= maxGraphicCount {
		if !t.evictOldestUnreferencedLocked() {
			return nil, errors.New("ENOSPC:too many images")
		}
	}
	graphic := &Graphic{
		Key:    nextGraphicKey(),
		ID:     id,
		Width:  pixels.width,
		Height: pixels.height,
		Pix:    pixels.pix,
	}
	t.graphics[id] = graphic
	t.graphicBytes += size
	return graphic, nil
}

func (t *Terminal) forgetImageNumberLocked(id uint32) {
	for number, imageID := range t.imageNumbers {
		if imageID == id {
			delete(t.imageNumbers, number)
		}
	}
}

func (t *Terminal) evictUnreferencedLocked(need int) {
	if len(t.graphics) == 0 {
		return
	}
	referenced := make(map[uint32]bool, len(t.placements))
	for _, placement := range t.placements {
		referenced[placement.imageID] = true
	}
	for id, graphic := range t.graphics {
		if referenced[id] {
			continue
		}
		t.graphicBytes -= len(graphic.Pix)
		delete(t.graphics, id)
		t.forgetImageNumberLocked(id)
		if t.graphicBytes+need <= maxGraphicBytes {
			return
		}
	}
}

func (t *Terminal) evictOldestUnreferencedLocked() bool {
	var (
		oldestID  uint32
		oldestKey uint64
		found     bool
	)
	referenced := make(map[uint32]bool, len(t.placements))
	for _, placement := range t.placements {
		referenced[placement.imageID] = true
	}
	for id, graphic := range t.graphics {
		if referenced[id] {
			continue
		}
		if !found || graphic.Key < oldestKey {
			oldestID, oldestKey, found = id, graphic.Key, true
		}
	}
	if !found {
		return false
	}
	graphic := t.graphics[oldestID]
	t.graphicBytes -= len(graphic.Pix)
	delete(t.graphics, oldestID)
	t.forgetImageNumberLocked(oldestID)
	return true
}

func (t *Terminal) removeGraphicLocked(id uint32) {
	graphic := t.graphics[id]
	if graphic == nil {
		return
	}
	t.removePlacementsLocked(func(p graphicPlacement) bool { return p.imageID == id })
	t.graphicBytes -= len(graphic.Pix)
	delete(t.graphics, id)
	t.forgetImageNumberLocked(id)
}

func (t *Terminal) removePlacementsLocked(match func(graphicPlacement) bool) {
	if len(t.placements) == 0 {
		return
	}
	kept := t.placements[:0]
	for _, placement := range t.placements {
		if !match(placement) {
			kept = append(kept, placement)
		}
	}
	t.placements = kept
}

func (t *Terminal) dropUnreferencedIDsLocked(ids map[uint32]bool) {
	if len(ids) == 0 || len(t.graphics) == 0 {
		return
	}
	referenced := make(map[uint32]bool, len(t.placements))
	for _, placement := range t.placements {
		referenced[placement.imageID] = true
	}
	for id := range ids {
		if referenced[id] {
			continue
		}
		graphic := t.graphics[id]
		if graphic == nil {
			continue
		}
		t.graphicBytes -= len(graphic.Pix)
		delete(t.graphics, id)
		t.forgetImageNumberLocked(id)
	}
}

func (t *Terminal) clearAltGraphicsLocked() {
	ids := make(map[uint32]bool)
	for _, placement := range t.placements {
		if placement.alt {
			ids[placement.imageID] = true
		}
	}
	t.removePlacementsLocked(func(p graphicPlacement) bool { return p.alt })
	t.dropUnreferencedIDsLocked(ids)
	t.pendingGraphic = nil
}

func (t *Terminal) clearGraphicsLocked() {
	t.graphics = nil
	t.placements = nil
	t.graphicBytes = 0
	t.imageNumbers = nil
	t.nextGraphicID = 0
	t.scrollBase = 0
	t.pendingGraphic = nil
}

func (t *Terminal) pruneScrollbackGraphicsLocked() {
	if len(t.placements) == 0 {
		return
	}
	oldest := t.scrollBase - len(t.scrollback)
	t.removePlacementsLocked(func(p graphicPlacement) bool {
		return !p.alt && p.absRow+p.rows <= oldest
	})
}

func (t *Terminal) graphicsScrollUpLocked(top, bottom, n int) {
	if len(t.placements) == 0 || n <= 0 {
		return
	}
	regionTop := top
	regionBottom := bottom
	if !t.altScreen {
		regionTop = t.scrollBase + top
		regionBottom = t.scrollBase + bottom
	}
	kept := t.placements[:0]
	for _, placement := range t.placements {
		if placement.alt != t.altScreen {
			kept = append(kept, placement)
			continue
		}
		row := placement.absRow
		if row < regionTop || row > regionBottom {
			kept = append(kept, placement)
			continue
		}
		if !t.altScreen && top == 0 {
			kept = append(kept, placement)
			continue
		}
		placement.absRow -= n
		if placement.absRow < regionTop {
			continue
		}
		kept = append(kept, placement)
	}
	t.placements = kept
}

func (t *Terminal) graphicsScrollDownLocked(top, bottom, n int) {
	if len(t.placements) == 0 || n <= 0 {
		return
	}
	regionTop := top
	regionBottom := bottom
	if !t.altScreen {
		regionTop = t.scrollBase + top
		regionBottom = t.scrollBase + bottom
	}
	kept := t.placements[:0]
	for _, placement := range t.placements {
		if placement.alt != t.altScreen {
			kept = append(kept, placement)
			continue
		}
		row := placement.absRow
		if row < regionTop || row > regionBottom {
			kept = append(kept, placement)
			continue
		}
		placement.absRow += n
		if placement.absRow > regionBottom {
			continue
		}
		kept = append(kept, placement)
	}
	t.placements = kept
}

func (t *Terminal) graphicsDeleteLinesLocked(row, bottom, n int) {
	if len(t.placements) == 0 || n <= 0 {
		return
	}
	regionTop := row
	regionBottom := bottom
	if !t.altScreen {
		regionTop = t.scrollBase + row
		regionBottom = t.scrollBase + bottom
	}
	kept := t.placements[:0]
	for _, placement := range t.placements {
		if placement.alt != t.altScreen {
			kept = append(kept, placement)
			continue
		}
		abs := placement.absRow
		if abs < regionTop || abs > regionBottom {
			kept = append(kept, placement)
			continue
		}
		placement.absRow -= n
		if placement.absRow < regionTop {
			continue
		}
		kept = append(kept, placement)
	}
	t.placements = kept
}

func (t *Terminal) placeGraphicLocked(graphic *Graphic, params *kittyParams, move bool) *Graphic {
	srcX, srcY, srcW, srcH := graphicSource(graphic, params.sourceX, params.sourceY, params.sourceW, params.sourceH)
	cols, rows := t.graphicCells(srcW, srcH, params.cols, params.rows)
	absRow := t.cursor.Row
	if !t.altScreen {
		absRow = t.scrollBase + t.cursor.Row
	}
	placement := graphicPlacement{
		imageID:     graphic.ID,
		placementID: params.placementID,
		alt:         t.altScreen,
		absRow:      absRow,
		col:         t.cursor.Col,
		cols:        cols,
		rows:        rows,
		reqCols:     params.cols,
		reqRows:     params.rows,
		x:           params.cellX,
		y:           params.cellY,
		srcX:        srcX,
		srcY:        srcY,
		srcW:        srcW,
		srcH:        srcH,
		z:           params.z,
	}
	replaced := false
	if placement.placementID != 0 {
		for i := range t.placements {
			existing := &t.placements[i]
			if existing.alt == placement.alt &&
				existing.imageID == placement.imageID &&
				existing.placementID == placement.placementID {
				*existing = placement
				replaced = true
				break
			}
		}
	}
	if !replaced {
		if len(t.placements) >= maxGraphicPlacements {
			t.placements = t.placements[1:]
		}
		t.placements = append(t.placements, placement)
	}
	if move {
		t.cursor.Col += cols
		t.cursor.Row += rows
		if t.cursor.Col >= t.cfg.Cols {
			t.cursor.Col = t.cfg.Cols - 1
		}
		if t.cursor.Row >= t.cfg.Rows {
			t.cursor.Row = t.cfg.Rows - 1
		}
		t.pendingWrap = false
	}
	return graphic
}

func (t *Terminal) recomputeGraphicCellsLocked() {
	for i := range t.placements {
		placement := &t.placements[i]
		placement.cols, placement.rows = t.graphicCells(
			placement.srcW, placement.srcH, placement.reqCols, placement.reqRows,
		)
	}
}

func (t *Terminal) graphicCells(srcW, srcH, cols, rows int) (int, int) {
	cellW := t.cellW
	cellH := t.cellH
	if cellW <= 0 {
		cellW = defaultGraphicCellW
	}
	if cellH <= 0 {
		cellH = defaultGraphicCellH
	}
	if srcW <= 0 {
		srcW = 1
	}
	if srcH <= 0 {
		srcH = 1
	}
	switch {
	case cols <= 0 && rows <= 0:
		cols = int(math.Ceil(float64(srcW) / float64(cellW)))
		rows = int(math.Ceil(float64(srcH) / float64(cellH)))
	case cols > 0 && rows <= 0:
		widthPx := float64(cols) * float64(cellW)
		heightPx := widthPx * float64(srcH) / float64(srcW)
		rows = int(math.Ceil(heightPx / float64(cellH)))
	case cols <= 0 && rows > 0:
		heightPx := float64(rows) * float64(cellH)
		widthPx := heightPx * float64(srcW) / float64(srcH)
		cols = int(math.Ceil(widthPx / float64(cellW)))
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if cols > 4096 {
		cols = 4096
	}
	if rows > 4096 {
		rows = 4096
	}
	return cols, rows
}

func graphicSource(graphic *Graphic, x, y, w, h int) (int, int, int, int) {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x > graphic.Width {
		x = graphic.Width
	}
	if y > graphic.Height {
		y = graphic.Height
	}
	if w <= 0 || x+w > graphic.Width {
		w = graphic.Width - x
	}
	if h <= 0 || y+h > graphic.Height {
		h = graphic.Height - y
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return x, y, w, h
}

func (t *Terminal) latestImageForNumberLocked(number uint32) uint32 {
	if number == 0 {
		return 0
	}
	return t.imageNumbers[number]
}

func (t *Terminal) resolveImageLocked(params *kittyParams) (*Graphic, uint32) {
	id := params.imageID
	if id == 0 && params.imageNumber != 0 {
		id = t.latestImageForNumberLocked(params.imageNumber)
	}
	if id == 0 {
		return nil, 0
	}
	return t.graphics[id], id
}

func decodeBase64(data []byte) ([]byte, error) {
	cleaned := make([]byte, 0, len(data))
	for _, b := range data {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		}
		cleaned = append(cleaned, b)
	}
	if len(cleaned) == 0 {
		return nil, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(string(cleaned)); err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(string(cleaned))
}

func decodeGraphicFile(data []byte) (graphicPixels, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return graphicPixels{}, errors.New("EINVAL:unsupported image data")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 ||
		int64(cfg.Width)*int64(cfg.Height) > maxGraphicPixels {
		return graphicPixels{}, errors.New("ENOSPC:image is too large")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return graphicPixels{}, errors.New("EINVAL:invalid image data")
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(dst, dst.Bounds(), img, bounds.Min, stddraw.Src)
	return graphicPixels{width: width, height: height, pix: dst.Pix}, nil
}

func decodeRawPixels(format, width, height int, data []byte) (graphicPixels, error) {
	if width <= 0 || height <= 0 {
		return graphicPixels{}, errors.New("EINVAL:image dimensions are required")
	}
	pixels := int64(width) * int64(height)
	if pixels > maxGraphicPixels {
		return graphicPixels{}, errors.New("ENOSPC:image is too large")
	}
	expected := int(pixels * 4)
	if format == 24 {
		if int64(len(data)) < pixels*3 {
			return graphicPixels{}, errors.New("EINVAL:not enough RGB data")
		}
		pix := make([]byte, expected)
		for i, j := 0, 0; i < int(pixels); i, j = i+1, j+3 {
			pix[i*4] = data[j]
			pix[i*4+1] = data[j+1]
			pix[i*4+2] = data[j+2]
			pix[i*4+3] = 0xff
		}
		return graphicPixels{width: width, height: height, pix: pix}, nil
	}
	if len(data) < expected {
		return graphicPixels{}, errors.New("EINVAL:not enough RGBA data")
	}
	pix := make([]byte, expected)
	copy(pix, data[:expected])
	return graphicPixels{width: width, height: height, pix: pix}, nil
}

func inflate(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("EINVAL:invalid compressed data")
	}
	defer reader.Close()
	const maxInflated = int64(maxGraphicPixels) * 4
	limited := io.LimitReader(reader, maxInflated+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, errors.New("EINVAL:invalid compressed data")
	}
	if int64(len(out)) > maxInflated {
		return nil, errors.New("ENOSPC:image is too large")
	}
	return out, nil
}

func readGraphicPath(path string, remove bool) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("EBADF:Failed to read image file")
	}
	if info.Size() <= 0 || info.Size() > maxGraphicPayload {
		return nil, errors.New("EBADF:Failed to read image file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("EBADF:Failed to read image file")
	}
	if remove {
		_ = os.Remove(path)
	}
	return data, nil
}

func ownFormatIsImage(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "auto", "png", "jpeg", "jpg", "gif", "webp":
		return true
	}
	return false
}

func parseUint32(value string) uint32 {
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

func parseInt(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func splitControl(data []byte) (control, payload []byte) {
	if index := bytes.IndexByte(data, ';'); index >= 0 {
		return data[:index], data[index+1:]
	}
	return data, nil
}
