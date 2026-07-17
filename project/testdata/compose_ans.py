#!/usr/bin/env python3
"""
compose_ans.py — слоение текстовых слоёв сразу в ANSI, без .dur.
Каждый слой — plain txt; цвет задаётся слою целиком (256-палитра).
Пробел = прозрачность. --knockout N выбивает гало вокруг фигуры.

Usage:
  python3 compose_ans.py bg.txt:240 fg.txt@11,13:231 --knockout 1 > collage.ans
  спецификация слоя:  path[@ROW,COL][:FG]     ROW,COL 1-based (как vim ruler)
  z-order = порядок аргументов, последний сверху
"""
import sys

args = [a for a in sys.argv[1:]]
knock = 0
if '--knockout' in args:
    i = args.index('--knockout')
    knock = int(args[i + 1]); del args[i:i + 2]

def parse(spec):
    fg = None
    if ':' in spec.rsplit('/', 1)[-1]:          # двоеточие в имени, не в пути
        spec, fg = spec.rsplit(':', 1); fg = int(fg)
    r = c = 0
    if '@' in spec:
        spec, at = spec.rsplit('@', 1)
        r, c = (int(x) - 1 for x in at.split(','))
    grid = [list(line.rstrip('\n')) for line in open(spec, encoding='utf-8')]
    return grid, r, c, fg

# канва: (символ, цвет); растёт по мере штамповки
canvas = []
def put(y, x, ch, fg):
    while y >= len(canvas): canvas.append([])
    row = canvas[y]
    while len(row) <= x: row.append((' ', None))
    row[x] = (ch, fg)

for spec in args:
    grid, R, C, fg = parse(spec)
    cells = [(R + y, C + x, ch) for y, row in enumerate(grid)
                                for x, ch in enumerate(row) if ch != ' ']
    if knock:
        for y, x, _ in cells:
            for dy in range(-knock, knock + 1):
                for dx in range(-knock, knock + 1):
                    if y + dy >= 0 and x + dx >= 0:
                        put(y + dy, x + dx, ' ', None)
    for y, x, ch in cells:
        put(y, x, ch, fg)

out = []
for row in canvas:
    cur = object()          # заведомо не равен ни одному fg
    line = []
    for ch, fg in row:
        if fg != cur:
            line.append(f'\x1b[38;5;{fg}m' if fg is not None else '\x1b[39m')
            cur = fg
        line.append(ch)
    out.append(''.join(line).rstrip() + '\x1b[0m')
print('\n'.join(out))
