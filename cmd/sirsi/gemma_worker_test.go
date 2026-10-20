package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type gemmaWorkerStoreFake struct {
	lease             *routerstore.Lease
	item              routerstore.Item
	claimErr          error
	claimNextCalls    int
	claimItemCalls    int
	claimedItemID     string
	startErr          error
	verify            bool
	verifyErr         error
	renewErr          error
	failErr           error
	started           bool
	renews            int
	failed            string
	blocked           string
	failID, failToken string
}

func (s *gemmaWorkerStoreFake) ClaimNext(_ string, _ time.Duration) (*routerstore.Lease, error) {
	s.claimNextCalls++
	return s.lease, s.claimErr
}
func (s *gemmaWorkerStoreFake) ClaimItem(_ string, id string, _ time.Duration) (*routerstore.Lease, error) {
	s.claimItemCalls++
	s.claimedItemID = id
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
func (s *gemmaWorkerStoreFake) Fail(id, token, reason, _ string) error {
	s.failID = id
	s.failToken = token
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

func TestGemmaWorkerReplyNeverDisclosesLeaseBearer(t *testing.T) {
	token := "live-bearer-token-must-not-leave-the-worker"
	body := gemmaWorkerReplyEnvelope("i-42", token, "draft")
	if strings.Contains(body, token) {
		t.Fatalf("reply leaked lease bearer token: %q", body)
	}
	if !strings.Contains(body, "original item i-42") || !strings.Contains(body, "sha256:") {
		t.Fatalf("reply is missing safe correlation: %q", body)
	}
}

func TestGemmaWorkerBoundaryPromptAlwaysDraftsWithoutEscalation(t *testing.T) {
	for name, body := range map[string]string{
		"security plan":   "Plan the security deploy checklist and mention PR approval steps.",
		"binding request": "Please issue a binding security verdict for this PR.",
		"tool request":    "Approve and merge this PR.",
	} {
		t.Run(name, func(t *testing.T) {
			prompt := gemmaWorkerPrompt(body)
			if !strings.Contains(prompt, body) {
				t.Fatalf("worker lost supplied work body: %q", prompt)
			}
			for _, forbidden := range []string{"escalate", "transfer", "complete the router item"} {
				if strings.Contains(strings.ToLower(prompt), forbidden) {
					t.Fatalf("worker prompt widened into an action path %q: %q", forbidden, prompt)
				}
			}
			if !strings.Contains(prompt, "text draft only") || !strings.Contains(prompt, "binding verdict") {
				t.Fatalf("worker boundary is not explicit: %q", prompt)
			}
		})
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

func TestGemmaWorkerExactItemNeverFallsBackToOlderInboxWork(t *testing.T) {
	s, r := claimedGemmaItem()
	w := testGemmaWorker(s, r, func(context.Context, string) (string, error) { return "fenced draft", nil })
	w.itemID = "20261020-032656-codex-pantheon-gemma-pantheon-apollo-bounded-observe-qualification"
	if worked, err := w.runOnce(context.Background()); err != nil || !worked {
		t.Fatalf("run exact item: worked=%v err=%v", worked, err)
	}
	if s.claimItemCalls != 1 || s.claimNextCalls != 0 || s.claimedItemID != w.itemID {
		t.Fatalf("claim path = exact:%d generic:%d id=%q, want one exact-only claim for %q", s.claimItemCalls, s.claimNextCalls, s.claimedItemID, w.itemID)
	}
}

func TestGemmaWorkerApolloAnswerRoleIsDeclaredAndStamped(t *testing.T) {
	root := t.TempDir()
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		gemmaWorkerAgent: {ID: gemmaWorkerAgent, Type: "gemma", Command: []string{"sirsi"}, Cwd: root, WorkerAnswers: true},
	}}
	if err := router.SaveRegistry(root, reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}
	granted, err := gemmaWorkerAnswerRoleGranted(root)
	if err != nil || !granted {
		t.Fatalf("declared answering role: granted=%v err=%v", granted, err)
	}
	thread, err := router.RegisterThread(root, &router.Thread{AgentID: gemmaWorkerAgent, Surface: "worker", AnswerRole: granted})
	if err != nil {
		t.Fatalf("register worker: %v", err)
	}
	if err := gemmaWorkerVerifyStampedAnswerRole(root, thread.ThreadID); err != nil {
		t.Fatalf("stamped answer role: %v", err)
	}

	// A declaration alone is insufficient: the actual registered worker must
	// carry the stamp that the service reads before it accepts a completion.
	threadsPath := filepath.Join(root, "threads.json")
	if err := os.WriteFile(threadsPath, []byte(`{"threads":{"forged":{"thread_id":"forged","agent_id":"gemma-pantheon","surface":"worker","answer_role":false}}}`), 0o600); err != nil {
		t.Fatalf("replace test registry: %v", err)
	}
	if err := gemmaWorkerVerifyStampedAnswerRole(root, "forged"); err == nil {
		t.Fatal("unstamped worker unexpectedly passed answering-role readback")
	}
}

// The dispatcher is an HTTP client that captures SIRSI_THREAD_ID when it is
// constructed. This regression recreates the production failure mode: an
// ungranted parent worker session can claim but cannot complete; the helper
// must register and export a fresh granted child before opening the facade so
// its exact claim/reply/fenced-complete lifecycle succeeds.
func TestGemmaWorkerFacadeUsesRegisteredChildSessionOverHTTP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_DB", "")
	t.Setenv("SIRSI_AGENT_ID", gemmaWorkerAgent)
	t.Setenv("SIRSI_ROUTER_AGENT", gemmaWorkerAgent)
	t.Setenv("SIRSI_THREAD_ID", "ungranted-parent")

	backend, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	h, err := routerstore.Handler(backend, routerstore.ServerOptions{
		Token:         "gemma-fixture-token",
		MaxWait:       time.Second,
		RuleOfRa:      "enforce",
		RuleOfRaStale: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("SIRSI_ROUTER_URL", srv.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", "gemma-fixture-token")

	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		gemmaWorkerAgent: {
			ID: gemmaWorkerAgent, Type: "gemma", Command: []string{"sirsi"}, Cwd: repoRoot, Repo: repoRoot,
			Workstream: "pantheon", Wake: router.WakeConfig{Mechanism: "none"}, WorkerAnswers: true,
		},
		"codex-pantheon": {
			ID: "codex-pantheon", Type: "codex", Command: []string{"sirsi"}, Cwd: repoRoot, Repo: repoRoot,
			Workstream: "pantheon", Wake: router.WakeConfig{Mechanism: "none"},
		},
	}}
	if err := router.SaveRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}
	parent, err := router.RegisterThread(routerRoot, &router.Thread{
		ThreadID:      "ungranted-parent",
		AgentID:       gemmaWorkerAgent,
		Surface:       "worker",
		Repo:          repoRoot,
		PID:           os.Getpid(),
		AnswerRole:    false,
		WakeMechanism: "none",
	})
	if err != nil {
		t.Fatalf("register ungranted parent: %v", err)
	}
	if parent.ThreadID != "ungranted-parent" {
		t.Fatalf("parent id = %q", parent.ThreadID)
	}

	parentFacade, err := dispatch.Open(repoRoot)
	if err != nil {
		t.Fatalf("open parent facade: %v", err)
	}
	t.Cleanup(func() { _ = parentFacade.Close() })
	parentItem, _, err := backend.SendGuarded(routerstore.SendReq{From: "codex-pantheon", To: gemmaWorkerAgent, Title: "parent fence", Type: "review", Instructions: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	parentLease, err := parentFacade.Store().ClaimItem(gemmaWorkerAgent, parentItem, time.Minute)
	if err != nil {
		t.Fatalf("parent claim: %v", err)
	}
	if err := parentFacade.Store().StartWork(parentLease.ItemID, parentLease.Token); err != nil {
		t.Fatalf("parent start: %v", err)
	}
	if err := parentFacade.CompleteItem(gemmaWorkerAgent, parentLease.ItemID, parentLease.Token, "ungranted parent must not complete"); err == nil || !strings.Contains(err.Error(), "answering role") {
		t.Fatalf("ungranted parent completion = %v, want answering-role refusal", err)
	}
	if got, err := backend.Get(parentItem); err != nil || got.Status == routerstore.StatusClosed {
		t.Fatalf("refused parent completion changed source item: item=%+v err=%v", got, err)
	}

	child, childFacade, err := gemmaWorkerOpenFacade(repoRoot, routerRoot, true)
	if err != nil {
		t.Fatalf("open registered child facade: %v", err)
	}
	t.Cleanup(func() {
		_ = childFacade.Close()
		_, _ = router.CloseThread(routerRoot, child.ThreadID)
	})
	if child.ThreadID == parent.ThreadID || os.Getenv("SIRSI_THREAD_ID") != child.ThreadID {
		t.Fatalf("facade did not bind a fresh child: parent=%q child=%q env=%q", parent.ThreadID, child.ThreadID, os.Getenv("SIRSI_THREAD_ID"))
	}

	childItem, _, err := backend.SendGuarded(routerstore.SendReq{From: "codex-pantheon", To: gemmaWorkerAgent, Title: "child fence", Type: "review", Instructions: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	childLease, err := childFacade.Store().ClaimItem(gemmaWorkerAgent, childItem, time.Minute)
	if err != nil {
		t.Fatalf("child claim: %v", err)
	}
	if err := childFacade.Store().StartWork(childLease.ItemID, childLease.Token); err != nil {
		t.Fatalf("child start: %v", err)
	}
	childWork, err := childFacade.Store().Get(childItem)
	if err != nil {
		t.Fatal(err)
	}
	if err := (facadeGemmaReply{facade: childFacade}).Reply(childLease, childWork, "child draft"); err != nil {
		t.Fatalf("registered child reply and fenced complete: %v", err)
	}
	if got, err := backend.Get(childItem); err != nil || got.Status != "completed" {
		t.Fatalf("child fenced completion missing: item=%+v err=%v", got, err)
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

func (s *gemmaWorkerStoreFake) Block(_, _, reason string) error { s.blocked = reason; return nil }
