package project

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/aleglutz/cliizdat/cellbuf"
)

// Фикстуры инлайном: арт в репозиторий не кладём.

// tdfigletFixture — форма вывода tdfiglet: bg всегда задан (40 = чёрный),
// fg из ярких 90–97, `\x1b[0m\x1b[0m` в конце каждой строки и пустая
// первая строка.
const tdfigletFixture = "\n" +
	"\x1b[94;40m█▀\x1b[96;40m▄\x1b[0m\x1b[0m  \x1b[0m\n" +
	"\x1b[90;40m▄\x1b[97;47m▀\x1b[97;40m█\x1b[0m\x1b[0m  \x1b[0m\n"

func at(t *testing.T, grid [][]cellbuf.Cell, r, c int) cellbuf.Cell {
	t.Helper()
	if r >= len(grid) || c >= len(grid[r]) {
		t.Fatalf("ячейки (%d,%d) нет: сетка %dx%d", r, c, len(grid[0]), len(grid))
	}
	return grid[r][c]
}

func want(t *testing.T, got cellbuf.Cell, g rune, fg int, what string) {
	t.Helper()
	if got.G != g || got.Fg != fg {
		t.Errorf("%s: %q/%d, хотели %q/%d", what, got.G, got.Fg, g, fg)
	}
}

func TestParseANSITdfiglet(t *testing.T) {
	grid := ParseANSI([]byte(tdfigletFixture))
	if len(grid) != 3 || len(grid[0]) != 3 {
		t.Fatalf("сетка %dx%d, хотели 3x3 (пустая первая строка сохраняется)", len(grid[0]), len(grid))
	}
	for c := 0; c < 3; c++ {
		want(t, at(t, grid, 0, c), ' ', -1, "первая строка пуста")
	}
	want(t, at(t, grid, 1, 0), '█', 12, "94 → 12")
	want(t, at(t, grid, 1, 1), '▀', 12, "цвет держится до смены")
	want(t, at(t, grid, 1, 2), '▄', 14, "96 → 14")
	want(t, at(t, grid, 2, 0), '▄', 8, "90 → 8")
	want(t, at(t, grid, 2, 1), '▀', 15, "полублок на белом фоне: fg остаётся, bg теряется")
	want(t, at(t, grid, 2, 2), '█', 15, "97 → 15")
}

func TestParseANSIBackgroundRules(t *testing.T) {
	// пробел на цветном фоне → полный блок цветом фона (единственное место,
	// где фон переживает импорт точно); '█' поверх фона фон не показывает
	grid := ParseANSI([]byte("\x1b[31;44m \x1b[0m\x1b[33;41m█\x1b[0m\x1b[32;40m \x1b[0m\x1b[35;104m \x1b[0m\n"))
	want(t, at(t, grid, 0, 0), '█', 4, "пробел на синем фоне")
	want(t, at(t, grid, 0, 1), '█', 3, "полный блок: fg, не bg")
	want(t, at(t, grid, 0, 2), ' ', -1, "пробел на чёрном фоне прозрачен")
	want(t, at(t, grid, 0, 3), '█', 12, "104 → bg 12")
}

func TestParseANSIBoldBright(t *testing.T) {
	grid := ParseANSI([]byte("\x1b[1;31mX\x1b[22mY\x1b[1;38;5;200mZ\n"))
	want(t, at(t, grid, 0, 0), 'X', 9, "bold+31 → 9")
	want(t, at(t, grid, 0, 1), 'Y', 1, "22 гасит bold")
	want(t, at(t, grid, 0, 2), 'Z', 200, "bold не сдвигает 256-цвет")
}

func TestParseANSICursorForwardAndCR(t *testing.T) {
	grid := ParseANSI([]byte("A\x1b[3CB\rC\x1b[?25lD\n"))
	want(t, at(t, grid, 0, 0), 'C', -1, "CR вернул в начало, C затёр A")
	want(t, at(t, grid, 0, 1), 'D', -1, "неизвестный CSI проглочен целиком")
	want(t, at(t, grid, 0, 4), 'B', -1, "ESC[3C = три колонки вперёд")
}

func TestParseANSICP437(t *testing.T) {
	// не-UTF-8 байты: 0xDB=█, 0xDF=▀, 0xB0=░
	grid := ParseANSI([]byte{0x1b, '[', '3', '1', 'm', 0xDB, 0xDF, 0xB0, '\n'})
	want(t, at(t, grid, 0, 0), '█', 1, "0xDB")
	want(t, at(t, grid, 0, 1), '▀', 1, "0xDF")
	want(t, at(t, grid, 0, 2), '░', 1, "0xB0")
}

func TestParseANSIStripsSAUCE(t *testing.T) {
	body := []byte("\x1b[31mX\n")
	sauce := make([]byte, 128)
	copy(sauce, "SAUCE00")
	raw := append(append(body, 0x1a), sauce...)
	grid := ParseANSI(raw)
	if len(grid) != 1 || len(grid[0]) != 1 {
		t.Fatalf("сетка %dx%d, хотели 1x1", len(grid[0]), len(grid))
	}
	want(t, at(t, grid, 0, 0), 'X', 1, "SAUCE срезан")
}

func TestCropInk(t *testing.T) {
	grid := ParseANSI([]byte(tdfigletFixture))
	cut := CropInk(grid)
	if len(cut) != 2 || len(cut[0]) != 3 {
		t.Fatalf("после кропа %dx%d, хотели 3x2", len(cut[0]), len(cut))
	}
	want(t, at(t, cut, 0, 0), '█', 12, "левый верхний угол краски")
	if CropInk(ParseANSI([]byte("\x1b[31m   \n"))) != nil {
		t.Error("пустой вход должен давать nil")
	}
}

// Круг замыкается: наш собственный экспорт читается обратно ячейка в ячейку.
func TestExportANSParseRoundTrip(t *testing.T) {
	dir := writeProject(t, false)
	p, err := Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	grid := ParseANSI(p.ExportANS())
	for r := 0; r < len(grid); r++ {
		for c := 0; c < len(grid[r]); c++ {
			var eff cellbuf.Cell
			for _, L := range p.Layers { // верхний слой поверх нижнего, пробел прозрачен
				cl := L.Buf.Get(r-L.AtR, c-L.AtC)
				if cl.G == ' ' {
					continue
				}
				fg := L.Fg
				if cl.Fg >= 0 {
					fg = cl.Fg
				}
				eff = cellbuf.Cell{G: cl.G, Fg: fg}
			}
			if eff.G == 0 {
				eff = cellbuf.Blank
			}
			if got := grid[r][c]; got != eff {
				t.Fatalf("(%d,%d): распарсили %q/%d, в проекте %q/%d", r, c, got.G, got.Fg, eff.G, eff.Fg)
			}
		}
	}
}

func TestImportANS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "banner.ans")
	// 231 трижды → база; 240 один раз → override
	body := "\x1b[38;5;231m██\x1b[38;5;240m▄\x1b[0m\n\x1b[38;5;231m█\x1b[0m\n"
	os.WriteFile(path, []byte(body), 0o644)

	p, err := ImportANS(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.CanvasW != 3 || p.CanvasH != 2 {
		t.Errorf("канва %dx%d, хотели 3x2", p.CanvasW, p.CanvasH)
	}
	L := p.Layers[0]
	if L.Fg != 231 {
		t.Errorf("базовый fg = %d, хотели 231", L.Fg)
	}
	if got := L.Buf.Get(0, 2); got.G != '▄' || got.Fg != 240 {
		t.Errorf("override: %+v", got)
	}
	if got := L.Buf.Get(0, 0); got.Fg != -1 {
		t.Errorf("базовый цвет должен стать -1: %+v", got)
	}
	if p.Path != filepath.Join(dir, "banner.json") || L.AbsPath != filepath.Join(dir, "banner.txt") {
		t.Errorf("пути: %s, %s", p.Path, L.AbsPath)
	}
	if !p.Dirty() {
		t.Error("импортированный проект обязан быть dirty")
	}

	if err := p.SaveAll(); err != nil {
		t.Fatal(err)
	}
	txt, _ := os.ReadFile(filepath.Join(dir, "banner.txt"))
	if w := "██▄\n█\n"; string(txt) != w {
		t.Errorf("txt = %q, хотели %q", txt, w)
	}
	color, _ := os.ReadFile(filepath.Join(dir, "banner.color"))
	if w := "1 3 240\n"; string(color) != w {
		t.Errorf("сайдкар = %q, хотели %q", color, w)
	}
}

func TestImportANSRefusesExistingTxt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "banner.ans")
	os.WriteFile(path, []byte("\x1b[31mX\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "banner.txt"), []byte("x\n"), 0o644)
	if _, err := ImportANS(path); err == nil {
		t.Fatal("хотели ошибку: banner.txt уже существует")
	}
}

func TestImportANSEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "void.ans")
	os.WriteFile(path, bytes.Repeat([]byte(" "), 8), 0o644)
	if _, err := ImportANS(path); err == nil {
		t.Fatal("хотели ошибку на пустом вводе")
	}
}
