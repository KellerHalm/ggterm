package video

import (
	"bytes"
	"io"
	"os/exec"
	"testing"
	"time"
)

func TestParseFFprobeJSON(t *testing.T) {
	data := []byte(`{
		"streams": [
			{"width": 1920, "height": 1080, "avg_frame_rate": "30000/1001", "r_frame_rate": "30/1", "duration": "12.5"}
		],
		"format": {"duration": "13.25"}
	}`)
	info, err := parseFFprobeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 1920 || info.Height != 1080 {
		t.Errorf("size = %dx%d", info.Width, info.Height)
	}
	if info.Duration != 12500*time.Millisecond {
		t.Errorf("duration = %v", info.Duration)
	}
	if info.FPS < 29.9 || info.FPS > 30.0 {
		t.Errorf("fps = %f", info.FPS)
	}
}

func TestParseFFprobeFallsBackToFormatDuration(t *testing.T) {
	data := []byte(`{
		"streams": [{"width": 640, "height": 360, "avg_frame_rate": "0/0", "r_frame_rate": "25/1", "duration": "N/A"}],
		"format": {"duration": "3.5"}
	}`)
	info, err := parseFFprobeJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if info.Duration != 3500*time.Millisecond {
		t.Errorf("duration = %v", info.Duration)
	}
	if info.FPS != 25 {
		t.Errorf("fps = %f", info.FPS)
	}
}

func TestParseFFmpegInfo(t *testing.T) {
	output := `Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'movie.mp4':
  Duration: 00:01:02.50, start: 0.000000, bitrate: 1234 kb/s
  Stream #0:0[0x1](und): Video: h264 (High), yuv420p(tv, progressive), 1280x720 [SAR 1:1 DAR 16:9], 29.97 fps, 29.97 tbr, 90k tbn
`
	info, err := parseFFmpegInfo(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 1280 || info.Height != 720 {
		t.Errorf("size = %dx%d", info.Width, info.Height)
	}
	if info.Duration != 62*time.Second+500*time.Millisecond {
		t.Errorf("duration = %v", info.Duration)
	}
	if info.FPS < 29.9 || info.FPS > 30.0 {
		t.Errorf("fps = %f", info.FPS)
	}
}

func TestParseFFmpegInfoRejectsAudioOnly(t *testing.T) {
	if _, err := parseFFmpegInfo("Stream #0:0: Audio: aac, 48000 Hz, stereo\n"); err == nil {
		t.Fatal("audio-only input was accepted")
	}
}

func TestFitSize(t *testing.T) {
	if w, h := fitSize(1920, 1080, Options{MaxDimension: 960}); w != 960 || h != 540 {
		t.Errorf("landscape fit = %dx%d", w, h)
	}
	if w, h := fitSize(1080, 1920, Options{MaxDimension: 960}); w != 540 || h != 960 {
		t.Errorf("portrait fit = %dx%d", w, h)
	}
	if w, h := fitSize(4000, 2000, Options{Width: 640, Height: 360}); w != 640 || h != 320 {
		t.Errorf("box fit = %dx%d", w, h)
	}
	if w, h := fitSize(320, 200, Options{MaxDimension: 960}); w != 320 || h != 200 {
		t.Errorf("small source should not be upscaled: %dx%d", w, h)
	}
}

func TestEffectiveFPS(t *testing.T) {
	if got := effectiveFPS(60, 30); got != 30 {
		t.Errorf("fps cap = %f", got)
	}
	if got := effectiveFPS(24, 30); got != 24 {
		t.Errorf("source fps = %f", got)
	}
	if got := effectiveFPS(0, 0); got != defaultMaxFPS {
		t.Errorf("default fps = %f", got)
	}
}

func TestPlayerReadsFramesAndControls(t *testing.T) {
	info := Info{Width: 4, Height: 4, Duration: 500 * time.Millisecond, FPS: 10}
	opts := Options{
		FFmpeg: "fake-ffmpeg",
		Info:   &info,
		MaxFPS: 10,
		start:  fakeStarter(5, 4, 4),
	}
	player, err := Open("fake.mp4", opts)
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()

	frame, ok := player.Frame()
	if !ok || frame.Width != 4 || frame.Height != 4 || len(frame.Pix) != 4*4*4 {
		t.Fatalf("first frame = %+v ok=%v", frame, ok)
	}

	// Wait for the finite stream to finish.
	deadline := time.Now().Add(2 * time.Second)
	for {
		state := player.State()
		if state.Ended || state.Err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("stream did not finish: %+v", state)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state := player.State(); !state.Ended || state.Err != nil {
		t.Errorf("final state = %+v", state)
	}

	if !player.Toggle() {
		t.Error("Toggle did not start the finished stream")
	}
	if !player.State().Playing {
		t.Error("play did not restart the finished stream")
	}

	player.Pause()
	if player.State().Playing {
		t.Error("pause did not stop playback")
	}

	if before, ok := player.Frame(); !ok || before.Seq == 0 {
		t.Error("frame disappeared after pause")
	}

	player.Seek(100 * time.Millisecond)
	if state := player.State(); state.Position < 100*time.Millisecond {
		t.Errorf("seek position = %v", state.Position)
	}

	player.Stop()
	if state := player.State(); !state.Stopped || state.Playing {
		t.Errorf("stop state = %+v", state)
	}
	if _, ok := player.Frame(); ok {
		t.Error("stop kept the last frame")
	}
}

func fakeStarter(frames, width, height int) func(*exec.Cmd) (io.ReadCloser, error) {
	return func(*exec.Cmd) (io.ReadCloser, error) {
		buf := make([]byte, 0, frames*width*height*4)
		for frame := 0; frame < frames; frame++ {
			for i := 0; i < width*height; i++ {
				buf = append(buf, byte(frame*20), byte(i), byte(255-frame), 0xff)
			}
		}
		return io.NopCloser(bytes.NewReader(buf)), nil
	}
}
