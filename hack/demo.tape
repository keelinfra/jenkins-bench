# The animated demo at the top of the README: docs/demo.gif.
#
# Re-record it with vhs (https://github.com/charmbracelet/vhs) from the
# repository root whenever the sample or the report layout changes:
#
#   vhs hack/demo.tape
#
# It builds the binary from this checkout, so the recording always shows the
# code it ships with. The output is the real `scan --demo` report; the hidden
# wrapper only paces it line by line so it can be read as it scrolls past.

Output docs/demo.gif

Require go
Require script

Set Shell "bash"
Set FontSize 14
Set Width 1000
Set Height 600
Set Padding 24
# keelinfra's own palette: a navy ground, amber for the attention colour, the
# same values as the README art and keelinfra.io. COLORTERM below lets the
# brand badge use the exact amber instead of its 256-colour stand-in.
Set Theme { "name": "keelinfra", "background": "#0c141d", "foreground": "#dfe7ee", "cursor": "#e8a33d", "selection": "#24425f", "black": "#0b2239", "red": "#e0566a", "green": "#4cc38a", "yellow": "#e8a33d", "blue": "#6f9fd8", "purple": "#c38fd6", "cyan": "#8fb8d4", "white": "#c3d0dc", "brightBlack": "#5b6e82", "brightRed": "#f0788a", "brightGreen": "#7ad8a8", "brightYellow": "#f3c46d", "brightBlue": "#8fb6e6", "brightPurple": "#d3a8e3", "brightCyan": "#acd0e6", "brightWhite": "#ffffff" }
Set Margin 20
Set MarginFill "#0b2239"
Set BorderRadius 10
Set TypingSpeed 60ms
Set Framerate 10
Set WindowBar Colorful

Env COLORTERM "truecolor"

Set WaitTimeout 180s
Set WaitPattern /^\$ *$/

Hide
Type `export PS1='$ ' BIN=$(mktemp -d) && go build -o $BIN/jenkins-bench ./cmd/jenkins-bench`
Enter
Wait
Type `jenkins-bench() { script -qfec "$BIN/jenkins-bench $*" /dev/null | while IFS= read -r l; do printf '%s\n' "$l"; sleep 0.02; done; } && clear`
Enter
Wait
Show

Sleep 500ms
Type "jenkins-bench scan --demo"
Sleep 600ms
Enter
Sleep 12s
