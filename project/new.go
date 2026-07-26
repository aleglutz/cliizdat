package project

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aleglutz/cliizdat/cellbuf"
)

// DefaultCanvasW, DefaultCanvasH — холст нового проекта.
const (
	DefaultCanvasW = 80
	DefaultCanvasH = 55
)

// New создаёт манифест нового проекта: <name>.json с холстом w×h, одним слоем
// <name>.txt и списком палитр. Сам слой на диск не пишется — его создаст первый
// `w` (как и для слоёв, отсутствующих у существующего манифеста).
// Возвращает путь манифеста. Существующие файлы не перезаписываются.
func New(name string, w, h int, palettes []string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("нужно имя проекта")
	}
	if w < 1 || h < 1 {
		return "", fmt.Errorf("холст %dx%d: размеры должны быть > 0", w, h)
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if ext := filepath.Ext(name); ext != "" && ext != ".json" && ext != ".txt" {
		base = name // «sigil.v2» — не расширение, а часть имени
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	manifest, layer := abs+".json", abs+".txt"
	for _, f := range []string{manifest, layer} {
		if _, err := os.Stat(f); err == nil {
			return "", fmt.Errorf("%s уже существует", f)
		}
	}
	p := &Project{
		Path:    manifest,
		Dir:     filepath.Dir(abs),
		CanvasW: w,
		CanvasH: h,
		Slots:   DefaultSlots,
		Layers: []*Layer{{
			File:    filepath.Base(layer),
			AbsPath: layer,
			Fg:      -1,
			Buf:     cellbuf.New(w, h),
			NewFile: true,
		}},
	}
	for _, pal := range palettes {
		a, err := filepath.Abs(pal)
		if err != nil {
			return "", err
		}
		p.PalettePaths = append(p.PalettePaths, a)
	}
	if err := p.SaveManifest(); err != nil {
		return "", err
	}
	return manifest, nil
}

// DiscoverPalettes ищет доступные палитры: $CLIIZDAT_PALETTES (список файлов
// и/или каталогов через ':'), затем каталог `palettes/` вверх по дереву от dir,
// затем ~/.config/cliizdat/palettes. Возвращает абсолютные пути *.txt.
func DiscoverPalettes(dir string) []string {
	if env := os.Getenv("CLIIZDAT_PALETTES"); env != "" {
		var out []string
		for _, part := range filepath.SplitList(env) {
			out = append(out, paletteFiles(part)...)
		}
		return out
	}
	d, err := filepath.Abs(dir)
	if err != nil {
		return nil
	}
	for {
		if fs := paletteFiles(filepath.Join(d, "palettes")); len(fs) > 0 {
			return fs
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	if home, err := os.UserHomeDir(); err == nil {
		return paletteFiles(filepath.Join(home, ".config", "cliizdat", "palettes"))
	}
	return nil
}

// AllPalette — имя собранной палитры: если она лежит в каталоге, берём только
// её, иначе страницы задвоятся своими же источниками (см. scripts/palettegen.py).
const AllPalette = "all.txt"

// paletteFiles: файл → он сам, каталог → all.txt, если есть, иначе все *.txt
// по алфавиту.
func paletteFiles(path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil
	}
	if !st.IsDir() {
		return []string{abs}
	}
	if all := filepath.Join(abs, AllPalette); fileExists(all) {
		return []string{all}
	}
	files, _ := filepath.Glob(filepath.Join(abs, "*.txt"))
	sort.Strings(files)
	return files
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
