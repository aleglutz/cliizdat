// Package palette читает файлы-спесимены: строки `HEX  glyph glyph …`,
// секции `== NAME ==` становятся страницами пикера.
package palette

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

type Page struct {
	Name string
	Rows [][]rune
}

var (
	sectionRe = regexp.MustCompile(`^==\s*(.*?)\s*==$`)
	hexRe     = regexp.MustCompile(`^[0-9A-Fa-f]{2,7}$`)
)

func Load(path string) ([]Page, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pages []Page
	cur := -1
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" {
			continue
		}
		if m := sectionRe.FindStringSubmatch(line); m != nil {
			pages = append(pages, Page{Name: m[1]})
			cur = len(pages) - 1
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 1 && hexRe.MatchString(fields[0]) {
			fields = fields[1:]
		}
		var row []rune
		for _, tok := range fields {
			row = append(row, []rune(tok)...)
		}
		if len(row) == 0 {
			continue
		}
		if cur < 0 {
			pages = append(pages, Page{Name: "PALETTE"})
			cur = 0
		}
		pages[cur].Rows = append(pages[cur].Rows, row)
	}
	// секции без глифов не показываем
	out := pages[:0]
	for _, p := range pages {
		if len(p.Rows) > 0 {
			out = append(out, p)
		}
	}
	return collapseOctants(out), nil
}

// collapseOctants: если палитра содержит октанты, все её глифы семейства 2×4
// (октанты + блок-эквиваленты) сводятся в одну страницу, отсортированную по
// плотности (меньше пикселей → больше). Прозаический мусор из файла (подписи
// «доли», «+ пробел» и т.п.) отбрасывается автоматически. Палитры без октантов
// (NF, specimen) возвращаются как есть.
func collapseOctants(pages []Page) []Page {
	hasOctant := false
	for _, p := range pages {
		for _, row := range p.Rows {
			for _, g := range row {
				if g >= 0x1CD00 && g <= 0x1CDE5 {
					hasOctant = true
				}
			}
		}
	}
	if !hasOctant {
		return pages
	}
	seen := map[rune]bool{}
	var glyphs []rune
	for _, p := range pages {
		for _, row := range p.Rows {
			for _, g := range row {
				if _, ok := pixelCount(g); ok && !seen[g] {
					seen[g] = true
					glyphs = append(glyphs, g)
				}
			}
		}
	}
	sort.Slice(glyphs, func(i, j int) bool {
		pi, _ := pixelCount(glyphs[i])
		pj, _ := pixelCount(glyphs[j])
		if pi != pj {
			return pi < pj
		}
		return glyphs[i] < glyphs[j]
	})
	const perRow = 16
	var rows [][]rune
	for i := 0; i < len(glyphs); i += perRow {
		rows = append(rows, glyphs[i:min(i+perRow, len(glyphs))])
	}
	return []Page{{Name: "OCTANTS", Rows: rows}}
}
