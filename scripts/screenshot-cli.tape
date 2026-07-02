# Sirsi Pantheon — CLI surface screenshot (assets/screenshot-cli.png)
#
# Captures a real `sirsi scan` result: findings table, top findings with
# evidence and severity, and a "what's next" block. Nothing is faked — this is
# exactly what the command prints on a real machine.
#
# Record with vhs (https://github.com/charmbracelet/vhs):
#   cd <repo root> && go build -o bin/sirsi ./cmd/sirsi/ && PATH="$PWD/bin:$PATH" vhs scripts/screenshot-cli.tape

Set Shell "zsh"
Set FontSize 14
Set Width 1100
Set Height 900
Set Padding 20
Set TypingSpeed 30ms
Set Theme { "name": "sirsi-gold", "background": "#0F0F0F", "foreground": "#C8A951", "cursor": "#0F0F0F", "selection": "#1A1A5E", "black": "#0F0F0F", "red": "#E5484D", "green": "#30A46C", "yellow": "#C8A951", "blue": "#3E63DD", "magenta": "#8E4EC6", "cyan": "#05A2C2", "white": "#EDEDED", "brightBlack": "#666666", "brightRed": "#FF6369", "brightGreen": "#3CB179", "brightYellow": "#E2C56E", "brightBlue": "#5B7FE8", "brightMagenta": "#A46FD8", "brightCyan": "#39C0D8", "brightWhite": "#FFFFFF" }

Sleep 300ms
Type "sirsi scan"
Enter
Sleep 11s
Screenshot assets/screenshot-cli.png
