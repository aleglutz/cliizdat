package project

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleglutz/cliizdat/cellbuf"
)

// durFg16 — таблица durdraw color_256_to_ansi_16: внутренние цвета 1–16
// (256-color режим) → ANSI-256 индексы. Остальные значения — сырые ANSI-256.
var durFg16 = [17]int{0, 4, 2, 6, 1, 5, 3, 7, 8, 12, 10, 14, 9, 13, 11, 15, 16}

type durFrame struct {
	Contents []string  `json:"contents"`
	ColorMap [][][]int `json:"colorMap"` // [col][row] → [fg, bg]
}

type durMovie struct {
	FormatVersion int             `json:"formatVersion"`
	ColorFormat   json.RawMessage `json:"colorFormat"`
	SizeX         int             `json:"sizeX"`
	SizeY         int             `json:"sizeY"`
	Frames        []durFrame      `json:"frames"`
}

// ImportDur читает durdraw .dur (gzip или plain JSON, ключ DurMovie) и строит
// implicit-проект: слой = contents кадра 0, базовый fg = самый частый цвет,
// отличия — как overrides. Ничего не пишет на диск до SaveAll: манифест
// <имя>.json, слой <имя>.txt, сайдкар <имя>.color.
func ImportDur(path string) (*Project, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(raw, []byte{0x1f, 0x8b}) {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: gzip: %w", path, err)
		}
		raw, err = io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("%s: gzip: %w", path, err)
		}
	}
	var file struct {
		DurMovie durMovie `json:"DurMovie"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m := file.DurMovie
	if cf := strings.Trim(string(m.ColorFormat), `"`); cf != "256" {
		return nil, fmt.Errorf("%s: colorFormat %q — поддерживается только 256", path, cf)
	}
	if len(m.Frames) == 0 {
		return nil, fmt.Errorf("%s: нет кадров", path)
	}
	fr := m.Frames[0]

	h := len(fr.Contents)
	w := 1
	rows := make([][]rune, h)
	for r, line := range fr.Contents {
		rows[r] = []rune(line)
		if len(rows[r]) > w {
			w = len(rows[r])
		}
	}
	buf := cellbuf.New(w, h)

	// colorMap транспонирован: [col][row]. fg 1–16 — палитра durdraw.
	fgAt := func(r, c int) int {
		if c >= len(fr.ColorMap) || r >= len(fr.ColorMap[c]) || len(fr.ColorMap[c][r]) == 0 {
			return 7
		}
		fg := fr.ColorMap[c][r][0]
		if fg >= 1 && fg <= 16 {
			return durFg16[fg]
		}
		return fg
	}

	freq := map[int]int{}
	var group []cellbuf.Change
	for r := 0; r < h; r++ {
		for c, g := range rows[r] {
			if g == ' ' {
				continue
			}
			fg := fgAt(r, c)
			freq[fg]++
			group = append(group, cellbuf.Change{Row: r, Col: c, After: cellbuf.Cell{G: g, Fg: fg}})
		}
	}
	if len(group) == 0 {
		return nil, fmt.Errorf("%s: кадр пуст", path)
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
