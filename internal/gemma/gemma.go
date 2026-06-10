// Package gemma is the OPTIONAL local-AI backend for Pantheon.
//
// Pantheon's core — scan, clean, diagnose, insight — is fully deterministic and
// NEVER requires this package. gemma only ENRICHES output with natural language
// when a local MLX Gemma install happens to be present. Every entry point is
// gated by Available(); the absence of a backend is a normal, expected state,
// not an error. This is the "operate without AI, include AI if present" contract.
package gemma

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultModelID = "mlx-community/gemma-2-27b-it-bf16-4bit"
	defaultVenvRel = ".venvs/mlx"
	defaultMaxTok  = 256
)

// Config locates the local Gemma runtime. Defaults match chip A's install; a
// ~/.config/sirsi/gemma.toml overrides any field.
type Config struct {
	ModelID   string
	VenvPath  string
	MaxTokens int
}

func home() string { h, _ := os.UserHomeDir(); return h }

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home(), p[2:])
	}
	return p
}

// DefaultConfig returns the zero-config defaults (no file needed).
func DefaultConfig() Config {
	return Config{ModelID: defaultModelID, VenvPath: filepath.Join(home(), defaultVenvRel), MaxTokens: defaultMaxTok}
}

// Load reads ~/.config/sirsi/gemma.toml when present; missing file or keys keep
// the defaults. Never errors — a malformed/absent config just means defaults.
func Load() Config {
	c := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(home(), ".config", "sirsi", "gemma.toml"))
	if err != nil {
		return c
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch k {
		case "model_id":
			c.ModelID = v
		case "venv_path":
			c.VenvPath = expandHome(v)
		case "max_tokens":
			if n, err := strconv.Atoi(v); err == nil {
				c.MaxTokens = n
			}
		}
	}
	return c
}

func (c Config) genBin() string { return filepath.Join(c.VenvPath, "bin", "mlx_lm.generate") }

// Available reports whether the local Gemma backend can run — i.e. the
// mlx_lm.generate console script exists in the configured venv. A cheap stat,
// no model load. Callers MUST gate every Generate on this.
func (c Config) Available() bool {
	info, err := os.Stat(c.genBin())
	return err == nil && !info.IsDir()
}

// runFn is the injectable exec seam (Rule A16), guarded by a mutex (Rule A21)
// so swapping it in tests never races a concurrent Generate.
var (
	runMu sync.RWMutex
	runFn = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, bin, args...).Output()
	}
)

func getRunFn() func(context.Context, string, ...string) ([]byte, error) {
	runMu.RLock()
	defer runMu.RUnlock()
	return runFn
}

func setRunFn(fn func(context.Context, string, ...string) ([]byte, error)) {
	runMu.Lock()
	defer runMu.Unlock()
	runFn = fn
}

// Generate runs a single one-shot completion against the local model. The caller
// MUST have checked Available(); on any failure it returns an error that the
// caller treats as "no enrichment available" — never fatal to the deterministic
// path that produced the prompt.
func (c Config) Generate(ctx context.Context, prompt string) (string, error) {
	if !c.Available() {
		return "", fmt.Errorf("gemma backend not available at %s", c.genBin())
	}
	mt := c.MaxTokens
	if mt <= 0 {
		mt = defaultMaxTok
	}
	out, err := getRunFn()(ctx, c.genBin(), "--model", c.ModelID, "--max-tokens", strconv.Itoa(mt), "--prompt", prompt)
	if err != nil {
		return "", fmt.Errorf("gemma generate: %w", err)
	}
	return cleanOutput(string(out)), nil
}

// cleanOutput strips the mlx_lm.generate "====" framing and Gemma's turn tokens.
func cleanOutput(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "====") || strings.HasPrefix(t, "Prompt:") ||
			strings.HasPrefix(t, "Generation:") || strings.HasPrefix(t, "Peak memory:") ||
			strings.HasPrefix(t, "Fetching ") {
			continue
		}
		keep = append(keep, line)
	}
	out := strings.Join(keep, "\n")
	for _, tok := range []string{"<end_of_turn>", "<start_of_turn>", "<eos>", "model\n"} {
		out = strings.ReplaceAll(out, tok, "")
	}
	return strings.TrimSpace(out)
}
