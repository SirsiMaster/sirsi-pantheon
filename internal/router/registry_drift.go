package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// RegistryDriftEntry describes a single agent that differs between the
// working-tree agents.json and origin/main.
type RegistryDriftEntry struct {
	AgentID string
	// Detail describes what changed (mechanism, label, added, removed).
	Detail string
}

// RegistryDrift compares the working-tree agents.json against the committed
// origin/main version and returns one entry per agent that differs.
// It is read-only and never modifies any file.
//
// A non-nil error means the comparison could not be run (e.g., git not
// available, no origin remote) — the caller should surface it as a skipped
// check rather than an alarm.
func RegistryDrift(repoRoot string) ([]RegistryDriftEntry, error) {
	agentsPath := filepath.Join(".agents", "idea-router", "agents.json")

	mainJSON, err := gitShowOriginMain(repoRoot, agentsPath)
	if err != nil {
		return nil, fmt.Errorf("read origin/main agents.json: %w", err)
	}

	diskJSON, err := os.ReadFile(filepath.Join(repoRoot, agentsPath))
	if err != nil {
		return nil, fmt.Errorf("read working-tree agents.json: %w", err)
	}

	// Parse both — use raw map so unknown fields (e.g. consumer, env) are
	// preserved without needing the full struct hierarchy.
	mainAgents, err := parseAgentWakeMap(mainJSON)
	if err != nil {
		return nil, fmt.Errorf("parse origin/main agents.json: %w", err)
	}
	diskAgents, err := parseAgentWakeMap(diskJSON)
	if err != nil {
		return nil, fmt.Errorf("parse working-tree agents.json: %w", err)
	}

	var drifted []RegistryDriftEntry

	// Agents in origin/main — check they exist and match in working-tree.
	ids := sortedKeys(mainAgents)
	for _, id := range ids {
		mainWake := mainAgents[id]
		diskWake, exists := diskAgents[id]
		if !exists {
			drifted = append(drifted, RegistryDriftEntry{AgentID: id, Detail: "present in origin/main, missing from working-tree"})
			continue
		}
		if diff := wakeFieldDiff(mainWake, diskWake); diff != "" {
			drifted = append(drifted, RegistryDriftEntry{AgentID: id, Detail: diff})
		}
	}

	// Agents added in working-tree that origin/main doesn't know about.
	for _, id := range sortedKeys(diskAgents) {
		if _, exists := mainAgents[id]; !exists {
			drifted = append(drifted, RegistryDriftEntry{AgentID: id, Detail: "not in origin/main (working-tree addition)"})
		}
	}

	return drifted, nil
}

// gitShowOriginMain runs `git show origin/main:<path>` relative to repoRoot.
func gitShowOriginMain(repoRoot, relPath string) ([]byte, error) {
	cmd := exec.Command("git", "show", "origin/main:"+relPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			return nil, fmt.Errorf("git show origin/main:%s failed (exit %d): %s", relPath, exitErr.ExitCode(), bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, err
	}
	return out, nil
}

func isExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

// agentWakeSnapshot holds the wake fields we care about for drift comparison.
type agentWakeSnapshot struct {
	mechanism        string
	launchAgentLabel string
	endpoint         string
	mcpServer        string
}

func parseAgentWakeMap(data []byte) (map[string]agentWakeSnapshot, error) {
	var raw struct {
		Agents map[string]struct {
			Wake struct {
				Mechanism        string `json:"mechanism"`
				LaunchAgentLabel string `json:"launch_agent_label"`
				Endpoint         string `json:"endpoint"`
				MCPServer        string `json:"mcp_server"`
			} `json:"wake"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]agentWakeSnapshot, len(raw.Agents))
	for id, a := range raw.Agents {
		out[id] = agentWakeSnapshot{
			mechanism:        a.Wake.Mechanism,
			launchAgentLabel: a.Wake.LaunchAgentLabel,
			endpoint:         a.Wake.Endpoint,
			mcpServer:        a.Wake.MCPServer,
		}
	}
	return out, nil
}

// wakeFieldDiff returns a human-readable description of the first wake field
// difference between main and disk, or "" if they match.
func wakeFieldDiff(main, disk agentWakeSnapshot) string {
	if main.mechanism != disk.mechanism {
		return fmt.Sprintf("wake.mechanism: origin/main=%q disk=%q", main.mechanism, disk.mechanism)
	}
	if main.launchAgentLabel != "" && disk.launchAgentLabel == "" {
		return fmt.Sprintf("wake.launch_agent_label: origin/main=%q disk=<missing>", main.launchAgentLabel)
	}
	if main.launchAgentLabel != disk.launchAgentLabel && disk.launchAgentLabel != "" {
		return fmt.Sprintf("wake.launch_agent_label: origin/main=%q disk=%q", main.launchAgentLabel, disk.launchAgentLabel)
	}
	if main.endpoint != disk.endpoint {
		return fmt.Sprintf("wake.endpoint: origin/main=%q disk=%q", main.endpoint, disk.endpoint)
	}
	if main.mcpServer != disk.mcpServer {
		return fmt.Sprintf("wake.mcp_server: origin/main=%q disk=%q", main.mcpServer, disk.mcpServer)
	}
	return ""
}

func sortedKeys(m map[string]agentWakeSnapshot) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
