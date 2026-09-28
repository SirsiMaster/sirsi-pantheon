#!/bin/sh
# Keep Stack Lab's native app surface bound to the canonical Go doctor report.
# The macOS UI may render guidance, but it may not recreate, soften, or replace
# the doctor’s origin/registry authority in Swift.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
view="$root/macapp/Sources/SirsiMenubar/StackLabView.swift"
catalog_view="$root/macapp/Sources/SirsiMenubar/StackLabCatalogView.swift"
library="$root/macapp/Sources/SirsiMenubar/ControlCenter.swift"
engine="$root/macapp/Sources/SirsiMenubar/SirsiEngine.swift"

[ -f "$view" ] || { echo "missing Stack Lab native view" >&2; exit 1; }
[ -f "$catalog_view" ] || { echo "missing Stack Lab catalog view" >&2; exit 1; }
[ -f "$library" ] || { echo "missing native control center" >&2; exit 1; }
[ -f "$engine" ] || { echo "missing native command engine" >&2; exit 1; }

for needle in \
  '["stacklab", "doctor", "--json"]' \
  'StackLabReport' \
  'ProjectBar(engine: engine)' \
  'Choose the Pantheon project' \
  'No registry result has been inferred yet.' \
  'No authority result was inferred.' \
  'not treated as clean' \
  'Review evidence in Ma'\''at'; do
  /usr/bin/grep -Fq "$needle" "$view" || {
    echo "Stack Lab native contract missing: $needle" >&2
    exit 1
  }
done

for needle in \
  '["stacklab", "catalog", "--json"]' \
  'StackLabCatalog' \
  'Replaceable product recipes' \
  'Local source contracts · remote authority remains in Doctor'; do
  /usr/bin/grep -Fq "$needle" "$catalog_view" || {
    echo "Stack Lab native catalog contract missing: $needle" >&2
    exit 1
  }
done

/usr/bin/grep -Fq 'StackLabCatalogView(engine: engine)' "$view" || {
  echo "Stack Lab recipe catalog is not reachable from the native doctor" >&2
  exit 1
}

/usr/bin/grep -Fq 'libraryLink("Stack Lab"' "$library" || {
  echo "Stack Lab is not reachable from the Pantheon library" >&2
  exit 1
}

/usr/bin/grep -Fq '"stacklab"' "$engine" || {
  echo "Stack Lab is not repository-scoped in the native command engine" >&2
  exit 1
}

if /usr/bin/grep -Eq 'Process\(|runProgram\(|/bin/sh|/bin/bash' "$view" "$catalog_view"; then
  echo "Stack Lab native view bypasses the canonical sirsi doctor contract" >&2
  exit 1
fi

echo "Stack Lab native surface contract: pass"
