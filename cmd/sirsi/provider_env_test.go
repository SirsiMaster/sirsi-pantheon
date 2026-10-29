package main_test

import "strings"

// Keep the construction seam pure so provider aliases can be checked without
// changing the test process's (or the operator's) actual session environment.
func sirsiTestEnvFrom(base []string, cwd, storeDB string, extra ...string) []string {
	dir := cwd
	out := make([]string, 0, len(base)+len(extra)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		// Identity inputs must come from the test, never the host. resolveCurrentAgent
		// consults $SIRSI_AGENT_ID and then the session marker at
		// ~/.claude/run/agent-by-session/$CLAUDE_CODE_SESSION_ID. Inheriting either
		// makes identity tests environment-dependent: they pass in CI (no session)
		// and fail on any developer machine running inside a CCD session, which is
		// exactly how TestRouterRespondStoreOnlyItem broke. Worse, `thread register`
		// WRITES that marker, so tests inheriting a real session id overwrite the
		// live operator's identity — 99 markers on this host had been rewritten to
		// "test-adr024-idem", including the running conduit's own.
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "SIRSI_AGENT_ID", "SIRSI_THREAD_ID", "CLAUDE_CODE_SESSION_ID", "CODEX_SESSION_ID", "CODEX_THREAD_ID":
			// TestMain also clears these, but a later fixture must not leak its
			// provider binding into another synthetic registration subprocess.
			continue
		}
		if dir != "" && strings.HasPrefix(kv, "PWD=") {
			continue
		}
		out = append(out, kv)
	}
	if dir != "" {
		out = append(out, "PWD="+dir)
	}
	// Sandbox the router store: without this, every test send lands in the
	// LIVE ~/.sirsi/router.db (six polluted rows found there 2026-07-07).
	//
	// The caller names the store. Mutating router tests pass a path inside their
	// own t.TempDir(), so each gets a virgin store — that is what actually
	// defeats the send idempotency window: dedupe keys on (from, to, title)
	// within a time bucket, so any two tests — or any two runs — that share a
	// store and send the same logical item will see the second send return
	// "Deduped ... nothing appended" and fail on an item that was never created.
	// A per-run store narrows that race; only a per-test store closes it.
	// Read-only commands (version, help) pass testStoreDB, the per-run store
	// inside TestMain's MkdirTemp, which is removed when the run ends.
	//
	// An explicit SIRSI_ROUTER_DB in extra still wins (append order).
	out = append(out, "SIRSI_ROUTER_DB="+storeDB)
	return append(out, extra...)
}
