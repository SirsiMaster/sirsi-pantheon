#!/usr/bin/env bash
# Read-only probes for one Mac console's tailnet entrance (SSA 050808 item 3).
#
#   verify-mac-console.sh entrance <port|/path/to.sock>   # on the Mac itself
#   verify-mac-console.sh tailnet  <name> [tailnet]       # from any tailnet device
#
# entrance: the protected listener must refuse a request carrying no Tailscale
#           identity (nothing but tailscaled should ever stamp one).
# tailnet:  through `tailscale serve`, a person's device must be admitted as
#           itself, and a forged Tailscale-User-Login must never win. From a
#           TAGGED device (a tag:sirsi-mac Mac) Tailscale sends no person, so a
#           403 there is the correct tagged-source refusal, not a failure.
# Changes nothing; prints PASS/FAIL per case and exits non-zero on any FAIL.
set -u
fail=0
pass() { echo "PASS  $*"; }
bad() { echo "FAIL  $*"; fail=1; }

case "${1:-}" in
entrance)
  t=${2:?port or socket path}
  if [[ $t == /* ]]; then
    code=$(curl -s -o /dev/null -w '%{http_code}' --unix-socket "$t" http://console/healthz)
  else
    code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$t/healthz")
  fi
  [[ $code == 403 ]] && pass "entrance without an identity -> 403" || bad "entrance without an identity -> $code (want 403)"
  ;;
tailnet)
  name=${2:?mac name}; tn=${3:-taild727f7.ts.net}
  url="https://$(tr '[:upper:]' '[:lower:]' <<<"$name").$tn"
  forged=nobody@evil.example
  code=$(curl -s -o /dev/null -w '%{http_code}' "$url/healthz")
  who=$(curl -s "$url/whoami")
  fcode=$(curl -s -o /tmp/verify-mac-console.$$ -w '%{http_code}' -H "Tailscale-User-Login: $forged" "$url/whoami")
  fwho=$(cat /tmp/verify-mac-console.$$ 2>/dev/null); rm -f /tmp/verify-mac-console.$$
  case $code in
  200) [[ $who == *@sirsi.ai ]] && pass "person admitted as $who" || bad "healthz 200 but whoami says '$who'" ;;
  403) pass "no person admitted (tagged or non-sirsi source refused) — run from a personal sirsi.ai device to prove admission" ;;
  *) bad "healthz -> $code" ;;
  esac
  if [[ $fcode == 200 && $fwho == *"$forged"* ]]; then
    bad "forged identity header was admitted as $forged"
  elif [[ $fcode == 200 && $fwho == "$who" ]] || [[ $fcode == 403 ]]; then
    pass "forged header ignored (admitted: ${fwho:-none}, code $fcode)"
  else
    bad "forged-header probe -> $fcode '$fwho' (expected the real identity or 403)"
  fi
  ;;
*)
  sed -n '2,13p' "$0"; exit 2 ;;
esac
exit $fail
