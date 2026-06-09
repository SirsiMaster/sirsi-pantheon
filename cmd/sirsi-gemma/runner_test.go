package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records calls and returns scripted answers. Used wherever a
// test needs to assert tool wiring without spawning Python.
type fakeRunner struct {
	healthErr  error
	genFunc    func(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error)
	lastPrompt string
	lastMaxTok int
	lastTemp   float64
	calls      int
}

func (f *fakeRunner) Generate(ctx context.Context, prompt string, maxTokens int, temperature float64) (string, error) {
	f.calls++
	f.lastPrompt, f.lastMaxTok, f.lastTemp = prompt, maxTokens, temperature
	if f.genFunc != nil {
		return f.genFunc(ctx, prompt, maxTokens, temperature)
	}
	return "ok", nil
}

func (f *fakeRunner) Health(_ context.Context) error { return f.healthErr }

func TestLoadConfig_MissingFileReturnsDefaults(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if cfg.ModelID == "" || cfg.MaxTokens == 0 {
		t.Fatalf("defaults not populated: %+v", cfg)
	}
}

func TestLoadConfig_OverlaysValues(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gemma.toml")
	body := `
# comment
model_id = "my-model"
venv_path = "/tmp/v"
max_tokens = 256
temperature = 0.2
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ModelID != "my-model" || cfg.VenvPath != "/tmp/v" || cfg.MaxTokens != 256 || cfg.Temperature != 0.2 {
		t.Fatalf("overlay failed: %+v", cfg)
	}
}

func TestLoadConfig_UnknownKeyFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "gemma.toml")
	_ = os.WriteFile(p, []byte("bogus = 1\n"), 0o644)
	if _, err := LoadConfig(p); err == nil {
		t.Fatal("expected unknown-key error")
	}
}

func TestRenderChatPrompt_BasicTemplate(t *testing.T) {
	out, err := RenderChatPrompt("be terse", []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "again"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"be terse", "<start_of_turn>user", "<start_of_turn>assistant", "<start_of_turn>model"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in rendered prompt:\n%s", want, out)
		}
	}
}

func TestRenderChatPrompt_RejectsEmptyAndBadRoles(t *testing.T) {
	if _, err := RenderChatPrompt("", nil); err == nil {
		t.Error("empty history should error")
	}
	_, err := RenderChatPrompt("", []ChatMessage{{Role: "narrator", Content: "x"}})
	if err == nil {
		t.Error("unknown role should error")
	}
}

func TestChatHandler_DispatchesToRunner(t *testing.T) {
	fr := &fakeRunner{genFunc: func(_ context.Context, _ string, _ int, _ float64) (string, error) {
		return "world", nil
	}}
	h := makeChatHandler(fr)
	res, err := h(map[string]any{
		"system": "sys",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"max_tokens":  float64(42),
		"temperature": float64(0.3),
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Content[0].Text)
	}
	if res.Content[0].Text != "world" {
		t.Errorf("want world, got %q", res.Content[0].Text)
	}
	if fr.lastMaxTok != 42 || fr.lastTemp != 0.3 {
		t.Errorf("args not forwarded: max=%d temp=%f", fr.lastMaxTok, fr.lastTemp)
	}
	if !strings.Contains(fr.lastPrompt, "sys") {
		t.Error("system prompt not rendered into final prompt")
	}
}

func TestChatHandler_MissingMessagesReturnsErrorResult(t *testing.T) {
	h := makeChatHandler(&fakeRunner{})
	res, err := h(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
}

func TestCompleteHandler_DispatchesToRunner(t *testing.T) {
	fr := &fakeRunner{genFunc: func(_ context.Context, p string, _ int, _ float64) (string, error) {
		return "echo:" + p, nil
	}}
	h := makeCompleteHandler(fr)
	res, err := h(map[string]any{"prompt": "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Content[0].Text != "echo:ping" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestCompleteHandler_EmptyPromptIsError(t *testing.T) {
	h := makeCompleteHandler(&fakeRunner{})
	res, _ := h(map[string]any{"prompt": ""})
	if !res.IsError {
		t.Fatal("expected error result for empty prompt")
	}
}

func TestDisabledRunner_SurfacesActionableMessage(t *testing.T) {
	d := &disabledRunner{reason: "venv missing"}
	_, err := d.Generate(context.Background(), "x", 0, 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "MLX_GEMMA_LOCAL.md") {
		t.Errorf("error should point to setup doc, got: %v", err)
	}
}

func TestStripMLXBanner(t *testing.T) {
	in := "loading...\n==========\nhello world\n==========\nstats line"
	if got := stripMLXBanner(in); got != "hello world" {
		t.Errorf("got %q", got)
	}
	if got := stripMLXBanner("just text"); got != "just text" {
		t.Errorf("fallback failed: %q", got)
	}
	// Real mlx_lm.generate output: body carries Gemma's <end_of_turn> token,
	// stats follow the closing banner and must be dropped.
	real := "==========\nPONG<end_of_turn>\n\n==========\nPrompt: 15 tokens, 28.8 tokens-per-sec\nPeak memory: 15.5 GB"
	if got := stripMLXBanner(real); got != "PONG" {
		t.Errorf("real-output strip failed: got %q, want PONG", got)
	}
}

func TestStripGemmaTokens(t *testing.T) {
	if got := stripGemmaTokens("hi<end_of_turn>"); got != "hi" {
		t.Errorf("got %q", got)
	}
	if got := stripGemmaTokens("<start_of_turn>model\nanswer<eos>"); got != "\nanswer" {
		t.Errorf("got %q", got)
	}
}

func TestSelectRunner_FallsBackToDisabledOnHealthFail(t *testing.T) {
	// Production path: MLXRunner.Health will fail because there's no python
	// at the configured venv. selectRunner should swap to disabledRunner so
	// the MCP loop still serves and returns actionable errors.
	cfg := DefaultConfig()
	cfg.VenvPath = "/definitely/not/a/path"
	r := selectRunner(cfg, false, nullLogger())
	if _, ok := r.(*disabledRunner); !ok {
		// On a machine where chip A's install actually exists, the probe
		// could succeed — accept that too, just verify the surface.
		if err := r.Health(context.Background()); err != nil {
			t.Errorf("got non-disabled runner whose Health still errors: %v", err)
		}
	}
}

// nullLogger silences selectRunner during tests.
func nullLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// Compile-time sanity: ensure errors.As is referenced (keeps the import
// from drifting unused if runner.go changes).
var _ = errors.As
