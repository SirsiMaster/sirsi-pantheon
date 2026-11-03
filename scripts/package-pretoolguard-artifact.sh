#!/bin/bash
# Prepare a signed, notarized guard DMG via the existing signer; never publish.
set -euo pipefail
if [ "$#" -ne 7 ]; then
  echo 'usage: bash scripts/package-pretoolguard-artifact.sh <signed-guard> <guard-sha256> <guard-identifier> <signed-installer> <installer-sha256> <installer-identifier> <output-directory>' >&2
  exit 2
fi
binary=$1
expected=$2
identifier=$3
installer=$4
installer_expected=$5
installer_identifier=$6
output=$7
[[ "$binary" = /* && -f "$binary" && ! -L "$binary" ]] || { echo 'absolute regular signed guard required' >&2; exit 2; }
[[ "$installer" = /* && -f "$installer" && ! -L "$installer" ]] || { echo 'absolute regular signed installer required' >&2; exit 2; }
[[ "$expected" =~ ^[0-9a-f]{64}$ && "$identifier" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ && "$installer_expected" =~ ^[0-9a-f]{64}$ && "$installer_identifier" =~ ^[A-Za-z0-9][A-Za-z0-9.-]*$ ]] || { echo 'invalid digest or code identifier' >&2; exit 2; }
[[ "$output" = /* && -d "$output" && ! -L "$output" ]] || { echo 'existing absolute output directory required' >&2; exit 2; }
[[ $(uname -s) == Darwin && $(uname -m) == arm64 ]] || { echo 'native Darwin arm64 required' >&2; exit 1; }
[[ $(/usr/bin/stat -f '%u:%Lp' "$output") == "$(id -u):700" ]] || { echo 'private owned output directory required' >&2; exit 1; }
: "${PANTHEON_SIGN_CLIENT:?qualified existing signer client required}"
: "${SIRSI_SIGN_HOSTS:?explicit qualified signing host required}"
root=$(cd "$(dirname "$0")/.." && pwd -P)
[[ "$PANTHEON_SIGN_CLIENT" == "$root/tools/signing/sirsi-sign" && -x "$PANTHEON_SIGN_CLIENT" && ! -L "$PANTHEON_SIGN_CLIENT" && "$SIRSI_SIGN_HOSTS" == sirsi-m1 ]] || { echo 'qualified repository signer and M1 route required' >&2; exit 1; }
requirement="=identifier \"$identifier\" and anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists"
installer_requirement="=identifier \"$installer_identifier\" and anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists"
/usr/bin/codesign --verify --strict -R "$requirement" "$binary"
[[ $(/usr/bin/shasum -a 256 "$binary" | /usr/bin/awk '{print $1}') == "$expected" ]] || { echo 'guard digest mismatch' >&2; exit 1; }
/usr/bin/codesign --verify --strict -R "$installer_requirement" "$installer"
[[ $(/usr/bin/shasum -a 256 "$installer" | /usr/bin/awk '{print $1}') == "$installer_expected" ]] || { echo 'installer digest mismatch' >&2; exit 1; }
dir=$(mktemp -d "$output/pretoolguard-package.XXXXXXXX")
echo "Retaining package directory: $dir"
printf '%s\n' 'INCOMPLETE: do not use; signing or mount outcome may be ambiguous' > "$dir/INCOMPLETE"
mkdir "$dir/payload"
/bin/cp -p "$binary" "$dir/payload/sirsi-display-power-guard"
/bin/cp -p "$installer" "$dir/payload/sirsi-pretoolguard-install"
[[ $(/usr/bin/shasum -a 256 "$dir/payload/sirsi-display-power-guard" | /usr/bin/awk '{print $1}') == "$expected" ]] || { echo 'staged guard digest mismatch' >&2; exit 1; }
/usr/bin/codesign --verify --strict -R "$requirement" "$dir/payload/sirsi-display-power-guard"
[[ $(/usr/bin/shasum -a 256 "$dir/payload/sirsi-pretoolguard-install" | /usr/bin/awk '{print $1}') == "$installer_expected" ]] || { echo 'staged installer digest mismatch' >&2; exit 1; }
/usr/bin/codesign --verify --strict -R "$installer_requirement" "$dir/payload/sirsi-pretoolguard-install"
/usr/bin/python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$dir/payload/sirsi-display-power-guard"
/usr/bin/python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$dir/payload/sirsi-pretoolguard-install"
LC_ALL=C /usr/bin/codesign -d --verbose=4 "$dir/payload/sirsi-display-power-guard" > "$dir/guard-signature-details.txt" 2>&1
/usr/bin/grep -q '^Timestamp=' "$dir/guard-signature-details.txt" || { echo 'guard secure timestamp not reported' >&2; exit 1; }
/usr/bin/grep -E -q '^CodeDirectory .*flags=.*\(.*runtime.*\)' "$dir/guard-signature-details.txt" || { echo 'guard hardened runtime flag not reported' >&2; exit 1; }
LC_ALL=C /usr/bin/codesign -d --verbose=4 "$dir/payload/sirsi-pretoolguard-install" > "$dir/installer-signature-details.txt" 2>&1
/usr/bin/grep -q '^Timestamp=' "$dir/installer-signature-details.txt" || { echo 'installer secure timestamp not reported' >&2; exit 1; }
/usr/bin/grep -E -q '^CodeDirectory .*flags=.*\(.*runtime.*\)' "$dir/installer-signature-details.txt" || { echo 'installer hardened runtime flag not reported' >&2; exit 1; }
/usr/bin/hdiutil create -quiet -format UDZO -srcfolder "$dir/payload" "$dir/pretoolguard.dmg"
"$PANTHEON_SIGN_CLIENT" "$dir/pretoolguard.dmg" dmg
signed="$dir/signed-pretoolguard.dmg"
[[ -f "$signed" && ! -L "$signed" && -f "$signed.receipt.txt" && ! -L "$signed.receipt.txt" ]] || { echo 'signed image or receipt missing' >&2; exit 1; }
/usr/bin/codesign --verify --strict -R '=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists' "$signed"
LC_ALL=C /usr/bin/codesign -d --verbose=4 "$signed" > "$dir/image-signature-details.txt" 2>&1
/usr/bin/grep -q '^Timestamp=' "$dir/image-signature-details.txt" || { echo 'image secure timestamp not reported' >&2; exit 1; }
/usr/bin/xcrun stapler validate "$signed"
mkdir "$dir/mount"
mounted=0
cleanup_mount() {
  if [[ "$mounted" == 1 ]]; then
    mounted=0
    /usr/bin/hdiutil detach "$dir/mount" || echo "WARNING: detach failed; inspect mount at $dir/mount" >&2
  fi
}
trap cleanup_mount EXIT
/usr/bin/hdiutil attach -quiet -readonly -nobrowse -mountpoint "$dir/mount" "$signed"
mounted=1
/usr/bin/python3 - "$dir/mount" <<'PY'
import os, stat, sys
p = sys.argv[1]
names = {'sirsi-display-power-guard', 'sirsi-pretoolguard-install'}
if set(os.listdir(p)) != names:
    raise SystemExit('unexpected image payload')
for name in names:
    s = os.lstat(os.path.join(p, name))
    if not stat.S_ISREG(s.st_mode) or s.st_nlink != 1 or stat.S_IMODE(s.st_mode) != 0o755:
        raise SystemExit('payload must be an executable single-link regular file')
PY
[[ $(/usr/bin/shasum -a 256 "$dir/mount/sirsi-display-power-guard" | /usr/bin/awk '{print $1}') == "$expected" ]] || { echo 'returned image payload differs' >&2; exit 1; }
/usr/bin/codesign --verify --strict -R "$requirement" "$dir/mount/sirsi-display-power-guard"
[[ $(/usr/bin/shasum -a 256 "$dir/mount/sirsi-pretoolguard-install" | /usr/bin/awk '{print $1}') == "$installer_expected" ]] || { echo 'returned image installer differs' >&2; exit 1; }
/usr/bin/codesign --verify --strict -R "$installer_requirement" "$dir/mount/sirsi-pretoolguard-install"
mounted=0
if ! /usr/bin/hdiutil detach "$dir/mount"; then
  echo "detach failed; inspect mount at $dir/mount" >&2
  exit 1
fi
/usr/bin/shasum -a 256 "$signed" > "$dir/signed-dmg.sha256"
printf '%s\n' "$expected" > "$dir/guard.sha256"
printf '%s\n' "$installer_expected" > "$dir/installer.sha256"
printf '%s\n' 'ACCEPTED: local packaging checks and detach completed' > "$dir/INCOMPLETE"
mv "$dir/INCOMPLETE" "$dir/ACCEPTED"
echo 'Signed/stapled image staged; publisher readback and installation remain separate.'
