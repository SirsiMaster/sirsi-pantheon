package rules

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// baseOf returns the embedded baseScanRule of either a bare rule or an
// envGuardedRule, so structural assertions work on both shapes.
func baseOf(r jackal.ScanRule) *baseScanRule {
	switch v := r.(type) {
	case *envGuardedRule:
		return v.baseScanRule
	case *baseScanRule:
		return v
	}
	return nil
}

// TestAdditionalAICacheRulesGuarded locks the scan-truthfulness fix: the five
// expanded.go AI cache rules (ONNX, vLLM, JAX, Stable Diffusion, LangChain) must
// carry the 30-day mtime guard, or an actively-used cache is reported as waste.
func TestAdditionalAICacheRulesGuarded(t *testing.T) {
	t.Parallel()
	rules := map[string]jackal.ScanRule{
		"onnx":      NewOnnxCacheRule(),
		"vllm":      NewVLLMCacheRule(),
		"jax":       NewJaxCacheRule(),
		"sd":        NewStableDiffusionModelsRule(),
		"langchain": NewLangChainCacheRule(),
	}
	for name, r := range rules {
		base := baseOf(r)
		if base == nil {
			t.Fatalf("%s: unexpected rule type %T", name, r)
		}
		if base.minAgeDays != aiCacheMinAgeDays {
			t.Errorf("%s: minAgeDays = %d, want %d (unguarded cache would be reported as waste)",
				name, base.minAgeDays, aiCacheMinAgeDays)
		}
	}
}

// TestAICacheRulesWithPinVarAreEnvGuarded: the three rules whose runtimes expose
// a canonical cache-dir env var must additionally be env-guarded with that real
// var (we do NOT fabricate env vars for tools that lack one — A23).
func TestAICacheRulesWithPinVarAreEnvGuarded(t *testing.T) {
	t.Parallel()
	want := map[string]string{
		"onnx": "ORT_CACHE_DIR",
		"vllm": "VLLM_CACHE_ROOT",
		"jax":  "JAX_COMPILATION_CACHE_DIR",
	}
	rules := map[string]jackal.ScanRule{
		"onnx": NewOnnxCacheRule(),
		"vllm": NewVLLMCacheRule(),
		"jax":  NewJaxCacheRule(),
	}
	for name, r := range rules {
		g, ok := r.(*envGuardedRule)
		if !ok {
			t.Fatalf("%s: want *envGuardedRule, got %T", name, r)
		}
		found := false
		for _, v := range g.envVars {
			if v == want[name] {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: envVars %v missing canonical pin var %q", name, g.envVars, want[name])
		}
	}
}

// TestLiveTargetsFromEnv_TildeResolvesAbsolute locks the homeDir fix: a
// ~-prefixed env value (e.g. HF_HOME=~/.cache/huggingface, a common shell
// config) must resolve to an ABSOLUTE path. With an empty home it expanded to a
// relative path that never matched a finding, silently defeating the guard.
func TestLiveTargetsFromEnv_TildeResolvesAbsolute(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir")
	}
	lookup := func(k string) string {
		if k == "HF_HOME" {
			return "~/.cache/huggingface"
		}
		return ""
	}
	got := liveTargetsFromEnv([]string{"HF_HOME"}, lookup)
	wantPath := filepath.Clean(filepath.Join(home, ".cache", "huggingface"))
	if len(got) != 1 || got[0] != wantPath {
		t.Fatalf("liveTargetsFromEnv = %v, want [%q] (tilde must expand to absolute)", got, wantPath)
	}
}

// TestEnvGuardedRule_SuppressesPinnedPath: when a pin env var points at the
// scanned dir, the otherwise-cold cache is excluded from findings entirely.
func TestEnvGuardedRule_SuppressesPinnedPath(t *testing.T) {
	tmp := t.TempDir()
	cache := filepath.Join(tmp, ".cache", "vllm")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "kernels.bin"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -60) // comfortably past the 30-day guard
	os.Chtimes(cache, old, old)

	rule := &envGuardedRule{
		baseScanRule: &baseScanRule{
			name:       "vllm_cache",
			category:   jackal.CategoryAI,
			platforms:  []string{"darwin", "linux"},
			paths:      []string{cache},
			minAgeDays: aiCacheMinAgeDays,
		},
		envVars: []string{"VLLM_CACHE_ROOT"},
	}

	saved := getEnv
	defer func() { getEnv = saved }()

	// No pin set → the cold cache IS a finding.
	getEnv = func(string) string { return "" }
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error (no pin): %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected cold vLLM cache to be a finding when unpinned")
	}

	// Pin var points at the scanned dir → finding suppressed.
	getEnv = func(k string) string {
		if k == "VLLM_CACHE_ROOT" {
			return cache
		}
		return ""
	}
	findings, err = rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error (pinned): %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("pinned vLLM cache must be suppressed, got %d findings", len(findings))
	}
}

// TestSirsiGemmaLivePaths_ProtectsConfiguredModel is the A1 regression guard:
// a clean dry-run over a fixture HF cache that contains the configured model
// must report that model dir as a live path (skipped), not reclaimable.
// This is the exact failure that caused the Gemma substrate to be deleted:
// the rule had no conf-file guard, so a cold-mtime model was listed as waste.
func TestSirsiGemmaLivePaths_ProtectsConfiguredModel(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Write a realistic gemma-model.conf — one model ID per line.
	sirsiDir := filepath.Join(tmp, ".sirsi")
	if err := os.MkdirAll(sirsiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelID := "mlx-community/gemma-4-12B-it-qat-mxfp8"
	confPath := filepath.Join(sirsiDir, "gemma-model.conf")
	if err := os.WriteFile(confPath, []byte(modelID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Redirect readFileLines to the fixture sirsi dir so the test is hermetic.
	savedRead := readFileLines
	defer func() { readFileLines = savedRead }()
	readFileLines = func(path string) ([]string, error) {
		// Rewrite any ~/.sirsi/* path to point at the fixture dir.
		base := strings.TrimPrefix(filepath.Base(path), "")
		fixture := filepath.Join(sirsiDir, base)
		f, err := os.Open(fixture) //nolint:gosec
		if err != nil {
			return nil, err
		}
		defer f.Close()
		var lines []string
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
				lines = append(lines, line)
			}
		}
		return lines, sc.Err()
	}

	got := sirsiGemmaLivePaths(tmp)
	wantDir := filepath.Join(tmp, ".cache", "huggingface", "hub", "models--mlx-community--gemma-4-12B-it-qat-mxfp8")
	found := false
	for _, p := range got {
		if p == wantDir {
			found = true
		}
	}
	if !found {
		t.Errorf("sirsiGemmaLivePaths = %v, want to include %q — configured model must be a live path", got, wantDir)
	}

	// Regression: cold model dir under HF hub with mtime >30 days ago must
	// be suppressed when it IS the configured model.
	hubDir := filepath.Join(tmp, ".cache", "huggingface", "hub")
	modelDir := filepath.Join(hubDir, "models--mlx-community--gemma-4-12B-it-qat-mxfp8")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.safetensors"), make([]byte, 1024), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stamp mtime 60 days ago — past both the 30-day guard AND the conf guard would be needed.
	old := time.Now().AddDate(0, 0, -60)
	os.Chtimes(modelDir, old, old)

	savedGetEnv := getEnv
	defer func() { getEnv = savedGetEnv }()
	getEnv = func(string) string { return "" } // no env var pins

	rule := NewHuggingFaceCacheRule().(*envGuardedRule)
	rule.baseScanRule.paths = []string{hubDir} // scan only our fixture hub
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	for _, f := range findings {
		if hasPrefixPath(f.Path, modelDir) || f.Path == modelDir {
			t.Errorf("configured model %q surfaced as a finding — A1 violation: sirsi clean would delete its own substrate", f.Path)
		}
	}
}
