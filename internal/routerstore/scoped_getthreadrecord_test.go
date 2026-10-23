package routerstore

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestScopedGetThreadRecordLifecycleAndSibling(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	rs := f.client("cloud-a")
	own := ThreadRecord{ThreadID: "own", Agent: "cloud-a", Host: f.host, Status: "active", LastSeenAt: "2026-10-23T10:00:00Z", Payload: []byte(`{"thread_id":"own","agent_id":"cloud-a","status":"active"}`)}
	foreign := ThreadRecord{ThreadID: "foreign", Agent: "other", Host: f.host, Status: "active", LastSeenAt: own.LastSeenAt, Payload: []byte(`{"private":"foreign-resume-secret"}`)}
	sibling := own
	sibling.ThreadID = "sibling"
	sibling.Payload = []byte(`{"thread_id":"sibling","agent_id":"cloud-a","status":"active"}`)
	if err := f.backend.UpsertThreads([]ThreadRecord{own, foreign, sibling}); err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"active", "closed"} {
		got, err := rs.GetThreadRecord("own")
		if err != nil || got.Agent != "cloud-a" {
			t.Fatalf("own read: %+v %v", got, err)
		}
		got.Status = status
		got.LastSeenAt = time.Date(2026, 10, 23, 10, i+1, 0, 0, time.UTC).Format(time.RFC3339)
		got.Payload = []byte(fmt.Sprintf(`{"thread_id":"own","agent_id":"cloud-a","status":%q}`, status))
		if ok, err := rs.UpsertThreadCAS(got); err != nil || !ok {
			t.Fatalf("own lifecycle: %v %v", ok, err)
		}
	}
	if got, err := rs.GetThreadRecord("foreign"); err == nil || len(got.Payload) != 0 {
		t.Fatalf("foreign full payload escaped: %v", err)
	}
	if _, err := rs.GetThreadRecord("unknown"); !errors.Is(err, ErrThreadUnknown) {
		t.Fatalf("unknown row: %v", err)
	}
	after, err := f.backend.GetThreadRecord("sibling")
	if err != nil || !bytes.Equal(after.Payload, sibling.Payload) || after.LastSeenAt != sibling.LastSeenAt {
		t.Fatal("sibling changed")
	}
	// An old client still using the existing RPC is accepted by the new server.
	if _, err := rs.ThreadBinding("own"); err != nil {
		t.Fatal("old client RPC regressed", err)
	}
}
