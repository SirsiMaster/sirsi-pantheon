#!/bin/bash
# sirsi-triage.sh — the always-on Tier-1 triage loop (orchestration brain, Step 1).
#
# WHAT THIS IS (owner directive 2026-06-30): a headless, always-on service that works
# the router's stranded inboxes 24/7 so the owner never has to babysit. It is the
# Tier-1 of the orchestration brain (docs/prd/ORCHESTRATION_BRAIN.md): RULES first,
# a local model (gemma via sirsi-brain) for the ambiguous remainder, escalate the rest.
#
# It is NOT a model running a loop — the loop is deterministic bash; a model is invoked
# only to classify an item the rules couldn't. It works with NO model too (Level 0):
# rules alone classify, and anything unclear is ESCALATED (fail-safe), never guessed.
#
# SAFETY (Rule A1/A12 — conservative by construction):
#   - Default action is ESCALATE (leave the item, surface it). Mutation is the exception.
#   - AUTO-RESOLVE only closes items on a TIGHT allowlist: pure completion/ack/FYI
#     notices with NO open request. Anything that asks for build/review/bind/judgment,
#     or is ambiguous, is NEVER auto-closed — it goes to the owner digest.
#   - DRY_RUN=1 classifies + digests but performs zero mutations.
#   - Every action is logged with its reason. Never routes/binds/deploys anything.
#
# OUTPUT:
#   - ~/.sirsi/triage-digest.md  — the live "what needs you / what's noise" digest.
#   - closes pure-FYI items it can safely resolve (logged), shrinking the noise.
#   - escalates genuine-judgment items by leaving them + listing them in the digest.

set -u
REPO=${SIRSI_TRIAGE_REPO:-$HOME/Development/sirsi-pantheon}
SIRSI=$HOME/.local/bin/sirsi
BRAIN=$HOME/.local/bin/sirsi-brain.sh
ORCH=${SIRSI_TRIAGE_ORCH:-claude-home}          # the human-conduit agent (owner's session)
POLL=${SIRSI_TRIAGE_POLL:-90}
DRY_RUN=${DRY_RUN:-0}
AUTO_RESOLVE=${SIRSI_TRIAGE_AUTORESOLVE:-1}      # close allowlisted pure-FYI items
USE_MODEL=${SIRSI_TRIAGE_USE_MODEL:-1}           # ask gemma for ambiguous items (0 = rules-only, Level 0)
LOG=$HOME/.sirsi/triage.log
DIGEST=$HOME/.sirsi/triage-digest.md
ITEMS="$REPO/.agents/idea-router/items"
mkdir -p "$(dirname "$LOG")"

log(){ echo "[$(date -u +%FT%TZ)] $*" >> "$LOG"; }

# classify_rules <title> <body>  → echoes ROUTINE | NEEDS-AGENT | AMBIGUOUS
# Pure heuristic, no model. Conservative: only ROUTINE when it's clearly terminal +
# carries no open request; NEEDS-AGENT when it asks for agentic work; else AMBIGUOUS.
classify_rules() {
  python3 - "$1" "$2" <<'PY'
import sys,re
title=sys.argv[1].lower(); body=sys.argv[2].lower()
text=title+"\n"+body
# Agentic asks → must reach an agent/owner. Never auto-resolve these.
agentic=re.search(r'\b(bind|review|build|implement|deploy|validate|fix|merge|open a pr|please|need(s|ed)?|decide|approve|escalat|provision|secret|owner)\b', text)
question=('?' in body)
# Terminal/FYI markers with no open ask.
done=re.search(r'\b(done|closed|complete[d]?|verified|shipped|merged|landed|ack(nowledg)?|fyi|no action|completed by|received)\b', text) or '✅' in text
if agentic or question:
    print("NEEDS-AGENT" if agentic else "AMBIGUOUS"); sys.exit()
if done:
    print("ROUTINE"); sys.exit()
print("AMBIGUOUS")
PY
}

# ask_model <title> <body> → ROUTINE | NEEDS-AGENT | NEEDS-OWNER  (gemma, zero-token)
ask_model() {
  [ "$USE_MODEL" = "1" ] && [ -x "$BRAIN" ] || { echo "NEEDS-OWNER"; return; }
  local out
  out=$("$BRAIN" ask --max-tokens 40 "Classify this router item into EXACTLY one word: ROUTINE (a pure FYI/ack/completion notice needing no action), NEEDS-AGENT (asks for build/review/bind/code work), or NEEDS-OWNER (needs a human decision/credential). Title: $1. Body: $2. Answer with one word only." 2>/dev/null \
        | grep -oiE 'ROUTINE|NEEDS-AGENT|NEEDS-OWNER' | head -1 | tr a-z A-Z)
  echo "${out:-NEEDS-OWNER}"   # fail-safe: unclear → owner
}

cycle() {
  [ -d "$ITEMS" ] || { log "no items dir at $ITEMS"; return; }
  local routine=0 needagent=0 needowner=0 closed=0
  : > "$DIGEST.tmp"
  echo "# Sirsi Triage Digest — $(date -u +%FT%TZ)" >> "$DIGEST.tmp"
  echo "" >> "$DIGEST.tmp"
  echo "## Needs you / an agent" >> "$DIGEST.tmp"

  # Enumerate open items (any recipient) — id, from, to, type, title.
  while IFS=$'\t' read -r id frm to typ title; do
    [ -z "$id" ] && continue
    local f="$ITEMS/$id.md"
    [ -f "$f" ] || continue
    local body cat
    body=$(awk 'ff{print} /## Instructions/{ff=1}' "$f" | sed '/## Result/,$d' | tr '\n' ' ' | cut -c1-600)
    # STRUCTURAL VETO before any classifier, on the FULL body (the 600-char
    # snippet once hid a 27-finding item's "Ask:" section from both rules and
    # model — it auto-closed as FYI and sat looking-resolved for 3 days;
    # claude-nexus report 20260705-191402). An explicit required-action
    # section names a task someone must do: NEVER auto-closable, whatever
    # terminal markers the rest of the body carries. Deterministic floor
    # rules the model (ADR-034).
    if grep -qiE '^[[:space:]]*(##[[:space:]]*)?(ask|action required|required action|owner action)[[:space:]]*[:\-]' "$f"; then
      cat="NEEDS-AGENT"
    else
      cat=$(classify_rules "$title" "$body")
      [ "$cat" = "AMBIGUOUS" ] && cat=$(ask_model "$title" "$body")
    fi

    case "$cat" in
      ROUTINE)
        routine=$((routine+1))
        if [ "$AUTO_RESOLVE" = "1" ] && [ "$DRY_RUN" != "1" ]; then
          if (cd "$REPO" && "$SIRSI" router close "$id" --agent horus --result "Tier-1 triage: routine FYI/completion notice with no explicit required-action section, no open action (auto-resolved by sirsi-triage)." >/dev/null 2>&1); then
            closed=$((closed+1)); log "CLOSED routine $id (to=$to from=$frm)"
          else log "close FAILED $id"; fi
        else
          log "WOULD-CLOSE routine $id (dry-run/auto-resolve off)"
        fi
        ;;
      NEEDS-AGENT)
        needagent=$((needagent+1))
        echo "- [AGENT] to=$to from=$frm — $title" >> "$DIGEST.tmp"
        ;;
      *)
        needowner=$((needowner+1))
        echo "- [OWNER] to=$to from=$frm — $title" >> "$DIGEST.tmp"
        ;;
    esac
  done < <(cd "$REPO" && python3 - "$ITEMS" <<'PY'
import os,re,sys
d=sys.argv[1]
for fn in sorted(os.listdir(d)):
    if not fn.endswith('.md'): continue
    fm=re.match(r'---\n(.*?)\n---',open(os.path.join(d,fn)).read(),re.DOTALL)
    if not fm: continue
    f=fm.group(1)
    if not re.search(r'status:\s*"?open"?',f): continue
    g=lambda k:(re.search(r'^%s:\s*"?(.*?)"?\s*$'%k,f,re.M) or [None,''])[1]
    print('\t'.join([fn[:-3], g('from'), g('to'), g('type'), g('title')[:80]]))
PY
)

  echo "" >> "$DIGEST.tmp"
  echo "## Summary: ${needowner} need-owner · ${needagent} need-agent · ${routine} routine (${closed} auto-closed this pass)" >> "$DIGEST.tmp"
  mv "$DIGEST.tmp" "$DIGEST"
  log "cycle: owner=$needowner agent=$needagent routine=$routine closed=$closed (dry=$DRY_RUN autoresolve=$AUTO_RESOLVE model=$USE_MODEL)"
  (cd "$REPO" && "$SIRSI" thread heartbeat --thread "${SIRSI_TRIAGE_THREAD:-triage}" >/dev/null 2>&1) || true
}

case "${1:-loop}" in
  once) cycle ;;
  loop)
    log "sirsi-triage started (poll=${POLL}s dry=$DRY_RUN autoresolve=$AUTO_RESOLVE model=$USE_MODEL)"
    echo $$ > "$HOME/.sirsi/triage.pid"
    trap 'rm -f "$HOME/.sirsi/triage.pid"' EXIT
    while true; do cycle; sleep "$POLL"; done
    ;;
  *) echo "usage: sirsi-triage.sh once|loop"; exit 1 ;;
esac
