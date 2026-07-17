package project

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aleglutz/cliizdat/cellbuf"
)

const bgLayer = "" +
	"𜴀𜴁𜴂𜴃𜴄𜴅𜴆𜴇\n" +
	"𜴈𜴉𜴊𜴋𜴌𜴍𜴎𜴏\n" +
	"𜴐𜴑𜴒𜴓𜴔𜴕𜴖𜴗\n" +
	"𜴘𜴙𜴚𜴛𜴜𜴝𜴞𜴟\n" +
	"𜴠𜴡𜴢𜴣𜴤𜴥𜴦𜴧\n"

const fgLayer = "" +
	" ▀▀ \n" +
	"█🮕🮖█\n" +
	" ▄▄ \n"

func writeProject(t *testing.T, knockBoth bool) (dir string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "layers"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "layers/bg.txt"), []byte(bgLayer), 0o644)
	os.WriteFile(filepath.Join(dir, "layers/fg.txt"), []byte(fgLayer), 0o644)
	kb := ""
	if knockBoth {
		kb = `, "knockout": 1`
	}
	manifest := `{
  "canvas": [40, 20],
  "layers": [
    {"file": "layers/bg.txt", "at": [1, 1], "fg": 240` + kb + `},
    {"file": "layers/fg.txt", "at": [2, 3], "fg": 231` + kb + `}
  ],
  "slots": ["█","▀","▄","𜴀","𜴵","𜶫","🮕","🮖","·"," "]
}`
	os.WriteFile(filepath.Join(dir, "project.json"), []byte(manifest), 0o644)
	return dir
}

func TestLoadManifest(t *testing.T) {
	dir := writeProject(t, false)
	p, err := Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.CanvasW != 40 || p.CanvasH != 20 {
		t.Fatalf("canvas %dx%d", p.CanvasW, p.CanvasH)
	}
	if len(p.Layers) != 2 {
		t.Fatalf("layers = %d", len(p.Layers))
	}
	L := p.Layers[1]
	if L.AtR != 1 || L.AtC != 2 || L.Fg != 231 {
		t.Fatalf("layer2 at=(%d,%d) fg=%d", L.AtR, L.AtC, L.Fg)
	}
	if p.Slots[1] != '█' || p.Slots[4] != '𜴀' || p.Slots[0] != ' ' {
		t.Fatalf("slots parsed wrong: %q %q %q", p.Slots[1], p.Slots[4], p.Slots[0])
	}
}

func TestManifestRoundtrip(t *testing.T) {
	dir := writeProject(t, false)
	p, err := Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	p.SetSlot(4, '\U0001FB00')
	if err := p.SaveManifest(); err != nil {
		t.Fatal(err)
	}
	p2, err := Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p2.Slots[4] != '\U0001FB00' {
		t.Fatalf("slot did not survive manifest roundtrip: %U", p2.Slots[4])
	}
	if p2.Layers[1].AtR != 1 || p2.Layers[1].AtC != 2 || p2.Layers[1].Fg != 231 {
		t.Fatal("layer spec drifted through manifest roundtrip")
	}
}

// Golden: экспорт байт-в-байт с compose_ans.py (оракул в testdata).
func TestExportMatchesComposeOracle(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}
	for _, knock := range []bool{false, true} {
		dir := writeProject(t, knock)
		p, err := Load(filepath.Join(dir, "project.json"))
		if err != nil {
			t.Fatal(err)
		}
		got := p.ExportANS()

		oracle, _ := filepath.Abs("testdata/compose_ans.py")
		args := []string{oracle, "layers/bg.txt:240", "layers/fg.txt@2,3:231"}
		if knock {
			args = append(args, "--knockout", "1")
		}
		cmd := exec.Command(python, args...)
		cmd.Dir = dir
		want, err := cmd.Output()
		if err != nil {
			t.Fatalf("oracle failed: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("knockout=%v: export differs from compose_ans.py\ngot:  %q\nwant: %q",
				knock, got, want)
		}
	}
}

// Overrides: экспорт с посотовым цветом — отдельный golden (самопорождённый смысл-тест).
func TestExportWithOverride(t *testing.T) {
	dir := writeProject(t, false)
	p, err := Load(filepath.Join(dir, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	// override посреди bg-слоя
	buf := p.Layers[0].Buf
	buf.Apply([]cellbuf.Change{{Row: 1, Col: 2, After: cellbuf.Cell{G: buf.Get(1, 2).G, Fg: 208}}})
	out := string(p.ExportANS())
	if !bytes.Contains([]byte(out), []byte("\x1b[38;5;208m")) {
		t.Fatal("override color missing from export")
	}
	if !bytes.Contains([]byte(out), []byte("\x1b[38;5;240m")) {
		t.Fatal("base color missing from export")
	}
}

func TestLoadGrowsLayerToCanvas(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "l.txt"), []byte("ab\ncd\n"), 0o644)
	manifest := `{"canvas": [82, 45], "layers": [{"file": "l.txt", "at": [1, 1], "fg": 7}]}`
	os.WriteFile(filepath.Join(dir, "p.json"), []byte(manifest), 0o644)
	p, err := Load(filepath.Join(dir, "p.json"))
	if err != nil {
		t.Fatal(err)
	}
	buf := p.Layers[0].Buf
	if buf.W != 82 || buf.H != 45 {
		t.Fatalf("layer %dx%d; want 82x45 (canvas)", buf.W, buf.H)
	}
	if buf.Dirty() {
		t.Fatal("GrowTo at load must not mark buffer dirty")
	}
	if buf.Get(0, 0).G != 'a' || buf.Get(1, 1).G != 'd' {
		t.Fatal("content lost while growing")
	}
}

func TestManifestSlotZeroForcedToSpace(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "l.txt"), []byte("a\n"), 0o644)
	// 10-й слот (→ слот 0) в манифесте = '█'; должен быть перекрыт пробелом
	manifest := `{"canvas":[4,2],"layers":[{"file":"l.txt","at":[1,1],"fg":7}],
	  "slots":["1","2","3","4","5","6","7","8","9","█"]}`
	os.WriteFile(filepath.Join(dir, "p.json"), []byte(manifest), 0o644)
	p, err := Load(filepath.Join(dir, "p.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Slots[0] != ' ' {
		t.Fatalf("manifest slot 0 = %U; want space", p.Slots[0])
	}
}
