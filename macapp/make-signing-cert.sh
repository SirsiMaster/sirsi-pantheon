#!/usr/bin/env bash
# Creates a self-signed code-signing certificate in the login keychain so the
# menubar app can be signed with a STABLE designated requirement. Ad-hoc
# signatures are not honored by TCC for Full Disk Access across relaunch (the
# grant toggle reverts); a self-signed cert fixes that with zero Apple/Developer
# Program involvement. Idempotent: re-running reuses the existing identity.
#
# After this, build-app.sh signs with SIGN_ID automatically. The user grants FDA
# once and it persists across relaunch and future rebuilds (same cert => same DR).
set -euo pipefail

SIGN_ID="${SIRSI_SIGN_IDENTITY:-Sirsi Local Code Signing}"
KEYCHAIN="$HOME/Library/Keychains/login.keychain-db"

if security find-identity -p codesigning 2>/dev/null | grep -q "$SIGN_ID"; then
	echo "▸ identity already present: $SIGN_ID"
	exit 0
fi

D="$(mktemp -d)"; trap 'rm -rf "$D"' EXIT
cat > "$D/openssl.cnf" <<EOF
[req]
distinguished_name = req_dn
x509_extensions = codesign_ext
prompt = no
[req_dn]
CN = $SIGN_ID
[codesign_ext]
basicConstraints = critical, CA:FALSE
keyUsage = critical, digitalSignature
extendedKeyUsage = critical, codeSigning
EOF

echo "▸ generating self-signed code-signing cert (EKU=codeSigning, 10y)…"
openssl req -x509 -newkey rsa:2048 -nodes \
	-keyout "$D/key.pem" -out "$D/cert.pem" -days 3650 -config "$D/openssl.cnf" >/dev/null 2>&1

# Apple's security(1) PKCS12 parser needs the legacy SHA1/3DES algorithms.
openssl pkcs12 -export -inkey "$D/key.pem" -in "$D/cert.pem" -out "$D/identity.p12" \
	-name "$SIGN_ID" -passout pass:sirsi \
	-legacy -certpbe PBE-SHA1-3DES -keypbe PBE-SHA1-3DES -macalg sha1 >/dev/null 2>&1

echo "▸ importing into login keychain (authorizing codesign)…"
security import "$D/identity.p12" -k "$KEYCHAIN" -P sirsi -T /usr/bin/codesign -A >/dev/null 2>&1

security find-identity -p codesigning 2>/dev/null | grep "$SIGN_ID" \
	&& echo "▸ done — '$SIGN_ID' ready (untrusted is fine; TCC keys on the cert hash)."
