#!/usr/bin/env bash
# Read-only preflight: print the sole 100% serving revision, or an empty line
# only when a successful service listing proves that sirsi-router is absent.
set -euo pipefail
fail() { echo "stage: $*; refusing deployment" >&2; exit 1; }
services=$(gcloud run services list "$@" --format=json) || fail "cannot list services"
count=$(jq -er 'if type == "array" and all(.[]; (.metadata.name | type == "string" and length > 0)) then [ .[] | select(.metadata.name == "sirsi-router") ] | length else error("invalid service list") end' <<<"$services") || fail "invalid service listing"
[[ "$count" =~ ^[0-9]+$ ]] || fail "invalid service listing"
if [ "$count" = 0 ]; then
  echo
  exit 0
fi
[ "$count" = 1 ] || fail "ambiguous service listing"
service=$(gcloud run services describe sirsi-router "$@" --format=json) || fail "cannot describe existing service"
revision=$(jq -er '
  if .metadata.name != "sirsi-router" or (.status.traffic | type) != "array" then error("invalid service") else .status.traffic end
  | if all(.[]; type == "object" and
      ((.percent | type) == "number" and .percent >= 0 and .percent <= 100 and (.percent | floor) == .percent
       or (.percent == null and (.tag | type == "string" and length > 0))))
    then . else error("invalid traffic") end
  | [ .[] | select((.percent // 0) > 0) ]
  | if length == 1 and .[0].percent == 100 and (.[0].revisionName | type == "string" and test("^[a-z]([-a-z0-9]*[a-z0-9])?$"))
    then .[0].revisionName else error("traffic is split or unknown") end
' <<<"$service") || fail "cannot identify a single 100% serving revision"
[[ "$revision" =~ ^[a-z]([-a-z0-9]*[a-z0-9])?$ ]] || fail "invalid serving revision"
printf '%s\n' "$revision"
