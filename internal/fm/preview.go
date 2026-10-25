package fm

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// PreviewKind describes what the right-hand column contains.
type PreviewKind uint8

const (
	PreviewNone PreviewKind = iota
	PreviewDir
	PreviewText
	PreviewImage
	PreviewVideo
	PreviewBinary
	PreviewError
)

// Preview is the content shown in the preview column.
type Preview struct {
	Kind       PreviewKind
	Path       string
	Size       int64
	ModTime    time.Time
	DirEntries []Entry
	Lines      []Line
	Image      image.Image
	Truncated  bool
	Message    string
}

func buildPreview(entry Entry, showHidden bool) Preview {
	preview := Preview{Path: entry.Path, Size: entry.Size, ModTime: entry.ModTime}
	if entry.Dir {
		entries, err := readEntries(entry.Path, showHidden)
		if err != nil {
			preview.Kind = PreviewError
			preview.Message = err.Error()
			return preview
		}
		if len(entries) > maxDirPreview {
			entries = entries[:maxDirPreview]
			preview.Truncated = true
		}
		preview.Kind = PreviewDir
		preview.DirEntries = entries
		return preview
	}
	if entry.Size <= 0 {
		preview.Kind = PreviewBinary
		preview.Message = "empty file"
		return preview
	}
	if IsVideoPath(entry.Path) {
		preview.Kind = PreviewVideo
		return preview
	}
	if IsImagePath(entry.Path) {
		if imagePreview, message := loadImagePreview(entry.Path); imagePreview != nil {
			preview.Kind = PreviewImage
			preview.Image = imagePreview
			return preview
		} else if message != "" {
			preview.Kind = PreviewBinary
			preview.Message = message
			return preview
		}
	}
	text, truncated, binary, err := readTextPreview(entry.Path)
	if err != nil {
		preview.Kind = PreviewError
		preview.Message = err.Error()
		return preview
	}
	if binary {
		preview.Kind = PreviewBinary
		preview.Message = "binary file"
		return preview
	}
	preview.Kind = PreviewText
	preview.Truncated = truncated
	lines := strings.Split(text, "\n")
	if len(lines) > maxPreviewLines {
		lines = lines[:maxPreviewLines]
		preview.Truncated = true
	}
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		lines[i] = strings.ReplaceAll(line, "\t", "    ")
	}
	preview.Lines = Highlight(entry.Path, lines)
	return preview
}

func loadImagePreview(path string) (image.Image, string) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err.Error()
	}
	if info.Size() > maxImagePreview {
		return nil, fmt.Sprintf("image is too large to preview (%s)", HumanSize(info.Size()))
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err.Error()
	}
	defer file.Close()

	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, "unsupported image format"
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, fmt.Sprintf("image is too large to preview (%dx%d)", cfg.Width, cfg.Height)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err.Error()
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, "invalid image data"
	}
	return img, ""
}

func readTextPreview(path string) (text string, truncated, binary bool, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, false, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxTextPreview+1))
	if err != nil {
		return "", false, false, err
	}
	if len(data) > maxTextPreview {
		data = data[:maxTextPreview]
		truncated = true
	}
	if looksBinary(data) {
		return "", false, true, nil
	}
	return strings.ToValidUTF8(string(data), "\uFFFD"), truncated, false, nil
}

func looksBinary(data []byte) bool {
	sample := data
	if len(sample) > 8192 {
		sample = sample[:8192]
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return true
	}
	if !utf8.Valid(sample) {
		return true
	}
	control := 0
	for _, b := range sample {
		switch b {
		case '\n', '\r', '\t', '\f', '\b':
			continue
		}
		if b < 0x20 {
			control++
		}
	}
	return control*20 > len(sample)
}

// HumanSize formats a byte count for the status line.
func HumanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// BaseName returns the display name of a path.
func BaseName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return path
	}
	return name
}
