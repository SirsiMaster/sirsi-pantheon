// Package router — consumer.go
//
// The inbox-consumer capability (codex-pantheon review of #389).
//
// A wake loop that spawns "something non-interactive" is not a consumer. The
// first cut of #389 inferred consumption from a `--print` flag, which proves CLI
// mode and nothing else: `claude --print` with no prompt drains no inbox, and a
// bare `codex` REPL passed the gate purely because its type was not "claude".
// Non-interactive and drains-this-inbox are different properties.
//
// So consumption is DECLARED, never inferred — the same stance
// ExplicitWakeMechanism takes for wake (constraint 4: a bare Command array has
// declared no intent and must not be acted on). An agent without a consumer
// block has no consumer, and every surface says so out loud rather than
// guessing.
//
// The declaration must also carry the ROUTER CONTRACT: which inbox to drain and
// how to drain it. A spawned process that was never told its agent id cannot
// consume the queue whose depth triggered it, so a declaration that does not
// carry the identity is rejected at resolve time rather than dispatched and
// hoped over.
package router

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Consumer placeholders substituted into the declared argv/prompt.
const (
	consumerAgentPlaceholder = "{{agent}}"
	consumerRootPlaceholder  = "{{router_root}}"
)

// Env vars every dispatched consumer receives. A consumer that prefers reading
// its identity from the environment (rather than argv) still gets an explicit,
// unambiguous contract.
const (
	EnvConsumerAgent = "SIRSI_ROUTER_AGENT"
	EnvConsumerRoot  = "SIRSI_ROUTER_ROOT"
)

// EnvGOMAXPROCS is the standard Go env var consumer.go caps by default on
// every SPAWNED (command-mode) consumer — see consumerGOMAXPROCS.
const EnvGOMAXPROCS = "GOMAXPROCS"

// consumerGOMAXPROCS is the default GOMAXPROCS cap applied to a spawned
// consumer's environment (R7/G6). Measured 2026-08-06: two workers each
// fanning `go test`/`go build` out to GOMAXPROCS=NCPU on an 18-core host
// produced 10 concurrent compile/test procs and load average 36 — 2x
// oversubscription from just THREE live lanes. `go build`/`go test` both
// default their own `-p` parallelism to GOMAXPROCS, so setting it here alone
// bounds compiler/test fan-out without every consumer hand-writing `-p 2
// -parallel 2`. Configurable via SetConsumerGOMAXPROCS.
var consumerGOMAXPROCS = 4

// SetConsumerGOMAXPROCS overrides the default GOMAXPROCS cap for spawned
// consumers. n <= 0 is ignored (keeps the previous value).
func SetConsumerGOMAXPROCS(n int) {
	if n > 0 {
		consumerGOMAXPROCS = n
	}
}

// capGOMAXPROCS strips any inherited GOMAXPROCS from env, then appends the
// authoritative value: the operator's explicit override if present, otherwise
// the default cap. Stripping first prevents os.Environ() from racing the
// operator value when both land in the same slice.
func capGOMAXPROCS(env []string, cfgEnv map[string]string) []string {
	prefix := EnvGOMAXPROCS + "="
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	if v, explicit := cfgEnv[EnvGOMAXPROCS]; explicit {
		return append(out, EnvGOMAXPROCS+"="+v)
	}
	return append(out, EnvGOMAXPROCS+"="+strconv.Itoa(consumerGOMAXPROCS))
}

const (
	ConsumerModeCommand        = "command"
	ConsumerModeResident       = "resident"
	consumerHealthCheckTimeout = 5 * time.Second
)

// DeclaresResidentConsumer reports whether cfg's declared consumer is a
// RESIDENT one — an external worker that is health-checked rather than spawned.
//
// This is the authorization boundary for publishing consumer capability from
// outside the wake loop (`thread register --consumer-capable`). It reads the
// DECLARATION only: unlike ResolveConsumer it does not run the health check,
// because the caller publishing the capability IS the live worker and its own
// heartbeat is stronger evidence than a health probe (A33/PR #389).
//
// It exists so "resident" has exactly one definition. Re-deriving it in the CLI
// would be a second answer to the same question, which is how the two drift.
func (cfg AgentConfig) DeclaresResidentConsumer() bool {
	switch strings.TrimSpace(cfg.Consumer.Mode) {
	case ConsumerModeResident, "external/resident":
		return true
	}
	return false
}

// ConsumerConfig declares how an agent's router inbox is actually DRAINED.
//
// This is deliberately separate from AgentConfig.Command. Command is the
// agent's launch command and its own doc says the work prompt is appended "by
// the executor" — so Command alone is an incomplete invocation, which is
// precisely why spawning it drained nothing. A consumer is a COMPLETE, runnable
// invocation that pulls and works the inbox on its own.
type ConsumerConfig struct {
	// Mode selects the consumer contract. Empty means "command" for backwards
	// compatibility with the first #389 declaration. "resident" means an
	// already-running worker owns the inbox; the wake loop may mark the lane
	// consumer-capable after a health check but must never spawn a duplicate.
	Mode string `json:"mode,omitempty"`

	// Command is the complete argv of the draining invocation. Placeholders
	// {{agent}} and {{router_root}} are substituted at dispatch.
	Command []string `json:"command,omitempty"`

	// Prompt, when set, is appended to Command as a final argument after
	// placeholder substitution. This is the router contract in the common case:
	// the instruction that tells an agent CLI to pull and work its inbox.
	Prompt string `json:"prompt,omitempty"`

	// Interactive marks a declaration that would open a REPL. Declaring one is
	// an error rather than a silent refusal: it means the operator believed they
	// had configured a consumer when they had configured a session.
	Interactive bool `json:"interactive,omitempty"`

	// HealthCheck is required for resident consumers. It proves the external
	// worker surface exists without spawning another copy of it.
	HealthCheck []string `json:"health_check,omitempty"`
}

// ResolvedConsumer is a validated, ready-to-dispatch draining invocation.
type ResolvedConsumer struct {
	Argv        []string
	Env         []string
	Cwd         string
	Resident    bool
	HealthCheck []string
}

// ResolveConsumer validates cfg's declared consumer and substitutes the router
// contract. It returns (nil, reason) whenever dispatching would not be an
// honest consume — every reason is phrased for an operator reading a log or a
// doctor surface, because "this lane has no consumer" is exactly the fact the
// watch-only loop used to hide.
func ResolveConsumer(cfg AgentConfig, routerRoot string) (*ResolvedConsumer, string) {
	c := cfg.Consumer
	mode := strings.TrimSpace(c.Mode)
	if mode == "" {
		mode = ConsumerModeCommand
	}
	switch mode {
	case ConsumerModeCommand:
	case ConsumerModeResident, "external/resident":
		return resolveResidentConsumer(cfg, routerRoot)
	default:
		return nil, fmt.Sprintf("unsupported consumer.mode %q", c.Mode)
	}

	if len(c.Command) == 0 {
		return nil, "no consumer declared — set consumer.command to the invocation that pulls and works this inbox " +
			"(agent.command is the launch command, not a complete draining invocation)"
	}
	if c.Interactive {
		return nil, "consumer.interactive is set — a REPL is a session, not an inbox consumer"
	}

	subst := func(s string) string {
		s = strings.ReplaceAll(s, consumerAgentPlaceholder, cfg.ID)
		return strings.ReplaceAll(s, consumerRootPlaceholder, routerRoot)
	}

	argv := make([]string, 0, len(c.Command)+1)
	for _, a := range c.Command {
		argv = append(argv, subst(a))
	}
	if p := strings.TrimSpace(c.Prompt); p != "" {
		argv = append(argv, subst(p))
	}

	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, fmt.Sprintf("consumer command %q not found in PATH", argv[0])
	}

	// The identity contract, enforced rather than assumed. A declaration that
	// never names the agent cannot address the right inbox — that was finding 2
	// of the #389 review, and the fix is to make it unrepresentable rather than
	// to document it. Env alone does not satisfy this: a CLI that takes its
	// target from argv would silently drain whatever its own default is.
	if !strings.Contains(strings.Join(argv, " "), cfg.ID) {
		return nil, fmt.Sprintf(
			"consumer declaration never references agent id %q — it carries no inbox contract "+
				"and would drain some other (or no) queue; use %s in consumer.command or consumer.prompt",
			cfg.ID, consumerAgentPlaceholder)
	}

	env := append(os.Environ(),
		EnvConsumerAgent+"="+cfg.ID,
		EnvConsumerRoot+"="+routerRoot,
	)
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	env = capGOMAXPROCS(env, cfg.Env)

	// A declared cwd that does not exist is WORSE than no consumer at all.
	// RunWakeLoop would persist ConsumerCapable=true, every cmd.Start would fail
	// on the invalid directory, and WakePass would then suppress rescue because
	// the worker reads armed — an inbox stranded behind a consumer that can
	// never start (codex-pantheon, PR #389). Refusing here keeps the loop
	// watch-only and UNARMED, which is the honest state.
	if cfg.Cwd != "" {
		info, err := os.Stat(cfg.Cwd)
		if err != nil {
			return nil, fmt.Sprintf("consumer cwd %q is not usable: %v", cfg.Cwd, err)
		}
		if !info.IsDir() {
			return nil, fmt.Sprintf("consumer cwd %q is not a directory", cfg.Cwd)
		}
	}

	return &ResolvedConsumer{Argv: argv, Env: env, Cwd: cfg.Cwd}, ""
}

// residentConsumerThreadFn resolves whether the resident agent has PUBLISHED a
// live, consumer-capable thread of its own. Indirected for tests.
var residentConsumerThreadFn = residentConsumerThread

// residentConsumerThread reports whether agentID owns a thread that is active,
// heartbeat-fresh, and self-declared ConsumerCapable — i.e. the resident is
// publishing its own capability rather than having it inferred on its behalf.
func residentConsumerThread(routerRoot, agentID string, now time.Time) (bool, string) {
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil || reg == nil {
		// Fail CLOSED. An unreadable registry is not evidence of a live
		// consumer, and crediting the lane here would recreate exactly the
		// false-green this function exists to remove.
		return false, "thread registry unreadable — cannot confirm a published consumer"
	}
	sawThread := false
	for _, t := range reg.Threads {
		if t == nil || t.AgentID != agentID {
			continue
		}
		sawThread = true
		if t.Status != ThreadStatusActive {
			continue
		}
		if !t.ConsumerCapable {
			continue
		}
		if now.Sub(t.LastSeenAt) > DefaultThreadStaleAfter {
			continue
		}
		return true, ""
	}
	if !sawThread {
		return false, "resident published no thread — it must register a consumer-capable thread and heartbeat it"
	}
	return false, "resident's thread is not an active, heartbeat-fresh, consumer-capable record — it stopped publishing, so it is no longer credited"
}

// resolveResidentConsumer credits a resident lane ONLY on evidence the resident
// itself published (H6, follow-up to PR #389).
//
// What this replaces: the resolver used to run `consumer.health_check` and treat
// a zero exit as proof of a live consumer. gemma-pantheon's declared check was
// `sirsi-gemma-worker.sh --version` — that proves a binary can print a string.
// It passes identically whether the worker is draining an inbox or was stopped
// an hour ago, so the wake watcher was BORROWING capability on the resident's
// behalf and could never withdraw the credit. A probe that cannot go stale is
// not liveness (A35: the check's scope was "a file exists and exits 0"; its
// claim was "a consumer is alive").
//
// What replaces it: the resident must publish its OWN thread — active,
// heartbeat-fresh, ConsumerCapable — exactly as every other consumer surface
// does. Capability now decays on its own: a resident that stops heartbeating
// stops being credited within DefaultThreadStaleAfter, with no probe to fool.
//
// health_check is retained as an OPTIONAL, secondary signal only. It may
// disqualify (a declared check that fails is still a real negative), but it can
// never by itself qualify a lane.
func resolveResidentConsumer(cfg AgentConfig, routerRoot string) (*ResolvedConsumer, string) {
	c := cfg.Consumer
	if len(c.Command) > 0 {
		return nil, "consumer.mode resident must not declare consumer.command — a resident publishes its own thread, it is not spawned"
	}
	if c.Interactive {
		return nil, "consumer.interactive is set — a REPL is a session, not an inbox consumer"
	}

	// The published thread is the ONLY thing that can qualify a resident.
	if ok, why := residentConsumerThreadFn(routerRoot, cfg.ID, time.Now()); !ok {
		return nil, "resident consumer not credited: " + why
	}

	subst := func(s string) string {
		s = strings.ReplaceAll(s, consumerAgentPlaceholder, cfg.ID)
		return strings.ReplaceAll(s, consumerRootPlaceholder, routerRoot)
	}
	health := make([]string, 0, len(c.HealthCheck))
	for _, a := range c.HealthCheck {
		health = append(health, subst(a))
	}
	// Optional and disqualifying-only: if a check is declared and fails, that is
	// a real negative worth honoring. Its ABSENCE is no longer a refusal, and
	// its SUCCESS is no longer a qualification.
	if len(health) > 0 {
		if _, err := exec.LookPath(health[0]); err != nil {
			return nil, fmt.Sprintf("resident consumer health_check command %q not found in PATH", health[0])
		}
		if err := runConsumerHealthCheck(health); err != nil {
			return nil, fmt.Sprintf("resident consumer health_check failed: %v", err)
		}
	}

	env := append(os.Environ(),
		EnvConsumerAgent+"="+cfg.ID,
		EnvConsumerRoot+"="+routerRoot,
	)
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	return &ResolvedConsumer{Env: env, Cwd: cfg.Cwd, Resident: true, HealthCheck: health}, ""
}

func runConsumerHealthCheck(argv []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), consumerHealthCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %s", consumerHealthCheckTimeout)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}

// HasConsumerCapability reports whether cfg declares a dispatchable consumer.
// This is the predicate the armed calculation consults: a lane is only armed by
// something that can actually drain it.
func HasConsumerCapability(cfg AgentConfig, routerRoot string) bool {
	rc, _ := ResolveConsumer(cfg, routerRoot)
	return rc != nil
}
