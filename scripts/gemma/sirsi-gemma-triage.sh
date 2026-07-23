#!/bin/bash
# sirsi-gemma-triage.sh — local-Gemma router triage (zero API tokens).
#
# Screens open router items with the local MLX-Gemma model so the expensive
# cloud model only reads the ESCALATE set. Gemma classifies + summarizes;
# it NEVER issues a binding verdict (the quant misses subtle bugs — screening
# only, verification stays with claude-home).
#
# Usage:
#   sirsi-gemma-triage.sh <agent_id>     # triage one recipient's open queue
#   sirsi-gemma-triage.sh --all          # triage every open item
#
# Output (stdout, one line per item):
#   <CLASS>\t<item_id>\t<one-line-reason>
# CLASS ∈ STALE | SUPERSEDED | ACTIONABLE | ESCALATE
#
# Designed to run inside the scheduled supervisor task: pipe ESCALATE rows to
# claude-home for real review; STALE/SUPERSEDED rows get auto-closed with the
# Gemma reason as evidence; ACTIONABLE rows stay open for their recipient.

set -u
# WARM-FIRST: hit the always-on MLX server so the model is loaded ONCE and reused across all
# items — no per-item cold reload (which was ~15GB per item, slow, and Jetsam-prone under memory
# pressure). The server also splits reasoning vs the final answer, so message.content is ALREADY
# the clean CLASS/REASON. mlx_lm.generate is the cold fallback only if the server is unreachable.
# Port comes from the port file the live server writes — hardcoding 11434 silently missed the
# warm server (capped-server listens on 8765) and cold-loaded every item, defeating the whole point.
# resolve_server re-reads the port file EVERY call — the warm server moves
# ports across model-swap restarts, and a port resolved once at startup went
# stale mid-run on 2026-07-23: SERVER pointed at a dead port, every warm call
# refused, and the cold fallback hung the whole run (item 20260723-043612).
resolve_server() {
  local port
  port=$( [ -f "$HOME/.sirsi/gemma-server.port" ] && cat "$HOME/.sirsi/gemma-server.port" || echo 11434 )
  echo "${GEMMA_SERVER:-http://127.0.0.1:${port}/v1/chat/completions}"
}
SERVER=$(resolve_server)

# run_bounded <secs> <cmd...> — hard wall-clock cap on ANY child. The cold
# `sirsi gemma` fallback has no internal timeout and blocks on the broker
# flock; unbounded it turned a dead warm port into a 9-minute silent hang.
run_bounded() {
  local secs=$1; shift
  if command -v timeout >/dev/null 2>&1; then timeout "$secs" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then gtimeout "$secs" "$@"
  else perl -e 'alarm shift; exec @ARGV' "$secs" "$@"
  fi
}
MLX=$HOME/.venvs/mlx/bin/mlx_lm.generate
MODEL=${GEMMA_MODEL:-$( [ -f "$HOME/.sirsi/gemma-model.conf" ] && cat "$HOME/.sirsi/gemma-model.conf" || echo "mlx-community/gemma-4-12B-it-8bit" )}
REPO=$HOME/Development/sirsi-pantheon
DIR=$REPO/.agents/idea-router/items
MAXTOK=2048  # gemma-4 is a REASONING model: it deliberates (~450-900 tok) before emitting
             # CLASS/REASON. Generous headroom (2048) so even a long reasoning trace on a complex
             # item completes. Well within the 256K ctx; KV cost is trivial here.

# Server-first, so the CLI runtime is optional. Only hard-require it if the server is down.
server_up() { curl -s -o /dev/null -m 3 -w '%{http_code}' "${SERVER%/v1/*}/v1/models" 2>/dev/null | grep -q 200; }
if ! server_up && [ ! -x "$HOME/.local/bin/sirsi" ]; then
  echo "ERROR: warm server unreachable AND sirsi broker missing" >&2; exit 1
fi

# CALIBRATION PROBE — fail LOUD and FAST (router item 20260715-175752). A warm
# server decoding at 0.5 tok/s made `--all` over a 10-item queue need 20+ min
# and die on the supervisor's timeout (exit 124) — a silent hang that read as
# "triage broken" instead of "server degraded". One tiny measured completion up
# front: below the floor → exit 2 with the measured rate so the supervisor
# reports a DEGRADED screen and the cloud path proceeds knowingly.
TOKS_FLOOR=${GEMMA_TOKS_FLOOR:-5}
if server_up; then
  echo "· calibration probe (${SERVER})" >&2
  # NO "model" field: the probe measures whatever model the server actually
  # holds — same as every per-item call below. Pinning the id from
  # gemma-model.conf 401'd whenever the conf drifted from the loaded model
  # (resolver re-resolves; conf said qat-mxfp8 while the server served 8bit,
  # 2026-07-23) and the empty rate read as a false "0 tok/s DEGRADED".
  rate=$(SERVER="$SERVER" python3 - <<'PY' 2>/dev/null
import os, json, time, urllib.request
body = json.dumps({"messages":[{"role":"user","content":"Count from 1 to 20, digits only."}],
                   "max_tokens": 48, "temperature": 0.0}).encode()
req = urllib.request.Request(os.environ["SERVER"], data=body, headers={"Content-Type":"application/json"})
t0 = time.time()
try:
    with urllib.request.urlopen(req, timeout=90) as r:
        toks = json.loads(r.read())["usage"]["completion_tokens"]
except Exception:
    raise SystemExit
dt = time.time() - t0
print(int(toks / dt) if dt > 0 else 0)
PY
)
  # An EMPTY rate here means the server answered /v1/models but the probe
  # completion timed out — that is slower than any floor, not a pass. Verified
  # live 2026-07-16: a 1 tok/s server timed the 90s probe out and the empty
  # rate fail-OPENED into the exact crawl this guard exists to prevent.
  if [ -z "$rate" ] || [ "$rate" -lt "$TOKS_FLOOR" ]; then
    echo "ERROR: warm server DEGRADED — measured ${rate:-<probe timeout>} tok/s < ${TOKS_FLOOR} tok/s floor on ${MODEL}. Refusing to crawl; run sirsi-gemma-model-resolver.sh (empirical fit) or free memory." >&2
    exit 2
  fi
fi

filter_agent="${1:-}"
[ "$filter_agent" = "--all" ] && filter_agent=""

# Build the item list (open only, optionally filtered by recipient)
items=$(cd "$REPO" && python3 -c "
import os, re, sys
d = '$DIR'
flt = '$filter_agent'
for fn in sorted(os.listdir(d)):
    if not fn.endswith('.md'): continue
    txt = open(os.path.join(d, fn)).read()
    m = re.match(r'---\n(.*?)\n---', txt, re.DOTALL)
    if not m: continue
    fm = m.group(1)
    if not re.search(r'^status:\s*\"?open\"?', fm, re.MULTILINE): continue
    to = re.search(r'^to:\s*\"?([\w-]+)\"?', fm, re.MULTILINE)
    to = to.group(1) if to else '?'
    if flt and to != flt: continue
    title = re.search(r'^title:\s*\"(.*?)\"', fm, re.MULTILINE)
    title = title.group(1) if title else ''
    # first 400 chars of the instructions body for context
    body = txt[m.end():]
    bm = re.search(r'## Instructions\s*\n(.*)', body, re.DOTALL)
    snippet = (bm.group(1) if bm else body).strip().replace(chr(10), ' ')[:400]
    print(f'{fn[:-3]}\t{to}\t{title}\t{snippet}')
")

[ -z "$items" ] && { echo "(no open items$([ -n "$filter_agent" ] && echo " for $filter_agent"))"; exit 0; }

while IFS=$'\t' read -r id to title snippet; do
  [ -z "$id" ] && continue
  # Progress per item (stderr, so stdout stays the clean TSV contract): a
  # hang is now attributable to a named item instead of reading as a dead run.
  echo "· triaging ${id}" >&2
  prompt="You are a router-queue triage assistant for a multi-agent dev system. Classify this work item into EXACTLY ONE category and give a one-line reason (max 15 words).

Categories:
- STALE: old (weeks), recipient never acted, no longer relevant
- SUPERSEDED: the work was done elsewhere or replaced by a newer artifact
- ACTIONABLE: real outstanding work the recipient should do
- ESCALATE: needs a human/senior reviewer's judgment (security, binding verdict, architecture decision, ambiguous)

Item recipient: ${to}
Item title: ${title}
Item body: ${snippet}

Respond in EXACTLY this format, nothing else:
CLASS: <one category>
REASON: <one line>"

  # WARM SERVER first: one HTTP call, model already loaded. The server returns the clean final
  # answer in message.content (reasoning is split into message.reasoning), so no channel-strip
  # is needed on the happy path. Falls back to a cold in-process generate only if the server is
  # down — and that fallback still needs the <channel|> strip since the CLI interleaves both.
  final=$(SERVER="$SERVER" MAXTOK="$MAXTOK" python3 - "$prompt" <<'PY' 2>/dev/null
import sys, os, json, urllib.request
body = json.dumps({"messages":[{"role":"user","content":sys.argv[1]}],
                   "max_tokens":int(os.environ["MAXTOK"]),"temperature":0.0,"stream":False}).encode()
req = urllib.request.Request(os.environ["SERVER"], data=body, headers={"Content-Type":"application/json"})
try:
    with urllib.request.urlopen(req, timeout=120) as r:
        m = json.loads(r.read())["choices"][0]["message"]
        print(m.get("content") or m.get("reasoning") or "")
except Exception:
    pass
PY
)
  if [ -z "$final" ]; then
    # Warm call failed — before paying for a cold load, re-resolve the port
    # (the server moves across model-swap restarts; a stale port here is the
    # 2026-07-23 hang) and retry warm once on the fresh endpoint.
    fresh=$(resolve_server)
    if [ "$fresh" != "$SERVER" ]; then
      echo "· server moved (${SERVER} → ${fresh}), retrying warm" >&2
      SERVER="$fresh"
      final=$(SERVER="$SERVER" MAXTOK="$MAXTOK" python3 - "$prompt" <<'PY' 2>/dev/null
import sys, os, json, urllib.request
body = json.dumps({"messages":[{"role":"user","content":sys.argv[1]}],
                   "max_tokens":int(os.environ["MAXTOK"]),"temperature":0.0,"stream":False}).encode()
req = urllib.request.Request(os.environ["SERVER"], data=body, headers={"Content-Type":"application/json"})
try:
    with urllib.request.urlopen(req, timeout=120) as r:
        m = json.loads(r.read())["choices"][0]["message"]
        print(m.get("content") or m.get("reasoning") or "")
except Exception:
    pass
PY
)
    fi
  fi
  if [ -z "$final" ]; then
    # Fallback: server unreachable → cold in-process generate (per-item reload). Strip reasoning.
    # ADR-031-C: cold path through the broker (NodeCapacity gate + lock), never raw mlx_lm.
    # HARD-BOUNDED: sirsi gemma blocks on the broker flock with no internal
    # timeout — unbounded, a dead warm port turned into a 9-minute silent hang.
    echo "· ${id}: warm unreachable, cold fallback (180s cap)" >&2
    raw=$(run_bounded 180 "$HOME/.local/bin/sirsi" gemma --model "$MODEL" --max-tokens "$MAXTOK" "$prompt" 2>/dev/null)
    final=$(printf '%s' "$raw" | perl -0777 -pe 's/.*<channel\|>//s' 2>/dev/null)
    [ -z "$final" ] && final="$raw"
  fi
  final=$(printf '%s\n' "$final" | grep -vE "^=+$|^Prompt:|^Generation:|^Peak memory:|^Fetching|<end_of_turn>|<start_of_turn>|<\|?channel>|^model$")
  # Take the LAST CLASS:/REASON: — the final verdict is always last; any reasoning-block echo
  # appears earlier. This stays correct even if the channel-strip missed (truncated trace).
  cls=$(echo "$final" | grep -oE "CLASS:[[:space:]]*(STALE|SUPERSEDED|ACTIONABLE|ESCALATE)" | grep -oE "(STALE|SUPERSEDED|ACTIONABLE|ESCALATE)" | tail -1)
  [ -z "$cls" ] && cls=$(echo "$final" | grep -oE "\b(STALE|SUPERSEDED|ACTIONABLE|ESCALATE)\b" | tail -1)
  reason=$(echo "$final" | grep -oE "REASON:.*" | sed 's/REASON:[[:space:]]*//' | tail -1)
  [ -z "$reason" ] && reason=$(echo "$final" | sed '/^[[:space:]]*$/d' | grep -vE "^[[:space:]]*CLASS:" | tail -1 | cut -c1-120)
  [ -z "$cls" ] && cls="ESCALATE"  # genuine parse failure → escalate (safe default)
  [ -z "$reason" ] && reason="(gemma parse failure — escalated for safety)"
  printf "%s\t%s\t%s\t%s\n" "$cls" "$id" "$to" "$reason"
done <<< "$items"
