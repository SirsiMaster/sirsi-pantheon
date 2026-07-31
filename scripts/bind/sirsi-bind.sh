#!/usr/bin/env bash
# sirsi-bind.sh — record an independent bind on a PR, as the `sirsi-bind` GitHub App.
#
# WHY THIS EXISTS
#   Every agent authenticates as the one account `SirsiMaster`, and GitHub forbids
#   approving your own PR. So an agent cannot bind an agent's PR — "bind" had no
#   mechanical existence, only a convention (see docs/ADR-041). This mints a token
#   for a SECOND identity (the sirsi-bind App) and records a real approving review,
#   which is what .github/workflows/binding-hold.yml requires on authority-model paths.
#
# WHY THE KEY IS LOCAL AND NOT A GITHUB SECRET
#   A private key in GitHub Secrets could be minted by any workflow, which would let
#   a PR approve itself and restore the exact circularity ADR-041 removes. The key
#   lives only on the conduit host. Do not "improve" this by uploading it.
#
# SETUP (owner, one-time): docs/runbooks/bind-identity-setup.md
#
# Usage:  scripts/bind/sirsi-bind.sh <pr-number> [--repo owner/name] [--body "verdict"|@file]
#                                    [--request-changes]
set -euo pipefail

APP_ID_FILE="${SIRSI_BIND_APP_ID_FILE:-$HOME/.sirsi/bind-app.id}"
KEY_FILE="${SIRSI_BIND_KEY_FILE:-$HOME/.sirsi/bind-app.pem}"
REPO="SirsiMaster/sirsi-pantheon"
BODY="Independent bind recorded by the sirsi-bind identity (ADR-041)."
PR=""
EVENT="APPROVE"

while [ $# -gt 0 ]; do
  case "$1" in
    --repo) REPO="$2"; shift 2 ;;
    --request-changes) EVENT="REQUEST_CHANGES"; shift ;;
    --body)
      # @file support, matching `sirsi router send/close --instructions/--result`
      # and `gh --body-file`.
      #
      # It was ABSENT here, and the omission is an evidence-loss trap rather than
      # a missing convenience: the router verbs REQUIRE @file (inline bodies get
      # shell-evaluated), so an operator who has correctly internalised "always
      # @file the body" silently recorded the literal string "@bind333.md" as the
      # approving verdict on PR #333 — and the bind reported success. The gate
      # opened on a review whose justification was a filename, with no signal
      # that anything was lost. Caught only because the artifact was read back.
      if [ "${2#@}" != "$2" ]; then
        _bf="${2#@}"
        [ -r "$_bf" ] || { echo "sirsi-bind: --body @$_bf is not readable" >&2; exit 2; }
        BODY="$(cat "$_bf")"
      else
        BODY="$2"
      fi
      shift 2 ;;
    -h|--help) sed -n '2,20p' "$0"; exit 0 ;;
    *) PR="$1"; shift ;;
  esac
done

[ -n "$PR" ] || { echo "usage: $0 <pr-number> [--repo owner/name] [--body text|@file] [--request-changes]" >&2; exit 2; }

# A blocking verdict must NEVER post as APPROVE. This script previously hardcoded
# event=APPROVE, so a bind whose body opened "CHANGES REQUESTED ..." was recorded by
# GitHub as an APPROVED review — and the gate re-run at the bottom then CLEARED
# binding-hold on the very PR the binder had just blocked. That is how PR #416 sat
# APPROVED + MERGEABLE + binding-hold=pass carrying a verdict that reads "Blocking on
# one thing". reviewDecision is a badge; the body was the truth, and they disagreed.
#
# Refuse rather than silently flip: guessing the caller's intent from prose is how the
# first bug happened. The caller states it with --request-changes, and this only stops
# the two from contradicting each other.
case "$BODY" in
  "CHANGES REQUESTED"*|"REJECT"*|"BLOCKED"*|"CHANGES-REQUESTED"*)
    [ "$EVENT" = "REQUEST_CHANGES" ] || {
      echo "✗ refusing to APPROVE: --body opens with a blocking verdict but --request-changes was not passed." >&2
      echo "  Pass --request-changes to record the block, or reword the body if you meant to approve." >&2
      exit 2
    } ;;
esac

if [ ! -r "$KEY_FILE" ] || [ ! -r "$APP_ID_FILE" ]; then
  cat >&2 <<EOF
✗ bind identity not installed on this host.
  missing: $([ -r "$KEY_FILE" ] || echo "$KEY_FILE") $([ -r "$APP_ID_FILE" ] || echo "$APP_ID_FILE")
  The sirsi-bind GitHub App is an OWNER setup step (creating an App and granting it
  review rights are access-control actions an agent must not perform).
  Runbook: docs/runbooks/bind-identity-setup.md
EOF
  exit 3
fi

APP_ID="$(tr -d '[:space:]' < "$APP_ID_FILE")"

b64url() { openssl base64 -A | tr '+/' '-_' | tr -d '='; }

now=$(date +%s)
header='{"alg":"RS256","typ":"JWT"}'
# iat backdated 60s: GitHub rejects a JWT whose iat is even slightly in the future
# when the local clock runs fast. exp capped at 10min by GitHub; 9min stays inside it.
payload=$(printf '{"iat":%d,"exp":%d,"iss":"%s"}' "$((now - 60))" "$((now + 540))" "$APP_ID")
signing_input="$(printf '%s' "$header" | b64url).$(printf '%s' "$payload" | b64url)"
sig=$(printf '%s' "$signing_input" | openssl dgst -sha256 -sign "$KEY_FILE" -binary | b64url)
JWT="$signing_input.$sig"

INSTALL_ID=$(gh api -H "Authorization: Bearer $JWT" "repos/$REPO/installation" --jq '.id') || {
  echo "✗ sirsi-bind App is not installed on $REPO — install it (runbook: docs/runbooks/bind-identity-setup.md)." >&2
  exit 4
}
TOKEN=$(gh api -X POST -H "Authorization: Bearer $JWT" \
  "app/installations/$INSTALL_ID/access_tokens" --jq '.token')

HEAD_SHA=$(gh api "repos/$REPO/pulls/$PR" --jq '.head.sha')
AUTHOR=$(gh api "repos/$REPO/pulls/$PR" --jq '.user.login')

# Pin the review to the head SHA the binder actually reviewed. The gate re-checks
# this; a later push drops the bind rather than inheriting it.
GH_TOKEN="$TOKEN" gh api -X POST "repos/$REPO/pulls/$PR/reviews" \
  -f event="$EVENT" -f commit_id="$HEAD_SHA" -f body="$BODY" --jq '"bound: " + .user.login + " @ " + .commit_id' \
  || { echo "✗ review rejected — if the App IS the PR author, GitHub forbids self-approval (that is the gate working)." >&2; exit 5; }

# Re-run the gate so it re-reads the reviews API and clears. This is done HERE, and
# not with a `pull_request_review` trigger on the workflow, because a review-triggered
# run resolves its SHA against the base branch — its check would land on the wrong
# commit and the PR would stay blocked forever. Re-running the PR's own run keeps the
# correct head-SHA context.
#
# Only an APPROVE may re-run it. On a block there is nothing to clear, and re-running
# would refresh a green binding-hold check on a PR that is being rejected — the same
# green-surface-over-a-dead-thing the event fix above removes.
RUN_ID=""
if [ "$EVENT" = "APPROVE" ]; then
  RUN_ID=$(gh run list --repo "$REPO" --workflow binding-hold.yml --commit "$HEAD_SHA" \
             --limit 1 --json databaseId --jq '.[0].databaseId' 2>/dev/null || true)
fi
if [ "$EVENT" != "APPROVE" ]; then
  echo "  binding-hold left in place — this bind is a block, not a clearance."
elif [ -n "${RUN_ID:-}" ] && [ "$RUN_ID" != "null" ]; then
  gh run rerun "$RUN_ID" --repo "$REPO" >/dev/null 2>&1 \
    && echo "  re-ran binding-hold (run $RUN_ID) — it will re-read the bind and clear." \
    || echo "  ⚠ could not re-run binding-hold run $RUN_ID — re-run it from the Checks tab." >&2
else
  echo "  ⚠ no binding-hold run found for $HEAD_SHA — push or re-run the check manually." >&2
fi

if [ "$EVENT" = "APPROVE" ]; then
  echo "✔ PR #$PR bound on $HEAD_SHA (author: $AUTHOR)."
else
  echo "✔ PR #$PR BLOCKED on $HEAD_SHA (author: $AUTHOR) — changes requested, gate still held."
fi
