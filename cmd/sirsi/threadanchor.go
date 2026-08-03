package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const maxAnchorAncestryHops = 16

type anchorProcess struct {
	parentPID int
	command   string
}

type anchorProcessLookup func(pid int) (anchorProcess, error)

type anchorChild struct {
	pid     int
	process anchorProcess
}

type anchorChildLookup func(parentPID int) ([]anchorChild, error)

// resolveAnchorPID locates the durable runtime that owns the current session.
// A fixed parent/grandparent depth is forbidden: desktop apps and tool runners
// insert transient per-prompt helpers at different depths, which creates fresh
// but doomed CTR records. Known interactive surfaces are resolved by executable
// identity; other resident surfaces must provide an explicit --anchor-pid.
func resolveAnchorPID(surface string) (int, error) {
	anchor, err := resolveDurableAnchor(os.Getppid(), surface, lookupAnchorProcess)
	if err != nil {
		return 0, err
	}
	return refineDesktopAnchor(anchor, surface, os.Getenv("CODEX_INTERNAL_ORIGINATOR_OVERRIDE"), lookupAnchorChildren)
}

var lookupAnchorProcess anchorProcessLookup = func(pid int) (anchorProcess, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "ppid=,comm=").Output()
	if err != nil {
		return anchorProcess{}, fmt.Errorf("inspect pid %d: %w", pid, err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) < 2 {
		return anchorProcess{}, fmt.Errorf("inspect pid %d: missing parent or command", pid)
	}
	parent, err := strconv.Atoi(parts[0])
	if err != nil {
		return anchorProcess{}, fmt.Errorf("inspect pid %d parent %q: %w", pid, parts[0], err)
	}
	return anchorProcess{parentPID: parent, command: strings.Join(parts[1:], " ")}, nil
}

func lookupAnchorChildren(parentPID int) ([]anchorChild, error) {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		// pgrep exits 1 when there are no matches. Treat that as an empty set;
		// every other failure means we cannot prove the desktop binding.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect children of pid %d: %w", parentPID, err)
	}
	var children []anchorChild
	for _, field := range strings.Fields(string(out)) {
		pid, parseErr := strconv.Atoi(field)
		if parseErr != nil || pid <= 1 {
			return nil, fmt.Errorf("inspect children of pid %d: invalid child pid %q", parentPID, field)
		}
		proc, lookupErr := lookupAnchorProcess(pid)
		if lookupErr != nil {
			// A child may exit between pgrep and ps. Ignore that normal race; a
			// durable host will still be present and uniquely provable below.
			continue
		}
		children = append(children, anchorChild{pid: pid, process: proc})
	}
	return children, nil
}

// refineDesktopAnchor handles runtimes whose task host is not an ancestor of
// tool commands. Codex Desktop is the concrete case: commands are children of
// the application-wide `codex` broker, while the task lifetime belongs to its
// `codex-code-mode-host` child. Anchoring to the broker keeps closed tasks alive.
// Guessing among multiple hosts would bind one task to another, so ambiguity is
// a hard error and callers can provide --anchor-pid explicitly.
func refineDesktopAnchor(anchor int, surface, originator string, children anchorChildLookup) (int, error) {
	if !strings.EqualFold(strings.TrimSpace(surface), "codex") ||
		!strings.EqualFold(strings.TrimSpace(originator), "Codex Desktop") {
		return anchor, nil
	}
	proc, err := lookupAnchorProcess(anchor)
	if err != nil {
		return 0, err
	}
	if executableName(proc.command) == "codex-code-mode-host" {
		return anchor, nil
	}
	if executableName(proc.command) != "codex" {
		return 0, fmt.Errorf("Codex Desktop ancestry resolved unexpected broker %q", proc.command)
	}
	if children == nil {
		return 0, fmt.Errorf("Codex Desktop child lookup is nil")
	}
	candidates, err := children(anchor)
	if err != nil {
		return 0, err
	}
	var hosts []int
	for _, child := range candidates {
		if executableName(child.process.command) == "codex-code-mode-host" {
			hosts = append(hosts, child.pid)
		}
	}
	if len(hosts) != 1 {
		return 0, fmt.Errorf("Codex Desktop broker pid %d has %d durable code-mode hosts; pass --anchor-pid", anchor, len(hosts))
	}
	return hosts[0], nil
}

func resolveDurableAnchor(startPID int, surface string, lookup anchorProcessLookup) (int, error) {
	if startPID <= 1 {
		return 0, fmt.Errorf("invalid ancestry start pid %d", startPID)
	}
	if lookup == nil {
		return 0, fmt.Errorf("process lookup is nil")
	}

	pid := startPID
	seen := make(map[int]struct{}, maxAnchorAncestryHops)
	for hop := 0; hop < maxAnchorAncestryHops && pid > 1; hop++ {
		if _, duplicate := seen[pid]; duplicate {
			return 0, fmt.Errorf("process ancestry cycle at pid %d", pid)
		}
		seen[pid] = struct{}{}

		proc, err := lookup(pid)
		if err != nil {
			return 0, err
		}
		if durableRuntimeForSurface(surface, proc.command) {
			return pid, nil
		}
		if proc.parentPID <= 1 || proc.parentPID == pid {
			break
		}
		pid = proc.parentPID
	}

	label := strings.TrimSpace(surface)
	if label == "" {
		label = "known interactive"
	}
	return 0, fmt.Errorf("no durable %s runtime found within %d ancestors", label, maxAnchorAncestryHops)
}

func durableRuntimeForSurface(surface, command string) bool {
	name := executableName(command)
	if name == "" || strings.Contains(name, "helper") {
		return false
	}

	match := func(names ...string) bool {
		for _, candidate := range names {
			if name == candidate {
				return true
			}
		}
		return false
	}

	switch strings.ToLower(strings.TrimSpace(surface)) {
	case "codex":
		return match("codex", "codex-code-mode-host")
	case "claude":
		return match("claude", "claude-code")
	case "gemini":
		return match("gemini", "gemini-cli")
	case "gemma":
		return match("gemma", "sirsi-gemma", "sirsi-gemma-worker")
	case "qwen":
		return match("qwen", "qwen-agent")
	case "":
		return match("codex", "codex-code-mode-host", "claude", "claude-code", "gemini", "gemini-cli", "gemma", "sirsi-gemma", "sirsi-gemma-worker", "qwen", "qwen-agent")
	default:
		// mcp/api/webhook/worker processes have no universal executable name.
		// Guessing would recreate the transient-helper defect, so their caller
		// must provide the already-known resident PID explicitly.
		return false
	}
}

func executableName(command string) string {
	return strings.ToLower(filepath.Base(strings.TrimSpace(command)))
}
