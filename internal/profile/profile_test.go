package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────
// DefaultProfiles
// ─────────────────────────────────────────────────

func TestDefaultProfiles_Count(t *testing.T) {
	profiles := DefaultProfiles()
	if len(profiles) != 4 {
		t.Errorf("expected 4 default profiles, got %d", len(profiles))
	}
}

func TestDefaultProfiles_Names(t *testing.T) {
	expected := map[string]bool{
		"general":     true,
		"developer":   true,
		"ai-engineer": true,
		"devops":      true,
	}

	profiles := DefaultProfiles()
	for _, p := range profiles {
		if !expected[p.Name] {
			t.Errorf("unexpected default profile name: %q", p.Name)
		}
		delete(expected, p.Name)
	}
	for name := range expected {
		t.Errorf("missing default profile: %q", name)
	}
}

func TestDefaultProfiles_AllHaveRequiredFields(t *testing.T) {
	profiles := DefaultProfiles()
	for _, p := range profiles {
		if p.Name == "" {
			t.Error("profile has empty Name")
		}
		if p.Description == "" {
			t.Errorf("profile %q has empty Description", p.Name)
		}
		if len(p.Categories) == 0 {
			t.Errorf("profile %q has no Categories", p.Name)
		}
	}
}

func TestDefaultProfiles_AllIncludeGeneral(t *testing.T) {
	// Every profile should include "general" cleanup
	profiles := DefaultProfiles()
	for _, p := range profiles {
		hasGeneral := false
		for _, cat := range p.Categories {
			if cat == "general" {
				hasGeneral = true
				break
			}
		}
		if !hasGeneral {
			t.Errorf("profile %q does not include 'general' category", p.Name)
		}
	}
}

func TestDefaultProfiles_MinAgeDaysPositive(t *testing.T) {
	profiles := DefaultProfiles()
	for _, p := range profiles {
		if p.MinAgeDays <= 0 {
			t.Errorf("profile %q has MinAgeDays=%d, expected positive", p.Name, p.MinAgeDays)
		}
	}
}

// ─────────────────────────────────────────────────
// DefaultConfig
// ─────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ActiveProfile != "developer" {
		t.Errorf("ActiveProfile = %q, want %q", cfg.ActiveProfile, "developer")
	}
	if !cfg.DryRunDefault {
		t.Error("DryRunDefault should be true by default (safety first)")
	}
	if !cfg.UseTrash {
		t.Error("UseTrash should be true by default (safety first)")
	}
	if cfg.JSONOutput {
		t.Error("JSONOutput should be false by default")
	}
}

// ─────────────────────────────────────────────────
// LoadProfile — built-in
// ─────────────────────────────────────────────────

func TestLoadProfile_BuiltIn(t *testing.T) {
	for _, name := range []string{"general", "developer", "ai-engineer", "devops"} {
		p, err := LoadProfile(name)
		if err != nil {
			t.Errorf("LoadProfile(%q) error: %v", name, err)
			continue
		}
		if p.Name != name {
			t.Errorf("LoadProfile(%q) returned profile with Name=%q", name, p.Name)
		}
	}
}

func TestLoadProfile_NotFound(t *testing.T) {
	_, err := LoadProfile("nonexistent-profile")
	if err == nil {
		t.Error("expected error for non-existent profile, got nil")
	}
}

// ─────────────────────────────────────────────────
// SaveConfig + LoadConfig round-trip
// ─────────────────────────────────────────────────

func TestConfigRoundTrip(t *testing.T) {
	// Use temp dir as config directory to avoid touching real system
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &Config{
		ActiveProfile: "ai-engineer",
		DryRunDefault: false,
		JSONOutput:    true,
		UseTrash:      true,
		Settings:      map[string]string{"custom": "value"},
	}

	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}

	if loaded.ActiveProfile != cfg.ActiveProfile {
		t.Errorf("ActiveProfile = %q, want %q", loaded.ActiveProfile, cfg.ActiveProfile)
	}
	if loaded.DryRunDefault != cfg.DryRunDefault {
		t.Errorf("DryRunDefault = %v, want %v", loaded.DryRunDefault, cfg.DryRunDefault)
	}
	if loaded.JSONOutput != cfg.JSONOutput {
		t.Errorf("JSONOutput = %v, want %v", loaded.JSONOutput, cfg.JSONOutput)
	}
	if loaded.UseTrash != cfg.UseTrash {
		t.Errorf("UseTrash = %v, want %v", loaded.UseTrash, cfg.UseTrash)
	}
	if loaded.Settings["custom"] != "value" {
		t.Errorf("Settings[custom] = %q, want %q", loaded.Settings["custom"], "value")
	}
}

func TestLoadConfig_DefaultWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error when no file: %v", err)
	}

	def := DefaultConfig()
	if cfg.ActiveProfile != def.ActiveProfile {
		t.Errorf("missing config should return default, got ActiveProfile=%q", cfg.ActiveProfile)
	}
}

// ─────────────────────────────────────────────────
// SaveProfile + LoadProfile round-trip (user profiles)
// ─────────────────────────────────────────────────

func TestProfileRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	p := &Profile{
		Name:         "custom-scan",
		Description:  "Custom scan profile for testing",
		Categories:   []string{"general", "ai"},
		MinAgeDays:   3,
		ExcludeRules: []string{"trash"},
		Settings:     map[string]string{"verbose": "true"},
	}

	if err := SaveProfile(p); err != nil {
		t.Fatalf("SaveProfile() error: %v", err)
	}

	loaded, err := LoadProfile("custom-scan")
	if err != nil {
		t.Fatalf("LoadProfile('custom-scan') error: %v", err)
	}

	if loaded.Name != p.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, p.Name)
	}
	if loaded.Description != p.Description {
		t.Errorf("Description = %q, want %q", loaded.Description, p.Description)
	}
	if len(loaded.Categories) != 2 {
		t.Errorf("Categories count = %d, want 2", len(loaded.Categories))
	}
	if loaded.MinAgeDays != 3 {
		t.Errorf("MinAgeDays = %d, want 3", loaded.MinAgeDays)
	}
}

// ─────────────────────────────────────────────────
// ListProfiles
// ─────────────────────────────────────────────────

func TestListProfiles_IncludesBuiltIn(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error: %v", err)
	}

	if len(profiles) < 4 {
		t.Errorf("expected at least 4 built-in profiles, got %d", len(profiles))
	}
}

func TestListProfiles_IncludesUserProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Save a user profile
	p := &Profile{
		Name:        "my-custom",
		Description: "My custom profile",
		Categories:  []string{"general"},
		MinAgeDays:  1,
	}
	if err := SaveProfile(p); err != nil {
		t.Fatalf("SaveProfile() error: %v", err)
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error: %v", err)
	}

	found := false
	for _, pr := range profiles {
		if pr.Name == "my-custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("user profile 'my-custom' not found in ListProfiles()")
	}
}

func TestListProfiles_BuiltInTakesPrecedence(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Save a user profile with same name as built-in
	profileDir := filepath.Join(tmpDir, ".config", "anubis", "profiles")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(profileDir, "developer.yaml"),
		[]byte("name: developer\ndescription: overridden\ncategories: [general]\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error: %v", err)
	}

	// Count how many "developer" profiles exist — should be exactly 1
	devCount := 0
	for _, p := range profiles {
		if p.Name == "developer" {
			devCount++
		}
	}
	if devCount != 1 {
		t.Errorf("expected exactly 1 'developer' profile (built-in wins), got %d", devCount)
	}
}

// ─────────────────────────────────────────────────
// isYAMLFile
// ─────────────────────────────────────────────────

func TestIsYAMLFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"config.yaml", true},
		{"config.yml", true},
		{"config.json", false},
		{"config.toml", false},
		{"config", false},
		{".yaml", true},
	}

	for _, tt := range tests {
		got := isYAMLFile(tt.name)
		if got != tt.want {
			t.Errorf("isYAMLFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
