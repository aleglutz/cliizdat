package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Форма вывода tdfiglet: пустая первая строка, bg задан всегда, добивка
// пробелами до ширины экрана. Буква «A» шрифтом incorped, 5x3 после кропа.
const tdfBanner = "\n" +
	"\x1b[97;40m█▀▀▀█\x1b[0m\x1b[0m  \x1b[0m\n" +
	"\x1b[97;40m█▀▀▀█\x1b[0m\x1b[0m  \x1b[0m\n" +
	"\x1b[90;47m▄\x1b[37;40m   \x1b[90;47m▄\x1b[0m\x1b[0m  \x1b[0m\n"

func TestClipFromFileThenPaste(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLIIZDAT_CLIPBOARD", filepath.Join(dir, "clipboard.json"))
	src := filepath.Join(dir, "a.ans")
	if err := os.WriteFile(src, []byte(tdfBanner), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := clipCmd([]string{src}); err != nil {
		t.Fatal(err)
	}

	p := implicitProject(t, 20, 8)
	m := newTestModel(p, nil)
	m.termW, m.termH = 80, 20
	m = press(m, "down", "down", "right", "right", "p") // курсор 2,2 (0-based)

	buf := p.Layers[0].Buf
	if got := buf.Get(2, 2); got.G != '█' || got.Fg != 15 {
		t.Errorf("левый верхний угол: %q/%d, хотели █/15", got.G, got.Fg)
	}
	if got := buf.Get(4, 2); got.G != '▄' || got.Fg != 8 {
		t.Errorf("засечка: %q/%d, хотели ▄/8", got.G, got.Fg)
	}
	if got := buf.Get(4, 3); got.G != ' ' {
		t.Errorf("пробел шрифта должен остаться прозрачным: %q", got.G)
	}
	if buf.Get(1, 2).G != ' ' {
		t.Error("пустая первая строка tdfiglet не обрезана — баннер съехал вниз")
	}
	if !strings.Contains(m.status, "paste 5x3") {
		t.Errorf("статус: %q, хотели paste 5x3", m.status)
	}
	m = press(m, "u")
	if buf.Get(2, 2).G != ' ' {
		t.Error("вставка не откатилась одним undo")
	}
}

func TestClipFromStdinWithForcedFg(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLIIZDAT_CLIPBOARD", filepath.Join(dir, "clipboard.json"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		w.WriteString(tdfBanner)
		w.Close()
	}()
	saved := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = saved }()

	if err := clipCmd([]string{"-fg", "246"}); err != nil {
		t.Fatal(err)
	}
	clip, _, err := readClip()
	if err != nil {
		t.Fatal(err)
	}
	if len(clip) != 3 || len(clip[0]) != 5 {
		t.Fatalf("клип %dx%d, хотели 5x3", len(clip[0]), len(clip))
	}
	for r, row := range clip {
		for c, cell := range row {
			if cell.G == ' ' {
				if cell.Fg != -1 {
					t.Errorf("(%d,%d): у прозрачной ячейки цвет %d", r, c, cell.Fg)
				}
				continue
			}
			if cell.Fg != 246 {
				t.Errorf("(%d,%d): цвет %d, хотели 246", r, c, cell.Fg)
			}
		}
	}
}

func TestClipRawKeepsMargins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLIIZDAT_CLIPBOARD", filepath.Join(dir, "clipboard.json"))
	src := filepath.Join(dir, "a.ans")
	os.WriteFile(src, []byte(tdfBanner), 0o644)
	if err := clipCmd([]string{"-raw", src}); err != nil {
		t.Fatal(err)
	}
	clip, _, err := readClip()
	if err != nil {
		t.Fatal(err)
	}
	if len(clip) != 4 || len(clip[0]) != 5 {
		t.Fatalf("-raw клип %dx%d, хотели 5x4 (с пустой первой строкой)", len(clip[0]), len(clip))
	}
	if clip[0][0].G != ' ' {
		t.Error("-raw должен сохранить пустую первую строку")
	}
}

func TestClipRejectsEmptyAndBadFg(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLIIZDAT_CLIPBOARD", filepath.Join(dir, "clipboard.json"))
	src := filepath.Join(dir, "void.ans")
	os.WriteFile(src, []byte("\x1b[31m   \n\n"), 0o644)
	if err := clipCmd([]string{src}); err == nil {
		t.Error("хотели ошибку на вводе без краски")
	}
	if err := clipCmd([]string{"-fg", "300", src}); err == nil {
		t.Error("хотели ошибку на -fg 300")
	}
}
