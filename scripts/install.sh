#!/bin/bash
# Sirsi Pantheon — supported macOS bootstrap.
#
# The commercial product is one notarized Pantheon.app payload distributed as a
# Homebrew Cask. The Cask links its bundled `sirsi` CLI, so this path never
# downloads a separate, unsigned CLI archive or leaves the menu bar behind.
set -euo pipefail

readonly CASK="sirsimaster/tools/sirsi-pantheon"
readonly HOMEBREW_URL="https://brew.sh"

say() { printf '%s\n' "$*"; }
fail() { say "ERROR: $*" >&2; exit 1; }

if [[ "$(/usr/bin/uname -s)" != "Darwin" ]]; then
    fail "Pantheon’s supported commercial installer is the macOS Cask. Build from source only if you explicitly need an unsupported development environment."
fi

if [[ "$(/usr/bin/uname -m)" != "arm64" ]]; then
    fail "Pantheon’s current commercial app is Apple-silicon only. Use an arm64 Mac, or wait for a separately published universal build."
fi

if ! command -v brew >/dev/null 2>&1; then
    say "Pantheon installs through Homebrew so the app, CLI, upgrades, and uninstall share one managed payload."
    say "1. Install Homebrew: ${HOMEBREW_URL}"
    say "2. Re-run this installer."
    exit 1
fi

if brew list --cask "$CASK" >/dev/null 2>&1; then
    say "Updating the managed Pantheon app and CLI…"
    brew upgrade --cask "$CASK"
else
    say "Installing the managed Pantheon app and CLI…"
    brew install --cask "$CASK"
fi

if ! command -v sirsi >/dev/null 2>&1; then
    fail "Homebrew installed the Cask but did not expose its bundled CLI. Run 'brew reinstall --cask ${CASK}', then retry; do not install a separate sirsi archive."
fi

say "Pantheon is ready."
sirsi version
say "Open /Applications/Pantheon.app to start the menu bar control center."
