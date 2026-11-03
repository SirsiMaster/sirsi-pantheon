#!/bin/bash
# Prepare the guard image for the existing release job. No publication/install.
# Invoke only within the release job's own qualified host window and admission.
set -euo pipefail
if [[ $# != 3 ]]; then
  echo 'usage: build-release-pretoolguard.sh <exact-commit> <version> <private-attempt-directory>' >&2
  exit 2
fi
commit=$1 version=$2 attempt=$3
[[ "$commit" =~ ^[0-9a-f]{40}$ && "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([\-][A-Za-z0-9.\-]+)?$ ]] || exit 2
[[ "$attempt" = /* && -d "$attempt" && ! -L "$attempt" && $(/usr/bin/stat -f '%u:%Lp' "$attempt") == "$(id -u):700" ]] || exit 2
root=$(git rev-parse --show-toplevel)
cd "$root"
# Bind the strict SSH alias to this already qualified M1, without printing the
# hardware identifier. A pinned alias alone does not prove host placement.
local_host=$(/usr/sbin/ioreg -rd1 -c IOPlatformExpertDevice | /usr/bin/awk -F '"' '/"IOPlatformUUID"/{print $4}')
remote_host=$(/usr/bin/ssh -o ConnectTimeout=10 -o BatchMode=yes -o StrictHostKeyChecking=yes -o PasswordAuthentication=no -o KbdInteractiveAuthentication=no sirsi-m1 "/usr/sbin/ioreg -rd1 -c IOPlatformExpertDevice | /usr/bin/awk -F '\"' '/\"IOPlatformUUID\"/{print \$4}'")
[[ "$local_host" =~ ^[A-Fa-f0-9-]{36}$ && "$remote_host" == "$local_host" ]] || { echo 'signer host does not match qualified release runner' >&2; exit 1; }
# The signer must actually emit these identities; the signing helper refuses
# output if it does not. This is not a claim that the remote signer is qualified.
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
  bash scripts/sign-pretoolguard-artifact.sh "$stage/$name" "$unsigned" "$identifier"
done
guards=("$attempt/guard"/signing.*)
installers=("$attempt/installer"/signing.*)
[[ ${#guards[@]} == 1 && ${#installers[@]} == 1 && -f "${guards[0]}/ACCEPTED" && -f "${installers[0]}/ACCEPTED" ]] || { echo 'ambiguous signing output' >&2; exit 1; }
guard="${guards[0]}/signed-sirsi-display-power-guard"
installer="${installers[0]}/signed-sirsi-pretoolguard-install"
guard_hash=$(/usr/bin/shasum -a 256 "$guard" | /usr/bin/awk '{print $1}')
installer_hash=$(/usr/bin/shasum -a 256 "$installer" | /usr/bin/awk '{print $1}')
bash scripts/package-pretoolguard-artifact.sh "$guard" "$guard_hash" "$guard_id" "$installer" "$installer_hash" "$installer_id" "$attempt/package"
packages=("$attempt/package"/pretoolguard-package.*)
[[ ${#packages[@]} == 1 && -f "${packages[0]}/ACCEPTED" ]] || { echo 'ambiguous package output' >&2; exit 1; }
mkdir -p "$root/bin"
# Exclusively create the release candidate and manifest; never replace outputs
# from another attempt. The publisher independently re-verifies both payloads.
/usr/bin/python3 -B - "${packages[0]}/signed-pretoolguard.dmg" "$root/bin/SirsiPantheon-$version-arm64-pretoolguard.dmg" "$attempt/payload-manifest.json" "$guard_hash" "$installer_hash" "$guard_id" "$installer_id" <<'PY'
import json, os, shutil, sys
source, target, manifest, guard_hash, installer_hash, guard_id, installer_id = sys.argv[1:]
with open(source, 'rb') as src, open(target, 'xb') as dst:
    shutil.copyfileobj(src, dst, 1024 * 1024)
    dst.flush()
    os.fsync(dst.fileno())
payload = {'sirsi-display-power-guard': {'sha256': guard_hash, 'identifier': guard_id},
           'sirsi-pretoolguard-install': {'sha256': installer_hash, 'identifier': installer_id}}
with open(manifest, 'x') as out:
    json.dump(payload, out, sort_keys=True)
    out.write('\n')
    out.flush()
    os.fsync(out.fileno())
PY
echo "Guard release custody retained: $attempt"
