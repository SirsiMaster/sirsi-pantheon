#!/bin/bash
# Native release supervisor selection. An independently enrolled v164 pair is
# required; this helper never stages, installs or changes either launcher.
set -euo pipefail

fail() { echo "release Ma’at preflight: $*" >&2; exit 1; }
mode=${1:-}
case "$mode" in verify) [[ $# == 1 ]] || fail 'verify takes no payload' ;; run) [[ $# -gt 1 ]] || fail 'run needs a payload'; shift ;; *) fail 'use verify or run' ;; esac
source_path=scripts/maat/admitted-run.py

if [[ ${GITHUB_REF:-} == refs/tags/v0.24.164 || ${BUILD_VERSION:-} == 0.24.164 ]]; then
    [[ ${GITHUB_REF:-} == refs/tags/v0.24.164 && ${BUILD_VERSION:-} == 0.24.164 ]] || fail 'v164 tag/version disagree'
    [[ ${PANTHEON_RUNNER_NAME:-} == m1-pantheon && ${PANTHEON_RUNNER_OS:-} == macOS && ${PANTHEON_RUNNER_ARCH:-} == ARM64 ]] || fail 'v164 qualified runner context required'
    [[ $(/usr/bin/uname -s) == Darwin && $(/usr/bin/uname -m) == arm64 && $(/usr/sbin/sysctl -n hw.model) == MacBookPro18,1 ]] || fail 'v164 native M1 required'
    [[ $HOME == /Users/sirsimasterdev && $(/usr/bin/id -u) == 501 && $(/usr/bin/id -ru) == 501 ]] || fail 'v164 enrolled runner account required'
    [[ ${SIRSI_ROUTER_URL:-} == spool:///var/sirsipantheon/relay && ${SIRSI_RELAY_TRUST_GROUP:-} == _sirsipantheon ]] || fail 'v164 canonical protected spool required'

    runtime=/usr/local/libexec/sirsi-pantheon/release-runtimes/v0.24.164
    supervisor="$runtime/admitted-run.py"
    framework=/Library/Developer/CommandLineTools/Library/Frameworks/Python3.framework/Versions/3.9
    python="$framework/Resources/Python.app/Contents/MacOS/Python"
    cli=/usr/local/libexec/sirsi-pantheon/sirsi
    supervisor_sha=9515e1a830147522d6e2a6a9c465b1e9ca8a6cfbf58939ea24b1ae63b939fdc0
    pin() {
        local result
        [[ -f $1 && ! -L $1 ]] || fail "not a regular pinned file: $1"
        result=$(/usr/bin/shasum -a 256 "$1") || fail "cannot hash: $1"
        [[ ${result%% *} == "$2" ]] || fail "hash mismatch: $1"
    }
    for parent in / /usr /usr/local /usr/local/libexec /usr/local/libexec/sirsi-pantheon \
        /usr/local/libexec/sirsi-pantheon/release-runtimes "$runtime" \
        /Library /Library/Developer /Library/Developer/CommandLineTools \
        /Library/Developer/CommandLineTools/Library /Library/Developer/CommandLineTools/Library/Frameworks \
        "${framework%/Versions/3.9}" "${framework%/3.9}" "$framework" "$framework/Resources" \
        "$framework/Resources/Python.app" "$framework/Resources/Python.app/Contents" \
        "$framework/Resources/Python.app/Contents/MacOS"; do
        [[ -d $parent && ! -L $parent ]] || fail "missing directory or symlink: $parent"
        [[ $(/usr/bin/stat -f '%u' "$parent") == 0 ]] || fail "unexpected directory owner: $parent"
        [[ $(/bin/ls -lde "$parent" | /usr/bin/wc -l | /usr/bin/tr -d ' ') == 1 ]] || fail "ACL or unreadable directory: $parent"
        bits=$(/usr/bin/stat -f '%OLp' "$parent")
        [[ $bits =~ ^[0-7]{3,4}$ ]] && (( (8#$bits & 022) == 0 )) || fail "writable directory: $parent"
    done
    [[ -f $supervisor && ! -L $supervisor ]] || fail 'v164 isolated supervisor not enrolled'
    [[ $(/usr/bin/stat -f '%u:%OLp:%l' "$supervisor") == 0:444:1 ]] || fail 'v164 supervisor requires root, mode 0444, one link'
    for protected in "$supervisor" "$python" "$framework/Python3" "$cli"; do
        [[ -f $protected && ! -L $protected ]] || fail "missing protected file or symlink: $protected"
        [[ $(/usr/bin/stat -f '%u:%l' "$protected") == 0:1 ]] || fail "protected file must be root-owned and single-link: $protected"
        [[ $(/bin/ls -lde "$protected" | /usr/bin/wc -l | /usr/bin/tr -d ' ') == 1 ]] || fail "ACL or unreadable file: $protected"
        bits=$(/usr/bin/stat -f '%OLp' "$protected")
        [[ $bits =~ ^[0-7]{3,4}$ ]] && (( (8#$bits & 022) == 0 )) || fail "writable file: $protected"
    done
    pin "$source_path" "$supervisor_sha"
    pin "$supervisor" "$supervisor_sha"
    pin "$python" 295b2fd03b2b05f12b6f4887d70dbf43e5358173f8eec0909c121c9bdec2756f
    pin "$framework/Python3" 6d3f7badc96687779a34b0bb10b74800b69cd97f7cda4a39a41a35e5d7175d01
    pin "$cli" 84a1458f7b85557d91b43063f073fe5b65eb682453546078e82bb85030119688
    for executable in "$python" "$framework/Python3"; do
        /usr/bin/codesign --verify --strict -R='anchor apple' "$executable" || fail 'v164 interpreter signature'
    done
    /usr/bin/codesign --verify --strict -R='anchor apple generic and certificate leaf[subject.OU] = "9D382WV988"' "$cli" || fail 'v164 protected CLI signature'
    [[ $mode == verify ]] || exec "$python" -I -S -B "$supervisor" --scope fabric --protected-cli "$@"
else
    supervisor="$HOME/.local/bin/sirsi-maat-run"
    [[ -x $supervisor ]] || fail 'installed Ma’at supervisor is missing'
    /usr/bin/cmp -s "$supervisor" "$source_path" || fail 'installed Ma’at supervisor does not match release source'
    [[ $mode == verify ]] || exec "$supervisor" "$@"
fi
