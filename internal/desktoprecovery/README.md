# Desktop recovery bridge

One bounded loopback gateway embeds pinned noVNC and proxies only allowlisted
private Apple RFB destinations. Authorizer and AdmissionStore separate operator
identity from transport and durable replay protection. Config injects clock,
randomness and dialing for deterministic controls; Apple remains the desktop
authority. POST disconnect cancels the active proxy and clears the cookie;
Gateway.Close cancels sessions on shutdown, including hijacked WebSockets.

No issuer, production ingress or Apple desktop proof is supplied by this package.
See ADR-075 and the mobile desktop recovery user guide for acceptance gates.
SHA hardware observation consumption belongs at the Apollo Supervisor launch
boundary and is outside this release-source transplant: a compute HOLD never removes recovery access.

The durable claim directory must belong to the bridge effective operator and
have mode 0700. Startup and every claim check its retained no-follow descriptor;
stat failure or changed ownership/permissions denies admission without changing
permissions or removing replay records. Same-user/root mutation is outside this
permission check; it does not establish issuer enrollment or Apple input release.
