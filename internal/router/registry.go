// Package router — registry.go
//
// Agent registry for the multi-agent work queue (Router v3).
// Agents are registered in .agents/idea-router/agents.json.
// Ra owns the router; Thoth preserves continuity; Ma'at validates governance.
package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AgentConfig defines a registered agent that can receive work.
type AgentConfig struct {
	// ID is the unique agent identifier (e.g., "claude-pantheon", "codex-pantheon")
	ID string `json:"id"`

	// Type is the agent platform (e.g., "claude", "codex", "gemini", "qwen")
	Type string `json:"type"`

	// Command is the launch command as an array (never shell strings).
	// The work prompt is appended as the final argument by the executor.
	Command []string `json:"command"`

	// Cwd is the working directory for the agent.
	Cwd string `json:"cwd"`

	// Env is optional environment variable overrides.
	Env map[string]string `json:"env,omitempty"`

	// Workstream is the default workstream this agent handles.
	Workstream string `json:"workstream,omitempty"`

	// Wake describes how Horus should wake this agent when work is pending.
	// Missing wake metadata defaults to cli-spawn for existing agents.
	Wake WakeConfig `json:"wake,omitempty"`

	// extra preserves JSON fields not known to this version of AgentConfig
	// (e.g. "consumer") so a LoadRegistry→SaveRegistry round-trip never
	// silently erases metadata the struct does not model. Adding a typed field
	// later is additive and safe — the extra copy is just dropped on load.
	extra map[string]json.RawMessage
}

// UnmarshalJSON decodes known fields into the struct and captures everything
// else in extra so it is re-emitted on save.
func (cfg *AgentConfig) UnmarshalJSON(b []byte) error {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	// type plain breaks the UnmarshalJSON recursion.
	type plain AgentConfig
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*cfg = AgentConfig(p)

	// Marshal the typed struct to discover its canonical key set, then
	// keep anything left over as extra. This avoids a hard-coded whitelist
	// that would need updating every time a new field is added to the struct.
	knownJSON, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(knownJSON, &known); err != nil {
		return err
	}
	for k := range known {
		delete(all, k)
	}
	if len(all) > 0 {
		cfg.extra = all
	}
	return nil
}

// MarshalJSON emits the typed fields followed by any extras captured on load.
func (cfg AgentConfig) MarshalJSON() ([]byte, error) {
	type plain AgentConfig
	knownJSON, err := json.Marshal(plain(cfg))
	if err != nil {
		return nil, err
	}
	if len(cfg.extra) == 0 {
		return knownJSON, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(knownJSON, &m); err != nil {
		return nil, err
	}
	for k, v := range cfg.extra {
		if _, ok := m[k]; !ok { // never let an extra shadow a typed field
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// WakeConfig defines the pluggable wake adapter for a registered agent.
type WakeConfig struct {
	Mechanism   string            `json:"mechanism,omitempty"`
	Endpoint    string            `json:"endpoint,omitempty"`
	Auth        string            `json:"auth,omitempty"`
	MCPServer   string            `json:"mcp_server,omitempty"`
	HealthCheck []string          `json:"health_check,omitempty"`
	AuthCheck   []string          `json:"auth_check,omitempty"`
	Hooks       map[string]string `json:"hooks,omitempty"`
	// LaunchAgentLabel stores whatever value is present in agents.json for
	// round-trip fidelity. LoadRegistry does NOT auto-fill this field.
	// The operational label is always derived at call time via
	// WakeLaunchAgentLabel(cfg.ID) — never read back from this stored value.
	// A missing or stale stored label is caught by registrydrift.go.
	LaunchAgentLabel string `json:"launch_agent_label,omitempty"`
}

// Registry holds all registered agent configurations.
type Registry struct {
	Agents map[string]AgentConfig `json:"agents"`
}

// LoadRegistry reads agents.json from the router directory.
func LoadRegistry(routerRoot string) (*Registry, error) {
	path := filepath.Join(routerRoot, "agents.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Registry{Agents: make(map[string]AgentConfig)}, nil
		}
		return nil, fmt.Errorf("read agents.json: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parse agents.json: %w", err)
	}
	if reg.Agents == nil {
		reg.Agents = make(map[string]AgentConfig)
	}

	// Inject IDs from map keys — this is the ONLY post-unmarshal mutation
	// LoadRegistry performs. No other fields (including WakeConfig.LaunchAgentLabel)
	// are auto-filled. The operational launchd label is derived at call time via
	// WakeLaunchAgentLabel(cfg.ID); see wake.go.
	for id, cfg := range reg.Agents {
		cfg.ID = id
		reg.Agents[id] = cfg
	}

	return &reg, nil
}

// SaveRegistry writes agents.json losslessly: unknown JSON keys present in the
// existing file (e.g. "consumer") are preserved on every write because
// AgentConfig.MarshalJSON re-emits them via the extra field. Known fields
// reflect the current Go state — a field cleared to its zero value is absent
// from output (not restored from disk). Top-level JSON keys the schema has not
// modeled survive because we seed from the existing file before overwriting.
func SaveRegistry(routerRoot string, reg *Registry) error {
	path := filepath.Join(routerRoot, "agents.json")

	// Preserve any top-level JSON keys the schema hasn't modeled by seeding
	// from the existing file. D2: a non-empty but unparseable file is an error —
	// silently dropping it would erase the very unknown keys we aim to keep.
	var fileRaw map[string]json.RawMessage
	if existing, err := os.ReadFile(path); err == nil {
		if len(existing) > 0 {
			if err := json.Unmarshal(existing, &fileRaw); err != nil {
				return fmt.Errorf("parse existing agents.json: %w", err)
			}
		}
	}
	if fileRaw == nil {
		fileRaw = make(map[string]json.RawMessage)
	}

	// AgentConfig.MarshalJSON is already lossless: typed fields reflect the
	// current Go state (cleared omitempty = absent), extra unknown fields are
	// re-emitted. No deepMerge against the old disk bytes is needed or wanted —
	// deepMerge would restore stale cleared values from disk (D1).
	agents := make(map[string]json.RawMessage, len(reg.Agents))
	for id, cfg := range reg.Agents {
		cfgBytes, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("marshal agent %q: %w", id, err)
		}
		agents[id] = json.RawMessage(cfgBytes)
	}

	agentsBytes, err := json.MarshalIndent(agents, "  ", "  ")
	if err != nil {
		return fmt.Errorf("marshal agents map: %w", err)
	}
	fileRaw["agents"] = json.RawMessage(agentsBytes)

	data, err := json.MarshalIndent(fileRaw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agents.json: %w", err)
	}
	data = append(data, '\n')

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(routerRoot, ".agents.json-*")
	if err != nil {
		return fmt.Errorf("create temporary agents.json: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary agents.json: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temporary agents.json: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary agents.json: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace agents.json: %w", err)
	}
	return nil
}

// Lookup returns the agent config for the given ID, or an error if not registered.
func (r *Registry) Lookup(agentID string) (*AgentConfig, error) {
	cfg, ok := r.Agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent %q not registered — add to .agents/idea-router/agents.json", agentID)
	}
	return &cfg, nil
}

// Validate checks that an agent config has the minimum required fields.
func (cfg *AgentConfig) Validate() error {
	if cfg.ID == "" {
		return fmt.Errorf("agent ID is required")
	}
	if cfg.Type == "" {
		return fmt.Errorf("agent %q: type is required", cfg.ID)
	}
	switch cfg.WakeMechanism() {
	case "", "cli-spawn", WakeLaunchAgent:
		// launchagent is a cli-spawn wrapped in a resident launchd pull-loop
		// (`sirsi router wake-install`): it ultimately spawns the agent's command,
		// so it carries the same requirements. It is a first-class mechanism —
		// NOT the catch-all "unsupported" default, which was the true source of
		// the board's false "unsupported wake mechanism launchagent" label.
		if len(cfg.Command) == 0 {
			return fmt.Errorf("agent %q: command array is required for %s (no shell strings)", cfg.ID, cfg.WakeMechanism())
		}
		if cfg.Cwd == "" {
			return fmt.Errorf("agent %q: cwd is required for %s", cfg.ID, cfg.WakeMechanism())
		}
	case "api-call":
		if cfg.Wake.Endpoint == "" {
			return fmt.Errorf("agent %q: wake.endpoint is required for api-call", cfg.ID)
		}
	case "mcp-notification":
		if cfg.Wake.MCPServer == "" {
			return fmt.Errorf("agent %q: wake.mcp_server is required for mcp-notification", cfg.ID)
		}
	case WakeNone:
		// explicit opt-out: agent cannot be auto-woken, operator must start it manually
	default:
		return fmt.Errorf("agent %q: unsupported wake mechanism %q", cfg.ID, cfg.Wake.Mechanism)
	}
	return nil
}

// WakeMechanism returns the configured wake mechanism, defaulting legacy agents
// with commands to cli-spawn.
func (cfg AgentConfig) WakeMechanism() string {
	if cfg.Wake.Mechanism != "" {
		return cfg.Wake.Mechanism
	}
	if len(cfg.Command) > 0 {
		return "cli-spawn"
	}
	return ""
}

// ValidateAll checks every registered agent.
func (r *Registry) ValidateAll() []error {
	var errs []error
	for _, cfg := range r.Agents {
		if err := cfg.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// IsRegistered returns true if the agent ID exists in the registry.
func (r *Registry) IsRegistered(agentID string) bool {
	_, ok := r.Agents[agentID]
	return ok
}
