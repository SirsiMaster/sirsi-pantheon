# Router token publication

Secret-backed mint creates the complete new token and scope in a pending state. The router refuses pending tokens. The command publishes the secret and writes its nonsecret receipt before activating that token. If publication or receipt fails, it retires only the pending token; it does not revoke other tokens or live sessions for the host. A short receipt write is a failure.

If activation reports an error, its database commit may have succeeded. The command reports cleanup debt with the token ID and does not blindly revoke a potentially active token. Reconcile the token and secret version through the authorized operator workflow before any retry. A process crash can leave an inactive pending record; it is not authority to mint sessions. Explicit `router token revoke` retains its separate host-wide session-revocation behavior.

This is source behavior until a reviewed and signed job image is deployed. Do not rotate credentials, replace a running token job or infer an installed update from these tests.
