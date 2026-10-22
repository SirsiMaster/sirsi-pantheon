package schedule

import "testing"

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
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio-2", "2026-10-22T10:05:00Z", "2026-10-22T18:05:00Z", "studio", "m5"))
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
	// Reserve directly on the now-revoked resource must succeed (no fence left).
	if _, err := l.Reserve(mkReq("studio", "claude-io", "2026-10-22T10:00:00Z", "2026-10-22T10:30:00Z", RegimeQuiet), false); err != nil {
		t.Fatalf("a revoked delegation must not fence the canonical ledger: %v", err)
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

func TestRenewDelegation_ExtendsNotAfterWithoutBumpingEpoch(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T10:15:00Z", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := l.RenewDelegation("studio-m5", d.Epoch, at("2026-10-22T10:14:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Epoch != d.Epoch {
		t.Fatalf("renewal must not bump epoch, got %d want %d", renewed.Epoch, d.Epoch)
	}
	wantNotAfter := "2026-10-22T22:14:00Z" // now + 12h cap
	if renewed.NotAfter != wantNotAfter {
		t.Fatalf("want not_after extended to %s, got %s", wantNotAfter, renewed.NotAfter)
	}
}

func TestRenewDelegation_SupersededEpochRefused(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RenewDelegation("studio-m5", 99, at("2026-10-22T10:05:00Z")); err == nil {
		t.Fatal("renewing with a stale epoch must be refused as fenced")
	}
}

func TestRenewDelegation_RevokedRefused(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	d, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Revoke("studio-m5"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RenewDelegation("studio-m5", d.Epoch, at("2026-10-22T10:05:00Z")); err == nil {
		t.Fatal("renewing a revoked delegation must be refused as fenced")
	}
}

func TestAckDelegation_AdvancesAndNeverRegresses(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio", "2026-10-22T10:00:00Z", "2026-10-22T18:00:00Z", "studio")); err != nil {
		t.Fatal(err)
	}
	if err := l.AckDelegation("studio-m5", 5); err != nil {
		t.Fatal(err)
	}
	d, err := l.GetDelegation("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if d.AckedSeq != 5 {
		t.Fatalf("want acked_seq 5, got %d", d.AckedSeq)
	}
	if err := l.AckDelegation("studio-m5", 2); err != nil {
		t.Fatal(err)
	}
	d, err = l.GetDelegation("studio-m5")
	if err != nil {
		t.Fatal(err)
	}
	if d.AckedSeq != 5 {
		t.Fatalf("acked_seq must never regress, got %d want 5", d.AckedSeq)
	}
}

func TestAckDelegation_UnknownPodErrors(t *testing.T) {
	l := fixedLedger("2026-10-22T10:00:00Z")
	if err := l.AckDelegation("never-delegated", 1); err == nil {
		t.Fatal("acking a pod with no delegation must error")
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
	// Re-issue bumps the epoch, so the first epoch is now stale.
	if _, err := l.Delegate(mkDelegation("studio-m5", "M5-Studio-2", "2026-10-22T10:05:00Z", "2026-10-22T18:05:00Z", "studio")); err != nil {
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
