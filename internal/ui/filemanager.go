package ui

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"ggterm/internal/fm"
	"ggterm/internal/mux"
	"ggterm/internal/video"
)

type fmPrompt uint8

const (
	fmPromptNone fmPrompt = iota
	fmPromptFilter
	fmPromptRename
	fmPromptNewFile
	fmPromptNewDir
	fmPromptDelete
)

type fmHit struct {
	column int
	index  int
	entry  fm.Entry
	x0, y0 int
	x1, y1 int
}

type fmRect struct {
	x0, y0 int
	x1, y1 int
}

type fileManager struct {
	model *fm.Model

	prompt fmPrompt
	input  []rune

	parentOffset  int
	currentOffset int
	previewOffset int
	listRows      int

	hits         []fmHit
	previewRect  fmRect
	previewTotal int

	image     *ebiten.Image
	imageKey  string
	imagePath string

	video          *video.Player
	videoPath      string
	videoErr       error
	videoImage     *ebiten.Image
	videoSeq       uint64
	previewVisible bool

	lastClick     time.Time
	lastClickPath string
}

func newFileManager(dir string) (*fileManager, error) {
	model, err := fm.New(dir)
	if err != nil {
		return nil, err
	}
	return &fileManager{model: model, listRows: 1}, nil
}

func (f *fileManager) close() {
	if f.image != nil {
		f.image.Deallocate()
		f.image = nil
	}
	f.stopVideo()
}

func (f *fileManager) stopVideo() {
	if f.video != nil {
		f.video.Close()
		f.video = nil
	}
	if f.videoImage != nil {
		f.videoImage.Deallocate()
		f.videoImage = nil
	}
	f.videoPath = ""
	f.videoErr = nil
	f.videoSeq = 0
}

// syncVideo keeps the preview player in step with the selected entry. Opening
// a decoder is cheap enough to do inline: ffprobe and the first ffmpeg frame
// take a few dozen milliseconds.
func (f *fileManager) syncVideo() {
	entry, ok := f.model.Selected()
	if !f.previewVisible || !ok || f.model.Preview().Kind != fm.PreviewVideo {
		if f.videoPath != "" {
			f.stopVideo()
		}
		return
	}
	if entry.Path == f.videoPath {
		return
	}
	f.stopVideo()
	f.videoPath = entry.Path
	player, err := video.Open(entry.Path, video.Options{
		MaxDimension: 960,
		MaxFPS:       30,
		Loop:         true,
	})
	if err != nil {
		f.videoErr = err
		return
	}
	f.video = player
	f.videoErr = nil
	f.videoSeq = 0
}

func (f *fileManager) updateVideoFrame() {
	if f.video == nil {
		return
	}
	if f.video.State().Stopped {
		if f.videoImage != nil {
			f.videoImage.Deallocate()
			f.videoImage = nil
		}
		f.videoSeq = 0
		return
	}
	frame, ok := f.video.Frame()
	if !ok || frame.Seq == f.videoSeq {
		return
	}
	if f.videoImage == nil ||
		f.videoImage.Bounds().Dx() != frame.Width ||
		f.videoImage.Bounds().Dy() != frame.Height {
		if f.videoImage != nil {
			f.videoImage.Deallocate()
		}
		f.videoImage = ebiten.NewImage(frame.Width, frame.Height)
	}
	f.videoImage.WritePixels(frame.Pix)
	f.videoSeq = frame.Seq
}

func (g *game) toggleFileManager(view mux.View) {
	if g.files != nil {
		files := g.files
		g.files = nil
		files.close()
		return
	}
	dir := ""
	if active := view.ActivePane(); active != nil {
		dir = active.Dir()
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	files, err := newFileManager(dir)
	if err != nil {
		g.notify("file manager: "+err.Error(), 5*time.Second)
		return
	}
	g.files = files
	g.prefixing = false
	g.showHelp = false
}

func (g *game) handleFileManagerInput() error {
	files := g.files
	files.syncVideo()
	files.updateVideoFrame()
	mods := currentModifiers()
	keys := inpututil.AppendJustPressedKeys(g.keys[:0])
	g.keys = keys
	chars := ebiten.AppendInputChars(g.chars[:0])
	g.chars = chars

	if files.prompt != fmPromptNone {
		changed := false
		for _, key := range keys {
			if isModifierKey(key) {
				continue
			}
			switch key {
			case ebiten.KeyEscape:
				files.cancelPrompt()
				return nil
			case ebiten.KeyEnter:
				files.commitPrompt()
				return nil
			case ebiten.KeyBackspace:
				if len(files.input) > 0 {
					files.input = files.input[:len(files.input)-1]
					changed = true
				}
			case ebiten.KeyU:
				if mods.ctrl {
					files.input = nil
					changed = true
				}
			case ebiten.KeyC:
				if mods.ctrl {
					files.cancelPrompt()
					return nil
				}
			}
		}
		if !mods.ctrl && !mods.alt {
			for _, r := range chars {
				if r == 0 {
					continue
				}
				files.input = append(files.input, r)
				changed = true
			}
		}
		if changed && files.prompt == fmPromptFilter {
			files.model.SetFilter(string(files.input))
			files.ensureCurrent()
		}
		return nil
	}

	for _, key := range keys {
		if isModifierKey(key) {
			continue
		}
		if mods.ctrl {
			switch key {
			case ebiten.KeyC:
				g.closeFileManager()
				return nil
			case ebiten.KeyU:
				files.scrollPreview(-maxInt(files.listRows/2, 1))
				continue
			case ebiten.KeyD:
				files.scrollPreview(maxInt(files.listRows/2, 1))
				continue
			}
			continue
		}
		if mods.alt {
			continue
		}
		switch key {
		case ebiten.KeyEscape:
			if files.model.Filter() != "" {
				files.model.SetFilter("")
				files.ensureCurrent()
			} else {
				g.closeFileManager()
				return nil
			}
		case ebiten.KeyQ:
			g.closeFileManager()
			return nil
		case ebiten.KeyArrowUp:
			files.move(-1)
		case ebiten.KeyK:
			if mods.shift {
				files.scrollPreview(-maxInt(files.listRows/2, 1))
			} else {
				files.move(-1)
			}
		case ebiten.KeyArrowDown:
			files.move(1)
		case ebiten.KeyJ:
			if mods.shift {
				files.scrollPreview(maxInt(files.listRows/2, 1))
			} else {
				files.move(1)
			}
		case ebiten.KeyPageUp:
			files.move(-maxInt(files.listRows-1, 1))
		case ebiten.KeyPageDown:
			files.move(maxInt(files.listRows-1, 1))
		case ebiten.KeyHome:
			files.model.Top()
			files.afterMove()
		case ebiten.KeyEnd:
			files.model.Bottom()
			files.afterMove()
		case ebiten.KeyG:
			if mods.shift {
				files.model.Bottom()
			} else {
				files.model.Top()
			}
			files.afterMove()
		case ebiten.KeyArrowLeft, ebiten.KeyH, ebiten.KeyBackspace:
			if err := files.model.Back(); err != nil {
				files.model.SetNotice(err.Error())
			}
			files.currentOffset = 0
			files.parentOffset = 0
			files.previewOffset = 0
		case ebiten.KeyArrowRight, ebiten.KeyL, ebiten.KeyEnter, ebiten.KeyO:
			if entry, open := files.activate(); open {
				g.openFileManagerEntry(entry, false)
			}
		case ebiten.KeyE:
			if entry, open := files.activate(); open {
				g.openFileManagerEntry(entry, mods.shift)
			}
		case ebiten.KeySpace:
			if files.video != nil {
				files.video.Toggle()
			}
		case ebiten.KeyLeftBracket:
			if files.video != nil {
				files.video.Seek(-videoSeekStep(mods.shift))
			}
		case ebiten.KeyRightBracket:
			if files.video != nil {
				files.video.Seek(videoSeekStep(mods.shift))
			}
		case ebiten.KeyS:
			if files.video != nil {
				files.video.Stop()
			}
		case ebiten.KeySlash:
			files.openPrompt(fmPromptFilter)
		case ebiten.KeyPeriod:
			if err := files.model.ToggleHidden(); err != nil {
				files.model.SetNotice(err.Error())
			}
		case ebiten.KeyY:
			if !files.model.CopySelection() {
				files.model.SetNotice("nothing selected")
			}
		case ebiten.KeyX:
			if files.model.CutSelection() {
				files.model.SetNotice("cut " + files.selectedName())
			}
		case ebiten.KeyP:
			if err := files.model.Paste(); err != nil {
				files.model.SetNotice(err.Error())
			}
			files.afterMove()
		case ebiten.KeyD:
			if _, ok := files.model.Selected(); ok {
				files.openPrompt(fmPromptDelete)
			}
		case ebiten.KeyR:
			if mods.shift {
				if err := files.model.Refresh(); err != nil {
					files.model.SetNotice(err.Error())
				}
				break
			}
			if entry, ok := files.model.Selected(); ok {
				files.openPrompt(fmPromptRename)
				files.input = []rune(entry.Name)
			}
		case ebiten.KeyA:
			if mods.shift {
				files.openPrompt(fmPromptNewDir)
			} else {
				files.openPrompt(fmPromptNewFile)
			}
		case ebiten.KeyF5:
			if err := files.model.Refresh(); err != nil {
				files.model.SetNotice(err.Error())
			}
		}
	}
	return nil
}

func (g *game) closeFileManager() {
	if g.files == nil {
		return
	}
	files := g.files
	g.files = nil
	files.close()
}

func (f *fileManager) selectedName() string {
	if entry, ok := f.model.Selected(); ok {
		return entry.Name
	}
	return ""
}

func (f *fileManager) move(delta int) {
	before := f.model.Index()
	f.model.Move(delta)
	if f.model.Index() == before {
		f.ensureCurrent()
		return
	}
	f.afterMove()
}

func (f *fileManager) afterMove() {
	f.previewOffset = 0
	f.ensureCurrent()
}

func (f *fileManager) ensureCurrent() {
	f.currentOffset = ensureOffset(f.currentOffset, f.model.Index(), f.listRows)
}

func ensureOffset(offset, index, rows int) int {
	if rows <= 0 {
		return 0
	}
	if index < offset {
		offset = index
	}
	if index >= offset+rows {
		offset = index - rows + 1
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

func (f *fileManager) scrollPreview(delta int) {
	kind := f.model.Preview().Kind
	if kind != fm.PreviewText && kind != fm.PreviewDir {
		return
	}
	maxOffset := f.previewTotal - maxInt(f.listRows, 1)
	if maxOffset < 0 {
		maxOffset = 0
	}
	f.previewOffset += delta
	if f.previewOffset < 0 {
		f.previewOffset = 0
	}
	if f.previewOffset > maxOffset {
		f.previewOffset = maxOffset
	}
}

// activate enters the selected directory or reports that a file should be
// opened by the caller.
func (f *fileManager) activate() (fm.Entry, bool) {
	entry, entered, err := f.model.Enter()
	if err != nil {
		f.model.SetNotice(err.Error())
		return fm.Entry{}, false
	}
	if entered {
		f.currentOffset = 0
		f.parentOffset = 0
		f.previewOffset = 0
		return fm.Entry{}, false
	}
	if entry.Path == "" {
		return fm.Entry{}, false
	}
	return entry, true
}

func (f *fileManager) openPrompt(kind fmPrompt) {
	f.prompt = kind
	f.input = f.input[:0]
	if kind == fmPromptFilter {
		f.input = []rune(f.model.Filter())
	}
}

func (f *fileManager) cancelPrompt() {
	if f.prompt == fmPromptFilter {
		f.model.SetFilter("")
	}
	f.prompt = fmPromptNone
	f.input = nil
}

func (f *fileManager) commitPrompt() {
	prompt := f.prompt
	value := strings.TrimSpace(string(f.input))
	f.prompt = fmPromptNone
	f.input = nil

	switch prompt {
	case fmPromptFilter:
		f.model.SetFilter(value)
		f.ensureCurrent()
	case fmPromptRename:
		if err := f.model.RenameSelected(value); err != nil {
			f.model.SetNotice("rename: " + err.Error())
		}
	case fmPromptNewFile:
		if err := f.model.CreateEntry(value, false); err != nil {
			f.model.SetNotice("new file: " + err.Error())
		}
	case fmPromptNewDir:
		if err := f.model.CreateEntry(value, true); err != nil {
			f.model.SetNotice("new directory: " + err.Error())
		}
	case fmPromptDelete:
		if strings.EqualFold(value, "y") || strings.EqualFold(value, "yes") {
			if err := f.model.DeleteSelection(); err != nil {
				f.model.SetNotice("delete: " + err.Error())
			}
		} else {
			f.model.SetNotice("delete cancelled")
		}
	}
	f.ensureCurrent()
}

func (f *fileManager) promptLabel() string {
	switch f.prompt {
	case fmPromptFilter:
		return "filter"
	case fmPromptRename:
		return "rename"
	case fmPromptNewFile:
		return "new file"
	case fmPromptNewDir:
		return "new directory"
	case fmPromptDelete:
		return "delete " + f.selectedName() + "? (y/n)"
	}
	return ""
}

func (g *game) drawFileManager(screen *ebiten.Image, view mux.View, contentY float64) {
	files := g.files
	if files == nil {
		return
	}
	files.previewVisible = false
	width := float64(view.Cols) * g.cellW
	height := float64(g.contentRows) * g.cellH
	if width <= 0 || height <= 0 {
		return
	}
	x0 := float64(g.padding)
	y0 := contentY
	files.hits = files.hits[:0]

	vector.FillRect(screen, float32(x0), float32(y0), float32(width), float32(height), g.theme.OverlayBackground, false)
	drawFrame(screen, x0, y0, width, height, 1, g.theme.BorderActive)

	rows := g.contentRows
	if rows < 1 {
		return
	}
	files.listRows = maxInt(rows-2, 0)
	if files.listRows > 0 {
		files.ensureCurrent()
		files.parentOffset = ensureOffset(files.parentOffset, files.model.ParentIndex(), files.listRows)
	}

	headerY := y0
	footerY := y0 + float64(rows-1)*g.cellH
	g.drawFileManagerHeader(screen, files, x0, headerY, width)
	if rows >= 2 {
		vector.FillRect(screen, float32(x0), float32(footerY), float32(width), 1, g.theme.Border, false)
		g.drawFileManagerFooter(screen, files, x0, footerY, width)
	}

	if files.listRows <= 0 {
		return
	}
	listY := y0 + g.cellH
	inset := g.cellW
	available := float64(view.Cols) - 2
	if available < 4 {
		available = 4
	}

	showParent := available >= 44
	showPreview := available >= 24
	files.previewVisible = showPreview && files.listRows > 0
	var parentW, currentW, previewW float64
	switch {
	case showParent && showPreview:
		parentW = math.Floor(available / 4)
		currentW = math.Floor(available * 0.36)
		previewW = available - parentW - currentW - 2
	case showPreview:
		currentW = math.Floor(available * 0.46)
		previewW = available - currentW - 1
	default:
		currentW = available
	}
	if parentW < 8 {
		parentW = 8
	}
	if currentW < 8 {
		currentW = 8
	}
	if previewW < 8 {
		previewW = 8
	}

	xCursor := x0 + inset
	if showParent {
		g.drawFileManagerParent(screen, files, xCursor, listY, parentW)
		xCursor += (parentW + 1) * g.cellW
		vector.FillRect(screen, float32(xCursor-g.cellW), float32(listY), 1, float32(files.listRows)*float32(g.cellH), g.theme.Border, false)
	}
	g.drawFileManagerCurrent(screen, files, xCursor, listY, currentW)
	if showPreview {
		xCursor += currentW * g.cellW
		vector.FillRect(screen, float32(xCursor), float32(listY), 1, float32(files.listRows)*float32(g.cellH), g.theme.Border, false)
		xCursor += g.cellW
		g.drawFileManagerPreview(screen, files, xCursor, listY, previewW)
	}
}

func (g *game) drawFileManagerHeader(screen *ebiten.Image, files *fileManager, x, y, width float64) {
	innerX := x + g.cellW
	innerWidth := width - 2*g.cellW
	if innerWidth <= 0 {
		return
	}
	right := ""
	if filter := files.model.Filter(); filter != "" {
		right = "filter: " + filter + "  "
	}
	if paths, cut := files.model.Clipboard(); len(paths) > 0 {
		if cut {
			right += "CUT "
		} else {
			right += "COPY "
		}
	}
	rightWidth := text.Advance(right, g.face)
	left := fitText(" "+files.model.Cwd(), innerWidth-rightWidth, g.face)
	drawText(screen, g.face, left, float32(innerX), float32(y), g.cellH, g.theme.BarForeground)
	if right != "" {
		drawText(screen, g.face, right, float32(innerX+innerWidth-rightWidth), float32(y), g.cellH, g.theme.BorderActive)
	}
}

func (g *game) drawFileManagerFooter(screen *ebiten.Image, files *fileManager, x, y, width float64) {
	innerX := x + g.cellW
	innerWidth := width - 2*g.cellW
	if innerWidth <= 0 {
		return
	}

	left := ""
	if files.prompt != fmPromptNone {
		left = " " + files.promptLabel()
		if files.prompt != fmPromptDelete {
			left += ": " + string(files.input) + "_"
		}
	} else {
		left = " ↑↓/jk move  enter open  ←/h back  / filter  y copy  x cut  p paste  d delete  r rename  a/A new  . hidden  q close"
	}
	right := files.model.Notice()
	if right == "" {
		if entry, ok := files.model.Selected(); ok {
			if entry.Dir {
				right = "dir"
			} else {
				right = fm.HumanSize(entry.Size)
			}
			right += "  " + entry.ModTime.Format("2006-01-02 15:04")
		}
	}
	rightWidth := text.Advance(right, g.face)
	left = fitText(left, innerWidth-rightWidth-g.cellW, g.face)
	drawText(screen, g.face, left, float32(innerX), float32(y), g.cellH, g.theme.BarForeground)
	if right != "" {
		noticeColor := g.theme.BorderActive
		if files.model.Notice() != "" {
			noticeColor = g.theme.Palette[3]
		}
		drawText(screen, g.face, right, float32(innerX+innerWidth-rightWidth), float32(y), g.cellH, noticeColor)
	}
}

func (g *game) drawFileManagerParent(screen *ebiten.Image, files *fileManager, x, y, widthCells float64) {
	entries := files.model.Parent()
	selected := files.model.ParentIndex()
	offset := files.parentOffset
	for row := 0; row < files.listRows; row++ {
		index := offset + row
		if index >= len(entries) {
			break
		}
		entry := entries[index]
		label := entry.Name
		if entry.Dir {
			label += "/"
		}
		rowY := y + float64(row)*g.cellH
		if index == selected {
			vector.FillRect(screen, float32(x), float32(rowY), float32(widthCells*g.cellW), float32(g.cellH), g.theme.BarBackground, false)
		}
		clr := g.theme.Palette[8]
		if entry.Dir {
			clr = g.theme.Palette[4]
		}
		drawText(screen, g.face, fitText(label, widthCells*g.cellW, g.face), float32(x), float32(rowY), g.cellH, clr)
		files.hits = append(files.hits, fmHit{
			column: 0,
			index:  index,
			entry:  entry,
			x0:     int(x),
			y0:     int(rowY),
			x1:     int(x + widthCells*g.cellW),
			y1:     int(rowY + g.cellH),
		})
	}
	if len(entries) == 0 {
		drawText(screen, g.face, "..", float32(x), float32(y), g.cellH, g.theme.Palette[8])
	}
}

func (g *game) drawFileManagerCurrent(screen *ebiten.Image, files *fileManager, x, y, widthCells float64) {
	entries := files.model.Entries()
	selected := files.model.Index()
	offset := files.currentOffset
	if len(entries) == 0 {
		message := "(empty directory)"
		if files.model.Filter() != "" {
			message = "(no matches)"
		}
		drawText(screen, g.face, message, float32(x), float32(y), g.cellH, g.theme.Palette[8])
		return
	}
	for row := 0; row < files.listRows; row++ {
		index := offset + row
		if index >= len(entries) {
			break
		}
		entry := entries[index]
		label := entry.Name
		if entry.Dir {
			label += "/"
		}
		rowY := y + float64(row)*g.cellH
		fg := g.theme.Foreground
		if entry.Dir {
			fg = g.theme.Palette[4]
		}
		if entry.Symlink {
			fg = g.theme.Palette[6]
		}
		if index == selected {
			vector.FillRect(screen, float32(x), float32(rowY), float32(widthCells*g.cellW), float32(g.cellH), g.theme.TabActiveBackground, false)
			fg = g.theme.TabActiveForeground
		}
		drawText(screen, g.face, fitText(label, widthCells*g.cellW, g.face), float32(x), float32(rowY), g.cellH, fg)
		files.hits = append(files.hits, fmHit{
			column: 1,
			index:  index,
			entry:  entry,
			x0:     int(x),
			y0:     int(rowY),
			x1:     int(x + widthCells*g.cellW),
			y1:     int(rowY + g.cellH),
		})
	}
}

func (g *game) drawFileManagerPreview(screen *ebiten.Image, files *fileManager, x, y, widthCells float64) {
	preview := files.model.Preview()
	width := widthCells * g.cellW
	files.previewRect = fmRect{
		x0: int(x),
		y0: int(y),
		x1: int(x + width),
		y1: int(y + float64(files.listRows)*g.cellH),
	}
	files.previewTotal = 0

	if preview.Path != files.imagePath {
		files.imagePath = preview.Path
		files.previewOffset = 0
	}
	switch preview.Kind {
	case fm.PreviewText:
		files.previewTotal = len(preview.Lines)
	case fm.PreviewDir:
		files.previewTotal = len(preview.DirEntries)
	}
	if maxOffset := files.previewTotal - files.listRows; files.previewOffset > maxOffset {
		files.previewOffset = maxInt(maxOffset, 0)
	}

	switch preview.Kind {
	case fm.PreviewDir:
		entries := preview.DirEntries
		for row := 0; row < files.listRows; row++ {
			index := files.previewOffset + row
			if index >= len(entries) {
				break
			}
			entry := entries[index]
			label := entry.Name
			if entry.Dir {
				label += "/"
			}
			clr := g.theme.Palette[7]
			if entry.Dir {
				clr = g.theme.Palette[4]
			}
			drawText(screen, g.face, fitText(label, width, g.face), float32(x), float32(y+float64(row)*g.cellH), g.cellH, clr)
		}
		if len(entries) == 0 {
			drawText(screen, g.face, "(empty directory)", float32(x), float32(y), g.cellH, g.theme.Palette[8])
		}
	case fm.PreviewText:
		lineCount := len(preview.Lines)
		numberWidth := len(strconv.Itoa(maxInt(lineCount, 1)))
		gutter := float64(numberWidth+1) * g.cellW
		for row := 0; row < files.listRows; row++ {
			index := files.previewOffset + row
			if index >= lineCount {
				break
			}
			rowY := y + float64(row)*g.cellH
			number := fmt.Sprintf("%*d ", numberWidth, index+1)
			drawText(screen, g.face, number, float32(x), float32(rowY), g.cellH, g.theme.Palette[8])
			g.drawFMSpans(screen, preview.Lines[index], x+gutter, rowY, width-gutter)
		}
	case fm.PreviewImage:
		g.drawFMImage(screen, files, preview, x, y, width, float64(files.listRows)*g.cellH)
	case fm.PreviewVideo:
		g.drawFMVideo(screen, files, x, y, width, float64(files.listRows)*g.cellH)
	case fm.PreviewBinary:
		drawText(screen, g.face, fitText(preview.Message, width, g.face), float32(x), float32(y), g.cellH, g.theme.Palette[3])
		drawText(screen, g.face, fm.HumanSize(preview.Size), float32(x), float32(y+g.cellH), g.cellH, g.theme.Palette[8])
	case fm.PreviewError:
		drawText(screen, g.face, fitText(preview.Message, width, g.face), float32(x), float32(y), g.cellH, g.theme.Palette[1])
	default:
		drawText(screen, g.face, "—", float32(x), float32(y), g.cellH, g.theme.Palette[8])
	}
}

func (g *game) drawFMSpans(screen *ebiten.Image, line fm.Line, x, y, maxWidth float64) {
	for _, span := range line {
		if maxWidth <= 0 {
			return
		}
		value := span.Text
		width := text.Advance(value, g.face)
		if width > maxWidth {
			runes := []rune(value)
			for len(runes) > 0 && text.Advance(string(runes), g.face) > maxWidth {
				runes = runes[:len(runes)-1]
			}
			value = string(runes)
			width = text.Advance(value, g.face)
		}
		if value != "" {
			drawText(screen, g.face, value, float32(x), float32(y), g.cellH, g.fmTokenColor(span.Token))
			x += width
			maxWidth -= width
		}
		if value != span.Text {
			return
		}
	}
}

func (g *game) fmTokenColor(token fm.Token) color.RGBA {
	switch token {
	case fm.TokenKeyword:
		return g.theme.Palette[5]
	case fm.TokenString:
		return g.theme.Palette[2]
	case fm.TokenComment:
		return g.theme.Palette[8]
	case fm.TokenNumber:
		return g.theme.Palette[3]
	case fm.TokenType:
		return g.theme.Palette[6]
	case fm.TokenFunction, fm.TokenKey:
		return g.theme.Palette[4]
	case fm.TokenBuiltin:
		return g.theme.Palette[6]
	default:
		return g.theme.Foreground
	}
}

func (g *game) drawFMImage(screen *ebiten.Image, files *fileManager, preview fm.Preview, x, y, width, height float64) {
	if preview.Image == nil {
		return
	}
	key := preview.Path + ":" + strconv.FormatInt(preview.Size, 10) + ":" + strconv.FormatInt(preview.ModTime.UnixNano(), 10)
	if files.image == nil || files.imageKey != key {
		if files.image != nil {
			files.image.Deallocate()
		}
		files.image = ebiten.NewImageFromImage(preview.Image)
		files.imageKey = key
	}
	bounds := files.image.Bounds()
	imageWidth, imageHeight := float64(bounds.Dx()), float64(bounds.Dy())
	if imageWidth <= 0 || imageHeight <= 0 {
		return
	}
	scale := math.Min(width/imageWidth, height/imageHeight)
	if scale > 1 {
		scale = 1
	}
	drawWidth := imageWidth * scale
	drawHeight := imageHeight * scale
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(x+(width-drawWidth)/2, y+(height-drawHeight)/2)
	screen.DrawImage(files.image, op)
}

func (g *game) drawFMVideo(screen *ebiten.Image, files *fileManager, x, y, width, height float64) {
	if files.videoErr != nil {
		drawText(screen, g.face, fitText(files.videoErr.Error(), width, g.face), float32(x), float32(y), g.cellH, g.theme.Palette[1])
		return
	}
	if files.video == nil || files.videoImage == nil {
		message := "loading video..."
		if files.video != nil && !files.video.State().Playing {
			message = "stopped - press space to play"
		}
		drawText(screen, g.face, fitText(message, width, g.face), float32(x), float32(y), g.cellH, g.theme.Palette[8])
		return
	}

	bounds := files.videoImage.Bounds()
	imageWidth, imageHeight := float64(bounds.Dx()), float64(bounds.Dy())
	if imageWidth > 0 && imageHeight > 0 {
		overlay := g.cellH
		areaHeight := math.Max(height-overlay, 1)
		scale := math.Min(width/imageWidth, areaHeight/imageHeight)
		if scale > 1 {
			scale = 1
		}
		drawWidth := imageWidth * scale
		drawHeight := imageHeight * scale
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(x+(width-drawWidth)/2, y+(areaHeight-drawHeight)/2)
		screen.DrawImage(files.videoImage, op)
	}

	state := files.video.State()
	label := "PAUSE"
	if !state.Playing {
		label = "PLAY"
	}
	bar := fmt.Sprintf(" %s  %s / %s   space play  [ ] seek  s stop", label,
		formatDuration(state.Position), formatDuration(state.Info.Duration))
	barY := y + height - g.cellH
	if barY < y {
		barY = y
	}
	vector.FillRect(screen, float32(x), float32(barY), float32(width), float32(g.cellH), g.theme.BarBackground, false)
	if state.Info.Duration > 0 {
		progress := float64(state.Position) / float64(state.Info.Duration)
		progress = math.Max(0, math.Min(progress, 1))
		vector.FillRect(screen, float32(x), float32(barY), float32(width*progress), 2, g.theme.BorderActive, false)
	}
	drawText(screen, g.face, fitText(bar, width, g.face), float32(x), float32(barY), g.cellH, g.theme.BarForeground)
}

func videoSeekStep(shift bool) time.Duration {
	if shift {
		return 30 * time.Second
	}
	return 5 * time.Second
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	total := int(value / time.Second)
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func (g *game) handleFileManagerMouse() {
	files := g.files
	if files == nil {
		return
	}
	x, y := ebiten.CursorPosition()
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	for _, hit := range files.hits {
		if x < hit.x0 || x >= hit.x1 || y < hit.y0 || y >= hit.y1 {
			continue
		}
		switch hit.column {
		case 0:
			if hit.entry.Dir {
				if err := files.model.OpenDir(hit.entry.Path); err != nil {
					files.model.SetNotice(err.Error())
				}
				files.currentOffset = 0
				files.parentOffset = 0
				files.previewOffset = 0
			}
		case 1:
			files.model.Select(hit.index)
			first := files.lastClickPath != hit.entry.Path || time.Since(files.lastClick) > 450*time.Millisecond
			files.lastClick = time.Now()
			files.lastClickPath = hit.entry.Path
			if !first {
				g.activateFileManagerEntry(hit.entry)
			}
		}
		return
	}
}

func (g *game) handleFileManagerWheel() {
	files := g.files
	if files == nil {
		return
	}
	_, delta := ebiten.Wheel()
	if delta == 0 {
		return
	}
	steps := int(delta)
	if steps == 0 {
		if delta > 0 {
			steps = 1
		} else {
			steps = -1
		}
	}
	kind := files.model.Preview().Kind
	x, y := ebiten.CursorPosition()
	if (kind == fm.PreviewText || kind == fm.PreviewDir) &&
		x >= files.previewRect.x0 && x < files.previewRect.x1 &&
		y >= files.previewRect.y0 && y < files.previewRect.y1 {
		files.scrollPreview(-steps)
		return
	}
	files.move(steps)
}

func (g *game) activateFileManagerEntry(entry fm.Entry) {
	if entry.Dir {
		if err := g.files.model.OpenDir(entry.Path); err != nil {
			g.files.model.SetNotice(err.Error())
		}
		g.files.currentOffset = 0
		g.files.parentOffset = 0
		g.files.previewOffset = 0
		return
	}
	g.openFileManagerEntry(entry, false)
}

func (g *game) openFileManagerEntry(entry fm.Entry, edit bool) {
	command := openCommand(entry.Path)
	if edit {
		command = editCommand(entry.Path)
	}
	g.closeFileManager()
	g.session.WriteInput([]byte(command + "\r"))
	g.notify("opened "+entry.Name, 3*time.Second)
}

func openCommand(path string) string {
	switch runtime.GOOS {
	case "windows":
		return `cmd /c start "" ` + quoteShell(path)
	case "darwin":
		return "open " + quoteShell(path)
	default:
		return "xdg-open " + quoteShell(path)
	}
}

func editCommand(path string) string {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		switch runtime.GOOS {
		case "windows":
			editor = "notepad"
		case "darwin":
			editor = "open -e"
		default:
			editor = "nano"
		}
	}
	return editor + " " + quoteShell(path)
}

func quoteShell(path string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(path, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
