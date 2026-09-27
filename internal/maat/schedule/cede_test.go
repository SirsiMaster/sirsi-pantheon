package schedule

import (
	"testing"
	"time"
)

func mkCede(resource, requester, holder, ask string, minutes int, reason string) Cede {
	return Cede{Resource: resource, Requester: requester, Holder: holder, Ask: ask, Minutes: minutes, Reason: reason}
}

func TestRequestCede_PendingAndNeverTouchesReservations(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	// A live reservation held by the same lane the cede targets.
	res, err := l.Reserve(mkReq("m1", "sne", "2026-09-26T09:00:00Z", "2026-09-26T11:00:00Z", RegimeLoaded), false)
	if err != nil {
		t.Fatal(err)
	}

	c, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "need the box for H9"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != CedeStatusPending || c.ID == "" {
		t.Fatalf("want pending cede with an id, got %+v", c)
	}

	// The reservation must be untouched — filing an ask is not a hold.
	rows, err := l.Status("m1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != res.Reservation.ID || rows[0].Status != StatusActive {
		t.Fatalf("RequestCede must never change a reservation's status, got %+v", rows)
	}
}

func TestCede_GrantThenDone(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "cores:2", 20, "need cores for a run"))

	got, err := l.RespondCede(c.ID, "sne", CedeStatusGranted, "sure, after my run", "after my current run", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CedeStatusGranted || got.Decision == nil || got.Decision.By != "sne" || got.Decision.StartAt != "after my current run" {
		t.Fatalf("want granted with decision recorded, got %+v", got)
	}

	// Once the holder actually releases/doesn't renew, the exchange is done.
	done, err := l.RespondCede(c.ID, "sne", CedeStatusGranted, "", "", "")
	if err == nil {
		t.Fatalf("a second respond on a non-pending cede must be refused, got %+v", done)
	}
}

func TestCede_Counter(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m5", "claude-io", "sne", "machine", 60, "need the whole box"))
	got, err := l.RespondCede(c.ID, "sne", CedeStatusCountered, "can only spare cores", "", "cores:4 for 30m at 2026-09-26T11:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CedeStatusCountered || got.Decision.Counter == "" {
		t.Fatalf("want countered with a counter-offer, got %+v", got)
	}
}

func TestCede_Decline(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "need it"))
	got, err := l.RespondCede(c.ID, "sne", CedeStatusDeclined, "mid a 6h run, can't cede", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CedeStatusDeclined || got.Decision.Reason == "" {
		t.Fatalf("want declined with a reason, got %+v", got)
	}
}

func TestCede_NonHolderRespondRefused(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "need it"))
	if _, err := l.RespondCede(c.ID, "someone-else", CedeStatusGranted, "not mine to give", "", ""); err == nil {
		t.Fatal("only the named holder may answer")
	}
}

func TestCede_DoubleRespondRefused(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "need it"))
	if _, err := l.RespondCede(c.ID, "sne", CedeStatusGranted, "ok", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RespondCede(c.ID, "sne", CedeStatusDeclined, "changed my mind", "", ""); err == nil {
		t.Fatal("a cede already answered must be refused a second respond")
	}
}

func TestCede_WithdrawByRequesterOnly(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "need it"))
	if _, err := l.WithdrawCede(c.ID, "sne"); err == nil {
		t.Fatal("only the requester may withdraw")
	}
	got, err := l.WithdrawCede(c.ID, "claude-io")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != CedeStatusWithdrawn {
		t.Fatalf("want withdrawn, got %+v", got)
	}
	if _, err := l.WithdrawCede(c.ID, "claude-io"); err == nil {
		t.Fatal("withdrawing a non-pending cede must be refused")
	}
}

func TestCede_ValidatesMinutesAndAsk(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 0, "x")); err == nil {
		t.Fatal("minutes 0 must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 61, "x")); err == nil {
		t.Fatal("minutes > 60 must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "cores:0", 10, "x")); err == nil {
		t.Fatal("cores:0 must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "cores:65", 10, "x")); err == nil {
		t.Fatal("cores:65 must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "gpu", 10, "x")); err == nil {
		t.Fatal("an ask other than machine/cores:N must be rejected")
	}
	if _, err := l.RequestCede(mkCede("BAD RESOURCE", "claude-io", "sne", "machine", 10, "x")); err == nil {
		t.Fatal("bad resource must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "", "sne", "machine", 10, "x")); err == nil {
		t.Fatal("empty requester must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "", "machine", 10, "x")); err == nil {
		t.Fatal("empty holder must be rejected")
	}
	if _, err := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 10, "")); err == nil {
		t.Fatal("empty reason must be rejected")
	}
}

func TestCede_UnansweredStaysPending_NoAutoGrant(t *testing.T) {
	l := NewLedger(newFakeStore())
	base := at("2026-09-26T10:00:00Z")
	clock := base
	l.WithClock(func() time.Time { return clock })

	c, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "need it"))
	// Time passes — far beyond any reservation lease TTL — with no response.
	clock = base.Add(6 * time.Hour)
	list, err := l.ListCedes(CedeFilter{Resource: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != c.ID || list[0].Status != CedeStatusPending {
		t.Fatalf("an unanswered cede must stay pending indefinitely, got %+v", list)
	}
}

func TestListCedes_Filters(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 30, "a"))
	l.RequestCede(mkCede("m5", "sne", "claude-io", "cores:4", 20, "b"))
	c3, _ := l.RequestCede(mkCede("m1", "sne", "claude-io", "machine", 10, "c"))
	l.RespondCede(c3.ID, "claude-io", CedeStatusDeclined, "no", "", "")

	all, _ := l.ListCedes(CedeFilter{})
	if len(all) != 3 {
		t.Fatalf("want 3 cedes total, got %d", len(all))
	}
	onM1, _ := l.ListCedes(CedeFilter{Resource: "m1"})
	if len(onM1) != 2 {
		t.Fatalf("want 2 cedes on m1, got %d", len(onM1))
	}
	pending, _ := l.ListCedes(CedeFilter{Resource: "m1", PendingOnly: true})
	if len(pending) != 1 || pending[0].Requester != "claude-io" {
		t.Fatalf("want 1 pending cede on m1, got %+v", pending)
	}
	byHolder, _ := l.ListCedes(CedeFilter{Holder: "claude-io"})
	if len(byHolder) != 2 {
		t.Fatalf("want 2 cedes held by claude-io, got %d", len(byHolder))
	}
	byHolderSne, _ := l.ListCedes(CedeFilter{Holder: "sne"})
	if len(byHolderSne) != 1 || byHolderSne[0].Requester != "claude-io" {
		t.Fatalf("want 1 cede held by sne, got %+v", byHolderSne)
	}
}

// --- "no ticket beyond an open ask, but never a lockout" (owner directive 2026-09-26) ---

func TestReserve_PendingCedeGrantsFloorNotRefusal(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	res, err := l.Reserve(mkReq("m5", "sne", "2026-09-26T09:00:00Z", "2026-09-26T09:12:00Z", RegimeLoaded), false)
	if err != nil || !res.Granted {
		t.Fatalf("initial reserve: %v %+v", err, res)
	}
	c, _ := l.RequestCede(mkCede("m5", "claude-io", "sne", "machine", 20, "need m5"))

	// A new reservation for sne is GRANTED, but only the floor share.
	capped, err := l.Reserve(mkReq("m5", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:20:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if !capped.Granted {
		t.Fatal("a pending cede must never refuse the reserve — floor grant only")
	}
	if capped.Reservation.Share != ShareFloor || capped.Reservation.Cores != 4 { // m5 default 18/4
		t.Fatalf("want a floor grant of 4 cores on m5, got %+v", capped.Reservation)
	}
	if len(capped.PendingCedes) != 1 || capped.PendingCedes[0] != c.ID {
		t.Fatalf("want the open cede named, got %+v", capped.PendingCedes)
	}

	// Extending the CURRENT ticket also never locks out.
	ext, err := l.Extend(res.Reservation.ID, "2026-09-26T09:30:00Z")
	if err != nil {
		t.Fatalf("extend must never be refused, got %v", err)
	}
	if ext.EstEnd != "2026-09-26T09:30:00Z" {
		t.Fatalf("extend must still push the end out, got %+v", ext)
	}
	if ext.Share != ShareFloor || ext.Cores != 4 {
		t.Fatalf("extend while a cede is pending must cap cores to the floor, got %+v", ext)
	}
	if len(ext.PendingCedes) != 1 || ext.PendingCedes[0] != c.ID {
		t.Fatalf("extend result must carry the open cede id, got %+v", ext)
	}

	// Heartbeat and Release are always available regardless.
	if _, err := l.Heartbeat(res.Reservation.ID); err != nil {
		t.Fatalf("heartbeat must still work: %v", err)
	}
	if _, err := l.Release(res.Reservation.ID); err != nil {
		t.Fatalf("release must still work: %v", err)
	}
}

func TestReserve_FullGrantRestoredAfterEachResponseType(t *testing.T) {
	for _, status := range []CedeStatus{CedeStatusGranted, CedeStatusCountered, CedeStatusDeclined} {
		l := fixedLedger("2026-09-26T10:00:00Z")
		c, _ := l.RequestCede(mkCede("m5", "claude-io", "sne", "machine", 20, "need m5"))
		capped, _ := l.Reserve(mkReq("m5", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:20:00Z", RegimeQuiet), false)
		if capped.Reservation.Share != ShareFloor {
			t.Fatalf("[%s] must be a floor grant before the response", status)
		}
		if _, err := l.RespondCede(c.ID, "sne", status, "answered", "", "counter offer"); err != nil {
			t.Fatalf("[%s] respond: %v", status, err)
		}
		full, err := l.Reserve(mkReq("m5", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:20:00Z", RegimeQuiet), false)
		if err != nil || !full.Granted || full.Reservation.Share != ShareFull {
			t.Fatalf("[%s] must be a full grant after the response, got %v %+v", status, err, full)
		}
	}
}

func TestReserve_FullGrantRestoredAfterWithdraw(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c, _ := l.RequestCede(mkCede("m5", "claude-io", "sne", "machine", 20, "need m5"))
	capped, _ := l.Reserve(mkReq("m5", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:20:00Z", RegimeQuiet), false)
	if capped.Reservation.Share != ShareFloor {
		t.Fatal("must be a floor grant before the withdraw")
	}
	if _, err := l.WithdrawCede(c.ID, "claude-io"); err != nil {
		t.Fatal(err)
	}
	full, err := l.Reserve(mkReq("m5", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:20:00Z", RegimeQuiet), false)
	if err != nil || !full.Granted || full.Reservation.Share != ShareFull {
		t.Fatalf("must be a full grant after the withdraw, got %v %+v", err, full)
	}
}

func TestReserve_PendingCedeToOtherLaneDoesNotCapThisOne(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	l.RequestCede(mkCede("m5", "claude-io", "sne", "machine", 20, "need m5")) // addressed to sne, not claude-io
	granted, err := l.Reserve(mkReq("m1", "claude-io", "2026-09-26T10:00:00Z", "2026-09-26T10:20:00Z", RegimeQuiet), false)
	if err != nil || !granted.Granted || granted.Reservation.Share != ShareFull {
		t.Fatalf("a cede addressed to another lane must not cap this one, got %v %+v", err, granted)
	}
}

func TestReserve_FloorGrantListsEveryOpenID(t *testing.T) {
	l := fixedLedger("2026-09-26T10:00:00Z")
	c1, _ := l.RequestCede(mkCede("m1", "claude-io", "sne", "machine", 20, "a"))
	c2, _ := l.RequestCede(mkCede("m5", "sha", "sne", "cores:2", 10, "b"))
	capped, err := l.Reserve(mkReq("m1", "sne", "2026-09-26T10:00:00Z", "2026-09-26T10:10:00Z", RegimeQuiet), false)
	if err != nil {
		t.Fatal(err)
	}
	if !capped.Granted || len(capped.PendingCedes) != 2 {
		t.Fatalf("want a floor grant naming both open ids, got %+v", capped)
	}
	got := map[string]bool{capped.PendingCedes[0]: true, capped.PendingCedes[1]: true}
	if !got[c1.ID] || !got[c2.ID] {
		t.Fatalf("floor grant must list every open id, want %s and %s, got %+v", c1.ID, c2.ID, capped.PendingCedes)
	}
}
