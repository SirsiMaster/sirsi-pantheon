#!/bin/bash
# Continue the original v0.24.164 guard build without signing the guard again.
set -euo pipefail
[[ $# == 3 ]] || { echo 'usage: recover-v164-pretoolguard.sh <source-root> <original-evidence> <private-attempt>' >&2; exit 2; }
source_root=$1 evidence=$2 attempt=$3
commit=ca31d2a8984c02e54fa762de4c63333cd92bdf22
original_guard=43b36b6f9ccab1a7eec3836037132a24c80b3fc3f105b013fb81bb0188948cff
original_installer=12d8e142c41d989a0c03e33b60a842ec730bbdb5f27516ff45b73f393eebf082
signed_guard=f324c7aa206567a6c0e5405af8e002381f0bd37db1e61b34e19ec8b761eba0ab
norm_input=909cd4bc7b44606427789cb8928279ce9c7e6b4f89fc5a9db28a10b7ca6cfde5
norm_output=54e9e266223a0c8eca43dc7c0fed266b094a672962e674830d6c8c2d0aff3bbf
root=$(cd "$(dirname "$0")/.." && pwd -P)
[[ "$source_root" = /* && "$evidence" = /* && "$attempt" = /* && -d "$attempt" && ! -L "$attempt" && $(stat -f '%u:%Lp' "$attempt") == "$(id -u):700" ]] || exit 2
[[ $(git -C "$source_root" rev-parse 'HEAD^{commit}') == "$commit" && -z $(git -C "$source_root" status --porcelain --untracked-files=normal) ]] || exit 1
cmp -s "$source_root/tools/signing/sirsi-sign" "$root/tools/signing/sirsi-sign"
cmp -s "$source_root/scripts/verify-pretoolguard-entitlements.py" "$root/scripts/verify-pretoolguard-entitlements.py"
[[ ${PANTHEON_SIGN_CLIENT:-} == "$root/tools/signing/sirsi-sign" && ${SIRSI_SIGN_HOSTS:-} == sirsi-m1 ]] || exit 1
digest() { /usr/bin/shasum -a 256 "$1" | /usr/bin/awk '{print $1}'; }
input="$evidence/guard/signing.m7L0tGqb/normalized.kxo2ka9R/input"
output="$evidence/guard/signing.m7L0tGqb/normalized.kxo2ka9R/output"
receipt="$evidence/guard/signing.m7L0tGqb/signed-sirsi-display-power-guard.receipt.txt"
guard="$HOME/sirsi-sign/outbox/b814fa10-48ce-428e-89d0-5e6c7d7005ca/sirsi-display-power-guard"
[[ $(digest "$input") == "$norm_input" && $(digest "$output") == "$norm_output" && $(digest "$guard") == "$signed_guard" ]] || exit 1
/usr/bin/grep -Fqx "input sha256 $original_guard" "$receipt"
/usr/bin/grep -Fqx "output sha256 $signed_guard" "$receipt"
/usr/bin/grep -Fq 'status: Accepted' "$receipt"
/usr/bin/grep -Fq '18e0bd68-8ac6-44d8-bbeb-585e2a4a2c3e' "$receipt"
mkdir -m 700 "$attempt/build" "$attempt/installer" "$attempt/package"
cd "$source_root"
bash scripts/build-pretoolguard-artifact.sh "$commit" "$attempt/build"
builds=("$attempt/build"/pretoolguard.*)
[[ ${#builds[@]} == 1 && -f ${builds[0]}/ACCEPTED ]] || exit 1
[[ $(digest "${builds[0]}/sirsi-display-power-guard") == "$original_guard" && $(digest "${builds[0]}/sirsi-pretoolguard-install") == "$original_installer" ]] || { echo 'original inputs not reproduced' >&2; exit 1; }
cp -p "${builds[0]}/sirsi-display-power-guard" "$attempt/rebuilt-normalized"
if /usr/bin/codesign -d "$attempt/rebuilt-normalized" >/dev/null 2>&1; then /usr/bin/codesign --remove-signature "$attempt/rebuilt-normalized"; fi
[[ $(digest "$attempt/rebuilt-normalized") == "$norm_input" ]] || exit 1
cp -p "$guard" "$attempt/signed-normalized"
/usr/bin/codesign --remove-signature "$attempt/signed-normalized"
[[ $(digest "$attempt/signed-normalized") == "$norm_output" ]] || exit 1
python3 -B "$root/scripts/compare-pretoolguard-macho.py" "$attempt/rebuilt-normalized" "$attempt/signed-normalized" "$attempt/guard-revalidation.json"
requirement='=identifier "sirsi-display-power-guard" and anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists'
/usr/bin/codesign --verify --strict -R "$requirement" "$guard"
python3 -B "$root/scripts/verify-pretoolguard-entitlements.py" "$guard"
cp -p "${builds[0]}/sirsi-pretoolguard-install" "$attempt/installer/sirsi-pretoolguard-install"
bash "$root/scripts/sign-pretoolguard-artifact.sh" "$attempt/installer/sirsi-pretoolguard-install" "$original_installer" sirsi-pretoolguard-install
installers=("$attempt/installer"/signing.*)
[[ ${#installers[@]} == 1 && -f ${installers[0]}/ACCEPTED ]] || exit 1
installer="${installers[0]}/signed-sirsi-pretoolguard-install"
installer_hash=$(digest "$installer")
PANTHEON_SIGN_CLIENT="$source_root/tools/signing/sirsi-sign" bash scripts/package-pretoolguard-artifact.sh "$guard" "$signed_guard" sirsi-display-power-guard "$installer" "$installer_hash" sirsi-pretoolguard-install "$attempt/package"
packages=("$attempt/package"/pretoolguard-package.*)
[[ ${#packages[@]} == 1 && -f ${packages[0]}/ACCEPTED ]] || exit 1
mkdir -p "$source_root/bin"
python3 -B - "${packages[0]}/signed-pretoolguard.dmg" "$source_root/bin/SirsiPantheon-0.24.164-arm64-pretoolguard.dmg" "$attempt/payload-manifest.json" "$signed_guard" "$installer_hash" <<'PY'
import json, os, shutil, sys
source, target, manifest, guard, installer = sys.argv[1:]
with open(source, 'rb') as src, open(target, 'xb') as dst:
    shutil.copyfileobj(src, dst); dst.flush(); os.fsync(dst.fileno())
with open(manifest, 'x') as out:
    json.dump({'sirsi-display-power-guard': {'sha256': guard, 'identifier': 'sirsi-display-power-guard'}, 'sirsi-pretoolguard-install': {'sha256': installer, 'identifier': 'sirsi-pretoolguard-install'}}, out, sort_keys=True)
    out.write('\n'); out.flush(); os.fsync(out.fileno())
PY
