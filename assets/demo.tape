# Sirsi Pantheon Demo — VHS Tape
# Run: vhs assets/demo.tape
# Produces: assets/demo.gif

Output assets/demo.gif

Set Shell "zsh"
Set FontSize 14
Set Width 900
Set Height 500
Set Theme "Catppuccin Mocha"
Set Padding 20
Set TypingSpeed 40ms

# Title
Type "# 𓁢 Sirsi Pantheon — Infrastructure Hygiene"
Enter
Sleep 1s

# Scan
Type "sirsi scan"
Enter
Sleep 8s

# Open dashboard
Type "sirsi dashboard"
Enter
Sleep 3s

# Show version
Type@200ms "# 80 rules · 627 findings · 31.9 GB waste found"
Enter
Sleep 2s

Type@200ms "# One binary. All deities. Zero telemetry."
Enter
Sleep 3s
