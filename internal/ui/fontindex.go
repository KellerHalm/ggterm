package ui

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// fontStyle selects one of the four faces of a font family.
type fontStyle uint8

const (
	fontRegular fontStyle = iota
	fontBold
	fontItalic
	fontBoldItalic
)

func (s fontStyle) bold() bool   { return s == fontBold || s == fontBoldItalic }
func (s fontStyle) italic() bool { return s == fontItalic || s == fontBoldItalic }

// fontIndex maps normalised family names to the best file for each face.
type fontIndex struct {
	families map[string][4]string
}

// lookup returns a file for the requested face. When the exact face is not
// available it falls back to the regular face and reports exact=false.
func (i *fontIndex) lookup(family string, style fontStyle) (string, bool) {
	entry, ok := i.families[normalizeFamily(family)]
	if !ok {
		return "", false
	}
	if path := entry[style]; path != "" {
		return path, true
	}
	if path := entry[fontRegular]; path != "" {
		return path, false
	}
	for _, path := range entry {
		if path != "" {
			return path, false
		}
	}
	return "", false
}

// lookupExact returns only the requested face, never a substitute.
func (i *fontIndex) lookupExact(family string, style fontStyle) (string, bool) {
	entry, ok := i.families[normalizeFamily(family)]
	if !ok {
		return "", false
	}
	path := entry[style]
	return path, path != ""
}

type fontIndexCacheEntry struct {
	key   string
	index *fontIndex
}

var fontIndexCache struct {
	mu    sync.Mutex
	entry *fontIndexCacheEntry
}

// fontIndexFor builds (or reuses) an index for the given directories.
func fontIndexFor(dirs []string) *fontIndex {
	dirs = cleanDirs(dirs)
	key := strings.Join(dirs, "\x00")

	fontIndexCache.mu.Lock()
	defer fontIndexCache.mu.Unlock()
	if fontIndexCache.entry != nil && fontIndexCache.entry.key == key {
		return fontIndexCache.entry.index
	}
	index := buildFontIndex(dirs)
	fontIndexCache.entry = &fontIndexCacheEntry{key: key, index: index}
	return index
}

func cleanDirs(dirs []string) []string {
	seen := make(map[string]bool, len(dirs))
	cleaned := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if seen[dir] {
			continue
		}
		seen[dir] = true
		cleaned = append(cleaned, dir)
	}
	sort.Strings(cleaned)
	return cleaned
}

func buildFontIndex(dirs []string) *fontIndex {
	index := &fontIndex{families: make(map[string][4]string)}
	for _, root := range dirs {
		root := root
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if path == root {
					return nil
				}
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil || strings.Count(rel, string(filepath.Separator)) >= 3 {
					return fs.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".ttf", ".otf", ".ttc", ".otc":
			default:
				return nil
			}
			family, style := classifyFontFilename(entry.Name())
			if family == "" {
				return nil
			}
			faces := index.families[family]
			if faces[style] == "" || weightRank(entry.Name()) < weightRank(filepath.Base(faces[style])) {
				faces[style] = path
				index.families[family] = faces
			}
			return nil
		})
	}
	return index
}

// fontStyleWords can be dropped when they are a whole trailing component of a
// file name. Distinct families and weights such as "Arial Narrow", "Roboto
// Condensed", "Arial Black", "Times New Roman" and "Roboto Light" deliberately
// are not in this list, so they stay separate families.
var fontStyleWords = map[string]bool{
	"extrabold": true, "ultrabold": true, "semibold": true, "demibold": true,
	"bolditalic": true, "boldoblique": true,
	"italic": true, "oblique": true, "regular": true, "normal": true,
	"bold": true,
}

// concatenatedStyleSuffixes are safe to trim from unseparated names such as
// "JetBrainsMonoBold". Weight words like "Narrow" are excluded because they can
// be part of a distinct family name ("Arial Narrow").
var concatenatedStyleSuffixes = []string{
	"bolditalic", "boldoblique", "italic", "oblique", "bold",
}

// classifyFontFilename derives a family key and face style from a file name.
// Parsing names keeps the scan cheap: font files themselves are only opened
// when they are actually used for rendering.
func classifyFontFilename(name string) (string, fontStyle) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	lower := strings.ToLower(base)
	style := fontRegular
	if strings.Contains(lower, "bold") {
		style = fontBold
	}
	if strings.Contains(lower, "italic") || strings.Contains(lower, "oblique") {
		style = fontBoldItalic
		if !strings.Contains(lower, "bold") {
			style = fontItalic
		}
	}
	family := familyKeyFromName(base)
	return family, style
}

// weightRank prefers a regular bold file over semibold or extrabold when a
// family ships several bold-ish faces.
func weightRank(name string) int {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "extrabold"), strings.Contains(lower, "ultrabold"):
		return 2
	case strings.Contains(lower, "semibold"), strings.Contains(lower, "demibold"):
		return 1
	default:
		return 0
	}
}

func familyKeyFromName(base string) string {
	lower := strings.ToLower(base)
	// Drop whole style components separated by "-", "_", spaces, etc. so that
	// family names containing those words ("Bookman", "Blackletter") survive.
	for {
		cut := strings.LastIndexFunc(lower, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
		})
		if cut < 0 {
			break
		}
		token := lower[cut+1:]
		if token == "" || !fontStyleWords[token] {
			break
		}
		lower = strings.TrimRight(lower[:cut], " \t_-.")
	}
	// Handle unseparated names like "JetBrainsMonoBold".
	for _, suffix := range concatenatedStyleSuffixes {
		if len(lower) > len(suffix) && strings.HasSuffix(lower, suffix) {
			lower = strings.TrimSuffix(lower, suffix)
			break
		}
	}
	return normalizeFamily(lower)
}

func normalizeFamily(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// familyKeyFromPath returns the family key encoded in a font file name.
func familyKeyFromPath(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return familyKeyFromName(base)
}

// systemFontDirs lists the platform font directories. GGTERM_FONT_DIRS adds
// extra directories (separated by the OS path list separator).
func systemFontDirs() []string {
	var dirs []string
	add := func(dir string) {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	switch runtime.GOOS {
	case "windows":
		if windir := os.Getenv("WINDIR"); windir != "" {
			add(filepath.Join(windir, "Fonts"))
		}
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			add(filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		}
	case "darwin":
		add("/System/Library/Fonts")
		add("/Library/Fonts")
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, "Library", "Fonts"))
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".local", "share", "fonts"))
			add(filepath.Join(home, ".fonts"))
		}
		add("/usr/share/fonts")
		add("/usr/local/share/fonts")
	}
	if extra := os.Getenv("GGTERM_FONT_DIRS"); extra != "" {
		for _, dir := range filepath.SplitList(extra) {
			add(dir)
		}
	}
	return dirs
}
