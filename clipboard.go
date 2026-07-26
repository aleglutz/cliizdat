package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aleglutz/cliizdat/cellbuf"
)

// Буфер обмена живёт файлом, а не в памяти процесса: `y`/`d` пишут его, `p`
// перечитывает, если файл изменился с нашей последней записи. Так вырезанное
// в одной вкладке терминала вставляется в другой — и переживает выход.
// Путь: $CLIIZDAT_CLIPBOARD, иначе $XDG_CACHE_HOME/cliizdat/clipboard.json,
// иначе ~/.cache/cliizdat/clipboard.json.

type clipFile struct {
	ID   string   `json:"id"` // pid+наносекунды: отличает чужую запись от своей
	W    int      `json:"w"`
	H    int      `json:"h"`
	Rows []string `json:"rows"`
	Fg   [][]int  `json:"fg"` // -1 = цвет терминала по умолчанию
}

func clipPath() string {
	if p := os.Getenv("CLIIZDAT_CLIPBOARD"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "cliizdat", "clipboard.json")
}

// writeClip сохраняет буфер атомарно (temp + rename в том же каталоге).
func writeClip(clip [][]cellbuf.Cell) (string, error) {
	path := clipPath()
	if path == "" || len(clip) == 0 {
		return "", nil
	}
	cf := clipFile{
		ID: fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()),
		W:  len(clip[0]), H: len(clip),
	}
	for _, row := range clip {
		var runes []rune
		fg := make([]int, len(row))
		for i, cell := range row {
			runes = append(runes, cell.G)
			fg[i] = cell.Fg
		}
		cf.Rows = append(cf.Rows, string(runes))
		cf.Fg = append(cf.Fg, fg)
	}
	data, err := json.Marshal(cf)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".clipboard-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return cf.ID, nil
}

// readClip читает разделяемый буфер. Строки короче w дополняются пробелами,
// длиннее — режутся; отсутствующий fg = -1.
func readClip() ([][]cellbuf.Cell, string, error) {
	path := clipPath()
	if path == "" {
		return nil, "", os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var cf clipFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, "", fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if cf.W < 1 || len(cf.Rows) == 0 {
		return nil, "", fmt.Errorf("%s: пустой буфер", filepath.Base(path))
	}
	clip := make([][]cellbuf.Cell, len(cf.Rows))
	for r, s := range cf.Rows {
		runes := []rune(s)
		row := make([]cellbuf.Cell, cf.W)
		for c := range row {
			cell := cellbuf.Blank
			if c < len(runes) {
				cell.G = runes[c]
			}
			if r < len(cf.Fg) && c < len(cf.Fg[r]) {
				cell.Fg = cf.Fg[r][c]
			}
			row[c] = cell
		}
		clip[r] = row
	}
	return clip, cf.ID, nil
}
