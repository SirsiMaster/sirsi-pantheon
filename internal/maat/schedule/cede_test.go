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
