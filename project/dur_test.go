package project

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// Кадр 2×3: contents по рядам, colorMap [col][row] (транспонирован).
//   ряд 0: "AB"  A=durdraw fg 4 (→ANSI 1), B=208
//   ряд 1: "C "  C=208
//   ряд 2: "𜴀D"  𜴀=208, D=208  → base 208, override только у A
const durFixture = `{
  "DurMovie": {
    "formatVersion": 7, "colorFormat": "256",
    "sizeX": 2, "sizeY": 3,
    "frames": [{
      "frameNumber": 1, "delay": 0,
      "contents": ["AB", "C ", "\ud833\udd00D"],
      "colorMap": [
        [[4, 0], [208, 0], [208, 0]],
        [[208, 0], [7, 0], [208, 0]]
      ]
    }]
  }
}`

func writeDur(t *testing.T, dir string, gzipped bool) string {
	t.Helper()
	path := filepath.Join(dir, "sig.dur")
	data := []byte(durFixture)
	if gzipped {
		var b bytes.Buffer
		zw := gzip.NewWriter(&b)
		zw.Write(data)
		zw.Close()
		data = b.Bytes()
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportDur(t *testing.T) {
	for _, gz := range []bool{false, true} {
		dir := t.TempDir()
		p, err := ImportDur(writeDur(t, dir, gz))
		if err != nil {
			t.Fatalf("gzip=%v: %v", gz, err)
		}
		L := p.Layers[0]
		if L.Fg != 208 {
			t.Errorf("base fg = %d, want 208", L.Fg)
		}
		if got := L.Buf.Get(0, 0); got.G != 'A' || got.Fg != 1 {
			t.Errorf("A: %+v, want fg 1 (durdraw 4 → ANSI red)", got)
		}
		if got := L.Buf.Get(0, 1); got.G != 'B' || got.Fg != -1 {
			t.Errorf("B: %+v, want base (-1)", got)
		}
		if got := L.Buf.Get(1, 1); got.G != ' ' {
			t.Errorf("(1,1): %+v, want transparent space", got)
		}
		if got := L.Buf.Get(2, 0); got.G != '\U0001CD00' || got.Fg != -1 {
			t.Errorf("octant: %+v", got)
		}
		if p.Path != filepath.Join(dir, "sig.json") || L.AbsPath != filepath.Join(dir, "sig.txt") {
			t.Errorf("paths: %s, %s", p.Path, L.AbsPath)
		}
		if !p.Dirty() {
			t.Error("imported project must be dirty")
		}
	}
}

func TestImportDurSaveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p, err := ImportDur(writeDur(t, dir, true))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SaveAll(); err != nil {
		t.Fatal(err)
	}
	txt, _ := os.ReadFile(filepath.Join(dir, "sig.txt"))
	if want := "AB\nC\n\U0001CD00D\n"; string(txt) != want {
		t.Errorf("txt = %q, want %q", txt, want)
	}
	color, _ := os.ReadFile(filepath.Join(dir, "sig.color"))
	if want := "1 1 1\n"; string(color) != want {
		t.Errorf("sidecar = %q, want %q", color, want)
	}
	p2, err := Load(filepath.Join(dir, "sig.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Layers[0].Fg != 208 {
		t.Errorf("reloaded base fg = %d", p2.Layers[0].Fg)
	}
	if got := p2.Layers[0].Buf.Get(0, 0); got.G != 'A' || got.Fg != 1 {
		t.Errorf("reloaded A: %+v", got)
	}
}

func TestImportDurRefusesExistingTxt(t *testing.T) {
	dir := t.TempDir()
	path := writeDur(t, dir, false)
	os.WriteFile(filepath.Join(dir, "sig.txt"), []byte("x\n"), 0o644)
	if _, err := ImportDur(path); err == nil {
		t.Fatal("want error when sig.txt exists")
	}
}
