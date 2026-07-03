// Package brain — config.go is the Orchestration Brain CONTROL PLANE (ADR-034,
// PANTHEON_RULES A29). It defines the tiered, pluggable, user-navigable brain
// config: which provider (deterministic / local model / hosted) drives each
// orchestration ROLE, and the LLM Level (0–3) that config derives to.
//
// This is deliberately SEPARATE from the model download/inference files in this
// package (downloader.go / inference.go): those are the mechanism a local
// provider USES; this is the operator-facing config that SELECTS a provider per
// role. The brain never becomes the loop — Tier-0 dispatch is deterministic and
// ships ON at Level 0 with zero LLM (see A29 §"deterministic floor").
//
// Storage is `~/.sirsi/brain.yaml` via gopkg.in/yaml.v3 — the repo's actual
// config standard (see internal/profile/profile.go). The plan's "brain.yaml not
// bespoke .conf" amendment is honored by using structured YAML; we reuse the
// established yaml.v3 pattern rather than adding a new dependency (Rule 0 —
// reuse, not reinvent).
package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Role is one orchestration responsibility that a provider can be plugged into.
// Roles are pluggable INDEPENDENTLY (A29 §per-role): triage may run a local
// model while execution stays deterministic, etc.
type Role string

const (
	// RoleDispatch — Tier-0: watch inbox, route by rules, heartbeat, close acks.
	// ALWAYS deterministic (ProviderNone) — the invariant floor. A29 forbids a
	// model from becoming the dispatch loop; this role exists in config for
	// visibility/symmetry but must never be set to a model provider.
	RoleDispatch Role = "dispatch"
	// RoleTriage — Tier-1: classify ambiguous items, emit action grammar.
	RoleTriage Role = "triage"
	// RoleExecution — Tier-2: agentic build/review/bind/deploy.
	RoleExecution Role = "execution"
)

// Roles returns the roles in canonical (stable) order for display.
func Roles() []Role { return []Role{RoleDispatch, RoleTriage, RoleExecution} }

// ProviderKind classifies a provider without parsing its reference string.
type ProviderKind string

const (
	// ProviderNone — deterministic, no LLM. The mandatory Tier-0 floor.
	ProviderNone ProviderKind = "none"
	// ProviderLocal — an on-device model (MLX/Ollama/CoreML), zero-token.
	// Reference form: "local:<model-id>".
	ProviderLocal ProviderKind = "local"
	// ProviderHosted — an opt-in hosted API (Haiku/Sonnet/Opus or any
	// OpenAI-compatible). The only path that adds per-token cost.
	// Reference form: "hosted:<provider-id>".
	ProviderHosted ProviderKind = "hosted"
)

// Provider is a role's plugged-in engine, stored as a single reference string so
// the YAML stays human-editable ("none", "local:qwen2.5-7b-instruct",
// "hosted:claude-3-5-haiku"). Kind + Ref are derived, never stored redundantly.
type Provider struct {
	Kind ProviderKind
	Ref  string // model/provider id for local/hosted; "" for none
}

// ParseProvider derives a Provider from its reference string. Empty or "none"
// is the deterministic floor. Unknown kinds are rejected so a typo in brain.yaml
// surfaces as a config error in `sirsi brain doctor`, never a silent fallback.
func ParseProvider(s string) (Provider, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == string(ProviderNone) {
		return Provider{Kind: ProviderNone}, nil
	}
	kind, ref, found := strings.Cut(s, ":")
	kind = strings.TrimSpace(kind)
	ref = strings.TrimSpace(ref)
	switch ProviderKind(kind) {
	case ProviderLocal, ProviderHosted:
		if !found || ref == "" {
			return Provider{}, fmt.Errorf("provider %q needs a model/provider id (e.g. %s:<id>)", s, kind)
		}
		return Provider{Kind: ProviderKind(kind), Ref: ref}, nil
	case ProviderNone:
		return Provider{Kind: ProviderNone}, nil
	default:
		return Provider{}, fmt.Errorf("unknown provider kind %q (want none|local:<id>|hosted:<id>)", kind)
	}
}

// String renders the provider back to its canonical reference form.
func (p Provider) String() string {
	if p.Kind == ProviderNone || p.Kind == "" {
		return string(ProviderNone)
	}
	return string(p.Kind) + ":" + p.Ref
}

// Config is the brain control-plane config: one provider per role. It is stored
// as a plain string map in YAML for hand-editability; the typed Provider is
// derived on load via Provider(role).
type Config struct {
	// Roles maps each role to its provider reference string. Absent roles
	// default to the deterministic floor (ProviderNone).
	Roles map[string]string `yaml:"roles"`
}

// DefaultConfig is the fresh-install state: every role deterministic (Level 0).
// A public download runs the realm with zero AI, zero keys, zero cost (A29 §floor).
func DefaultConfig() Config {
	return Config{Roles: map[string]string{
		string(RoleDispatch):  string(ProviderNone),
		string(RoleTriage):    string(ProviderNone),
		string(RoleExecution): string(ProviderNone),
	}}
}

// Provider returns the derived Provider for a role, defaulting to the
// deterministic floor when the role is unset or malformed. Malformation is
// reported by Validate/doctor, not here — a read must never panic the loop.
func (c Config) Provider(role Role) Provider {
	if c.Roles == nil {
		return Provider{Kind: ProviderNone}
	}
	p, err := ParseProvider(c.Roles[string(role)])
	if err != nil {
		return Provider{Kind: ProviderNone}
	}
	return p
}

// Level derives the LLM spectrum Level (0–3) from the config — the single number
// the user navigates (A29 §spectrum). It is the HIGHEST capability any role
// currently uses:
//
//	Level 0 — all roles deterministic (no LLM).
//	Level 1 — triage uses a LOCAL model (zero-token), execution still non-agentic.
//	Level 2 — execution uses a model (agentic), still local (no hosted keys).
//	Level 3 — any role uses a HOSTED provider (opt-in, per-token cost).
func (c Config) Level() int {
	level := 0
	if c.Provider(RoleTriage).Kind == ProviderLocal {
		level = 1
	}
	if c.Provider(RoleExecution).Kind == ProviderLocal {
		if level < 2 {
			level = 2
		}
	}
	for _, r := range Roles() {
		if c.Provider(r).Kind == ProviderHosted {
			return 3
		}
	}
	return level
}

// Validate reports every config problem in a stable order so `doctor` can list
// them all at once. It does NOT touch the model/RAM/loop — those are live checks
// layered on top in controlplane.go; Validate is pure config hygiene.
func (c Config) Validate() []error {
	var errs []error
	keys := make([]string, 0, len(c.Roles))
	for k := range c.Roles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	known := map[string]bool{}
	for _, r := range Roles() {
		known[string(r)] = true
	}
	for _, k := range keys {
		if !known[k] {
			errs = append(errs, fmt.Errorf("unknown role %q in brain.yaml (want dispatch|triage|execution)", k))
			continue
		}
		if _, err := ParseProvider(c.Roles[k]); err != nil {
			errs = append(errs, fmt.Errorf("role %q: %w", k, err))
		}
	}
	// The deterministic-floor invariant: dispatch (Tier-0) must never be a model.
	if p := c.Provider(RoleDispatch); p.Kind != ProviderNone {
		errs = append(errs, fmt.Errorf("role %q must stay deterministic (Tier-0 floor, A29) — got %q", RoleDispatch, p.String()))
	}
	return errs
}

// ConfigPath is the on-disk location of brain.yaml (~/.sirsi/brain.yaml).
// Overridable via SIRSI_BRAIN_CONFIG for tests + non-default homes (A16 spirit).
func ConfigPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("SIRSI_BRAIN_CONFIG")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".sirsi", "brain.yaml"), nil
}

// LoadConfig reads brain.yaml, returning DefaultConfig (Level 0) when the file
// does not exist — a fresh install is deterministic, never an error. This is
// what makes "works out of the box with zero AI" true (A29 §floor).
func LoadConfig() (Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Roles == nil {
		cfg.Roles = map[string]string{}
	}
	return cfg, nil
}

// SaveConfig writes brain.yaml (0600 — it may later hold nothing secret, but the
// hosted-key handling rule A29 §keys keeps keys OUT of this file; 0600 is the
// conservative default for an operator config). Creates ~/.sirsi if absent.
func SaveConfig(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return fmt.Errorf("create config dir: %w", mkErr)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal brain config: %w", err)
	}
	if wErr := os.WriteFile(path, data, 0o600); wErr != nil {
		return fmt.Errorf("write %s: %w", path, wErr)
	}
	return nil
}

// Set plugs a provider into a role (the `sirsi brain use <role> <provider>`
// mutation), validating both before persisting so a bad swap is rejected with a
// plain-English error and the on-disk config is never left malformed. Swaps take
// effect on next read — no restart (A29 §swap-without-restart).
func Set(role Role, providerRef string) error {
	known := false
	for _, r := range Roles() {
		if r == role {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("unknown role %q (want dispatch|triage|execution)", role)
	}
	p, err := ParseProvider(providerRef)
	if err != nil {
		return err
	}
	if role == RoleDispatch && p.Kind != ProviderNone {
		return fmt.Errorf("role %q must stay deterministic (Tier-0 floor, A29) — only 'none' is allowed", role)
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.Roles == nil {
		cfg.Roles = map[string]string{}
	}
	cfg.Roles[string(role)] = p.String()
	return SaveConfig(cfg)
}
