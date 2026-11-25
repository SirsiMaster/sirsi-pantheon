package router

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestRegistryWakeCoverage asserts the checked-in agent registry actually
// carries wake metadata for every agent that needs it.
//
// This guard exists because the fleet ran UNWAKEABLE for weeks and nothing
// caught it. The repo root sat on a squat branch whose HEAD (287dc7ea) had
// bulk-adopted a 2026-06-08-era registry from before the wake schema existed:
// 17 agents, 16 with an empty wake block. Every conduit pass reported ~11
// stranded agents, and the only signal was that stranded count. main itself was
// never broken — 287dc7ea was never an ancestor of it — so no test, lint or CI
// gate ever looked at the thing that was actually wrong.
//
// Reading the file from disk rather than a fixture is the entire point: the
// value being protected is the committed registry, not a copy of it.
func TestRegistryWakeCoverage(t *testing.T) {
	const path = "../../.agents/idea-router/agents.json"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read agent registry %s: %v", path, err)
	}

	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("parse agent registry %s: %v", path, err)
	}
	if len(reg.Agents) == 0 {
		t.Fatalf("agent registry %s has no agents — a truncated or pre-schema copy", path)
	}

	// NO PREFIX EXEMPTION. This guard used to skip every id starting with
	// "codex-", on the assumption that codex lanes are wake-dead. That
	// assumption is false per-id and it hid a real defect for weeks:
	// codex-pantheon had a LIVE ai.sirsi.router.wake.codex-pantheon LaunchAgent
	// (pid 99109, status 0) while its registry block was {}, so doctor stamped
	// its items wake-unavailable and the guard — exempting it by name — could
	// never notice. Seven sibling codex ids genuinely are wake-dead. The prefix
	// could not tell them apart, because wake capability is a property of an
	// AGENT, not of its name (router items 20260729-045819, 20260729-141541).
	//
	// So the guard now tests a property of the DATA: every agent must state its
	// wake posture explicitly. A real mechanism, or "none" — which is a claim
	// the registry makes and can be checked — but never an empty block, which
	// is merely an absence and is indistinguishable from an oversight.
	//
	// No launch_agent_label assertion here: WakeConfig.LaunchAgentLabel now round-
	// trips correctly, so the drift check (registrydrift.go) catches a missing
	// label generically. WakeLaunchAgentLabel() still derives the operational label
	// from the agent id by convention; the stored field is preserved for fidelity.
	var missing []string
	for id, cfg := range reg.Agents {
		if cfg.Wake.Mechanism == "" {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("%d of %d agents do not state a wake mechanism: %v\n"+
			"Horus cannot wake these, so work routed to them strands silently.\n"+
			"An agent that genuinely cannot be woken must say so with mechanism:none — "+
			"an empty wake block is an absence, not a decision.\n"+
			"If the registry looks reverted, check whether this worktree is on a branch "+
			"whose HEAD carries a pre-wake copy (see 287dc7ea) rather than origin/main's.",
			len(missing), len(reg.Agents), missing)
	}
}

// TestCodexInferenceNeverUsesCLISpawnWake was retired 2026-09-10 on the owner's
// decision ("retire the guard: wake it like the others"): codex-inference now
// wakes under launchd through the spool relay like every other codex lane.
// The app-automation-only rule it encoded no longer holds.

func TestRegistryConsumerCoverage(t *testing.T) {
	const path = "../../.agents/idea-router/agents.json"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read agent registry %s: %v", path, err)
	}

	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("parse agent registry %s: %v", path, err)
	}

	var missing []string
	var invalid []string
	for id, cfg := range reg.Agents {
		// Only EXECUTABLE agents can declare a consumer. `owner` and `user` are
		// type=human and `horus` is type=service — they are message RECIPIENTS
		// with no command, so demanding a spawn invocation from them would be
		// demanding the impossible, and the only way to satisfy it would be to
		// write a fake one. A registry entry that cannot run anything cannot
		// drain an inbox by spawning, and must not be credited as if it could.
		if len(cfg.Command) == 0 {
			continue
		}
		// A declared attended lane is served by its attended session (owner 2026-10-16: secondary consumers
		// never override the attended lane; the wake loop holds dispatch for it), so it need not declare a
		// spawn consumer. A consumer it does declare is still validated below.
		if cfg.Attended && len(cfg.Consumer.Command) == 0 && cfg.Consumer.Mode == "" {
			continue
		}
		switch cfg.Consumer.Mode {
		case "", ConsumerModeCommand:
			if len(cfg.Consumer.Command) == 0 {
				missing = append(missing, id)
				continue
			}
			joined := ""
			for _, arg := range cfg.Consumer.Command {
				joined += arg + " "
			}
			joined += cfg.Consumer.Prompt
			if !containsEither(joined, id, consumerAgentPlaceholder) {
				invalid = append(invalid, id+": command consumer does not carry an inbox identity")
			}
		case ConsumerModeResident, "external/resident":
			if len(cfg.Consumer.Command) > 0 {
				invalid = append(invalid, id+": resident consumer declares a spawn command")
			}
			if len(cfg.Consumer.HealthCheck) == 0 {
				invalid = append(invalid, id+": resident consumer lacks health_check")
			}
		default:
			invalid = append(invalid, id+": unsupported consumer.mode "+cfg.Consumer.Mode)
		}
	}
	sort.Strings(missing)
	sort.Strings(invalid)

	if len(missing) > 0 {
		t.Errorf("%d of %d agents do not declare an inbox consumer: %v",
			len(missing), len(reg.Agents), missing)
	}
	if len(invalid) > 0 {
		t.Errorf("invalid consumer declarations: %v", invalid)
	}
}

func containsEither(s, a, b string) bool {
	return strings.Contains(s, a) || strings.Contains(s, b)
}

// The consumer prompt IS the contract handed to a spawned agent. It previously
// ended "If the inbox is empty, exit immediately and do nothing else", which
// contradicts the owner's standing permanent-loop instruction: an empty inbox
// alone is never completion — reconcile the task registry and work the ledger
// before stopping. A prompt that tells the agent to quit early makes the whole
// dispatch mechanism finish less work than it should.
func TestConsumerPromptCarriesThePermanentLoopContract(t *testing.T) {
	// Read the COMMITTED registry from disk, same as the sibling guards: the
	// value being protected is the file that ships, not a fixture of it.
	raw, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatalf("read agent registry: %v", err)
	}
	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatalf("parse agent registry: %v", err)
	}
	required := []string{
		"ledger",         // work the ledger, not just the inbox
		"task",           // reconcile the task registry
		"pull again",     // it is a loop
		"NOT COMPLETION", // the explicit correction of the old behavior
		"EVIDENCE",       // never a bare ack
	}
	// ADR-079 D3: a lane with a live interactive session gets a one-pass headless
	// worker (explicit onePassLanes list). That session owns the ledger, so the
	// worker finishes the items it was woken for and exits instead of looping beside
	// it. A one-pass prompt must be complete and on a listed lane; every other prompt
	// keeps the loop contract.
	for id := range onePassLanes {
		if _, ok := reg.Agents[id]; !ok {
			t.Errorf("%s: listed one-pass lane is missing from the registry", id)
		}
	}
	for id, cfg := range reg.Agents {
		p := cfg.Consumer.Prompt
		if p == "" {
			continue // resident consumers carry no prompt
		}
		if strings.Contains(p, "exit immediately and do nothing else") {
			t.Errorf("%s: prompt still tells the agent to quit on an empty inbox", id)
		}
		if msg := onePassViolation(id, p); msg != "" {
			t.Errorf("%s: %s", id, msg)
		}
		if onePassLanes[id] {
			continue
		}
		for _, want := range required {
			if !strings.Contains(p, want) {
				t.Errorf("%s: prompt omits %q — the permanent-loop contract is incomplete", id, want)
			}
		}
	}
}

// onePassLanes is the EXPLICIT list of lanes with a live interactive session whose
// headless worker is a one-pass job (ADR-079 D3; owner 2026-11-24). Adding a lane
// takes a deliberate edit here, so the contract cannot spread by accident: every
// lane not listed keeps the loop contract and must not carry ONE-PASS.
var onePassLanes = map[string]bool{
	"ra":                    true,
	"codex-finalwishes":     true,
	"claude-finalwishes-m1": true,
	"mercury":               true,
}

// onePassViolation returns why prompt p breaks the list above for lane id, or "".
func onePassViolation(id, p string) string {
	if !onePassLanes[id] {
		if strings.Contains(p, "ONE-PASS") {
			return "one-pass prompt on a lane not in onePassLanes — add it to the list deliberately or restore the loop contract"
		}
		return ""
	}
	for _, want := range []string{"ONE-PASS", "EXIT", "EVIDENCE"} {
		if !strings.Contains(p, want) {
			return "one-pass lane's worker prompt omits " + want + " — the ADR-079 D3 contract is incomplete"
		}
	}
	if strings.Contains(p, "pull again") {
		return "one-pass lane's worker prompt still loops (pull again) beside the interactive session"
	}
	return ""
}

// Negative controls, both directions: a listed lane losing the contract fails, an
// unlisted lane gaining it fails, and the right shape on each side passes.
func TestOnePassGuardBothDirections(t *testing.T) {
	onePass := "You are {{agent}}. ONE-PASS job: close each WITH EVIDENCE, then EXIT 0."
	loop := "You are {{agent}}. LOOP: pull again. NOT COMPLETION. EVIDENCE."
	if onePassViolation("codex-finalwishes", loop) == "" {
		t.Error("listed lane that lost the one-pass contract was not rejected")
	}
	if onePassViolation("codex-finalwishes", "ONE-PASS EVIDENCE but never leaves") == "" {
		t.Error("listed lane with an incomplete one-pass prompt was not rejected")
	}
	if onePassViolation("claude-home", onePass) == "" {
		t.Error("unlisted lane that gained the one-pass contract was not rejected")
	}
	if msg := onePassViolation("mercury", onePass); msg != "" {
		t.Errorf("complete one-pass prompt on a listed lane rejected: %s", msg)
	}
	if msg := onePassViolation("claude-home", loop); msg != "" {
		t.Errorf("loop prompt on an unlisted lane rejected: %s", msg)
	}
}

// TestRegistryConsumersAvoidSymlinkedRelayRoot is the regression guard for the
// known failure "codex-symlinked-writable-root": the codex executor rejects a
// writable root whose last component is a symlink, and ~/.sirsi/relay is one (to
// /var/sirsipantheon/relay). A consumer that lists it as a writable root or
// --add-dir fails every command, makes no progress, and quarantines. The router URL
// (SIRSI_ROUTER_URL=spool://...) legitimately names the path and is exempt.
func TestRegistryConsumersAvoidSymlinkedRelayRoot(t *testing.T) {
	raw, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}
	for id, cfg := range reg.Agents {
		rc := cfg.Consumer.Command
		for _, arg := range rc {
			if strings.HasPrefix(arg, "SIRSI_ROUTER_URL=") {
				continue
			}
			if strings.Contains(arg, "/.sirsi/relay") {
				t.Errorf("%s: consumer argument %q names the symlinked ~/.sirsi/relay; list /var/sirsipantheon/relay instead (known failure codex-symlinked-writable-root)", id, arg)
			}
		}
	}
}
