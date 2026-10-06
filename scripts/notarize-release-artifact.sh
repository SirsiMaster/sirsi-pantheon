#!/bin/bash
# notarize-release-artifact.sh — bounded, fail-closed Apple notarization.
#
# Apple Notary uploads are multipart S3 transfers.  An accepted submission can
# still abort with HTTPClientError.deadlineExceeded while the upload is being
# resumed.  That is transport failure, not a notarization verdict.  Retry only
# that narrow failure class; a rejected artifact, bad credential, or any other
# notary error remains immediately fatal.
set -euo pipefail

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

artifact="${1:-}"
[[ $# -eq 1 && -f "$artifact" && ! -L "$artifact" ]] || {
    echo "usage: $0 <regular artifact>" >&2
    exit 2
}

attempt_limit="${PANTHEON_NOTARY_UPLOAD_ATTEMPTS:-5}"
retry_delay="${PANTHEON_NOTARY_RETRY_DELAY_SECONDS:-30}"
[[ "$attempt_limit" =~ ^[1-9][0-9]*$ && "$attempt_limit" -le 8 ]] || {
    echo "PANTHEON_NOTARY_UPLOAD_ATTEMPTS must be a decimal integer from 1 through 8" >&2
    exit 2
}
[[ "$retry_delay" =~ ^[0-9]+$ && "$retry_delay" -le 300 ]] || {
    echo "PANTHEON_NOTARY_RETRY_DELAY_SECONDS must be a non-negative decimal integer no greater than 300" >&2
    exit 2
}

# Production always resolves xcrun from its absolute macOS path. An explicit
# test seam is accepted only with the test-mode sentinel, so a caller cannot
# redirect an unattended commercial release to a PATH-selected shim.
xcrun_bin="/usr/bin/xcrun"
if [[ -n "${PANTHEON_NOTARY_TEST_XCRUN:-}" ]]; then
    [[ "${PANTHEON_NOTARY_TEST_MODE:-}" == "1" && -x "${PANTHEON_NOTARY_TEST_XCRUN}" ]] || {
        echo "PANTHEON_NOTARY_TEST_XCRUN requires PANTHEON_NOTARY_TEST_MODE=1 and an executable helper" >&2
        exit 2
    }
    xcrun_bin="${PANTHEON_NOTARY_TEST_XCRUN}"
fi

is_transient_upload_failure() {
    /usr/bin/grep -Eqi \
        'abortedUpload|HTTPClientError\.deadlineExceeded|SotoS3|ResumeMultipartUpload' <<<"$1"
}

attempt=1
while [[ "$attempt" -le "$attempt_limit" ]]; do
    echo "Notarizing $(basename "$artifact") (attempt ${attempt}/${attempt_limit})..."
    if [[ -n "${APPLE_NOTARY_PROFILE:-}" ]]; then
        if output="$("$xcrun_bin" notarytool submit "$artifact" \
            --keychain-profile "$APPLE_NOTARY_PROFILE" --timeout 20m --wait 2>&1)"; then
            printf '%s\n' "$output"
            exit 0
        else
            status=$?
        fi
    else
        if output="$("$xcrun_bin" notarytool submit "$artifact" \
            --apple-id "${APPLE_ID:?APPLE_ID is required}" \
            --team-id "${APPLE_TEAM_ID:?APPLE_TEAM_ID is required}" \
            --password "${APPLE_APP_PASSWORD:?APPLE_APP_PASSWORD is required}" \
            --timeout 20m --wait 2>&1)"; then
            printf '%s\n' "$output"
            exit 0
        else
            status=$?
        fi
    fi

    printf '%s\n' "$output" >&2
    if ! is_transient_upload_failure "$output" || [[ "$attempt" -eq "$attempt_limit" ]]; then
        echo "ERROR: Apple notarization failed without a retryable upload recovery path." >&2
        exit "$status"
    fi
    # Reopening a multipart upload immediately can hit the same stalled Apple
    # edge.  Space only the narrow transport retry class, with a bounded
    # exponential delay; ordinary notarization, signing, and credential errors
    # still return at once above.
    delay=$((retry_delay * (1 << (attempt - 1))))
    [[ "$delay" -le 300 ]] || delay=300
    echo "Transient Apple multipart-upload deadline; retrying in ${delay}s." >&2
    /bin/sleep "$delay"
    attempt=$((attempt + 1))
done
