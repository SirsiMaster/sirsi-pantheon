#!/usr/bin/env bash
# Exercise the normal binder, not a copied guard. Optional argument selects ONLY
# the test subject for main's independent guard-omission negative control. The
# production script has no test/bypass flag. Every credential is plaintext fake;
# gh, sirsi and openssl are closed command doubles, never real network/crypto.
set -euo pipefail
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SUBJECT="${1:-$SOURCE_DIR/sirsi-bind.sh}"
case "$SUBJECT" in /*) ;; *) echo 'test subject must be an absolute path' >&2; exit 2 ;; esac
[ -r "$SUBJECT" ] || { echo 'test subject is unreadable' >&2; exit 2; }
JQ_BIN="$(command -v jq)"
fixture_dir="$(mktemp -d /tmp/sirsi-bind-head-fixture.XXXXXX)"
trap '/bin/rm -rf -- "$fixture_dir"' EXIT
mkdir "$fixture_dir/bin" "$fixture_dir/home"
cp "$SUBJECT" "$fixture_dir/sirsi-bind.sh"
cp "$SOURCE_DIR/router-rejection.jq" "$fixture_dir/router-rejection.jq"
printf '123\n' > "$fixture_dir/fake-app.id"
printf 'NOT A PRIVATE KEY: plaintext fixture only\n' > "$fixture_dir/fake-app.pem"

cat > "$fixture_dir/bin/openssl" <<'DOUBLE'
#!/bin/bash
set -euo pipefail
case "$*" in
  'base64 -A') ;;
  "dgst -sha256 -sign $SIRSI_BIND_KEY_FILE -binary") ;;
  *) echo 'unexpected fake openssl operation' >&2; exit 91 ;;
esac
/bin/cat >/dev/null
printf 'ZmFrZQ==' # fixed fake bytes; no encoding, key read or cryptography
DOUBLE

cat > "$fixture_dir/bin/sirsi" <<'DOUBLE'
#!/bin/bash
set -euo pipefail
[ "$*" = 'router dump' ] || { echo 'unexpected fake sirsi operation' >&2; exit 91; }
printf 'dump\n' >> "$BIND_FIXTURE_DUMPS"
[ "$BIND_FIXTURE_ROUTER_RC" = 0 ] || exit "$BIND_FIXTURE_ROUTER_RC"
/bin/cat "$BIND_FIXTURE_DATA"
DOUBLE

cat > "$fixture_dir/bin/gh" <<'DOUBLE'
#!/bin/bash
set -euo pipefail
case "$1" in
  api)
    endpoint='' method=GET query='' event='' commit='' body=''
    shift
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -H) shift 2 ;; # synthetic authorization is deliberately not logged
        -X) method="$2"; shift 2 ;;
        --jq) query="$2"; shift 2 ;;
        --paginate) shift ;;
        -f)
          case "$2" in event=*) event="${2#event=}" ;; commit_id=*) commit="${2#commit_id=}" ;; body=*) body="${2#body=}" ;; *) exit 91 ;; esac
          shift 2 ;;
        repos/*|app/*) endpoint="$1"; shift ;;
        *) echo 'unexpected fake gh argument' >&2; exit 91 ;;
      esac
    done
    case "$method:$endpoint" in
      GET:repos/SirsiMaster/sirsi-pantheon/installation) printf '7\n' ;;
      POST:app/installations/7/access_tokens) printf 'fake-token\n' ;;
      GET:repos/SirsiMaster/sirsi-pantheon/pulls/927)
        case "$query" in '.head.sha') printf '%s\n' "$BIND_FIXTURE_HEAD" ;; '.user.login') printf 'SirsiMaster\n' ;; *) exit 91 ;; esac ;;
      GET:repos/SirsiMaster/sirsi-pantheon/pulls/927/reviews) : ;; # no native GitHub rejection
      POST:repos/SirsiMaster/sirsi-pantheon/pulls/927/reviews)
        "$BIND_FIXTURE_JQ" -cn --arg event "$event" --arg commit "$commit" --arg body "$body" \
          '{event:$event,commit:$commit,body:$body}' >> "$BIND_FIXTURE_POSTS"
        printf 'bound fixture\n' ;;
      *) echo 'unexpected fake gh endpoint/method' >&2; exit 91 ;;
    esac ;;
  run)
    [ "$2" = list ] || { echo 'unexpected fake gh run action' >&2; exit 91; }
    printf 'list\n' >> "$BIND_FIXTURE_RUNS"
    # No existing workflow run: never call a real rerun.
    ;;
  *) echo 'unexpected fake gh command' >&2; exit 91 ;;
esac
DOUBLE
chmod 700 "$fixture_dir/bin/gh" "$fixture_dir/bin/sirsi" "$fixture_dir/bin/openssl"

REPO=SirsiMaster/sirsi-pantheon
HEAD=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
OLD=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
item() {
  "$JQ_BIN" -cn --arg id "$1" --arg from "$2" --arg title "$3" --arg body "$4" \
    '{id:$id,from:$from,title:$title,body:$body,type:"review",opened:"2026-10-23T00:00:00Z"}'
}
declaration() {
  printf '```sirsi-bind-verdict-v1\nrepo: %s\npr: 927\nhead: %s\nverdict: %s\nresolves: %s\n```' "$2" "$3" "$1" "$4"
}
r1="$(item r1 codex-pantheon 'PR927 CHANGES REQUIRED' 'original finding')"
r2="$(item r2 claude-pantheon 'PR927 REJECT' 'other reviewer finding')"
accept="$(declaration ACCEPT "$REPO" "$HEAD" r1)"
fails=0
check() { # name expected-exit expected-event-or-none input-jsonl [binder-args...]
  local name="$1" want="$2" event="$3" data="$4"; shift 4
  local rc=0 count=0
  printf '%s\n' "$data" > "$fixture_dir/input.jsonl"
  : > "$fixture_dir/posts.jsonl"
  : > "$fixture_dir/dumps"
  : > "$fixture_dir/runs"
  (
    cd "$fixture_dir"
    env -i HOME="$fixture_dir/home" PATH="$fixture_dir/bin:$(dirname "$JQ_BIN"):/usr/bin:/bin" \
      SIRSI_BIND_APP_ID_FILE="$fixture_dir/fake-app.id" SIRSI_BIND_KEY_FILE="$fixture_dir/fake-app.pem" \
      BIND_FIXTURE_JQ="$JQ_BIN" BIND_FIXTURE_HEAD="$HEAD" BIND_FIXTURE_DATA="$fixture_dir/input.jsonl" \
      BIND_FIXTURE_POSTS="$fixture_dir/posts.jsonl" BIND_FIXTURE_DUMPS="$fixture_dir/dumps" \
      BIND_FIXTURE_RUNS="$fixture_dir/runs" BIND_FIXTURE_ROUTER_RC="${router_rc:-0}" \
      /bin/bash "$fixture_dir/sirsi-bind.sh" 927 --repo "$REPO" --body 'independent fixture verdict' "$@"
  ) > "$fixture_dir/stdout" 2> "$fixture_dir/stderr" || rc=$?
  count="$("$JQ_BIN" -s 'length' "$fixture_dir/posts.jsonl")"
  if [ "$rc" != "$want" ]; then
    printf 'FAIL — %s (exit %s, wanted %s)\n' "$name" "$rc" "$want"
    /bin/cat "$fixture_dir/stderr"; fails=$((fails+1)); return
  fi
  if [ "$event" = none ]; then
    if [ "$count" != 0 ] || [ -s "$fixture_dir/runs" ]; then
      printf 'FAIL — %s published a review or touched gate rerun after refusal\n' "$name"; fails=$((fails+1)); return
    fi
  elif [ "$count" != 1 ] || ! "$JQ_BIN" -e --arg event "$event" --arg head "$HEAD" \
      '.event == $event and .commit == $head' "$fixture_dir/posts.jsonl" >/dev/null; then
    printf 'FAIL — %s did not publish exactly one intended event at exact current head\n' "$name"; fails=$((fails+1)); return
  fi
  if [ "$event" = REQUEST_CHANGES ]; then
    if [ -s "$fixture_dir/dumps" ] || [ -s "$fixture_dir/runs" ]; then
      printf 'FAIL — request-changes entered approval guard/rerun\n'; fails=$((fails+1)); return
    fi
  elif [ ! -s "$fixture_dir/dumps" ]; then
    printf 'FAIL — %s skipped the normal router channel\n' "$name"; fails=$((fails+1)); return
  fi
  printf 'ok — %s\n' "$name"
}

check 'normal current rejection refuses before APPROVE' 6 none "$r1"
check 'title-only acceptance cannot clear normal bind' 6 none "$r1
$(item a1 codex-pantheon 'PR927 exact head ACCEPT' '')"
check 'current SHA mentioned as not reviewed cannot clear' 6 none "$r1
$(item a1 codex-pantheon 'PR927 PASS baseline' "Head $HEAD is NOT reviewed; accepted old $OLD only")"
check 'stale declared head cannot clear' 6 none "$r1
$(item a1 codex-pantheon 'PR927 ACCEPT' "$(declaration ACCEPT "$REPO" "$OLD" r1)")"
check 'other reviewer cannot resolve rejection' 6 none "$r1
$(item a1 claude-pantheon 'PR927 ACCEPT' "$accept")"
check 'all reviewers unresolved set is retained' 6 none "$r1
$r2
$(item a1 codex-pantheon 'PR927 ACCEPT' "$accept")"
check 'truncated declaration cannot clear' 6 none "$r1
$(item a1 codex-pantheon 'PR927 ACCEPT' "$(printf '%s\n' "$accept" | sed '$d')")"
check 'proper resolution posts APPROVE at current head' 0 APPROVE "$r1
$(item a1 codex-pantheon 'PR927 ACCEPT' "$accept")"
check 'each reviewer resolves their own item' 0 APPROVE "$r1
$r2
$(item a1 codex-pantheon 'PR927 ACCEPT' "$accept")
$(item a2 claude-pantheon 'PR927 ACCEPT' "$(declaration ACCEPT "$REPO" "$HEAD" r2)")"
check 'known unrelated repository does not block' 0 APPROVE "$(item r1 codex-pantheon 'PR927 REJECT' "$(declaration REJECT SirsiMaster/other "$HEAD" -)")"
check 'explicit matching override remains usable' 0 APPROVE "$r1" --override-pr 927 --override-finding 'reviewed fixture finding'
if ! "$JQ_BIN" -e '.body | contains("router rejection r1 cleared: reviewed fixture finding")' "$fixture_dir/posts.jsonl" >/dev/null; then
  echo 'FAIL — explicit override evidence lost'; fails=$((fails+1))
fi
check 'wrong PR override cannot clear' 6 none "$r1" --override-pr 928 --override-finding 'reviewed fixture finding'
check 'request-changes preserves explicit event and commit pin' 0 REQUEST_CHANGES "$r1" --request-changes
router_rc=7
check 'unreadable canonical channel refuses' 6 none ''
unset router_rc
exit "$fails"
