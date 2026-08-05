#!/bin/bash
# model-resolver-cache.test.sh — pins the conf-WRITE decision of
# sirsi-gemma-model-resolver.sh. Run after touching the is_cached block.
#
#   bash scripts/gemma/model-resolver-cache.test.sh
#
# Regression guard for the empty-cache defect (router 20260803-215612): when
# NEITHER the chosen model NOR the fallback is cached, the old
# `! is_cached MODEL && is_cached FALLBACK` guard fell through and wrote the
# UNCACHED chosen model to conf — poisoning conf with a model nobody has.
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

run_write() { # $1=which models to seed cache with ("model"|"fallback"|"none") → prints "<exit>|<conf>"
  local hub conf tmp rc
  hub=$(mktemp -d "${TMPDIR:-/tmp}/gemmacache.XXXXXX"); conf=$(mktemp "${TMPDIR:-/tmp}/gemmaconf.XXXXXX")
  echo "SENTINEL-UNCHANGED" > "$conf"     # detect an untouched conf
  case "$1" in
    model)    mkdir -p "$hub/$(cache_dir "$MODEL_ID")";;
    fallback) mkdir -p "$hub/$(cache_dir "$FB_ID")";;
    none)     : ;;                          # empty cache — the defect case
  esac
  tmp=$(mktemp "${TMPDIR:-/tmp}/gemmawrite.XXXXXX")
  { echo "HF_CACHE='$hub'; MODEL='$MODEL_ID'; FALLBACK='$FB_ID'; CONF='$conf'; log(){ :; }"
    printf '%s\n' "$BLOCK"
  } > "$tmp"
  bash "$tmp"; rc=$?
  printf '%s|%s\n' "$rc" "$(cat "$conf")"
  rm -rf "$hub" "$conf" "$tmp"
}

check() { # $1=label $2=expected $3=actual
  if [ "$2" = "$3" ]; then echo "  ok   $1"; PASS=$((PASS+1))
  else echo "  FAIL $1"; echo "         expected: $2"; echo "         actual:   $3"; FAIL=$((FAIL+1)); fi
}

echo "conf-write decision:"

check "chosen model cached → conf names the model, exit 0" \
  "0|$MODEL_ID" "$(run_write model)"

check "only fallback cached → conf serves fallback, exit 0" \
  "0|$FB_ID" "$(run_write fallback)"

# THE REGRESSION. Neither cached: must REFUSE — conf untouched, non-zero exit.
check "empty cache → conf UNCHANGED and exit non-zero (no uncached model written)" \
  "1|SENTINEL-UNCHANGED" "$(run_write none)"

echo
echo "passed=$PASS failed=$FAIL"
[ "$FAIL" -eq 0 ]
