#!/bin/bash
# sirsi-gemma-worker.sh — local-Gemma router worker.
#
# Polls the `gemma` inbox and COMPLETES well-scoped reasoning/text tasks with the
# local MLX-Gemma model (zero API tokens), writing results back to the router as
# a close+Result. Runs as a daemon (cli-spawn wake target registered via
# `sirsi agent register gemma`).
#
# HONEST CAPABILITY BOUNDARY:
#   Gemma here is a single-shot completion model (mlx_lm.generate). It can:
#     - classify / triage / score
#     - summarize long threads, PR bodies, logs
#     - draft text (changelog entries, release notes, item summaries, replies)
#     - analyze + reason over provided text (multi-step via chained prompts)
#     - extract structured data (action items, entities, decisions)
#   It CANNOT:
#     - use tools (no git, gh, file edits, web) — it only reads the item text + emits text
#     - issue binding security/review verdicts (the 4-bit quant misses subtle bugs)
#     - run a true multi-step agentic loop with side effects
#   Items needing tools or binding judgment are answered with an ESCALATE result
#   pointing back to claude-home. "Complex multi-agent agentic workload" that is
#   pure reasoning/decomposition Gemma WILL attempt (it can produce the plan/
#   breakdown/draft); execution-with-side-effects still belongs to a real agent.
#
# Each task item should put the work in its Instructions body. Optional first line
# directive `TASK: <classify|summarize|draft|analyze|plan|extract|build>` tunes the
# prompt; absent → Gemma infers and answers generally.
#
# BUILD TASKS (canon, spec-fix Gap 2): gemma is text-in/text-out — it resolves NO
# file paths. A `TASK: build` item MUST embed the source it edits via
# `--instructions @file` (or inline). An un-embedded build task is malformed and
# produces a deliverable built on nothing.
#
# CANONICAL SOURCE: scripts/gemma/sirsi-gemma-worker.sh in sirsi-pantheon (CI-visible,
# reviewed). Deploy = `make install-gemma-worker` (copies to ~/.local/bin + kickstarts
# the ai.sirsi.gemma-worker LaunchAgent). Do NOT hand-edit the ~/.local/bin copy.

set -u
MLX=$HOME/.venvs/mlx/bin/mlx_lm.generate
# Model is resolved by sirsi-gemma-model-resolver.sh (largest/most-advanced/most-
# recent that fits the RAM budget) and written to gemma-model.conf. Env override
# wins; conf is next; proven fallback last.
MODEL_CONF=$HOME/.sirsi/gemma-model.conf
MAX_CONF=$HOME/.sirsi/gemma-model-max.conf
DEFAULT_MODEL=${GEMMA_MODEL:-$( [ -f "$MODEL_CONF" ] && cat "$MODEL_CONF" || echo "mlx-community/gemma-4-12B-it-8bit" )}
MAX_MODEL=$( [ -f "$MAX_CONF" ] && cat "$MAX_CONF" || echo "$DEFAULT_MODEL" )
# Free-RAM (GB) a model must have to load WITHOUT overcommitting the fleet.
# The max model (~28GB) only runs when the machine genuinely has room — else the
# task falls back to the fleet-safe default and says so. Pantheon discipline:
# never let a model load Jetsam-kill a sibling session.
MAX_MODEL_MIN_FREE_GB=${GEMMA_MAX_MIN_FREE:-32}
REPO=$HOME/Development/sirsi-pantheon
SIRSI=$HOME/.local/bin/sirsi
LOG=$HOME/.sirsi/gemma-worker.log
MAXTOK=${GEMMA_MAXTOK:-8192}   # was 512 → truncated builds. gemma-4 is a REASONING builder:
                               # it spends tokens thinking (checking its work) THEN answers, so it
                               # needs generous headroom to reach the final answer. Tunable via env.
POLL=${GEMMA_POLL:-60}

mkdir -p "$(dirname "$LOG")"
[ -x "$MLX" ] || { echo "[$(date -u +%FT%TZ)] ERROR: MLX runtime missing at $MLX" >> "$LOG"; exit 1; }

log(){ echo "[$(date -u +%FT%TZ)] $*" >> "$LOG"; }

free_gb() {
  vm_stat 2>/dev/null | python3 -c "
import sys,re
pg=16384;d={}  # Apple Silicon 16KB page (was 4096 = 4x-low)
for l in sys.stdin:
    m=re.match(r'\"?([\w\s]+)\"?:\s+(\d+)',l)
    if m:d[m.group(1).strip()]=int(m.group(2))*pg
print(round((d.get('Pages free',0)+d.get('Pages inactive',0)+d.get('Pages speculative',0))/1e9,1))"
}

# pick_model <body> → echoes the model to use + (on stderr) a note if it downgraded.
# A `MODEL: max` directive requests the big 31B model, but it loads ONLY if free RAM
# can take it without overcommitting the fleet; otherwise falls back to the default
# and the caller appends a note. Hybrid policy (owner-chosen 2026-06-14):
# 12B fleet-safe default + 31B on-demand, RAM-gated so neither path Jetsams a sibling.
DOWNGRADE_NOTE=""
pick_model() {
  DOWNGRADE_NOTE=""
  if echo "$1" | grep -qiE "^MODEL:[[:space:]]*max|MODEL:[[:space:]]*31b|use the (max|big|large|31b) model"; then
    local f; f=$(free_gb)
    if python3 -c "import sys;sys.exit(0 if $f>=$MAX_MODEL_MIN_FREE_GB else 1)"; then
      echo "$MAX_MODEL"; return
    fi
    DOWNGRADE_NOTE="(requested the 31B max model, but only ${f}GB free < ${MAX_MODEL_MIN_FREE_GB}GB needed — ran on the fleet-safe default to avoid Jetsam-killing a live session. Free RAM and re-route with MODEL: max for the big model.)"
  fi
  echo "$DEFAULT_MODEL"
}

gen() {
  # $1 = full prompt, $2 = model (defaults to DEFAULT_MODEL); echoes the FINAL answer.
  #
  # Routes through `sirsi gemma` (Go broker), NOT raw mlx_lm.generate. This is the
  # fix for the 2026-07-03 swap-thrash incident: this worker used to shell mlx_lm
  # directly, cold-loading a fresh model copy on every call regardless of the warm
  # server at 127.0.0.1:11434 and regardless of how loaded the machine already was
  # (concurrent go test runs, multiple Claude/Codex sessions). `sirsi gemma` already
  # solves exactly this (ADR-031-A/B, born from the 2026-06-18 OOM incident): it
  # prefers the warm broker automatically, and its cold-path fallback is gated by
  # guard.NodeCapacity.Fits() — a real free-RAM + DynamicReserve(OS + live Claude/
  # Codex RSS) check — plus a machine-wide file lock so concurrent callers can never
  # stack multiple full model loads. Reuse that broker instead of reinventing a
  # weaker copy of it in bash. Prompt goes via stdin (safe for long item bodies —
  # no arg-length/quoting risk).
  #
  # gemma-4 is a reasoning builder; `sirsi gemma`'s own gemmaClean() already extracts
  # the final answer past the last <channel|> closer and strips control tokens, so
  # its stdout is normally ready to use as-is. The pipeline below is kept as a
  # harmless idempotent safety net (a no-op on already-clean text) in case a residual
  # scratchpad fragment ever slips through.
  local m="${2:-$DEFAULT_MODEL}"
  "$SIRSI" gemma --model "$m" --max-tokens "$MAXTOK" <<<"$1" 2>/dev/null \
    | grep -vE "^=+$|^Prompt:|^Generation:|^Peak memory:|^Fetching|<end_of_turn>|<start_of_turn>|^model$" \
    | python3 -c "
import sys,re
t=sys.stdin.read()
# Final-answer delimiters, in priority order (observed + harmony variants).
delims=['<channel|>','<|channel|>final','<channel|>final','<|message>','<|message|>']
final=None
for d in delims:
    if d in t:
        final=t.split(d)[-1]; break
if final is None:
    # No closer → model never finished thinking. Drop the opened thought block so we
    # don't emit a raw scratchpad; leave empty if there's nothing past it (triggers retry).
    if '<|channel>thought' in t:
        final=''   # incomplete reasoning, no final answer reached
    else:
        final=t    # plain (non-reasoning) output, pass through
# Remove residual control tokens that have a pipe on either side (keeps Go '<-chan').
final=re.sub(r'<\|[^>]*>|<[^>]*\|>','',final)
# Also strip bare special tokens some quants emit (<pad>/<eos>/<bos>/<unk>/turn markers).
final=re.sub(r'<(pad|eos|bos|unk|end_of_turn|start_of_turn)>','',final)
# If what's left is only whitespace/padding, return empty so the worker's retry fires.
if not final.strip() or set(final.split())<= {''}:
    final=''
sys.stdout.write(final.strip()+('\n' if final.strip() else ''))
"
}

looks_truncated() {
  # $1 = gen()'s extracted final answer. Catches the case gen() itself can't:
  # a closer WAS found (so gen() didn't return empty) but the text past it still
  # got cut off mid-deliverable — an odd code-fence count, or a stray control-token
  # fragment that slipped past the stripping regex. Evidence: the F9 incident
  # (2026-06-29) produced exactly this shape before gen()'s closer-detection existed.
  local out="$1"
  local fences
  fences=$(grep -o '```' <<<"$out" | wc -l | tr -d ' ')
  [ $(( fences % 2 )) -ne 0 ] && return 0
  echo "$out" | grep -qE '<\|[a-z_]*$|^[a-z_]*\|>' && return 0
  return 1
}

status_write() {
  # $1 = state (idle|processing), $2 = item id or "-". Zero-effort operator
  # visibility: `cat ~/.sirsi/gemma-status.json` or glance at it from the menubar,
  # instead of tailing logs to figure out whether gemma is alive and what it's doing.
  cat > "$HOME/.sirsi/gemma-status.json" <<EOF
{"state":"$1","item":"$2","model":"$DEFAULT_MODEL","pid":$$,"updated":"$(date -u +%FT%TZ)"}
EOF
}

needs_escalation() {
  # $1 = body, $2 = task mode.
  # An explicit safe task mode (plan/draft/summarize/analyze/extract/classify) is
  # inherently PRODUCE-TEXT-ONLY — Gemma drafts/reasons, it does not bind or act.
  # These NEVER escalate, even when the SUBJECT is security/signing/deploy. A plan
  # ABOUT a security deploy is fine; only being ASKED TO bind/act is not.
  case "$2" in
    plan|draft|summarize|analyze|extract|classify|build) return 1 ;;
  esac
  # General/no-directive mode only: escalate when the body DIRECTS Gemma to issue a
  # verdict or take a tool action (imperative phrasing), not when it merely mentions
  # the topic. Anchored to instruction verbs aimed at the worker.
  echo "$1" | grep -qiE "(issue|give|provide|render).{0,20}(binding|verdict|sign-?off)|(approve|merge|deploy|push|land) (the |this )?(pr|change|branch|build)|is this (safe|secure|correct|exploitable)|sign off on" && return 0
  return 1
}

process_item() {
  local id=$1
  # Read through the facade (`router show`), not the raw items/ file — under
  # StoreWake an item can exist only in the store. Same frontmatter+body format.
  local f; f=$(mktemp -t gemma-item)
  (cd "$REPO" && "$SIRSI" router show "$id" 2>/dev/null) > "$f"
  [ -s "$f" ] || { rm -f "$f"; return 0; }
  local t_start=$(date +%s)
  status_write "processing" "$id"

  # Extract from + instructions body
  local from body task
  from=$(awk '/^---$/{n++; if(n==2)exit; next} n==1 && /^from:/{gsub(/^from:[[:space:]]*"?|"?$/,""); print; exit}' "$f")
  body=$(awk 'f{print} /## Instructions/{f=1}' "$f" | sed '/## Result/,$d')
  task=$(echo "$body" | grep -oiE "^TASK:[[:space:]]*(classify|summarize|draft|analyze|plan|extract|build)" | head -1 | sed -E 's/^TASK:[[:space:]]*//I' | tr A-Z a-z)

  local result_file
  result_file=$(mktemp -t gemma-result)

  # Gemma NEVER refuses (owner directive 2026-06-12). It ALWAYS produces the best
  # deliverable it can. When the ask reaches for a binding verdict / security
  # sign-off / tool action, Gemma still does its part (the draft / analysis /
  # plan / recommendation) and APPENDS a "VERIFICATION REQUIRED" flag for
  # claude-home — it flags, it does not decline.
  local flag_for_review=""
  needs_escalation "$body" "$task" && flag_for_review="yes"

  local instruction
  case "$task" in
    classify)  instruction="Classify the following work item and explain your reasoning in 2-3 sentences." ;;
    summarize) instruction="Summarize the following in 5 bullet points capturing the key decisions, actions, and open questions." ;;
    draft)     instruction="Draft the requested text. Be concise and concrete. Output only the draft." ;;
    analyze)   instruction="Analyze the following carefully. Identify the core problem, the constraints, the options, and a recommendation. Think step by step." ;;
    plan)      instruction="Decompose the following into a concrete step-by-step plan. For each step give: what to do, why, and what 'done' looks like. This plan will be executed by a real agent with tools — be specific and ordered." ;;
    extract)   instruction="Extract every action item, decision, owner, and deadline from the following as a structured list." ;;
    build)     instruction="Complete the requested build. The task embeds ALL source you need — you cannot open files. Output ONLY the complete file(s)/code requested, ready to save and run: no prose, no explanation, no placeholder elisions. For multiple files, precede each with a line '=== FILE: <path> ==='. If the task references a file whose content is NOT embedded, do your best from what IS embedded and add one final line 'MISSING-SOURCE: <path>' so the router can re-scope." ;;
    *)         instruction="Complete the following request thoroughly and concretely. Produce a full, useful text deliverable. If it ultimately requires a tool action or a binding sign-off you cannot perform, do ALL the analysis/drafting toward it anyway and clearly mark what a human/claude-home still needs to confirm — never refuse or return empty." ;;
  esac

  local prompt="You are Gemma, a local AI worker in the Sirsi multi-agent system. You run on-device and produce text deliverables. ALWAYS produce your best, complete attempt — never refuse, never return an empty or 'I can't' response. If a task ultimately needs a binding verdict or a tool action, still deliver the full draft/analysis/plan and note what claude-home must confirm; that note does NOT replace doing the work.
CREDENTIAL SAFETY (AGENTS.md, established 2026-06-30 after a hardcoded-admin-credential incident): if a task seems to call for a login/credential/API key/secret value in code, NEVER write a literal value. Draft it as an environment-variable or secret-manager REFERENCE instead (e.g. process.env.X, a Secret Manager lookup) and explicitly flag in your deliverable that a real secret must be provisioned by claude-home/the owner. This satisfies 'never refuse' without shipping a vulnerability. ${instruction}

--- WORK ITEM (from ${from}) ---
${body}
--- END ---

Your response:"
  # Hybrid model select: default 12B (fleet-safe) unless body requests MODEL: max
  # AND free RAM can take the 31B without overcommitting the fleet.
  local model; model=$(pick_model "$body")
  local out
  out=$(gen "$prompt" "$model")
  # Retry once with a more forceful prompt if the model returned nothing, a refusal,
  # or a closer-found-but-still-cut-off answer (looks_truncated). The retry explicitly
  # forbids scratchpad output — the F9 incident (2026-06-29) burned its whole budget on
  # the reasoning channel and never reached the answer; telling it to skip that channel
  # entirely gives the retry a real shot instead of repeating the same failure mode.
  if [ -z "$out" ] || echo "$out" | grep -qiE "^\s*(i (can\047t|cannot|am unable|won\047t)|as an ai|i'm sorry|sorry,? i)" || looks_truncated "$out"; then
    out=$(gen "Produce a concrete, complete text deliverable for this request. Do not include any internal reasoning, thinking, or scratchpad in your response — output ONLY the final deliverable text, nothing else. Do not refuse, do not caveat about being an AI — just do the work as far as text can take it.

${body}

Deliverable:" "$model")
  fi
  local truncated_after_retry=""
  if [ -z "$out" ]; then
    out="(model returned empty after retry — claude-home should re-scope; Gemma did not refuse, the prompt likely needs more context)"
  elif looks_truncated "$out"; then
    truncated_after_retry="yes"
  fi

  cat > "$result_file" <<EOF
COMPLETED by local Gemma worker (zero API tokens, on-device).
Model: ${model##*/}   Task mode: ${task:-general}
EOF
  [ -n "$DOWNGRADE_NOTE" ] && echo "Note: $DOWNGRADE_NOTE" >> "$result_file"
  cat >> "$result_file" <<EOF

${out}
EOF
  if [ "$flag_for_review" = "yes" ]; then
    cat >> "$result_file" <<EOF

⚑ VERIFICATION REQUIRED — this deliverable reaches toward a binding decision
(verdict / security sign-off / tool action). Gemma did the drafting/analysis;
claude-home must verify before anything binding ships. (Gemma flags, never refuses.)
EOF
  fi
  if [ "$truncated_after_retry" = "yes" ]; then
    flag_for_review="yes"
    cat >> "$result_file" <<EOF

⚑ LOOKS TRUNCATED — this deliverable still shows signs of a cut-off generation
after one retry (unbalanced code fence or a residual control-token fragment).
Treat it as a draft to verify, not a finished answer — re-run with MODEL: max
or a narrower scope if it's missing content you expected.
EOF
  fi
  cat >> "$result_file" <<EOF

---
Local-model deliverable. If it feeds a binding decision, claude-home verifies
before it ships — Gemma is the drafting/reasoning layer, not the verdict authority.

— gemma (local worker, $(date -u +%FT%TZ))
EOF
  local t_end=$(date +%s)
  log "COMPLETED $id (from $from, task=${task:-general}, flagged=${flag_for_review:-no}, duration=$((t_end - t_start))s, out_chars=${#out})"

  # RELAY (Rule: a request requires a routed response). Route the deliverable BACK to
  # the requester as a FRESH inbound item so their watcher wakes — close+Result is
  # audit-only and never wakes anyone. Without this, gemma's drafts sit unseen.
  local title
  title=$(awk '/^---$/{n++; if(n==2)exit; next} n==1 && /^title:/{gsub(/^title:[[:space:]]*"?|"?$/,""); print; exit}' "$f")
  if [ -n "$from" ] && [ "$from" != "gemma" ]; then
    (cd "$REPO" && "$SIRSI" router send --from gemma --to "$from" --type review \
      --title "gemma deliverable: ${title:-$id}" \
      --instructions "@$result_file" >/dev/null 2>&1) \
      && log "routed reply -> $from for $id" || log "reply-route FAILED $id"
  fi

  # Close the original with a short pointer (full deliverable lives in the routed reply).
  (cd "$REPO" && "$SIRSI" router close "$id" \
    --result "Completed by gemma; full deliverable routed back to ${from} as a review item. (Model: ${model##*/}, task=${task:-general}, flagged=${flag_for_review:-no})" >/dev/null 2>&1) \
    && log "closed $id" || log "close FAILED $id"
  rm -f "$result_file" "$f"
}

log "gemma-worker started (poll=${POLL}s, model=$DEFAULT_MODEL)"
echo $$ > "$HOME/.sirsi/gemma-worker.pid"
trap 'rm -f "$HOME/.sirsi/gemma-worker.pid"' EXIT
status_write "idle" "-"

while true; do
  # Pull open gemma items through the router FACADE (`router pull`), never by
  # globbing items/*.md: dispatch/facade.go stops writing item files when
  # StoreWake flips (SIRSI_ROUTER_STORE_WAKE) — a file-globbing worker would go
  # silently blind that day. `pull` reads store ∪ files. (Spec-fix Gap 2.)
  ids=$(cd "$REPO" && "$SIRSI" router pull gemma 2>/dev/null | awk '/^  • /{print $2}')
  if [ -n "$ids" ]; then
    while IFS= read -r id; do
      [ -z "$id" ] && continue
      log "processing $id"
      process_item "$id"
    done <<< "$ids"
    status_write "idle" "-"
  fi
  sleep "$POLL"
done
