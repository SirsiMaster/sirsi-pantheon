package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type gemmaWorkerStoreFake struct {
	lease     *routerstore.Lease
	item      routerstore.Item
	claimErr  error
	startErr  error
	verify    bool
	verifyErr error
	renewErr  error
	failErr   error
	started   bool
	renews    int
	failed    string
}

func (s *gemmaWorkerStoreFake) ClaimNext(_ string, _ time.Duration) (*routerstore.Lease, error) {
	return s.lease, s.claimErr
}
func (s *gemmaWorkerStoreFake) StartWork(_, _ string) error           { s.started = true; return s.startErr }
func (s *gemmaWorkerStoreFake) Get(string) (routerstore.Item, error)  { return s.item, nil }
func (s *gemmaWorkerStoreFake) AckItem(string) error                  { return nil }
func (s *gemmaWorkerStoreFake) VerifyLease(_, _ string) (bool, error) { return s.verify, s.verifyErr }
func (s *gemmaWorkerStoreFake) RenewLease(_, _ string, _ time.Duration) error {
	s.renews++
	return s.renewErr
}
func (s *gemmaWorkerStoreFake) Fail(_, _, reason, _ string) error {
	s.failed = reason
	return s.failErr
}

type gemmaWorkerReplyFake struct {
	err   error
	calls int
}

func (r *gemmaWorkerReplyFake) Reply(_ *routerstore.Lease, _ routerstore.Item, _ string) error {
	r.calls++
	return r.err
}

func testGemmaWorker(store *gemmaWorkerStoreFake, reply *gemmaWorkerReplyFake, generate func(context.Context, string) (string, error)) nativeGemmaWorker {
	return nativeGemmaWorker{store: store, reply: reply, generate: generate, leaseTTL: time.Minute}
}

func claimedGemmaItem() (*gemmaWorkerStoreFake, *gemmaWorkerReplyFake) {
	return &gemmaWorkerStoreFake{
		lease:  &routerstore.Lease{ItemID: "i-1", Token: "fenced"},
		item:   routerstore.Item{ID: "i-1", From: "codex-pantheon", Instructions: "TASK: draft\nhello"},
		verify: true,
	}, &gemmaWorkerReplyFake{}
}

func TestGemmaWorkerBrokerFailureLeavesRetryableEvidence(t *testing.T) {
	s, r := claimedGemmaItem()
	_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) { return "", errors.New("SNE unavailable") }).runOnce(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SNE unavailable") {
		t.Fatalf("want broker error, got %v", err)
	}
	if r.calls != 0 {
		t.Fatalf("reply called after broker failure")
	}
	if !strings.Contains(s.failed, "GEMMA RETRYABLE FAILURE: native inference unavailable: SNE unavailable") {
		t.Fatalf("failure evidence missing: %q", s.failed)
	}
}

func TestGemmaWorkerEmptyAndTruncatedOutputsStayOpen(t *testing.T) {
	for name, output := range map[string]string{"empty": "", "truncated": "```unfinished"} {
		t.Run(name, func(t *testing.T) {
			s, r := claimedGemmaItem()
			_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) { return output, nil }).runOnce(context.Background())
			if err == nil || r.calls != 0 || s.failed == "" {
				t.Fatalf("want unresolved %s output; err=%v reply=%d failure=%q", name, err, r.calls, s.failed)
			}
		})
	}
}

func TestGemmaWorkerReplyFailurePreservesExactDraftForRetry(t *testing.T) {
	s, r := claimedGemmaItem()
	r.err = errors.New("relay unavailable")
	draft := "the exact local draft"
	_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) { return draft, nil }).runOnce(context.Background())
	if err == nil || r.calls != 1 {
		t.Fatalf("want reply failure after one attempt: err=%v calls=%d", err, r.calls)
	}
	if !strings.Contains(s.failed, "DRAFT PRESERVED FOR RETRY:\n"+draft) {
		t.Fatalf("draft lost from retry evidence: %q", s.failed)
	}
}

func TestGemmaWorkerLeaseLossPreventsPublication(t *testing.T) {
	s, r := claimedGemmaItem()
	s.verify = false
	_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) { return "draft", nil }).runOnce(context.Background())
	if err == nil || r.calls != 0 {
		t.Fatalf("lost lease must not publish: err=%v calls=%d", err, r.calls)
	}
}

func TestGemmaWorkerRenewalLossCancelsBeforePublication(t *testing.T) {
	s, r := claimedGemmaItem()
	s.renewErr = errors.New("superseded lease")
	w := testGemmaWorker(s, r, func(ctx context.Context, _ string) (string, error) {
		<-ctx.Done()
		return "draft", nil
	})
	w.leaseTTL = 15 * time.Millisecond
	_, err := w.runOnce(context.Background())
	if err == nil || r.calls != 0 || s.renews == 0 {
		t.Fatalf("renewal loss must prevent reply: err=%v calls=%d renews=%d", err, r.calls, s.renews)
	}
}

func TestGemmaWorkerStartFailureReleasesOwnedLease(t *testing.T) {
	s, r := claimedGemmaItem()
	s.startErr = errors.New("start fence lost")
	_, err := testGemmaWorker(s, r, func(context.Context, string) (string, error) { return "draft", nil }).runOnce(context.Background())
	if err == nil || s.failed == "" || r.calls != 0 {
		t.Fatalf("start failure must retain retryable error: err=%v failure=%q replies=%d", err, s.failed, r.calls)
	}
}

func TestGemmaWorkerSingletonRejectsDuplicate(t *testing.T) {
	old := gemmaWorkerTryLock
	t.Cleanup(func() { gemmaWorkerTryLock = old })
	held := false
	gemmaWorkerTryLock = func(string) (func(), error) {
		if held {
			return nil, errors.New("already locked")
		}
		held = true
		return func() { held = false }, nil
	}
	unlock, err := gemmaWorkerTryLock("gemma-pantheon-worker")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := gemmaWorkerTryLock("gemma-pantheon-worker"); err == nil {
		t.Fatal("duplicate worker lock unexpectedly acquired")
	}
}
