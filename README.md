# cool-notes

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Built with Claude Code](https://img.shields.io/badge/Built_with-Claude_Code-D97757?logo=claude&logoColor=white)](https://claude.com/claude-code)

A tabbed note editor for the terminal. All your notes are stored together in a
single plain-text file, one tab per note, separated by marker lines. You can
open that file in any other editor. Changes save as you type.

![cool-notes demo: notes in tabs, find and replace, moving and restoring a note, then quitting and reopening where you left off](docs/demo.gif)

## Install

Prebuilt binaries are on the
[releases page](https://github.com/thinkbig1979/cool-notes/releases) for Linux,
macOS and Windows, on both amd64 (Intel/AMD) and arm64 (Apple Silicon, ARM).
The easiest way to install is through a package manager, which also handles
updates.

### Homebrew (macOS, Linux)

```sh
brew install thinkbig1979/tap/cool-notes
```

Update with `brew upgrade cool-notes`.

### Scoop (Windows)

```powershell
scoop bucket add thinkbig1979 https://github.com/thinkbig1979/scoop-bucket
scoop install cool-notes
```

Update with `scoop update cool-notes`.

### Linux and macOS, without Homebrew

This downloads the latest release for your machine:

```sh
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m); case $arch in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
mkdir -p ~/.local/bin
curl -fsSL "https://github.com/thinkbig1979/cool-notes/releases/latest/download/cool-notes_${os}_${arch}.tar.gz" \
  | tar -xz -C ~/.local/bin cool-notes
```

This installs to `~/.local/bin`, which most Linux distributions already have on
`PATH`. On macOS it usually isn't, so add it once:

```sh
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

If you download the archive in a browser instead, macOS blocks the unsigned
binary on first run. Clear the flag with
`xattr -d com.apple.quarantine ~/.local/bin/cool-notes`.

### Windows, without Scoop

In PowerShell:

```powershell
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$dir = "$env:LOCALAPPDATA\Programs\cool-notes"
New-Item -ItemType Directory -Force $dir | Out-Null
Invoke-WebRequest "https://github.com/thinkbig1979/cool-notes/releases/latest/download/cool-notes_windows_$arch.zip" -OutFile "$env:TEMP\cool-notes.zip"
Expand-Archive "$env:TEMP\cool-notes.zip" $dir -Force
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$dir*") { [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User') }
```

Open a new terminal afterwards so the updated `PATH` applies. Use Windows
Terminal: the older console host doesn't handle all of the key combinations.

### From source

With Go 1.25 or later, on any platform:

```
go install github.com/thinkbig1979/cool-notes@latest
```

## Run

```
cool-notes                   # first run asks where to keep your notes
cool-notes --file notes.txt  # open a specific file
cool-notes --version
```

## Keys

| Key | Action |
|---|---|
| `Ctrl+T` | New note (appended to the file) |
| `Ctrl+W` | Delete note, after confirmation. `Ctrl+Z` straight after brings it back |
| `Alt+←/→`, `Ctrl+PgUp/PgDn` | Previous / next tab |
| `Alt+1…8`, `Alt+9` | Go to tab 1–8, or the last tab |
| `Alt+Shift+←/→`, `Ctrl+Shift+PgUp/PgDn` | Move the tab left / right (changes the note's place in the file) |
| `Ctrl+F` | Find in all notes. `Enter` / `Shift+Enter` next / previous match, `Esc` closes |
| `Ctrl+R` | Replace, in the current note or all notes (`Alt+N`). `Enter` replaces and moves on, `↓` skips, `Alt+A` replaces all |
| `Tab` / `Shift+Tab` | Indent / outdent the line, or every selected line |
| `Shift+arrows`, `Ctrl+A` | Select, select all |
| `Ctrl+C` / `Ctrl+X` / `Ctrl+V` | Copy / cut / paste (copy also sets the system clipboard via OSC 52) |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo. Straight after a replace all, `Ctrl+Z` undoes it in every note |
| `Ctrl+←/→`, `Ctrl+Backspace` | Move / delete by word |
| `F2` / `Shift+F2` | Next / previous theme |
| `F1` | Help |
| `Ctrl+Q` | Quit |

A hotkey bar above the status line shows the main keys and adapts to the
terminal width (copy and cut appear while text is selected). Click a hint to
run it.

Find ignores case and searches every note. Select a word before `Ctrl+F` to
search for it. `↑` / `↓` also step through matches, which helps in terminals
that send `Shift+Enter` as plain `Enter`.

Replace starts in the current note. `Tab` switches between the find and replace
fields, and `Alt+N` widens the search to all notes. Matching ignores case, and
the replacement goes in exactly as you typed it. Replacing across notes asks
first.

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

Settings live in a `cool-notes` folder in your system's config directory:

| OS | Folder |
|---|---|
| Linux | `~/.config/cool-notes/` (or `$XDG_CONFIG_HOME/cool-notes/`) |
| macOS | `~/Library/Application Support/cool-notes/` |
| Windows | `%AppData%\cool-notes\` |

To use a different folder, set the `COOL_NOTES_CONFIG_DIR` environment
variable when you start the app, for example
`COOL_NOTES_CONFIG_DIR=~/my-settings cool-notes` (Linux, macOS) or
`$env:COOL_NOTES_CONFIG_DIR = "$HOME\my-settings"; cool-notes` (PowerShell), or
set it in your shell profile. The folder holds:

- `config.json`: `file` (notes path) and `theme`. Delete `file` to see the
  first-run prompt again.
- `state.json`: the open note and cursor positions, so you continue where you
  left off.

Built-in themes: `auto` (follows the terminal's light or dark background),
`catppuccin-mocha`, `catppuccin-latte`, `nord`, `gruvbox-dark`, `tokyo-night`,
`rose-pine-dawn`, and `terminal`, which uses your terminal's own 16 colors.

Add your own as `themes/<name>.json` inside the settings folder. Any field you leave
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

## Releasing

Pushing a `v*` tag runs GoReleaser (`.goreleaser.yaml`) in GitHub Actions, which
tests, builds and publishes the binaries, then updates the Homebrew cask in
[homebrew-tap](https://github.com/thinkbig1979/homebrew-tap) and the Scoop
manifest in [scoop-bucket](https://github.com/thinkbig1979/scoop-bucket). Each
of those repos has a write-only deploy key whose private half is stored as a
secret here (`HOMEBREW_TAP_DEPLOY_KEY`, `SCOOP_BUCKET_DEPLOY_KEY`).

```sh
git tag -a v0.2.0 -m "cool-notes v0.2.0" && git push origin v0.2.0
```

## License

MIT, see [LICENSE](LICENSE).
