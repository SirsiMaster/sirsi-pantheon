package thoth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Router item 20260730-015122: `sirsi thoth sync` wrote raw PreCompact hook
// payloads into memory.yaml as "Session Decisions". FinalWishes reached 937
// lines / 310 KB with 780 of those lines being verbatim payloads from just three
// session_ids — 95% of the file, teaching a reader nothing, and making the file
// too expensive to read at all.

const realPayload = `{"session_id":"019e8b57-0000-7000-8000-000000000000","turn_id":"019f95a6-0000-7000-8000-000000000000","transcript_path":"/Users/x/.codex/sessions/2026/06/02/rollout-abc.jsonl","cwd":"/Users/x/Development/FinalWishes","hook_event_name":"PreCompact","model":"gpt-5.5","trigger":"auto"}`

func TestIsMachinePayloadRejectsTheRealShape(t *testing.T) {
	got, why := isMachinePayload(realPayload)
	if !got {
		t.Fatal("the exact payload shape found in FinalWishes must be rejected")
	}
	if !strings.Contains(why, "PreCompact") {
		t.Fatalf("reason should name the hook event, got %q", why)
	}
}

func TestIsMachinePayloadRejectsJSONWithoutAHookField(t *testing.T) {
	// A future payload shape with different keys is still not prose.
	if got, _ := isMachinePayload(`{"some":"other","machine":"record"}`); !got {
		t.Fatal("any JSON object is machine telemetry, not a decision")
	}
}

func TestIsMachinePayloadKeepsRealDecisions(t *testing.T) {
	for _, line := range []string{
		"Chose SQLite over Postgres — single-writer workload, no ops burden",
		"Reverted the cache layer: it hid a correctness bug in the resolver",
		"Set the broker cap to 1/3 of RAM (ADR-040)",
		"Use {} as the empty-config sentinel", // braces, but not valid JSON alone
	} {
		if got, why := isMachinePayload(line); got {
			t.Fatalf("real decision rejected as %q: %s", why, line)
		}
	}
}

// The cap is the half that survives an UNKNOWN payload shape. A filter alone
// would share the shape of the bug it prevents.
func TestDecisionsSectionIsBounded(t *testing.T) {
	dir := t.TempDir()
	thothDir := filepath.Join(dir, ".thoth")
	if err := os.MkdirAll(thothDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mem := filepath.Join(thothDir, "memory.yaml")
	if err := os.WriteFile(mem, []byte("# Project\n\n"+sessionDecisionsHeader+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Far more compactions than the cap, each with a legitimate decision.
	for i := 0; i < maxSessionDecisions*3; i++ {
		if err := appendSessionDecisions(mem, "a genuine decision line"); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	raw, err := os.ReadFile(mem)
	if err != nil {
		t.Fatal(err)
	}
	n := strings.Count(string(raw), "a genuine decision line")
	if n > maxSessionDecisions {
		t.Fatalf("decisions section grew to %d entries — it must be bounded at %d", n, maxSessionDecisions)
	}
	if n == 0 {
		t.Fatal("the cap dropped everything — real decisions must survive")
	}
}

// End to end: a payload never reaches the file, and the file it lands in stays
// small. This is the property FinalWishes violated.
func TestPayloadsNeverReachMemory(t *testing.T) {
	dir := t.TempDir()
	thothDir := filepath.Join(dir, ".thoth")
	if err := os.MkdirAll(thothDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mem := filepath.Join(thothDir, "memory.yaml")
	if err := os.WriteFile(mem, []byte("# Project\n\n"+sessionDecisionsHeader+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The real-world pattern: three sessions compacting repeatedly.
	for i := 0; i < 100; i++ {
		if err := appendSessionDecisions(mem, realPayload); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	raw, err := os.ReadFile(mem)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "hook_event_name") {
		t.Fatal("a hook payload reached memory.yaml — the filter did not hold")
	}
	if strings.Contains(body, "transcript_path") {
		t.Fatal("a transcript path reached memory.yaml")
	}
	if len(raw) > 4096 {
		t.Fatalf("memory.yaml grew to %d bytes from payloads alone — it must stay small", len(raw))
	}
}

// A summary containing BOTH a payload and a real decision must keep the
// decision. Dropping the whole summary would lose real content.
func TestMixedSummaryKeepsTheDecision(t *testing.T) {
	dir := t.TempDir()
	thothDir := filepath.Join(dir, ".thoth")
	if err := os.MkdirAll(thothDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mem := filepath.Join(thothDir, "memory.yaml")
	if err := os.WriteFile(mem, []byte("# Project\n\n"+sessionDecisionsHeader+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := appendSessionDecisions(mem, realPayload+"\nBounded the decisions section at 40 entries"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(mem)
	body := string(raw)
	if !strings.Contains(body, "Bounded the decisions section at 40 entries") {
		t.Fatal("the real decision was dropped along with the payload")
	}
	if strings.Contains(body, "hook_event_name") {
		t.Fatal("the payload survived alongside the decision")
	}
}

// Router item 20260730-040045: the npm delegation path handed the RAW summary to
// a binary that writes memory.yaml itself, bypassing the Go-side filter
// entirely. SanitizeSummary is the shared gate both writers now pass through.
func TestSanitizeSummaryStripsPayloadsKeepsDecisions(t *testing.T) {
	in := realPayload + "\nChose the intersect rule for reap-orphans\n" + realPayload
	out := SanitizeSummary(in)
	if strings.Contains(out, "hook_event_name") {
		t.Fatal("payload survived sanitization — the delegation path is still bypassable")
	}
	if !strings.Contains(out, "Chose the intersect rule") {
		t.Fatal("a real decision was stripped")
	}
	if strings.TrimSpace(SanitizeSummary(realPayload)) != "" {
		t.Fatal("an all-payload summary must sanitize to empty so no writer is invoked")
	}
}
