#!/usr/bin/env bash
# Trusted builds only. New CI-only read keys; never a Mac/signing/bind credential.
set +x
set -euo pipefail
if [[ ${GITHUB_ACTIONS:-} == true ]]; then
  case ${GITHUB_EVENT_NAME:-} in
    push|workflow_dispatch) ;;
    pull_request)
      [[ ${CI_HEAD_REPO:-} == "${GITHUB_REPOSITORY:-}" ]] || { echo 'private SDK access refused for fork' >&2; exit 77; }
      case ${CI_AUTHOR_ASSOCIATION:-} in OWNER|MEMBER|COLLABORATOR) ;; *) echo 'private SDK access requires a trusted collaborator' >&2; exit 77;; esac ;;
    *) echo 'private SDK access refused for this event' >&2; exit 77 ;;
  esac
fi
[[ $# -gt 0 ]] || { echo 'usage: ci-private-sdk.sh command [args...]' >&2; exit 64; }
[[ -n ${APOLLO_CI_INFERENCE_READ_KEY:-} && -n ${APOLLO_CI_MERCURY_READ_KEY:-} ]] || { echo 'missing read-only private SDK credentials' >&2; exit 78; }
caller_umask=$(umask)
umask 077 # only for the key/config files written below
sdk_auth_dir=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/pantheon-sdk-auth.XXXXXXXX")
cleanup() {
  rm -f "$sdk_auth_dir/inference" "$sdk_auth_dir/mercury" "$sdk_auth_dir/config" "$sdk_auth_dir/known_hosts" "$sdk_auth_dir/gitconfig"
  rmdir "$sdk_auth_dir"
}
trap cleanup EXIT
printf '%s\n' "$APOLLO_CI_INFERENCE_READ_KEY" > "$sdk_auth_dir/inference"
printf '%s\n' "$APOLLO_CI_MERCURY_READ_KEY" > "$sdk_auth_dir/mercury"
unset APOLLO_CI_INFERENCE_READ_KEY APOLLO_CI_MERCURY_READ_KEY
# GitHub's Ed25519 key, independently retrieved from authenticated GET /meta.
printf '%s\n' 'github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl' > "$sdk_auth_dir/known_hosts"
for sdk in inference mercury; do
  printf 'Host github-%s-ci\n  HostName github.com\n  HostKeyAlias github.com\n  User git\n  IdentityFile "%s/%s"\n  IdentitiesOnly yes\n  BatchMode yes\n  PasswordAuthentication no\n  KbdInteractiveAuthentication no\n  StrictHostKeyChecking yes\n  UserKnownHostsFile "%s/known_hosts"\n' "$sdk" "$sdk_auth_dir" "$sdk" "$sdk_auth_dir" >> "$sdk_auth_dir/config"
done
export GIT_CONFIG_GLOBAL="$sdk_auth_dir/gitconfig"
git config --file "$GIT_CONFIG_GLOBAL" url.ssh://git@github-inference-ci/SirsiMaster/sirsi-inference.insteadOf https://github.com/SirsiMaster/sirsi-inference
git config --file "$GIT_CONFIG_GLOBAL" url.ssh://git@github-mercury-ci/SirsiMaster/sirsi-mercury.insteadOf https://github.com/SirsiMaster/sirsi-mercury
export GIT_SSH_COMMAND="ssh -F '$sdk_auth_dir/config'"
export GIT_TERMINAL_PROMPT=0 GOPRIVATE=github.com/SirsiMaster/sirsi-inference,github.com/SirsiMaster/sirsi-mercury
export GONOPROXY="$GOPRIVATE" GONOSUMDB="$GOPRIVATE"
umask "$caller_umask" # the wrapped build/test must not inherit the secret-file umask
"$@"
