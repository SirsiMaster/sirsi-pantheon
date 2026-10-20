# Connecting several IDE sessions

Configure each IDE to launch `sirsi mcp` using the absolute path to your installed
`sirsi` executable. Each IDE owns a separate stdio connection, so several sessions
can use the tools at once. Closing one session does not close another's server.
The legacy `sirsi seshat mcp` command provides the same connection.

After installing a release with the per-client fix, reconnect Sirsi in your IDE.
In Claude Code, use `/mcp reconnect sirsi` where supported, or reconnect the server
through the `/mcp` menu. Existing sessions can remain open during reconnection.

A manual `sirsi mcp` invocation waits for JSON-RPC input; it is not a background
service shared between clients. Diagnostics go to stderr and protocol replies go
to stdout. Closing stdin ends that client's process. Shared router task claims
and resource safety rules still apply independently of the connection.
