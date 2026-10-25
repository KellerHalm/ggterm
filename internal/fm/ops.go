package fm

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CopySelection remembers the highlighted entry for copying.
func (m *Model) CopySelection() bool {
	entry, ok := m.Selected()
	if !ok {
		return false
	}
	m.clipboard = []string{entry.Path}
	m.cut = false
	m.notice = "copied " + entry.Name
	return true
}

// CutSelection remembers the highlighted entry for moving.
func (m *Model) CutSelection() bool {
	entry, ok := m.Selected()
	if !ok {
		return false
	}
	m.clipboard = []string{entry.Path}
	m.cut = true
	m.notice = "cut " + entry.Name
	return true
}

// ClearClipboard forgets the pending copy or move.
func (m *Model) ClearClipboard() {
	m.clipboard = nil
	m.cut = false
}

// Paste copies or moves the clipboard into the current directory.
func (m *Model) Paste() error {
	if len(m.clipboard) == 0 {
		return errors.New("clipboard is empty")
	}
	var lastDest string
	processed := 0
	for _, src := range m.clipboard {
		if m.cut {
			if samePath(filepath.Dir(src), m.cwd) {
				continue
			}
			dst := filepath.Join(m.cwd, filepath.Base(src))
			if isWithin(src, dst) {
				return fmt.Errorf("cannot move %s into itself", filepath.Base(src))
			}
			if exists(dst) && !samePath(src, dst) {
				return fmt.Errorf("%q already exists", filepath.Base(src))
			}
			if err := movePath(src, dst); err != nil {
				return err
			}
			lastDest = dst
			processed++
			continue
		}
		dst := uniquePath(m.cwd, filepath.Base(src))
		if isWithin(src, dst) {
			return fmt.Errorf("cannot copy %s into itself", filepath.Base(src))
		}
		if err := copyPath(src, dst); err != nil {
			return err
		}
		lastDest = dst
		processed++
	}
	if m.cut {
		m.clipboard = nil
		m.cut = false
	}
	if processed == 0 {
		m.notice = "nothing to paste"
		return nil
	}
	if err := m.Refresh(); err != nil {
		return err
	}
	if lastDest != "" {
		m.selectPath(lastDest)
	}
	m.notice = fmt.Sprintf("pasted %d item(s)", processed)
	return nil
}

// DeleteSelection removes the highlighted entry recursively.
func (m *Model) DeleteSelection() error {
	entry, ok := m.Selected()
	if !ok {
		return nil
	}
	if err := os.RemoveAll(entry.Path); err != nil {
		return err
	}
	index := m.index
	if err := m.Refresh(); err != nil {
		return err
	}
	if index >= len(m.visible) {
		index = len(m.visible) - 1
	}
	m.Select(index)
	m.notice = "deleted " + entry.Name
	return nil
}

// RenameSelected renames the highlighted entry inside the current directory.
func (m *Model) RenameSelected(name string) error {
	entry, ok := m.Selected()
	if !ok {
		return nil
	}
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return err
	}
	if name == entry.Name {
		return nil
	}
	dst := filepath.Join(m.cwd, name)
	if _, err := os.Lstat(dst); err == nil && !samePath(entry.Path, dst) {
		return fmt.Errorf("%q already exists", name)
	}
	if err := os.Rename(entry.Path, dst); err != nil {
		return err
	}
	if err := m.Refresh(); err != nil {
		return err
	}
	m.selectPath(dst)
	m.notice = "renamed to " + name
	return nil
}

// CreateEntry creates a file or directory in the current directory.
func (m *Model) CreateEntry(name string, dir bool) error {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return err
	}
	path := filepath.Join(m.cwd, name)
	if dir {
		if err := os.Mkdir(path, 0o755); err != nil {
			return err
		}
	} else {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	if err := m.Refresh(); err != nil {
		return err
	}
	m.selectPath(path)
	if dir {
		m.notice = "created directory " + name
	} else {
		m.notice = "created file " + name
	}
	return nil
}

func validateName(name string) error {
	if name == "" {
		return errors.New("name must not be empty")
	}
	if name == "." || name == ".." {
		return errors.New("invalid name")
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return errors.New("name must not contain a path separator")
	}
	return nil
}

func uniquePath(dir, name string) string {
	target := filepath.Join(dir, name)
	if !exists(target) {
		return target
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base, ext = name, ""
	}
	for i := 1; i < 10000; i++ {
		suffix := " (copy)"
		if i > 1 {
			suffix = fmt.Sprintf(" (copy %d)", i)
		}
		candidate := filepath.Join(dir, base+suffix+ext)
		if !exists(candidate) {
			return candidate
		}
	}
	return filepath.Join(dir, base+" (copy)"+ext)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func isWithin(parent, child string) bool {
	parent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	child, err = filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err == nil {
			if err := os.Symlink(target, dst); err == nil {
				return nil
			}
		}
		info, err = os.Stat(src)
		if err != nil {
			return err
		}
	}
	if info.IsDir() {
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o755
		}
		if err := os.MkdirAll(dst, mode); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyFile(src, dst string, mode fs.FileMode) error {
	input, err := os.Open(src)
	if err != nil {
		return err
	}
	defer input.Close()
	if mode == 0 {
		mode = 0o644
	}
	output, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(dst)
		return err
	}
	return output.Close()
}

func movePath(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}
