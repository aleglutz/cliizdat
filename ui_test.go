package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/aleglutz/cliizdat/cellbuf"
	"github.com/aleglutz/cliizdat/palette"
	"github.com/aleglutz/cliizdat/project"
)

func TestOctantWidthIsOne(t *testing.T) {
	for _, g := range []rune{'\U0001CD00', '\U0001CDE5', '\U0001FB00'} {
		if w := runewidth.RuneWidth(g); w != 1 {
			t.Fatalf("RuneWidth(%U) = %d; want 1", g, w)
		}
	}
}

// newTestModel — как newModel, но со снятым сплэшем (в жизни его закрывает
// первое нажатие клавиши).
func newTestModel(p *project.Project, pal []palette.Page) model {
	m := newModel(p, pal)
	m.mode = modeEdit
	return m
}

func press(m model, keys ...string) model {
	types := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"shift+up": tea.KeyShiftUp, "shift+down": tea.KeyShiftDown,
		"shift+left": tea.KeyShiftLeft, "shift+right": tea.KeyShiftRight,
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab,
	}
	for _, key := range keys {
		var k tea.KeyMsg
		if kt, ok := types[key]; ok {
			k = tea.KeyMsg{Type: kt}
		} else {
			k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
		nm, _ := m.Update(k)
		m = nm.(model)
	}
	return m
}

func blink(m model) model {
	nm, _ := m.Update(dotBlinkMsg{})
	return nm.(model)
}

func implicitProject(t *testing.T, w, h int) *project.Project {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "l.txt")
	return &project.Project{
		Dir:     dir,
		CanvasW: w, CanvasH: h,
		Layers: []*project.Layer{{File: "l.txt", AbsPath: path, Fg: -1, Buf: cellbuf.New(w, h), NewFile: true}},
		Slots:  project.DefaultSlots,
	}
}

func TestStampAdvanceUndoView(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)
	m.termW, m.termH = 60, 10

	m = press(m, "1")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '█' {
		t.Fatalf("stamp: %U", g)
	}
	if m.curC != 1 {
		t.Fatalf("cursor did not advance: col %d", m.curC)
	}
	m = press(m, "9")
	if g := p.Layers[0].Buf.Get(0, 1).G; g != '·' {
		t.Fatalf("stamp ·: %U", g)
	}
	m = press(m, "u")
	if g := p.Layers[0].Buf.Get(0, 1).G; g != ' ' {
		t.Fatalf("undo: %U", g)
	}
	view := m.View()
	if !strings.Contains(view, "█") {
		t.Fatal("View lost the stamped glyph")
	}
	if !strings.Contains(view, "1,3") {
		t.Fatal("status bar has no 1-based position")
	}
}

func TestSelectionFillAndRepaint(t *testing.T) {
	p := implicitProject(t, 10, 5)
	m := newTestModel(p, nil)
	m.termW, m.termH = 60, 10

	// выделение 3x2 от 0,0 и заливка слотом 1
	m = press(m, "shift+right", "shift+right", "shift+down", "1")
	buf := p.Layers[0].Buf
	for r := 0; r <= 1; r++ {
		for c := 0; c <= 2; c++ {
			if g := buf.Get(r, c).G; g != '█' {
				t.Fatalf("fill miss at %d,%d: %U", r, c, g)
			}
		}
	}
	// атомарный undo заливки
	m = press(m, "u")
	if buf.Get(0, 0).G != ' ' || buf.Get(1, 2).G != ' ' {
		t.Fatal("fill undo not atomic")
	}
	m = press(m, "U") // redo

	// repaint выделения цветом 208 (кисть через прямое поле — пикер отдельно)
	m.curFg = 208
	m = press(m, "r")
	if fg := buf.Get(0, 0).Fg; fg != 208 {
		t.Fatalf("repaint fg = %d", fg)
	}
	// глиф не тронут
	if g := buf.Get(0, 0).G; g != '█' {
		t.Fatalf("repaint touched glyph: %U", g)
	}
}

func TestEyedropper(t *testing.T) {
	p := implicitProject(t, 5, 2)
	m := newTestModel(p, nil)
	m.termW, m.termH = 40, 8
	m.curFg = 196
	m = press(m, "1")    // █ fg196 override at 0,0
	m = press(m, "left") // назад на ячейку
	m.curFg = -1
	m = press(m, "C")
	if m.curFg != 196 {
		t.Fatalf("eyedropper picked %d; want 196", m.curFg)
	}
}

func TestColorPicker(t *testing.T) {
	p := implicitProject(t, 5, 2)
	m := newTestModel(p, nil)
	m.termW, m.termH = 60, 24
	m = press(m, "c")
	if m.mode != modeColor {
		t.Fatal("c did not open color picker")
	}
	if !strings.Contains(m.View(), "color:") {
		t.Fatal("color picker view missing")
	}
	m = press(m, "down", "right", "enter") // 0 → 16 → 17
	if m.mode != modeEdit || m.curFg != 17 {
		t.Fatalf("picked fg %d; want 17", m.curFg)
	}
}

// Приёмка Фазы 2: NF-глиф в слот 4 из palette_nf.txt, штамп fg 208 поверх
// слоя с базой 240, reopen → слот, глиф и override живы.
func TestPhase2Acceptance(t *testing.T) {
	dir := t.TempDir()
	layer := strings.Repeat("▒▒▒▒▒▒▒▒\n", 4)
	os.WriteFile(filepath.Join(dir, "base.txt"), []byte(layer), 0o644)
	manifest := `{
  "canvas": [8, 4],
  "layers": [{"file": "base.txt", "at": [1, 1], "fg": 240}],
  "slots": ["█","▀","▄","𜴀","𜴵","𜶫","🮕","🮖","·"," "],
  "palette": "palette_nf.txt"
}`
	os.WriteFile(filepath.Join(dir, "project.json"), []byte(manifest), 0o644)
	src, err := os.ReadFile("palette/testdata/palette_nf.txt")
	if os.IsNotExist(err) {
		t.Skip("private palette palette_nf.txt absent (gitignored)")
	}
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "palette_nf.txt"), src, 0o644)

	p, err := project.Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	pal, err := palette.LoadAll(p.PalettePaths)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(p, pal)
	m.termW, m.termH = 80, 30

	// Tab → слот 4 → второй глиф первой страницы → Enter
	m = press(m, "tab", "4", "right", "enter")
	if m.mode != modeEdit {
		t.Fatal("palette picker did not close")
	}
	nfGlyph := pal[0].Rows[0][1]
	if p.Slots[4] != nfGlyph {
		t.Fatalf("slot4 = %U; want %U", p.Slots[4], nfGlyph)
	}
	// кисть 208, штамп слотом 4
	m.curFg = 208
	m = press(m, "4", "w")

	// reopen
	p2, err := project.Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Slots[4] != nfGlyph {
		t.Fatalf("slot did not survive reopen: %U", p2.Slots[4])
	}
	cell := p2.Layers[0].Buf.Get(0, 0)
	if cell.G != nfGlyph {
		t.Fatalf("glyph did not survive reopen: %U", cell.G)
	}
	if cell.Fg != 208 {
		t.Fatalf("override did not survive reopen: fg %d", cell.Fg)
	}
}

func TestLayerSwitchSoloAndKnockoutView(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bg.txt"), []byte("████\n████\n████\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "fg.txt"), []byte("·\n"), 0o644)
	manifest := `{
  "canvas": [4, 3],
  "layers": [
    {"file": "bg.txt", "at": [1, 1], "fg": 240},
    {"file": "fg.txt", "at": [2, 2], "fg": 231, "knockout": 1}
  ],
  "slots": ["█","▀","▄","𜴀","𜴵","𜶫","🮕","🮖","·"," "]
}`
	os.WriteFile(filepath.Join(dir, "project.json"), []byte(manifest), 0o644)
	p, err := project.Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(p, nil)
	m.termW, m.termH = 40, 8

	if m.active != 0 {
		t.Fatal("initial layer not 0")
	}
	m = press(m, "]")
	if m.active != 1 {
		t.Fatal("] did not switch layer")
	}
	// knockout: гало вокруг · выбило bg — в композите вокруг центра пробелы
	grid := m.composite()
	if grid[1][1].g != '·' {
		t.Fatalf("fg glyph missing: %U", grid[1][1].g)
	}
	if grid[0][0].g != ' ' || grid[2][2].g != ' ' {
		t.Fatal("knockout halo did not clear bg in composite")
	}
	if grid[0][3].g != '█' {
		t.Fatal("knockout ate too much")
	}
	m = press(m, "v")
	if !m.solo {
		t.Fatal("v did not toggle solo")
	}
	grid = m.composite()
	if grid[0][3].g != ' ' {
		t.Fatal("solo still shows other layers")
	}
}

func TestViewRendersOctants(t *testing.T) {
	p := implicitProject(t, 5, 2)
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0, After: cellbuf.Cell{G: '\U0001CD00', Fg: -1}}})
	m := newTestModel(p, nil)
	m.termW, m.termH = 20, 5
	if !strings.Contains(m.View(), "\U0001CD00") {
		t.Fatal("octant dropped from View")
	}
}

func TestStickySelection(t *testing.T) {
	p := implicitProject(t, 10, 5)
	m := newTestModel(p, nil)
	m.termW, m.termH = 60, 10

	// s + стрелки: рост вниз и вправо без shift
	m = press(m, "s", "down", "down", "right", "1")
	for r := 0; r < 3; r++ {
		for c := 0; c < 2; c++ {
			if g := p.Layers[0].Buf.Get(r, c).G; g != '█' {
				t.Fatalf("fill miss at %d,%d: %U", r, c, g)
			}
		}
	}
	// после заливки sticky-выделение живо; esc снимает
	if !m.selOn || !m.selSticky {
		t.Fatal("sticky selection dropped after fill")
	}
	m = press(m, "esc")
	if m.selOn || m.selSticky {
		t.Fatal("esc did not drop sticky selection")
	}
	// повторный s ставит новый якорь, второй s снимает
	m = press(m, "s", "up")
	if r1, _, r2, _ := m.selRect(); r1 != 1 || r2 != 2 {
		t.Fatalf("sticky rect rows %d..%d; want 1..2", r1, r2)
	}
	m = press(m, "s")
	if m.selOn {
		t.Fatal("second s did not drop selection")
	}
}

func TestSlotZeroIsAlwaysEraser(t *testing.T) {
	p := implicitProject(t, 10, 5)
	m := newTestModel(p, nil)
	m.termW, m.termH = 70, 26
	// прямой SetSlot(0) игнорируется
	p.SetSlot(0, '█')
	if p.Slots[0] != ' ' {
		t.Fatalf("SetSlot(0) rebound eraser to %U", p.Slots[0])
	}
	// штамп 0 стирает: поставим глиф, затем 0 поверх
	m = press(m, "1", "left", "0")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != ' ' {
		t.Fatalf("slot 0 did not erase: %U", g)
	}
}

func TestHelpOpensAndCloses(t *testing.T) {
	p := implicitProject(t, 10, 5)
	m := newTestModel(p, nil)
	m.termW, m.termH = 70, 26
	m = press(m, "h")
	if m.mode != modeHelp {
		t.Fatalf("h did not open help; mode=%d", m.mode)
	}
	if v := m.View(); !strings.Contains(v, "controls") || !strings.Contains(v, "always the eraser") {
		t.Fatal("help view missing content")
	}
	if press(m, "x").mode != modeEdit {
		t.Fatal("any key did not close help")
	}
}

// ── braille dot-mode (Phase 4a) ────────────────────────────────────────

func TestDotModeToggleAndErase(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)

	m = press(m, "b", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⠁' { // d1
		t.Fatalf("first dot: got %U, want U+2801", g)
	}
	m = press(m, "down", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⠃' { // d1+d2
		t.Fatalf("second dot: got %U, want U+2803", g)
	}
	// погасили обе — ячейка стала пробелом, не U+2800
	m = press(m, ".", "up", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != ' ' {
		t.Fatalf("cleared cell: got %U, want space", g)
	}
	_ = m
}

func TestDotModeCrossesCellBoundary(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)

	// 4 шага вниз от (0,0): точки 0..3 в ячейке 0 → точка 0 ячейки 1
	m = press(m, "b", "down", "down", "down", "down")
	if m.curR != 1 || m.dotR != 0 {
		t.Fatalf("cell crossing down: curR=%d dotR=%d", m.curR, m.dotR)
	}
	// 2 шага вправо: столбцы 0,1 ячейки → столбец 0 следующей
	m = press(m, "right", "right")
	if m.curC != 1 || m.dotC != 0 {
		t.Fatalf("cell crossing right: curC=%d dotC=%d", m.curC, m.dotC)
	}
	// клампится в глобальной дот-сетке
	m = press(m, "up", "up", "up", "up", "up", "up")
	if m.curR != 0 || m.dotR != 0 {
		t.Fatalf("clamp top: curR=%d dotR=%d", m.curR, m.dotR)
	}
}

func TestDotModeReplacesForeignGlyph(t *testing.T) {
	p := implicitProject(t, 10, 3)
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '\U00010122', Fg: 240}}}) // эгейский крест
	m := newTestModel(p, nil)

	m = press(m, "b", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⠁' {
		t.Fatalf("foreign glyph not replaced: %U", g)
	}
	// undo возвращает эгейский
	m = press(m, "u")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '\U00010122' {
		t.Fatalf("undo after replace: %U", g)
	}
	_ = m
}

func TestDotModeFgOverride(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)

	// кисть = база слоя (-1) → override не пишется
	m = press(m, "b", ".")
	if fg := p.Layers[0].Buf.Get(0, 0).Fg; fg != -1 {
		t.Fatalf("fg with base brush: %d, want -1", fg)
	}
	// кисть 246 ≠ базы → override
	m.curFg = 246
	m = press(m, "right", ".")
	if fg := p.Layers[0].Buf.Get(0, 0).Fg; fg != 246 {
		t.Fatalf("fg override: %d, want 246", fg)
	}
}

func TestDotModeLeavesStampingAlone(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)

	// вне дот-режима цифры штампуют как раньше
	m = press(m, "b", "esc", "1")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '█' {
		t.Fatalf("stamp after dot-mode exit: %U", g)
	}
	if m.dotOn {
		t.Fatal("dot-mode still on after esc")
	}
}

func TestDotCursorShowsContentAndBlinksTarget(t *testing.T) {
	p := implicitProject(t, 4, 2)
	// ⠉ = d1+d4 (верхний ряд); цель — d2 (левый столбец, второй ряд)
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '⠉', Fg: -1}}})
	m := newTestModel(p, nil)
	m.termW, m.termH = 20, 8
	m = press(m, "b", "down")

	// фаза 0 — правда: под курсором ровно то, что нарисовано
	if v := m.View(); !strings.Contains(v, "⠉") {
		t.Fatal("фаза 0: содержимое ячейки не показано как есть")
	}
	m = blink(m)
	if !m.dotPhase {
		t.Fatal("тик не переключил фазу")
	}
	if v := m.View(); !strings.Contains(v, "⠋") { // ⠉ + d2
		t.Fatal("фаза 1: целевая точка не подмешана в глиф")
	}
	// вне дот-режима фаза не бежит — курсор не мигает
	m = blink(press(m, "esc"))
	if m.dotPhase {
		t.Fatal("мигание продолжается вне дот-режима")
	}
}

func TestDotCursorMarksEraseOnInkedTarget(t *testing.T) {
	p := implicitProject(t, 4, 2)
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '⠁', Fg: -1}}}) // d1 — на нём и стоим
	m := newTestModel(p, nil)
	m = press(m, "b")

	_, st := m.dotCursor('⠁')
	if st.GetForeground() != dotEraseColor {
		t.Fatalf("цель закрашена → ждём цвет стирания, got %v", st.GetForeground())
	}
	if _, st := m.dotCursor(' '); st.GetForeground() != dotAddColor {
		t.Fatalf("цель пуста → ждём акцентный цвет, got %v", st.GetForeground())
	}
}

// ── octant dot-mode ────────────────────────────────────────────────────

func TestOctantTableIsComplete(t *testing.T) {
	if len(octMasks) != 256 {
		t.Fatalf("octMasks: %d глифов, want 256 (есть коллизии)", len(octMasks))
	}
	// 230 блочных октантов лежат подряд, пропуская 26 масок из octExtra
	if octGlyph[0x04] != 0x1CD00 || octGlyph[0xFE] != 0x1CDE5 {
		t.Fatalf("границы диапазона: %U %U", octGlyph[0x04], octGlyph[0xFE])
	}
	for mask, want := range map[int]rune{
		0x00: ' ', 0x01: '𜺨', 0x03: '🮂', 0x05: '▘', 0x0F: '▀',
		0x55: '▌', 0xF0: '▄', 0xFF: '█',
	} {
		if octGlyph[mask] != want {
			t.Fatalf("маска %#02x: got %U, want %U", mask, octGlyph[mask], want)
		}
	}
}

func TestOctantDotModeToggleAndErase(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)

	m = press(m, "o", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '𜺨' { // октант 1
		t.Fatalf("первый октант: got %U, want U+1CEA8", g)
	}
	m = press(m, "right", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '🮂' { // 1+2 = верхняя четверть
		t.Fatalf("второй октант: got %U, want U+1FB82", g)
	}
	m = press(m, "down", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '𜴅' { // октанты 1+2+4
		t.Fatalf("третий октант: got %U, want U+1CD05", g)
	}
	// гасим все три — ячейка пробел, не пустой октант
	m = press(m, ".", "up", ".", "left", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != ' ' {
		t.Fatalf("очищенная ячейка: got %U, want space", g)
	}
}

func TestDotModeRasterSwitch(t *testing.T) {
	p := implicitProject(t, 10, 3)
	m := newTestModel(p, nil)

	m = press(m, "b", "o") // чужая клавиша переключает растр, не выходит
	if !m.dotOn || !m.dotOct {
		t.Fatalf("b→o: dotOn=%v dotOct=%v", m.dotOn, m.dotOct)
	}
	m = press(m, ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '𜺨' {
		t.Fatalf("после переключения рисуем октантами: %U", g)
	}
	// брайль поверх октанта: чужое семейство = маска 0, ячейка заменяется
	m = press(m, "b", ".")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⠁' {
		t.Fatalf("брайль поверх октанта: %U", g)
	}
	if m = press(m, "b"); m.dotOn {
		t.Fatal("своя клавиша не выключила dot-mode")
	}
}

func TestFlipOctantGlyphInPlace(t *testing.T) {
	p := implicitProject(t, 1, 1)
	// 𜺨 = октант 1 (верх-лево). H → октант 2 (𜺫); V → октант 7 (𜺣).
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '𜺨', Fg: -1}}})
	m := newTestModel(p, nil)

	m = press(m, "X")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '𜺫' {
		t.Fatalf("flip X: got %U, want U+1CEAB", g)
	}
	m = press(m, "X", "Y")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '𜺣' {
		t.Fatalf("flip Y: got %U, want U+1CEA3", g)
	}
	// инволюция на всём диапазоне
	for mask := 0; mask < 256; mask++ {
		g := octGlyph[mask]
		if h := mirrorGlyph(mirrorGlyph(g, true), true); h != g {
			t.Fatalf("flip X дважды, маска %#02x: %U != %U", mask, h, g)
		}
		if v := mirrorGlyph(mirrorGlyph(g, false), false); v != g {
			t.Fatalf("flip Y дважды, маска %#02x: %U != %U", mask, v, g)
		}
	}
}

// ── flip (Phase 4a) ────────────────────────────────────────────────────

func TestFlipBrailleGlyphInPlace(t *testing.T) {
	p := implicitProject(t, 1, 1) // 1×1 → позиция не меняется, виден только бит-свап
	// ⠁ = d1 (верх-лево). H-mirror → d4 (⠈); V-mirror → d7 (⡀).
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '⠁', Fg: -1}}})
	m := newTestModel(p, nil)

	m = press(m, "X")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⠈' {
		t.Fatalf("flip X of ⠁: got %U, want U+2808 ⠈", g)
	}
	m = press(m, "X", "Y") // вернуть ⠁, затем вертикально → ⡀ (d7)
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⡀' {
		t.Fatalf("flip Y of ⠁: got %U, want U+2840 ⡀", g)
	}
	_ = m
}

func TestFlipHorizontalMovesAndMirrors(t *testing.T) {
	p := implicitProject(t, 4, 1)
	// ⠁ в колонке 0; после flip X всего слоя (ширина 4) уедет в колонку 3 как ⠈
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '⠁', Fg: 240}}})
	m := newTestModel(p, nil)

	m = press(m, "X")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != ' ' {
		t.Fatalf("col0 after flip: %U, want space", g)
	}
	c3 := p.Layers[0].Buf.Get(0, 3)
	if c3.G != '⠈' {
		t.Fatalf("col3 after flip: %U, want ⠈", c3.G)
	}
	if c3.Fg != 240 {
		t.Fatalf("fg did not travel: %d, want 240", c3.Fg)
	}
	_ = m
}

func TestFlipVerticalReversesRows(t *testing.T) {
	p := implicitProject(t, 2, 3)
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: 'A', Fg: -1}}})
	m := newTestModel(p, nil)

	m = press(m, "Y")
	if g := p.Layers[0].Buf.Get(2, 0).G; g != 'A' {
		t.Fatalf("row0 should move to row2: %U", g)
	}
	if g := p.Layers[0].Buf.Get(0, 0).G; g != ' ' {
		t.Fatalf("row0 should clear: %U", g)
	}
	_ = m
}

func TestFlipWholeIsOneUndo(t *testing.T) {
	p := implicitProject(t, 4, 1)
	p.Layers[0].Buf.Apply([]cellbuf.Change{{Row: 0, Col: 0,
		After: cellbuf.Cell{G: '⠇', Fg: -1}}})
	m := newTestModel(p, nil)

	m = press(m, "X", "u")
	if g := p.Layers[0].Buf.Get(0, 0).G; g != '⠇' {
		t.Fatalf("single undo should restore original: %U", g)
	}
	_ = m
}

func TestFlipSelectionOnly(t *testing.T) {
	p := implicitProject(t, 4, 1)
	// '/' в col0, '/' в col3; выделяем col0..col1 → зеркалим только их
	p.Layers[0].Buf.Apply([]cellbuf.Change{
		{Row: 0, Col: 0, After: cellbuf.Cell{G: '/', Fg: -1}},
		{Row: 0, Col: 3, After: cellbuf.Cell{G: '/', Fg: -1}},
	})
	m := newTestModel(p, nil)
	// выделение col0..col1 (курсор в 0,0 → shift+right)
	m = press(m, "shift+right", "X")
	if g := p.Layers[0].Buf.Get(0, 1).G; g != '\\' {
		t.Fatalf("slash in selection should mirror to backslash at col1: %U", g)
	}
	if g := p.Layers[0].Buf.Get(0, 3).G; g != '/' {
		t.Fatalf("slash outside selection must stay: %U", g)
	}
	_ = m
}

// ── light-bg grayscale reflection (Phase 4a) ───────────────────────────

func TestAdaptFgReflectsGrayOnLight(t *testing.T) {
	p := implicitProject(t, 4, 2)
	m := newTestModel(p, nil)

	m.lightBg = false
	for _, fg := range []int{240, 246, 232, 255, 231, 15, -1} {
		if got := m.adaptFg(fg); got != fg {
			t.Fatalf("dark bg: adaptFg(%d)=%d, want unchanged", fg, got)
		}
	}

	m.lightBg = true
	cases := map[int]int{
		246: 241, // drawing → darker (stronger on light)
		240: 247, // texture → lighter (recedes into paper)
		232: 255, // ramp ends swap
		255: 232,
		231: 231, // colored: untouched
		15:  15,
		-1:  -1, // default: untouched
	}
	for fg, want := range cases {
		if got := m.adaptFg(fg); got != want {
			t.Fatalf("light bg: adaptFg(%d)=%d, want %d", fg, got, want)
		}
	}
}

// Новый проект: холст 80×55, все палитры в пикере, Tab листает страницы.
func TestNewProjectPicker(t *testing.T) {
	dir := t.TempDir()
	pals := filepath.Join(dir, "palettes")
	os.MkdirAll(pals, 0o755)
	os.WriteFile(filepath.Join(pals, "a_braille.txt"), []byte("== BRAILLE ==\n⠁⠂⠄\n"), 0o644)
	os.WriteFile(filepath.Join(pals, "b_octants.txt"), []byte("== OCTANTS ==\n\U0001CD00 \U0001CD01\n"), 0o644)
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)

	path, err := project.New("sigil", project.DefaultCanvasW, project.DefaultCanvasH, project.DiscoverPalettes(dir))
	if err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	pal, err := palette.LoadAll(p.PalettePaths)
	if err != nil {
		t.Fatal(err)
	}
	if p.CanvasW != 80 || p.CanvasH != 55 {
		t.Fatalf("холст %dx%d", p.CanvasW, p.CanvasH)
	}
	m := newTestModel(p, pal)
	m.termW, m.termH = 100, 40
	m = press(m, "tab")
	if m.mode != modePalette {
		t.Fatal("пикер не открылся")
	}
	if !strings.Contains(m.View(), "BRAILLE") || !strings.Contains(m.View(), "(1/2, Tab)") {
		t.Errorf("первая страница/счётчик не показаны:\n%s", m.View())
	}
	m = press(m, "tab")
	if !strings.Contains(m.View(), "OCTANTS") {
		t.Errorf("Tab не пролистал на октанты:\n%s", m.View())
	}
}

// Общий буфер: `y` в одном инстансе → `p` в другом (соседняя вкладка терминала).
func TestSharedClipboardBetweenInstances(t *testing.T) {
	t.Setenv("CLIIZDAT_CLIPBOARD", filepath.Join(t.TempDir(), "clipboard.json"))

	// вкладка A: рисуем 2 глифа цветом 208 и копируем их
	pa := implicitProject(t, 6, 3)
	a := newTestModel(pa, nil)
	a.termW, a.termH = 60, 12
	a.curFg = 208
	a = press(a, "1", "9")                      // █ · в 0,0 и 0,1
	a = press(a, "left", "left", "shift+right") // выделение 2x1 от 0,0
	a = press(a, "y")
	if len(a.clip) != 1 || len(a.clip[0]) != 2 {
		t.Fatalf("yank %dx%d", len(a.clip[0]), len(a.clip))
	}

	// вкладка B: свой проект, своя память — буфер приходит из файла
	pb := implicitProject(t, 6, 3)
	b := newTestModel(pb, nil)
	b.termW, b.termH = 60, 12
	b = press(b, "down", "p")
	buf := pb.Layers[0].Buf
	if g := buf.Get(1, 0).G; g != '█' {
		t.Fatalf("вставка не пришла: %U", g)
	}
	if g := buf.Get(1, 1).G; g != '·' {
		t.Fatalf("второй глиф: %U", g)
	}
	if fg := buf.Get(1, 0).Fg; fg != 208 {
		t.Fatalf("цвет потерян: %d", fg)
	}
	if !strings.Contains(b.status, "общий буфер") {
		t.Errorf("статус не отметил чужой буфер: %q", b.status)
	}
	b = press(b, "u")
	if buf.Get(1, 0).G != ' ' || buf.Get(1, 1).G != ' ' {
		t.Error("вставка не откатилась одним undo")
	}

	// своя копия в B перебивает файл; A после этого видит уже её
	b = press(b, "esc", "2")
	b = press(b, "left", "y")
	a = press(a, "down", "down", "p")
	if len(a.clip) != 1 || len(a.clip[0]) != 1 || a.clip[0][0].G != '▀' {
		t.Fatalf("A не подхватил свежий буфер B: %+v", a.clip)
	}
	found := false
	for c := 0; c < pa.Layers[0].Buf.W; c++ {
		if pa.Layers[0].Buf.Get(2, c).G == '▀' {
			found = true
		}
	}
	if !found {
		t.Error("A не вставил свежий буфер B")
	}
}

// Свой же буфер после y не перечитывается как чужой.
func TestClipboardOwnWriteNotForeign(t *testing.T) {
	t.Setenv("CLIIZDAT_CLIPBOARD", filepath.Join(t.TempDir(), "clipboard.json"))
	p := implicitProject(t, 5, 3)
	m := newTestModel(p, nil)
	m.termW, m.termH = 60, 12
	m = press(m, "1", "left", "y", "down", "p")
	if strings.Contains(m.status, "общий буфер") {
		t.Errorf("своя запись помечена чужой: %q", m.status)
	}
}

// Пикер разлепляет ряды пустой строкой — иначе блочные глифы сливаются в массу.
func TestPickerBlankRowBetweenGlyphRows(t *testing.T) {
	pal := []palette.Page{{Name: "P", Rows: [][]rune{
		{'█', '▀'}, {'▄', '▌'}, {'▐', '░'},
	}}}
	m := newTestModel(implicitProject(t, 5, 3), pal)
	m.termW, m.termH = 40, 20
	m = press(m, "tab")
	lines := strings.Split(m.viewPalette(), "\n")
	if len(lines) < 7 {
		t.Fatalf("вид короче ожидаемого:\n%s", m.viewPalette())
	}
	if !strings.Contains(lines[2], "█") || lines[3] != "" || !strings.Contains(lines[4], "▄") {
		t.Fatalf("ряды не разлеплены:\n%q", lines[:6])
	}
	// окно прокрутки считает ряд за две строки
	m.termH = 8
	m.ppRow = 2
	view := m.viewPalette()
	if strings.Count(view, "\n") > 8 {
		t.Fatalf("окно не ужалось под высоту:\n%s", view)
	}
	if !strings.Contains(view, "▐") {
		t.Fatalf("текущий ряд вне окна:\n%s", view)
	}
}

func TestCompositeOverstrikeLayer(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("⠁▌#\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("⠈▐E\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{
  "canvas": [3, 1],
  "layers": [
    {"file": "a.txt", "at": [1, 1], "fg": 240},
    {"file": "b.txt", "at": [1, 1], "fg": 231, "overstrike": true}
  ]
}`), 0o644)
	p, err := project.Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(p, nil)
	for c, want := range []rune{'⠉', '█', 'E'} {
		vc := m.compositeCell(0, c)
		if vc.g != want || vc.fg != 231 {
			t.Fatalf("ячейка %d: got %c fg %d, want %c fg 231", c, vc.g, vc.fg, want)
		}
	}
	m.solo = true
	if vc := m.compositeCell(0, 0); vc.g != '⠁' {
		t.Fatalf("solo нижнего слоя: got %c, want ⠁", vc.g)
	}
}
