# cliizdat

> A cli-native ANSI and ASCII text editor inspired by ACiDDraw, its Unix
> successor [DurDraw](https://github.com/durdraw/durdraw), and the rich tradition of concrete poetry in the Eastern
> Bloc samizdat.

`cliizdat` is a keyboard-only, cell-grid glyph editor for Unicode block art —
octants (U+1CD00–1CDE5), Symbols for Legacy Computing, and Nerd Font glyphs —
the material that ncurses editors silently drop because `wcwidth()` returns −1.

Editing is **replace-only**: there is no insert mode, so columns never drift and
the grid physically cannot break. Keys just do things — no modes to toggle.

## Features

- Modeless, keyboard-only editing on a fixed cell grid
- Per-cell 256-color, stored as sparse git-diffable `.color` sidecars
- Layers with a JSON manifest, knockout halos, solo view
- Rectangular selection: fill, repaint, copy / cut / paste, flip X / Y
- Braille dot-mode: draw at 2×4 sub-cell resolution, one dot per keystroke
- Loadable glyph palettes (octants auto-sorted by pixel density)
- `.ans` export (fg-only, terminal background is the paper)
- `.dur` import — bring existing durdraw art in, colors and all
- Width authority is [go-runewidth](https://github.com/mattn/go-runewidth),
  never libc `wcwidth`

## Build

Requires Go 1.25+.

```sh
go build -o ~/bin/cliizdat .
```

## Usage

```sh
cliizdat project.json          # a manifest project
cliizdat layer.txt             # a single layer (implicit project)
cliizdat art.dur               # import durdraw art
cliizdat -p palette.txt file   # attach a glyph palette
```

Press `h` in the editor for the full control list.

| key | action |
|-----|--------|
| arrows | move cursor |
| `Shift`+arrows / `s` | select (`s`: anchor, arrows grow, `Esc` drops) |
| `1`–`9` | stamp slot (fills the selection) |
| `0` / `x` | erase (slot 0 is always the eraser) |
| `y` `d` `p` | copy · cut · paste |
| `X` / `Y` | flip selection/layer horizontally · vertically (mirrors glyphs) |
| `b` | braille dot-mode: arrows move per dot, `.` toggles the dot |
| `c` / `C` / `r` | color picker · eyedropper · repaint |
| `Tab` | glyph palette → slot |
| `[` `]` / `v` | prev/next layer · solo |
| `u` / `U` | undo · redo |
| `R` | resize canvas (crop / extend) |
| `g` | go to row,col |
| `w` / `e` | save · export `.ans` |
| `q` | quit |

## File model

```
project.json      collage manifest (the single source of assembly)
layers/
  sigils.txt      layer = plain UTF-8 text, LF, space = transparent
  sigils.color    sparse per-cell color overrides: ROW COL FG, 1-based
```

Coordinates are 1-based everywhere in the UI, manifest, and sidecars. Saving
strips trailing spaces, enforces LF, and sorts sidecars by row,col.

Opening a bare `.txt` is an *implicit project*. On the first save it is
promoted to a manifest: a sibling `<name>.json` is written capturing the
canvas size, palette, and slots — so a resized canvas (whose empty margins
a bare `.txt` cannot record) persists. Reopening the same `.txt` afterwards
picks the manifest up automatically.
