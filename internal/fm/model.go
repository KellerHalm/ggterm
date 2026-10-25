// Package fm implements the file manager model used by the UI overlay.
//
// The model owns the current directory, the Miller columns of the yazi-style
// browser and the file operations. It deliberately has no dependency on the
// graphics layer so navigation and filesystem handling can be tested without a
// window.
package fm

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	// maxDirPreview limits how many child entries are exposed for the preview
	// column.
	maxDirPreview = 256
	// maxTextPreview is the number of bytes read from a text file.
	maxTextPreview = 512 << 10
	// maxPreviewLines caps the number of rendered lines.
	maxPreviewLines = 4000
	// maxImagePreview is the largest image file that is decoded for preview.
	maxImagePreview = 32 << 20
	// maxImagePixels protects against decompression bombs and keeps the
	// preview texture within the limits used by the terminal graphics layer.
	maxImagePixels = 16 << 20
)

// Entry is one file or directory shown in a column.
type Entry struct {
	Name    string
	Path    string
	Dir     bool
	Symlink bool
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
}

// Model is a yazi-style file browser rooted at a directory.
type Model struct {
	cwd         string
	entries     []Entry
	visible     []Entry
	index       int
	parent      []Entry
	parentIndex int
	showHidden  bool
	filter      string
	preview     Preview
	clipboard   []string
	cut         bool
	notice      string
}

// New creates a browser rooted at dir. An empty or invalid directory falls
// back to the current working directory.
func New(dir string) (*Model, error) {
	if strings.TrimSpace(dir) == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = cwd
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	m := &Model{cwd: filepath.Clean(abs)}
	if err := m.refresh("", ""); err != nil {
		return nil, err
	}
	return m, nil
}

// Cwd returns the directory currently shown in the middle column.
func (m *Model) Cwd() string { return m.cwd }

// Entries returns the filtered entries of the current directory.
func (m *Model) Entries() []Entry { return m.visible }

// AllEntries returns every entry, including ones hidden by the fuzzy filter.
func (m *Model) AllEntries() []Entry { return m.entries }

// Parent returns the entries of the parent directory.
func (m *Model) Parent() []Entry { return m.parent }

// ParentIndex returns the position of the current directory inside Parent.
func (m *Model) ParentIndex() int { return m.parentIndex }

// Index returns the selected position in the filtered entry list.
func (m *Model) Index() int { return m.index }

// ShowHidden reports whether dotfiles are displayed.
func (m *Model) ShowHidden() bool { return m.showHidden }

// Filter returns the active fuzzy filter.
func (m *Model) Filter() string { return m.filter }

// Notice returns the latest operation message.
func (m *Model) Notice() string { return m.notice }

// SetNotice replaces the operation message.
func (m *Model) SetNotice(message string) { m.notice = message }

// Clipboard reports copied or cut paths and whether they will be moved.
func (m *Model) Clipboard() ([]string, bool) { return m.clipboard, m.cut }

// Preview returns the preview for the selected entry.
func (m *Model) Preview() Preview { return m.preview }

// Selected returns the highlighted entry.
func (m *Model) Selected() (Entry, bool) {
	if m.index < 0 || m.index >= len(m.visible) {
		return Entry{}, false
	}
	return m.visible[m.index], true
}

// Select moves the cursor to an absolute index.
func (m *Model) Select(index int) {
	if len(m.visible) == 0 {
		m.index = 0
		m.preview = Preview{}
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.visible) {
		index = len(m.visible) - 1
	}
	if index == m.index {
		return
	}
	m.index = index
	m.notice = ""
	m.refreshPreview()
}

// Move shifts the selection by delta entries.
func (m *Model) Move(delta int) { m.Select(m.index + delta) }

// Top selects the first entry.
func (m *Model) Top() { m.Select(0) }

// Bottom selects the last entry.
func (m *Model) Bottom() { m.Select(len(m.visible) - 1) }

// ToggleHidden shows or hides dotfiles and reloads the columns.
func (m *Model) ToggleHidden() error {
	m.showHidden = !m.showHidden
	if err := m.Refresh(); err != nil {
		m.showHidden = !m.showHidden
		return err
	}
	return nil
}

// SetFilter applies a fuzzy query to the current entry list.
func (m *Model) SetFilter(query string) {
	query = strings.TrimSpace(query)
	if query == m.filter {
		return
	}
	m.filter = query
	m.applyFilter("")
}

// OpenDir changes the current directory.
func (m *Model) OpenDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("fm: not a directory")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	previousCwd, previousFilter := m.cwd, m.filter
	m.cwd = filepath.Clean(abs)
	m.filter = ""
	if err := m.refresh("", ""); err != nil {
		m.cwd = previousCwd
		m.filter = previousFilter
		return err
	}
	return nil
}

// Back moves to the parent directory and keeps the previous directory
// selected.
func (m *Model) Back() error {
	parent := filepath.Dir(m.cwd)
	if samePath(parent, m.cwd) {
		return nil
	}
	name := filepath.Base(m.cwd)
	abs, err := filepath.Abs(parent)
	if err != nil {
		abs = parent
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("fm: not a directory")
	}
	previousCwd, previousFilter := m.cwd, m.filter
	m.cwd = filepath.Clean(abs)
	m.filter = ""
	if err := m.refresh("", name); err != nil {
		m.cwd = previousCwd
		m.filter = previousFilter
		return err
	}
	return nil
}

// Enter opens the selected directory. For files it returns the entry and
// reports that no navigation happened.
func (m *Model) Enter() (Entry, bool, error) {
	entry, ok := m.Selected()
	if !ok {
		return Entry{}, false, nil
	}
	if !entry.Dir {
		return entry, false, nil
	}
	return entry, true, m.OpenDir(entry.Path)
}

// Refresh reloads the columns and keeps the selected entry when possible.
func (m *Model) Refresh() error { return m.refresh("", "") }

func (m *Model) refresh(preferPath, preferName string) error {
	current, ok := m.Selected()
	previousPath := ""
	if ok {
		previousPath = current.Path
	}

	entries, err := readEntries(m.cwd, m.showHidden)
	if err != nil {
		return err
	}
	parentPath := filepath.Dir(m.cwd)
	if parentPath == "" {
		parentPath = m.cwd
	}
	parent, _ := readEntries(parentPath, m.showHidden)

	m.entries = entries
	m.parent = parent
	m.parentIndex = -1
	for i, entry := range parent {
		if samePath(entry.Path, m.cwd) {
			m.parentIndex = i
			break
		}
	}

	m.applyFilter(previousPath)
	if preferPath != "" {
		m.selectPath(preferPath)
	} else if preferName != "" {
		m.selectName(preferName)
	}
	return nil
}

func (m *Model) applyFilter(preferPath string) {
	if preferPath == "" {
		if entry, ok := m.Selected(); ok {
			preferPath = entry.Path
		}
	}
	m.visible = m.visible[:0]
	if m.filter == "" {
		m.visible = append(m.visible, m.entries...)
	} else {
		for _, entry := range m.entries {
			if matchesQuery(entry.Name, m.filter) {
				m.visible = append(m.visible, entry)
			}
		}
	}
	m.index = 0
	if preferPath != "" {
		m.selectPath(preferPath)
	}
	m.refreshPreview()
}

func (m *Model) selectPath(path string) {
	for i, entry := range m.visible {
		if samePath(entry.Path, path) {
			m.index = i
			m.refreshPreview()
			return
		}
	}
}

func (m *Model) selectName(name string) {
	for i, entry := range m.visible {
		if strings.EqualFold(entry.Name, name) {
			m.index = i
			m.refreshPreview()
			return
		}
	}
}

func (m *Model) refreshPreview() {
	entry, ok := m.Selected()
	if !ok {
		m.preview = Preview{}
		return
	}
	m.preview = buildPreview(entry, m.showHidden)
}

func readEntries(dir string, showHidden bool) ([]Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		name := item.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := item.Info()
		if err != nil {
			continue
		}
		entry := Entry{
			Name:    name,
			Path:    path,
			Dir:     info.IsDir(),
			Symlink: info.Mode()&fs.ModeSymlink != 0,
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
		}
		if entry.Symlink {
			if target, err := os.Stat(path); err == nil {
				entry.Dir = target.IsDir()
				entry.Size = target.Size()
				entry.ModTime = target.ModTime()
			}
		}
		entries = append(entries, entry)
	}
	sortEntries(entries)
	return entries, nil
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Dir != entries[j].Dir {
			return entries[i].Dir
		}
		left := strings.ToLower(entries[i].Name)
		right := strings.ToLower(entries[j].Name)
		if left != right {
			return left < right
		}
		return entries[i].Name < entries[j].Name
	})
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	left, errLeft := filepath.Abs(a)
	right, errRight := filepath.Abs(b)
	if errLeft == nil {
		a = left
	}
	if errRight == nil {
		b = right
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" && strings.EqualFold(a, b) {
		return true
	}
	return a == b
}

func matchesQuery(name, query string) bool {
	name = strings.ToLower(name)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !subsequence(term, name) {
			return false
		}
	}
	return true
}

func subsequence(needle, haystack string) bool {
	if needle == "" {
		return true
	}
	runes := []rune(needle)
	position := 0
	for _, r := range haystack {
		if r == runes[position] {
			position++
			if position == len(runes) {
				return true
			}
		}
	}
	return false
}

// IsImagePath reports whether path has an extension decoded by the preview.
func IsImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return true
	}
	return false
}
