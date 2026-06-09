package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Runner is the injectable surface (Rule A16) between sirsi-gemma's MCP
// tools and the actual MLX subprocess. Tests pass a fake; production passes
// MLXRunner. Keeps the tool layer free of exec.Command, which would make
// unit tests require a live Python install.
type Runner interface {
	// Generate runs a single prompt → text generation. The prompt is the
	// fully-rendered string (chat templating is the caller's job — Gemma's
	// instruction template is applied by mlx_lm.generate automatically when
	// the model is an "-it" variant).
	Generate(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error)

	// Health is a 1-token sanity call. Used at startup to detect a broken
	// install before the operator's first real request hits an opaque error.
	Health(ctx context.Context) error
}

// MLXRunner shells out to `python -m mlx_lm.generate`. Each Generate call
// is its own subprocess — simple, mockable, no shared state. A future
// upgrade path is mlx_lm.server (OpenAI-compatible HTTP) for streaming;
// see cmd/sirsi-gemma/README.md.
type MLXRunner struct {
	cfg Config
}

func NewMLXRunner(cfg Config) *MLXRunner {
	return &MLXRunner{cfg: cfg}
}

func (r *MLXRunner) Generate(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	if maxTokens <= 0 {
		maxTokens = r.cfg.MaxTokens
	}
	if temperature <= 0 {
		temperature = r.cfg.Temperature
	}
	// Invoke chip A's canonical console script (<venv>/bin/mlx_lm.generate),
	// matching docs/setup/MLX_GEMMA_LOCAL.md + scripts/gemma-smoke.sh exactly,
	// rather than `python -m mlx_lm.generate` — no dependency on a python
	// symlink resolving inside the venv.
	genBin := filepath.Join(r.cfg.VenvPath, "bin", "mlx_lm.generate")
	args := []string{
		"--model", r.cfg.ModelID,
		"--prompt", prompt,
		"--max-tokens", strconv.Itoa(maxTokens),
		"--temp", strconv.FormatFloat(temperature, 'f', -1, 64),
	}
	cmd := exec.CommandContext(ctx, genBin, args...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("mlx_lm.generate exit %d: %s",
				exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("mlx_lm.generate: %w (venv=%s — see docs/setup/MLX_GEMMA_LOCAL.md)",
			err, r.cfg.VenvPath)
	}
	return stripMLXBanner(string(out)), nil
}

func (r *MLXRunner) Health(ctx context.Context) error {
	_, err := r.Generate(ctx, "ping", 1, 0.01)
	return err
}

// stripMLXBanner removes the "==========" framing mlx_lm.generate writes
// around its output (the text body sits between the two banner lines; the
// per-call stats — tokens/sec, peak memory — follow the closing banner and
// are dropped). It also trims Gemma's turn-terminator tokens so the MCP
// client sees clean text, not raw `<end_of_turn>`/`<eos>` markers.
func stripMLXBanner(out string) string {
	lines := strings.Split(out, "\n")
	var keep []string
	inBody := false
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "==========") {
			inBody = !inBody
			continue
		}
		if inBody {
			keep = append(keep, ln)
		}
	}
	body := strings.Join(keep, "\n")
	if len(keep) == 0 {
		body = out
	}
	return strings.TrimSpace(stripGemmaTokens(body))
}

// stripGemmaTokens removes Gemma's special turn/end markers that mlx_lm
// echoes into the generated text.
func stripGemmaTokens(s string) string {
	for _, tok := range []string{"<end_of_turn>", "<eos>", "<start_of_turn>model", "<start_of_turn>"} {
		s = strings.ReplaceAll(s, tok, "")
	}
	return s
}

// disabledRunner is used when the startup health probe fails. Every tool
// call returns the same actionable error pointing the operator at the
// setup doc, instead of the binary refusing to start. This keeps the MCP
// server discoverable from Claude Code even when Gemma is misconfigured.
type disabledRunner struct{ reason string }

func (d *disabledRunner) Generate(_ context.Context, _ string, _ int, _ float64) (string, error) {
	return "", fmt.Errorf("local MLX-Gemma not configured: %s — see ~/Development/sirsi-pantheon/docs/setup/MLX_GEMMA_LOCAL.md", d.reason)
}

func (d *disabledRunner) Health(_ context.Context) error {
	return fmt.Errorf("disabled: %s", d.reason)
}
