package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/aleglutz/cliizdat/cellbuf"
	"github.com/aleglutz/cliizdat/palette"
	"github.com/aleglutz/cliizdat/project"
)

type uiMode int

const (
	modeSplash uiMode = iota
	modeEdit
	modeGoto
	modeQuit
	modePalette
	modeColor
	modeResize
	modeHelp
)

var (
	// euromancer blue ribbon #2f6f9c на светлом фоне, зелёный 46 на тёмном
	accent      = lipgloss.AdaptiveColor{Dark: "46", Light: "#2f6f9c"}
	cursorStyle = lipgloss.NewStyle().Background(accent).
			Foreground(lipgloss.AdaptiveColor{Dark: "16", Light: "#f5f0e6"})
	frameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	markStyle   = lipgloss.NewStyle().Foreground(accent)
	statusStyle = lipgloss.NewStyle().Reverse(true)
	selStyle    = lipgloss.NewStyle().Background(lipgloss.Color("238"))
	faintStyle  = lipgloss.NewStyle().Faint(true)
)

type model struct {
	proj   *project.Project
	pal    []palette.Page
	active int
	solo   bool

	curFg    int // текущий цвет кисти; -1 = default
	lastSlot int

	curR, curC   int // 0-based, координаты канвы
	offR, offC   int
	termW, termH int

	dotOn      bool
	dotR, dotC int // суб-позиция точки в ячейке: ряд 0..3, столбец 0..1

	selOn      bool
	selSticky  bool // выделение от `s`: стрелки растят, не сбрасывают
	selR, selC int  // якорь выделения

	clip [][]cellbuf.Cell // copy/paste; Fg — эффективный цвет источника

	mode   uiMode
	input  string
	status string

	ppPage, ppRow, ppCol, ppSlot int // palette picker
	cp                           int // color picker 0..255

	deco [2][]rune // случайные октанты сплэша, слева и справа
}

const appVersion = "0.0.1"

const splashDesc = "cli-native ANSI and ASCII text editor inspired by ACiDDraw, " +
	"its Unix successor DurDraw, and the rich tradition of concrete poetry " +
	"in the Eastern Bloc samizdat"

func newModel(p *project.Project, pal []palette.Page) model {
	var deco [2][]rune
	for i := range deco {
		r := make([]rune, 128)
		for j := range r {
			r[j] = rune(0x1CD00 + rand.IntN(0xE6))
		}
		deco[i] = r
	}
	return model{
		proj: p, pal: pal,
		mode:     modeSplash,
		deco:     deco,
		curFg:    p.Layers[0].Fg,
		lastSlot: 1,
		termW:    80, termH: 24,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m *model) layer() *project.Layer { return m.proj.Layers[m.active] }

func (m model) viewH() int { return max(1, m.termH-3) } // рамка (2) + статус-бар
func (m model) viewW() int { return max(1, m.termW-2) } // рамка по бокам

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.termW = msg.Width
		}
		if msg.Height > 0 {
			m.termH = msg.Height
		}
		m.clampView()
		return m, nil
	case tea.KeyMsg:
		// быстрый ввод может склеить руны в один KeyMsg — разбираем по одной
		if msg.Type == tea.KeyRunes && !msg.Paste && len(msg.Runes) > 1 {
			var mm tea.Model = m
			var cmd tea.Cmd
			for _, r := range msg.Runes {
				mm, cmd = mm.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				if cmd != nil {
					return mm, cmd
				}
			}
			return mm, nil
		}
		switch m.mode {
		case modeSplash:
			m.mode = modeEdit
			return m, nil
		case modeResize:
			return m.updateResize(msg)
		case modeHelp:
			m.mode = modeEdit
			return m, nil
		case modeGoto:
			return m.updateGoto(msg)
		case modeQuit:
			return m.updateQuit(msg)
		case modePalette:
			return m.updatePalette(msg)
		case modeColor:
			return m.updateColor(msg)
		default:
			return m.updateEdit(msg)
		}
	}
	return m, nil
}

// ── edit mode ──────────────────────────────────────────────────────────

func (m model) updateEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	key := msg.String()
	if m.dotOn {
		switch key {
		case "up", "down", "left", "right":
			m.moveDot(key)
			m.clampView()
			return m, nil
		case ".":
			m.toggleDot()
			return m, nil
		case "b", "esc":
			m.dotOn = false
			m.status = "dot-mode off"
			return m, nil
		}
	}
	switch key {
	case "b":
		m.dotOn = true
		m.status = "dot-mode: стрелки — точка, `.` — тоггл, b/Esc — выход"
	case "up", "down", "left", "right":
		if !m.selSticky {
			m.selOn = false
		}
		m.moveCursor(key)
	case "shift+up", "shift+down", "shift+left", "shift+right":
		if !m.selOn {
			m.selOn = true
			m.selR, m.selC = m.curR, m.curC
		}
		m.moveCursor(strings.TrimPrefix(key, "shift+"))
	case "home":
		m.curC = 0
	case "end":
		L := m.layer()
		if c := L.Buf.LastContentCol(m.curR - L.AtR); c >= 0 {
			m.curC = L.AtC + c
		} else {
			m.curC = 0
		}
	case "pgup":
		m.curR -= m.viewH()
	case "pgdown":
		m.curR += m.viewH()
	case "g":
		m.mode = modeGoto
		m.input = ""
	case "R":
		m.mode = modeResize
		m.input = ""
	case "h", "?":
		m.mode = modeHelp
	case "esc":
		m.selOn = false
		m.selSticky = false
	case "s":
		if m.selSticky {
			m.selOn, m.selSticky = false, false
			m.status = "выделение снято"
		} else {
			m.selOn, m.selSticky = true, true
			m.selR, m.selC = m.curR, m.curC
			m.status = "выделение: стрелки растят, Esc — снять"
		}
	case "x":
		m.fillOrStamp(0, false)
	case "y":
		m.yank(false)
	case "d":
		m.yank(true)
	case "p":
		m.paste()
	case "r":
		m.repaint()
	case "X":
		m.flip(true)
	case "Y":
		m.flip(false)
	case "c":
		m.mode = modeColor
		if m.curFg >= 0 {
			m.cp = m.curFg
		} else {
			m.cp = 0
		}
	case "C":
		vc := m.compositeCell(m.curR, m.curC)
		m.curFg = vc.fg
		m.status = "fg " + fgLabel(m.curFg)
	case "tab":
		if len(m.pal) == 0 {
			m.status = "палитра не загружена (нет \"palette\" в манифесте)"
		} else {
			m.mode = modePalette
			m.ppSlot = m.lastSlot
			if m.ppSlot == 0 {
				m.ppSlot = 1
			}
		}
	case "[", "]":
		n := len(m.proj.Layers)
		if key == "[" {
			m.active = (m.active + n - 1) % n
		} else {
			m.active = (m.active + 1) % n
		}
		m.status = fmt.Sprintf("layer %d/%d %s", m.active+1, n, filepath.Base(m.layer().File))
	case "v":
		m.solo = !m.solo
		if m.solo {
			m.status = "solo"
		}
	case "u":
		if m.layer().Buf.Undo() {
			m.status = "undo"
		} else {
			m.status = "nothing to undo"
		}
	case "U":
		if m.layer().Buf.Redo() {
			m.status = "redo"
		} else {
			m.status = "nothing to redo"
		}
	case "w":
		if err := m.proj.SaveAll(); err != nil {
			m.status = "save failed: " + err.Error()
		} else {
			m.status = "saved"
		}
	case "e":
		out := m.proj.ANSPath()
		if err := os.WriteFile(out, m.proj.ExportANS(), 0o644); err != nil {
			m.status = "export failed: " + err.Error()
		} else {
			m.status = "exported " + filepath.Base(out)
		}
	case "q":
		if m.proj.Dirty() {
			m.mode = modeQuit
			return m, nil
		}
		return m, tea.Quit
	case "ctrl+c":
		return m, tea.Quit
	default:
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			m.fillOrStamp(int(key[0]-'0'), true)
		}
	}
	m.clampCursor()
	m.clampView()
	return m, nil
}

func (m *model) moveCursor(dir string) {
	switch dir {
	case "up":
		m.curR--
	case "down":
		m.curR++
	case "left":
		m.curC--
	case "right":
		m.curC++
	}
}

// selRect — нормализованный прямоугольник выделения (включительно).
func (m *model) selRect() (r1, c1, r2, c2 int) {
	r1, r2 = min(m.selR, m.curR), max(m.selR, m.curR)
	c1, c2 = min(m.selC, m.curC), max(m.selC, m.curC)
	return
}

// cellFg — что писать в Cell.Fg: override только если кисть ≠ база слоя.
func (m *model) cellFg(g rune) int {
	if g == ' ' || m.curFg == m.layer().Fg {
		return -1
	}
	return m.curFg
}

func (m *model) fillOrStamp(slot int, advance bool) {
	g := m.proj.Slots[slot]
	L := m.layer()
	if m.selOn {
		r1, c1, r2, c2 := m.selRect()
		var group []cellbuf.Change
		for r := r1; r <= r2; r++ {
			for c := c1; c <= c2; c++ {
				lr, lc := r-L.AtR, c-L.AtC
				if lr < 0 || lr >= L.Buf.H || lc < 0 || lc >= L.Buf.W {
					continue
				}
				group = append(group, cellbuf.Change{Row: lr, Col: lc,
					After: cellbuf.Cell{G: g, Fg: m.cellFg(g)}})
			}
		}
		if len(group) == 0 {
			m.status = "выделение вне слоя"
			return
		}
		L.Buf.Apply(group)
		m.lastSlot = slot
		return
	}
	lr, lc := m.curR-L.AtR, m.curC-L.AtC
	if lr < 0 || lr >= L.Buf.H || lc < 0 || lc >= L.Buf.W {
		m.status = "вне активного слоя"
		return
	}
	L.Buf.Apply([]cellbuf.Change{{Row: lr, Col: lc, After: cellbuf.Cell{G: g, Fg: m.cellFg(g)}}})
	m.lastSlot = slot
	if advance && m.curC < m.proj.CanvasW-1 {
		m.curC++
	}
}

// ── braille dot-mode ───────────────────────────────────────────────────

// dotBits — бит точки брайля по [ряд][столбец] сетки 2×4:
// d1,d2,d3,d7 — левый столбец, d4,d5,d6,d8 — правый. Глиф = U+2800 | маска.
var dotBits = [4][2]int{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// moveDot двигает курсор на одну точку в глобальной дот-сетке канвы
// (W×2, H×4); границы ячеек прозрачны.
func (m *model) moveDot(dir string) {
	dy := m.curR*4 + m.dotR
	dx := m.curC*2 + m.dotC
	switch dir {
	case "up":
		dy--
	case "down":
		dy++
	case "left":
		dx--
	case "right":
		dx++
	}
	dy = min(max(dy, 0), m.proj.CanvasH*4-1)
	dx = min(max(dx, 0), m.proj.CanvasW*2-1)
	m.curR, m.dotR = dy/4, dy%4
	m.curC, m.dotC = dx/2, dx%2
}

// brailleMask — маска точек глифа; не-брайль (включая пробел) = 0,
// поэтому первый тоггл на чужой ячейке заменяет её брайлем с одной точкой.
func brailleMask(g rune) int {
	if g >= 0x2800 && g <= 0x28FF {
		return int(g - 0x2800)
	}
	return 0
}

// toggleDot — XOR точки под курсором. Погасшая последняя точка пишет
// пробел (прозрачность), не U+2800.
func (m *model) toggleDot() {
	L := m.layer()
	lr, lc := m.curR-L.AtR, m.curC-L.AtC
	if lr < 0 || lr >= L.Buf.H || lc < 0 || lc >= L.Buf.W {
		m.status = "вне активного слоя"
		return
	}
	mask := brailleMask(L.Buf.Get(lr, lc).G) ^ dotBits[m.dotR][m.dotC]
	after := cellbuf.Blank
	if mask != 0 {
		g := rune(0x2800 + mask)
		after = cellbuf.Cell{G: g, Fg: m.cellFg(g)}
	}
	L.Buf.Apply([]cellbuf.Change{{Row: lr, Col: lc, After: after}})
}

// ── flip (mirror) ──────────────────────────────────────────────────────

// Зеркальные пары глифов. Брайль и октанты зеркалятся арифметикой (см.
// mirrorGlyph); здесь — ASCII и блок-элементы. Симметричные (эгейские
// кресты/решётки, '│', 'X'…) в таблицах отсутствуют → проходят как есть.
var mirrorHPairs = map[rune]rune{
	'(': ')', '[': ']', '{': '}', '<': '>', '/': '\\',
	'▌': '▐', '▖': '▗', '▘': '▝', '▙': '▟', '▛': '▜', '▚': '▞', '◀': '▶',
	'╱': '╲', 'b': 'd', 'p': 'q',
}
var mirrorVPairs = map[rune]rune{
	'/': '\\', '▀': '▄', '▘': '▖', '▝': '▗', '▛': '▙', '▜': '▟',
	'▚': '▞', 'v': '^', 'b': 'p', 'd': 'q', 'M': 'W',
}

func symPairs(m map[rune]rune) map[rune]rune {
	out := make(map[rune]rune, len(m)*2)
	for a, b := range m {
		out[a], out[b] = b, a
	}
	return out
}

var mirrorH = symPairs(mirrorHPairs)
var mirrorV = symPairs(mirrorVPairs)

// brailleSwap переставляет биты точек по списку пар (a,b).
func brailleSwap(mask int, pairs [4][2]int) int {
	out := 0
	for _, p := range pairs {
		if mask&p[0] != 0 {
			out |= p[1]
		}
		if mask&p[1] != 0 {
			out |= p[0]
		}
	}
	return out
}

// mirrorGlyph зеркалит один глиф по горизонтали (h=true) или вертикали.
func mirrorGlyph(g rune, h bool) rune {
	if g >= 0x2800 && g <= 0x28FF {
		mask := int(g - 0x2800)
		if h {
			return rune(0x2800 + brailleSwap(mask, [4][2]int{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}))
		}
		return rune(0x2800 + brailleSwap(mask, [4][2]int{{0x01, 0x40}, {0x02, 0x04}, {0x08, 0x80}, {0x10, 0x20}}))
	}
	tbl := mirrorV
	if h {
		tbl = mirrorH
	}
	if p, ok := tbl[g]; ok {
		return p
	}
	return g
}

// flip зеркалит выделение (или весь активный слой) по оси, зеркаля и сами
// глифы. Один шаг undo. Цвет ячейки едет вместе с глифом — двухтон сохраняется.
func (m *model) flip(h bool) {
	L := m.layer()
	r1, c1, r2, c2 := 0, 0, L.Buf.H-1, L.Buf.W-1
	if m.selOn {
		sr1, sc1, sr2, sc2 := m.selRect()
		r1, c1, r2, c2 = sr1-L.AtR, sc1-L.AtC, sr2-L.AtR, sc2-L.AtC
	}
	r1, c1 = max(r1, 0), max(c1, 0)
	r2, c2 = min(r2, L.Buf.H-1), min(c2, L.Buf.W-1)
	if r1 > r2 || c1 > c2 {
		m.status = "flip: пусто"
		return
	}
	// снимок — After считаем из до-флипового состояния, порядок Apply не важен
	snap := make([][]cellbuf.Cell, r2-r1+1)
	for r := r1; r <= r2; r++ {
		row := make([]cellbuf.Cell, c2-c1+1)
		for c := c1; c <= c2; c++ {
			row[c-c1] = L.Buf.Get(r, c)
		}
		snap[r-r1] = row
	}
	var group []cellbuf.Change
	for r := r1; r <= r2; r++ {
		for c := c1; c <= c2; c++ {
			sr, sc := r, c
			if h {
				sc = c2 - (c - c1)
			} else {
				sr = r2 - (r - r1)
			}
			src := snap[sr-r1][sc-c1]
			after := cellbuf.Cell{G: mirrorGlyph(src.G, h), Fg: src.Fg}
			if after != L.Buf.Get(r, c) {
				group = append(group, cellbuf.Change{Row: r, Col: c, After: after})
			}
		}
	}
	if len(group) == 0 {
		m.status = "flip: без изменений"
		return
	}
	L.Buf.Apply(group)
	axis := "Y"
	if h {
		axis = "X"
	}
	m.status = fmt.Sprintf("flip %s (%d×%d)", axis, c2-c1+1, r2-r1+1)
}

// repaint красит глифы текущим цветом, не трогая символы.
func (m *model) repaint() {
	L := m.layer()
	r1, c1, r2, c2 := m.curR, m.curC, m.curR, m.curC
	if m.selOn {
		r1, c1, r2, c2 = m.selRect()
	}
	var group []cellbuf.Change
	for r := r1; r <= r2; r++ {
		for c := c1; c <= c2; c++ {
			lr, lc := r-L.AtR, c-L.AtC
			if lr < 0 || lr >= L.Buf.H || lc < 0 || lc >= L.Buf.W {
				continue
			}
			cell := L.Buf.Get(lr, lc)
			if cell.G == ' ' {
				continue
			}
			fg := -1
			if m.curFg != L.Fg {
				fg = m.curFg
			}
			if cell.Fg == fg {
				continue
			}
			group = append(group, cellbuf.Change{Row: lr, Col: lc,
				After: cellbuf.Cell{G: cell.G, Fg: fg}})
		}
	}
	if len(group) == 0 {
		m.status = "нечего красить"
		return
	}
	L.Buf.Apply(group)
	m.status = fmt.Sprintf("repaint %d cells → fg %s", len(group), fgLabel(m.curFg))
}

// yank копирует ячейку/выделение (cut=true — и стирает). В буфере Fg хранится
// эффективный цвет источника, чтобы вставка в слой с другим базовым fg не
// меняла картинку.
func (m *model) yank(cut bool) {
	L := m.layer()
	r1, c1, r2, c2 := m.curR, m.curC, m.curR, m.curC
	if m.selOn {
		r1, c1, r2, c2 = m.selRect()
	}
	clip := make([][]cellbuf.Cell, r2-r1+1)
	var erase []cellbuf.Change
	got := false
	for r := r1; r <= r2; r++ {
		row := make([]cellbuf.Cell, c2-c1+1)
		for c := c1; c <= c2; c++ {
			cell := cellbuf.Blank
			lr, lc := r-L.AtR, c-L.AtC
			if lr >= 0 && lr < L.Buf.H && lc >= 0 && lc < L.Buf.W {
				cell = L.Buf.Get(lr, lc)
				if cell.G != ' ' {
					got = true
					if cell.Fg < 0 {
						cell.Fg = L.Fg
					}
					if cut {
						erase = append(erase, cellbuf.Change{Row: lr, Col: lc, After: cellbuf.Blank})
					}
				}
			}
			row[c-c1] = cell
		}
		clip[r-r1] = row
	}
	if !got {
		m.status = "нечего копировать"
		return
	}
	m.clip = clip
	m.selOn = false
	verb := "yank"
	if cut {
		L.Buf.Apply(erase)
		verb = "cut"
	}
	m.status = fmt.Sprintf("%s %dx%d", verb, c2-c1+1, r2-r1+1)
}

// paste ставит буфер левым верхним углом в курсор; пробелы прозрачны,
// один шаг undo. Эффективный fg сворачивается обратно в override/базу слоя.
func (m *model) paste() {
	if len(m.clip) == 0 {
		m.status = "буфер пуст"
		return
	}
	L := m.layer()
	var group []cellbuf.Change
	for dr, row := range m.clip {
		for dc, cell := range row {
			if cell.G == ' ' {
				continue
			}
			lr, lc := m.curR+dr-L.AtR, m.curC+dc-L.AtC
			if lr < 0 || lr >= L.Buf.H || lc < 0 || lc >= L.Buf.W {
				continue
			}
			fg := cell.Fg
			if fg == L.Fg {
				fg = -1
			}
			group = append(group, cellbuf.Change{Row: lr, Col: lc,
				After: cellbuf.Cell{G: cell.G, Fg: fg}})
		}
	}
	if len(group) == 0 {
		m.status = "вставка вне слоя"
		return
	}
	L.Buf.Apply(group)
	m.status = fmt.Sprintf("paste %dx%d", len(m.clip[0]), len(m.clip))
}

// ── goto / quit prompts ────────────────────────────────────────────────

// updateResize — промпт W,H: кроп/расширение канвы. Сбрасывает undo-историю.
func (m model) updateResize(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeEdit
	case "enter":
		m.mode = modeEdit
		parts := strings.FieldsFunc(m.input, func(r rune) bool {
			return r == ',' || r == ' ' || r == 'x' || r == 'X'
		})
		if len(parts) != 2 {
			m.status = "нужно W,H"
			return m, nil
		}
		w, err1 := strconv.Atoi(parts[0])
		h, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || w < 1 || h < 1 {
			m.status = "нужно W,H"
			return m, nil
		}
		m.proj.Resize(w, h)
		m.clampCursor()
		m.clampView()
		m.status = fmt.Sprintf("canvas %d×%d (undo сброшен)", w, h)
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		s := msg.String()
		if len(s) == 1 && (s[0] >= '0' && s[0] <= '9' || s[0] == ',' || s[0] == ' ' || s[0] == 'x') {
			m.input += s
		}
	}
	return m, nil
}

func (m model) updateGoto(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeEdit
	case "enter":
		m.mode = modeEdit
		parts := strings.FieldsFunc(m.input, func(r rune) bool { return r == ',' || r == ' ' })
		if len(parts) >= 1 {
			if r, err := strconv.Atoi(parts[0]); err == nil {
				m.curR = r - 1
			}
		}
		if len(parts) >= 2 {
			if c, err := strconv.Atoi(parts[1]); err == nil {
				m.curC = c - 1
			}
		}
		m.clampCursor()
		m.clampView()
	case "backspace":
		if len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
	default:
		s := msg.String()
		if len(s) == 1 && (s[0] >= '0' && s[0] <= '9' || s[0] == ',' || s[0] == ' ') {
			m.input += s
		}
	}
	return m, nil
}

func (m model) updateQuit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "q":
		return m, tea.Quit
	default:
		m.mode = modeEdit
		m.status = "quit cancelled"
	}
	return m, nil
}

// ── palette picker ─────────────────────────────────────────────────────

func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	page := m.pal[m.ppPage]
	switch key := msg.String(); key {
	case "esc":
		m.mode = modeEdit
	case "tab", "pgdown", "right+ctrl":
		m.ppPage = (m.ppPage + 1) % len(m.pal)
		m.ppRow, m.ppCol = 0, 0
	case "shift+tab", "pgup":
		m.ppPage = (m.ppPage + len(m.pal) - 1) % len(m.pal)
		m.ppRow, m.ppCol = 0, 0
	case "up":
		if m.ppRow > 0 {
			m.ppRow--
		}
	case "down":
		if m.ppRow < len(page.Rows)-1 {
			m.ppRow++
		}
	case "left":
		if m.ppCol > 0 {
			m.ppCol--
		}
	case "right":
		m.ppCol++
	case "enter":
		if len(page.Rows) > 0 {
			row := page.Rows[min(m.ppRow, len(page.Rows)-1)]
			g := row[min(m.ppCol, len(row)-1)]
			m.proj.SetSlot(m.ppSlot, g)
			m.lastSlot = m.ppSlot
			m.mode = modeEdit
			m.status = fmt.Sprintf("slot %d = %c U+%04X", m.ppSlot, g, g)
		}
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			m.ppSlot = int(key[0] - '0') // 0 — ластик, целью не выбирается
		}
	}
	if len(page.Rows) > 0 {
		m.ppRow = min(m.ppRow, len(page.Rows)-1)
		m.ppCol = min(m.ppCol, len(page.Rows[m.ppRow])-1)
		m.ppCol = max(m.ppCol, 0)
	}
	return m, nil
}

// ── color picker ───────────────────────────────────────────────────────

func (m model) updateColor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeEdit
	case "up":
		if m.cp >= 16 {
			m.cp -= 16
		}
	case "down":
		if m.cp < 240 {
			m.cp += 16
		}
	case "left":
		if m.cp > 0 {
			m.cp--
		}
	case "right":
		if m.cp < 255 {
			m.cp++
		}
	case "enter":
		m.curFg = m.cp
		m.mode = modeEdit
		m.status = "fg " + fgLabel(m.curFg)
	}
	return m, nil
}

// ── clamping ───────────────────────────────────────────────────────────

func (m *model) clampCursor() {
	m.curR = min(max(m.curR, 0), m.proj.CanvasH-1)
	m.curC = min(max(m.curC, 0), m.proj.CanvasW-1)
}

func (m *model) clampView() {
	vh := m.viewH()
	if m.curR < m.offR {
		m.offR = m.curR
	}
	if m.curR >= m.offR+vh {
		m.offR = m.curR - vh + 1
	}
	if m.curC < m.offC {
		m.offC = m.curC
	}
	if m.curC >= m.offC+m.viewW() {
		m.offC = m.curC - m.viewW() + 1
	}
	m.offR = max(m.offR, 0)
	m.offC = max(m.offC, 0)
}

// ── compositing & view ─────────────────────────────────────────────────

type vcell struct {
	g     rune
	fg    int
	layer int // -1 = пустая канва
}

// compositeCell — видимая ячейка канвы (для eyedropper).
func (m *model) compositeCell(r, c int) vcell {
	grid := m.composite()
	if r >= 0 && r < len(grid) && c >= 0 && c < len(grid[r]) {
		return grid[r][c]
	}
	return vcell{' ', -1, -1}
}

func (m *model) composite() [][]vcell {
	W, H := m.proj.CanvasW, m.proj.CanvasH
	grid := make([][]vcell, H)
	for r := range grid {
		grid[r] = make([]vcell, W)
		for c := range grid[r] {
			grid[r][c] = vcell{' ', -1, -1}
		}
	}
	put := func(y, x int, vc vcell) {
		if y >= 0 && y < H && x >= 0 && x < W {
			grid[y][x] = vc
		}
	}
	for li, L := range m.proj.Layers {
		if m.solo && li != m.active {
			continue
		}
		type pos struct{ y, x int }
		var cells []pos
		for r := 0; r < L.Buf.H; r++ {
			for c := 0; c < L.Buf.W; c++ {
				if L.Buf.Get(r, c).G != ' ' {
					cells = append(cells, pos{L.AtR + r, L.AtC + c})
				}
			}
		}
		if k := L.Knockout; k > 0 {
			for _, p := range cells {
				for dy := -k; dy <= k; dy++ {
					for dx := -k; dx <= k; dx++ {
						put(p.y+dy, p.x+dx, vcell{' ', -1, li})
					}
				}
			}
		}
		for _, p := range cells {
			cl := L.Buf.Get(p.y-L.AtR, p.x-L.AtC)
			fg := L.Fg
			if cl.Fg >= 0 {
				fg = cl.Fg
			}
			put(p.y, p.x, vcell{cl.G, fg, li})
		}
	}
	return grid
}

func glyphWidth(g rune) int {
	if w := runewidth.RuneWidth(g); w > 0 {
		return w
	}
	return 1
}

func (m model) View() string {
	switch m.mode {
	case modeSplash:
		return m.viewSplash()
	case modeHelp:
		return m.viewHelp()
	case modePalette:
		return m.viewPalette()
	case modeColor:
		return m.viewColor()
	}
	grid := m.composite()
	r1, c1, r2, c2 := 0, 0, -1, -1
	if m.selOn {
		r1, c1, r2, c2 = m.selRect()
	}
	var b strings.Builder
	b.WriteString(m.frameTop())
	b.WriteByte('\n')
	vh, vw := m.viewH(), m.viewW()
	bar := frameStyle.Render("│")
	for vr := 0; vr < vh; vr++ {
		r := m.offR + vr
		b.WriteString(bar)
		acc := 0
		if r < len(grid) {
			for c := m.offC; c < m.proj.CanvasW; c++ {
				vc := grid[r][c]
				gw := glyphWidth(vc.g)
				if acc+gw > vw {
					break
				}
				st := lipgloss.NewStyle()
				if vc.fg >= 0 {
					st = st.Foreground(lipgloss.Color(strconv.Itoa(vc.fg)))
				}
				if !m.solo && vc.layer >= 0 && vc.layer != m.active {
					st = st.Faint(true)
				}
				switch {
				case r == m.curR && c == m.curC:
					st = cursorStyle
					if m.dotOn {
						// превью того, что сделает `.`: маска с XOR целевой точки
						vc.g = rune(0x2800 + (brailleMask(vc.g) ^ dotBits[m.dotR][m.dotC]))
					}
				case r >= r1 && r <= r2 && c >= c1 && c <= c2:
					st = st.Background(lipgloss.Color("238"))
				}
				b.WriteString(st.Render(string(vc.g)))
				acc += gw
			}
		}
		if acc < vw {
			b.WriteString(strings.Repeat(" ", vw-acc))
		}
		b.WriteString(bar)
		b.WriteByte('\n')
	}
	b.WriteString(frameStyle.Render("╰" + strings.Repeat("─", vw) + "╯"))
	b.WriteByte('\n')
	b.WriteString(m.statusBar())
	return b.String()
}

func fgLabel(fg int) string {
	if fg < 0 {
		return "def"
	}
	return strconv.Itoa(fg)
}

// viewSplash — стартовый хедер как часть рамки: октанты в верхней линии
// по обе стороны от имени, дескриптор внутри. Исчезает по любой клавише.
func (m model) viewSplash() string {
	vw, vh := m.viewW(), m.viewH()
	name := m.proj.Path
	if name == "" {
		name = m.layer().AbsPath
	}

	title := fmt.Sprintf(" CLIIZDAT v%s ", appVersion)
	fill := max(0, vw-runewidth.StringWidth(title))
	strip := func(r []rune, n int) string {
		if n > len(r) {
			n = len(r)
		}
		return markStyle.Render(string(r[:n]))
	}
	top := frameStyle.Render("╭") + strip(m.deco[0], fill/2) + title +
		strip(m.deco[1], fill-fill/2) + frameStyle.Render("╮")

	center := func(plain, rendered string) string {
		lpad := max(0, (vw-runewidth.StringWidth(plain))/2)
		rpad := max(0, vw-runewidth.StringWidth(plain)-lpad)
		return frameStyle.Render("│") + strings.Repeat(" ", lpad) + rendered +
			strings.Repeat(" ", rpad) + frameStyle.Render("│")
	}
	var body []string
	for _, l := range wrapWords(splashDesc, min(60, vw-4)) {
		body = append(body, center(l, l))
	}
	info := fmt.Sprintf("%s · %d×%d · %d layer(s)",
		filepath.Base(name), m.proj.CanvasW, m.proj.CanvasH, len(m.proj.Layers))
	hint := "any key to begin"
	body = append(body, center("", ""), center(info, info), center(hint, faintStyle.Render(hint)))

	blank := center("", "")
	tpad := max(0, (vh-len(body))/2)
	var b strings.Builder
	b.WriteString(top)
	b.WriteByte('\n')
	for i := 0; i < vh; i++ {
		switch {
		case i >= tpad && i-tpad < len(body):
			b.WriteString(body[i-tpad])
		default:
			b.WriteString(blank)
		}
		b.WriteByte('\n')
	}
	b.WriteString(frameStyle.Render("╰" + strings.Repeat("─", vw) + "╯"))
	b.WriteByte('\n')
	b.WriteString(m.statusBar())
	return b.String()
}

// wrapWords — перенос по словам под ширину w.
func wrapWords(s string, w int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		switch {
		case line == "":
			line = word
		case runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= w:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

var helpRows = [][2]string{
	{"arrows", "move cursor"},
	{"Shift+←→ / s", "select (s: anchor, arrows grow, Esc to drop)"},
	{"Home End", "line start / end"},
	{"PgUp PgDn", "scroll one screen"},
	{"g", "go to row,col"},
	{"", ""},
	{"1–9", "stamp slot (fills the selection)"},
	{"0 / x", "erase (slot 0 is always the eraser)"},
	{"y d p", "copy · cut · paste"},
	{"b", "braille dot-mode: arrows move per dot, `.` toggles"},
	{"X Y", "flip selection/layer horizontally · vertically (mirrors glyphs)"},
	{"", ""},
	{"c", "256-color picker"},
	{"C", "eyedropper: pick color under cursor"},
	{"r", "repaint glyph(s) with current color"},
	{"Tab", "glyph palette → slot"},
	{"", ""},
	{"[ ]", "previous / next layer"},
	{"v", "solo the active layer"},
	{"u U", "undo · redo"},
	{"", ""},
	{"R", "resize canvas (crop / extend)"},
	{"w", "save · e — export .ans"},
	{"q", "quit · h — this help"},
}

// viewHelp — список управления в рамке. Закрывается любой клавишей.
func (m model) viewHelp() string {
	vw, vh := m.viewW(), m.viewH()
	line := func(s string) string {
		return frameStyle.Render("│") + " " + s +
			strings.Repeat(" ", max(0, vw-1-runewidth.StringWidth(s))) + frameStyle.Render("│")
	}
	var body []string
	body = append(body, "", "  cliizdat — controls", "")
	for _, r := range helpRows {
		if r[0] == "" {
			body = append(body, "")
			continue
		}
		body = append(body, fmt.Sprintf("  %-12s %s", r[0], r[1]))
	}
	body = append(body, "", "  any key to close")

	var b strings.Builder
	b.WriteString(frameStyle.Render("╭─ ") + markStyle.Render("༒") + " help " +
		frameStyle.Render(strings.Repeat("─", max(0, vw-8))+"╮"))
	b.WriteByte('\n')
	for i := 0; i < vh; i++ {
		if i < len(body) {
			b.WriteString(line(body[i]))
		} else {
			b.WriteString(line(""))
		}
		b.WriteByte('\n')
	}
	b.WriteString(frameStyle.Render("╰" + strings.Repeat("─", vw) + "╯"))
	b.WriteByte('\n')
	b.WriteString(m.statusBar())
	return b.String()
}

const wordmark = "༒cliizdat༒"

// frameTop — титул рамки: ╭─ ༒cliizdat༒ file ────╮ на всю ширину терминала.
func (m model) frameTop() string {
	name := m.proj.Path
	if name == "" {
		name = m.layer().AbsPath
	}
	title := " " + filepath.Base(name) + " "
	used := runewidth.StringWidth("╭─ "+wordmark+title) + 1 // +╮
	fill := m.termW - used
	if fill < 0 {
		fill = 0
	}
	return frameStyle.Render("╭─ ") + markStyle.Render(wordmark) + title +
		frameStyle.Render(strings.Repeat("─", fill)+"╮")
}

func (m model) statusBar() string {
	var s string
	switch m.mode {
	case modeGoto:
		s = " go to row,col: " + m.input + "█"
	case modeResize:
		s = fmt.Sprintf(" canvas %d×%d → W,H: %s█", m.proj.CanvasW, m.proj.CanvasH, m.input)
	case modeQuit:
		s = " unsaved changes — quit? (y/n)"
	default:
		g := m.compositeCell(m.curR, m.curC).g
		dirty := ""
		if m.proj.Dirty() {
			dirty = " *"
		}
		sel := ""
		if m.selOn {
			r1, c1, r2, c2 := m.selRect()
			sel = fmt.Sprintf(" │ sel %dx%d", c2-c1+1, r2-r1+1)
		}
		solo := ""
		if m.solo {
			solo = " │ SOLO"
		}
		dot := ""
		if m.dotOn {
			dot = fmt.Sprintf(" │ DOT %d,%d", m.curR*4+m.dotR+1, m.curC*2+m.dotC+1)
		}
		left := fmt.Sprintf(" %d,%d │ L%d/%d │ %d%c U+%04X │ fg %s ",
			m.curR+1, m.curC+1,
			m.active+1, len(m.proj.Layers),
			m.lastSlot, m.proj.Slots[m.lastSlot], g,
			fgLabel(m.curFg))
		if m.status != "" {
			left = " " + m.status + " │" + left
		}
		right := dot + sel + solo + dirty
		if pad := m.termW - runewidth.StringWidth(left) - 1 - runewidth.StringWidth(right); pad > 0 {
			right += strings.Repeat(" ", pad)
		}
		swatch := statusStyle.Render("■")
		if m.curFg >= 0 {
			swatch = lipgloss.NewStyle().
				Background(lipgloss.Color(strconv.Itoa(m.curFg))).Render(" ")
		}
		return statusStyle.Render(left) + swatch + statusStyle.Render(right)
	}
	if w := runewidth.StringWidth(s); w < m.termW {
		s += strings.Repeat(" ", m.termW-w)
	}
	return statusStyle.Render(s)
}

func (m model) viewPalette() string {
	page := m.pal[m.ppPage]
	var b strings.Builder
	pages := ""
	if len(m.pal) > 1 {
		pages = fmt.Sprintf(" (%d/%d, Tab)", m.ppPage+1, len(m.pal))
	}
	fmt.Fprintf(&b, " palette: %s%s  →  slot %d %c   [1-9 слот · Enter взять · Esc]\n\n",
		page.Name, pages, m.ppSlot, m.proj.Slots[m.ppSlot])
	// окно строк, чтобы длинные секции влезали
	vh := max(1, m.termH-4)
	start := 0
	if m.ppRow >= vh {
		start = m.ppRow - vh + 1
	}
	for ri := start; ri < len(page.Rows) && ri < start+vh; ri++ {
		row := page.Rows[ri]
		b.WriteString("  ")
		for ci, g := range row {
			cell := runewidth.FillRight(string(g), 2)
			if ri == m.ppRow && ci == m.ppCol {
				cell = cursorStyle.Render(cell)
			}
			b.WriteString(cell)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (m model) viewColor() string {
	var b strings.Builder
	fmt.Fprintf(&b, " color: %d   [стрелки · Enter взять · Esc]\n\n", m.cp)
	for row := 0; row < 16; row++ {
		b.WriteString("  ")
		for col := 0; col < 16; col++ {
			idx := row*16 + col
			st := lipgloss.NewStyle().Foreground(lipgloss.Color(strconv.Itoa(idx)))
			sw := "██"
			if idx == m.cp {
				st = st.Reverse(true)
				sw = "▓▓"
			}
			b.WriteString(st.Render(sw))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
