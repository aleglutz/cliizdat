package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProjectManifest(t *testing.T) {
	dir := t.TempDir()
	pals := []string{filepath.Join(dir, "palettes", "a.txt"), filepath.Join(dir, "palettes", "b.txt")}
	os.MkdirAll(filepath.Join(dir, "palettes"), 0o755)
	for _, p := range pals {
		os.WriteFile(p, []byte("== X ==\n▀ ▄\n"), 0o644)
	}
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)

	path, err := New("sigil", DefaultCanvasW, DefaultCanvasH, pals)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "sigil.json" {
		t.Fatalf("манифест %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{`"canvas": [80, 55]`, `"file": "sigil.txt"`, `"palettes/a.txt"`, `"palettes/b.txt"`} {
		if !strings.Contains(got, want) {
			t.Errorf("нет %s в манифесте:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sigil.txt")); !os.IsNotExist(err) {
		t.Error("слой не должен писаться до первого `w`")
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.CanvasW != 80 || p.CanvasH != 55 {
		t.Fatalf("холст %dx%d", p.CanvasW, p.CanvasH)
	}
	if len(p.PalettePaths) != 2 {
		t.Fatalf("палитр %d, ждали 2", len(p.PalettePaths))
	}
	if len(p.Layers) != 1 || p.Layers[0].Buf.W != 80 || p.Layers[0].Buf.H != 55 {
		t.Fatalf("слой не по холсту: %+v", p.Layers)
	}
	if !p.Layers[0].NewFile {
		t.Error("слой должен быть помечен NewFile")
	}
	// первый `w` материализует слой
	if err := p.SaveAll(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sigil.txt")); err != nil {
		t.Errorf("SaveAll не создал слой: %v", err)
	}
}

func TestNewRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)
	if _, err := New("dup", 10, 10, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := New("dup", 10, 10, nil); err == nil {
		t.Fatal("повторный new должен падать, а не перезаписывать")
	}
	if _, err := New("dup.json", 10, 10, nil); err == nil {
		t.Fatal("new dup.json должен падать по тому же манифесту")
	}
}

func TestPaletteFieldBothForms(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "l.txt"), []byte("ab\n"), 0o644)
	write := func(pal string) *Project {
		mp := filepath.Join(dir, "p.json")
		os.WriteFile(mp, []byte(`{"canvas":[4,2],"layers":[{"file":"l.txt","at":[1,1]}],"palette":`+pal+`}`), 0o644)
		p, err := Load(mp)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := write(`"one.txt"`); len(p.PalettePaths) != 1 || filepath.Base(p.PalettePaths[0]) != "one.txt" {
		t.Errorf("строка: %v", p.PalettePaths)
	}
	if p := write(`["one.txt","two.txt"]`); len(p.PalettePaths) != 2 {
		t.Errorf("массив: %v", p.PalettePaths)
	}
	// одна палитра сохраняется строкой (старый формат), несколько — массивом
	p := write(`["one.txt","two.txt"]`)
	p.ManifestDirty = true
	if err := p.SaveManifest(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p.Path)
	if !strings.Contains(string(data), "\"palette\": [\n") {
		t.Errorf("массив не сохранился:\n%s", data)
	}
	p.PalettePaths = p.PalettePaths[:1]
	if err := p.SaveManifest(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(p.Path)
	if !strings.Contains(string(data), `"palette": "one.txt"`) {
		t.Errorf("одиночная палитра должна остаться строкой:\n%s", data)
	}
}

func TestDiscoverPalettes(t *testing.T) {
	dir := t.TempDir()
	deep := filepath.Join(dir, "a", "b")
	os.MkdirAll(deep, 0o755)
	os.MkdirAll(filepath.Join(dir, "palettes"), 0o755)
	for _, n := range []string{"z.txt", "a.txt", "skip.png"} {
		os.WriteFile(filepath.Join(dir, "palettes", n), []byte("x\n"), 0o644)
	}
	got := DiscoverPalettes(deep)
	if len(got) != 2 || filepath.Base(got[0]) != "a.txt" || filepath.Base(got[1]) != "z.txt" {
		t.Fatalf("поиск вверх по дереву: %v", got)
	}
	t.Setenv("CLIIZDAT_PALETTES", filepath.Join(dir, "palettes", "z.txt"))
	got = DiscoverPalettes(deep)
	if len(got) != 1 || filepath.Base(got[0]) != "z.txt" {
		t.Fatalf("CLIIZDAT_PALETTES: %v", got)
	}
}

// Собранная палитра выигрывает у своих же источников — иначе страницы задвоятся.
func TestDiscoverPrefersAllPalette(t *testing.T) {
	dir := t.TempDir()
	pals := filepath.Join(dir, "palettes")
	os.MkdirAll(pals, 0o755)
	for _, n := range []string{"braille.txt", "octants.txt"} {
		os.WriteFile(filepath.Join(pals, n), []byte("x\n"), 0o644)
	}
	if got := DiscoverPalettes(dir); len(got) != 2 {
		t.Fatalf("без all.txt: %v", got)
	}
	os.WriteFile(filepath.Join(pals, AllPalette), []byte("x\n"), 0o644)
	got := DiscoverPalettes(dir)
	if len(got) != 1 || filepath.Base(got[0]) != AllPalette {
		t.Fatalf("с all.txt: %v", got)
	}
}
