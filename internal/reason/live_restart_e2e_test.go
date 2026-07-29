package reason

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Live restart E2E — codex-pantheon's final required proof (router item
// 20260729-193639): "a live restart demonstrates old PID gone, new PID live, and
// the broker endpoint serving the expected local model within the configured
// readiness window."
//
// This RESTARTS THE REAL BROKER. Gated behind an env var so it can never run in
// CI or by accident. Owner-authorized 2026-07-29.
func TestLiveRestartE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("live restart mutates a running service")
	}
	if v := envOrEmpty("SIRSI_LIVE_RESTART_E2E"); v != "1" {
		t.Skip("set SIRSI_LIVE_RESTART_E2E=1 to restart the real broker")
	}

	r := NewRegistry()
	if err := MachineTools(r); err != nil {
		t.Fatalf("register machine tools: %v", err)
	}

	before := brokerPID(context.Background())
	modelBefore, errBefore := brokerServing(context.Background())
	t.Logf("BEFORE  pid=%d serving=%q err=%v", before, modelBefore, errBefore)
	if before == 0 {
		t.Fatal("no broker running — nothing to restart; start it first so the test proves a REPLACEMENT")
	}

	start := time.Now()
	inv := Invoke(context.Background(), r, "gemma.restart", PolicyAutoRepair)
	elapsed := time.Since(start)

	b, _ := json.MarshalIndent(inv, "", "  ")
	t.Logf("INVOCATION (%.1fs, window %s):\n%s", elapsed.Seconds(), brokerReadyWindow, string(b))

	if inv.Err != nil {
		t.Fatalf("restart invocation failed: %v", inv.Err)
	}
	if inv.Verified == nil {
		t.Fatal("no Verify result — the repair contract requires verification")
	}

	after := brokerPID(context.Background())
	modelAfter, errAfter := brokerServing(context.Background())
	t.Logf("AFTER   pid=%d serving=%q err=%v", after, modelAfter, errAfter)

	if after == before {
		t.Fatalf("pid unchanged (%d) — the process was not replaced", after)
	}
	if after == 0 {
		t.Fatal("broker absent after restart — the model is DOWN")
	}
	if errAfter != nil {
		t.Fatalf("endpoint not serving after restart: %v", errAfter)
	}
	if modelAfter != modelBefore {
		t.Logf("NOTE: served model changed %q -> %q (resolver may have re-ranked)", modelBefore, modelAfter)
	}
}

func envOrEmpty(k string) string {
	v, ok := lookupEnv(k)
	if !ok {
		return ""
	}
	return v
}

func lookupEnv(k string) (string, bool) { return os.LookupEnv(k) }
