#!/bin/bash
# Host-wide queue for the Ma'at pre-push gate. Several agents push from one Mac, and the gate runs
# golangci-lint, which refuses to run twice at once (and its stderr used to be discarded), so two
# overlapping pushes failed one of them with "golangci-lint failed" for no reason in its code
# (2026-10-06: the release train lost a run to the headless ra worker's push). The gate now waits
# its turn instead. Portable to macOS bash 3.2: mkdir is the atomic primitive (no flock on macOS).
#
#   gate_lock_acquire   waits (default up to 30 min), reclaims a lock whose holder is dead
#   gate_lock_release   idempotent; also installed as the EXIT trap by acquire
# MAAT_GATE_LOCK=off disables it; MAAT_GATE_LOCK_DIR and MAAT_GATE_LOCK_WAIT_SECS override defaults.

gate_lock_dir() { echo "${MAAT_GATE_LOCK_DIR:-$HOME/.sirsi/locks/maat-gate.lock}"; }

gate_lock_release() {
    local d; d="$(gate_lock_dir)"
    [ -f "$d/pid" ] && [ "$(cat "$d/pid" 2>/dev/null)" = "$$" ] && rm -rf "$d"
    return 0
}

gate_lock_acquire() {
    [ "${MAAT_GATE_LOCK:-}" = "off" ] && return 0
    local d waited=0 max="${MAAT_GATE_LOCK_WAIT_SECS:-1800}" holder said=0
    d="$(gate_lock_dir)"; mkdir -p "$(dirname "$d")"
    while ! mkdir "$d" 2>/dev/null; do
        holder="$(cat "$d/pid" 2>/dev/null || true)"
        # A holder that is gone (or a lock with no owner recorded for over 30 s) is stale.
        if [ -n "$holder" ] && ! kill -0 "$holder" 2>/dev/null; then rm -rf "$d"; continue; fi
        if [ -z "$holder" ] && [ -d "$d" ] && [ "$(( $(date +%s) - $(stat -f %m "$d" 2>/dev/null || echo 0) ))" -gt 30 ]; then rm -rf "$d"; continue; fi
        if [ "$said" = 0 ]; then echo "  𓆄 Ma'at: another gate is running (pid ${holder:-?}); waiting for it to finish..."; said=1; fi
        if [ "$waited" -ge "$max" ]; then echo "  ❌ Ma'at: waited ${max}s for the gate lock held by pid ${holder:-?}; not pushing"; return 1; fi
        sleep 2; waited=$((waited + 2))
    done
    echo "$$" > "$d/pid"
    trap gate_lock_release EXIT
    return 0
}
