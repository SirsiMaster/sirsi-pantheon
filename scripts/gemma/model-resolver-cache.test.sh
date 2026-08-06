#!/bin/bash
# model-resolver-cache.test.sh — pins the conf-WRITE decision of
# sirsi-gemma-model-resolver.sh. Run after touching the is_cached block.
#
#   bash scripts/gemma/model-resolver-cache.test.sh
#
# Regression guard for the empty-cache WEDGE (ADR-031, router 20260803-215612):
# when neither the chosen model nor the fallback is cached the resolver must NOT
# write an uncached model to conf. The WEDGE contract:
#   - model cached           → write MODEL, exit 0
#   - only fallback cached   → write FALLBACK, exit 0
#   - neither cached, conf exists → PRESERVE conf unchanged, exit 0
#   - neither cached, no conf    → write FALLBACK as sentinel, exit 0
#
# Like the ranking test, this EXTRACTS the real decision block from the live
# script (a restated copy would drift and pass while the real thing was broken)
# and runs it hermetically against a fake HF cache dir.
set -u
RESOLVER=${RESOLVER:-"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/sirsi-gemma-model-resolver.sh"}
PASS=0; FAIL=0

# Pull the write-decision block: from the is_cached() definition to its `fi`.
BLOCK=$(awk '/^is_cached\(\) \{/{f=1} f{print} f&&/^fi$/{exit}' "$RESOLVER")
[ -z "$BLOCK" ] && { echo "FAIL: could not extract is_cached block from $RESOLVER"; exit 1; }

MODEL_ID="mlx-community/gemma-4-31B-it-qat-4bit"
FB_ID="mlx-community/gemma-4-12B-it-8bit"
cache_dir() { echo "models--$(echo "$1" | sed 's/\//--/')"; }

# run_write: $1=which models to seed cache with ("model"|"fallback"|"none")
#            $2=conf initial state ("absent"|any string to pre-seed)
# prints "<exit>|<conf_content>"
run_write() {
  local hub conf tmp rc seed_conf="$2"
  hub=$(mktemp -d "${TMPDIR:-/tmp}/gemmacache.XXXXXX")
  conf=$(mktemp "${TMPDIR:-/tmp}/gemmaconf.XXXXXX")
  case "$seed_conf" in
    absent) rm -f "$conf";;
    *)      echo "$seed_conf" > "$conf";;
  esac
  case "$1" in
    model)    mkdir -p "$hub/$(cache_dir "$MODEL_ID")";;
    fallback) mkdir -p "$hub/$(cache_dir "$FB_ID")";;
    none)     :;;
  esac
  tmp=$(mktemp "${TMPDIR:-/tmp}/gemmawrite.XXXXXX")
  { echo "HF_CACHE='$hub'; MODEL='$MODEL_ID'; FALLBACK='$FB_ID'; CONF='$conf'; WEDGE=0; log(){ :; }"
    printf '%s\n' "$BLOCK"
  } > "$tmp"
  bash "$tmp"; rc=$?
  local content
  content=$([ -f "$conf" ] && cat "$conf" || echo "ABSENT")
  printf '%s|%s\n' "$rc" "$content"
  rm -rf "$hub" "$tmp"
  [ -f "$conf" ] && rm -f "$conf"
}

check() {
  if [ "$2" = "$3" ]; then echo "  ok   $1"; PASS=$((PASS+1))
  else echo "  FAIL $1"; echo "         expected: $2"; echo "         actual:   $3"; FAIL=$((FAIL+1)); fi
}

echo "conf-write decision (WEDGE contract):"

check "chosen model cached → conf names the model, exit 0" \
  "0|$MODEL_ID" "$(run_write model absent)"

check "only fallback cached → conf serves fallback, exit 0" \
  "0|$FB_ID" "$(run_write fallback absent)"

# WEDGE cases: neither cached — must gracefully degrade, never write an uncached model.
check "empty cache + no conf → write FALLBACK sentinel, exit 0" \
  "0|$FB_ID" "$(run_write none absent)"

check "empty cache + existing conf → preserve conf unchanged, exit 0" \
  "0|PREVIOUS-CONF" "$(run_write none PREVIOUS-CONF)"

echo
echo "passed=$PASS failed=$FAIL"
[ "$FAIL" -eq 0 ]
