package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gomono"

	"ggterm/internal/config"
	"ggterm/internal/terminal"
)

// faceVariant is one resolved face together with the transformations that are
// still needed because the font has no real bold or italic face.
type faceVariant struct {
	face            text.Face
	syntheticBold   bool
	syntheticItalic bool
}

type fontSet struct {
	variants  [4]faceVariant
	metrics   text.Metrics
	ligatures bool
}

func (f *fontSet) selectFace(style terminal.Style) faceVariant {
	index := fontRegular
	if style.Bold {
		index |= 1
	}
	if style.Italic {
		index |= 2
	}
	return f.variants[index]
}

type fontRef struct {
	ident    string
	source   *text.GoTextFaceSource
	meta     text.Metadata
	embedded bool
}

type cachedFontSource struct {
	ref     fontRef
	modTime time.Time
	size    int64
}

var (
	fontSourceCache = struct {
		sync.Mutex
		refs map[string]cachedFontSource
	}{refs: make(map[string]cachedFontSource)}

	builtinFont struct {
		once sync.Once
		ref  fontRef
		err  error
	}
)

func loadFontSet(font config.Font) (*fontSet, error) {
	primary, err := primaryFontRef(font)
	if err != nil {
		return nil, err
	}
	refs := [4]fontRef{primary, primary, primary, primary}

	if ref, ok, err := variantFontRef(font, fontBold, primary); err != nil {
		return nil, err
	} else if ok {
		refs[fontBold] = ref
	}
	if ref, ok, err := variantFontRef(font, fontItalic, primary); err != nil {
		return nil, err
	} else if ok {
		refs[fontItalic] = ref
	}
	if ref, ok, err := variantFontRef(font, fontBoldItalic, primary); err != nil {
		return nil, err
	} else if ok {
		refs[fontBoldItalic] = ref
	} else {
		switch {
		case refStyle(refs[fontBold]).bold():
			refs[fontBoldItalic] = refs[fontBold]
		case refStyle(refs[fontItalic]).italic():
			refs[fontBoldItalic] = refs[fontItalic]
		}
	}

	set := &fontSet{ligatures: font.Ligatures}
	for i := range refs {
		style := fontStyle(i)
		face, err := buildFace(refs[i], font, style, make(map[string]bool))
		if err != nil {
			return nil, err
		}
		actual := refStyle(refs[i])
		set.variants[i] = faceVariant{
			face:            face,
			syntheticBold:   style.bold() && !actual.bold(),
			syntheticItalic: style.italic() && !actual.italic(),
		}
	}
	set.metrics = set.variants[fontRegular].face.Metrics()
	return set, nil
}

// refStyle merges the font's own metadata with its file name. Some fonts
// report a generic weight even in their bold file, so the explicit "Bold" or
// "Italic" in the file name is treated as authoritative too.
func refStyle(ref fontRef) fontStyle {
	bold := ref.meta.Weight >= text.WeightBold
	italic := ref.meta.Style == text.StyleItalic
	if !ref.embedded {
		_, fileStyle := classifyFontFilename(filepath.Base(ref.ident))
		bold = bold || fileStyle.bold()
		italic = italic || fileStyle.italic()
	}
	switch {
	case bold && italic:
		return fontBoldItalic
	case bold:
		return fontBold
	case italic:
		return fontItalic
	default:
		return fontRegular
	}
}

func buildFace(primary fontRef, font config.Font, style fontStyle, seen map[string]bool) (text.Face, error) {
	faces := []text.Face{&text.GoTextFace{Source: primary.source, Size: font.Size}}
	for _, fallback := range font.Fallback {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" {
			continue
		}
		ref, err := fallbackFontRef(fallback, style)
		if err != nil {
			return nil, fmt.Errorf("fallback font %q: %w", fallback, err)
		}
		if ref.ident == primary.ident || seen[ref.ident] {
			continue
		}
		seen[ref.ident] = true
		faces = append(faces, &text.GoTextFace{Source: ref.source, Size: font.Size})
	}
	if len(faces) == 1 {
		return faces[0], nil
	}
	return text.NewMultiFace(faces...)
}

func primaryFontRef(font config.Font) (fontRef, error) {
	spec := strings.TrimSpace(font.Path)
	if spec == "" {
		spec = strings.TrimSpace(font.Family)
	}
	if spec == "" || isBuiltinFontName(spec) {
		return builtinFontRef()
	}
	if isFile(spec) {
		return loadFontFile(spec)
	}
	if looksLikePath(spec) {
		return fontRef{}, fmt.Errorf("font %q: file not found", spec)
	}
	path, _ := fontIndexFor(systemFontDirs()).lookup(spec, fontRegular)
	if path == "" {
		return fontRef{}, fmt.Errorf("font %q: family not found in the system font directories; set font.path to a .ttf/.otf file", spec)
	}
	return loadFontFile(path)
}

func variantFontRef(font config.Font, style fontStyle, primary fontRef) (fontRef, bool, error) {
	if explicit := explicitVariantPath(font, style); explicit != "" {
		ref, err := resolveFontSpec(explicit, style, true)
		if err != nil {
			return fontRef{}, false, err
		}
		if ref.ident == primary.ident {
			return fontRef{}, false, nil
		}
		return ref, true, nil
	}

	if primary.ident != "" && !primary.embedded {
		dir := filepath.Dir(primary.ident)
		family := familyKeyFromPath(primary.ident)
		if path, ok := fontIndexFor([]string{dir}).lookupExact(family, style); ok {
			if filepath.Clean(path) != filepath.Clean(primary.ident) {
				ref, err := loadFontFile(path)
				if err != nil {
					return fontRef{}, false, err
				}
				return ref, true, nil
			}
		}
	}

	family := strings.TrimSpace(font.Family)
	if family != "" && !isBuiltinFontName(family) && !looksLikePath(family) {
		if path, ok := fontIndexFor(systemFontDirs()).lookupExact(family, style); ok {
			ref, err := loadFontFile(path)
			if err != nil {
				return fontRef{}, false, err
			}
			return ref, true, nil
		}
	}
	return fontRef{}, false, nil
}

func explicitVariantPath(font config.Font, style fontStyle) string {
	switch style {
	case fontBold:
		return strings.TrimSpace(font.Bold)
	case fontItalic:
		return strings.TrimSpace(font.Italic)
	case fontBoldItalic:
		return strings.TrimSpace(font.BoldItalic)
	default:
		return ""
	}
}

func fallbackFontRef(spec string, style fontStyle) (fontRef, error) {
	if isBuiltinFontName(spec) {
		return builtinFontRef()
	}
	if isFile(spec) {
		return loadFontFile(spec)
	}
	if looksLikePath(spec) {
		return fontRef{}, fmt.Errorf("file not found: %s", spec)
	}
	path, exact := fontIndexFor(systemFontDirs()).lookup(spec, style)
	if path == "" {
		return fontRef{}, fmt.Errorf("family %q not found", spec)
	}
	_ = exact
	return loadFontFile(path)
}

func resolveFontSpec(spec string, style fontStyle, strict bool) (fontRef, error) {
	if isBuiltinFontName(spec) {
		return builtinFontRef()
	}
	if isFile(spec) {
		return loadFontFile(spec)
	}
	if looksLikePath(spec) {
		return fontRef{}, fmt.Errorf("file not found: %s", spec)
	}
	path, exact := fontIndexFor(systemFontDirs()).lookup(spec, style)
	if path == "" || (strict && !exact) {
		return fontRef{}, fmt.Errorf("font family %q has no %s face", spec, styleName(style))
	}
	return loadFontFile(path)
}

func styleName(style fontStyle) string {
	switch style {
	case fontBold:
		return "bold"
	case fontItalic:
		return "italic"
	case fontBoldItalic:
		return "bold italic"
	default:
		return "regular"
	}
}

func builtinFontRef() (fontRef, error) {
	builtinFont.once.Do(func() {
		source, err := sourceFromBytes(gomono.TTF)
		if err != nil {
			builtinFont.err = err
			return
		}
		builtinFont.ref = fontRef{
			ident:    "builtin:gomono",
			source:   source,
			meta:     source.Metadata(),
			embedded: true,
		}
	})
	return builtinFont.ref, builtinFont.err
}

func loadFontFile(path string) (fontRef, error) {
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return fontRef{}, err
	}

	fontSourceCache.Lock()
	if cached, ok := fontSourceCache.refs[clean]; ok &&
		cached.modTime.Equal(info.ModTime()) && cached.size == info.Size() {
		fontSourceCache.Unlock()
		return cached.ref, nil
	}
	fontSourceCache.Unlock()

	data, err := os.ReadFile(clean)
	if err != nil {
		return fontRef{}, err
	}
	var source *text.GoTextFaceSource
	switch strings.ToLower(filepath.Ext(clean)) {
	case ".ttc", ".otc":
		sources, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
		if err != nil {
			return fontRef{}, err
		}
		if len(sources) == 0 {
			return fontRef{}, fmt.Errorf("font collection %q is empty", clean)
		}
		source = sources[0]
	default:
		source, err = sourceFromBytes(data)
		if err != nil {
			return fontRef{}, err
		}
	}
	ref := fontRef{ident: clean, source: source, meta: source.Metadata()}

	fontSourceCache.Lock()
	fontSourceCache.refs[clean] = cachedFontSource{ref: ref, modTime: info.ModTime(), size: info.Size()}
	fontSourceCache.Unlock()
	return ref, nil
}

func sourceFromBytes(data []byte) (*text.GoTextFaceSource, error) {
	return text.NewGoTextFaceSource(bytes.NewReader(data))
}

func isBuiltinFontName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "go mono", "gomono", "go-mono", "default", "monospace", "mono":
		return true
	}
	return false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func looksLikePath(value string) bool {
	if strings.ContainsAny(value, `/\`) {
		return true
	}
	switch strings.ToLower(filepath.Ext(value)) {
	case ".ttf", ".otf", ".ttc", ".otc":
		return true
	}
	return false
}

func themeColors(theme config.Theme, style terminal.Style) (color.RGBA, color.RGBA) {
	fg := resolveColor(style.FG, theme.Foreground, theme.Palette)
	bg := resolveColor(style.BG, theme.Background, theme.Palette)
	if style.Bold && style.FG.Mode == terminal.ColorIndexed && style.FG.Index < 8 {
		fg = theme.Palette[style.FG.Index+8]
	}
	if style.Reverse {
		fg, bg = bg, fg
	}
	if style.Faint {
		fg.R = uint8(int(fg.R) * 0xa0 / 0xff)
		fg.G = uint8(int(fg.G) * 0xa0 / 0xff)
		fg.B = uint8(int(fg.B) * 0xa0 / 0xff)
		fg.A = 0xa0
	}
	return fg, bg
}

func resolveColor(value terminal.Color, fallback color.RGBA, palette [16]color.RGBA) color.RGBA {
	switch value.Mode {
	case terminal.ColorIndexed:
		return indexedColor(palette, value.Index)
	case terminal.ColorRGB:
		return color.RGBA{R: value.R, G: value.G, B: value.B, A: 0xff}
	default:
		return fallback
	}
}

func indexedColor(palette [16]color.RGBA, index uint8) color.RGBA {
	if index < 16 {
		return palette[index]
	}
	if index < 232 {
		levels := [6]uint8{0, 95, 135, 175, 215, 255}
		n := int(index) - 16
		return color.RGBA{R: levels[n/36], G: levels[(n/6)%6], B: levels[n%6], A: 0xff}
	}
	gray := uint8(8 + (int(index)-232)*10)
	return color.RGBA{R: gray, G: gray, B: gray, A: 0xff}
}
