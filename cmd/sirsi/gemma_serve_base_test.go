package main

import (
	"strings"
	"testing"
)

// gemmaServerBase must find the launchd-owned SNE on its default port when the
// old broker's port file does not exist, and must report nothing when nothing
// answers (both directions).
func TestGemmaServerBaseFallsBackToDefaultPort(t *testing.T) {
	home := t.TempDir() // no gemma-server.port
	old := gemmaServerPingFn
	defer func() { gemmaServerPingFn = old }()

	gemmaServerPingFn = func(base string) bool { return strings.HasSuffix(base, ":8477") }
	if got := gemmaServerBase(home); got != "http://127.0.0.1:8477" {
		t.Fatalf("SNE answering on the default port was not found: %q", got)
	}
	gemmaServerPingFn = func(string) bool { return false }
	if got := gemmaServerBase(home); got != "" {
		t.Fatalf("nothing answers, got %q", got)
	}
}
