# cool-notes

A tabbed note editor for the terminal. Every note lives in one plain-text file
that you can open in any other editor. Changes save as you type.

```
go build -o cool-notes .
./cool-notes                   # first run asks where to keep your notes
./cool-notes --file notes.txt  # open a specific file
```

## Keys

| Key | Action |
|---|---|
| `Ctrl+T` | New note (appended to the file) |
| `Ctrl+W` | Delete note, after confirmation |
| `Alt+←/→`, `Ctrl+PgUp/PgDn` | Previous / next note |
| `Alt+1…9` | Jump to note (9 = last) |
| `Tab` / `Shift+Tab` | Indent / dedent the line, or every selected line |
| `Shift+arrows`, `Ctrl+A` | Select, select all |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste (copy also sets the system clipboard via OSC 52) |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo |
| `Ctrl+←/→`, `Ctrl+Backspace` | Move / delete by word |
| `F2` / `Shift+F2` | Next / previous theme |
| `F1` | Help |
| `Ctrl+Q` | Quit |

A hotkey bar above the status line shows the main keys and adapts to the
terminal width (copy and cut appear while text is selected). Click a hint to
run it.

Mouse: click a tab to open it, `×` to delete it (middle-click works too), `+`
for a new note. In the text, click to place the cursor, drag to select,
double-click for a word, triple-click for a line, wheel to scroll. Wheel over the
tab bar switches notes.

## The notes file

```
=============== note: Shopping list ===============
Shopping list
	milk
	eggs

=============== note: Project ideas ===============
Project ideas
```

- A separator is a line of three or more `=`, a space, `note`, an optional
  `: title`, a space, and three or more `=`. Trailing whitespace and Windows
  line endings are fine.
- The title is cosmetic. cool-notes rewrites it from the note's first line on
  every save and ignores it when reading.
- A line that doesn't match exactly, such as a damaged separator, is ordinary
  text in the note above it. Text above the first separator becomes a note.
- To add a note by hand, type a separator line (`=== note ===` is enough).
- If a note contains a line that looks like a separator, cool-notes writes it
  with a leading `\` so it can't split the note, and removes it when reading.
- Writes are atomic (temp file, fsync, rename), so a crash never leaves a
  half-written file.

While cool-notes is open it watches the file. If another program changes it, the
notes reload. If you have edits that haven't been written yet, yours are kept
and the other version is saved next to the file as
`notes.txt.conflict-<time>.bak`.

## Settings and themes

Settings live in `~/.config/cool-notes/` (override with `COOL_NOTES_CONFIG_DIR`):

- `config.json`: `file` (notes path) and `theme`. Delete `file` to see the
  first-run prompt again.
- `state.json`: the open note and cursor positions, so you continue where you
  left off.

Built-in themes: `auto` (follows the terminal's light or dark background),
`catppuccin-mocha`, `catppuccin-latte`, `nord`, `gruvbox-dark`, `tokyo-night`,
`rose-pine-dawn`, and `terminal`, which uses your terminal's own 16 colors.

Add your own as `~/.config/cool-notes/themes/<name>.json`. Any field you leave
out comes from the theme named in `extends`:

```json
{
  "name": "my-theme",
  "extends": "nord",
  "accent": "#ff79c6",
  "selection": "#44475a"
}
```

Fields: `text`, `muted`, `bar`, `tab`, `tab_text`, `accent`, `accent_text`,
`selection`, `guide`, `surface`, `danger`, `ok`. Values are `#rrggbb`, an ANSI
color number (`"0"`–`"255"`), or `""` for the terminal default.

## Tests

```
go test ./...     # unit tests: file format, editor, app model
e2e/run.sh        # drives the real app in a virtual terminal with tui-goggles
```
