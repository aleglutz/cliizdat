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
	pal, err := palette.Load(p.PalettePath)
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
