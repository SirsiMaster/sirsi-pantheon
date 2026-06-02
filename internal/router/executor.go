// Package router — executor.go
//
// Pluggable agent executor for Router v3. Resolves an agent ID from the
// registry, launches the agent's command with the work context, and verifies
// the agent wrote back to the router before marking dispatch successful.
// Ra owns execution; Thoth preserves continuity; Ma'at validates governance.
package router

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Executor launches registered agents and verifies writeback.
type Executor struct {
	registry       *Registry
	router         *Router
	workQueue      *WorkQueue
	out            io.Writer
	timeout        time.Duration
	authProbe      AuthProbeFunc
	wakeHTTPClient wakeHTTPClient
}

// DefaultExecutorTimeout is the per-dispatch wall-clock cap for an agent CLI
// run. Claude/Codex sessions that read context and write router artifacts
// routinely take 5–20 minutes; the previous 5-minute cap killed legitimate
// in-progress work with `signal: killed` before writeback. Override with
// SIRSI_ROUTER_EXECUTOR_TIMEOUT (a Go duration, e.g. "45m").
const DefaultExecutorTimeout = 30 * time.Minute

// NewExecutor creates an executor backed by the given registry, router, and work queue.
func NewExecutor(reg *Registry, r *Router, wq *WorkQueue, out io.Writer) *Executor {
	return &Executor{
		registry:  reg,
		router:    r,
		workQueue: wq,
		out:       out,
		timeout:   resolveExecutorTimeout(),
		authProbe: DefaultAuthProbe,
	}
}

func resolveExecutorTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("SIRSI_ROUTER_EXECUTOR_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			return d
		}
	}
	return DefaultExecutorTimeout
}

// SetTimeout overrides the per-dispatch timeout (for testing).
func (e *Executor) SetTimeout(d time.Duration) {
	if d > 0 {
		e.timeout = d
	}
}

// SetAuthProbe overrides the auth probe function (for testing).
func (e *Executor) SetAuthProbe(fn AuthProbeFunc) {
	e.authProbe = fn
}

// Dispatch launches the target agent for a work item and verifies writeback.
// Returns nil if the agent completed and wrote back successfully.
func (e *Executor) Dispatch(ctx context.Context, item *WorkItem) error {
	// Resolve agent from registry
	cfg, err := e.registry.Lookup(item.TargetAgentID)
	if err != nil {
		e.workQueue.UpdateStatus(item.ID, StatusFailed, err.Error())
		return err
	}

	if vErr := cfg.Validate(); vErr != nil {
		e.workQueue.UpdateStatus(item.ID, StatusFailed, vErr.Error())
		return vErr
	}

	// Pre-dispatch auth check: fail fast for direct agent CLI launches.
	if e.authProbe != nil && len(cfg.Command) > 0 && shouldProbeCommandAuth(cfg.Command[0], cfg.Type) {
		cliPath, lookErr := exec.LookPath(cfg.Command[0])
		if lookErr != nil {
			reason := fmt.Sprintf("%s CLI not found in PATH — install before dispatching", cfg.Command[0])
			e.workQueue.UpdateStatus(item.ID, StatusBlocked, reason)
			_ = e.workQueue.Save()
			fmt.Fprintf(e.out, "  Blocked: %s\n", reason)
			return fmt.Errorf("dispatch to %s blocked: %s", item.TargetAgentID, reason)
		}
		authOK, needsLogin, detail := e.authProbe(cliPath, cfg.Type)
		if !authOK {
			var reason string
			if needsLogin {
				reason = fmt.Sprintf("%s CLI not authenticated — run '%s' then /login to unblock dispatch", cfg.Type, cfg.Type)
			} else {
				reason = fmt.Sprintf("%s CLI auth check failed: %s", cfg.Type, detail)
			}
			e.workQueue.UpdateStatus(item.ID, StatusBlocked, reason)
			_ = e.workQueue.Save()
			fmt.Fprintf(e.out, "  Blocked: %s\n", reason)
			return fmt.Errorf("dispatch to %s blocked: %s", item.TargetAgentID, reason)
		}
	}

	// Build the work prompt
	prompt := buildWorkPrompt(item, cfg)

	// Record pre-dispatch state for writeback detection
	preState, _ := e.router.ReadState()
	preLastRead := ""
	if preState != nil {
		switch cfg.Type {
		case "claude":
			preLastRead = preState.LastClaudeRead
		case "codex":
			preLastRead = preState.LastCodexRead
		}
	}

	// Mark dispatched
	e.workQueue.UpdateStatus(item.ID, StatusDispatched, "")
	_ = e.workQueue.Save()
	fmt.Fprintf(e.out, "  Dispatching to %s (%s)...\n", item.TargetAgentID, cfg.Type)

	// Wake with timeout
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	err = e.wake(execCtx, item, cfg, prompt)
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// Record attempt
	stderrStr := errString(err)
	if len(stderrStr) > 4000 {
		stderrStr = stderrStr[:4000] + "...(truncated)"
	}
	e.workQueue.RecordAttempt(item.ID, exitCode, errString(err), stderrStr)

	if cfg.WakeMechanism() != WakeCLISpawn {
		if err != nil {
			reason := fmt.Sprintf("wake failed: %s", errString(err))
			e.workQueue.UpdateStatus(item.ID, StatusFailed, reason)
			_ = e.workQueue.Save()
			fmt.Fprintf(e.out, "  Failed: %s\n", reason)
			return fmt.Errorf("wake %s failed: %w", item.TargetAgentID, err)
		}
		e.workQueue.UpdateStatus(item.ID, StatusDispatched, "")
		_ = e.workQueue.Save()
		fmt.Fprintf(e.out, "  Wake sent via %s\n", cfg.WakeMechanism())
		return nil
	}

	// Check for writeback: did the agent update state.json?
	postState, _ := e.router.ReadState()
	writebackDetected := false
	if postState != nil {
		switch cfg.Type {
		case "claude":
			writebackDetected = postState.LastClaudeRead != preLastRead
		case "codex":
			writebackDetected = postState.LastCodexRead != preLastRead
		}
	}

	// Determine final status
	if err != nil {
		reason := fmt.Sprintf("agent exited with code %d: %s", exitCode, errString(err))
		e.workQueue.UpdateStatus(item.ID, StatusFailed, reason)
		_ = e.workQueue.Save()
		fmt.Fprintf(e.out, "  Failed: %s\n", reason)
		return fmt.Errorf("dispatch to %s failed: %w", item.TargetAgentID, err)
	}

	if writebackDetected {
		e.workQueue.UpdateStatus(item.ID, StatusCompleted, "")
		_ = e.workQueue.Save()
		fmt.Fprintf(e.out, "  Completed: %s wrote back to router\n", item.TargetAgentID)
		return nil
	}

	// Agent exited clean but no writeback detected
	reason := "agent exited successfully but no router writeback detected"
	e.workQueue.UpdateStatus(item.ID, StatusFailed, reason)
	_ = e.workQueue.Save()
	fmt.Fprintf(e.out, "  Warning: %s\n", reason)
	return fmt.Errorf("%s", reason)
}

func (e *Executor) wakeCLI(ctx context.Context, _ *WorkItem, cfg *AgentConfig, prompt string) error {
	args := make([]string, len(cfg.Command)-1)
	copy(args, cfg.Command[1:])
	args = append(args, prompt)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, cfg.Command[0], args...)
	cmd.Dir = cfg.Cwd
	cmd.Stdout = e.out
	cmd.Stderr = &stderr

	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

func shouldProbeCommandAuth(command, agentType string) bool {
	return filepath.Base(command) == agentType
}

func buildWorkPrompt(item *WorkItem, cfg *AgentConfig) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("You are %s (agent_id: %s), processing work from the Idea Router.\n\n", cfg.Type, cfg.ID))
	sb.WriteString(fmt.Sprintf("Repository: %s\n", cfg.Cwd))
	sb.WriteString(fmt.Sprintf("Work item: %s\n", item.DocID))
	if item.Topic != "" {
		sb.WriteString(fmt.Sprintf("Topic: %s\n", item.Topic))
	}
	if item.Goal != "" {
		sb.WriteString(fmt.Sprintf("/goal: %s\n", item.Goal))
	}
	sb.WriteString("\nRead the following files in order:\n")
	sb.WriteString("1. .agents/idea-router/state.json\n")
	sb.WriteString("2. .agents/idea-router/agents.json\n")
	sb.WriteString("3. The addressed work item document\n")
	sb.WriteString("4. .agents/idea-router/DESIGN.md (for protocol rules)\n\n")
	sb.WriteString("Then act:\n")
	sb.WriteString("- Implement the work described in the router item.\n")
	sb.WriteString("- Keep work scoped to this repository only.\n")
	sb.WriteString("- Continue until the /goal is met, blocked by safety/user approval, or impossible.\n")
	sb.WriteString("- Run tests and record evidence.\n")
	sb.WriteString("- Write result to .agents/idea-router/reviews/ or decisions/.\n")
	sb.WriteString("- Update state.json: your last_read timestamp, pending items for the next agent.\n")
	sb.WriteString("- Do not leave vague next steps when you can complete the work yourself.\n")
	return sb.String()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
