#!/bin/bash
# Wrapper for the claude-home router worker (nexus fabric fix 2026-07-23).
# Sources the OAuth token from a 600-mode file so no credential lives in the
# plist (the quarantined pantheon worker embedded it in plaintext — scrubbed).
set -u
export PATH="$HOME/.local/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
[ -f "$HOME/.sirsi/secrets/claude-worker.env" ] && . "$HOME/.sirsi/secrets/claude-worker.env"
export SIRSI_CLAUDE_WORKER_AGENT=claude-home
export SIRSI_CLAUDE_WORKER_REPO="$HOME/Development/sirsi-pantheon"
# claude-home's core duty is judgment (binds, reviews) — frontier per tiering canon A30.
export SIRSI_CLAUDE_WORKER_MODEL=claude-opus-4-8
export SIRSI_SUPERVISOR=0 SIRSI_NO_TOAST=1
exec "$HOME/.local/bin/sirsi-claude-worker.sh"
