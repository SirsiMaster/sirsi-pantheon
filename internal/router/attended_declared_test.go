package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeReg(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// A declared-attended lane is held for any secondary consumer even when no session heartbeat is fresh;
// an undeclared lane is not; and the flag survives a registry round trip.
func TestDeclaredAttendedLaneIsNeverOverridden(t *testing.T) {
	setAttendedLiveFn(func(string, string) bool { return false }) // no armed session anywhere
	t.Cleanup(func() { setAttendedLiveFn(nil) })
	root := writeReg(t, `{"agents":{
	  "att":{"id":"att","type":"codex","attended":true,"wake":{"mechanism":"launchagent"}},
	  "wrk":{"id":"wrk","type":"codex","wake":{"mechanism":"launchagent"}}}}`)
	if !attendedSessionOwnsInbox(root, "att", 3) {
		t.Fatal("declared attended lane was not held")
	}
	if attendedSessionOwnsInbox(root, "wrk", 3) {
		t.Fatal("undeclared lane was held")
	}
	if hold, _ := currentHold(root, "att", 0, time.Time{}); hold != HoldAttended {
		t.Fatalf("lane-state hold = %q, want %q", hold, HoldAttended)
	}
	reg, err := LoadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := reg.Agents["att"].MarshalJSON()
	if !strings.Contains(string(b), `"attended":true`) {
		t.Fatalf("attended flag lost on round trip: %s", b)
	}
}
