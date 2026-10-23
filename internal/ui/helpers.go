package ui

func windowTitle(title string) string {
	if title == "" {
		return "ggterm"
	}
	return "ggterm — " + title
}

func pageStep(rows int) int {
	if rows <= 2 {
		return 1
	}
	return rows - 1
}

func clampOffset(offset, limit int) int {
	if offset < 0 {
		return 0
	}
	if offset > limit {
		return limit
	}
	return offset
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
