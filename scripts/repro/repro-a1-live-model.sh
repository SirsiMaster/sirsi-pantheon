#!/usr/bin/env bash
# A1 live-model-substrate repro. Builds a synthetic $HOME containing a
# HuggingFace hub with one model served by an installed SNE launchd job and one
# cold model, ages both past the rule's 30-day gate, then asks each binary what
# it would reclaim.
#
# Usage: repro-a1.sh <before-binary> <after-binary>
set -euo pipefail
BEFORE="${1:?before binary}"; AFTER="${2:?after binary}"
H=$(mktemp -d)
trap 'rm -rf "$H"' EXIT

HUB="$H/.cache/huggingface/hub"
SERVED="$HUB/models--mlx-community--gemma-4-12B-it-8bit/snapshots/200bb6db"
COLD="$HUB/models--someone--abandoned-7b/snapshots/aaaa"
mkdir -p "$SERVED" "$COLD" "$H/Library/LaunchAgents"
mkfile -n 2g "$SERVED/model.safetensors" 2>/dev/null || dd if=/dev/zero of="$SERVED/model.safetensors" bs=1m count=64 2>/dev/null
dd if=/dev/zero of="$COLD/model.safetensors" bs=1m count=64 2>/dev/null

cat > "$H/Library/LaunchAgents/ai.sirsi.gemma-broker.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>Label</key><string>ai.sirsi.gemma-broker</string>
  <key>ProgramArguments</key><array>
    <string>/opt/sne/sne-server-macos-arm64</string>
    <string>serve</string>
    <string>$SERVED</string>
    <string>--profile</string><string>interactive</string>
    <string>127.0.0.1:8477</string>
  </array>
</dict></plist>
PLIST

# Age everything past minAgeDays=30 (the state this hub reaches on any machine
# that simply stops downloading models for a month).
find "$HUB" -exec touch -t 202601010000 {} + 2>/dev/null || true

report() {
  HOME="$H" "$1" scan --all --json 2>/dev/null | python3 -c "
import json,sys
d=json.load(sys.stdin)
fs=d.get('Findings') or []
hf=[f for f in fs if f.get('RuleName')=='huggingface_cache']
served='$SERVED'
if not hf: print('    (no huggingface findings)')
for f in hf:
    p=f.get('Path','')
    kills = served.startswith(p) or p.startswith(served)
    print(f\"    {p}  {f.get('SizeBytes',0)/1e6:.0f} MB{'   <-- DESTROYS THE LIVE MODEL' if kills else ''}\")
"
}

echo "served snapshot: $SERVED"
echo
echo "BEFORE ($BEFORE):"; report "$BEFORE"
echo
echo "AFTER  ($AFTER):"; report "$AFTER"
