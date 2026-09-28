#!/bin/sh
# Keep Ma'at as Pantheon's public knowledge/decision surface while the legacy
# Seshat ingestion implementation remains a deliberately bounded adapter.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
actions="$root/internal/dashboard/actions.go"
knowledge="$root/cmd/sirsi/maatknowledge.go"
native="$root/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"
recipe="$root/contracts/stacklab/maat-system-one-recipe-v1.json"

for target in "$actions" "$knowledge" "$native" "$recipe"; do
  [ -f "$target" ] || { echo "missing Ma'at knowledge contract input: $target" >&2; exit 1; }
done

if /usr/bin/grep -Fq 'Key: "seshat/ingest"' "$actions" || /usr/bin/grep -Fq 'Label: "Seshat Ingest"' "$actions"; then
  echo "legacy Seshat dashboard action remains public" >&2
  exit 1
fi

for needle in \
  'Key: "maat/knowledge/refresh"' \
  'Label: "Refresh Ma'\''at knowledge"' \
  'Args: []string{"seshat", "ingest"}' \
  'dashboard callers never need to address that legacy deity directly.'; do
  /usr/bin/grep -Fq "$needle" "$actions" || {
    echo "Ma'at knowledge dashboard route missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'Ma'\''at is the single operator authority' \
  'safeMaatKnowledgeItems' \
  'secret match' \
  'does not ingest, export, rescore, or change'; do
  /usr/bin/grep -Fq "$needle" "$knowledge" || {
    echo "Ma'at knowledge projection contract missing: $needle" >&2
    exit 1
  }
done

/usr/bin/grep -Fq 'MaatKnowledgeView' "$native" || {
  echo "native Ma'at knowledge surface is missing" >&2
  exit 1
}

/usr/bin/grep -Fq '"id": "maat-knowledge-surface"' "$recipe" || {
  echo "Stack Lab Ma'at knowledge component is missing" >&2
  exit 1
}

echo "Ma'at knowledge surface contract: pass"
