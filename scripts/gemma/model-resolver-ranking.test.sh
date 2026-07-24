#!/bin/bash
# test-gemma-model-resolver.sh — pins the RANKING invariants of
# sirsi-gemma-model-resolver.sh. Run after touching the candidate sort.
#
#   bash ~/.local/bin/test-gemma-model-resolver.sh
#
# The ranking is inline python inside the resolver, so this test EXTRACTS that
# block from the live script rather than restating it — a copy would drift and
# then pass while the real thing was broken. Feeds synthetic HF API payloads and
# asserts which model wins.
set -u
# Default to the resolver sitting NEXT TO this test (repo copy, so CI checks what
# is versioned). Override with RESOLVER=... to point at an installed copy.
RESOLVER=${RESOLVER:-"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sirsi-gemma-model-resolver.sh"}
PASS=0; FAIL=0

# Pull the ranking block: from the CHOSEN= heredoc opener to its closing ").
BLOCK=$(awk '/^  CHOSEN=\$\(echo "\$json" \| python3 -c "$/{f=1;next} f&&/^"\)$/{exit} f' "$RESOLVER")
[ -z "$BLOCK" ] && { echo "FAIL: could not extract ranking block from $RESOLVER"; exit 1; }

run_rank() { # $1=budget  $2=json payload  [$3=resident model] → prints winning model id
  # The block must run as a FILE, not on stdin: the ranking reads the payload from
  # stdin itself, so piping the code in too makes them fight (silent empty result).
  # Neutralize the on-disk cache probes so the test is hermetic — `cached()` and the
  # local-seed listdir must not depend on this machine's real HF cache.
  local tmp; tmp=$(mktemp -t gemmarank)
  printf '%s\n' "$BLOCK" \
    | sed "s|^budget=.*|budget=$1|" \
    | sed "s|^resident=.*|resident='${3:-}'|" \
    | sed "s|^HUB=.*|HUB='/nonexistent-hub-for-test'|" > "$tmp"
  python3 "$tmp" <<<"$2" | cut -f1
  rm -f "$tmp"
}

payload() { # each arg is a model id
  local out="[" first=1
  for m in "$@"; do
    [ $first -eq 0 ] && out="$out,"
    out="$out{\"id\":\"$m\"}"; first=0
  done
  echo "$out]"
}

check() { # $1=label $2=expected $3=actual
  if [ "$2" = "$3" ]; then echo "  ok   $1"; PASS=$((PASS+1))
  else echo "  FAIL $1"; echo "         expected: $2"; echo "         actual:   $3"; FAIL=$((FAIL+1)); fi
}

echo "quoting safety (the block runs inside a double-quoted bash string):"

# TEST 0 — the trap that made every other test USELESS on 2026-07-24. The ranking
# block is embedded as `python3 -c "<block>"`, so a double quote inside it ENDS the
# bash string and silently truncates the code; backticks and dollar-paren execute as
# shell. The resolver then logs "no fitting candidate" as if the API were down.
# The other tests read the block straight off disk and so never saw it — they passed
# green against a script that could not produce a candidate at all. This test reads
# the block the same way but checks for the characters bash would act on, which is
# what actually distinguishes a working script from a broken one.
# `$budget` is the ONE intentional expansion (the script substitutes the RAM budget).
BAD=$(printf '%s\n' "$BLOCK" | grep -n '"\|`\|\$(' || true)
if [ -z "$BAD" ]; then
  echo "  ok   no unescaped double quote / backtick / dollar-paren in ranking block"
  PASS=$((PASS+1))
else
  echo "  FAIL shell-active characters in ranking block (bash will truncate or execute):"
  printf '%s\n' "$BAD" | sed 's/^/         /'
  FAIL=$((FAIL+1))
fi

echo
echo "ranking invariants:"

# 1. With NOTHING resident, mxfp8 and 8bit are both 8-bit so the qat tiebreak
#    applies and qat-mxfp8 wins. This is the correct cold-start pick.
check "qat-mxfp8 beats plain 8bit when nothing is resident (qat breaks the tie)" \
  "mlx-community/gemma-4-12B-it-qat-mxfp8" \
  "$(run_rank 18.0 "$(payload mlx-community/gemma-4-12B-it-qat-mxfp8 mlx-community/gemma-4-12B-it-8bit)")"

# 1b. THE CHURN FIX. Same two models, but 8bit is already being served. Resident
#     outranks qat, so the pick STAYS PUT — no conf/resident split, no second 12B
#     loaded by the probe, no broker bounce. This is the case that ran ~2x/hr.
check "resident 8bit HOLDS against qat-mxfp8 (hysteresis suppresses a lateral swap)" \
  "mlx-community/gemma-4-12B-it-8bit" \
  "$(run_rank 18.0 "$(payload mlx-community/gemma-4-12B-it-qat-mxfp8 mlx-community/gemma-4-12B-it-8bit)" mlx-community/gemma-4-12B-it-8bit)"

# 1c. Hysteresis must NOT freeze out a genuine upgrade. A resident 12B loses to a
#     31B that fits, because params outrank the resident flag.
check "resident 12B still LOSES to a 31B that fits (real upgrades still swap)" \
  "mlx-community/gemma-4-31B-it-qat-4bit" \
  "$(run_rank 40.0 "$(payload mlx-community/gemma-4-12B-it-8bit mlx-community/gemma-4-31B-it-qat-4bit)" mlx-community/gemma-4-12B-it-8bit)"

# 2. THE SECOND-ORDER REGRESSION. Fixing (1) by special-casing 8 bits let
#    qat-6bit win instead. Bits must dominate qat at EVERY depth, not just 8.
check "8-bit plain beats qat-6bit (more bits wins over qat)" \
  "mlx-community/gemma-4-12B-it-8bit" \
  "$(run_rank 18.0 "$(payload mlx-community/gemma-4-12B-it-qat-6bit mlx-community/gemma-4-12B-it-8bit)")"

# 3. QAT IS STILL PREFERRED where it is meaningful: between EQUAL-bit variants.
check "qat-4bit beats plain 4bit (equal bits, qat breaks the tie)" \
  "mlx-community/gemma-4-12B-it-qat-4bit" \
  "$(run_rank 18.0 "$(payload mlx-community/gemma-4-12B-it-4bit mlx-community/gemma-4-12B-it-qat-4bit)")"

# 4. Params still dominate bits — a bigger model that fits wins over a smaller
#    higher-bit one. Budget is generous so both fit.
check "31B 4bit beats 12B 8bit when both fit (params outrank bits)" \
  "mlx-community/gemma-4-31B-it-qat-4bit" \
  "$(run_rank 40.0 "$(payload mlx-community/gemma-4-12B-it-8bit mlx-community/gemma-4-31B-it-qat-4bit)")"

# 5. Budget is respected — the 31B must be excluded on the real 18GB budget.
check "31B excluded at the 18GB steady-state budget" \
  "mlx-community/gemma-4-12B-it-8bit" \
  "$(run_rank 18.0 "$(payload mlx-community/gemma-4-31B-it-qat-4bit mlx-community/gemma-4-12B-it-8bit)")"

# 6. Newer family wins regardless of size/bits.
check "newer family outranks older" \
  "mlx-community/gemma-4-12B-it-8bit" \
  "$(run_rank 18.0 "$(payload mlx-community/gemma-3-12B-it-8bit mlx-community/gemma-4-12B-it-8bit)")"

echo
echo "passed=$PASS failed=$FAIL"
[ "$FAIL" -eq 0 ]
