# Managed command safety hook

The PreToolUse adapter reads one JSON envelope and checks both string `command`
and `cmd` fields with the shared agentguard recognizer. The managed policy keeps
Bash compatibility and routes every tool name to this binary. Valid non-command
payloads allow (exit 0); malformed JSON and recognized display-power commands
deny (exit 2). The binary has no shell or display actuator.

Run `go test -race ./cmd/sirsi-pretoolguard ./internal/agentguard ./internal/desktopcustody ./internal/stacklab`.
Fixtures use fake dispatch and durable temporary custody stores only. The pinned
installer artifacts describe a development macOS arm64 build; installation and
signing are separate qualification steps. GUI coordinates and unmanaged shells
are outside this narrow control. See ADR-040 and the managed custody recipe.
