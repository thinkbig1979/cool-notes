# Renders docs/demo.gif for the README. Needs vhs and ttyd.
#   vhs docs/demo.tape   (run from the repo root)
# VHS can't send some keys (Alt+Shift+arrows, End), so those are typed as raw
# escape sequences with Type@0ms.
Output docs/demo.gif

Set Shell bash
Set FontSize 16
Set Width 1000
Set Height 520
Set Padding 10
Set Theme "Catppuccin Mocha"
Set TypingSpeed 60ms

Hide
Type "go build -o /tmp/cool-notes-demo/cool-notes . && rm -rf /tmp/cool-notes-demo/cfg /tmp/cool-notes-demo/notes.txt" Enter
Sleep 3s
Type "cd /tmp/cool-notes-demo && export COOL_NOTES_CONFIG_DIR=$PWD/cfg PATH=$PWD:$PATH PS1='$ ' && clear" Enter
Sleep 500ms
Show

Type "cool-notes --file notes.txt" Sleep 300ms Enter
Sleep 1.2s

Type "Shopping list" Enter Tab
Type "milk" Enter
Type "eggs" Enter
Type "coffee"
Sleep 600ms

Ctrl+T
Sleep 400ms
Type "Project ideas" Enter Tab
Type "terminal pomodoro timer" Enter
Type "notes sync over git"
Sleep 500ms

Ctrl+T
Sleep 400ms
Type "Meeting notes"
Sleep 800ms

# Find across notes: jumps back to the shopping list
Ctrl+F
Sleep 500ms
Type "eggs"
Sleep 1.5s
Escape
Sleep 500ms

# Alt+Shift+Right moves the note one place right, twice
Type@0ms "[1;4C"
Sleep 1.2s
Type@0ms "[1;4C"
Sleep 1.5s

# Delete it, then Ctrl+Z brings it back
Ctrl+W
Sleep 1.2s
Type "y"
Sleep 1.5s
Ctrl+Z
Sleep 1.5s

# Leave the cursor at the end of "eggs" (End) and quit
Down
Up
Type@0ms "[F"
Sleep 800ms
Ctrl+Q
Sleep 1.5s

Type "cool-notes --file notes.txt" Sleep 300ms Enter
Sleep 1.8s
Type " (a dozen)"
Sleep 2.5s

Hide
Ctrl+Q
