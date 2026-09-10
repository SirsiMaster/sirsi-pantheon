#!/usr/bin/env bash
# Hermetic rehearsal of cutover-m5.sh's node-local restore against DISPOSABLE paths (SSA 2026-09-10):
# 1 placeholder directory + frozen copy → file restored, frozen untouched, WAL, previous binary restored;
# 2 placeholder directory + NO frozen copy → refuses with exit 2 (never a silent exit 0);
# 3 already a file → idempotent.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd); T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
RESTORE_LOCAL=$(sed -n "/^RESTORE_LOCAL='/,/^'\$/p" "$here/cutover-m5.sh" | sed '1s/^RESTORE_LOCAL=.//' | sed '$d')
[ -n "$RESTORE_LOCAL" ] && bash -n <<<"$RESTORE_LOCAL" || { echo "FAIL: could not extract RESTORE_LOCAL"; exit 1; }
sqlite3 "$T/frozen.db" "pragma user_version=16; create table t(i); insert into t values(1);"; H0=$(shasum -a 256 "$T/frozen.db" | cut -c1-16)
mkdir "$T/router.db"; chmod 000 "$T/router.db"; printf '#!/bin/sh\necho prev\n' >"$T/prev"; chmod +x "$T/prev"; printf '#!/bin/sh\necho live\n' >"$T/bin"; chmod +x "$T/bin"
# 1
DB=$T/router.db FROZEN=$T/frozen.db PREV=$T/prev BIN=$T/bin bash -s <<<"$RESTORE_LOCAL" | grep -q '^restored:' || { echo "FAIL 1: restore"; exit 1; }
[ -f "$T/router.db" ] && [ "$(sqlite3 "$T/router.db" 'select count(*) from t')" = 1 ] && [ "$(sqlite3 "$T/router.db" 'pragma journal_mode')" = wal ] || { echo "FAIL 1: content/wal"; exit 1; }
[ "$(shasum -a 256 "$T/frozen.db" | cut -c1-16)" = "$H0" ] || { echo "FAIL 1: frozen copy modified"; exit 1; }
[ "$("$T/bin")" = prev ] || { echo "FAIL 1: previous binary not restored"; exit 1; }
# 2
rm -rf "$T/router.db"; mkdir "$T/router.db"
set +e; DB=$T/router.db FROZEN= PREV= BIN=$T/bin bash -s <<<"$RESTORE_LOCAL" >/dev/null 2>"$T/err"; rc=$?; set -e
[ "$rc" = 2 ] && grep -q REFUSED "$T/err" && [ -d "$T/router.db" ] || { echo "FAIL 2: rc=$rc"; cat "$T/err"; exit 1; }
# 3
rmdir "$T/router.db"; cp "$T/frozen.db" "$T/router.db"
DB=$T/router.db FROZEN=$T/frozen.db PREV= BIN=$T/bin bash -s <<<"$RESTORE_LOCAL" | grep -q '^restored:' || { echo "FAIL 3: idempotent"; exit 1; }
echo "OK: restore from placeholder, refusal without frozen copy (exit 2), idempotent on a file"
