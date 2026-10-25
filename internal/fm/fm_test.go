package fm

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func selectPath(t *testing.T, model *Model, path string) {
	t.Helper()
	for i, entry := range model.Entries() {
		if entry.Path == path {
			model.Select(i)
			return
		}
	}
	t.Fatalf("%s is not in the current column: %+v", path, model.Entries())
}

func TestModelListsAndNavigates(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeFile(t, filepath.Join(root, ".hidden"), "secret\n")
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "src", "app.go"), "package src\n")

	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	entries := model.Entries()
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2 without hidden files", entries)
	}
	if !entries[0].Dir || entries[0].Name != "src" {
		t.Errorf("first entry = %+v, want the src directory", entries[0])
	}
	if model.Index() != 0 {
		t.Errorf("index = %d, want 0", model.Index())
	}

	model.Move(1)
	if entry, _ := model.Selected(); entry.Name != "main.go" {
		t.Errorf("selected = %q, want main.go", entry.Name)
	}
	model.Move(10)
	if entry, _ := model.Selected(); entry.Name != "main.go" {
		t.Errorf("selection was not clamped: %q", entry.Name)
	}
	model.Top()
	if model.Index() != 0 {
		t.Errorf("top index = %d", model.Index())
	}

	if err := model.ToggleHidden(); err != nil {
		t.Fatal(err)
	}
	if len(model.Entries()) != 3 {
		t.Errorf("hidden file not shown: %+v", model.Entries())
	}
}

func TestModelEnterAndBackSelectsPreviousDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "b", "file.txt"), "hello\n")

	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	selectPath(t, model, filepath.Join(root, "b"))
	if _, entered, err := model.Enter(); err != nil || !entered {
		t.Fatalf("Enter = %v %v", entered, err)
	}
	if !strings.HasSuffix(model.Cwd(), "b") {
		t.Fatalf("cwd = %q", model.Cwd())
	}
	if len(model.Entries()) != 1 || model.Entries()[0].Name != "file.txt" {
		t.Errorf("child entries = %+v", model.Entries())
	}
	if err := model.Back(); err != nil {
		t.Fatal(err)
	}
	if model.Cwd() != root {
		t.Errorf("cwd after back = %q", model.Cwd())
	}
	if entry, _ := model.Selected(); entry.Name != "b" {
		t.Errorf("selected after back = %q, want b", entry.Name)
	}
}

func TestModelFilterAndParentColumn(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.go", "README.md", "script.sh", "src"} {
		path := filepath.Join(root, name)
		if name == "src" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		writeFile(t, path, name+"\n")
	}

	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	model.SetFilter("rm")
	names := make([]string, len(model.Entries()))
	for i, entry := range model.Entries() {
		names[i] = entry.Name
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "README.md") {
		t.Errorf("fuzzy filter missed README.md: %v", names)
	}
	if strings.Contains(joined, "script.sh") {
		t.Errorf("fuzzy filter kept script.sh: %v", names)
	}
	model.SetFilter("")
	if entry := model.Parent(); len(entry) == 0 {
		t.Error("parent column is empty")
	}
	if model.ParentIndex() < 0 {
		t.Error("current directory is not marked in the parent column")
	}
}

func TestModelTextAndImagePreview(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "code.go"), "package main\n\n// comment\nfunc main() {\n\tprintln(42)\n}\n")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 50), G: uint8(y * 50), B: 200, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pixel.png"), encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	selectPath(t, model, filepath.Join(root, "code.go"))
	preview := model.Preview()
	if preview.Kind != PreviewText || len(preview.Lines) == 0 {
		t.Fatalf("text preview = %+v", preview)
	}
	if !hasToken(preview.Lines, TokenKeyword) || !hasToken(preview.Lines, TokenComment) || !hasToken(preview.Lines, TokenNumber) {
		t.Errorf("syntax highlighting is missing: %+v", preview.Lines)
	}

	selectPath(t, model, filepath.Join(root, "pixel.png"))
	preview = model.Preview()
	if preview.Kind != PreviewImage || preview.Image == nil {
		t.Fatalf("image preview = %+v", preview)
	}
	if bounds := preview.Image.Bounds(); bounds.Dx() != 4 || bounds.Dy() != 4 {
		t.Errorf("image bounds = %v", bounds)
	}
}

func TestModelVideoPreview(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "clip.mp4"), "not a real video")
	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	selectPath(t, model, filepath.Join(root, "clip.mp4"))
	if preview := model.Preview(); preview.Kind != PreviewVideo {
		t.Fatalf("preview = %+v, want video", preview)
	}
	for _, path := range []string{"a.mp4", "b.MKV", "c.webm", "d.mov", "e.m4v"} {
		if !IsVideoPath(path) {
			t.Errorf("IsVideoPath(%q) = false", path)
		}
	}
	if IsVideoPath("image.png") || IsVideoPath("notes.txt") {
		t.Error("non-video path was classified as video")
	}
}

func TestModelBinaryPreview(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if preview := model.Preview(); preview.Kind != PreviewBinary {
		t.Errorf("preview = %+v, want binary", preview)
	}
}

func TestModelFileOperations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "alpha")
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "dir", "nested.txt"), "nested")

	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	selectPath(t, model, filepath.Join(root, "a.txt"))
	if !model.CopySelection() {
		t.Fatal("CopySelection returned false")
	}
	if err := model.Paste(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "a (copy).txt")); err != nil {
		t.Errorf("copy was not created: %v", err)
	}

	selectPath(t, model, filepath.Join(root, "a.txt"))
	if !model.CutSelection() {
		t.Fatal("CutSelection returned false")
	}
	if err := os.Mkdir(filepath.Join(root, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := model.OpenDir(filepath.Join(root, "target")); err != nil {
		t.Fatal(err)
	}
	if err := model.Paste(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "a.txt")); err != nil {
		t.Errorf("move did not reach the target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("source still exists after cut: %v", err)
	}

	selectPath(t, model, filepath.Join(root, "target", "a.txt"))
	if err := model.RenameSelected("renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "renamed.txt")); err != nil {
		t.Errorf("rename failed: %v", err)
	}

	if err := model.CreateEntry("created.txt", false); err != nil {
		t.Fatal(err)
	}
	if err := model.CreateEntry("created-dir", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "created.txt")); err != nil {
		t.Errorf("file was not created: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "target", "created-dir")); err != nil || !info.IsDir() {
		t.Errorf("directory was not created: %v", err)
	}

	if err := model.DeleteSelection(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "created-dir")); !os.IsNotExist(err) {
		t.Errorf("delete failed: %v", err)
	}

	model.OpenDir(root)
	selectPath(t, model, filepath.Join(root, "dir"))
	model.CopySelection()
	if err := model.Paste(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dir (copy)", "nested.txt")); err != nil {
		t.Errorf("recursive directory copy failed: %v", err)
	}
}

func hasToken(lines []Line, token Token) bool {
	for _, line := range lines {
		for _, span := range line {
			if span.Token == token {
				return true
			}
		}
	}
	return false
}

func TestHighlightBlockCommentsAndKeys(t *testing.T) {
	lines := Highlight("main.go", []string{"/* start", "still comment", "*/ func main() {}", "x := 42"})
	if !hasToken(lines, TokenComment) || !hasToken(lines, TokenKeyword) {
		t.Errorf("go highlighting failed: %+v", lines)
	}
	jsonLines := Highlight("config.json", []string{`{"name": "ggterm", "count": 2, "ok": true}`})
	if !hasToken(jsonLines, TokenKey) || !hasToken(jsonLines, TokenNumber) {
		t.Errorf("json highlighting failed: %+v", jsonLines)
	}
	md := Highlight("README.md", []string{"# Title", "", "text"})
	if md[0][0].Token != TokenKeyword {
		t.Errorf("markdown heading = %+v", md[0])
	}
}

func TestUniquePathDotfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".bashrc"), "export A=1\n")
	got := uniquePath(dir, ".bashrc")
	if want := filepath.Join(dir, ".bashrc (copy)"); got != want {
		t.Errorf("uniquePath = %q, want %q", got, want)
	}
}

func TestValidateName(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b", `a\b`} {
		if err := validateName(name); err == nil {
			t.Errorf("validateName(%q) accepted an invalid name", name)
		}
	}
	for _, name := range []string{"file.txt", ".hidden", "with space"} {
		if err := validateName(name); err != nil {
			t.Errorf("validateName(%q) = %v", name, err)
		}
	}
}

func TestPasteRefusesToOverwriteOnMove(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "source")
	if err := os.Mkdir(filepath.Join(root, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "target", "a.txt"), "existing")

	model, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	selectPath(t, model, filepath.Join(root, "a.txt"))
	model.CutSelection()
	if err := model.OpenDir(filepath.Join(root, "target")); err != nil {
		t.Fatal(err)
	}
	if err := model.Paste(); err == nil {
		t.Fatal("Paste overwrote an existing file")
	}
	if data, err := os.ReadFile(filepath.Join(root, "target", "a.txt")); err != nil || string(data) != "existing" {
		t.Errorf("target file changed: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Errorf("source disappeared: %v", err)
	}
}
