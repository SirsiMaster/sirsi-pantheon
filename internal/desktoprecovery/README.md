# Desktop recovery engine

The shared Go gateway carries an allowlisted RFB stream through a short-lived single-use WebSocket session. CLI supplies config, embedded noVNC supplies the browser RFB client. `Authorizer` is the admission seam; production requires a loopback peer, allowlisted tailnet login and independent bcrypt-verified recovery operator password. A header alone cannot admit a session. `Now`, `Rand` and `DialContext` support deterministic tests without host changes.

Fresh/expired `/client` requests render entry; same-origin POST authenticates and mints a Secure HttpOnly cookie; reopening entry renews admission after disconnect. Credentials are transient browser input and traverse the trusted HTTPS terminator; they are neither logged nor persisted by this engine. noVNC handles AppleARD credentials in browser memory.

Limitations: actual Serve identity, encrypted ingress, Mac authentication and iPhone controls require live qualification. A local privileged attacker controlling the process, config or TLS terminator is outside this admission boundary. Source tests do not establish installed security. Never expose the loopback bridge or RFB port publicly.
