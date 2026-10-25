package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	durationRe = regexp.MustCompile(`Duration:\s*(\d+):(\d{2}):(\d{2}(?:\.\d+)?)`)
	sizeRe     = regexp.MustCompile(`(\d{2,5})x(\d{2,5})`)
	fpsRe      = regexp.MustCompile(`([\d.]+)\s+fps`)
	fpsFracRe  = regexp.MustCompile(`(\d+)/(\d+)\s+fps`)
)

// Probe reads the video dimensions, duration and frame rate of path. It uses
// ffprobe when available and falls back to parsing ffmpeg's stream summary.
func Probe(path string, opts Options) (Info, error) {
	if strings.TrimSpace(path) == "" {
		return Info{}, errors.New("video: empty path")
	}
	if probe, ok := findFFprobe(opts.FFprobe); ok {
		info, err := probeFFprobe(probe, path)
		if err == nil {
			return info, nil
		}
	}
	ffmpeg, err := findFFmpeg(opts.FFmpeg)
	if err != nil {
		return Info{}, err
	}
	return probeFFmpeg(ffmpeg, path)
}

func probeFFprobe(tool, path string) (Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate,r_frame_rate,duration:format=duration",
		"-of", "json",
		path,
	)
	cmd.SysProcAttr = sysProcAttr()
	output, err := cmd.Output()
	if err != nil {
		return Info{}, err
	}
	return parseFFprobeJSON(output)
}

type ffprobeDocument struct {
	Streams []struct {
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		AverageFPS string `json:"avg_frame_rate"`
		RealFPS    string `json:"r_frame_rate"`
		Duration   string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func parseFFprobeJSON(data []byte) (Info, error) {
	var document ffprobeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return Info{}, err
	}
	if len(document.Streams) == 0 {
		return Info{}, errors.New("video: no video stream")
	}
	stream := document.Streams[0]
	info := Info{Width: stream.Width, Height: stream.Height}
	if info.Width <= 0 || info.Height <= 0 {
		return Info{}, errors.New("video: no video stream")
	}
	info.FPS = parseFPS(stream.AverageFPS)
	if info.FPS <= 0 {
		info.FPS = parseFPS(stream.RealFPS)
	}
	if duration, ok := parseSeconds(stream.Duration); ok {
		info.Duration = duration
	} else if duration, ok := parseSeconds(document.Format.Duration); ok {
		info.Duration = duration
	}
	return info, nil
}

func probeFFmpeg(tool, path string) (Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "-hide_banner", "-nostdin", "-i", path)
	cmd.SysProcAttr = sysProcAttr()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	_ = cmd.Run()
	return parseFFmpegInfo(output.String())
}

func parseFFmpegInfo(output string) (Info, error) {
	info := Info{}
	videoLine := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Duration:") {
			if match := durationRe.FindStringSubmatch(line); match != nil {
				info.Duration = clockDuration(match[1], match[2], match[3])
			}
		}
		if strings.Contains(line, "Video:") && videoLine == "" {
			videoLine = line
		}
	}
	if videoLine == "" {
		return Info{}, errors.New("video: no video stream")
	}
	if match := sizeRe.FindStringSubmatch(videoLine); match != nil {
		info.Width, _ = strconv.Atoi(match[1])
		info.Height, _ = strconv.Atoi(match[2])
	}
	if info.Width <= 0 || info.Height <= 0 {
		return Info{}, errors.New("video: no video stream")
	}
	if match := fpsFracRe.FindStringSubmatch(videoLine); match != nil {
		numerator, _ := strconv.ParseFloat(match[1], 64)
		denominator, _ := strconv.ParseFloat(match[2], 64)
		if denominator > 0 {
			info.FPS = numerator / denominator
		}
	}
	if info.FPS <= 0 {
		if match := fpsRe.FindStringSubmatch(videoLine); match != nil {
			info.FPS, _ = strconv.ParseFloat(match[1], 64)
		}
	}
	return info, nil
}

func parseFPS(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return 0
	}
	if numerator, denominator, ok := strings.Cut(value, "/"); ok {
		top, errTop := strconv.ParseFloat(numerator, 64)
		bottom, errBottom := strconv.ParseFloat(denominator, 64)
		if errTop == nil && errBottom == nil && bottom > 0 {
			return top / bottom
		}
		return 0
	}
	fps, _ := strconv.ParseFloat(value, 64)
	return fps
}

func parseSeconds(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "N/A") {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

func clockDuration(hours, minutes, seconds string) time.Duration {
	h, _ := strconv.Atoi(hours)
	m, _ := strconv.Atoi(minutes)
	s, _ := strconv.ParseFloat(seconds, 64)
	return time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute +
		time.Duration(s*float64(time.Second))
}
