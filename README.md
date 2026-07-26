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
- Loadable glyph palettes, one page per section (octants auto-sorted by density)
- `.ans` export (fg-only, terminal background is the paper)
- Theme-adaptive grayscale: on a light terminal the gray ramp (256-color
  232–255) is reflected around its midpoint, so faint two-tone art keeps its
  contrast when the paper flips from black to white — one file, both themes
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
cliizdat new sigil             # new 80x55 project with every palette found
cliizdat new -s 120x40 sigil   # custom canvas
cliizdat new -p pal.txt sigil  # explicit palettes (files or directories, comma-separated)
cliizdat project.json          # a manifest project
cliizdat layer.txt             # a single layer (implicit project)
cliizdat art.dur               # import durdraw art
cliizdat -p palette.txt file   # attach a glyph palette
```

`new` writes `<name>.json` (canvas, slots, palettes) and opens it; the layer
`<name>.txt` lands on disk with the first `w`.

Palettes are discovered in `$CLIIZDAT_PALETTES` (`:`-separated files or
directories), else in the nearest `palettes/` directory up the tree, else
`~/.config/cliizdat/palettes`. A directory holding `all.txt` yields just that
file — the convention for a palette assembled from the others, whose sources
would otherwise double every page. A project whose manifest has no `"palette"`
falls back to the same search, so the `Tab` picker is never empty; the choice
is recorded in the manifest on the next `w`.

Every `== SECTION ==` is a page in the picker, shown one glyph row per two
screen lines so block glyphs do not fuse into a mass. Sections named `OCTANT…`
are merged into a single page sorted by pixel density, and non-2×4 glyphs are
dropped from them — that is how the prose in a hand-written octant table
disappears. The rule keys off the name alone, so octants deliberately mixed
with other glyphs on a shared page stay untouched; a single file can carry
octants, braille, blocks, and NF side by side.

Press `h` in the editor for the full control list.

| key | action |
|-----|--------|
| arrows | move cursor |
| `Shift`+arrows / `s` | select (`s`: anchor, arrows grow, `Esc` drops) |
| `1`–`9` | stamp slot (fills the selection) |
| `0` / `x` | erase (slot 0 is always the eraser) |
| `y` `d` `p` | copy · cut · paste (shared between running instances) |
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

The copy buffer is a file, not process memory: `y`/`d` write it, `p` re-reads
it when another instance wrote last. So an element yanked in one terminal tab
pastes into a project open in another, glyphs and colors intact, and survives
quitting. Location: `$CLIIZDAT_CLIPBOARD`, else
`${XDG_CACHE_HOME:-~/.cache}/cliizdat/clipboard.json`.

Opening a bare `.txt` is an *implicit project*. On the first save it is
promoted to a manifest: a sibling `<name>.json` is written capturing the
canvas size, palette, and slots — so a resized canvas (whose empty margins
a bare `.txt` cannot record) persists. Reopening the same `.txt` afterwards
picks the manifest up automatically.
