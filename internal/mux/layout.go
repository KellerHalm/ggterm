package mux

import "math"

// Direction selects how a pane tree node splits its available space.
type Direction uint8

const (
	// Vertical places the two children side by side (a vertical divider).
	Vertical Direction = iota
	// Horizontal stacks the two children (a horizontal divider).
	Horizontal
)

// MoveDirection is a focus navigation direction.
type MoveDirection uint8

const (
	MoveLeft MoveDirection = iota
	MoveRight
	MoveUp
	MoveDown
)

// Rect is a rectangle in terminal cells.
type Rect struct {
	X, Y, W, H int
}

// Node is a node in a binary pane layout tree. A leaf has Pane set; an
// internal node has First and Second children.
type Node struct {
	Pane          *Pane
	Dir           Direction
	Ratio         float64
	First, Second *Node
}

func leaf(p *Pane) *Node {
	return &Node{Pane: p}
}

// IsLeaf reports whether the node holds a pane.
func (n *Node) IsLeaf() bool {
	return n != nil && n.Pane != nil
}

func (n *Node) panes(out []*Pane) []*Pane {
	if n == nil {
		return out
	}
	if n.Pane != nil {
		return append(out, n.Pane)
	}
	out = n.First.panes(out)
	return n.Second.panes(out)
}

func (n *Node) find(target *Pane) *Node {
	if n == nil {
		return nil
	}
	if n.Pane == target {
		return n
	}
	if found := n.First.find(target); found != nil {
		return found
	}
	return n.Second.find(target)
}

// splitPane replaces the leaf holding target with a split node. The existing
// pane stays first; the new pane becomes second.
func splitPane(n *Node, target, added *Pane, dir Direction) bool {
	if n == nil {
		return false
	}
	if n.Pane != nil {
		if n.Pane != target {
			return false
		}
		n.Pane = nil
		n.Dir = dir
		n.Ratio = 0.5
		n.First = leaf(target)
		n.Second = leaf(added)
		return true
	}
	if splitPane(n.First, target, added, dir) {
		return true
	}
	return splitPane(n.Second, target, added, dir)
}

// removePane removes target from the tree and collapses nodes that are left
// with a single child. It returns the new root and whether anything was
// removed.
func removePane(n *Node, target *Pane) (*Node, bool) {
	if n == nil {
		return nil, false
	}
	if n.Pane != nil {
		if n.Pane == target {
			return nil, true
		}
		return n, false
	}

	if first, removed := removePane(n.First, target); removed {
		if first == nil {
			return n.Second, true
		}
		n.First = first
		return n, true
	}
	if second, removed := removePane(n.Second, target); removed {
		if second == nil {
			return n.First, true
		}
		n.Second = second
		return n, true
	}
	return n, false
}

func firstPane(n *Node) *Pane {
	if n == nil {
		return nil
	}
	if n.Pane != nil {
		return n.Pane
	}
	if p := firstPane(n.First); p != nil {
		return p
	}
	return firstPane(n.Second)
}

// PaneLayout describes where a pane is drawn. Rect is the outer rectangle and
// Content excludes the one-cell frame used for borders.
type PaneLayout struct {
	Pane    *Pane
	Rect    Rect
	Content Rect
	Active  bool
}

func (n *Node) layout(area Rect, active *Pane, out []PaneLayout) []PaneLayout {
	if n == nil || area.W <= 0 || area.H <= 0 {
		return out
	}
	if n.Pane != nil {
		content := area
		if area.W >= 3 && area.H >= 3 {
			content = Rect{X: area.X + 1, Y: area.Y + 1, W: area.W - 2, H: area.H - 2}
		}
		return append(out, PaneLayout{
			Pane:    n.Pane,
			Rect:    area,
			Content: content,
			Active:  n.Pane == active,
		})
	}

	ratio := n.Ratio
	if ratio <= 0 || ratio >= 1 {
		ratio = 0.5
	}
	if n.Dir == Vertical {
		if area.W < 2 {
			n.First.layout(area, active, out)
			return n.Second.layout(Rect{X: area.X, Y: area.Y, W: 0, H: area.H}, active, out)
		}
		firstW := int(math.Round(float64(area.W) * ratio))
		if firstW < 1 {
			firstW = 1
		}
		if firstW > area.W-1 {
			firstW = area.W - 1
		}
		out = n.First.layout(Rect{X: area.X, Y: area.Y, W: firstW, H: area.H}, active, out)
		return n.Second.layout(Rect{X: area.X + firstW, Y: area.Y, W: area.W - firstW, H: area.H}, active, out)
	}

	if area.H < 2 {
		out = n.First.layout(area, active, out)
		return n.Second.layout(Rect{X: area.X, Y: area.Y, W: area.W, H: 0}, active, out)
	}
	firstH := int(math.Round(float64(area.H) * ratio))
	if firstH < 1 {
		firstH = 1
	}
	if firstH > area.H-1 {
		firstH = area.H - 1
	}
	out = n.First.layout(Rect{X: area.X, Y: area.Y, W: area.W, H: firstH}, active, out)
	return n.Second.layout(Rect{X: area.X, Y: area.Y + firstH, W: area.W, H: area.H - firstH}, active, out)
}

func findDirectional(panes []PaneLayout, active *Pane, dir MoveDirection) *Pane {
	var current *PaneLayout
	for i := range panes {
		if panes[i].Pane == active {
			current = &panes[i]
			break
		}
	}
	if current == nil {
		return nil
	}

	var best *PaneLayout
	bestDistance := math.MaxInt
	bestOverlap := -1
	for i := range panes {
		candidate := &panes[i]
		if candidate.Pane == active {
			continue
		}
		distance, overlap := relation(current.Rect, candidate.Rect, dir)
		if distance < 0 {
			continue
		}
		if distance < bestDistance || (distance == bestDistance && overlap > bestOverlap) {
			best = candidate
			bestDistance = distance
			bestOverlap = overlap
		}
	}
	if best == nil {
		return nil
	}
	return best.Pane
}

// relation returns the distance between two rectangles in the given direction
// and how much they overlap on the perpendicular axis. A negative distance
// means the candidate is not in that direction.
func relation(from, to Rect, dir MoveDirection) (distance, overlap int) {
	switch dir {
	case MoveRight:
		if to.X < from.X+from.W {
			return -1, 0
		}
		overlap = overlapLen(from.Y, from.Y+from.H, to.Y, to.Y+to.H)
		if overlap <= 0 {
			return -1, 0
		}
		return to.X - (from.X + from.W), overlap
	case MoveLeft:
		if to.X+to.W > from.X {
			return -1, 0
		}
		overlap = overlapLen(from.Y, from.Y+from.H, to.Y, to.Y+to.H)
		if overlap <= 0 {
			return -1, 0
		}
		return from.X - (to.X + to.W), overlap
	case MoveDown:
		if to.Y < from.Y+from.H {
			return -1, 0
		}
		overlap = overlapLen(from.X, from.X+from.W, to.X, to.X+to.W)
		if overlap <= 0 {
			return -1, 0
		}
		return to.Y - (from.Y + from.H), overlap
	case MoveUp:
		if to.Y+to.H > from.Y {
			return -1, 0
		}
		overlap = overlapLen(from.X, from.X+from.W, to.X, to.X+to.W)
		if overlap <= 0 {
			return -1, 0
		}
		return from.Y - (to.Y + to.H), overlap
	}
	return -1, 0
}

func overlapLen(a1, a2, b1, b2 int) int {
	start := a1
	if b1 > start {
		start = b1
	}
	end := a2
	if b2 < end {
		end = b2
	}
	return end - start
}
