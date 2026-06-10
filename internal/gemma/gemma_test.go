package gemma

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanOutput(t *testing.T) {
	in := "Fetching 9 files: 100%\n==========\nHello there, I am local.<end_of_turn>\n==========\nPrompt: 5 tokens\nGeneration: 3 tokens\nPeak memory: 1 GB"
	if got := cleanOutput(in); got != "Hello there, I am local." {
		t.Errorf("cleanOutput = %q, want %q", got, "Hello there, I am local.")
	}
}

// TestAvailableFalseWhenMissing is the core of the AI-OPTIONAL contract: with no
// backend installed, Available() is false (callers skip enrichment, never block).
func TestAvailableFalseWhenMissing(t *testing.T) {
	c := Config{VenvPath: filepath.Join(t.TempDir(), "nope"), ModelID: "m"}
	if c.Available() {
		t.Error("Available() should be false when the venv/binary is absent")
	}
	// And Generate must refuse cleanly (not panic, not hang) when unavailable.
	if _, err := c.Generate(context.Background(), "hi"); err == nil {
		t.Error("Generate should error when backend unavailable")
	}
}

// TestGenerateUsesInjectedRunner proves Generate shells through the injectable
// seam and returns cleaned output — without ever spawning Python.
func TestGenerateUsesInjectedRunner(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "mlx_lm.generate"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := Config{VenvPath: dir, ModelID: "m", MaxTokens: 16}
	if !c.Available() {
		t.Fatal("Available() should be true once the script exists")
	}
	old := getRunFn()
	defer setRunFn(old)
	setRunFn(func(ctx context.Context, b string, args ...string) ([]byte, error) {
		return []byte("==========\nmocked reply<end_of_turn>\n=========="), nil
	})
	got, err := c.Generate(context.Background(), "hi")
	if err != nil || got != "mocked reply" {
		t.Errorf("Generate = %q, err=%v; want %q", got, err, "mocked reply")
	}
}

func TestLoadDefaultsWhenNoFile(t *testing.T) {
	c := DefaultConfig()
	if c.ModelID == "" || c.VenvPath == "" || c.MaxTokens == 0 {
		t.Errorf("defaults incomplete: %+v", c)
	}
}
