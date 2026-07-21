package palette

import (
	"os"
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
