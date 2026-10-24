#!/usr/bin/env bash
# Both directions of the exact-head, exact-sender, per-rejection clearance.
# Pure fixtures: no credentials, router, cryptography or network.
set -uo pipefail
JQ="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/router-rejection.jq"
REPO="SirsiMaster/sirsi-pantheon"
HEAD="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
OLD="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
fails=0
check() { # <desc> <pr> <want-id-or-empty> <jsonl>
  local got
  if ! got="$(printf '%s\n' "$4" | jq -sr --arg repo "$REPO" --arg pr "$2" --arg head "$HEAD" -f "$JQ")"; then
    echo "FAIL — $1 (filter error)"; fails=$((fails+1)); return
  fi
  if [ "$got" = "$3" ]; then echo "ok   — $1"; else echo "FAIL — $1 (want '$3', got '$got')"; fails=$((fails+1)); fi
}
it() { # id canonical-from title body; equal timestamps deliberately have no authority
  jq -cn --arg id "$1" --arg from "$2" --arg title "$3" --arg body "$4" \
    '{id:$id,from:$from,title:$title,body:$body,type:"review",opened:"2026-10-23T00:00:00Z"}'
}
decl() { # verdict repo head rejection-ids-or-dash
  printf '```sirsi-bind-verdict-v1\nrepo: %s\npr: 927\nhead: %s\nverdict: %s\nresolves: %s\n```' "$2" "$3" "$1" "$4"
}
r1="$(it r1 codex-pantheon 'PR927 CHANGES REQUIRED' 'Original finding')"
r2="$(it r2 claude-pantheon 'PR927 REJECT' 'Independent unresolved finding')"
good="$(decl ACCEPT "$REPO" "$HEAD" r1)"
check "historical rejection blocks" 927 r1 "$r1"
check "title-only accept never clears" 927 r1 "$r1
$(it a1 codex-pantheon 'PR927 exact head ACCEPT' '')"
check "SHA mentioned as not reviewed never clears" 927 r1 "$r1
$(it a1 codex-pantheon 'PR927 PASS baseline' "Current head $HEAD was NOT reviewed; only $OLD was accepted.")"
check "stale declared head never clears" 927 r1 "$r1
$(it a1 codex-pantheon 'PR927 ACCEPT' "$(decl ACCEPT "$REPO" "$OLD" r1)")"
check "same sender exact head and rejection clears" 927 "" "$r1
$(it a1 codex-pantheon 'PR927 ACCEPT' "$good")"
check "different sender cannot clear" 927 r1 "$r1
$(it a1 claude-pantheon 'PR927 ACCEPT' "$good")"
check "wrong exact rejection id never clears" 927 r1 "$r1
$(it a1 codex-pantheon 'PR927 ACCEPT' "$(decl ACCEPT "$REPO" "$HEAD" other-id)")"
check "wrong accepted repo never clears" 927 r1 "$r1
$(it a1 codex-pantheon 'PR927 ACCEPT' "$(decl ACCEPT SirsiMaster/other "$HEAD" r1)")"
check "each reviewer retains unresolved hold" 927 r2 "$r1
$r2
$(it a1 codex-pantheon 'PR927 ACCEPT' "$good")"
check "all unresolved ids are retained" 927 'r1, r2' "$r1
$r2"
check "each sender may resolve their own exact item" 927 "" "$r1
$r2
$(it a1 codex-pantheon 'PR927 ACCEPT' "$good")
$(it a2 claude-pantheon 'PR927 ACCEPT' "$(decl ACCEPT "$REPO" "$HEAD" r2)")"
check "one sender may resolve multiple own findings" 927 "" "$r1
$(it r3 codex-pantheon 'PR927 REJECT' '')
$(it a1 codex-pantheon 'PR927 ACCEPT' "$(decl ACCEPT "$REPO" "$HEAD" r1,r3)")"
check "resolution does not clear another rejection" 927 r3 "$r1
$(it r3 codex-pantheon 'PR927 REJECT' '')
$(it a1 codex-pantheon 'PR927 ACCEPT' "$good")"
check "known different repo rejection does not block" 927 "" "$(it r1 codex-pantheon 'PR927 REJECT' "$(decl REJECT SirsiMaster/other "$HEAD" -)")"
check "another PR does not block" 927 "" "$(it r1 codex-pantheon 'PR9270 REJECT' '')"
check "no verdict does not create a hold" 927 "" "$(it q1 codex-pantheon 'PR927 question' '')"
check "unknown rejection sender cannot be cleared" 927 r1 "$(it r1 '' 'PR927 REJECT' '')
$(it a1 '' 'PR927 ACCEPT' "$good")"
check "missing resolution identity cannot clear" 927 r1 "$r1
$(it '' codex-pantheon 'PR927 ACCEPT' "$good")"
check "whitespace sender is not authenticated custody" 927 r1 "$(it r1 ' ' 'PR927 REJECT' '')
$(it a1 ' ' 'PR927 ACCEPT' "$good")"
check "case-insensitive declared repo and full SHA qualify" 927 "" "$r1
$(it a1 codex-pantheon 'PR927 ACCEPT' "$(decl ACCEPT sirsimaster/SIRSI-PANTHEON AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA r1)")"

for bad in \
  "$(printf '%s\n' "$good" | sed '$d')" \
  "$good
$good" \
  "$(printf '%s\n' "$good" | sed 's/verdict: ACCEPT/verdict: PASS/')" \
  "$(printf '%s\n' "$good" | sed '/verdict: ACCEPT/a\
extra: unrecognized')" \
  "$(printf '%s\n' "$good" | sed "s/$HEAD/shortsha/")" \
  "$(decl ACCEPT "$REPO" "$HEAD" -)"; do
  check "malformed/ambiguous metadata cannot clear" 927 r1 "$r1
$(it a1 codex-pantheon 'PR927 ACCEPT' "$bad")"
done
check "contradictory rejection title cannot scope away a hold" 927 r1 "$(it r1 codex-pantheon 'PR927 REJECT' "$(decl ACCEPT SirsiMaster/other "$HEAD" r1)")"
exit "$fails"
