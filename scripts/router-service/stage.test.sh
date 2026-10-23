#!/usr/bin/env bash
# stage.test.sh: the router service deploy path never asks Cloud Build to compile Go (it cannot fetch the private
# sirsi-inference module), and stage.sh builds a COPY-only context around a static linux/amd64 binary.
# Needs the private module reachable: run under scripts/ci-private-sdk.sh in CI.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
fail() { echo "FAIL: $*" >&2; exit 1; }

# 1. No deploy script builds from the repo source in Cloud Build. Self-test first: a guard never shown red is untested.
compiles_in_cloud_build() { grep -rnE --exclude="*.test.sh" "(run deploy|builds submit)[^#]*--source[ =]\.( |$)" "$@" || true; }
FX=$(mktemp -d); trap "rm -rf \"$FX\"" EXIT
printf "gcloud run deploy sirsi-router --source . --quiet\n" >"$FX/bad.sh"
[ -n "$(compiles_in_cloud_build "$FX")" ] || fail "guard self-test: did not catch --source . in a fixture"
hits=$(compiles_in_cloud_build scripts)
[ -z "$hits" ] || fail "a deploy path builds the repo in Cloud Build (private module unreachable there): $hits"

# 2. The repo-root Dockerfile (what --source . would use) refuses instead of failing 3 minutes in.
grep -q "exit 1" Dockerfile && ! grep -q "go build" Dockerfile || fail "root Dockerfile must refuse, not compile"

# 3. stage.sh builds a static linux/amd64 binary and a COPY-only context.
CTX="$FX/ctx"
bash scripts/router-service/stage.sh --context-only "$CTX"
desc=$(file -b "$CTX/sirsi")
case "$desc" in *ELF\ 64-bit*x86-64*statically\ linked*) ;; *) fail "context binary is not static linux/amd64: $desc" ;; esac
bad=$(grep -vE "^(FROM gcr.io/distroless/static-debian12:nonroot|COPY sirsi /sirsi|COPY recipient-registry.json /.agents/idea-router/agents.json|WORKDIR /|ENTRYPOINT |CMD )" "$CTX/Dockerfile" || true)
[ -z "$bad" ] || fail "context Dockerfile does more than copy the prebuilt binary: $bad"
grep -q "^CMD \[\"router\",\"serve\"\]$" "$CTX/Dockerfile" || fail "context must run router serve"
jq -e --arg source "$(git rev-parse HEAD)" '.source_commit==$source and (.agents|type=="object") and (.agents|length>0)' "$CTX/recipient-registry.json" >/dev/null || fail "registry is not bound to this committed source"
jq -e '[.agents[]|keys[]|select(.!="type" and .!="description" and .!="mail_to" and .!="wake" and .!="delivery_contract")]|length==0' "$CTX/recipient-registry.json" >/dev/null || fail "registry contains runtime/credential-bearing fields"
jq -S '.agents|map_values({type,description,mail_to,wake:{mechanism:.wake.mechanism},delivery_contract}|with_entries(select(.value!=null)))' .agents/idea-router/agents.json >"$FX/expected-registry.json"
jq -S '.agents' "$CTX/recipient-registry.json" >"$FX/actual-registry.json"
cmp "$FX/expected-registry.json" "$FX/actual-registry.json" || fail "recipient policy drifted from source"
jq -e '.agents.horus.type=="service" and .agents.horus.mail_to=="ra" and .agents.horus.wake.mechanism=="none"' "$CTX/recipient-registry.json" >/dev/null || fail "Horus service mail custodian was lost from the cloud projection"
if [ "$(uname -s)" = Linux ]; then "$CTX/sirsi" version >/dev/null || fail "context binary does not run"; fi
echo "stage.test.sh: ok"
