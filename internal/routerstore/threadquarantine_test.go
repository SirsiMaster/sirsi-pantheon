package routerstore

import (
	"path/filepath"
	"testing"
)

// Quarantine rewrites a thread's status WITHOUT advancing last_seen_at (the
// staleness evidence is kept), so the CAS must accept an equal-timestamp write
// into 'quarantined' and any equal-timestamp release out of it — and must still
// refuse an equal-timestamp status flip between ordinary states.
func TestUpsertThreadCASQuarantineTransition(t *testing.T) {
	s, err := OpenPath(seedFromTemplate(t, filepath.Join(t.TempDir(), "router.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const ts = "2026-11-28T00:00:00Z"
	rec := func(status, payload string) ThreadRecord {
		return ThreadRecord{ThreadID: "thr-q", Agent: "a", Status: status, LastSeenAt: ts, Payload: []byte(payload), Host: "h"}
	}
	apply := func(r ThreadRecord) bool {
		ok, err := s.UpsertThreadCAS(r)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !apply(rec("active", `{"v":1}`)) {
		t.Fatal("initial insert refused")
	}
	// Negative control: an ordinary equal-timestamp status flip stays refused.
	if apply(rec("idle", `{"v":0}`)) {
		t.Fatal("equal-timestamp flip between ordinary states was accepted")
	}
	if !apply(rec("quarantined", `{"v":2}`)) {
		t.Fatal("equal-timestamp write into quarantined refused")
	}
	if !apply(rec("active", `{"v":3}`)) {
		t.Fatal("equal-timestamp release out of quarantined refused")
	}
}
