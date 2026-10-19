- Fix: `RunWakeLoop` sets `SIRSI_AGENT_ID`/`SIRSI_THREAD_ID` from its own agent argument at start, instead of leaving them unset for launchd. Every wake-loop on a host previously fell back to `os.Hostname()` for session/audit identity, lumping all lanes under one shared session cache and one audit line (`Mac@Mac`, `M1.local@M1.local`). Found root-causing the session-table blowup fixed in #1101.

Refs: internal/router/wake.go; internal/router/wake_test.go
