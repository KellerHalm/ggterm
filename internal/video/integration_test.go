package video

import (
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPlayerWithRealFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.mp4")
	generate := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=64x48:rate=10",
		"-t", "0.6", "-pix_fmt", "yuv420p", "-y", path,
	)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Skipf("cannot generate a test video: %v: %s", err, output)
	}

	info, err := Probe(path, Options{FFmpeg: ffmpeg})
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 64 || info.Height != 48 {
		t.Errorf("probe = %+v", info)
	}
	if info.Duration <= 0 {
		t.Errorf("duration = %v", info.Duration)
	}

	player, err := Open(path, Options{FFmpeg: ffmpeg, FFprobe: "", MaxFPS: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()
	frame, ok := player.Frame()
	if !ok || frame.Width != 64 || frame.Height != 48 {
		t.Fatalf("frame = %+v ok=%v", frame, ok)
	}
	if len(frame.Pix) != 64*48*4 {
		t.Errorf("pixel bytes = %d", len(frame.Pix))
	}

	deadline := time.Now().Add(3 * time.Second)
	for !player.State().Ended && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !player.State().Ended {
		t.Errorf("player did not finish: %+v", player.State())
	}
}
