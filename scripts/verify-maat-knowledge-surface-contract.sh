#!/bin/sh
# Keep Ma'at as Pantheon's public knowledge/decision surface while the legacy
# Seshat ingestion implementation remains a deliberately bounded adapter.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
actions="$root/internal/dashboard/actions.go"
knowledge="$root/cmd/sirsi/maatknowledge.go"
projection="$root/internal/maat/knowledge/knowledge.go"
mcp="$root/internal/mcp/tools.go"
dashboard="$root/internal/dashboard/maat.go"
native="$root/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"
recipe="$root/contracts/stacklab/maat-system-one-recipe-v1.json"

for target in "$actions" "$knowledge" "$projection" "$mcp" "$dashboard" "$native" "$recipe"; do
  [ -f "$target" ] || { echo "missing Ma'at knowledge contract input: $target" >&2; exit 1; }
done

if /usr/bin/grep -Fq 'Key: "seshat/ingest"' "$actions" || /usr/bin/grep -Fq 'Label: "Seshat Ingest"' "$actions"; then
  echo "legacy Seshat dashboard action remains public" >&2
  exit 1
fi

for needle in \
  'Key: "maat/knowledge/refresh"' \
  'Label: "Refresh Ma'\''at knowledge"' \
  'Args: []string{"maat", "knowledge", "refresh"}' \
  'dashboard callers never need to address that legacy deity directly.'; do
  /usr/bin/grep -Fq "$needle" "$actions" || {
    echo "Ma'at knowledge dashboard route missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'Use:   "refresh"' \
  'Refresh Ma'\''at'\''s local knowledge from configured sources' \
  'seshatIngestCmd.RunE' \
  '"source", "since", "profile", "all-profiles"' \
  '`--export` is not' \
  'maatKnowledgeCmd.AddCommand(maatKnowledgeRefreshCmd)'; do
  /usr/bin/grep -Fq "$needle" "$knowledge" || {
    echo "Ma'at public knowledge refresh contract missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'Package knowledge projects the retained local knowledge cache through Ma'\''at.' \
  'func Load(home, query string) (View, error)' \
  'func Project(items []seshat.KnowledgeItem, query string) View' \
  'seshat.DefaultFilter()' \
  'view.Withheld++'; do
  /usr/bin/grep -Fq "$needle" "$projection" || {
    echo "Ma'at shared knowledge projection contract missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'Name:        "maat_knowledge"' \
  'func handleMaatKnowledge' \
  'knowledge.Load(home, query)' \
  'Read-only: it cannot ingest, export, change, or disclose withheld records.'; do
  /usr/bin/grep -Fq "$needle" "$mcp" || {
    echo "Ma'at MCP knowledge contract missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'func (s *Server) apiMaatKnowledge' \
  'MaatKnowledgeFn == nil' \
  'Ma'\''at knowledge projection not available' \
  'writeJSON(w, view)'; do
  /usr/bin/grep -Fq "$needle" "$dashboard" || {
    echo "Ma'at Horus knowledge contract missing: $needle" >&2
    exit 1
  }
done

/usr/bin/grep -Fq 'MaatKnowledgeView' "$native" || {
  echo "native Ma'at knowledge surface is missing" >&2
  exit 1
}

for needle in \
  'Refresh Ma'\''at knowledge?' \
  'Refresh local knowledge' \
  '"maat", "knowledge", "refresh"' \
  'It will not export knowledge, open a browser, authorize work, or make a remote decision.' \
  'The current cache remains available.' \
  'Local cache · refresh requires confirmation'; do
  /usr/bin/grep -Fq "$needle" "$native" || {
    echo "native Ma'at knowledge refresh contract missing: $needle" >&2
    exit 1
  }
done

/usr/bin/grep -Fq '"id": "maat-knowledge-surface"' "$recipe" || {
  echo "Stack Lab Ma'at knowledge component is missing" >&2
  exit 1
}

for needle in \
  '"internal/maat/knowledge/knowledge.go"' \
  '"internal/mcp/tools.go"' \
  '"internal/dashboard/maat.go"' \
  '"one sensitivity-filtered Ma'\''at knowledge projection shared by CLI, MCP, Horus dashboard, and native app"'; do
  /usr/bin/grep -Fq "$needle" "$recipe" || {
    echo "Stack Lab Ma'at knowledge inventory is incomplete: $needle" >&2
    exit 1
  }
done

echo "Ma'at knowledge surface contract: pass"
