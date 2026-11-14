package cellbuf

// octExtra — 26 паттернов 2×4, закодированных ВНЕ U+1CD00–1CDE5: пустой,
// полный, квадранты, половины и четверти из Block Elements и Legacy
// Computing. Остальные 230 лежат в U+1CD00–1CDE5 подряд по возрастанию
// маски — из этого и собирается таблица.
var octExtra = map[int]rune{
	0x00: ' ', 0x01: '𜺨', 0x02: '𜺫', 0x03: '🮂', 0x05: '▘', 0x0A: '▝',
	0x0F: '▀', 0x14: '🯦', 0x28: '🯧', 0x3F: '🮅', 0x40: '𜺣', 0x50: '▖',
	0x55: '▌', 0x5A: '▞', 0x5F: '▛', 0x80: '𜺠', 0xA0: '▗', 0xA5: '▚',
	0xAA: '▐', 0xAF: '▜', 0xC0: '▂', 0xF0: '▄', 0xF5: '▙', 0xFA: '▟',
	0xFC: '▆', 0xFF: '█',
}

// OctGlyph — глиф по маске 2×4 (бит N = 1<<(N-1), нумерация построчно);
// OctMasks — обратная таблица.
var OctGlyph, OctMasks = buildOctants()

func buildOctants() ([256]rune, map[rune]int) {
	var glyphs [256]rune
	masks := make(map[rune]int, 256)
	next := rune(0x1CD00)
	for mask := 0; mask < 256; mask++ {
		g, ok := octExtra[mask]
		if !ok {
			g, next = next, next+1
		}
		glyphs[mask] = g
		masks[g] = mask
	}
	return glyphs, masks
}

// BrailleMask — маска точек глифа; не-брайль (включая пробел) = 0.
func BrailleMask(g rune) int {
	if g >= 0x2800 && g <= 0x28FF {
		return int(g - 0x2800)
	}
	return 0
}

func isBraille(g rune) bool { return g >= 0x2800 && g <= 0x28FF }

// Overstrike — печать over поверх under в одной ячейке. Глифы одного
// семейства (брайль+брайль, октант+октант) сливаются в глиф с объединённой
// маской; иначе общего глифа нет, остаётся over и возвращается false.
func Overstrike(under, over rune) (rune, bool) {
	if isBraille(under) && isBraille(over) {
		return under | over, true
	}
	um, uok := OctMasks[under]
	om, ook := OctMasks[over]
	if uok && ook && under != ' ' && over != ' ' {
		return OctGlyph[um|om], true
	}
	return over, false
}
