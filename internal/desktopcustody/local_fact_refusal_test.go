package desktopcustody

import (
	"testing"
	"time"
)

func TestLocalUserFactCannotBecomePrivilegedNativeExecutionBudget(t *testing.T) {
	data := []byte(`{"schema":"sirsi.maat.local-execution.v1","scope":"local","uid":501,"supervisor_pid":123,"supervisor_start":"start","argv_sha256":"fixture","started_at":"2026-10-25T12:00:00Z","deadline":"2026-10-25T12:01:00Z","cpu_cores":1}`)
	if _, err := parseExecutionBudget(data, 123, "start", []string{"sirsi", "vitals"}, time.Date(2026, 10, 25, 12, 0, 10, 0, time.UTC)); err == nil {
		t.Fatal("local observation fact became privileged authority")
	}
}
