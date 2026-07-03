package brain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseProvider(t *testing.T) {
	tests := []struct {
		in       string
		wantKind ProviderKind
		wantRef  string
		wantErr  bool
	}{
		{"", ProviderNone, "", false},
		{"none", ProviderNone, "", false},
		{"  none  ", ProviderNone, "", false},
		{"local:qwen2.5-7b-instruct", ProviderLocal, "qwen2.5-7b-instruct", false},
		{"hosted:claude-3-5-haiku", ProviderHosted, "claude-3-5-haiku", false},
		{"local:", ProviderNone, "", true},  // missing id
		{"hosted:", ProviderNone, "", true}, // missing id
		{"cloud:gpt", ProviderNone, "", true},
		{"garbage", ProviderNone, "", true},
	}
	for _, tt := range tests {
		got, err := ParseProvider(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseProvider(%q): want error, got %+v", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProvider(%q): unexpected error %v", tt.in, err)
			continue
		}
		if got.Kind != tt.wantKind || got.Ref != tt.wantRef {
			t.Errorf("ParseProvider(%q) = {%s %q}, want {%s %q}", tt.in, got.Kind, got.Ref, tt.wantKind, tt.wantRef)
		}
	}
}

func TestProviderString_RoundTrip(t *testing.T) {
	for _, ref := range []string{"none", "local:qwen2.5-7b-instruct", "hosted:claude-3-5-haiku"} {
		p, err := ParseProvider(ref)
		if err != nil {
			t.Fatalf("ParseProvider(%q): %v", ref, err)
		}
		if got := p.String(); got != ref {
			t.Errorf("round-trip: ParseProvider(%q).String() = %q", ref, got)
		}
	}
}

func TestConfigLevel(t *testing.T) {
	tests := []struct {
		name  string
		roles map[string]string
		want  int
	}{
		{"fresh install is level 0", DefaultConfig().Roles, 0},
		{"triage local -> level 1", map[string]string{"triage": "local:qwen"}, 1},
		{"execution local -> level 2", map[string]string{"execution": "local:qwen"}, 2},
		{"triage+execution local -> level 2", map[string]string{"triage": "local:a", "execution": "local:b"}, 2},
		{"any hosted -> level 3", map[string]string{"triage": "hosted:haiku"}, 3},
		{"hosted execution -> level 3", map[string]string{"triage": "local:q", "execution": "hosted:opus"}, 3},
		{"nil roles -> level 0", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Roles: tt.roles}
			if got := c.Level(); got != tt.want {
				t.Errorf("Level() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name     string
		roles    map[string]string
		wantErrs int
	}{
		{"default is clean", DefaultConfig().Roles, 0},
		{"valid triage", map[string]string{"triage": "local:qwen"}, 0},
		{"unknown role", map[string]string{"conductor": "none"}, 1},
		{"malformed provider", map[string]string{"triage": "cloud:x"}, 1},
		{"dispatch model breaches the Tier-0 floor", map[string]string{"dispatch": "local:qwen"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Roles: tt.roles}
			errs := c.Validate()
			if len(errs) != tt.wantErrs {
				t.Errorf("Validate() = %d errors %v, want %d", len(errs), errs, tt.wantErrs)
			}
		})
	}
}

func TestConfigProvider_DefaultsToFloorOnGarbage(t *testing.T) {
	c := Config{Roles: map[string]string{"triage": "garbage"}}
	if p := c.Provider(RoleTriage); p.Kind != ProviderNone {
		t.Errorf("garbage provider should read as the deterministic floor, got %s", p.String())
	}
}

func TestConfigRoundTrip_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "brain.yaml")
	t.Setenv("SIRSI_BRAIN_CONFIG", path)

	// Fresh (no file) reads as the deterministic default, never an error.
	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (no file): %v", err)
	}
	if got.Level() != 0 {
		t.Errorf("fresh install must be Level 0, got %d", got.Level())
	}

	// Set persists and reloads.
	if setErr := Set(RoleTriage, "local:qwen2.5-7b-instruct"); setErr != nil {
		t.Fatalf("Set: %v", setErr)
	}
	reloaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig (after set): %v", err)
	}
	if got := reloaded.Provider(RoleTriage).String(); got != "local:qwen2.5-7b-instruct" {
		t.Errorf("persisted triage = %q", got)
	}
	if reloaded.Level() != 1 {
		t.Errorf("after triage local, Level() = %d, want 1", reloaded.Level())
	}

	// The file is 0600 (operator config; keys never live here per A29).
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("brain.yaml perm = %o, want 600", perm)
	}
}

func TestSet_RejectsDispatchModel(t *testing.T) {
	t.Setenv("SIRSI_BRAIN_CONFIG", filepath.Join(t.TempDir(), "brain.yaml"))
	if err := Set(RoleDispatch, "local:qwen"); err == nil {
		t.Fatal("Set(dispatch, local:...) must be rejected — Tier-0 floor (A29)")
	}
	// dispatch none is fine.
	if err := Set(RoleDispatch, "none"); err != nil {
		t.Errorf("Set(dispatch, none) should succeed: %v", err)
	}
}

func TestSet_RejectsUnknownRole(t *testing.T) {
	t.Setenv("SIRSI_BRAIN_CONFIG", filepath.Join(t.TempDir(), "brain.yaml"))
	if err := Set(Role("conductor"), "none"); err == nil {
		t.Fatal("Set with unknown role must error")
	}
}

func TestSet_SwapWithoutRestart(t *testing.T) {
	// "swap takes effect on next read" — Set then LoadConfig reflects it with no
	// process restart / no cached state.
	t.Setenv("SIRSI_BRAIN_CONFIG", filepath.Join(t.TempDir(), "brain.yaml"))
	if err := Set(RoleTriage, "local:a"); err != nil {
		t.Fatal(err)
	}
	if c, _ := LoadConfig(); c.Level() != 1 {
		t.Fatalf("after local triage, level = %d", c.Level())
	}
	if err := Set(RoleTriage, "none"); err != nil {
		t.Fatal(err)
	}
	if c, _ := LoadConfig(); c.Level() != 0 {
		t.Fatalf("after revert to none, level = %d (want 0)", c.Level())
	}
}
