// Package cellbuf — сеточный буфер ячеек с undo-историей.
// Ширина глифов — только go-runewidth, никогда libc wcwidth.
package cellbuf

import (
	"os"
	"strings"
)

// Cell — одна ячейка сетки. Fg — 256-color override, -1 = базовый цвет слоя.
type Cell struct {
	G  rune
	Fg int
}

var Blank = Cell{G: ' ', Fg: -1}

// Change — атомарное изменение одной ячейки. Before заполняется в Apply.
type Change struct {
	Row, Col      int
	Before, After Cell
}

const maxHistory = 1000

type Buffer struct {
	W, H     int
	cells    [][]Cell
	hist     [][]Change
	histIdx  int
	savedIdx int
}

func New(w, h int) *Buffer {
	b := &Buffer{W: w, H: h, cells: make([][]Cell, h)}
	for r := range b.cells {
		row := make([]Cell, w)
		for c := range row {
			row[c] = Blank
		}
		b.cells[r] = row
	}
	return b
}

// Load читает plain txt (UTF-8, LF; терпит CRLF). Ширина = самая длинная
// строка, короткие добиваются прозрачными пробелами.
func Load(path string) (*Buffer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")

	rows := make([][]rune, len(lines))
	w := 1
	for i, l := range lines {
		rows[i] = []rune(l)
		if len(rows[i]) > w {
			w = len(rows[i])
		}
	}
	b := New(w, len(rows))
	for r, runes := range rows {
		for c, g := range runes {
			b.cells[r][c] = Cell{G: g, Fg: -1}
		}
	}
	return b, nil
}

func (b *Buffer) Get(r, c int) Cell {
	if r < 0 || r >= b.H || c < 0 || c >= b.W {
		return Blank
	}
	return b.cells[r][c]
}

// Apply применяет группу изменений как один шаг undo.
// Before в каждом Change заполняется текущим состоянием.
func (b *Buffer) Apply(group []Change) {
	if len(group) == 0 {
		return
	}
	for i := range group {
		ch := &group[i]
		ch.Before = b.Get(ch.Row, ch.Col)
		b.set(ch.Row, ch.Col, ch.After)
	}
	b.hist = b.hist[:b.histIdx]
	if b.savedIdx > b.histIdx {
		b.savedIdx = -1
	}
	b.hist = append(b.hist, group)
	b.histIdx++
	if len(b.hist) > maxHistory {
		drop := len(b.hist) - maxHistory
		b.hist = b.hist[drop:]
		b.histIdx -= drop
		if b.savedIdx >= 0 {
			b.savedIdx -= drop
		}
	}
}

func (b *Buffer) set(r, c int, cell Cell) {
	if r < 0 || r >= b.H || c < 0 || c >= b.W {
		return
	}
	b.cells[r][c] = cell
}

func (b *Buffer) Undo() bool {
	if b.histIdx == 0 {
		return false
	}
	group := b.hist[b.histIdx-1]
	for i := len(group) - 1; i >= 0; i-- {
		b.set(group[i].Row, group[i].Col, group[i].Before)
	}
	b.histIdx--
	return true
}

func (b *Buffer) Redo() bool {
	if b.histIdx == len(b.hist) {
		return false
	}
	group := b.hist[b.histIdx]
	for i := range group {
		b.set(group[i].Row, group[i].Col, group[i].After)
	}
	b.histIdx++
	return true
}

func (b *Buffer) Dirty() bool { return b.histIdx != b.savedIdx }

// SaveTxt пишет буфер: trailing-пробелы срезаны, LF, завершающий перевод строки.
func (b *Buffer) SaveTxt(path string) error {
	var sb strings.Builder
	for r := 0; r < b.H; r++ {
		line := make([]rune, b.W)
		for c := 0; c < b.W; c++ {
			line[c] = b.cells[r][c].G
		}
		sb.WriteString(strings.TrimRight(string(line), " "))
		sb.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return err
	}
	b.savedIdx = b.histIdx
	return nil
}

// GrowTo наращивает буфер минимум до w×h пустыми ячейками (часть загрузки:
// txt хранится без хвостовых пробелов и может быть уже/ниже канвы).
// Историю и dirty-состояние не трогает.
func (b *Buffer) GrowTo(w, h int) {
	if w <= b.W && h <= b.H {
		return
	}
	w, h = max(w, b.W), max(h, b.H)
	cells := make([][]Cell, h)
	for r := range cells {
		row := make([]Cell, w)
		for c := range row {
			if r < b.H && c < b.W {
				row[c] = b.cells[r][c]
			} else {
				row[c] = Blank
			}
		}
		cells[r] = row
	}
	b.cells, b.W, b.H = cells, w, h
}

// Resize обрезает или наращивает буфер до w×h (новые ячейки — Blank).
// Структурная операция: история undo сбрасывается, буфер становится dirty.
func (b *Buffer) Resize(w, h int) {
	cells := make([][]Cell, h)
	for r := range cells {
		row := make([]Cell, w)
		for c := range row {
			if r < b.H && c < b.W {
				row[c] = b.cells[r][c]
			} else {
				row[c] = Blank
			}
		}
		cells[r] = row
	}
	b.cells, b.W, b.H = cells, w, h
	b.hist, b.histIdx, b.savedIdx = nil, 0, -1
}

// LastContentCol — последняя непробельная колонка строки (0-based), -1 если пусто.
func (b *Buffer) LastContentCol(r int) int {
	if r < 0 || r >= b.H {
		return -1
	}
	for c := b.W - 1; c >= 0; c-- {
		if b.cells[r][c].G != ' ' {
			return c
		}
	}
	return -1
}
