#!/usr/bin/env bash
# Asserts sirsi-bind.sh can never post a blocking verdict as an APPROVE review.
#
# Regression: the script hardcoded `event=APPROVE`, so a bind whose body opened
# "CHANGES REQUESTED" was recorded APPROVED and its gate re-run cleared binding-hold
# on the PR it had just blocked (observed live on sirsi-pantheon PR #416).
#
# These cases all short-circuit before the App-key check, so no credentials or
# network are needed.
set -uo pipefail

BIND="$(cd "$(dirname "$0")" && pwd)/sirsi-bind.sh"
fails=0

check() { # <desc> <expected-exit> <args...>
  local desc="$1" want="$2"; shift 2
  local out; out="$("$BIND" "$@" 2>&1)"; local got=$?
  if [ "$got" = "$want" ]; then
    echo "ok   — $desc"
  else
    echo "FAIL — $desc (want exit $want, got $got)"; echo "$out" | sed 's/^/       /'
    fails=$((fails + 1))
  fi
}

# The bug: a blocking body with no --request-changes must be refused, not approved.
check "blocking body without --request-changes is refused" 2 \
  999 --body "CHANGES REQUESTED at abc1234 -- the mirror is partial."
check "REJECT body without --request-changes is refused" 2 \
  999 --body "REJECTED: supersedes nothing."
check "BLOCKED body without --request-changes is refused" 2 \
  999 --body "BLOCKED pending an independent read."

# Stated intent and body agreeing must pass the guard and fall through to the
# credential check (exit 3 here, since no App key is installed in CI).
check "blocking body WITH --request-changes passes the guard" 3 \
  999 --request-changes --body "CHANGES REQUESTED at abc1234 -- see above."

# An ordinary approving verdict is untouched.
check "approving body still reaches the credential check" 3 \
  999 --body "Bound: read the full diff, ran the suite, all pass."

# @file bodies are read, not recorded literally, and are guarded the same way.
tmp="$(mktemp)"; printf 'CHANGES REQUESTED at abc1234 -- from a file.\n' > "$tmp"
check "blocking @file body is refused too" 2 999 --body "@$tmp"
rm -f "$tmp"

[ "$fails" = 0 ] && echo "all bind-event checks pass" || echo "$fails check(s) failed"
exit "$fails"
