// Package video decodes video files into RGBA frames with ffmpeg.
//
// Playback is implemented by spawning ffmpeg as a subprocess and reading raw
// RGBA frames from its standard output. The package has no UI dependency and
// is used both by the terminal's graphics protocol and by the file manager
// preview. Audio is intentionally disabled: terminal playback is visual only.
package video

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrFFmpegNotFound is returned when the ffmpeg binary cannot be located.
var ErrFFmpegNotFound = errors.New("video: ffmpeg not found (install ffmpeg or set GGTERM_FFMPEG)")

// Info describes the video stream of a file.
type Info struct {
	Width, Height int
	Duration      time.Duration
	FPS           float64
}

// Frame is one decoded RGBA frame. Pix is immutable after it is published:
// readers may keep the slice while the decoder moves on to the next frame.
type Frame struct {
	Seq       uint64
	Width     int
	Height    int
	Pix       []byte
	Index     int
	Timestamp time.Duration
}

// Options controls probing and decoding.
type Options struct {
	// FFmpeg and FFprobe override the binaries found in PATH. Environment
	// variables GGTERM_FFMPEG and GGTERM_FFPROBE are used when these are
	// empty.
	FFmpeg  string
	FFprobe string

	// MaxDimension caps the long side of decoded frames. It defaults to 960.
	MaxDimension int

	// Width and Height, when set together, make the decoded frame fit inside
	// the given pixel box instead of using MaxDimension.
	Width, Height int

	// MaxFPS caps the decoded frame rate. It defaults to 30.
	MaxFPS int

	// Loop restarts playback from the beginning when the stream ends.
	Loop bool

	// Info skips probing when it is already known. This is mainly useful for
	// tests and for callers that probed the file themselves.
	Info *Info

	// start overrides process creation in tests.
	start func(*exec.Cmd) (io.ReadCloser, error)
}

// State is a snapshot of the player's control state.
type State struct {
	Info     Info
	Width    int
	Height   int
	Position time.Duration
	Playing  bool
	Ended    bool
	Stopped  bool
	Closed   bool
	Err      error
}

// Player decodes one video file and exposes the latest frame. It is safe for
// concurrent use; control methods may be called from any goroutine.
type Player struct {
	path string
	opts Options
	info Info
	w, h int
	fps  float64

	ffmpeg string

	mu     sync.Mutex
	gen    uint64
	cmd    *exec.Cmd
	wake   chan struct{}
	closed bool
	err    error

	frame    *Frame
	seq      uint64
	position time.Duration
	playing  bool
	ended    bool
	stopped  bool
}

const (
	defaultMaxDimension = 960
	defaultMaxFPS       = 30
)

// Open probes path (unless Options.Info is set), starts ffmpeg and waits for
// the first decoded frame.
func Open(path string, opts Options) (*Player, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("video: empty path")
	}
	info := Info{}
	if opts.Info != nil {
		info = *opts.Info
	} else {
		probed, err := Probe(path, opts)
		if err != nil {
			return nil, err
		}
		info = probed
	}
	if info.Width <= 0 || info.Height <= 0 {
		return nil, errors.New("video: no video stream")
	}

	ffmpeg, err := findFFmpeg(opts.FFmpeg)
	if err != nil {
		return nil, err
	}
	width, height := fitSize(info.Width, info.Height, opts)
	fps := effectiveFPS(info.FPS, opts.MaxFPS)

	p := &Player{
		path:    path,
		opts:    opts,
		info:    info,
		w:       width,
		h:       height,
		fps:     fps,
		ffmpeg:  ffmpeg,
		wake:    make(chan struct{}),
		playing: true,
	}
	p.mu.Lock()
	p.startLocked(0)
	err = p.err
	p.mu.Unlock()
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		p.mu.Lock()
		ready := p.frame != nil
		err := p.err
		ended := p.ended
		p.mu.Unlock()
		if ready {
			return p, nil
		}
		if err != nil {
			p.Close()
			return nil, err
		}
		if ended || time.Now().After(deadline) {
			p.Close()
			if ended {
				return nil, errors.New("video: file contains no decodable frames")
			}
			return nil, errors.New("video: decoder timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Path returns the file being played.
func (p *Player) Path() string { return p.path }

// State returns the current playback state.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := State{
		Info:     p.info,
		Width:    p.w,
		Height:   p.h,
		Position: p.position,
		Playing:  p.playing,
		Ended:    p.ended,
		Stopped:  p.stopped,
		Closed:   p.closed,
		Err:      p.err,
	}
	return state
}

// Frame returns the most recently decoded frame.
func (p *Player) Frame() (Frame, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.frame == nil {
		return Frame{}, false
	}
	return *p.frame, true
}

// Play starts or resumes playback. A finished or stopped player restarts from
// the beginning.
func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.playing {
		return
	}
	if p.cmd == nil || p.ended || p.stopped {
		pos := p.position
		if p.ended || p.stopped || (p.info.Duration > 0 && pos >= p.info.Duration) {
			pos = 0
		}
		p.stopped = false
		p.position = pos
		p.startLocked(pos)
	}
	p.playing = true
	p.signalLocked()
}

// Pause suspends decoding without discarding the current frame.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.playing = false
	p.signalLocked()
}

// Toggle switches between play and pause and returns whether playback is now
// running.
func (p *Player) Toggle() bool {
	p.mu.Lock()
	playing := p.playing
	p.mu.Unlock()
	if playing {
		p.Pause()
		return false
	}
	p.Play()
	return true
}

// Seek moves playback by delta and restarts the decoder at the new position.
func (p *Player) Seek(delta time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.seekLocked(p.position + delta)
}

// SeekTo moves playback to an absolute position.
func (p *Player) SeekTo(position time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.seekLocked(position)
}

// Stop stops decoding and resets the position to the beginning. The last frame
// is discarded; Play starts the file again.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.stopProcessLocked()
	p.position = 0
	p.playing = false
	p.ended = false
	p.stopped = true
	p.frame = nil
	p.seq++
}

// Close releases the decoder process. It is safe to call more than once.
func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.playing = false
	p.signalLocked()
	p.stopProcessLocked()
	p.frame = nil
}

func (p *Player) seekLocked(position time.Duration) {
	if position < 0 {
		position = 0
	}
	if p.info.Duration > 0 && position > p.info.Duration {
		position = p.info.Duration
	}
	playing := p.playing
	p.stopProcessLocked()
	p.position = position
	p.ended = false
	p.stopped = false
	p.startLocked(position)
	p.playing = playing
	p.signalLocked()
}

func (p *Player) signalLocked() {
	if p.wake != nil {
		close(p.wake)
	}
	p.wake = make(chan struct{})
}

func (p *Player) stopProcessLocked() {
	p.gen++
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	p.cmd = nil
}

func (p *Player) startLocked(start time.Duration) {
	p.gen++
	gen := p.gen
	p.err = nil
	p.ended = false

	cmd := exec.Command(p.ffmpeg, p.args(start)...)
	cmd.SysProcAttr = sysProcAttr()
	starter := p.opts.start
	if starter == nil {
		starter = startCommand
	}
	stdout, err := starter(cmd)
	if err != nil {
		p.err = fmt.Errorf("video: start ffmpeg: %w", err)
		p.playing = false
		p.ended = true
		return
	}
	p.cmd = cmd
	p.position = start
	go p.readLoop(cmd, stdout, gen, start)
}

func startCommand(cmd *exec.Cmd) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return stdout, nil
}

func (p *Player) args(start time.Duration) []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-re",
	}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-i", p.path)
	filter := fmt.Sprintf("scale=%d:%d", p.w, p.h)
	if p.fps > 0 {
		filter = fmt.Sprintf("fps=%s,%s", formatFloat(p.fps), filter)
	}
	args = append(args,
		"-vf", filter,
		"-an", "-sn", "-dn",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"pipe:1",
	)
	return args
}

func (p *Player) readLoop(cmd *exec.Cmd, stdout io.ReadCloser, gen uint64, start time.Duration) {
	decoded := false
	defer func() {
		_ = cmd.Wait()
		p.mu.Lock()
		p.finishLocked(gen, decoded)
		p.mu.Unlock()
	}()

	size := p.w * p.h * 4
	for index := 0; ; index++ {
		if index > 0 && !p.waitPlaying(gen) {
			return
		}
		buf := make([]byte, size)
		if _, err := io.ReadFull(stdout, buf); err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				p.mu.Lock()
				if gen == p.gen && !p.closed {
					p.err = fmt.Errorf("video: decode: %w", err)
				}
				p.mu.Unlock()
			}
			return
		}
		decoded = true
		p.publish(gen, index, start, buf)
	}
}

func (p *Player) waitPlaying(gen uint64) bool {
	for {
		p.mu.Lock()
		if p.closed || p.gen != gen {
			p.mu.Unlock()
			return false
		}
		playing := p.playing
		wake := p.wake
		p.mu.Unlock()
		if playing {
			return true
		}
		select {
		case <-wake:
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func (p *Player) publish(gen uint64, index int, start time.Duration, pix []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.gen != gen {
		return
	}
	p.seq++
	position := start
	if p.fps > 0 {
		position += time.Duration(float64(index) / p.fps * float64(time.Second))
	}
	if p.info.Duration > 0 && position > p.info.Duration {
		position = p.info.Duration
	}
	p.position = position
	p.frame = &Frame{
		Seq:       p.seq,
		Width:     p.w,
		Height:    p.h,
		Pix:       pix,
		Index:     index,
		Timestamp: position,
	}
}

func (p *Player) finishLocked(gen uint64, decoded bool) {
	if p.closed || p.gen != gen {
		return
	}
	p.cmd = nil
	if decoded && p.opts.Loop {
		p.startLocked(0)
		return
	}
	p.playing = false
	p.ended = true
	if p.info.Duration > 0 {
		p.position = p.info.Duration
	}
}

func fitSize(width, height int, opts Options) (int, int) {
	if width <= 0 || height <= 0 {
		return 2, 2
	}
	maxW, maxH := opts.Width, opts.Height
	maxDimension := opts.MaxDimension
	if maxDimension <= 0 {
		maxDimension = defaultMaxDimension
	}
	if maxW <= 0 && maxH <= 0 {
		if width >= height {
			maxW = maxDimension
		} else {
			maxH = maxDimension
		}
	}
	scale := 1.0
	if maxW > 0 {
		scale = math.Min(scale, float64(maxW)/float64(width))
	}
	if maxH > 0 {
		scale = math.Min(scale, float64(maxH)/float64(height))
	}
	if maxDimension > 0 {
		scale = math.Min(scale, float64(maxDimension)/float64(maxInt(width, height)))
	}
	outW := int(math.Round(float64(width) * scale))
	outH := int(math.Round(float64(height) * scale))
	outW = int(math.Max(2, float64(outW)))
	outH = int(math.Max(2, float64(outH)))
	if outW%2 != 0 {
		outW--
	}
	if outH%2 != 0 {
		outH--
	}
	if outW < 2 {
		outW = 2
	}
	if outH < 2 {
		outH = 2
	}
	return outW, outH
}

func effectiveFPS(source float64, max int) float64 {
	if max <= 0 {
		max = defaultMaxFPS
	}
	if source <= 0 || math.IsNaN(source) || math.IsInf(source, 0) {
		source = defaultMaxFPS
	}
	if source > float64(max) {
		source = float64(max)
	}
	return source
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func findFFmpeg(explicit string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		return value, nil
	}
	if value := strings.TrimSpace(os.Getenv("GGTERM_FFMPEG")); value != "" {
		return value, nil
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", ErrFFmpegNotFound
	}
	return path, nil
}

func findFFprobe(explicit string) (string, bool) {
	if value := strings.TrimSpace(explicit); value != "" {
		return value, true
	}
	if value := strings.TrimSpace(os.Getenv("GGTERM_FFPROBE")); value != "" {
		return value, true
	}
	path, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", false
	}
	return path, true
}
