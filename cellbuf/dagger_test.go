package cellbuf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Приёмка Фазы 1: реальный октантный материал открывается и сохраняется
// без дрейфа строк — меняются только trailing-пробелы (спека Save).
func TestDaggerRoundtrip(t *testing.T) {
	b, err := Load("testdata/dagger.txt")
	if err != nil {
		t.Fatal(err)
	}
	if b.H != 40 {
		t.Fatalf("H = %d; want 40", b.H)
	}

	out := filepath.Join(t.TempDir(), "out.txt")
	if err := b.SaveTxt(out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	orig, err := os.ReadFile("testdata/dagger.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(orig), "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	want := strings.Join(lines, "\n") + "\n"
	if string(got) != want {
		t.Fatal("roundtrip drifted beyond trailing-space strip")
	}

	// все октанты дошли до буфера
	found := false
	for r := 0; r < b.H && !found; r++ {
		for c := 0; c < b.W; c++ {
			if g := b.Get(r, c).G; g >= 0x1CD00 && g <= 0x1CDE5 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("no octants survived Load")
	}
}
