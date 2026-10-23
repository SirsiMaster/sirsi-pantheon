package schedule

import (
	"testing"
	"time"
)

func mkDelegation(pod, authority, start, notAfter string, resources ...string) Delegation {
	return Delegation{Pod: pod, AuthorityMachine: authority, Resources: resources, IssuedAt: start, NotAfter: notAfter}
}

func TestDelegate_GrantsFreshEpochOne(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio", "m5"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Epoch != 1 {
		t.Fatalf("want epoch 1 for a fresh delegation, got %d", d.Epoch)
	}
}

func TestDelegate_ReissueSamePodBumpsEpoch(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio", "m5")); err != nil {
		t.Fatal(err)
	}
	l.now = func() time.Time { v, _ := time.Parse(time.RFC3339, "2026-10-22T18:05:00Z"); return v }
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio-2", "2026-10-22T18:05:00Z", "2026-10-23T02:05:00Z", "studio", "m5"))
	if err != nil {
		t.Fatal(err)
	}
	if d.Epoch != 2 {
		t.Fatalf("want epoch bumped to 2 on re-issue, got %d", d.Epoch)
	}
}

func TestDelegate_OverlapWithDifferentLivePodRefused(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio", "m5")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Delegate(mkDelegation("other-pod", "M1", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "m5")); err == nil {
		t.Fatal("a resource already delegated to a different live pod must be refused")
	}
}

func TestDelegate_OverlapWithExpiredPodAllowed(t *testing.T) {
	l := fixedLedger("2026-10-22T20:00:00Z") // after the first delegation's not_after
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio", "m5")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Delegate(mkDelegation("other-pod", "M1", "2026-10-22T20:00:00Z", "2026-10-23T04:00:00Z", "m5")); err != nil {
		t.Fatalf("an expired delegation must not block a new one: %v", err)
	}
}

func TestDelegate_WindowExceedingCapRefused(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-23T10:00:01Z", "studio")); err == nil {
		t.Fatal("a window over the 12h finite cap must be refused")
	}
}

func TestRevoke_FencesImmediately(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio")); err != nil {
		t.Fatal(err)
	}
	d, err := l.Revoke("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Revoked {
		t.Fatal("want Revoked set")
	}
	// Cloud revocation cannot invalidate an offline cached lease; keep its fence.
	if _, err := l.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z", RegimeQuiet), false); err == nil {
		t.Fatal("cloud revocation reopened canonical admission while offline lease remains valid")
	}
}

func TestRevoke_UnknownPodErrors(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Revoke("never-delegated"); err == nil {
		t.Fatal("revoking a pod with no delegation must error")
	}
}

func TestReserve_RefusesDirectBookingOnDelegatedResource(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio", "m5")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Reserve(mkReq("studio", "mercury", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z", RegimeQuiet), false); err == nil {
		t.Fatal("a direct Reserve on a delegated resource must be refused (ADR-120 §2)")
	}
	// An undelegated resource is unaffected.
	if _, err := l.Reserve(mkReq("m1", "mercury", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z", RegimeQuiet), false); err != nil {
		t.Fatalf("an undelegated resource must still reserve normally: %v", err)
	}
}

func TestReserveAsAuthority_GrantsUnderLiveDelegation(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	req := Reservation{
		ID: "pod-grant-1", Resource: "studio", Holder: "mercury",
		Regime: RegimeQuiet, Start: "2026-10-22T10:00:00Z", EstEnd: "2026-10-22T10:30:00Z",
		LeaseTTLSec: 120, Authority: "pod:studio-m5", Epoch: d.Epoch,
	}
	res, err := l.ReserveAsAuthority(req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Granted || res.Reservation.Status != StatusActive {
		t.Fatalf("want an active grant, got %+v", res)
	}
}

func TestReserveAsAuthority_RefusesWithNoLiveDelegation(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	req := Reservation{ID: "x", Resource: "studio", Holder: "mercury", Regime: RegimeQuiet, Start: "2026-10-22T10:00:00Z", Authority: "pod:studio-m5", Epoch: 1}
	if _, err := l.ReserveAsAuthority(req); err == nil {
		t.Fatal("want fenced refusal with no live delegation")
	}
}

func TestReserveAsAuthority_RefusesStaleEpoch(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	// A new authority can be issued only after the old expiry and margin.
	l.now = func() time.Time { v, _ := time.Parse(time.RFC3339, "2026-10-22T18:05:00Z"); return v }
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio-2", "2026-10-22T18:05:00Z", "2026-10-23T02:05:00Z", "studio")); err != nil {
		t.Fatal(err)
	}
	req := Reservation{
		ID: "pod-grant-stale", Resource: "studio", Holder: "mercury", Regime: RegimeQuiet,
		Start: "2026-10-22T10:00:00Z", Authority: "pod:studio-m5", Epoch: d.Epoch, // the OLD epoch
	}
	if _, err := l.ReserveAsAuthority(req); err == nil {
		t.Fatal("want fenced refusal for a superseded epoch")
	}
}

func TestReserveAsAuthority_RefusesWrongPod(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	req := Reservation{
		ID: "x", Resource: "studio", Holder: "mercury", Regime: RegimeQuiet,
		Start: "2026-10-22T10:00:00Z", Authority: "pod:some-other-pod", Epoch: d.Epoch,
	}
	if _, err := l.ReserveAsAuthority(req); err == nil {
		t.Fatal("want fenced refusal when the authority names a pod that does not hold this resource")
	}
}

func TestUpsertReservation_IdempotentByID(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	req := Reservation{ID: "grant-1", Resource: "studio", Holder: "mercury", Regime: RegimeQuiet, Start: "2026-10-22T10:00:00Z", EstEnd: "2026-10-22T10:30:00Z", Status: StatusActive}
	if _, err := l.UpsertReservation(req); err != nil {
		t.Fatal(err)
	}
	// A retried replay with an updated field must rewrite the same row, not add a second.
	req.EstEnd = "2026-10-22T11:00:00Z"
	if _, err := l.UpsertReservation(req); err != nil {
		t.Fatal(err)
	}
	rows, err := l.Status("studio")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly one row after two upserts of the same id, got %d", len(rows))
	}
	if rows[0].EstEnd != "2026-10-22T11:00:00Z" {
		t.Fatalf("want the second upsert's fields to win, got %+v", rows[0])
	}
}

func TestUpsertReservation_RequiresID(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.UpsertReservation(Reservation{Resource: "studio"}); err == nil {
		t.Fatal("want an error for an upsert with no id")
	}
}

func TestDelegationPartitionHandback(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		l := fixedLedger("2026-10-22T10:00:00Z")
		d, err := l.Delegate(mkDelegation("pod-a", "old-host", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio"))
		if err != nil {
			t.Fatal(err)
		}
		if revoked {
			if _, err := l.Revoke("pod-a"); err != nil {
				t.Fatal(err)
			}
		}
		for _, stamp := range []string{"2026-10-22T10:01:00Z", "2026-10-22T18:00:00Z", "2026-10-22T18:04:59Z"} {
			now, _ := time.Parse(time.RFC3339, stamp)
			l.now = func() time.Time { return now }
			for _, pod := range []string{"pod-a", "pod-b"} {
				if _, err := l.Delegate(mkDelegation(pod, "new-host", stamp, now.Add(time.Hour).Format(time.RFC3339), "studio")); err == nil {
					t.Fatalf("revoked=%v moved authority before handback at %s", revoked, stamp)
				}
			}
			if got, err := l.delegationFor("studio", now); err != nil || got == nil {
				t.Fatalf("canonical fence missing revoked=%v at%s: %v", revoked, stamp, err)
			}
			if revoked {
				if _, err := l.ReserveAsAuthority(Reservation{ID: "stale", Resource: "studio", Authority: "pod:pod-a", Epoch: d.Epoch}); err == nil {
					t.Fatal("revoked authority accepted")
				}
			}
		}
		end, _ := time.Parse(time.RFC3339, "2026-10-22T18:05:00Z")
		l.now = func() time.Time { return end }
		if _, err := l.Delegate(mkDelegation("pod-a", "new-host", "2026-10-22T18:05:00Z", "2026-10-22T19:05:00Z", "studio")); err != nil {
			t.Fatal(err)
		}
	}
	if !(Delegation{NotAfter: "corrupt", Revoked: true}).blocksCanonical(time.Now()) {
		t.Fatal("corrupt retained lease expiry failed open")
	}
}

func TestUpsertReservationRefusesStaleOrResurrectingReplay(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	original := Reservation{ID: "closed-pod-grant", Resource: "studio", Holder: "mercury", Authority: "pod:a", Epoch: 7, Seq: 5, Status: StatusReleased}
	if _, err := l.UpsertReservation(original); err != nil {
		t.Fatal(err)
	}
	for _, req := range []Reservation{
		{ID: original.ID, Resource: "studio", Holder: "mercury", Authority: "pod:a", Epoch: 7, Seq: 4, Status: StatusReleased},
		{ID: original.ID, Resource: "studio", Holder: "mercury", Authority: "pod:a", Epoch: 7, Seq: 6, Status: StatusActive},
		{ID: original.ID, Resource: "studio", Holder: "other", Authority: "pod:a", Epoch: 7, Seq: 6, Status: StatusReleased},
		{ID: original.ID, Resource: "studio", Holder: "mercury", Authority: "pod:a", Epoch: 8, Seq: 6, Status: StatusReleased},
	} {
		if _, err := l.UpsertReservation(req); err == nil {
			t.Fatal("unsafe replay accepted")
		}
	}
	if _, err := l.UpsertReservation(original); err != nil {
		t.Fatal("exact duplicate should succeed", err)
	}
}

func TestAuthorityReplayRejectsUnboundedOrQueuedRows(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	d := mkDelegation("pod", "native", now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), "studio")
	d.Epoch = 1
	r := mkReq("studio", "ra", now.Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), RegimeQuiet)
	r.ID = "grant"
	r.Authority = "pod:pod"
	r.Epoch = 1
	r.Seq = 1
	r.Status = StatusActive
	r.Share = ShareFull
	r.Created = now.Format(time.RFC3339)
	r.HeartbeatAt = r.Created
	r.LeaseTTLSec = 120
	if e := validateAuthorityReplay(r, d); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Reservation){func(v *Reservation) { v.Status = StatusQueued }, func(v *Reservation) { v.EstEnd = now.Add(2 * time.Hour).Format(time.RFC3339) }, func(v *Reservation) { v.LeaseTTLSec = 0 }, func(v *Reservation) { v.Share = "made-up" }, func(v *Reservation) { v.Holder = "" }} {
		bad := r
		change(&bad)
		if e := validateAuthorityReplay(bad, d); e == nil {
			t.Fatal("invalid authority replay accepted", bad)
		}
	}
}
