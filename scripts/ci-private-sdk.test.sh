#!/usr/bin/env bash
set -euo pipefail
root=$(mktemp -d "${TMPDIR:-/tmp}/private-sdk-test.XXXXXXXX")
trap 'rmdir "$root"' EXIT
wrapper="$(cd "$(dirname "$0")" && pwd)/ci-private-sdk.sh"
expect_status() {
  local expected=$1; shift
  local actual=0
  "$@" || actual=$?
  [[ $actual == "$expected" ]] || { echo "wanted $expected, got $actual" >&2; exit 1; }
  if compgen -G "$root/pantheon-sdk-auth.*" >/dev/null; then echo 'credential residue' >&2; exit 1; fi
}
export RUNNER_TEMP="$root" APOLLO_CI_INFERENCE_READ_KEY=fixture-not-a-key APOLLO_CI_MERCURY_READ_KEY=fixture-not-a-key
expect_status 77 env GITHUB_ACTIONS=true GITHUB_EVENT_NAME=pull_request GITHUB_REPOSITORY=SirsiMaster/sirsi-pantheon CI_HEAD_REPO=stranger/fork CI_AUTHOR_ASSOCIATION=OWNER bash "$wrapper" true
expect_status 77 env GITHUB_ACTIONS=true GITHUB_EVENT_NAME=pull_request GITHUB_REPOSITORY=SirsiMaster/sirsi-pantheon CI_HEAD_REPO=SirsiMaster/sirsi-pantheon CI_AUTHOR_ASSOCIATION=NONE bash "$wrapper" true
expect_status 77 env GITHUB_ACTIONS=true GITHUB_EVENT_NAME=pull_request_target bash "$wrapper" true
expect_status 78 env -u APOLLO_CI_INFERENCE_READ_KEY GITHUB_ACTIONS=false bash "$wrapper" true
expect_status 23 env GITHUB_ACTIONS=false bash "$wrapper" bash -c 'exit 23'
expect_status 0 env GITHUB_ACTIONS=true GITHUB_EVENT_NAME=pull_request GITHUB_REPOSITORY=SirsiMaster/sirsi-pantheon CI_HEAD_REPO=SirsiMaster/sirsi-pantheon CI_AUTHOR_ASSOCIATION=OWNER bash "$wrapper" bash -c '
  test -z "${APOLLO_CI_INFERENCE_READ_KEY:-}${APOLLO_CI_MERCURY_READ_KEY:-}"
  test -f "$GIT_CONFIG_GLOBAL"
  test "$(git config --file "$GIT_CONFIG_GLOBAL" --get url.ssh://git@github-inference-ci/SirsiMaster/sirsi-inference.insteadOf)" = https://github.com/SirsiMaster/sirsi-inference
  test "$(git config --file "$GIT_CONFIG_GLOBAL" --get url.ssh://git@github-mercury-ci/SirsiMaster/sirsi-mercury.insteadOf)" = https://github.com/SirsiMaster/sirsi-mercury
'
# The secret-file umask stays inside the wrapper: the wrapped build/test runs with the caller's umask,
# while the key files it wrote are still owner-only.
expect_status 0 bash -c 'umask 022; env GITHUB_ACTIONS=false bash "$0" bash -c "test \"\$(umask)\" = 0022 && test \"\$(stat -f %Lp \$(dirname \$GIT_CONFIG_GLOBAL)/inference 2>/dev/null || stat -c %a \$(dirname \$GIT_CONFIG_GLOBAL)/inference)\" = 600"' "$wrapper"
echo 'private SDK trust, refusal, secret-env scrub and cleanup, umask fixtures PASS'
