package palette

import (
	"os"
	"path/filepath"
	"testing"
)

// requireTestdata пропускает тест, если приватная палитра недоступна
// (файлы не публикуются — см. .gitignore). Локально даёт полное покрытие.
func requireTestdata(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("private palette %s absent (gitignored)", path)
	}
}

func countGlyphs(pages []Page) int {
	n := 0
	for _, p := range pages {
		for _, r := range p.Rows {
			n += len(r)
		}
	}
	return n
}

func TestLoadOctants(t *testing.T) {
	requireTestdata(t, "testdata/octants.txt")
	pages, err := Load("testdata/octants.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no pages")
	}
	if n := countGlyphs(pages); n < 200 {
		t.Fatalf("octants glyphs = %d; want ≥200", n)
	}
	found := false
	for _, p := range pages {
		for _, row := range p.Rows {
			for _, g := range row {
				if g == '\U0001CD00' {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("U+1CD00 not loaded")
	}
}

func TestLoadSpecimenSections(t *testing.T) {
	requireTestdata(t, "testdata/specimen_v2.txt")
	pages, err := Load("testdata/specimen_v2.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 9 {
		t.Fatalf("specimen pages = %d; want 9", len(pages))
	}
	if pages[0].Name != "BLOCK ELEMENTS" {
		t.Fatalf("page 0 = %q", pages[0].Name)
	}
	// строка `000B7  ·· ∙ …` — слипшийся токен должен дать отдельные глифы
	var dots []rune
	for _, p := range pages {
		if p.Name == "DOTS / STIPPLE" {
			dots = p.Rows[0]
		}
	}
	if len(dots) < 3 || dots[0] != '·' || dots[1] != '·' {
		t.Fatalf("DOTS row parsed wrong: %q", string(dots))
	}
}

func TestLoadNF(t *testing.T) {
	requireTestdata(t, "testdata/palette_nf.txt")
	pages, err := Load("testdata/palette_nf.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 10 {
		t.Fatalf("nf pages = %d; want 10", len(pages))
	}
	for _, p := range pages {
		for _, row := range p.Rows {
			for _, g := range row {
				if g >= 0xE000 && g <= 0xF8FF {
					return // PUA дошёл
				}
			}
		}
	}
	t.Fatal("no PUA glyphs loaded from NF palette")
}

func TestCollapseOctantsSinglePageSorted(t *testing.T) {
	requireTestdata(t, "testdata/octants.txt")
	pages, err := Load("testdata/octants.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 {
		t.Fatalf("octants palette pages = %d; want 1", len(pages))
	}
	if pages[0].Name != "OCTANTS" {
		t.Fatalf("page name = %q", pages[0].Name)
	}
	// прозаический мусор («доли», «+ пробел») не должен просочиться в глифы
	var first, last rune
	var n, prev int
	for _, row := range pages[0].Rows {
		for _, g := range row {
			pc, ok := pixelCount(g)
			if !ok {
				t.Fatalf("non-2x4 glyph leaked: %U", g)
			}
			if pc < prev {
				t.Fatalf("not sorted by density at %U: %d after %d", g, pc, prev)
			}
			prev = pc
			if n == 0 {
				first = g
			}
			last = g
			n++
		}
	}
	if p, _ := pixelCount(first); p != 1 {
		t.Fatalf("first glyph density = %d; want 1 (fewest pixels)", p)
	}
	if p, _ := pixelCount(last); p != 8 {
		t.Fatalf("last glyph density = %d; want 8 (█ full)", p)
	}
	if n < 230 {
		t.Fatalf("only %d glyphs; expected ≥230 octants", n)
	}
}

// LoadAll: октантный файл схлопывается сам в себя и не съедает страницы соседей.
func TestLoadAllKeepsForeignPages(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "octants.txt"), []byte("== OCTANTS ==\n1CD00  \U0001CD00 \U0001CD01\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "braille.txt"), []byte("== BRAILLE ==\n⠁⠂⠄\n\n== ANCIENT ==\n༒ ༓\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "dup.txt"), []byte("== BRAILLE ==\n⣿\n"), 0o644)

	pages, err := LoadAll([]string{
		filepath.Join(dir, "octants.txt"),
		filepath.Join(dir, "braille.txt"),
		filepath.Join(dir, "dup.txt"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range pages {
		names = append(names, p.Name)
	}
	want := []string{"OCTANTS", "BRAILLE", "ANCIENT", "BRAILLE · dup"}
	if len(names) != len(want) {
		t.Fatalf("страницы %v, ждали %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("страницы %v, ждали %v", names, want)
		}
	}
	if _, err := LoadAll([]string{filepath.Join(dir, "нет.txt"), filepath.Join(dir, "braille.txt")}); err == nil {
		t.Error("отсутствующий файл должен вернуть ошибку")
	}
	pages, _ = LoadAll([]string{filepath.Join(dir, "нет.txt"), filepath.Join(dir, "braille.txt")})
	if len(pages) != 2 {
		t.Errorf("читаемые файлы должны грузиться несмотря на ошибку: %d", len(pages))
	}
}

// Один файл несёт и октанты, и всё прочее: октантные страницы схлопываются
// между собой, соседние остаются на местах.
func TestCollapseKeepsSiblingPages(t *testing.T) {
	dir := t.TempDir()
	src := "== USED ==\n░ ◉\n\n== BRAILLE ==\n⠁⠂⠄\n\n== OCTANTS ==\n\U0001CD00 \U0001CD01\nмусор\n\n" +
		"== OCTANT ДОПОЛНЕНИЕ ==\nкв-ты ▘ ▚ █\n\n== BLOCK ELEMENTS ==\n▀ ▁ ▂ ▃ ░ ▒ ▓\n"
	path := filepath.Join(dir, "all.txt")
	os.WriteFile(path, []byte(src), 0o644)

	pages, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range pages {
		names = append(names, p.Name)
	}
	want := []string{"USED", "BRAILLE", "OCTANTS", "BLOCK ELEMENTS"}
	if len(names) != len(want) {
		t.Fatalf("страницы %v, ждали %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("страницы %v, ждали %v", names, want)
		}
	}
	var oct []rune
	for _, row := range pages[2].Rows {
		oct = append(oct, row...)
	}
	if len(oct) != 5 { // 2 октанта + ▘ ▚ █, мусор и «кв-ты» отброшены
		t.Fatalf("OCTANTS = %q", string(oct))
	}
	for _, g := range oct {
		if _, ok := pixelCount(g); !ok {
			t.Fatalf("в OCTANTS просочилось %U", g)
		}
	}
	// блочная страница не съедена, шейды на месте
	var blocks []rune
	for _, row := range pages[3].Rows {
		blocks = append(blocks, row...)
	}
	if string(blocks) != "▀▁▂▃░▒▓" {
		t.Fatalf("BLOCK ELEMENTS = %q", string(blocks))
	}
}
