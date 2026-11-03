#!/bin/bash
# Controlled bridge to the existing signer; does not publish or install.
set -euo pipefail
if [ "$#" -ne 3 ]; then
  echo 'usage: bash scripts/sign-pretoolguard-artifact.sh <staged-guard> <unsigned-sha256> <code-identifier>' >&2
  exit 2
fi
file=$1
expected=$2
identifier=$3
[[ "$file" = /* && -f "$file" && ! -L "$file" ]] || { echo 'absolute regular artifact required' >&2; exit 2; }
[[ "$expected" =~ ^[0-9a-f]{64}$ ]] || { echo 'full unsigned SHA-256 required' >&2; exit 2; }
[[ "$identifier" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || { echo 'invalid code identifier' >&2; exit 2; }
[[ $(uname -s) == Darwin && $(uname -m) == arm64 ]] || { echo 'native Darwin arm64 required' >&2; exit 1; }
dir=${file%/*}
name=${file##*/}
[[ "$name" == sirsi-display-power-guard || "$name" == sirsi-pretoolguard-install ]] || { echo 'unexpected artifact name' >&2; exit 1; }
[[ ! -L "$dir" && $(/usr/bin/stat -f '%u:%Lp' "$dir") == "$(id -u):700" ]] || { echo 'private owned staging directory required' >&2; exit 1; }
[[ ! -e "$dir/signed-$name" && ! -L "$dir/signed-$name" && ! -e "$dir/signed-$name.receipt.txt" && ! -L "$dir/signed-$name.receipt.txt" ]] || { echo 'existing signer output: refusing overwrite' >&2; exit 1; }
[[ $(/usr/bin/shasum -a 256 "$file" | /usr/bin/awk '{print $1}') == "$expected" ]] || { echo 'unsigned digest mismatch' >&2; exit 1; }
: "${PANTHEON_SIGN_CLIENT:?qualified existing signer client required}"
: "${SIRSI_SIGN_HOSTS:?explicit qualified signing host required}"
root=$(cd "$(dirname "$0")/.." && pwd -P)
[[ "$PANTHEON_SIGN_CLIENT" == "$root/tools/signing/sirsi-sign" && -x "$PANTHEON_SIGN_CLIENT" && ! -L "$PANTHEON_SIGN_CLIENT" && "$SIRSI_SIGN_HOSTS" == sirsi-m1 ]] || { echo 'qualified repository signer and M1 route required' >&2; exit 1; }
job=$(mktemp -d "$dir/signing.XXXXXXXX")
echo "Retaining signing directory: $job"
printf '%s\n' 'INCOMPLETE: do not use; signing outcome may be ambiguous' > "$job/INCOMPLETE"
/bin/cp -p "$file" "$job/$name"
file="$job/$name"
dir="$job"
[[ $(/usr/bin/shasum -a 256 "$file" | /usr/bin/awk '{print $1}') == "$expected" ]] || { echo 'private signing input digest mismatch' >&2; exit 1; }
"$PANTHEON_SIGN_CLIENT" "$file" bin
signed="$dir/signed-$name"
[[ -f "$signed" && ! -L "$signed" && -f "$signed.receipt.txt" && ! -L "$signed.receipt.txt" ]] || { echo 'signed output or receipt missing' >&2; exit 1; }
requirement="=identifier \"$identifier\" and anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists"
/usr/bin/codesign --verify --strict -R "$requirement" "$signed"
/usr/bin/python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$signed"
LC_ALL=C /usr/bin/codesign -d --verbose=4 "$signed" > "$dir/signature-details.txt" 2>&1
/usr/bin/grep -q '^Timestamp=' "$dir/signature-details.txt" || { echo 'secure timestamp not reported' >&2; exit 1; }
/usr/bin/grep -E -q '^CodeDirectory .*flags=.*\(.*runtime.*\)' "$dir/signature-details.txt" || { echo 'hardened runtime flag not reported' >&2; exit 1; }
# Signatures change Mach-O bytes. Compare two retained copies after removing
# their signatures; if normalization cannot establish exact equality, stop.
norm=$(mktemp -d "$dir/normalized.XXXXXXXX")
/bin/cp -p "$file" "$norm/input"
/bin/cp -p "$signed" "$norm/output"
[[ $(/usr/bin/shasum -a 256 "$norm/input" | /usr/bin/awk '{print $1}') == "$expected" ]] || { echo 'signing input changed during signing' >&2; exit 1; }
if /usr/bin/codesign -d "$norm/input" >/dev/null 2>&1; then
  /usr/bin/codesign --remove-signature "$norm/input"
fi
/usr/bin/codesign --remove-signature "$norm/output"
/usr/bin/cmp -s "$norm/input" "$norm/output" || { echo 'signed payload differs after signature normalization' >&2; exit 1; }
/usr/bin/shasum -a 256 "$signed" > "$dir/signed.sha256"
printf '%s\n' 'ACCEPTED: local signing checks completed' > "$dir/INCOMPLETE"
mv "$dir/INCOMPLETE" "$dir/ACCEPTED"
echo 'Signed binary verified against requested identifier and team; notarization, publication and installation remain separate.'
