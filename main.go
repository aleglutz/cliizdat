package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aleglutz/cliizdat/palette"
	"github.com/aleglutz/cliizdat/project"
)

func main() {
	palFlag := flag.String("p", "", "palette file (для implicit-проекта из layer.txt)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cliizdat [-p palette.txt] <project.json | layer.txt | file.dur>")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	arg := flag.Arg(0)
	var proj *project.Project
	var err error
	if strings.HasSuffix(arg, ".dur") {
		proj, err = project.ImportDur(arg)
	} else {
		proj, err = project.Load(arg)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cliizdat:", err)
		os.Exit(1)
	}
	if *palFlag != "" {
		abs, err := filepath.Abs(*palFlag)
		if err == nil && abs != proj.PalettePath {
			proj.PalettePath = abs
			proj.ManifestDirty = true // попадёт в манифест при следующем `w`
		}
	}
	palPath := proj.PalettePath
	var pal []palette.Page
	if palPath != "" {
		pal, err = palette.Load(palPath)
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
