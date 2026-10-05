# Desktop recovery bridge

One bounded loopback gateway embeds pinned noVNC and proxies only allowlisted
private Apple RFB destinations. Authorizer and AdmissionStore separate operator
identity from transport and durable replay protection. Config injects clock,
randomness and dialing for deterministic controls; Apple remains the desktop
authority. POST disconnect cancels the active proxy and clears the cookie;
Gateway.Close cancels sessions on shutdown, including hijacked WebSockets.

No issuer, production ingress or Apple desktop proof is supplied by this package.
See ADR-064 and the mobile desktop recovery user guide for acceptance gates.
SHA hardware observation consumption lives at the Apollo Supervisor launch
boundary, not here: a compute HOLD never removes recovery access.
