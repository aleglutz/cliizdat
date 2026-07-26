package project

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/aleglutz/cliizdat/cellbuf"
)

// cp437High — верхняя половина CP437 (0x80–0xFF): старые .ans и `tdfiglet -e a`
// пишут байтами, а не UTF-8. 0xFF (NBSP) сведён к обычному пробелу.
const cp437High = "ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒáíóúñÑªº¿⌐¬½¼¡«»" +
	"░▒▓│┤╡╢╖╕╣║╗╝╜╛┐└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀" +
	"αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ "

var cp437 = []rune(cp437High)

// ParseANSI разбирает ANSI-поток (вывод tdfiglet/figlet, наш собственный
// экспорт, .ans из архивов) в сетку ячеек. Понимает SGR, LF/CR и CUF (ESC[nC);
// прочие CSI игнорирует, SAUCE-запись срезает, не-UTF-8 читает как CP437.
//
// У cellbuf.Cell нет фона, поэтому тройка (глиф, fg, bg) сводится к одному
// цвету — см. reduceCell. Хвостовые пустые строки отбрасываются, строки
// добиваются до общей ширины прозрачными пробелами.
func ParseANSI(raw []byte) [][]cellbuf.Cell {
	runes := []rune(decodeText(stripSAUCE(raw)))
	var grid [][]cellbuf.Cell
	fg, bg, bold := -1, 0, false
	row, col := 0, 0

	ensure := func(r, c int) {
		for len(grid) <= r {
			grid = append(grid, nil)
		}
		for len(grid[r]) <= c {
			grid[r] = append(grid[r], cellbuf.Blank)
		}
	}

	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch == 0x1b {
			if i+1 < len(runes) && runes[i+1] == '[' {
				j := i + 2
				for j < len(runes) && (runes[j] == ';' || runes[j] == '?' || runes[j] >= '0' && runes[j] <= '9') {
					j++
				}
				if j < len(runes) {
					params := string(runes[i+2 : j])
					switch runes[j] {
					case 'm':
						fg, bg, bold = applySGR(params, fg, bg, bold)
					case 'C':
						col += csiArg(params, 1)
					}
					i = j
					continue
				}
			}
			continue // одиночный ESC или обрубленная последовательность
		}
		switch {
		case ch == '\n':
			row++
			col = 0
		case ch == '\r':
			col = 0
		case ch < 0x20:
			// control-глифы CP437 (☺☻♥…) в арте бывают, но отличить их от
			// управляющих байтов нельзя — пропускаем
		default:
			cell := reduceCell(ch, fg, bold, bg)
			if cell.G != ' ' {
				ensure(row, col)
				grid[row][col] = cell
			}
			col++
		}
	}

	for len(grid) > 0 && len(grid[len(grid)-1]) == 0 {
		grid = grid[:len(grid)-1]
	}
	w := 0
	for _, r := range grid {
		if len(r) > w {
			w = len(r)
		}
	}
	for r := range grid {
		for len(grid[r]) < w {
			grid[r] = append(grid[r], cellbuf.Blank)
		}
	}
	return grid
}

// reduceCell сводит (глиф, fg, bg) к одной ячейке — фона у Cell нет:
//
//	bg = 0     фон и есть бумага, выбрасываем;
//	пробел     фон и есть картинка: полный блок цветом фона (точно);
//	'█'        фон под ним не виден (точно);
//	остальное  оставляем fg, фон теряется — полублоки и тени (приблизительно).
func reduceCell(g rune, fg int, bold bool, bg int) cellbuf.Cell {
	if bold && fg >= 0 && fg <= 7 {
		fg += 8 // ESC[1;3Xm — «яркий» набор старых .ans
	}
	if g == ' ' {
		if bg != 0 {
			return cellbuf.Cell{G: '█', Fg: bg}
		}
		return cellbuf.Blank
	}
	return cellbuf.Cell{G: g, Fg: fg}
}

func applySGR(params string, fg, bg int, bold bool) (int, int, bool) {
	if params == "" {
		return -1, 0, false
	}
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		n, err := strconv.Atoi(fields[i])
		if err != nil {
			n = 0 // пустой параметр — как 0 (ESC[;31m)
		}
		switch {
		case n == 0:
			fg, bg, bold = -1, 0, false
		case n == 1:
			bold = true
		case n == 21 || n == 22:
			bold = false
		case n >= 30 && n <= 37:
			fg = n - 30
		case n == 38:
			if v, skip, ok := extended(fields[i+1:]); ok {
				fg = v
				i += skip
			}
		case n == 39:
			fg = -1
		case n >= 40 && n <= 47:
			bg = n - 40
		case n == 48:
			if v, skip, ok := extended(fields[i+1:]); ok {
				bg = v
				i += skip
			}
		case n == 49:
			bg = 0
		case n >= 90 && n <= 97:
			fg = n - 90 + 8
		case n >= 100 && n <= 107:
			bg = n - 100 + 8
		}
	}
	return fg, bg, bold
}

// extended разбирает хвост 38/48: `5;N` — индекс палитры, `2;r;g;b` —
// truecolor (не поддерживаем, только проглатываем параметры).
func extended(rest []string) (val, skip int, ok bool) {
	if len(rest) == 0 {
		return 0, 0, false
	}
	switch rest[0] {
	case "5":
		if len(rest) >= 2 {
			if v, err := strconv.Atoi(rest[1]); err == nil && v >= 0 && v <= 255 {
				return v, 2, true
			}
		}
		return 0, 1, false
	case "2":
		return 0, min(4, len(rest)), false
	}
	return 0, 0, false
}

func csiArg(params string, def int) int {
	if params == "" {
		return def
	}
	n, err := strconv.Atoi(strings.Split(params, ";")[0])
	if err != nil || n < 1 {
		return def
	}
	return n
}

// stripSAUCE срезает 128-байтную запись SAUCE и блоки комментариев за
// DOS-EOF, если запись есть; без неё поток не трогается.
func stripSAUCE(raw []byte) []byte {
	if len(raw) < 128 || !bytes.HasPrefix(raw[len(raw)-128:], []byte("SAUCE00")) {
		return raw
	}
	raw = raw[:len(raw)-128]
	if i := bytes.LastIndexByte(raw, 0x1a); i >= 0 {
		raw = raw[:i]
	}
	return raw
}

func decodeText(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	var sb strings.Builder
	sb.Grow(len(raw))
	for _, b := range raw {
		if b < 0x80 {
			sb.WriteByte(b)
		} else {
			sb.WriteRune(cp437[b-0x80])
		}
	}
	return sb.String()
}

// CropInk обрезает сетку до bounding box непробельных ячеек: tdfiglet отдаёт
// пустую первую строку и добивает вывод до ширины экрана, а вставка должна
// лечь ровно под курсор.
func CropInk(grid [][]cellbuf.Cell) [][]cellbuf.Cell {
	r1, r2, c1, c2 := -1, -1, -1, -1
	for r, row := range grid {
		for c, cell := range row {
			if cell.G == ' ' {
				continue
			}
			if r1 < 0 {
				r1, c1, c2 = r, c, c
			}
			r2 = r
			if c < c1 {
				c1 = c
			}
			if c > c2 {
				c2 = c
			}
		}
	}
	if r1 < 0 {
		return nil
	}
	out := make([][]cellbuf.Cell, 0, r2-r1+1)
	for _, row := range grid[r1 : r2+1] {
		cut := make([]cellbuf.Cell, c2-c1+1)
		for i := range cut {
			if c := c1 + i; c < len(row) {
				cut[i] = row[c]
			} else {
				cut[i] = cellbuf.Blank
			}
		}
		out = append(out, cut)
	}
	return out
}

// ImportANS читает .ans (или любой ANSI-поток) и строит implicit-проект —
// как ImportDur: базовый fg = самый частый цвет, отличия как overrides.
// Ничего не пишет на диск до SaveAll.
func ImportANS(path string) (*Project, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	grid := ParseANSI(raw)
	if len(grid) == 0 {
		return nil, fmt.Errorf("%s: пусто", path)
	}
	return implicitFromCells(path, grid)
}

// implicitFromCells — общий хвост импортёров: сетка ячеек → implicit-проект
// <имя>.{json,txt,color}. Базовый fg слоя = самый частый цвет краски,
// остальные остаются per-cell overrides.
func implicitFromCells(path string, grid [][]cellbuf.Cell) (*Project, error) {
	h := len(grid)
	w := 1
	for _, row := range grid {
		if len(row) > w {
			w = len(row)
		}
	}
	freq := map[int]int{}
	var group []cellbuf.Change
	for r, row := range grid {
		for c, cell := range row {
			if cell.G == ' ' {
				continue
			}
			freq[cell.Fg]++
			group = append(group, cellbuf.Change{Row: r, Col: c, After: cell})
		}
	}
	if len(group) == 0 {
		return nil, fmt.Errorf("%s: краски нет", path)
	}
	base, best := -1, 0
	for fg, n := range freq {
		if n > best || (n == best && fg < base) {
			base, best = fg, n
		}
	}
	for i := range group {
		if group[i].After.Fg == base {
			group[i].After.Fg = -1
		}
	}
	buf := cellbuf.New(w, h)
	buf.Apply(group)

	abs, _ := filepath.Abs(path)
	stem := strings.TrimSuffix(abs, filepath.Ext(abs))
	if _, err := os.Stat(stem + ".txt"); err == nil {
		return nil, fmt.Errorf("%s.txt уже существует — импорт перезаписал бы его", stem)
	}
	return &Project{
		Path:          stem + ".json",
		Dir:           filepath.Dir(abs),
		CanvasW:       w,
		CanvasH:       h,
		Layers:        []*Layer{{File: filepath.Base(stem) + ".txt", AbsPath: stem + ".txt", Fg: base, Buf: buf, NewFile: true}},
		Slots:         DefaultSlots,
		ManifestDirty: true,
	}, nil
}
