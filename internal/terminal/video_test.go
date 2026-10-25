package terminal

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGgtermVideoControl(t *testing.T) {
	command := parseGgtermGraphics([]byte("action=video,id=7,path=/tmp/clip.mp4,cols=20,rows=10,loop=1,play=0,offset=2.5"))
	if command.action != "video" || command.id != 7 || command.path != "/tmp/clip.mp4" {
		t.Errorf("command = %+v", command)
	}
	if command.cols != 20 || command.rows != 10 || !command.loop {
		t.Errorf("video size/loop = %+v", command)
	}
	if command.autoplay {
		t.Error("play=0 did not pause autoplay")
	}
	if !command.hasOffset || command.offset != 2.5 {
		t.Errorf("offset = %v (has=%v)", command.offset, command.hasOffset)
	}

	seek := parseGgtermGraphics([]byte("action=video_seek,id=7,position=12.25"))
	if seek.action != "video_seek" || !seek.hasPosition || seek.position != 12.25 {
		t.Errorf("seek = %+v", seek)
	}

	defaults := parseGgtermGraphics([]byte("action=video,id=1,path=x.mp4"))
	if !defaults.autoplay || !defaults.move {
		t.Errorf("defaults = %+v", defaults)
	}
}

func TestVideoActionsDoNotSpawnWithoutStream(t *testing.T) {
	term := New(Config{Cols: 20, Rows: 10})
	defer term.Close()
	term.Write([]byte("\x1b_ggterm;action=video_play,id=99\x1b\\"))
	term.Write([]byte("\x1b_ggterm;action=video_stop,id=99\x1b\\"))
	if term.VideoCount() != 0 {
		t.Errorf("videos = %d, want 0", term.VideoCount())
	}
}

func TestVideoProtocolWithFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	t.Setenv("GGTERM_FFMPEG", ffmpeg)
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	generate := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=64x48:rate=10",
		"-t", "0.6", "-pix_fmt", "yuv420p", "-y", path,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test video: %v: %s", err, output)
	}

	term := New(Config{Cols: 20, Rows: 10})
	defer term.Close()
	quoted := strings.ReplaceAll(path, "\\", "/")
	term.Write([]byte("\x1b_ggterm;action=video,id=5,path=" + quoted + ",cols=8,rows=4,loop=1\x1b\\"))
	if term.VideoCount() != 1 {
		t.Fatalf("videos = %d, want 1", term.VideoCount())
	}
	snapshot := term.Snapshot(0)
	if len(snapshot.Images) == 0 || snapshot.Images[0].Graphic == nil {
		t.Fatal("video placement is missing")
	}
	if snapshot.Images[0].Graphic.Width <= 0 {
		t.Fatal("video graphic has no size")
	}

	term.Write([]byte("\x1b_ggterm;action=video_pause,id=5\x1b\\"))
	term.Write([]byte("\x1b_ggterm;action=video_stop,id=5\x1b\\"))
	if term.VideoCount() != 0 {
		t.Errorf("videos after stop = %d", term.VideoCount())
	}
	if images := term.Snapshot(0).Images; len(images) != 0 {
		t.Errorf("images after stop = %d", len(images))
	}
}
