#!/bin/bash
# triage-itemlist.test.sh — pins how sirsi-gemma-triage.sh SOURCES its work list.
#
#   bash scripts/gemma/triage-itemlist.test.sh
#
# Hermetic: stubs `sirsi router dump --json` with fixture JSONL, so it never
# touches the real store and never calls Gemma.
#
# Why this file exists. The triage screen used to walk the legacy
# .agents/idea-router/items directory and parse frontmatter with regexes. That
# directory stopped being written at the ADR-036/037 store-only cutover, so it
# silently matched ZERO items while seven were genuinely open -- and it reported
# that as "(no open items)", exit 0. A broken screen was indistinguishable from a
# healthy empty queue, and the conduit runs this FIRST. The tests below pin the
# two properties that failure violated: read from the store, and never claim
# "empty" when the source is unreadable.
set -u
TRIAGE=${TRIAGE:-"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sirsi-gemma-triage.sh"}
PASS=0; FAIL=0
WORK=$(mktemp -d -t triagetest); trap 'rm -rf "$WORK"' EXIT
# Point the full-script runs at a dead endpoint so server_up() is false and the
# live calibration probe is skipped entirely. These cases exercise the item-list
# source, not Gemma; without this the suite depends on a warm broker and pays
# ~4 min of probe per invocation.
DEAD_SERVER="http://127.0.0.1:9/v1/chat/completions"

check() { # $1=label $2=expected $3=actual
  if [ "$2" = "$3" ]; then echo "  ok   $1"; PASS=$((PASS+1))
  else echo "  FAIL $1"; echo "         expected: $2"; echo "         actual:   $3"; FAIL=$((FAIL+1)); fi
}

# Extract just the item-list program from the script, so the test exercises the
# REAL selection logic rather than a restatement of it that could drift.
sed -n "/^import os, sys, json$/,/^    print(f\"{d.get('id','')}/p" "$TRIAGE" > "$WORK/itemlist.py"
[ -s "$WORK/itemlist.py" ] || { echo "FAIL: could not extract item-list block from $TRIAGE"; exit 1; }

FIXTURE=$(cat <<'JSONL'
{"id":"a-open-pantheon","to":"claude-pantheon","status":"open","title":"open one","body":"first body"}
{"id":"b-closed-pantheon","to":"claude-pantheon","status":"closed","title":"closed one","body":"nope"}
{"id":"c-open-nexus","to":"claude-nexus","status":"open","title":"open two","body":"second body"}
JSONL
)

echo "item selection:"

check "open items only (closed excluded)" \
  "a-open-pantheon c-open-nexus" \
  "$(FLT= python3 "$WORK/itemlist.py" <<<"$FIXTURE" | cut -f1 | tr '\n' ' ' | sed 's/ $//')"

check "recipient filter narrows to one" \
  "c-open-nexus" \
  "$(FLT=claude-nexus python3 "$WORK/itemlist.py" <<<"$FIXTURE" | cut -f1)"

check "emits the 4-field TSV contract (id/to/title/snippet)" \
  "a-open-pantheon|claude-pantheon|open one|first body" \
  "$(FLT=claude-pantheon python3 "$WORK/itemlist.py" <<<"$FIXTURE" | head -1 | tr '\t' '|')"

# A multi-line body must not split into extra TSV rows — the caller reads this
# with `while IFS=$'\t' read -r id to title snippet`.
check "multi-line body collapses to one row" \
  "1" \
  "$(FLT= python3 "$WORK/itemlist.py" <<<'{"id":"m","to":"x","status":"open","title":"t","body":"line one\nline two\nline three"}' | wc -l | tr -d ' ')"

# FAIL CLOSED on a malformed record. Skipping it hides an open item we could not
# read, and if nothing else matches the script would then print "(no open items)"
# — a fabricated all-clear assembled from an unparseable source. The parser must
# abort instead, so the caller sees a broken screen rather than a green one.
malformed_out=$(FLT= python3 "$WORK/itemlist.py" <<<'not json
{"id":"good","to":"x","status":"open","title":"t","body":"b"}' 2>&1); malformed_rc=$?

check "malformed JSONL line is FATAL, not skipped" \
  "nonzero" \
  "$([ "$malformed_rc" -ne 0 ] && echo nonzero || echo "zero($malformed_rc)")"

check "malformed JSONL emits no item rows" \
  "absent" \
  "$(echo "$malformed_out" | grep -q 'good' && echo present || echo absent)"

echo
echo "silent-failure guard:"

# THE REGRESSION. An unreadable store must NOT be reported as an empty queue.
# Stub sirsi with a binary that produces nothing, and assert the script refuses
# to print the all-clear string and exits non-zero.
printf '#!/bin/sh\nexit 1\n' > "$WORK/sirsi-dead"; chmod +x "$WORK/sirsi-dead"
out=$(GEMMA_SERVER="$DEAD_SERVER" SIRSI_BIN="$WORK/sirsi-dead" bash "$TRIAGE" --all 2>&1); rc=$?

check "unreadable store exits non-zero" "nonzero" "$([ "$rc" -ne 0 ] && echo nonzero || echo "zero($rc)")"
check "unreadable store does NOT claim an empty queue" \
  "absent" \
  "$(echo "$out" | grep -q 'no open items' && echo present || echo absent)"

# THE SECOND FAIL-OPEN (codex-pantheon review, 2026-07-24). A dump that dies
# PARTWAY still writes the records it got to before failing, so stdout is
# non-empty and an empty-string check waves the partial read through as
# complete. Only the exit status distinguishes them — assert on it.
printf '#!/bin/sh\nprintf %%s\\\\n %s\nexit 1\n' \
  "'{\"id\":\"partial\",\"to\":\"claude-pantheon\",\"status\":\"closed\",\"title\":\"t\",\"body\":\"b\"}'" \
  > "$WORK/sirsi-partial"; chmod +x "$WORK/sirsi-partial"
out=$(GEMMA_SERVER="$DEAD_SERVER" SIRSI_BIN="$WORK/sirsi-partial" bash "$TRIAGE" --all 2>&1); rc=$?

check "partial dump (output + nonzero exit) exits non-zero" \
  "nonzero" "$([ "$rc" -ne 0 ] && echo nonzero || echo "zero($rc)")"
check "partial dump does NOT claim an empty queue" \
  "absent" \
  "$(echo "$out" | grep -q 'no open items' && echo present || echo absent)"

# End-to-end twin of the parser test: a corrupt record reaching the full script
# must break the run, never degrade into the all-clear message.
printf '#!/bin/sh\nprintf %%s\\\\n %s\nexit 0\n' "'not json'" > "$WORK/sirsi-corrupt"
chmod +x "$WORK/sirsi-corrupt"
out=$(GEMMA_SERVER="$DEAD_SERVER" SIRSI_BIN="$WORK/sirsi-corrupt" bash "$TRIAGE" --all 2>&1); rc=$?

check "corrupt JSONL exits non-zero end-to-end" \
  "nonzero" "$([ "$rc" -ne 0 ] && echo nonzero || echo "zero($rc)")"
check "corrupt JSONL does NOT claim an empty queue" \
  "absent" \
  "$(echo "$out" | grep -q 'no open items' && echo present || echo absent)"

# The counterweight: a HEALTHY dump with genuinely nothing open must still
# report the empty queue and exit 0. Fail-closed must not become fail-always.
printf '#!/bin/sh\nprintf %%s\\\\n %s\nexit 0\n' \
  "'{\"id\":\"z\",\"to\":\"claude-pantheon\",\"status\":\"closed\",\"title\":\"t\",\"body\":\"b\"}'" \
  > "$WORK/sirsi-empty"; chmod +x "$WORK/sirsi-empty"
out=$(GEMMA_SERVER="$DEAD_SERVER" SIRSI_BIN="$WORK/sirsi-empty" bash "$TRIAGE" --all 2>&1); rc=$?

check "healthy dump with no open items exits zero" \
  "zero" "$([ "$rc" -eq 0 ] && echo zero || echo "nonzero($rc)")"
check "healthy dump with no open items DOES report the empty queue" \
  "present" \
  "$(echo "$out" | grep -q 'no open items' && echo present || echo absent)"

echo
echo "passed=$PASS failed=$FAIL"
[ "$FAIL" -eq 0 ]
