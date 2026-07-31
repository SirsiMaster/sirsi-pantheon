package reason

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// codex-pantheon blocked PR #340 until "restart verification proves a new PID
// plus a live endpoint". The original Verify only checked pid > 0, so it passed
// in both failure modes that actually occur:
//
//  1. the restart did nothing and the OLD process is still running
//  2. the process is alive but its HTTP server is wedged or still loading
//
// These tests call restartVerdict — the PRODUCTION decision rule, extracted from
// the polling loop so it can be exercised without a live broker. They do not
// restate the rule: a test that reimplements the logic proves only that the copy
// agrees with itself, which is the same defect shape the rule exists to catch.

func TestRestartVerifyRejectsUnchangedPID(t *testing.T) {
	// The restart command exited 0 but replaced nothing.
	ok, reason := restartVerdict(4242, 4242, "gemma-3-12b", nil)
	if ok {
		t.Fatal("a broker with the SAME pid must not verify as restarted — the process was never replaced")
	}
	if !strings.Contains(reason, "pid unchanged") {
		t.Fatalf("reason = %q, want the unchanged-pid rejection", reason)
	}
}

func TestRestartVerifyRejectsLivePIDWithDeadEndpoint(t *testing.T) {
	// New process, but the server is wedged or still loading weights. This is
	// the green-surface-over-a-dead-thing case: a pid is not a service.
	ok, reason := restartVerdict(4242, 5150, "", context.DeadlineExceeded)
	if ok {
		t.Fatal("a new pid whose endpoint does not serve must not verify — a live process is not a live model")
	}
	if !strings.Contains(reason, "endpoint not serving") {
		t.Fatalf("reason = %q, want the dead-endpoint rejection", reason)
	}
}

func TestRestartVerifyRejectsEndpointServingNoModel(t *testing.T) {
	// The server answers but has no model loaded — it can accept a connection
	// and still be useless, which reads as healthy to any transport-only probe.
	ok, _ := restartVerdict(4242, 5150, "   ", nil)
	if ok {
		t.Fatal("an endpoint that names no model must not verify as a healthy broker")
	}
}

func TestRestartVerifyRejectsAbsentProcess(t *testing.T) {
	if ok, _ := restartVerdict(4242, 0, "", nil); ok {
		t.Fatal("no broker process must never verify")
	}
}

func TestRestartVerifyAcceptsNewPIDServingAModel(t *testing.T) {
	ok, reason := restartVerdict(4242, 5150, "gemma-3-12b-8bit", nil)
	if !ok {
		t.Fatalf("a NEW pid serving a named model is a verified restart, got %q", reason)
	}
}

// If nothing was running before, any live serving broker is a valid outcome —
// the rule must not demand a pid CHANGE when there was no prior pid to change.
func TestRestartVerifyAcceptsColdStart(t *testing.T) {
	if ok, reason := restartVerdict(0, 5150, "gemma-3-12b", nil); !ok {
		t.Fatalf("cold start (no prior pid) must verify, got %q", reason)
	}
}

// codex-pantheon (router item 20260729-193639): pidBefore was shared closure
// state between Run and Verify, so concurrent invocations of the same registered
// tool could overwrite each other's baseline and produce false verdicts. Verify
// now receives THIS invocation's Run result, so the baseline cannot cross over.
func TestVerifyBaselineIsInvocationLocal(t *testing.T) {
	var mu sync.Mutex
	seen := map[int]int{} // pid_before observed by Verify → count

	// A tool whose Run stamps a distinct baseline per invocation and whose Verify
	// reports back the baseline it was handed.
	var nextPID int64 = 1000
	tool := Tool{
		Name:       "test.baseline",
		Does:       "stamp a per-invocation baseline",
		Tier:       TierRepair,
		Reversible: true,
		Run: func(ctx context.Context) (Result, error) {
			pid := int(atomic.AddInt64(&nextPID, 1))
			return Result{Evidence: map[string]any{"pid_before": pid}, Changed: true}, nil
		},
		Verify: func(ctx context.Context, ran Result) (Result, error) {
			got, _ := ran.Evidence["pid_before"].(int)
			mu.Lock()
			seen[got]++
			mu.Unlock()
			return Result{Summary: "ok"}, nil
		},
	}

	r := NewRegistry()
	if err := r.Register(tool); err != nil {
		t.Fatalf("register: %v", err)
	}

	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Invoke(context.Background(), r, "test.baseline", PolicyAutoRepair)
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Fatalf("Verify observed %d distinct baselines across %d concurrent invocations — baselines are crossing between invocations", len(seen), n)
	}
	for pid, count := range seen {
		if count != 1 {
			t.Fatalf("baseline %d was observed %d times — it must belong to exactly one invocation", pid, count)
		}
	}
}

// A restart that comes back serving a DIFFERENT model is not a clean reload: the
// resolver re-ranked, usually because less RAM was free, and the machine now has
// less capability than before. Observed live 2026-07-29 —
// gemma-4-12B-it-8bit came back as Qwen2.5-3B-Instruct-4bit. That must be stated
// in the summary a user reads, not left in an evidence field nobody prints.
func TestRestartSummaryDisclosesAModelChange(t *testing.T) {
	// The verdict rule itself must still PASS — a smaller model is a working
	// broker, so this is a disclosure requirement, not a failure condition.
	if ok, reason := restartVerdict(4242, 5150, "Qwen2.5-3B-Instruct-4bit", nil); !ok {
		t.Fatalf("a serving broker must verify even if the model changed, got %q", reason)
	}

	summary, changed := restartSummary(
		4242,
		5150,
		"gemma-4-12B-it-8bit",
		"Qwen2.5-3B-Instruct-4bit",
		8<<30,
	)
	if !changed {
		t.Fatal("different before/after models must set model_changed")
	}
	for _, want := range []string{"⚠ MODEL CHANGED", `"gemma-4-12B-it-8bit"`, `"Qwen2.5-3B-Instruct-4bit"`} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestRestartSummaryDoesNotWarnWhenModelIsUnchanged(t *testing.T) {
	summary, changed := restartSummary(4242, 5150, "gemma-4-12B-it-8bit", "gemma-4-12B-it-8bit", 8<<30)
	if changed {
		t.Fatal("same model must not be reported as changed")
	}
	if strings.Contains(summary, "MODEL CHANGED") {
		t.Fatalf("unchanged model received a false warning:\n%s", summary)
	}
}
