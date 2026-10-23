package ui

import (
	"fmt"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
)

type modifiers struct {
	ctrl  bool
	alt   bool
	shift bool
}

func currentModifiers() modifiers {
	return modifiers{
		ctrl:  ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight),
		alt:   ebiten.IsKeyPressed(ebiten.KeyAlt) || ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight),
		shift: ebiten.IsKeyPressed(ebiten.KeyShift) || ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight),
	}
}

func keySequence(key ebiten.Key, mods modifiers) ([]byte, bool) {
	switch key {
	case ebiten.KeyEnter:
		return withAlt([]byte{'\r'}, mods.alt), true
	case ebiten.KeyTab:
		if mods.shift {
			return []byte("\x1b[Z"), true
		}
		return []byte{'\t'}, true
	case ebiten.KeyBackspace:
		return withAlt([]byte{0x7f}, mods.alt), true
	case ebiten.KeyEscape:
		return []byte{0x1b}, true
	case ebiten.KeySpace:
		if mods.ctrl {
			return []byte{0}, true
		}
		if mods.alt {
			return []byte{0x1b, ' '}, true
		}
		return nil, false
	case ebiten.KeyArrowUp:
		return cursorSequence('A', mods), true
	case ebiten.KeyArrowDown:
		return cursorSequence('B', mods), true
	case ebiten.KeyArrowRight:
		return cursorSequence('C', mods), true
	case ebiten.KeyArrowLeft:
		return cursorSequence('D', mods), true
	case ebiten.KeyHome:
		return cursorSequence('H', mods), true
	case ebiten.KeyEnd:
		return cursorSequence('F', mods), true
	case ebiten.KeyInsert:
		return []byte("\x1b[2~"), true
	case ebiten.KeyDelete:
		return []byte("\x1b[3~"), true
	case ebiten.KeyF1:
		return []byte("\x1b[11~"), true
	case ebiten.KeyF2:
		return []byte("\x1b[12~"), true
	case ebiten.KeyF3:
		return []byte("\x1b[13~"), true
	case ebiten.KeyF4:
		return []byte("\x1b[14~"), true
	case ebiten.KeyF5:
		return []byte("\x1b[15~"), true
	case ebiten.KeyF6:
		return []byte("\x1b[17~"), true
	case ebiten.KeyF7:
		return []byte("\x1b[18~"), true
	case ebiten.KeyF8:
		return []byte("\x1b[19~"), true
	case ebiten.KeyF9:
		return []byte("\x1b[20~"), true
	case ebiten.KeyF10:
		return []byte("\x1b[21~"), true
	case ebiten.KeyF11:
		return []byte("\x1b[23~"), true
	case ebiten.KeyF12:
		return []byte("\x1b[24~"), true
	}

	if mods.ctrl {
		if value, ok := controlByte(key, mods.shift); ok {
			return withAlt([]byte{value}, mods.alt), true
		}
	}
	if mods.alt {
		if r, ok := altRune(key); ok {
			if mods.shift {
				r = unicode.ToUpper(r)
			}
			return []byte{0x1b, byte(r)}, true
		}
	}
	return nil, false
}

func cursorSequence(final byte, mods modifiers) []byte {
	if !mods.ctrl && !mods.alt && !mods.shift {
		return []byte{0x1b, '[', final}
	}
	param := 1
	if mods.shift {
		param++
	}
	if mods.alt {
		param += 2
	}
	if mods.ctrl {
		param += 4
	}
	return []byte(fmt.Sprintf("\x1b[1;%d%c", param, final))
}

func controlByte(key ebiten.Key, shift bool) (byte, bool) {
	if key >= ebiten.KeyA && key <= ebiten.KeyZ {
		return byte(int(key-ebiten.KeyA) + 1), true
	}
	switch key {
	case ebiten.KeySpace:
		return 0, true
	case ebiten.KeyLeftBracket:
		return 27, true
	case ebiten.KeyBackslash:
		return 28, true
	case ebiten.KeyRightBracket:
		return 29, true
	case ebiten.KeyDigit6:
		if shift {
			return 30, true
		}
	case ebiten.KeyMinus:
		if shift {
			return 31, true
		}
	case ebiten.KeyDigit2:
		if shift {
			return 0, true
		}
	}
	return 0, false
}

func altRune(key ebiten.Key) (rune, bool) {
	if key >= ebiten.KeyA && key <= ebiten.KeyZ {
		return rune('a' + int(key-ebiten.KeyA)), true
	}
	if key >= ebiten.KeyDigit0 && key <= ebiten.KeyDigit9 {
		return rune('0' + int(key-ebiten.KeyDigit0)), true
	}
	switch key {
	case ebiten.KeyMinus:
		return '-', true
	case ebiten.KeyEqual:
		return '=', true
	case ebiten.KeyComma:
		return ',', true
	case ebiten.KeyPeriod:
		return '.', true
	case ebiten.KeySlash:
		return '/', true
	case ebiten.KeySemicolon:
		return ';', true
	case ebiten.KeyQuote:
		return '\'', true
	case ebiten.KeyLeftBracket:
		return '[', true
	case ebiten.KeyRightBracket:
		return ']', true
	case ebiten.KeyBackslash:
		return '\\', true
	case ebiten.KeyBackquote:
		return '`', true
	}
	return 0, false
}

func withAlt(sequence []byte, alt bool) []byte {
	if !alt {
		return sequence
	}
	return append([]byte{0x1b}, sequence...)
}
