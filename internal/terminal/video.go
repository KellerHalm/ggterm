package terminal

import (
	"sync"
	"time"

	"ggterm/internal/video"
)

// videoStream ties one ffmpeg decoder to one terminal graphic. Frames replace
// the graphic in place and bump the terminal revision, so the UI and the daemon
// pick them up through the normal graphics pipeline.
type videoStream struct {
	player    *video.Player
	graphicID uint32
	alt       bool
	done      chan struct{}

	once sync.Once
}

func (s *videoStream) close() {
	s.once.Do(func() {
		close(s.done)
		s.player.Close()
	})
}

const (
	videoMaxDimension = 640
	videoMaxFPS       = 15
)

func (t *Terminal) startVideoLocked(command *ggtermGraphics) {
	if command.path == "" {
		return
	}
	if existing := t.videos[command.id]; existing != nil && command.id != 0 {
		delete(t.videos, command.id)
		existing.close()
	}
	options := video.Options{
		MaxDimension: videoMaxDimension,
		MaxFPS:       videoMaxFPS,
		Loop:         command.loop,
	}
	if command.cols > 0 && command.rows > 0 {
		cellW, cellH := t.cellW, t.cellH
		if cellW <= 0 {
			cellW = defaultGraphicCellW
		}
		if cellH <= 0 {
			cellH = defaultGraphicCellH
		}
		options.Width = command.cols * cellW
		options.Height = command.rows * cellH
	}
	player, err := video.Open(command.path, options)
	if err != nil {
		return
	}
	frame, ok := player.Frame()
	if !ok {
		player.Close()
		return
	}
	graphic, err := t.storeGraphicLocked(command.id, graphicPixels{
		width:  frame.Width,
		height: frame.Height,
		pix:    frame.Pix,
	})
	if err != nil {
		player.Close()
		return
	}
	params := kittyParams{
		imageID: graphic.ID,
		cols:    command.cols,
		rows:    command.rows,
		cellX:   command.x,
		cellY:   command.y,
		z:       command.z,
	}
	t.placeGraphicLocked(graphic, &params, command.move)
	if !command.autoplay {
		player.Pause()
	}
	if t.videos == nil {
		t.videos = make(map[uint32]*videoStream)
	}
	stream := &videoStream{
		player:    player,
		graphicID: graphic.ID,
		alt:       t.altScreen,
		done:      make(chan struct{}),
	}
	t.videos[graphic.ID] = stream
	go t.pumpVideo(stream)
}

func (t *Terminal) pumpVideo(stream *videoStream) {
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()
	var last uint64
	for {
		select {
		case <-stream.done:
			return
		case <-ticker.C:
		}
		frame, ok := stream.player.Frame()
		if !ok || frame.Seq == last {
			continue
		}
		last = frame.Seq
		t.mu.Lock()
		if t.videos[stream.graphicID] != stream {
			t.mu.Unlock()
			return
		}
		if t.graphics[stream.graphicID] == nil {
			delete(t.videos, stream.graphicID)
			t.mu.Unlock()
			stream.close()
			return
		}
		t.applyVideoFrameLocked(stream.graphicID, frame)
		t.mu.Unlock()
	}
}

func (t *Terminal) applyVideoFrameLocked(id uint32, frame video.Frame) {
	previous := t.graphics[id]
	if previous == nil {
		return
	}
	t.graphicBytes += len(frame.Pix) - len(previous.Pix)
	// Replace the graphic instead of mutating it: snapshots returned earlier
	// keep the old immutable bitmap, so the UI and the daemon never read a
	// buffer that is being decoded into.
	t.graphics[id] = &Graphic{
		Key:    nextGraphicKey(),
		ID:     id,
		Width:  frame.Width,
		Height: frame.Height,
		Pix:    frame.Pix,
	}
	t.revision++
}

func (t *Terminal) controlVideoLocked(action string, command *ggtermGraphics) {
	stream := t.videos[command.id]
	if stream == nil {
		return
	}
	switch action {
	case "video_play":
		stream.player.Play()
	case "video_pause":
		stream.player.Pause()
	case "video_seek":
		if command.hasPosition {
			stream.player.SeekTo(secondsDuration(command.position))
		} else if command.hasOffset || command.offset != 0 {
			stream.player.Seek(secondsDuration(command.offset))
		}
	case "video_stop":
		t.removeGraphicLocked(command.id)
	}
}

func secondsDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

func (t *Terminal) stopVideoLocked(id uint32) {
	stream := t.videos[id]
	if stream == nil {
		return
	}
	delete(t.videos, id)
	stream.close()
}

func (t *Terminal) stopAllVideosLocked(altOnly bool) {
	for id, stream := range t.videos {
		if altOnly && !stream.alt {
			continue
		}
		delete(t.videos, id)
		stream.close()
	}
}

// Close stops all video decoders owned by the emulator. It is called when the
// pane goes away; the terminal buffer itself remains usable.
func (t *Terminal) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopAllVideosLocked(false)
}

// VideoCount reports how many videos are currently decoding in this terminal.
// It is primarily useful for tests and diagnostics.
func (t *Terminal) VideoCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.videos)
}
