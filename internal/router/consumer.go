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
	"fmt"
	"os"
	"os/exec"
	"strings"
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

// ConsumerConfig declares how an agent's router inbox is actually DRAINED.
//
// This is deliberately separate from AgentConfig.Command. Command is the
// agent's launch command and its own doc says the work prompt is appended "by
// the executor" — so Command alone is an incomplete invocation, which is
// precisely why spawning it drained nothing. A consumer is a COMPLETE, runnable
// invocation that pulls and works the inbox on its own.
type ConsumerConfig struct {
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
}

// ResolvedConsumer is a validated, ready-to-dispatch draining invocation.
type ResolvedConsumer struct {
	Argv []string
	Env  []string
	Cwd  string
}

// ResolveConsumer validates cfg's declared consumer and substitutes the router
// contract. It returns (nil, reason) whenever dispatching would not be an
// honest consume — every reason is phrased for an operator reading a log or a
// doctor surface, because "this lane has no consumer" is exactly the fact the
// watch-only loop used to hide.
func ResolveConsumer(cfg AgentConfig, routerRoot string) (*ResolvedConsumer, string) {
	c := cfg.Consumer
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

	return &ResolvedConsumer{Argv: argv, Env: env, Cwd: cfg.Cwd}, ""
}

// HasConsumerCapability reports whether cfg declares a dispatchable consumer.
// This is the predicate the armed calculation consults: a lane is only armed by
// something that can actually drain it.
func HasConsumerCapability(cfg AgentConfig, routerRoot string) bool {
	rc, _ := ResolveConsumer(cfg, routerRoot)
	return rc != nil
}
