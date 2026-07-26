// Package palette читает файлы-спесимены: строки `HEX  glyph glyph …`,
// секции `== NAME ==` становятся страницами пикера.
package palette

import (
	"os"
	"path/filepath"
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

// LoadAll грузит несколько файлов и склеивает страницы в один пикер.
// Каждый файл разбирается отдельно — collapseOctants схлопывает только свой
// файл, поэтому октантная палитра не съедает брайль и NF из соседних.
// Ошибки чтения отдельных файлов не фатальны: возвращается всё, что читается,
// плюс первая ошибка. Одинаковые имена страниц разводятся суффиксом-файлом.
func LoadAll(paths []string) ([]Page, error) {
	var out []Page
	var firstErr error
	seen := map[string]bool{}
	for _, p := range paths {
		pages, err := Load(p)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		for _, pg := range pages {
			if seen[pg.Name] {
				pg.Name += " · " + stem
			}
			seen[pg.Name] = true
			out = append(out, pg)
		}
	}
	return out, firstErr
}

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

// collapseOctants сводит страницы, названные `== OCTANT… ==`, в одну,
// отсортированную по плотности (меньше пикселей → больше), и выбрасывает с них
// всё, что не из семейства 2×4, — так уходит прозаический мусор рукописной
// таблицы («кв-ты», «доли», «+ пробел») и склеиваются основная секция с
// блок-эквивалентами (▘▚█ …), достраивающими сетку. Признак — только имя:
// октанты, разложенные автором вместе с другими глифами на общей странице,
// трогать нельзя. Остальные страницы остаются как есть, на своих местах.
func collapseOctants(pages []Page) []Page {
	octant := make([]bool, len(pages))
	first := -1
	for i, p := range pages {
		octant[i] = strings.HasPrefix(strings.ToUpper(p.Name), "OCTANT")
		if octant[i] && first < 0 {
			first = i
		}
	}
	if first < 0 {
		return pages
	}
	seen := map[rune]bool{}
	var glyphs []rune
	for i, p := range pages {
		if !octant[i] {
			continue
		}
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
	var out []Page
	for i, p := range pages {
		switch {
		case i == first:
			out = append(out, Page{Name: "OCTANTS", Rows: rows})
		case octant[i]:
		default:
			out = append(out, p)
		}
	}
	return out
}
