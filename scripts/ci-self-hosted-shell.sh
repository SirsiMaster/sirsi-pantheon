#!/usr/bin/env bash
# Run-step shell for CI jobs on the owner's self-hosted M5 runners.
# launchd exports the owner's live session settings to every agent, including
# these runners: SIRSI_ROUTER_URL (the live relay spool) and SSH_AUTH_SOCK (the
# owner's SSH agent). GitHub-hosted runners have neither, and tests must never
# see them. Variables this workflow sets on a step itself are kept.
# Usage (workflow): defaults.run.shell: bash scripts/ci-self-hosted-shell.sh {0}
keep=" SIRSI_ROUTER_DB SIRSI_SIZE "
for v in $(compgen -e); do
  case "$v" in
    SIRSI_*) [[ $keep == *" $v "* ]] || unset "$v" ;;
    SSH_AUTH_SOCK) unset "$v" ;;
  esac
done
exec bash --noprofile --norc -eo pipefail "$@"
