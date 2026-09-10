#!/usr/bin/env bash
# Hermetic fixtures for cutover-m5.sh write_env (SSA 2026-09-10): fresh host, legacy host carrying the OLD
# unconditional managed line, repeat run, and an unrelated comment that mentions router-service.env.
# Each: bash installation into a throwaway HOME, then `zsh -f` sources the zshenv with a lane's spool
# choice already set; the lane must keep its spool URL and have no token.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
eval "$(sed -n '/^SRC_LINE=/p;/^ZSHENV_MARKER=/p' "$here/cutover-m5.sh")"; eval "$(sed -n '/^write_env() {/,/^}/p' "$here/cutover-m5.sh")"
URL=https://router.example.test
probe() { # $1 HOME → prints url|tokenset as a lane shell (zsh -f) sees them after sourcing
  HOME="$1" SIRSI_ROUTER_URL=spool:///lane/relay zsh -f -c 'unset SIRSI_ROUTER_TOKEN; source "$HOME/.zshenv"; printf "%s|%s" "$SIRSI_ROUTER_URL" "${SIRSI_ROUTER_TOKEN:+set}"'
}
check() { local name=$1 home=$2; local got; got=$(probe "$home"); [ "$got" = "spool:///lane/relay|" ] || { echo "FAIL $name: lane saw '$got'"; cat "$home/.zshenv"; exit 1; }
  [ "$(grep -c "$ZSHENV_MARKER" "$home/.zshenv")" = 1 ] || { echo "FAIL $name: managed line count != 1"; cat "$home/.zshenv"; exit 1; }
  HOME="$home" zsh -f -c 'unset SIRSI_ROUTER_URL SIRSI_ROUTER_TOKEN; source "$HOME/.zshenv"; [ "$SIRSI_ROUTER_URL" = https://router.example.test ] && [ -n "$SIRSI_ROUTER_TOKEN" ]' || { echo "FAIL $name: plain shell did not get the service env"; exit 1; }; }
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
# fresh
F=$T/fresh; mkdir -p $F; echo FAKE-FIXTURE-TOKEN | HOME=$F write_env ""; check fresh $F
# legacy: the OLD unconditional managed line, plus unrelated content that mentions the file
L=$T/legacy; mkdir -p $L; printf '%s\n' '[ -f "$HOME/.cargo/env" ] && . "$HOME/.cargo/env"' '# note: router-service.env holds the service token (unrelated comment)' '[ -r "$HOME/.sirsi/router-service.env" ] && . "$HOME/.sirsi/router-service.env"  # ADR-062 router service' 'export FOO=bar' >$L/.zshenv
echo FAKE-FIXTURE-TOKEN | HOME=$L write_env ""; check legacy $L
grep -q 'unrelated comment' $L/.zshenv && grep -q 'FOO=bar' $L/.zshenv && grep -q cargo $L/.zshenv || { echo "FAIL legacy: unrelated lines lost"; cat $L/.zshenv; exit 1; }
# repeat: idempotent
echo FAKE-FIXTURE-TOKEN | HOME=$L write_env ""; check repeat $L
echo "OK: fresh, legacy upgrade (old managed line replaced, unrelated lines kept), repeat idempotent; lane keeps spool URL and no token in all cases"
