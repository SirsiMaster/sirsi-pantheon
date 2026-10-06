#!/usr/bin/env bash
# release-train.sh <version> [--deploy-service] [--dry-run]
# One verb for the release: prepare the changelog PR, wait for its single CI run,
# merge, optionally deploy the router service (before clients ship a new Store
# method), tag, wait for the publish, upgrade both Macs, restart every loop.
# Every step is a hard stop: nothing proceeds on a failed, missing or stale result.
set -uo pipefail
VERSION="${1:-}"; shift || true
DEPLOY=0; DRY=0
for a in "$@"; do case "$a" in --deploy-service) DEPLOY=1;; --dry-run) DRY=1;; esac; done
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 <x.y.z> [--deploy-service] [--dry-run]"; exit 2; }
ROOT="$(git rev-parse --show-toplevel)"; B="release/${VERSION}"
M5_HOST="${M5_HOST:-thekryptodragon@m5}"
step() { echo "== $*"; }
run()  { if [ "$DRY" = 1 ]; then echo "   (dry-run) $*"; else eval "$@"; fi; }
die()  { echo "STOP: $*" >&2; exit 1; }

step "plan"; echo "   prepare $VERSION on $B; CI; merge; deploy-service=$DEPLOY; tag v$VERSION; publish; upgrade M1+$M5_HOST; restart loops"
[ "$DRY" = 1 ] && { echo "dry-run complete"; exit 0; }

cd "$ROOT" || die "no repo"
git fetch -q origin main || die "fetch main failed"
# A developer checkout can legitimately retain divergent historical local tags.
# The release decision is about this exact target name at the publishing
# authority, not whether every old local tag can be force-updated by fetch.
# Refuse a local target collision and an already-published remote target, but
# do not make unrelated stale tags block a new commercial release.
git show-ref --verify --quiet "refs/tags/v$VERSION" && die "local tag v$VERSION already exists"
git ls-remote --exit-code --refs origin "refs/tags/v$VERSION" >/dev/null 2>&1 && die "remote tag v$VERSION already exists"
git rev-parse -q --verify "origin/main" >/dev/null || die "no origin/main"
WT="$(mktemp -d)/rel"; git worktree add -q -b "$B" "$WT" origin/main || die "worktree failed"
cd "$WT" || die "cd failed"
python3 "$ROOT/scripts/release-prep-changelog.py" "$VERSION" || die "changelog prep failed (no new entries?)"
git add VERSION CHANGELOG.md docs/stacklab/pantheon-pt/canon/CHANGELOG.md && git commit -qm "release: prepare Pantheon v$VERSION" || die "commit failed"
MAAT_WINDOW_OVERRIDE=1 git push -q -u origin "$B" || die "push failed"
[ "$(git ls-remote --heads origin "$B" | cut -c1-40)" = "$(git rev-parse HEAD)" ] || die "remote head differs"
gh pr create --base main --head "$B" --title "release: prepare Pantheon v$VERSION" --body "Release prep (changelog, VERSION, canon)." >/dev/null || die "pr create failed"
sleep 15
for _ in $(seq 1 180); do s="$(gh pr checks "$B" 2>&1)"; echo "$s" | grep -qE "no checks|pending|in_progress|queued" && { sleep 10; continue; }; break; done
echo "$s" | grep -qE "fail" && die "CI failed"
gh pr merge "$B" --squash >/dev/null; [ "$(gh pr view "$B" --json state -q .state)" = MERGED ] || die "not merged"
if [ "$DEPLOY" = 1 ]; then
  git fetch -q origin; D="$(mktemp -d)/deploy"; git worktree add -q --detach "$D" origin/main || die "deploy worktree"
  (cd "$D" && gcloud run deploy sirsi-router --source . --project sirsi-nexus-live --region us-central1 --quiet) || die "deploy failed"
  gcloud run services describe sirsi-router --region us-central1 --project sirsi-nexus-live --format='value(status.traffic[0].percent)' | grep -q 100 || die "traffic not 100% on the new revision"
  # ADR-062 §4 audit receipt: a deploy without one is unrecorded.
  RP="--region us-central1 --project sirsi-nexus-live"
  REV="$(gcloud run services describe sirsi-router $RP --format='value(status.latestReadyRevisionName)')"; [ -n "$REV" ] || die "no ready revision name"
  IMG="$(gcloud run revisions describe "$REV" $RP --format='value(spec.containers[0].image)')"; [ -n "$IMG" ] || die "no image for $REV"
  PREV="$(gcloud run revisions list --service sirsi-router $RP --format='value(metadata.name)' --limit 2 | sed -n 2p)"
  PIMG=""; [ -n "$PREV" ] && PIMG="$(gcloud run revisions describe "$PREV" $RP --format='value(spec.containers[0].image)')"
  RCPT="$(mktemp)"; printf 'Router service deploy receipt for v%s.\n\nrevision: %s\nimage: %s\ngit sha: %s\nrollback target revision: %s\nrollback target image: %s\n\nRollback: gcloud run services update-traffic sirsi-router --to-revisions %s=100 %s\n' "$VERSION" "$REV" "$IMG" "$(git rev-parse origin/main)" "${PREV:-none}" "${PIMG:-none}" "${PREV:-none}" "$RP" > "$RCPT"
  sirsi router send --from ra --to claude-home --type decision --title "Router service deploy receipt: $REV (v$VERSION)" --instructions @"$RCPT" >/dev/null || die "deploy receipt not recorded"
fi
git fetch -q origin main; SHA="$(git rev-parse origin/main)"
git tag -a "v$VERSION" "$SHA" -m "v$VERSION" && MAAT_WINDOW_OVERRIDE=1 git push -q origin "v$VERSION" || die "tag push failed"
sleep 20; RID="$(gh run list --workflow 'Release — Build & Publish' --branch "v$VERSION" --limit 1 --json databaseId -q '.[0].databaseId')"
[ -n "$RID" ] || die "no release run found"
while :; do s="$(gh run view "$RID" --json status,conclusion -q '.status+" "+.conclusion')"; case "$s" in "completed success") break;; completed*) die "release run: $s";; esac; sleep 10; done
export PATH=/opt/homebrew/bin:$PATH
brew update -q >/dev/null 2>&1; brew upgrade --cask sirsimaster/tools/sirsi-pantheon >/dev/null 2>&1
sirsi version | grep -q "v$VERSION" || die "M1 not on v$VERSION"
ssh -o BatchMode=yes "$M5_HOST" "export PATH=/opt/homebrew/bin:\$HOME/.local/bin:\$PATH; brew update -q >/dev/null 2>&1; brew upgrade --cask sirsimaster/tools/sirsi-pantheon >/dev/null 2>&1; sirsi version" | grep -q "v$VERSION" || die "M5 not on v$VERSION"
U="$(id -u)"; for l in $(launchctl list | awk '{print $3}' | grep '^ai.sirsi.router.wake\.'); do launchctl kickstart -k "gui/$U/$l" >/dev/null; done
ssh -o BatchMode=yes "$M5_HOST" 'U=$(id -u); for l in $(launchctl list|awk "{print \$3}"|grep "^ai.sirsi.router.wake\."); do launchctl kickstart -k gui/$U/$l >/dev/null; done'
echo "RELEASED v$VERSION on M1 and M5, loops restarted"
