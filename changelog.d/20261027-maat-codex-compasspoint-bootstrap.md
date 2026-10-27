Align the Codex CompassPoint consumer prompt with the Claude dispatcher-binding
contract. Keep actual-session registration and router/task coordination in
standalone installed CLI calls outside the compute wrapper, and state that
Thoth/canon reads use standalone permitted source-read commands outside it. Use
the existing registered-executor Ma’at wrapper only for payload execution. The
native cross-host command, proxy, host-pin, and public-key-authentication facts
remain byte-identical; only “Local work” is narrowed to “Local payload work.”
No admission or service behavior changed.
