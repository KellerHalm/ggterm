package terminal

func (t *Terminal) setSGR(params []int) {
	if len(params) == 0 {
		params = []int{0}
	}
	for i := 0; i < len(params); i++ {
		switch p := params[i]; p {
		case 0:
			t.style = Style{}
		case 1:
			t.style.Bold = true
		case 2:
			t.style.Faint = true
		case 3:
			t.style.Italic = true
		case 4:
			t.style.Underline = true
			t.style.DoubleUnderline = false
		case 21:
			t.style.Underline = true
			t.style.DoubleUnderline = true
		case 5:
			t.style.Blink = true
		case 7:
			t.style.Reverse = true
		case 9:
			t.style.Strikethrough = true
		case 22:
			t.style.Bold = false
			t.style.Faint = false
		case 23:
			t.style.Italic = false
		case 24:
			t.style.Underline = false
			t.style.DoubleUnderline = false
		case 25:
			t.style.Blink = false
		case 27:
			t.style.Reverse = false
		case 29:
			t.style.Strikethrough = false
		case 30, 31, 32, 33, 34, 35, 36, 37:
			t.style.FG = IndexedColor(uint8(p - 30))
		case 39:
			t.style.FG = Color{}
		case 40, 41, 42, 43, 44, 45, 46, 47:
			t.style.BG = IndexedColor(uint8(p - 40))
		case 49:
			t.style.BG = Color{}
		case 90, 91, 92, 93, 94, 95, 96, 97:
			t.style.FG = IndexedColor(uint8(p - 90 + 8))
		case 100, 101, 102, 103, 104, 105, 106, 107:
			t.style.BG = IndexedColor(uint8(p - 100 + 8))
		case 38:
			i = applyExtendedColor(params, i, &t.style.FG)
		case 48:
			i = applyExtendedColor(params, i, &t.style.BG)
		case 58:
			i = applyExtendedColor(params, i, &t.style.UnderlineColor)
		case 59:
			t.style.UnderlineColor = Color{}
		}
	}
}

func applyExtendedColor(params []int, index int, target *Color) int {
	if index+1 >= len(params) {
		return len(params)
	}
	switch params[index+1] {
	case 5:
		if index+2 >= len(params) {
			return len(params)
		}
		if target != nil {
			*target = IndexedColor(uint8(params[index+2]))
		}
		return index + 2
	case 2:
		if index+4 >= len(params) {
			return len(params)
		}
		if target != nil {
			*target = RGBColor(
				uint8(params[index+2]),
				uint8(params[index+3]),
				uint8(params[index+4]),
			)
		}
		return index + 4
	default:
		return index + 1
	}
}
