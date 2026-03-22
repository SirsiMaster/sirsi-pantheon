package rules

import (
	"testing"

	"github.com/SirsiMaster/sirsi-anubis/internal/jackal"
)

// ─────────────────────────────────────────────────
// AllRules registration
// ─────────────────────────────────────────────────

func TestAllRules_ReturnsNonEmpty(t *testing.T) {
	rules := AllRules()
	if len(rules) == 0 {
		t.Fatal("AllRules() returned 0 rules — cross-platform rules should always be present")
	}
}

func TestAllRules_CrossPlatformMinimum(t *testing.T) {
	// Cross-platform rules always register regardless of OS.
	// registry.go shows 29 cross-platform rules.
	cp := crossPlatformRules()
	if len(cp) < 29 {
		t.Errorf("expected at least 29 cross-platform rules, got %d", len(cp))
	}
}

func TestAllRules_UniqueNames(t *testing.T) {
	rules := AllRules()
	seen := make(map[string]bool)
	for _, r := range rules {
		name := r.Name()
		if seen[name] {
			t.Errorf("duplicate rule name: %q", name)
		}
		seen[name] = true
	}
}

func TestAllRules_AllHaveRequiredFields(t *testing.T) {
	rules := AllRules()
	for _, r := range rules {
		if r.Name() == "" {
			t.Error("found rule with empty Name()")
		}
		if r.DisplayName() == "" {
			t.Errorf("rule %q has empty DisplayName()", r.Name())
		}
		if r.Category() == "" {
			t.Errorf("rule %q has empty Category()", r.Name())
		}
		if r.Description() == "" {
			t.Errorf("rule %q has empty Description()", r.Name())
		}
		if len(r.Platforms()) == 0 {
			t.Errorf("rule %q has no Platforms()", r.Name())
		}
	}
}

// ─────────────────────────────────────────────────
// Category distribution
// ─────────────────────────────────────────────────

func TestAllRules_CategoryDistribution(t *testing.T) {
	rules := AllRules()
	categories := make(map[jackal.Category]int)
	for _, r := range rules {
		categories[r.Category()]++
	}

	// Must have rules in all 7 categories (cross-platform at minimum)
	expectedCategories := []jackal.Category{
		jackal.CategoryDev,
		jackal.CategoryAI,
		jackal.CategoryIDEs,
		jackal.CategoryCloud,
	}

	for _, cat := range expectedCategories {
		if categories[cat] == 0 {
			t.Errorf("no rules registered for category %q", cat)
		}
	}
}

func TestAllRules_ValidCategories(t *testing.T) {
	validCategories := map[jackal.Category]bool{
		jackal.CategoryGeneral:        true,
		jackal.CategoryVirtualization: true,
		jackal.CategoryDev:            true,
		jackal.CategoryAI:             true,
		jackal.CategoryIDEs:           true,
		jackal.CategoryCloud:          true,
		jackal.CategoryStorage:        true,
	}

	rules := AllRules()
	for _, r := range rules {
		if !validCategories[r.Category()] {
			t.Errorf("rule %q has invalid category: %q", r.Name(), r.Category())
		}
	}
}

// ─────────────────────────────────────────────────
// Platform filtering
// ─────────────────────────────────────────────────

func TestAllRules_ValidPlatforms(t *testing.T) {
	validPlatforms := map[string]bool{
		"darwin":  true,
		"linux":   true,
		"windows": true,
	}

	rules := AllRules()
	for _, r := range rules {
		for _, p := range r.Platforms() {
			if !validPlatforms[p] {
				t.Errorf("rule %q has invalid platform: %q", r.Name(), p)
			}
		}
	}
}

// ─────────────────────────────────────────────────
// Individual rule constructors — smoke tests
// ─────────────────────────────────────────────────

func TestCrossPlatformRules_Constructors(t *testing.T) {
	constructors := map[string]func() jackal.ScanRule{
		"node_modules":       NewNodeModulesRule,
		"go_mod_cache":       NewGoModCacheRule,
		"python_caches":      NewPythonCachesRule,
		"rust_targets":       NewRustTargetRule,
		"docker_desktop":     NewDockerRule,
		"npm_global_cache":   NewNpmGlobalCacheRule,
		"gradle_cache":       NewGradleCacheRule,
		"maven_cache":        NewMavenCacheRule,
		"composer_cache":     NewComposerCacheRule,
		"rubygems_cache":     NewRubyGemsCacheRule,
		"huggingface_cache":  NewHuggingFaceCacheRule,
		"ollama_models":      NewOllamaModelsRule,
		"pytorch_cache":      NewPyTorchCacheRule,
		"tensorflow_cache":   NewTensorFlowCacheRule,
		"onnx_cache":         NewOnnxCacheRule,
		"vllm_cache":         NewVLLMCacheRule,
		"jax_cache":          NewJaxCacheRule,
		"stable_diffusion":   NewStableDiffusionModelsRule,
		"langchain_cache":    NewLangChainCacheRule,
		"claude_code":        NewClaudeCodeRule,
		"gemini_cli":         NewGeminiCLIRule,
		"neovim_caches":      NewNeovimCacheRule,
		"codex_cli":          NewCodexCLIRule,
		"kubernetes_caches":  NewKubernetesCachesRule,
		"terraform_providers": NewTerraformCachesRule,
		"gcloud_caches":      NewGCloudCachesRule,
		"firebase_caches":    NewFirebaseCachesRule,
		"nginx_logs":         NewNginxLogsRule,
		"aws_cli_cache":      NewAWSCLICacheRule,
	}

	for expectedName, constructor := range constructors {
		rule := constructor()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", expectedName)
			continue
		}
		if rule.Name() != expectedName {
			t.Errorf("constructor for %q returned rule with Name() = %q", expectedName, rule.Name())
		}
	}
}

// ─────────────────────────────────────────────────
// Rule count verification (the 64-rule claim)
// ─────────────────────────────────────────────────

func TestDarwinRules_Count(t *testing.T) {
	// registry.go: 9 general + 4 virt + 2 AI + 7 IDEs + 3 pkg + 4 cloud = 29
	rules := darwinRules()
	if len(rules) < 29 {
		t.Errorf("expected at least 29 darwin rules, got %d", len(rules))
	}
}

func TestLinuxRules_Empty(t *testing.T) {
	// Currently returns empty (TODO in code)
	rules := linuxRules()
	if rules == nil {
		t.Error("linuxRules() should return empty slice, not nil")
	}
}
