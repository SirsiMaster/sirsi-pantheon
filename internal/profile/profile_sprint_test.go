package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveConfig_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg := &Config{
		ActiveProfile: "ai-engineer",
		DryRunDefault: false,
		JSONOutput:    true,
		UseTrash:      false,
		Settings:      map[string]string{"key": "value"},
	}

	err := SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	// Verify file exists
	path := filepath.Join(home, ".config", "anubis", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Config file not created: %v", err)
	}

	// Load it back
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded.ActiveProfile != "ai-engineer" {
		t.Errorf("ActiveProfile = %q, want ai-engineer", loaded.ActiveProfile)
	}
	if loaded.JSONOutput != true {
		t.Error("JSONOutput should be true")
	}
}

func TestSaveProfile_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	p := &Profile{
		Name:         "custom",
		Description:  "Custom test profile",
		Categories:   []string{"general", "dev"},
		MinAgeDays:   30,
		ExcludeRules: []string{"R1", "R2"},
	}

	err := SaveProfile(p)
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	// Verify file exists
	path := filepath.Join(home, ".config", "anubis", "profiles", "custom.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Profile file not created: %v", err)
	}

	// Load it back
	loaded, err := LoadProfile("custom")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if loaded.Name != "custom" {
		t.Errorf("Name = %q, want custom", loaded.Name)
	}
	if len(loaded.Categories) != 2 {
		t.Errorf("Categories = %v, want 2", loaded.Categories)
	}
}

func TestLoadConfig_NotExist(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig should return default: %v", err)
	}
	if cfg.ActiveProfile != "developer" {
		t.Errorf("Default profile should be 'developer', got %q", cfg.ActiveProfile)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write invalid YAML
	dir := filepath.Join(home, ".config", "anubis")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("{{{{invalid"), 0644)

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestListProfiles_WithUserProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Save a user profile first
	p := &Profile{
		Name:        "test-user-profile",
		Description: "A user profile",
		Categories:  []string{"general"},
	}
	SaveProfile(p)

	profiles, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}

	// Should have built-in + user
	if len(profiles) <= len(DefaultProfiles()) {
		t.Errorf("Expected more than %d profiles, got %d", len(DefaultProfiles()), len(profiles))
	}

	// Find our user profile
	found := false
	for _, prof := range profiles {
		if prof.Name == "test-user-profile" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find test-user-profile in listing")
	}
}

func TestLoadProfile_UserNotFound(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	_, err := LoadProfile("nonexistent-profile")
	if err == nil {
		t.Error("Expected error for missing profile")
	}
}
