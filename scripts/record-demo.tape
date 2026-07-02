# Sirsi Pantheon — README hero demo
#
# Generates assets/demo.gif (the GIF embedded at the top of README.md).
# Record with vhs (https://github.com/charmbracelet/vhs):
#
#   cd <repo root> && go build -o bin/sirsi ./cmd/sirsi/ && PATH="$PWD/bin:$PATH" vhs scripts/record-demo.tape
#
# Arc (~15s): scan → findings table → clean (dry-run preview) → diagnose → ghosts.
# Theme: Pantheon gold (#C8A951) on black (#0F0F0F), lapis accents.

Output assets/demo.gif

Set Shell "zsh"
Set FontSize 14
Set Width 1200
Set Height 700
Set Padding 16
Set TypingSpeed 35ms
Set Theme { "name": "sirsi-gold", "background": "#0F0F0F", "foreground": "#C8A951", "cursor": "#C8A951", "selection": "#1A1A5E", "black": "#0F0F0F", "red": "#E5484D", "green": "#30A46C", "yellow": "#C8A951", "blue": "#3E63DD", "magenta": "#8E4EC6", "cyan": "#05A2C2", "white": "#EDEDED", "brightBlack": "#666666", "brightRed": "#FF6369", "brightGreen": "#3CB179", "brightYellow": "#E2C56E", "brightBlue": "#5B7FE8", "brightMagenta": "#A46FD8", "brightCyan": "#39C0D8", "brightWhite": "#FFFFFF" }

Sleep 300ms

# 1. Find the waste (81 rules)
Type "sirsi scan"
Enter
Sleep 4.5s

# 2. Preview the fix — sirsi clean is the preferred alias of `sirsi anubis clean`;
#    it is a dry-run by default (--confirm applies, trash-first).
Type "sirsi clean"
Enter
Sleep 3.5s

# 3. Name the cause
Type "sirsi diagnose"
Enter
Sleep 3.5s

# 4. Find remnants of uninstalled apps
Type "sirsi ghosts"
Enter
Sleep 3s
