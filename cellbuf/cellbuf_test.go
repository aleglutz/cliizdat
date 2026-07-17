package cellbuf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOctantsAndRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.txt")
	// строка 2 короче и с trailing-пробелами; октанты как \U-escape
	content := "\U0001CD00\U0001CD35█\n▀▄   \n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if b.W != 5 || b.H != 2 {
		t.Fatalf("W,H = %d,%d; want 5,2", b.W, b.H)
	}
	if g := b.Get(0, 0).G; g != '\U0001CD00' {
		t.Fatalf("Get(0,0) = %U", g)
	}
	if g := b.Get(1, 4).G; g != ' ' {
		t.Fatalf("padding not blank: %U", g)
	}

	out := filepath.Join(dir, "out.txt")
	if err := b.SaveTxt(out); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	want := "\U0001CD00\U0001CD35█\n▀▄\n"
	if string(got) != want {
		t.Fatalf("save = %q; want %q", got, want)
	}
}

func TestApplyUndoRedoDirty(t *testing.T) {
	b := New(4, 2)
	if b.Dirty() {
		t.Fatal("fresh buffer dirty")
	}
	b.Apply([]Change{{Row: 0, Col: 1, After: Cell{G: '█', Fg: -1}}})
	if !b.Dirty() {
		t.Fatal("not dirty after Apply")
	}
	if g := b.Get(0, 1).G; g != '█' {
		t.Fatalf("after Apply: %U", g)
	}
	if !b.Undo() {
		t.Fatal("Undo failed")
	}
	if g := b.Get(0, 1).G; g != ' ' {
		t.Fatalf("after Undo: %U", g)
	}
	if b.Dirty() {
		t.Fatal("dirty after full undo")
	}
	if !b.Redo() {
		t.Fatal("Redo failed")
	}
	if g := b.Get(0, 1).G; g != '█' {
		t.Fatalf("after Redo: %U", g)
	}
	if b.Undo() && b.Undo() {
		t.Fatal("Undo past history start")
	}
}

func TestSaveClearsDirtyAndUndoMakesDirtyAgain(t *testing.T) {
	dir := t.TempDir()
	b := New(3, 1)
	b.Apply([]Change{{Row: 0, Col: 0, After: Cell{G: '·', Fg: -1}}})
	if err := b.SaveTxt(filepath.Join(dir, "s.txt")); err != nil {
		t.Fatal(err)
	}
	if b.Dirty() {
		t.Fatal("dirty right after save")
	}
	b.Undo()
	if !b.Dirty() {
		t.Fatal("undo below saved point must be dirty")
	}
}

func TestGroupUndoIsAtomic(t *testing.T) {
	b := New(3, 1)
	b.Apply([]Change{
		{Row: 0, Col: 0, After: Cell{G: 'a', Fg: -1}},
		{Row: 0, Col: 1, After: Cell{G: 'b', Fg: -1}},
	})
	b.Undo()
	if b.Get(0, 0).G != ' ' || b.Get(0, 1).G != ' ' {
		t.Fatal("group undo not atomic")
	}
}
