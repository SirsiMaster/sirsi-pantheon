#!/usr/bin/env bash
# Asserts sirsi-bind.sh never guesses a repository.
#
# It used to open with REPO="SirsiMaster/sirsi-pantheon". PR numbers collide
# across repos, so an omitted --repo did not fail — it found pantheon#N, posted
# a REAL approving review as sirsi-bind[bot], and reported success. Measured
# 2026-08-07: it fired twice against pantheon#64 (a months-merged CI PR nobody
# had read) while the operator meant Assiduous#64. GitHub will not dismiss a
# review on a merged PR, so both approvals are permanent.
#
# No network and no credentials: every case exits before the first gh API call.
# Exit codes are the discriminator — 2 is "refused at argument/resolution time",
# 3 is "bind identity not installed", which can only be reached AFTER the repo
# resolved. A case that wants "resolution succeeded" therefore asserts 3.
set -uo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/sirsi-bind.sh"
[ -x "$SCRIPT" ] || { echo "FAIL — $SCRIPT missing or not executable"; exit 1; }

# Point the identity at paths that cannot exist, so a resolved repo lands on
# exit 3 instead of proceeding to a real bind against a real PR.
export SIRSI_BIND_KEY_FILE=/nonexistent/sirsi-bind.pem
export SIRSI_BIND_APP_ID_FILE=/nonexistent/sirsi-bind.id

fails=0
check() { # <desc> <expected-exit> <workdir> [args...]
  local desc="$1" want="$2" dir="$3"; shift 3
  local out got
  out="$(cd "$dir" && "$SCRIPT" "$@" 2>&1)"; got=$?
  if [ "$got" = "$want" ]; then
    echo "ok   — $desc"
  else
    echo "FAIL — $desc (want exit $want, got $got)"
    printf '       output: %s\n' "$(printf '%s' "$out" | head -3 | tr '\n' '|')"
    fails=$((fails + 1))
  fi
}

NONREPO="$(mktemp -d)"
trap 'rm -rf "$NONREPO"' EXIT

# 1. The regression itself. Omitted --repo outside any GitHub checkout must
#    REFUSE. Before the fix this silently became SirsiMaster/sirsi-pantheon and
#    went on to bind pantheon#64 for real.
check "omitted --repo outside a repo refuses instead of defaulting" 2 "$NONREPO" 64

# 2. Explicit --repo is honoured from anywhere — binding another repo's PR from
#    an unrelated cwd is legitimate and must NOT be cross-checked against it.
#    Exit 3 proves resolution passed and only the absent identity stopped it.
check "explicit --repo is honoured outside a repo" 3 "$NONREPO" 64 --repo SirsiMaster/Assiduous

# 3. Inside a GitHub checkout an omitted --repo resolves from the cwd rather
#    than refusing — the fix must not make the common in-repo call unusable.
check "omitted --repo inside a checkout resolves from cwd" 3 "$(dirname "$SCRIPT")" 64

# 4. A missing PR number still fails at argument validation, before resolution.
check "missing PR number still refuses" 2 "$NONREPO"

# 5. Source guard: catches someone re-adding the default in a later edit, which
#    cases 1-3 would NOT catch if the resolution block were also removed.
if grep -qE '^REPO="[^"]+"' "$SCRIPT"; then
  echo "FAIL — sirsi-bind.sh reintroduced a hardcoded default REPO"
  fails=$((fails + 1))
else
  echo "ok   — no hardcoded default REPO in source"
fi

[ "$fails" -eq 0 ] && echo "PASS — repo resolution never guesses" || echo "$fails failure(s)"
exit $((fails > 0))
