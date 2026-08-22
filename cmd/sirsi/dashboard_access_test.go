package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This source-level release invariant prevents another dashboard constructor
// from silently restoring the unauthenticated migration mode. Runtime boundary
// behavior is covered by internal/dashboard/loopback_boundary_test.go.
func TestDashboardEntrypointRequiresLocalAICapability(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("dashboard.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"LoadOrCreateDefaultSNELocalAccessToken",
		"DefaultSNELocalAccessTokenPath",
		"SNELocalAccessToken:",
		"SNELocalAccessTokenPath:",
		"Refusing to start an unauthenticated local AI API",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("dashboard entrypoint lost required local capability wiring %q", required)
		}
	}
}
