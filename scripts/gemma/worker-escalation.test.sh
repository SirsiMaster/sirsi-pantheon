#!/bin/bash
# Native Gemma boundary regression guard. The retired shell worker's
# needs_escalation() extraction is intentionally gone: the Go worker always
# produces a text draft and never transfers, binds, or completes by keyword.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
go test ./cmd/sirsi -run '^TestGemmaWorkerBoundaryPromptAlwaysDraftsWithoutEscalation$' -count=1
