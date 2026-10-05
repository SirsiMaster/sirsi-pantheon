# Desktop recovery gateway

The gateway exchanges an Ed25519 capability for an expiring node-bound browser
cookie and carries RFB only to configured private destinations. The authorizer
returns a verified signer/nonce identity and its signed expiry. Canonical strict
base64url decoding prevents alternate encodings of one signature; admission
atomically consumes that identity until the capability expires, independently
of cookie expiry and disconnect. Custom authorizers must return those fields;
a login alone cannot authorize admission.

The consumed-nonce map is process-local. Restart or another gateway process
cannot preserve it. Global one-time admission is not qualified: production
release needs an authority-owned durable nonce consumption contract or a
reviewed process-generation binding. Do not represent this source repair as
restart-safe. Tests inject clocks and use recorders for the replay controls;
transport tests require permitted loopback sockets.
