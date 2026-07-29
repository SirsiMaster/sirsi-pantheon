#!/bin/bash
# worker-escalation.test.sh — regression guard for Gemma worker ASK-vs-SUBJECT routing.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKER="$ROOT/scripts/gemma/sirsi-gemma-worker.sh"

eval "$(awk '/^needs_escalation\(\)/,/^}/' "$WORKER")"

pass=0
fail=0

check() {
  local name="$1" want="$2" task="$3" body="$4"
  if needs_escalation "$body" "$task"; then
    got="yes"
  else
    got="no"
  fi
  if [ "$got" = "$want" ]; then
    pass=$((pass + 1))
    echo "ok - $name"
  else
    fail=$((fail + 1))
    echo "not ok - $name: got $got want $want" >&2
  fi
}

check "plan about security deploy stays local" no plan \
  "TASK: plan
Plan the security deploy checklist and mention PR approval steps."

check "draft about signing stays local" no draft \
  "TASK: draft
Draft release notes for the code-signing deployment."

check "summarize exploit discussion stays local" no summarize \
  "TASK: summarize
Summarize this thread about whether the finding is exploitable."

check "build text task stays local" no build \
  "TASK: build
Build the requested markdown from the embedded source."

check "general topic mention does not escalate" no "" \
  "This item discusses security, deploy, merge, signing, and PR review history."

check "binding verdict ask escalates" yes "" \
  "Please issue a binding security verdict for this PR."

check "tool action ask escalates" yes "" \
  "Approve and merge this PR."

check "exploitability signoff ask escalates" yes "" \
  "Is this exploitable? Sign off on the release if safe."

echo "passed=$pass failed=$fail"
[ "$fail" -eq 0 ]
