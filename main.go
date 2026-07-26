package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aleglutz/cliizdat/palette"
	"github.com/aleglutz/cliizdat/project"
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: cliizdat [-p palette.txt] <project.json | layer.txt | file.dur | file.ans>")
	fmt.Fprintln(os.Stderr, "       cliizdat new [-s WxH] [-p pal.txt,...] <name>")
	fmt.Fprintln(os.Stderr, "       cliizdat clip [-fg N] [-raw] [file|-]")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "new" {
		path, err := createNew(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "cliizdat:", err)
			os.Exit(1)
		}
		open(path, "")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "clip" {
		if err := clipCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "cliizdat:", err)
			os.Exit(1)
		}
		return
	}
	palFlag := flag.String("p", "", "palette file (для implicit-проекта из layer.txt)")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		os.Exit(2)
	}
	open(flag.Arg(0), *palFlag)
}

// createNew — подкоманда `new`: манифест WxH (по умолчанию 80×55) со всеми
// найденными палитрами. Возвращает путь созданного .json.
func createNew(args []string) (string, error) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	size := fs.String("s", fmt.Sprintf("%dx%d", project.DefaultCanvasW, project.DefaultCanvasH), "холст WxH")
	pals := fs.String("p", "", "палитры через запятую (файлы или каталоги); по умолчанию — все найденные")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cliizdat new [-s WxH] [-p pal.txt,...] <name>")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	w, h, err := parseSize(*size)
	if err != nil {
		return "", err
	}
	name := fs.Arg(0)
	var list []string
	if *pals != "" {
		for _, part := range strings.Split(*pals, ",") {
			if part = strings.TrimSpace(part); part != "" {
				list = append(list, project.DiscoverPalettes(part)...)
			}
		}
	} else {
		dir := filepath.Dir(name)
		if dir == "." {
			dir, _ = os.Getwd()
		}
		list = project.DiscoverPalettes(dir)
	}
	path, err := project.New(name, w, h, list)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "cliizdat: %s — холст %dx%d, палитр: %d\n", filepath.Base(path), w, h, len(list))
	return path, nil
}

func parseSize(s string) (int, int, error) {
	a, b, ok := strings.Cut(strings.ToLower(s), "x")
	if !ok {
		return 0, 0, fmt.Errorf("размер %q: нужен формат WxH", s)
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(a))
	h, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || w < 1 || h < 1 {
		return 0, 0, fmt.Errorf("размер %q: нужен формат WxH", s)
	}
	return w, h, nil
}

func open(arg, palOverride string) {
	var proj *project.Project
	var err error
	switch {
	case strings.HasSuffix(arg, ".dur"):
		proj, err = project.ImportDur(arg)
	case strings.HasSuffix(arg, ".ans"):
		proj, err = project.ImportANS(arg)
	default:
		proj, err = project.Load(arg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cliizdat:", err)
		os.Exit(1)
	}
	if palOverride != "" {
		var list []string
		for _, part := range strings.Split(palOverride, ",") {
			if part = strings.TrimSpace(part); part != "" {
				abs, err := filepath.Abs(part)
				if err == nil {
					list = append(list, abs)
				}
			}
		}
		if len(list) > 0 && !sameList(list, proj.PalettePaths) {
			proj.PalettePaths = list
			proj.ManifestDirty = true // попадёт в манифест при следующем `w`
		}
	}
	if len(proj.PalettePaths) == 0 {
		// в манифесте нет "palette" (или открыт голый .txt) — берём найденную
		// палитру на эту сессию; в манифест она попадёт при следующем `w`
		proj.PalettePaths = project.DiscoverPalettes(proj.Dir)
	}
	var pal []palette.Page
	if len(proj.PalettePaths) > 0 {
		pal, err = palette.LoadAll(proj.PalettePaths)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cliizdat: палитра:", err)
		}
	}
	p := tea.NewProgram(newModel(proj, pal), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "cliizdat:", err)
		os.Exit(1)
	}
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
