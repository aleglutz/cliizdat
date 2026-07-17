package cellbuf

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// SidecarPath — путь .color-сайдкара рядом с txt-слоем.
func SidecarPath(txtPath string) string {
	if strings.HasSuffix(txtPath, ".txt") {
		return strings.TrimSuffix(txtPath, ".txt") + ".color"
	}
	return txtPath + ".color"
}

// ApplySidecar накладывает overrides из .color на буфер (без записи в историю —
// это часть загрузки). Отсутствие файла — не ошибка.
func (b *Buffer) ApplySidecar(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var row, col, fg int
		if _, err := fmt.Sscan(sc.Text(), &row, &col, &fg); err != nil {
			continue
		}
		r, c := row-1, col-1
		if r >= 0 && r < b.H && c >= 0 && c < b.W {
			b.cells[r][c].Fg = fg
		}
	}
	return sc.Err()
}

// WriteSidecar пишет overrides: `ROW COL FG`, 1-based, сортировка row,col.
// Пустые overrides — файл удаляется.
func (b *Buffer) WriteSidecar(path string) error {
	var sb strings.Builder
	for r := 0; r < b.H; r++ {
		for c := 0; c < b.W; c++ {
			cell := b.cells[r][c]
			if cell.Fg >= 0 && cell.G != ' ' {
				fmt.Fprintf(&sb, "%d %d %d\n", r+1, c+1, cell.Fg)
			}
		}
	}
	if sb.Len() == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
