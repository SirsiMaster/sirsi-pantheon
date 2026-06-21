// Package router — sessionmarker.go
//
// Session→agent markers (thread-liveness guarantee, claude-home 20260621-015502,
// piece 1). The PRD-keepalive Stop hook must resolve which agent a Claude session
// belongs to so it can read .agents/prd/<agent>.json and block idle while work is
// open. It resolves via $SIRSI_AGENT_ID, then this marker, then a cwd match. CCD
// sessions run with cwd=$HOME, so the cwd match can't distinguish them — the
// marker is what lets the hook fire for those sessions.
//
// `sirsi thread register` writes ~/.claude/run/agent-by-session/<session_id> =
// <agent_id>; `sirsi thread close` removes it. The session id comes from
// $CLAUDE_CODE_SESSION_ID (set inside every Claude Code session).
package router

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SessionIDEnv is the environment variable Claude Code sets to the current
// session's id. The marker is keyed on its value.
const SessionIDEnv = "CLAUDE_CODE_SESSION_ID"

// sessionMarkerDirOverride lets tests redirect the marker dir away from the real
// ~/.claude/run/agent-by-session. Empty in production.
var sessionMarkerDirOverride string

func sessionMarkerDir() string {
	if sessionMarkerDirOverride != "" {
		return sessionMarkerDirOverride
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "run", "agent-by-session")
}

// sessionIDUnsafe matches anything that is NOT a safe session-id character. A
// session id is a UUID, but sanitizing guards against a crafted value escaping
// the marker dir via path separators.
var sessionIDUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeSessionID(s string) string {
	return sessionIDUnsafe.ReplaceAllString(strings.TrimSpace(s), "_")
}

// CurrentSessionID returns the Claude session id from the environment, or "".
func CurrentSessionID() string {
	return strings.TrimSpace(os.Getenv(SessionIDEnv))
}

// WriteSessionAgentMarker records that sessionID resolves to agentID. No-op when
// either is empty (e.g. a non-Claude / non-interactive context) so callers can
// invoke it unconditionally on register.
func WriteSessionAgentMarker(sessionID, agentID string) error {
	sessionID = sanitizeSessionID(sessionID)
	agentID = strings.TrimSpace(agentID)
	if sessionID == "" || agentID == "" {
		return nil
	}
	dir := sessionMarkerDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sessionID), []byte(agentID+"\n"), 0o644)
}

// RemoveSessionAgentMarker deletes the marker for sessionID. No-op when sessionID
// is empty or the marker is already gone (close is idempotent).
func RemoveSessionAgentMarker(sessionID string) error {
	sessionID = sanitizeSessionID(sessionID)
	if sessionID == "" {
		return nil
	}
	if err := os.Remove(filepath.Join(sessionMarkerDir(), sessionID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ReadSessionAgentMarker returns the agent id recorded for sessionID, or "" if no
// marker exists. (Used by the supervisor + exposed for the Stop hook's parity.)
func ReadSessionAgentMarker(sessionID string) string {
	sessionID = sanitizeSessionID(sessionID)
	if sessionID == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(sessionMarkerDir(), sessionID))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
