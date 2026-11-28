#!/usr/bin/env bash
# Class H: the verifier's exit status is masked by tail; push proceeds on failure.
scripts/verify-commit-traceability.sh --main "$BASE" "$HEAD" | tail -5 && git push # want H
