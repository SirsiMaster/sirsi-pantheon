package routerstore

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// budgetVerifier answers from a map (default: valid) and advances a fake clock
// 10 s per verification, so the 60 s budget with a 20 s reserve allows exactly 5.
type budgetVerifier struct {
	now    *time.Time
	answer map[string]error
	seen   *[]string
}

func (b budgetVerifier) Verify(_ context.Context, kind, value string) error {
	*b.now = b.now.Add(10 * time.Second)
	*b.seen = append(*b.seen, kind+":"+value)
	return b.answer[kind+":"+value]
}

func TestRecheckIsBoundedAndMakesProgressAcrossCalls(t *testing.T) {
	clock := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	prev := receiptRecheckNow
	receiptRecheckNow = func() time.Time { return clock }
	t.Cleanup(func() { receiptRecheckNow = prev })

	var seen []string
	v := budgetVerifier{now: &clock, answer: map[string]error{}, seen: &seen}
	useVerifier(t, v)
	s := openTestStore(t)

	rc := func(n int) string { return fmt.Sprintf("pr:SirsiMaster/sirsi-pantheon#%d", 100+n) }
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("t%02d", i)
		if err := s.CompleteTaskLease("lane", id, claimed(t, s, "lane", id), rc(i)); err != nil {
			t.Fatal(err)
		}
	}
	seen = nil
	v.answer[rc(1)] = fmt.Errorf("%w: GitHub answered 503", ErrReceiptUnverifiable) // stays unverifiable forever
	v.answer[rc(3)] = fmt.Errorf("%w: merge commit gone", ErrReceiptInvalid)        // proven false

	ids := func(out []ReceiptRecheck) (got []string) {
		for _, r := range out {
			if r.TaskID != "" {
				got = append(got, r.TaskID)
			}
		}
		return got
	}
	// Call 1: five receipts, then the budget stops it before a sixth starts.
	out, err := s.RecheckTaskReceipts(0)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(out), ","); got != "t01,t02,t03,t04,t05" || len(seen) != 5 {
		t.Fatalf("call 1 checked %q (verifier saw %d)", got, len(seen))
	}
	sum := out[len(out)-1]
	if sum.TaskID != "" || sum.Checked != 5 || sum.Remaining != 7 {
		t.Fatalf("summary %+v, want checked 5 remaining 7", sum)
	}
	// Only the proven-false receipt reopened; unverifiable and never-started stay done.
	for id, want := range map[string]string{"t01": "done", "t03": "pending", "t04": "done", "t12": "done"} {
		if got, _ := s.GetTask("lane", id); got.Status != want {
			t.Errorf("%s = %s, want %s", id, got.Status, want)
		}
	}
	// Call 2 resumes after the cursor even though t01 is still unverifiable.
	if out, err = s.RecheckTaskReceipts(0); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(out), ","); got != "t06,t07,t08,t09,t10" {
		t.Fatalf("call 2 checked %q, want t06..t10 (no progress = cursor lost)", got)
	}
	// Call 3 finishes the pass: no summary entry, cursor resets.
	if out, err = s.RecheckTaskReceipts(0); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(out), ","); got != "t11,t12" || len(out) != 2 {
		t.Fatalf("call 3 = %+v", out)
	}
	if c, _, _ := s.GetState(receiptRecheckCursorKey); c != "" {
		t.Fatalf("cursor %q after a full pass, want reset", c)
	}
	// Call 4 starts a new pass from the first receipt.
	if out, err = s.RecheckTaskReceipts(0); err != nil || ids(out)[0] != "t01" {
		t.Fatalf("call 4 = %+v %v", out, err)
	}
}
