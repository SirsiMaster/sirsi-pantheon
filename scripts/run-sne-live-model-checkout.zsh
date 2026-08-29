#!/bin/zsh
set -euo pipefail

repo=${0:A:h:h}
catalog=${1:-}
catalog_entry=${2:-}
destination=${3:-}
evidence=${4:-}
expected_catalog_sha=${SNE_EXPECTED_SOURCE_CATALOG_SHA256:-}

fail() { print -u2 -- "$1"; exit ${2:-64}; }

[[ -n $catalog && -n $catalog_entry && -n $destination && -n $evidence ]] || \
  fail "usage: $0 <source-catalog> <catalog-entry> <absolute-destination> <absolute-evidence-dir>"
[[ $catalog = /* && $destination = /* && $evidence = /* ]] || fail "catalog, destination, and evidence paths must be absolute"
[[ -f $catalog ]] || fail "source catalog is not a regular file"
[[ -n $expected_catalog_sha ]] || fail "SNE_EXPECTED_SOURCE_CATALOG_SHA256 is required" 67
[[ $destination != / && $evidence != / && $destination != $evidence ]] || fail "unsafe checkout path"

catalog_sha=$(shasum -a 256 "$catalog" | awk '{print $1}')
[[ $catalog_sha == $expected_catalog_sha ]] || fail "source catalog SHA-256 mismatch" 67

mkdir -p "$destination" "$evidence"
[[ -z $(find "$evidence" -mindepth 1 -maxdepth 1 -print -quit) ]] || fail "evidence directory must be empty" 67

scratch=$(mktemp -d "${TMPDIR:-/tmp}/sne-live-checkout.XXXXXX")
trap 'rm -rf "$scratch"' EXIT INT TERM
acquire_bin=${SNE_ACQUIRE_BIN:-}
if [[ -z $acquire_bin ]]; then
  acquire_bin=$scratch/sirsi-sne-acquire
  (cd "$repo" && go build -trimpath -o "$acquire_bin" ./cmd/sirsi-sne-acquire)
fi
[[ -x $acquire_bin ]] || fail "SNE acquisition executable is unavailable"
acquire_sha=$(shasum -a 256 "$acquire_bin" | awk '{print $1}')

started_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
set +e
"$acquire_bin" -source-catalog "$catalog" -catalog-entry "$catalog_entry" \
  -destination "$destination" -json-progress >"$evidence/acquisition.jsonl" 2>"$evidence/acquisition.stderr"
exit_status=$?
set -e
finished_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)

if (( exit_status != 0 )); then
  print -r -- "status=rejected" >"$evidence/receipt.env"
  print -r -- "exit_status=$exit_status" >>"$evidence/receipt.env"
  print -r -- "catalog_sha256=$catalog_sha" >>"$evidence/receipt.env"
  print -r -- "acquire_sha256=$acquire_sha" >>"$evidence/receipt.env"
  print -r -- "started_utc=$started_utc" >>"$evidence/receipt.env"
  print -r -- "finished_utc=$finished_utc" >>"$evidence/receipt.env"
  exit $exit_status
fi

result_count=$(/usr/bin/grep -c '"type":"result"' "$evidence/acquisition.jsonl" || true)
(( result_count == 1 )) || fail "acquisition did not emit exactly one terminal result" 65
[[ -z $(find "$destination" -type f -name '*.partial' -print -quit) ]] || fail "acquisition returned success with partial artifacts" 65

{
  print -r -- "status=accepted"
  print -r -- "exit_status=0"
  print -r -- "catalog_sha256=$catalog_sha"
  print -r -- "catalog_entry=$catalog_entry"
  print -r -- "acquire_sha256=$acquire_sha"
  print -r -- "destination=$destination"
  print -r -- "started_utc=$started_utc"
  print -r -- "finished_utc=$finished_utc"
  print -r -- "result_sha256=$(shasum -a 256 "$evidence/acquisition.jsonl" | awk '{print $1}')"
} >"$evidence/receipt.env"

print "sne_live_model_checkout accepted=true catalog_sha256=$catalog_sha catalog_entry=$catalog_entry"
