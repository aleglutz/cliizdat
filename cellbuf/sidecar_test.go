package cellbuf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSidecarRoundtrip(t *testing.T) {
	dir := t.TempDir()
	side := filepath.Join(dir, "l.color")

	b := New(10, 5)
	b.Apply([]Change{
		{Row: 2, Col: 7, After: Cell{G: '█', Fg: 208}},
		{Row: 0, Col: 3, After: Cell{G: '·', Fg: 231}},
		{Row: 2, Col: 1, After: Cell{G: '▀', Fg: -1}}, // база — не в сайдкар
	})
	if err := b.WriteSidecar(side); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(side)
	want := "1 4 231\n3 8 208\n" // сортировка row,col; 1-based
	if string(data) != want {
		t.Fatalf("sidecar = %q; want %q", data, want)
	}

	b2 := New(10, 5)
	b2.Apply([]Change{
		{Row: 2, Col: 7, After: Cell{G: '█', Fg: -1}},
		{Row: 0, Col: 3, After: Cell{G: '·', Fg: -1}},
	})
	if err := b2.ApplySidecar(side); err != nil {
		t.Fatal(err)
	}
	if fg := b2.Get(2, 7).Fg; fg != 208 {
		t.Fatalf("override lost: fg %d", fg)
	}
}

func TestSidecarEmptyRemovesFile(t *testing.T) {
	dir := t.TempDir()
	side := filepath.Join(dir, "l.color")
	os.WriteFile(side, []byte("1 1 100\n"), 0o644)
	b := New(3, 3)
	if err := b.WriteSidecar(side); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(side); !os.IsNotExist(err) {
		t.Fatal("empty sidecar file not removed")
	}
}

func TestSidecarPath(t *testing.T) {
	if p := SidecarPath("layers/sigils.txt"); p != "layers/sigils.color" {
		t.Fatalf("SidecarPath = %q", p)
	}
}
