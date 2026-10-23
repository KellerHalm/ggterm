package terminal

import "github.com/mattn/go-runewidth"

type grid struct {
	cells [][]Cell
	rows  int
	cols  int
}

func newGrid(rows, cols int) *grid {
	cells := make([][]Cell, rows)
	for i := range cells {
		cells[i] = make([]Cell, cols)
		for j := range cells[i] {
			cells[i][j] = Cell{Rune: ' ', Width: 1}
		}
	}
	return &grid{cells: cells, rows: rows, cols: cols}
}

func (g *grid) clear() {
	for i := range g.cells {
		for j := range g.cells[i] {
			g.cells[i][j] = Cell{Rune: ' ', Width: 1}
		}
	}
}

func (g *grid) clearRegion(startRow, startCol, endRow, endCol int) {
	for r := startRow; r <= endRow && r < g.rows; r++ {
		colStart := 0
		colEnd := g.cols - 1
		if r == startRow {
			colStart = startCol
		}
		if r == endRow {
			colEnd = endCol
		}
		for c := colStart; c <= colEnd && c < g.cols; c++ {
			g.cells[r][c] = Cell{Rune: ' ', Width: 1}
		}
	}
}

func (g *grid) setCell(row, col int, r rune, style Style) {
	if row < 0 || row >= g.rows || col < 0 || col >= g.cols {
		return
	}
	width := runewidth.RuneWidth(r)
	if width == 0 {
		return
	}
	if width == 2 && col+1 >= g.cols {
		g.cells[row][col] = Cell{Rune: ' ', Width: 1, Style: style}
		return
	}
	if col > 0 && g.cells[row][col-1].Width == 2 {
		g.cells[row][col-1] = Cell{Rune: ' ', Width: 1}
	}
	if col+1 < g.cols && g.cells[row][col+1].Width == 0 {
		g.cells[row][col+1] = Cell{Rune: ' ', Width: 1}
	}
	g.cells[row][col] = Cell{Rune: r, Width: uint8(width), Style: style}
	if width == 2 && col+1 < g.cols {
		g.cells[row][col+1] = Cell{Rune: 0, Width: 0, Style: style}
	}
}

func (g *grid) scrollUp(top, bottom, n int) {
	if n <= 0 || top >= bottom {
		return
	}
	if n > bottom-top+1 {
		n = bottom - top + 1
	}
	for i := 0; i < n; i++ {
		for r := top; r < bottom; r++ {
			copy(g.cells[r], g.cells[r+1])
		}
		for c := range g.cells[bottom] {
			g.cells[bottom][c] = Cell{Rune: ' ', Width: 1}
		}
	}
}

func (g *grid) scrollDown(top, bottom, n int) {
	if n <= 0 || top >= bottom {
		return
	}
	if n > bottom-top+1 {
		n = bottom - top + 1
	}
	for i := 0; i < n; i++ {
		for r := bottom; r > top; r-- {
			copy(g.cells[r], g.cells[r-1])
		}
		for c := range g.cells[top] {
			g.cells[top][c] = Cell{Rune: ' ', Width: 1}
		}
	}
}

func (g *grid) insertLines(row, bottom, n int) {
	g.scrollDown(row, bottom, n)
}

func (g *grid) deleteLines(row, bottom, n int) {
	g.scrollUp(row, bottom, n)
}

func (g *grid) insertChars(row, col, n int) {
	if row < 0 || row >= g.rows || col >= g.cols {
		return
	}
	if n > g.cols-col {
		n = g.cols - col
	}
	for c := g.cols - 1; c >= col+n; c-- {
		g.cells[row][c] = g.cells[row][c-n]
	}
	for c := col; c < col+n && c < g.cols; c++ {
		g.cells[row][c] = Cell{Rune: ' ', Width: 1}
	}
}

func (g *grid) insertCells(row, col, n int) {
	if n <= 0 {
		return
	}
	if col > 0 && g.cells[row][col].Width == 0 {
		col--
		n++
	}
	g.insertChars(row, col, n)
}

func (g *grid) deleteChars(row, col, n int) {
	if row < 0 || row >= g.rows || col >= g.cols {
		return
	}
	if n > g.cols-col {
		n = g.cols - col
	}
	for c := col; c < g.cols-n; c++ {
		g.cells[row][c] = g.cells[row][c+n]
	}
	for c := g.cols - n; c < g.cols; c++ {
		g.cells[row][c] = Cell{Rune: ' ', Width: 1}
	}
}

func (g *grid) eraseChars(row, col, n int) {
	if row < 0 || row >= g.rows || col >= g.cols {
		return
	}
	end := col + n
	if end > g.cols {
		end = g.cols
	}
	for c := col; c < end; c++ {
		g.cells[row][c] = Cell{Rune: ' ', Width: 1}
	}
}

func (g *grid) clone() [][]Cell {
	cloned := make([][]Cell, g.rows)
	for i := range g.cells {
		cloned[i] = make([]Cell, g.cols)
		copy(cloned[i], g.cells[i])
	}
	return cloned
}

func (g *grid) resize(newRows, newCols int) {
	if newRows == g.rows && newCols == g.cols {
		return
	}
	newCells := make([][]Cell, newRows)
	for i := 0; i < newRows; i++ {
		newCells[i] = make([]Cell, newCols)
		for j := 0; j < newCols; j++ {
			if i < g.rows && j < g.cols {
				newCells[i][j] = g.cells[i][j]
			} else {
				newCells[i][j] = Cell{Rune: ' ', Width: 1}
			}
		}
	}
	g.cells = newCells
	g.rows = newRows
	g.cols = newCols
}
