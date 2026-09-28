#!/bin/bash
# Contract tests for the supported macOS bootstrap. These use fake commands and
# never contact Homebrew, GitHub, or a local Pantheon installation.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d /private/tmp/pantheon-install-test.XXXXXX)"
cleanup() { /usr/bin/find "$tmp" -depth -delete 2>/dev/null || true; }
trap cleanup EXIT

mkdir -p "$tmp/bin"
cat > "$tmp/bin/brew" <<'EOF'
#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >> "${PANTHEON_BOOTSTRAP_LOG:?}"
if [[ "$1" == "list" ]]; then exit 1; fi
EOF
cat > "$tmp/bin/sirsi" <<'EOF'
#!/bin/bash
set -euo pipefail
[[ "$1" == "version" ]]
printf 'sirsi v-test\n'
EOF
chmod +x "$tmp/bin/brew" "$tmp/bin/sirsi"

PANTHEON_BOOTSTRAP_LOG="$tmp/brew.log" \
PATH="$tmp/bin:/usr/bin:/bin" \
  /bin/bash "$root/scripts/install.sh" > "$tmp/managed.out"

/usr/bin/grep -Fxq 'install --cask sirsimaster/tools/sirsi-pantheon' "$tmp/brew.log"
/usr/bin/grep -Fq 'Pantheon is ready.' "$tmp/managed.out"
/usr/bin/grep -Fq 'sirsi v-test' "$tmp/managed.out"

set +e
PATH="/usr/bin:/bin" /bin/bash "$root/scripts/install.sh" > "$tmp/no-brew.out" 2>&1
no_brew_rc=$?
set -e
[[ "$no_brew_rc" -ne 0 ]]
/usr/bin/grep -Fq 'Install Homebrew: https://brew.sh' "$tmp/no-brew.out"
/usr/bin/grep -Fq 'Re-run this installer.' "$tmp/no-brew.out"

echo "install bootstrap contract: pass"
