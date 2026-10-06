#!/bin/bash
# Ma'at consults the known-failure catalog on EVERY failing gate step, before anyone (human or
# model) spends time or tokens on it. Sourced by .githooks/pre-push.
#
#   gate_run <step> <command...>   run the command with output captured; on failure print the
#                                  failure lines, ask Ma'at, and return the command's status
#   gate_advise <step> <file>      ask Ma'at about a captured output file (known -> cause + fix;
#                                  unknown -> how to record it)
#
# The advisor (cmd/maat-advise) imports only the catalog package, so it still builds when the
# code under test does not compile. If it cannot run at all the gate says so and carries on:
# advising never changes the gate's verdict. MAAT_ADVISE=off disables it.

GATE_OUT="${GATE_OUT:-$(mktemp -d)}"

gate_advise() {
    local step="$1" file="$2"
    [ "${MAAT_ADVISE:-}" = "off" ] && return 0
    [ -s "$file" ] || return 0
    local root; root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
    if ! (cd "$root" && go run ./cmd/maat-advise --step "$step" < "$file") 2>"$GATE_OUT/advise.err"; then
        echo "  𓆄 Ma'at: the advisor could not run ($(head -c 160 "$GATE_OUT/advise.err" | tr '\n' ' ')); read the failure above."
    fi
    return 0
}

gate_show_failure() {
    local file="$1" lines
    # Prefer the lines that name the failure; fall back to the tail.
    lines="$(grep -E '^(--- FAIL|FAIL|panic:|fatal error:)|_test\.go:[0-9]+|\.go:[0-9]+:[0-9]+:|^#|error:|Error:' "$file" | head -40)"
    if [ -z "$lines" ]; then lines="$(tail -40 "$file")"; fi
    printf '%s\n' "$lines" | sed 's/^/     /'
}

gate_run() {
    local step="$1"; shift
    local log="$GATE_OUT/$step.log"
    "$@" >"$log" 2>&1
    local rc=$?
    # The command's own status, captured before anything else runs. (An `if cmd; then ..; fi`
    # with no else yields 0 when cmd fails, which would make every failing step look green.)
    [ "$rc" -eq 0 ] && return 0
    gate_show_failure "$log"
    gate_advise "$step" "$log"
    return "$rc"
}
