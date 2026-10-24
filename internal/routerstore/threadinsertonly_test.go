package routerstore

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestThreadInsertOnlyCASPreservesOccupiedAndScopedRows(t *testing.T) {
	f := newScopedFixture(t, []string{"cloud-a"})
	r := ThreadRecord{ThreadID: "occupied", Agent: "cloud-a", Host: f.host, Status: "active", LastSeenAt: "2026-10-24T10:00:00Z", Payload: []byte(`{"session_id":"different","workstream":"protected"}`)}
	if ok, err := f.backend.UpsertThreadCAS(r); err != nil || !ok {
		t.Fatal(ok, err)
	}
	for _, remote := range []bool{false, true} {
		candidate := r
		candidate.CreateOnly = true
		candidate.LastSeenAt = "2026-10-24T11:00:00Z"
		candidate.Payload = []byte(`{"session_id":"attempted"}`)
		var ok bool
		var err error
		if remote {
			ok, err = f.client("cloud-a").UpsertThreadCAS(candidate)
		} else {
			ok, err = f.backend.UpsertThreadCAS(candidate)
		}
		if err != nil || ok {
			t.Fatalf("occupied insert-only: %v %v", ok, err)
		}
		got, err := f.backend.GetThreadRecord(r.ThreadID)
		if err != nil || !bytes.Equal(got.Payload, r.Payload) || got.LastSeenAt != r.LastSeenAt {
			t.Fatal("occupied row overwritten", err)
		}
	}
	r.ThreadID = "new"
	r.CreateOnly = true
	if ok, err := f.client("cloud-a").UpsertThreadCAS(r); err != nil || !ok {
		t.Fatal("scoped insert failed", ok, err)
	}
	r.ThreadID = "foreign"
	r.Agent = "other"
	if _, err := f.client("cloud-a").UpsertThreadCAS(r); err == nil {
		t.Fatal("insert-only widened lane authority")
	}
}

func TestThreadInsertOnlyVersionFailsClosedForLegacyDecoder(t *testing.T) {
	r := ThreadRecord{ThreadID: "own", CreateOnly: true}
	wire, err := json.Marshal([]any{"thread-insert-only.v1", r})
	if err != nil {
		t.Fatal(err)
	}
	var legacy ThreadRecord
	if err := json.Unmarshal(wire, &legacy); err == nil {
		t.Fatal("legacy decoder ignored insert-only guard")
	}
	var current ThreadRecord
	if err := decodeThreadCASArgument(wire, &current); err != nil || !current.CreateOnly {
		t.Fatal("versioned restrictive argument rejected", err)
	}
	plain, _ := json.Marshal(r)
	if err := decodeThreadCASArgument(plain, &current); err == nil {
		t.Fatal("unversioned restriction accepted")
	}
}
