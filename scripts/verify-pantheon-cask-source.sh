#!/usr/bin/env bash
# Validate the Pantheon cask contract without contacting Homebrew or installing.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd -P)"
CASK="$ROOT/homebrew/Casks/sirsi-pantheon.rb"
fail() {
    echo "pantheon_cask_source accepted=false reason=$1" >&2
    exit 1
}
require() {
    local pattern="$1" label="$2"
    grep -Eq "$pattern" "$CASK" || fail "missing_${label}"
}
reject() {
    local pattern="$1" label="$2"
    ! grep -Eq "$pattern" "$CASK" || fail "forbidden_${label}"
}

[[ -f "$CASK" ]] || fail "missing_cask"
ruby -c "$CASK" >/dev/null || fail "ruby_syntax"

require '^cask "sirsi-pantheon" do$' cask_name
require '^  version "[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?"$' version
require '^  sha256 "[0-9a-f]{64}"$' sha256
require 'github\.com/SirsiMaster/sirsi-pantheon/releases/download/v#\{version\}/SirsiPantheon-#\{version\}-arm64\.dmg' dmg_url
require '^  app "Pantheon\.app"$' app_mapping
require 'binary "#\{appdir\}/Pantheon\.app/Contents/MacOS/sirsi"' cli_mapping
require 'uninstall launchctl: "ai\.sirsi\.pantheon"' launchd_label
require 'quit:[[:space:]]*"ai\.sirsi\.pantheon"' quit_mapping
require '"~/Library/LaunchAgents/ai\.sirsi\.pantheon\.plist"' launchd_zap

reject '^formula ' formula_syntax
reject 'brew (install|upgrade|reinstall|uninstall) sirsi-pantheon' formula_commands
reject 'sha256 "(TODO|0{64})"' placeholder_digest

echo "pantheon_cask_source accepted=true cask=sirsi-pantheon channel=homebrew-cask architecture=arm64 lifecycle=install-upgrade-rollback-uninstall"
