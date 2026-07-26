package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/aleglutz/cliizdat/cellbuf"
	"github.com/aleglutz/cliizdat/project"
)

// `cliizdat clip [-fg N] [-raw] [file|-]` — разбирает ANSI (или голый текст)
// и кладёт в общий буфер обмена; в открытом редакторе вставляется по `p`:
//
//	tdfiglet -f incorped.tdf A | cliizdat clip -fg 246
//
// Так баннер из внешнего генератора попадает в проект без промежуточных файлов
// и без новых клавиш в TUI.
func clipCmd(args []string) error {
	fs := flag.NewFlagSet("clip", flag.ExitOnError)
	fg := fs.Int("fg", noRecolor, "перекрасить всю краску в цвет 0–255 (-1 = цвет терминала)")
	raw := fs.Bool("raw", false, "не обрезать пустые поля")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cliizdat clip [-fg N] [-raw] [file|-]")
		fs.PrintDefaults()
	}
	fs.Parse(args)
	if fs.NArg() > 1 {
		fs.Usage()
		os.Exit(2)
	}
	if *fg != noRecolor && (*fg < -1 || *fg > 255) {
		return fmt.Errorf("-fg %d: нужен 0–255 или -1", *fg)
	}

	src := "-"
	if fs.NArg() == 1 {
		src = fs.Arg(0)
	}
	var data []byte
	var err error
	if src == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(src)
	}
	if err != nil {
		return err
	}

	grid := project.ParseANSI(data)
	if !*raw {
		grid = project.CropInk(grid)
	}
	if len(grid) == 0 {
		return fmt.Errorf("на входе нет краски")
	}
	if *fg != noRecolor {
		recolor(grid, *fg)
	}
	id, err := writeClip(grid)
	if err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("буфер обмена недоступен: некуда писать")
	}
	fmt.Fprintf(os.Stderr, "cliizdat: clip %dx%d → нажми p\n", len(grid[0]), len(grid))
	return nil
}

// noRecolor — «-fg не задан»: сам -1 занят (цвет терминала по умолчанию).
const noRecolor = -2

func recolor(grid [][]cellbuf.Cell, fg int) {
	for _, row := range grid {
		for c := range row {
			if row[c].G != ' ' {
				row[c].Fg = fg
			}
		}
	}
}
