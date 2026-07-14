// Package guard — loadbearing.go
//
// Load-Bearing Process Guard (ADR-040). Generalizes Rule A5 — "the Hapi module
// MUST NOT kill GPU processes that are actively training or inferencing" — from
// the GPU/VRAM domain to the RAM slayer. A launchd-managed local model /
// inference server (gemma-capped-server, ollama, llama-server, mlx_lm.server,
// vllm, …) is NOT reclaimable RAM: it holds a model resident ON PURPOSE.
//
// Forensic origin (2026-07-14). A RAM view sorted by RSS truncated argv at the
// exact byte that reveals identity, so a 25.8 GB process ranked #1 by RSS showed
// only as a nameless "Python". It was gemma-capped-server.py — a launchd-managed
// model server, actively serving (/health OK, R state, CPU active). "Just kill
// the hog to reclaim RAM" would have:
//  1. reclaimed nothing durably — launchd (PPID=1 / KeepAlive) respawns it and
//     reloads the model, trading resident RAM for a burst of pageins + a cold
//     reload;
//  2. severed the local-inference capability the fleet depends on;
//  3. acted on a misdiagnosis — "holding a model resident without serving" was
//     false; it WAS serving.
//
// The correct lever for a load-bearing hog is SIZING / POLICY — lower its RAM
// cap, run a smaller / more-quantized model, or evict-when-idle — never SIGKILL.
// This guard makes the slayer spare such processes and explain the real lever.
package guard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ProtectedProcess records a process the slayer spared and WHY, so the surface
// can explain that a kill would not have reclaimed durable RAM — and name the
// real lever instead of leaving the user thinking the tool did nothing.
type ProtectedProcess struct {
	PID    int
	Name   string
	Reason string
}

// loadBearingSignatures are lowercased argv substrings that identify a local
// model / inference server. These hold a model resident on purpose; killing them
// is churn (respawn + reload), not reclaim. Sirsi-owned servers first, then the
// common local LLM servers.
var loadBearingSignatures = []string{
	// Sirsi-owned local model servers
	"gemma-capped-server",
	"gemma-server",
	"sirsi-gemma",
	// Common local inference servers
	"mlx_lm.server",
	"mlx_lm",
	"llama-server",
	"llama.cpp",
	"llama_cpp",
	"ollama",
	"vllm",
	"text-generation-launcher",
	"text-generation-inference",
	"tabbyapi",
	"lmstudio",
	"lm-studio",
	"koboldcpp",
	"localai",
}

// isLocalModelServer is a generic heuristic for a local model / inference server
// not on the explicit signature list: it serves (a port / serve token) AND
// references a model (a --model flag or an on-disk weights file). Deterministic
// from argv alone. Loopback binding is deliberately NOT a decision input — a
// plain dev web server bound to 127.0.0.1 has no model and must stay killable.
func isLocalModelServer(cmd string) bool {
	serves := strings.Contains(cmd, "--port") ||
		strings.Contains(cmd, " serve") ||
		strings.Contains(cmd, "--host")
	if !serves {
		return false
	}
	return strings.Contains(cmd, "--model") ||
		strings.Contains(cmd, "--served-model-name") ||
		strings.Contains(cmd, ".gguf") ||
		strings.Contains(cmd, ".safetensors") ||
		strings.Contains(cmd, "mlx")
}

// isLoadBearingWith reports whether proc is a load-bearing local model /
// inference server that must NOT be slain to "reclaim RAM", plus a reason that
// names the real lever. The decision is deterministic from the process's FULL
// argv; the reason is enriched (best-effort, injectable) with launchd-management
// and serving state. Generalizes Rule A5 to RAM (guard).
func isLoadBearingWith(p platform.Platform, proc ProcessInfo) (bool, string) {
	cmd := strings.ToLower(fullCommand(p, proc))
	if strings.TrimSpace(cmd) == "" {
		return false, "" // can't misidentify what we can't read; other gates still apply
	}

	matched := ""
	for _, sig := range loadBearingSignatures {
		if strings.Contains(cmd, sig) {
			matched = sig
			break
		}
	}
	if matched == "" && isLocalModelServer(cmd) {
		matched = "local model server"
	}
	if matched == "" {
		return false, ""
	}

	// Enrich with the two facts that make a kill futile or harmful: it respawns
	// (launchd-managed) and it is doing work (CPU active — the Rule A5 signal).
	var notes []string
	if managedByLaunchd(p, proc.PID) {
		notes = append(notes, "launchd-managed — respawns on kill, so a kill reclaims no durable RAM")
	}
	if proc.CPUPercent >= 1.0 {
		notes = append(notes, fmt.Sprintf("actively serving (CPU %.0f%%)", proc.CPUPercent))
	}
	detail := ""
	if len(notes) > 0 {
		detail = " [" + strings.Join(notes, "; ") + "]"
	}

	reason := fmt.Sprintf(
		"load-bearing model server (%s)%s. It holds the model resident on purpose — killing it "+
			"reclaims no durable RAM. Use the sizing lever instead: lower its RAM cap, run a "+
			"smaller/more-quantized model, or enable evict-when-idle.",
		matched, detail,
	)
	return true, reason
}

// fullCommand returns the process's FULL argv. If proc.Command already carries
// arguments (a space beyond the executable) it is trusted; otherwise the argv is
// fetched on demand via `ps -o command=`. This is the "know what you'd kill
// before you kill it" discipline made mechanical: an interpreter such as bare
// "Python" or "node" is meaningless until its script/flags are read. Injectable
// via p.Command so tests stay hermetic; any probe failure falls back to the
// truncated Command (never a crash, never a false permit).
func fullCommand(p platform.Platform, proc ProcessInfo) string {
	if strings.Contains(strings.TrimSpace(proc.Command), " ") {
		return proc.Command
	}
	if proc.PID <= 1 {
		return proc.Command
	}
	out, err := p.Command("ps", "-o", "command=", "-p", strconv.Itoa(proc.PID))
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return proc.Command
	}
	return strings.TrimSpace(string(out))
}

// managedByLaunchd best-effort reports whether pid's parent is launchd (PPID==1)
// — a daemon that will be respawned after a kill. Injectable via p.Command so
// tests stay hermetic; any probe failure is treated as "unknown" (false).
func managedByLaunchd(p platform.Platform, pid int) bool {
	if pid <= 1 {
		return false
	}
	out, err := p.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}
