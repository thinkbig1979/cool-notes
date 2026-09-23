# Renders docs/demo.gif for the README. Needs vhs and ttyd.
#   vhs docs/demo.tape   (run from the repo root)
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
Sleep 800ms

Ctrl+T
Sleep 500ms
Type "Project ideas" Enter Tab
Type "terminal pomodoro timer" Enter
Type "notes sync over git"
Sleep 600ms

# Alt+1, sent as a raw escape sequence (VHS allows Alt only with letters)
Type@0ms "1"
Sleep 1s
# Alt+2, then Up and End (raw, VHS has no End key) to leave the cursor mid-note
Type@0ms "2"
Sleep 600ms
Up
Type@0ms "[F"
Sleep 1s

Ctrl+Q
Sleep 1.5s

Type "cool-notes --file notes.txt" Sleep 300ms Enter
Sleep 1.8s
Type " (in Go)"
Sleep 2.5s

Hide
Ctrl+Q
