// Package project — манифест коллажа, слои, сохранение, .ans-экспорт.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleglutz/cliizdat/cellbuf"
)

// DefaultSlots: индекс = клавиша-цифра, 0 = пробел/ластик.
var DefaultSlots = [10]rune{' ', '█', '▀', '▄', '\U0001CD00', '\U0001CD35', '\U0001CDAB', '🮕', '🮖', '·'}

type Layer struct {
	File     string // как в манифесте (относительный)
	AbsPath  string
	AtR, AtC int // 0-based внутренние
	Fg       int // -1 = терминальный default
	Knockout int
	Buf      *cellbuf.Buffer
	NewFile  bool
}

type Project struct {
	Path          string // путь манифеста; "" = implicit (открыт голый txt)
	Dir           string
	CanvasW       int
	CanvasH       int
	Layers        []*Layer
	Slots         [10]rune // индекс = клавиша-цифра
	PalettePath   string   // абсолютный; "" = нет
	ManifestDirty bool
}

type manifestLayer struct {
	File     string `json:"file"`
	At       [2]int `json:"at"`
	Fg       *int   `json:"fg"`
	Knockout int    `json:"knockout"`
}

type manifest struct {
	Canvas  [2]int          `json:"canvas"`
	Layers  []manifestLayer `json:"layers"`
	Slots   []string        `json:"slots"`
	Palette string          `json:"palette"`
}

// Load открывает project.json или одиночный layer.txt (implicit project).
func Load(path string) (*Project, error) {
	if strings.HasSuffix(path, ".json") {
		return loadManifest(path)
	}
	// если рядом лежит манифест, ссылающийся на этот слой (создан прошлым `w`),
	// открываем его — так холст/палитра/слоты переживают reopen голого txt.
	if mp := siblingManifest(path); mp != "" {
		return loadManifest(mp)
	}
	buf, err := cellbuf.Load(path)
	if err != nil {
		return nil, err
	}
	if err := buf.ApplySidecar(cellbuf.SidecarPath(path)); err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(path)
	return &Project{
		Dir:     filepath.Dir(abs),
		CanvasW: buf.W,
		CanvasH: buf.H,
		Layers:  []*Layer{{File: filepath.Base(path), AbsPath: abs, Fg: -1, Buf: buf}},
		Slots:   DefaultSlots,
	}, nil
}

// manifestPath — путь json-манифеста рядом с txt-слоем: <base>.json.
func manifestPath(txtPath string) string {
	return strings.TrimSuffix(txtPath, filepath.Ext(txtPath)) + ".json"
}

// siblingManifest возвращает путь <base>.json, если тот существует и ссылается
// на этот txt как на слой; иначе "".
func siblingManifest(txtPath string) string {
	mp := manifestPath(txtPath)
	data, err := os.ReadFile(mp)
	if err != nil {
		return ""
	}
	var m manifest
	if json.Unmarshal(data, &m) != nil {
		return ""
	}
	want := filepath.Base(txtPath)
	for _, L := range m.Layers {
		if filepath.Base(L.File) == want {
			return mp
		}
	}
	return ""
}

func loadManifest(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	abs, _ := filepath.Abs(path)
	dir := filepath.Dir(abs)
	p := &Project{
		Path:    abs,
		Dir:     dir,
		CanvasW: m.Canvas[0],
		CanvasH: m.Canvas[1],
		Slots:   DefaultSlots,
	}
	if p.CanvasW < 1 {
		p.CanvasW = 80
	}
	if p.CanvasH < 1 {
		p.CanvasH = 50
	}
	for i, s := range m.Slots {
		if i > 9 {
			break
		}
		runes := []rune(s)
		if len(runes) == 0 {
			continue
		}
		p.Slots[(i+1)%10] = runes[0] // манифест: слоты 1..9, потом 0
	}
	p.Slots[0] = ' ' // слот 0 — всегда ластик, что бы ни было в манифесте
	if m.Palette != "" {
		p.PalettePath = absJoin(dir, m.Palette)
	}
	for _, ml := range m.Layers {
		L := &Layer{
			File:     ml.File,
			AbsPath:  absJoin(dir, ml.File),
			AtR:      ml.At[0] - 1,
			AtC:      ml.At[1] - 1,
			Fg:       -1,
			Knockout: ml.Knockout,
		}
		if L.AtR < 0 {
			L.AtR = 0
		}
		if L.AtC < 0 {
			L.AtC = 0
		}
		if ml.Fg != nil {
			L.Fg = *ml.Fg
		}
		buf, err := cellbuf.Load(L.AbsPath)
		if os.IsNotExist(err) {
			buf = cellbuf.New(max(1, p.CanvasW-L.AtC), max(1, p.CanvasH-L.AtR))
			L.NewFile = true
		} else if err != nil {
			return nil, err
		}
		if !L.NewFile {
			if err := buf.ApplySidecar(cellbuf.SidecarPath(L.AbsPath)); err != nil {
				return nil, err
			}
		}
		buf.GrowTo(max(1, p.CanvasW-L.AtC), max(1, p.CanvasH-L.AtR))
		L.Buf = buf
		p.Layers = append(p.Layers, L)
	}
	if len(p.Layers) == 0 {
		return nil, fmt.Errorf("%s: манифест без слоёв", path)
	}
	return p, nil
}

func absJoin(dir, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(dir, rel)
}

func (p *Project) SetSlot(digit int, g rune) {
	if digit == 0 {
		return // слот 0 — всегда ластик, не переназначается
	}
	p.Slots[digit] = g
	p.ManifestDirty = true
}

func (p *Project) Dirty() bool {
	if p.ManifestDirty && p.Path != "" {
		return true
	}
	for _, L := range p.Layers {
		if L.Buf.Dirty() {
			return true
		}
	}
	return false
}

// Resize меняет размер канвы (кроп или расширение); каждый слой подгоняется
// под свой остаток канвы. История undo слоёв сбрасывается.
func (p *Project) Resize(w, h int) {
	p.CanvasW, p.CanvasH = w, h
	p.ManifestDirty = true
	for _, L := range p.Layers {
		L.Buf.Resize(max(1, w-L.AtC), max(1, h-L.AtR))
	}
}

// SaveAll пишет грязные слои + сайдкары и, при необходимости, манифест.
func (p *Project) SaveAll() error {
	for _, L := range p.Layers {
		if !L.Buf.Dirty() && !L.NewFile {
			continue
		}
		if err := L.Buf.SaveTxt(L.AbsPath); err != nil {
			return err
		}
		if err := L.Buf.WriteSidecar(cellbuf.SidecarPath(L.AbsPath)); err != nil {
			return err
		}
		L.NewFile = false
	}
	// промоушен implicit → manifest при первом сохранении: заводим json рядом
	// с первым слоем, чтобы холст/палитра/слоты персистились впредь.
	if p.Path == "" && len(p.Layers) > 0 {
		p.Path = manifestPath(p.Layers[0].AbsPath)
		p.ManifestDirty = true
	}
	if p.ManifestDirty && p.Path != "" {
		if err := p.SaveManifest(); err != nil {
			return err
		}
		p.ManifestDirty = false
	}
	return nil
}

// SaveManifest пишет project.json в формате примера из плана:
// слой — одна строка, массивы inline, git-diffable.
func (p *Project) SaveManifest() error {
	if p.Path == "" {
		return nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "{\n  \"canvas\": [%d, %d],\n  \"layers\": [\n", p.CanvasW, p.CanvasH)
	for i, L := range p.Layers {
		file, _ := json.Marshal(L.File)
		fmt.Fprintf(&sb, "    {\"file\": %s, \"at\": [%d, %d]", file, L.AtR+1, L.AtC+1)
		if L.Fg >= 0 {
			fmt.Fprintf(&sb, ", \"fg\": %d", L.Fg)
		}
		if L.Knockout > 0 {
			fmt.Fprintf(&sb, ", \"knockout\": %d", L.Knockout)
		}
		sb.WriteString("}")
		if i < len(p.Layers)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("  ],\n  \"slots\": [")
	for i := 0; i < 10; i++ {
		g, _ := json.Marshal(string(p.Slots[(i+1)%10]))
		sb.Write(g)
		if i < 9 {
			sb.WriteString(",")
		}
	}
	sb.WriteString("]")
	if p.PalettePath != "" {
		rel, err := filepath.Rel(p.Dir, p.PalettePath)
		if err != nil || strings.HasPrefix(rel, "..") {
			rel = p.PalettePath
		}
		pal, _ := json.Marshal(rel)
		fmt.Fprintf(&sb, ",\n  \"palette\": %s", pal)
	}
	sb.WriteString("\n}\n")
	return os.WriteFile(p.Path, []byte(sb.String()), 0o644)
}

// ANSPath — путь экспорта рядом с манифестом/слоем.
func (p *Project) ANSPath() string {
	src := p.Path
	if src == "" {
		src = p.Layers[0].AbsPath
	}
	ext := filepath.Ext(src)
	return strings.TrimSuffix(src, ext) + ".ans"
}

// ExportANS — точный порт scripts/compose_ans.py: канва растёт по мере
// штамповки, пробел = прозрачность, fg-runs, `\x1b[39m` для default,
// rstrip + `\x1b[0m` на строку, завершающий \n. Байт-в-байт с оракулом
// на входах без overrides.
func (p *Project) ExportANS() []byte {
	type cell struct {
		ch rune
		fg int // -1 = None
	}
	var canvas [][]cell
	put := func(y, x int, ch rune, fg int) {
		for y >= len(canvas) {
			canvas = append(canvas, nil)
		}
		for len(canvas[y]) <= x {
			canvas[y] = append(canvas[y], cell{' ', -1})
		}
		canvas[y][x] = cell{ch, fg}
	}
	type pos struct {
		y, x int
		ch   rune
		fg   int
	}
	for _, L := range p.Layers {
		var cells []pos
		for r := 0; r < L.Buf.H; r++ {
			for c := 0; c < L.Buf.W; c++ {
				cl := L.Buf.Get(r, c)
				if cl.G == ' ' {
					continue
				}
				fg := L.Fg
				if cl.Fg >= 0 {
					fg = cl.Fg
				}
				cells = append(cells, pos{L.AtR + r, L.AtC + c, cl.G, fg})
			}
		}
		if k := L.Knockout; k > 0 {
			for _, cl := range cells {
				for dy := -k; dy <= k; dy++ {
					for dx := -k; dx <= k; dx++ {
						if cl.y+dy >= 0 && cl.x+dx >= 0 {
							put(cl.y+dy, cl.x+dx, ' ', -1)
						}
					}
				}
			}
		}
		for _, cl := range cells {
			put(cl.y, cl.x, cl.ch, cl.fg)
		}
	}
	var out []string
	for _, row := range canvas {
		cur := -2
		var sb strings.Builder
		for _, cl := range row {
			if cl.fg != cur {
				if cl.fg < 0 {
					sb.WriteString("\x1b[39m")
				} else {
					fmt.Fprintf(&sb, "\x1b[38;5;%dm", cl.fg)
				}
				cur = cl.fg
			}
			sb.WriteRune(cl.ch)
		}
		out = append(out, strings.TrimRight(sb.String(), " ")+"\x1b[0m")
	}
	return []byte(strings.Join(out, "\n") + "\n")
}
