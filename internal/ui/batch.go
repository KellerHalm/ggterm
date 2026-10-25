package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// rectBatch accumulates solid rectangles and submits them as one DrawTriangles
// call. Terminal backgrounds are the most numerous draw operation, so batching
// them removes thousands of GPU draw calls per frame.
type rectBatch struct {
	vertices []ebiten.Vertex
	indices  []uint32
	white    *ebiten.Image
}

func (b *rectBatch) reset() {
	b.vertices = b.vertices[:0]
	b.indices = b.indices[:0]
}

func (b *rectBatch) add(x, y, width, height float64, clr color.RGBA) {
	if width <= 0 || height <= 0 || clr.A == 0 {
		return
	}
	r := float32(clr.R) / 255
	g := float32(clr.G) / 255
	bl := float32(clr.B) / 255
	a := float32(clr.A) / 255

	base := uint32(len(b.vertices))
	b.vertices = append(b.vertices,
		ebiten.Vertex{DstX: float32(x), DstY: float32(y), ColorR: r, ColorG: g, ColorB: bl, ColorA: a},
		ebiten.Vertex{DstX: float32(x + width), DstY: float32(y), ColorR: r, ColorG: g, ColorB: bl, ColorA: a},
		ebiten.Vertex{DstX: float32(x), DstY: float32(y + height), ColorR: r, ColorG: g, ColorB: bl, ColorA: a},
		ebiten.Vertex{DstX: float32(x + width), DstY: float32(y + height), ColorR: r, ColorG: g, ColorB: bl, ColorA: a},
	)
	b.indices = append(b.indices, base, base+1, base+2, base+1, base+3, base+2)
}

func (b *rectBatch) flush(dst *ebiten.Image) {
	if len(b.indices) == 0 {
		return
	}
	if b.white == nil {
		b.white = ebiten.NewImage(1, 1)
		b.white.Fill(color.White)
	}
	dst.DrawTriangles32(b.vertices, b.indices, b.white, &ebiten.DrawTrianglesOptions{
		Filter: ebiten.FilterNearest,
	})
	b.reset()
}
