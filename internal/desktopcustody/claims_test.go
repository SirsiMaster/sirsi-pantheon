//go:build unix

package desktopcustody

import (
	"os"
	"testing"
)

func TestAdmissionNonceIsConsumedAcrossRequestIDsAndReopen(t *testing.T) {
	f, s := newFixture(t)
	e := s.event(SleepPrepared, f.req, f.a, f.q, WakeReceipt{}, "", "")
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	j, err := NewFileSleepJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.BeginSleep(e); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	// A new journal instance cannot revive a consumed signed nonce.
	j, err = NewFileSleepJournal(root)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	e.RequestID = "different-request"
	e.OwnerTag = "pantheon-sleep-" + e.RequestID
	if err := j.BeginSleep(e); err == nil {
		t.Fatal("same nonce accepted under a different request ID")
	}
}

func TestDifferentNonceCannotBypassUnreconciledHostFence(t *testing.T) {
	f, s := newFixture(t)
	e := s.event(SleepPrepared, f.req, f.a, f.q, WakeReceipt{}, "", "")
	if err := f.j.BeginSleep(e); err != nil {
		t.Fatal(err)
	}
	e.RequestID, e.Nonce = "another-request", "another-nonce"
	e.OwnerTag = "pantheon-sleep-" + e.RequestID
	if err := f.j.BeginSleep(e); err == nil {
		t.Fatal("different request bypassed unresolved host transaction")
	}
}

func TestPreInvocationFailureReleasesOnlyHostFence(t *testing.T) {
	f, s := newFixture(t)
	e := s.event(SleepPrepared, f.req, f.a, f.q, WakeReceipt{}, "", "")
	if err := f.j.BeginSleep(e); err != nil {
		t.Fatal(err)
	}
	e.Phase = SleepFailed
	if err := f.j.AppendSleep(e); err != nil {
		t.Fatal(err)
	}
	// Consumed capability stays consumed after a safe pre-action failure.
	e.Phase, e.RequestID = SleepPrepared, "same-nonce-new-id"
	e.OwnerTag = "pantheon-sleep-" + e.RequestID
	if err := f.j.BeginSleep(e); err == nil {
		t.Fatal("failed transaction revived its authorization nonce")
	}
	e.RequestID, e.Nonce = "fresh-request", "fresh-nonce"
	e.OwnerTag = "pantheon-sleep-" + e.RequestID
	if err := f.j.BeginSleep(e); err != nil {
		t.Fatalf("confirmed pre-invocation cleanup retained host fence: %v", err)
	}
}
