package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// apolloRouter installs an inert Apollo.app router marker (a regular file, mode perm) under home.
func apolloRouter(t *testing.T, home string, perm os.FileMode) {
	t.Helper()
	dir := filepath.Join(home, "Applications", "Apollo.app", "Contents", "Helpers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apollo-router"), []byte("#!/bin/sh\nexit 0\n"), perm); err != nil {
		t.Fatal(err)
	}
}

func TestApolloDashboardSelectedModel(t *testing.T) {
	cases := []struct {
		name    string
		conf    *string // nil = no file
		apollo  os.FileMode
		want    string
		wantErr string
	}{
		{name: "selected model is used as-is", conf: strPtr("mlx-community/gemma-4-12B-it-8bit\n"), apollo: 0o755, want: "mlx-community/gemma-4-12B-it-8bit"},
		{name: "legacy broker without Apollo keeps local", conf: nil, want: "local"},
		{name: "legacy broker, blank selection keeps local", conf: strPtr("  \n"), want: "local"},
		{name: "Apollo, no selection is an error", conf: nil, apollo: 0o755, wantErr: "no model is selected"},
		{name: "Apollo, blank selection is an error", conf: strPtr("\n"), apollo: 0o755, wantErr: "no model is selected"},
		{name: "invalid Apollo marker is an error, never local", conf: nil, apollo: 0o644, wantErr: "regular executable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
				t.Fatal(err)
			}
			if c.conf != nil {
				if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-model.conf"), []byte(*c.conf), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if c.apollo != 0 {
				apolloRouter(t, home, c.apollo)
			}
			got, err := sneModel()
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("got (%q, %v), want error containing %q", got, err, c.wantErr)
				}
				if got != "" {
					t.Fatalf("on error the model must be empty, got %q", got)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got (%q, %v), want %q", got, err, c.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
