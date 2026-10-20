package router

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestProviderQuotaDiagnostics(t *testing.T) {
	for _, s := range []string{"You've hit your monthly spend limit.", "Error: monthly spend limit reached", "You've hit your monthly spend limit · raise it at claude.ai/settings/usage?foo=bar · your weekly limit resets10pm"} {
		if got := knownExitedFailure(s, errors.New("exit1")); got != OutcomeProviderQuota {
			t.Fatalf("diagnostic %q: %s", s, got)
		}
		if got := knownExitedFailure(s, nil); got == OutcomeProviderQuota {
			t.Fatal("successful output must not block")
		}
	}
	for _, s := range []string{"item: You've hit your monthly spend limit.", "item body\nYou've hit your monthly spend limit.", `"You've hit your monthly spend limit."`, "monthly budget report", "429 rate limited", "quota may reset tomorrow", "Please summarize monthly spend limit reached"} {
		if KnownFailure(s) == OutcomeProviderQuota {
			t.Fatalf("arbitrary/ambiguous text blocked: %q", s)
		}
	}
}

func TestProviderQuotaFailedExitAfterProgressStillHolds(t *testing.T) {
	old := getDispatchDir()
	SetDispatchDir(t.TempDir())
	t.Cleanup(func() { SetDispatchDir(old) })
	// Durable actions earlier in a consumer do not restore provider capacity.
	blocked, err := observeProviderQuotaExit("progress", "You've hit your monthly spend limit.", errors.New("exit1"))
	if err != nil || !blocked {
		t.Fatalf("failed exit did not arm independent hold: %v %v", blocked, err)
	}
	if held, err := providerQuotaBlocked("progress"); err != nil || !held {
		t.Fatalf("independent hold lost: %v %v", held, err)
	}
	if held, err := observeProviderQuotaExit("successful", "You've hit your monthly spend limit.", nil); err != nil || held {
		t.Fatalf("successful quoted output created a hold: %v %v", held, err)
	}
}

func TestProviderQuotaLockNeverBlocksControllerHeartbeat(t *testing.T) {
	old := getDispatchDir()
	SetDispatchDir(t.TempDir())
	t.Cleanup(func() { SetDispatchDir(old) })
	p, err := providerQuotaPath("contended")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	start := time.Now()
	if err := checkProviderQuotaStorage("contended"); err == nil {
		t.Fatal("contended controller lock was not refused")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("lock stalled controller for %s", elapsed)
	}
}
func TestProviderQuotaRestartRecoveryAndHistory(t *testing.T) {
	d := t.TempDir()
	old := getDispatchDir()
	SetDispatchDir(d)
	t.Cleanup(func() { SetDispatchDir(old) })
	if err := recordProviderQuota("lane"); err != nil {
		t.Fatal(err)
	}
	// Reload from disk, with no retained process state (controller restart).
	SetDispatchDir(d)
	if blocked, err := providerQuotaBlocked("lane"); err != nil || !blocked {
		t.Fatalf("restart lost hold: %v %v", blocked, err)
	}
	if err := RecordProviderAvailability("lane", "", 1); err == nil {
		t.Fatal("missing evidence accepted")
	}
	if err := RecordProviderAvailability("lane", "receipt:provider-availability-confirmed", 1); err != nil {
		t.Fatal(err)
	}
	if blocked, err := providerQuotaBlocked("lane"); err != nil || blocked {
		t.Fatalf("recovery failed: %v %v", blocked, err)
	}
	if err := recordProviderQuota("lane"); err != nil {
		t.Fatal(err)
	}
	r, err := readProviderQuota("lane")
	if err != nil || len(r.History) != 2 || r.Evidence == "" {
		t.Fatalf("history lost: %+v %v", r, err)
	}
	p, _ := providerQuotaPath("lane")
	if err := os.WriteFile(p, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := providerQuotaBlocked("lane"); err == nil {
		t.Fatal("corruption failed open")
	}
	if _, err := providerQuotaPath("../foreign"); err == nil {
		t.Fatal("foreign path accepted")
	}
}
func TestProviderQuotaPingAndNewerSuccess(t *testing.T) {
	l := &LaneState{ConsumerDeclared: true, LastOutcome: OutcomeProviderQuota}
	w := worker("a", true, l, time.Minute)
	if p := PingLane(regWith(w), AgentConfig{}, "a", pingNow); p.Workable || p.Verdict != "PROVIDER_QUOTA" {
		t.Fatalf("quota claimed workable: %+v", p)
	}
	a := &Thread{AgentID: "a", Surface: "claude", Status: ThreadStatusActive, LastSeenAt: pingNow}
	if p := PingLane(regWith(w, a), AgentConfig{}, "a", pingNow); p.Verdict != VerdictLive {
		t.Fatalf("attended ownership lost: %+v", p)
	}
	l.LastOutcome = OutcomeOK
	l.LastProgressAt = pingNow
	if p := PingLane(regWith(w), AgentConfig{}, "a", pingNow); !p.Workable {
		t.Fatalf("newer success lost: %+v", p)
	}
}

func TestProviderQuotaConcurrentRecoveryCannotClearNewFailure(t *testing.T) {
	old := getDispatchDir()
	SetDispatchDir(t.TempDir())
	t.Cleanup(func() { SetDispatchDir(old) })
	if err := recordProviderQuota("race"); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var failureErr, errorRecovery error
	go func() { defer wg.Done(); <-start; failureErr = recordProviderQuota("race") }()
	go func() {
		defer wg.Done()
		<-start
		errorRecovery = RecordProviderAvailability("race", "receipt:old-generation", 1)
	}()
	close(start)
	wg.Wait()
	if failureErr != nil {
		t.Fatal(failureErr)
	}
	// Recovery either wins first, or rejects the stale generation. Both orders
	// MUST leave generation2 held; no timing assumption about which wins.
	_ = errorRecovery
	r, err := readProviderQuota("race")
	if err != nil || r.Generation != 2 || len(r.History) != 2 {
		t.Fatalf("lost failure: %+v %v", r, err)
	}
	if blocked, err := providerQuotaBlocked("race"); err != nil || !blocked {
		t.Fatalf("new failure cleared: %v %v", blocked, err)
	}
	if err := RecordProviderAvailability("race", "receipt:stale", 1); err == nil {
		t.Fatal("stale recovery accepted")
	}
	if err := RecordProviderAvailability("race", "receipt:generation2-available", 2); err != nil {
		t.Fatal(err)
	}
	if blocked, err := providerQuotaBlocked("race"); err != nil || blocked {
		t.Fatalf("explicit current recovery failed: %v %v", blocked, err)
	}
}
func TestProviderQuotaPersistenceFailureHoldsBeforeDispatch(t *testing.T) {
	old := getDispatchDir()
	d := t.TempDir()
	SetDispatchDir(d)
	t.Cleanup(func() { SetDispatchDir(old) })
	// A regular file in place of the directory fails even under root, unlike chmod.
	broken := filepath.Join(d, "not-a-directory")
	if err := os.WriteFile(broken, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	SetDispatchDir(broken)
	if err := checkProviderQuotaStorage("lane"); err == nil {
		t.Fatal("unwritable storage allowed startup dispatch")
	}
	if err := recordProviderQuota("lane"); err == nil {
		t.Fatal("write failure ignored")
	}
	if blocked, err := providerQuotaBlocked("lane"); !blocked || err == nil {
		t.Fatal("persistence read failure did not hold")
	}
	SetDispatchDir(d)
	if err := recordProviderQuota("corrupt"); err != nil {
		t.Fatal(err)
	}
	p, _ := providerQuotaPath("corrupt")
	if err := os.WriteFile(p, []byte("{truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if blocked, err := providerQuotaBlocked("corrupt"); !blocked || err == nil {
		t.Fatal("restart corruption failed open")
	}
}
