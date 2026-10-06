#!/bin/bash
# Regression tests for the bounded transient-notary retry contract.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin"
artifact="$tmp/Pantheon.dmg"
printf 'fixture\n' > "$artifact"

cat > "$tmp/bin/xcrun" <<'EOF'
#!/bin/bash
set -euo pipefail
count_file="${NOTARY_TEST_COUNT:?}"
count=0
[[ -f "$count_file" ]] && count="$(cat "$count_file")"
count=$((count + 1))
printf '%s\n' "$count" > "$count_file"
if [[ "${NOTARY_TEST_MODE:-}" == "transient" && "$count" -le "${NOTARY_TEST_TRANSIENT_COUNT:-1}" ]]; then
  echo 'Error: abortedUpload HTTPClientError.deadlineExceeded' >&2
  exit 75
fi
if [[ "${NOTARY_TEST_MODE:-}" == "permanent" ]]; then
  echo 'Error: Invalid credentials' >&2
  exit 64
fi
echo 'status: Accepted'
EOF
chmod +x "$tmp/bin/xcrun"

count="$tmp/count"
NOTARY_TEST_COUNT="$count" NOTARY_TEST_MODE=transient \
  PANTHEON_NOTARY_TEST_MODE=1 PANTHEON_NOTARY_TEST_XCRUN="$tmp/bin/xcrun" \
  PANTHEON_NOTARY_RETRY_DELAY_SECONDS=0 \
  APPLE_ID=a APPLE_TEAM_ID=9D382WV988 APPLE_APP_PASSWORD=p \
  "$root/scripts/notarize-release-artifact.sh" "$artifact" >/dev/null
[[ "$(cat "$count")" == 2 ]] || { echo "transient upload was not retried exactly once" >&2; exit 1; }

# Apple can return the exact multipart deadline more than once. The default
# commercial budget must survive four transient failures and accept the fifth
# submission, without sleeping in this isolated fixture.
rm -f "$count"
NOTARY_TEST_COUNT="$count" NOTARY_TEST_MODE=transient NOTARY_TEST_TRANSIENT_COUNT=4 \
  PANTHEON_NOTARY_TEST_MODE=1 PANTHEON_NOTARY_TEST_XCRUN="$tmp/bin/xcrun" \
  PANTHEON_NOTARY_RETRY_DELAY_SECONDS=0 \
  APPLE_ID=a APPLE_TEAM_ID=9D382WV988 APPLE_APP_PASSWORD=p \
  "$root/scripts/notarize-release-artifact.sh" "$artifact" >/dev/null
[[ "$(cat "$count")" == 5 ]] || { echo "transient upload did not use the bounded five-attempt budget" >&2; exit 1; }

rm -f "$count"
if NOTARY_TEST_COUNT="$count" NOTARY_TEST_MODE=permanent \
  PANTHEON_NOTARY_TEST_MODE=1 PANTHEON_NOTARY_TEST_XCRUN="$tmp/bin/xcrun" \
  PANTHEON_NOTARY_RETRY_DELAY_SECONDS=0 \
  APPLE_ID=a APPLE_TEAM_ID=9D382WV988 APPLE_APP_PASSWORD=p \
  "$root/scripts/notarize-release-artifact.sh" "$artifact" >/dev/null 2>&1; then
    echo "permanent notary failure unexpectedly passed" >&2
    exit 1
fi
[[ "$(cat "$count")" == 1 ]] || { echo "permanent failure was retried" >&2; exit 1; }
