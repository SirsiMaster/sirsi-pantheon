#!/bin/bash
# Compatibility LaunchAgent entrypoint for the native fenced Gemma worker.
#
# All queue ownership, registration, heartbeat, retry evidence, and inference
# policy live in `sirsi gemma worker`. This wrapper intentionally has no queue
# logic and no Python/model fallback.
set -eu

REPO=${SIRSI_REPO:-$HOME/Development/sirsi-pantheon}
SIRSI=${SIRSI_BIN:-$HOME/.local/bin/sirsi}
cd "$REPO"
exec "$SIRSI" gemma worker "$@"
