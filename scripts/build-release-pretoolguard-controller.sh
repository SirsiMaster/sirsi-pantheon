#!/bin/bash
# Reviewed-controller signing bridge for a clean, separately pinned source tree.
# No publication, installation, or release retry is performed by this script.
set -euo pipefail
if [[ $# != 5 ]]; then
  echo 'usage: build-release-pretoolguard-controller.sh <source-commit> <version> <private-attempt> <controller-root> <controller-commit>' >&2
  exit 2
fi
commit=$1 version=$2 attempt=$3 controller=$4 controller_commit=$5
[[ "$commit" =~ ^[0-9a-f]{40}$ && "$controller_commit" =~ ^[0-9a-f]{40}$ && "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([\-][A-Za-z0-9.\-]+)?$ ]] || exit 2
[[ "$attempt" = /* && -d "$attempt" && ! -L "$attempt" && $(/usr/bin/stat -f '%u:%Lp' "$attempt") == "$(id -u):700" ]] || exit 2
[[ "$controller" = /* && -d "$controller" && ! -L "$controller" ]] || exit 2
source_root=$(git rev-parse --show-toplevel)
[[ "$(git rev-parse 'HEAD^{commit}')" == "$commit" && -z "$(git status --porcelain --untracked-files=normal)" ]] || { echo 'source identity or cleanliness mismatch' >&2; exit 1; }
[[ "$(git -C "$controller" rev-parse 'HEAD^{commit}')" == "$controller_commit" && -z "$(git -C "$controller" status --porcelain --untracked-files=normal)" ]] || { echo 'controller identity or cleanliness mismatch' >&2; exit 1; }
# Keep signer transport and entitlement policy identical to the tagged source.
/usr/bin/cmp -s "$source_root/tools/signing/sirsi-sign" "$controller/tools/signing/sirsi-sign"
/usr/bin/cmp -s "$source_root/scripts/verify-pretoolguard-entitlements.py" "$controller/scripts/verify-pretoolguard-entitlements.py"
[[ "${PANTHEON_SIGN_CLIENT:-}" == "$controller/tools/signing/sirsi-sign" ]] || { echo 'controller signer path mismatch' >&2; exit 1; }
# Record both source and controller identities before any signer operation.
/usr/bin/python3 -B - "$attempt/controller-provenance.json" "$commit" "$controller_commit" "$controller/scripts/compare-pretoolguard-macho.py" "$controller/scripts/sign-pretoolguard-artifact.sh" <<'PY'
import hashlib, json, os, sys
path, source, controller, helper, signer = sys.argv[1:]
def digest(p):
    with open(p, 'rb') as stream:
        return hashlib.sha256(stream.read()).hexdigest()
value = {'source_commit': source, 'controller_commit': controller,
         'comparator_sha256': digest(helper), 'signing_bridge_sha256': digest(signer)}
with open(path, 'x') as out:
    json.dump(value, out, sort_keys=True); out.write('\n'); out.flush(); os.fsync(out.fileno())
directory = os.open(os.path.dirname(path), os.O_RDONLY)
try:
    os.fsync(directory)
finally:
    os.close(directory)
with open(path, 'r') as check:
    if json.load(check) != value:
        raise SystemExit('controller provenance readback mismatch')
PY
cd "$source_root"
local_host=$(/usr/sbin/ioreg -rd1 -c IOPlatformExpertDevice | /usr/bin/awk -F '"' '/"IOPlatformUUID"/{print $4}')
remote_host=$(/usr/bin/ssh -o ConnectTimeout=10 -o BatchMode=yes -o StrictHostKeyChecking=yes -o PasswordAuthentication=no -o KbdInteractiveAuthentication=no sirsi-m1 "/usr/sbin/ioreg -rd1 -c IOPlatformExpertDevice | /usr/bin/awk -F '\"' '/\"IOPlatformUUID\"/{print \$4}'")
[[ "$local_host" =~ ^[A-Fa-f0-9-]{36}$ && "$remote_host" == "$local_host" ]] || { echo 'signer host does not match qualified release runner' >&2; exit 1; }
guard_id=sirsi-display-power-guard
installer_id=sirsi-pretoolguard-install
mkdir -m 700 "$attempt/build" "$attempt/guard" "$attempt/installer" "$attempt/package"
bash scripts/build-pretoolguard-artifact.sh "$commit" "$attempt/build"
builds=("$attempt/build"/pretoolguard.*)
[[ ${#builds[@]} == 1 && -f "${builds[0]}/ACCEPTED" ]] || { echo 'ambiguous build output' >&2; exit 1; }
build=${builds[0]}
for name in sirsi-display-power-guard sirsi-pretoolguard-install; do
  case "$name" in sirsi-display-power-guard) stage="$attempt/guard"; identifier=$guard_id;; *) stage="$attempt/installer"; identifier=$installer_id;; esac
  /bin/cp -p "$build/$name" "$stage/$name"
  unsigned=$(/usr/bin/shasum -a 256 "$stage/$name" | /usr/bin/awk '{print $1}')
  bash "$controller/scripts/sign-pretoolguard-artifact.sh" "$stage/$name" "$unsigned" "$identifier"
done
guards=("$attempt/guard"/signing.*)
installers=("$attempt/installer"/signing.*)
[[ ${#guards[@]} == 1 && ${#installers[@]} == 1 && -f "${guards[0]}/ACCEPTED" && -f "${installers[0]}/ACCEPTED" ]] || { echo 'ambiguous signing output' >&2; exit 1; }
guard="${guards[0]}/signed-sirsi-display-power-guard"
installer="${installers[0]}/signed-sirsi-pretoolguard-install"
guard_hash=$(/usr/bin/shasum -a 256 "$guard" | /usr/bin/awk '{print $1}')
installer_hash=$(/usr/bin/shasum -a 256 "$installer" | /usr/bin/awk '{print $1}')
PANTHEON_SIGN_CLIENT="$source_root/tools/signing/sirsi-sign" bash scripts/package-pretoolguard-artifact.sh "$guard" "$guard_hash" "$guard_id" "$installer" "$installer_hash" "$installer_id" "$attempt/package"
packages=("$attempt/package"/pretoolguard-package.*)
[[ ${#packages[@]} == 1 && -f "${packages[0]}/ACCEPTED" ]] || { echo 'ambiguous package output' >&2; exit 1; }
mkdir -p "$source_root/bin"
/usr/bin/python3 -B - "${packages[0]}/signed-pretoolguard.dmg" "$source_root/bin/SirsiPantheon-$version-arm64-pretoolguard.dmg" "$attempt/payload-manifest.json" "$guard_hash" "$installer_hash" "$guard_id" "$installer_id" <<'PY'
import json, os, shutil, sys
source, target, manifest, guard_hash, installer_hash, guard_id, installer_id = sys.argv[1:]
with open(source, 'rb') as src, open(target, 'xb') as dst:
    shutil.copyfileobj(src, dst, 1024 * 1024); dst.flush(); os.fsync(dst.fileno())
payload = {'sirsi-display-power-guard': {'sha256': guard_hash, 'identifier': guard_id},
           'sirsi-pretoolguard-install': {'sha256': installer_hash, 'identifier': installer_id}}
with open(manifest, 'x') as out:
    json.dump(payload, out, sort_keys=True); out.write('\n'); out.flush(); os.fsync(out.fileno())
PY
echo "Source and reviewed controller custody retained: $attempt"
