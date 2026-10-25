package terminal

type ColorMode uint8

const (
	ColorDefault ColorMode = iota
	ColorIndexed
	ColorRGB
)

type Color struct {
	Mode    ColorMode
	Index   uint8
	R, G, B uint8
}

func IndexedColor(index uint8) Color {
	return Color{Mode: ColorIndexed, Index: index}
}

func RGBColor(r, g, b uint8) Color {
	return Color{Mode: ColorRGB, R: r, G: g, B: b}
}

type Style struct {
	FG, BG          Color
	UnderlineColor  Color
	Bold, Faint     bool
	Italic          bool
	Underline       bool
	DoubleUnderline bool
	Blink, Reverse  bool
	Strikethrough   bool
}

type Cell struct {
	Rune  rune
	Width uint8
	Style Style
}

type Cursor struct {
	Row, Col int
	Visible  bool
}

type CursorStyle uint8

const (
	CursorBlock CursorStyle = iota
	CursorUnderline
	CursorBar
)

type Config struct {
	Cols, Rows    int
	MaxScrollback int
	TabWidth      int
}

type Snapshot struct {
	Cols, Rows    int
	Lines         [][]Cell
	Cursor        Cursor
	Title         string
	ScrollbackLen int
	ScrollOffset  int
	AltScreen     bool
}
