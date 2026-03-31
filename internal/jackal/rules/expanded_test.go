package rules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ── Mock Manifest for Horus branch testing ──────────────────────────────

type mockManifest struct {
	dirs map[string]struct {
		size  int64
		count int
	}
	files    map[string]bool
	globs    map[string][]string
	findDirs map[string][]string
}

func (m *mockManifest) DirSizeAndCount(dir string) (int64, int) {
	if entry, ok := m.dirs[dir]; ok {
		return entry.size, entry.count
	}
	return 0, 0
}

func (m *mockManifest) DirSize(dir string) int64 {
	s, _ := m.DirSizeAndCount(dir)
	return s
}

func (m *mockManifest) Exists(path string) bool {
	return m.files[path]
}

func (m *mockManifest) Glob(pattern string) []string {
	return m.globs[pattern]
}

func (m *mockManifest) FindDirsNamed(root, name string, maxDepth int) []string {
	key := root + "/" + name
	return m.findDirs[key]
}

// ── AI/ML Rule Constructors (ai.go + expanded.go) ──────────────────────

func TestAIRules_AllConstructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]struct {
		fn       func() jackal.ScanRule
		category jackal.Category
	}{
		"huggingface_cache": {NewHuggingFaceCacheRule, jackal.CategoryAI},
		"ollama_models":     {NewOllamaModelsRule, jackal.CategoryAI},
		"pytorch_cache":     {NewPyTorchCacheRule, jackal.CategoryAI},
		"tensorflow_cache":  {NewTensorFlowCacheRule, jackal.CategoryAI},
		"mlx_cache":         {NewMLXCacheRule, jackal.CategoryAI},
		"metal_shaders":     {NewMetalShaderCacheRule, jackal.CategoryAI},
		"onnx_cache":        {NewOnnxCacheRule, jackal.CategoryAI},
		"vllm_cache":        {NewVLLMCacheRule, jackal.CategoryAI},
		"jax_cache":         {NewJaxCacheRule, jackal.CategoryAI},
		"stable_diffusion":  {NewStableDiffusionModelsRule, jackal.CategoryAI},
		"langchain_cache":   {NewLangChainCacheRule, jackal.CategoryAI},
	}
	for name, tc := range constructors {
		rule := tc.fn()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.DisplayName() == "" {
			t.Errorf("%q: empty DisplayName", name)
		}
		if rule.Category() != tc.category {
			t.Errorf("%q: Category() = %q, want %q", name, rule.Category(), tc.category)
		}
		if rule.Description() == "" {
			t.Errorf("%q: empty Description", name)
		}
		if len(rule.Platforms()) == 0 {
			t.Errorf("%q: no platforms", name)
		}
	}
}

// ── IDE Rule Constructors (ides_cloud.go + expanded.go) ─────────────────

func TestIDERules_AllConstructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]struct {
		fn       func() jackal.ScanRule
		category jackal.Category
	}{
		"vscode_caches":    {NewVSCodeCachesRule, jackal.CategoryIDEs},
		"jetbrains_caches": {NewJetBrainsCachesRule, jackal.CategoryIDEs},
		"claude_code":      {NewClaudeCodeRule, jackal.CategoryIDEs},
		"gemini_cli":       {NewGeminiCLIRule, jackal.CategoryIDEs},
		"android_studio":   {NewAndroidStudioRule, jackal.CategoryIDEs},
		"cursor_caches":    {NewCursorCacheRule, jackal.CategoryIDEs},
		"windsurf_caches":  {NewWindsurfCacheRule, jackal.CategoryIDEs},
		"zed_caches":       {NewZedCacheRule, jackal.CategoryIDEs},
		"neovim_caches":    {NewNeovimCacheRule, jackal.CategoryIDEs},
		"codex_cli":        {NewCodexCLIRule, jackal.CategoryIDEs},
	}
	for name, tc := range constructors {
		rule := tc.fn()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.DisplayName() == "" {
			t.Errorf("%q: empty DisplayName", name)
		}
		if rule.Category() != tc.category {
			t.Errorf("%q: Category() = %q, want %q", name, rule.Category(), tc.category)
		}
	}
}

// ── Cloud/Infra Rule Constructors (ides_cloud.go + expanded.go) ─────────

func TestCloudRules_AllConstructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]struct {
		fn       func() jackal.ScanRule
		category jackal.Category
	}{
		"kubernetes_caches":   {NewKubernetesCachesRule, jackal.CategoryCloud},
		"terraform_providers": {NewTerraformCachesRule, jackal.CategoryCloud},
		"gcloud_caches":       {NewGCloudCachesRule, jackal.CategoryCloud},
		"firebase_caches":     {NewFirebaseCachesRule, jackal.CategoryCloud},
		"nginx_logs":          {NewNginxLogsRule, jackal.CategoryCloud},
		"aws_cli_cache":       {NewAWSCLICacheRule, jackal.CategoryCloud},
	}
	for name, tc := range constructors {
		rule := tc.fn()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.Category() != tc.category {
			t.Errorf("%q: Category() = %q, want %q", name, rule.Category(), tc.category)
		}
		if rule.Description() == "" {
			t.Errorf("%q: empty Description", name)
		}
	}
}

// ── Package Manager Rule Constructors (expanded.go) ─────────────────────

func TestPackageManagerRules_AllConstructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]struct {
		fn       func() jackal.ScanRule
		category jackal.Category
	}{
		"npm_global_cache": {NewNpmGlobalCacheRule, jackal.CategoryDev},
		"gradle_cache":     {NewGradleCacheRule, jackal.CategoryDev},
		"maven_cache":      {NewMavenCacheRule, jackal.CategoryDev},
		"composer_cache":   {NewComposerCacheRule, jackal.CategoryDev},
		"rubygems_cache":   {NewRubyGemsCacheRule, jackal.CategoryDev},
		"homebrew_cache":   {NewHomebrewCacheRule, jackal.CategoryDev},
		"cocoapods_cache":  {NewCocoaPodsCacheRule, jackal.CategoryDev},
		"spm_cache":        {NewSPMCacheRule, jackal.CategoryDev},
	}
	for name, tc := range constructors {
		rule := tc.fn()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.Category() != tc.category {
			t.Errorf("%q: Category() = %q, want %q", name, rule.Category(), tc.category)
		}
	}
}

// ── Storage Rule Constructors (ides_cloud.go + expanded.go) ──────────────

func TestStorageRules_AllConstructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]struct {
		fn       func() jackal.ScanRule
		category jackal.Category
	}{
		"onedrive_cache":     {NewOneDriveCacheRule, jackal.CategoryStorage},
		"google_drive_cache": {NewGoogleDriveCacheRule, jackal.CategoryStorage},
		"dropbox_cache":      {NewDropboxCacheRule, jackal.CategoryStorage},
		"icloud_cache":       {NewICloudCacheRule, jackal.CategoryStorage},
	}
	for name, tc := range constructors {
		rule := tc.fn()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.Category() != tc.category {
			t.Errorf("%q: Category() = %q, want %q", name, rule.Category(), tc.category)
		}
	}
}

// ── Scan-level tests: exercise actual rule scanning with temp dirs ───────

func TestHuggingFaceRule_ScanFindsCache(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, ".cache", "huggingface", "hub")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "model.bin"), make([]byte, 4096), 0o644)

	rule := &baseScanRule{
		name:      "huggingface_cache",
		category:  jackal.CategoryAI,
		platforms: []string{"darwin", "linux"},
		paths:     []string{filepath.Join(tmp, ".cache", "huggingface", "hub")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].SizeBytes < 4096 {
		t.Errorf("expected >= 4096 bytes, got %d", findings[0].SizeBytes)
	}
}

func TestOllamaRule_ScanFindsModels(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	modelDir := filepath.Join(tmp, ".ollama", "models")
	os.MkdirAll(modelDir, 0o755)
	os.WriteFile(filepath.Join(modelDir, "llama3.gguf"), make([]byte, 8192), 0o644)

	rule := &baseScanRule{
		name:      "ollama_models",
		category:  jackal.CategoryAI,
		platforms: []string{"darwin", "linux"},
		paths:     []string{filepath.Join(tmp, ".ollama", "models")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestClaudeCodeRule_ScanFindsLogs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	logDir := filepath.Join(tmp, ".claude", "logs")
	os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, "session.log")
	os.WriteFile(logFile, make([]byte, 2048), 0o644)
	// Set old timestamp to pass minAgeDays
	oldTime := time.Now().AddDate(0, 0, -30)
	os.Chtimes(logDir, oldTime, oldTime)

	rule := &baseScanRule{
		name:       "claude_code",
		category:   jackal.CategoryIDEs,
		platforms:  []string{"darwin", "linux"},
		paths:      []string{filepath.Join(tmp, ".claude", "logs")},
		minAgeDays: 14,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding for old claude logs, got %d", len(findings))
	}
}

func TestGeminiCLIRule_ExcludesSkills(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, ".gemini", "cache")
	skillsDir := filepath.Join(tmp, ".gemini", "antigravity", "skills")
	os.MkdirAll(cacheDir, 0o755)
	os.MkdirAll(skillsDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "data.bin"), make([]byte, 1024), 0o644)
	os.WriteFile(filepath.Join(skillsDir, "skill.js"), make([]byte, 512), 0o644)

	rule := &baseScanRule{
		name:       "gemini_cli",
		category:   jackal.CategoryIDEs,
		platforms:  []string{"darwin", "linux"},
		paths:      []string{filepath.Join(tmp, ".gemini", "cache")},
		excludes:   []string{filepath.Join(tmp, ".gemini", "antigravity", "skills")},
		minAgeDays: 14,
	}

	// Cache dir is new, so minAgeDays should filter it out
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	// Verify skills dir is not in findings even without age filter
	for _, f := range findings {
		if f.Path == skillsDir {
			t.Error("skills dir should be excluded")
		}
	}
}

func TestKubernetesRule_ScanFindsCaches(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	kubeCache := filepath.Join(tmp, ".kube", "cache")
	os.MkdirAll(kubeCache, 0o755)
	os.WriteFile(filepath.Join(kubeCache, "discovery.json"), make([]byte, 2048), 0o644)

	rule := &baseScanRule{
		name:      "kubernetes_caches",
		category:  jackal.CategoryCloud,
		platforms: []string{"darwin", "linux"},
		paths:     []string{filepath.Join(tmp, ".kube", "cache")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestFirebaseRule_ScanFindsCaches(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	fbCache := filepath.Join(tmp, ".cache", "firebase")
	os.MkdirAll(fbCache, 0o755)
	os.WriteFile(filepath.Join(fbCache, "emulator.jar"), make([]byte, 4096), 0o644)

	rule := &baseScanRule{
		name:      "firebase_caches",
		category:  jackal.CategoryCloud,
		platforms: []string{"darwin", "linux"},
		paths:     []string{filepath.Join(tmp, ".cache", "firebase")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestGCloudRule_ScanFindsLogs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	logsDir := filepath.Join(tmp, ".config", "gcloud", "logs")
	os.MkdirAll(logsDir, 0o755)
	os.WriteFile(filepath.Join(logsDir, "2026.03.31.log"), make([]byte, 1024), 0o644)
	// Set old timestamp
	oldTime := time.Now().AddDate(0, 0, -14)
	os.Chtimes(logsDir, oldTime, oldTime)

	rule := &baseScanRule{
		name:       "gcloud_caches",
		category:   jackal.CategoryCloud,
		platforms:  []string{"darwin", "linux"},
		paths:      []string{filepath.Join(tmp, ".config", "gcloud", "logs")},
		minAgeDays: 7,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestAWSCLIRule_ScanFindsSSOCache(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	ssoDir := filepath.Join(tmp, ".aws", "sso", "cache")
	os.MkdirAll(ssoDir, 0o755)
	os.WriteFile(filepath.Join(ssoDir, "token.json"), make([]byte, 512), 0o644)
	oldTime := time.Now().AddDate(0, 0, -14)
	os.Chtimes(ssoDir, oldTime, oldTime)

	rule := &baseScanRule{
		name:       "aws_cli_cache",
		category:   jackal.CategoryCloud,
		platforms:  []string{"darwin", "linux"},
		paths:      []string{filepath.Join(tmp, ".aws", "sso", "cache")},
		minAgeDays: 7,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

// ── findRule Scan tests ─────────────────────────────────────────────────

func TestFindRule_ScanFindsTargetDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "projects", "myapp")
	nodeModules := filepath.Join(projectDir, "node_modules")
	os.MkdirAll(nodeModules, 0o755)
	os.WriteFile(filepath.Join(nodeModules, "lodash.js"), make([]byte, 2048), 0o644)
	// Set old mod time
	oldTime := time.Now().AddDate(0, 0, -30)
	os.Chtimes(nodeModules, oldTime, oldTime)

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{filepath.Join(tmp, "projects")},
		maxDepth:    4,
		minAgeDays:  14,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Path != nodeModules {
		t.Errorf("expected path %q, got %q", nodeModules, findings[0].Path)
	}
}

func TestFindRule_FindsRecentDirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "projects", "active")
	nodeModules := filepath.Join(projectDir, "node_modules")
	os.MkdirAll(nodeModules, 0o755)
	os.WriteFile(filepath.Join(nodeModules, "react.js"), make([]byte, 1024), 0o644)

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{filepath.Join(tmp, "projects")},
		maxDepth:    4,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	// findRule filesystem walk finds all matching dirs regardless of age
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestFindRule_MatchFileFilter(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create a Rust project with target dir
	rustProject := filepath.Join(tmp, "projects", "mylib")
	targetDir := filepath.Join(rustProject, "target")
	os.MkdirAll(targetDir, 0o755)
	os.WriteFile(filepath.Join(targetDir, "build.o"), make([]byte, 4096), 0o644)
	os.WriteFile(filepath.Join(rustProject, "Cargo.toml"), []byte("[package]\nname = \"mylib\"\n"), 0o644)
	oldTime := time.Now().AddDate(0, 0, -14)
	os.Chtimes(targetDir, oldTime, oldTime)

	// Create a non-Rust dir with "target" (should be skipped)
	nonRust := filepath.Join(tmp, "projects", "other")
	fakeTarget := filepath.Join(nonRust, "target")
	os.MkdirAll(fakeTarget, 0o755)
	os.WriteFile(filepath.Join(fakeTarget, "data.bin"), make([]byte, 1024), 0o644)
	os.Chtimes(fakeTarget, oldTime, oldTime)

	rule := &findRule{
		name:        "rust_targets",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "target",
		searchPaths: []string{filepath.Join(tmp, "projects")},
		maxDepth:    3,
		minAgeDays:  7,
		matchFile:   "Cargo.toml",
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (Rust only), got %d", len(findings))
	}
	if findings[0].Path != targetDir {
		t.Errorf("expected path %q, got %q", targetDir, findings[0].Path)
	}
}

func TestFindRule_SkipsNonExistentSearchPaths(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{filepath.Join(tmp, "nonexistent")},
		maxDepth:    4,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(findings))
	}
}

func TestFindRule_RespectsMaxDepth(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create deeply nested target — beyond maxDepth
	deep := filepath.Join(tmp, "projects", "a", "b", "c", "d", "node_modules")
	os.MkdirAll(deep, 0o755)
	os.WriteFile(filepath.Join(deep, "pkg.js"), make([]byte, 512), 0o644)
	oldTime := time.Now().AddDate(0, 0, -30)
	os.Chtimes(deep, oldTime, oldTime)

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{filepath.Join(tmp, "projects")},
		maxDepth:    2, // a/b is depth 2, c/d is depth 4
		minAgeDays:  7,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (too deep), got %d", len(findings))
	}
}

func TestFindRule_CleanDryRun(t *testing.T) {
	t.Parallel()
	rule := &findRule{name: "test_find"}
	findings := []jackal.Finding{
		{Path: "/nonexistent/node_modules", SizeBytes: 1024},
	}
	result, err := rule.Clean(context.Background(), findings, jackal.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}
	if result == nil {
		t.Fatal("Clean returned nil result")
	}
}

func TestFindRule_Accessors(t *testing.T) {
	t.Parallel()
	rule := &findRule{
		name:        "test_find",
		displayName: "Test Find Rule",
		category:    jackal.CategoryDev,
		description: "A test find rule",
		platforms:   []string{"darwin"},
	}
	if rule.Name() != "test_find" {
		t.Errorf("Name() = %q", rule.Name())
	}
	if rule.DisplayName() != "Test Find Rule" {
		t.Errorf("DisplayName() = %q", rule.DisplayName())
	}
	if rule.Category() != jackal.CategoryDev {
		t.Errorf("Category() = %q", rule.Category())
	}
	if rule.Description() != "A test find rule" {
		t.Errorf("Description() = %q", rule.Description())
	}
	if len(rule.Platforms()) != 1 {
		t.Errorf("Platforms() len = %d", len(rule.Platforms()))
	}
}

// ── Scan-level tests for multiple real rule paths ───────────────────────

func TestBaseScanRule_ScanWithHomeExpansion(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Simulate ~/.cache/torch structure
	torchDir := filepath.Join(tmp, ".cache", "torch")
	os.MkdirAll(torchDir, 0o755)
	os.WriteFile(filepath.Join(torchDir, "model.pt"), make([]byte, 2048), 0o644)

	rule := &baseScanRule{
		name:      "pytorch_cache",
		category:  jackal.CategoryAI,
		platforms: []string{"darwin", "linux"},
		paths:     []string{"~/.cache/torch"},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding via ~ expansion, got %d", len(findings))
	}
}

func TestBaseScanRule_ScanMultiplePaths(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create two matching paths (simulating gcloud rule)
	logsDir := filepath.Join(tmp, ".config", "gcloud", "logs")
	cacheDir := filepath.Join(tmp, ".config", "gcloud", "cache")
	os.MkdirAll(logsDir, 0o755)
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(logsDir, "log.txt"), make([]byte, 256), 0o644)
	os.WriteFile(filepath.Join(cacheDir, "data.bin"), make([]byte, 512), 0o644)

	rule := &baseScanRule{
		name:      "gcloud_caches",
		category:  jackal.CategoryCloud,
		platforms: []string{"darwin", "linux"},
		paths: []string{
			filepath.Join(tmp, ".config", "gcloud", "logs"),
			filepath.Join(tmp, ".config", "gcloud", "cache"),
		},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 2 {
		t.Errorf("expected 2 findings from 2 paths, got %d", len(findings))
	}
}

func TestBaseScanRule_Clean_RealFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	targetFile := filepath.Join(tmp, "deleteme.dat")
	os.WriteFile(targetFile, make([]byte, 1024), 0o644)

	rule := &baseScanRule{name: "clean_real"}
	findings := []jackal.Finding{
		{Path: targetFile, SizeBytes: 1024},
	}

	result, err := rule.Clean(context.Background(), findings, jackal.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}
	if result.Cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", result.Cleaned)
	}
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Error("file should be deleted after clean")
	}
}

func TestBaseScanRule_Scan_FileNotDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Create a file (not a directory) that matches a path
	targetFile := filepath.Join(tmp, "cache.dat")
	os.WriteFile(targetFile, make([]byte, 512), 0o644)

	rule := &baseScanRule{
		name:      "file_test",
		category:  jackal.CategoryGeneral,
		platforms: []string{"darwin", "linux"},
		paths:     []string{filepath.Join(tmp, "cache.dat")},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for file, got %d", len(findings))
	}
	if findings[0].IsDir {
		t.Error("expected IsDir=false for file finding")
	}
	if findings[0].FileCount != 1 {
		t.Errorf("expected FileCount=1 for file, got %d", findings[0].FileCount)
	}
}

func TestFindRule_SkipsEmptyTargetDirs(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "projects", "empty-app")
	emptyModules := filepath.Join(projectDir, "node_modules")
	os.MkdirAll(emptyModules, 0o755)
	// Don't create any files inside — should be skipped

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{filepath.Join(tmp, "projects")},
		maxDepth:    4,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty target dir, got %d", len(findings))
	}
}

func TestFindRule_Clean_RealDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	targetDir := filepath.Join(tmp, "node_modules")
	os.MkdirAll(targetDir, 0o755)
	os.WriteFile(filepath.Join(targetDir, "pkg.js"), make([]byte, 256), 0o644)

	rule := &findRule{name: "clean_find"}
	findings := []jackal.Finding{
		{Path: targetDir, SizeBytes: 256, IsDir: true},
	}

	result, err := rule.Clean(context.Background(), findings, jackal.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean error: %v", err)
	}
	if result.Cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", result.Cleaned)
	}
}

func TestFindRule_WalkDirError(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	// Create a search path with a permission-denied subdirectory
	projectDir := filepath.Join(tmp, "projects")
	os.MkdirAll(projectDir, 0o755)
	restricted := filepath.Join(projectDir, "restricted")
	os.MkdirAll(restricted, 0o000) // No read permission
	defer os.Chmod(restricted, 0o755)

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{projectDir},
		maxDepth:    4,
	}

	// Should not crash on permission errors
	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		t.Fatalf("Scan should not error on permission issues: %v", err)
	}
	t.Logf("findings with restricted dirs: %d", len(findings))
}

// ── Manifest (Horus) branch tests ───────────────────────────────────────

func TestBaseScanRule_ScanWithManifest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "caches")

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			cachePath: {size: 4096, count: 10},
		},
		globs: map[string][]string{
			cachePath: {cachePath},
		},
		files: map[string]bool{
			cachePath: true,
		},
	}

	rule := &baseScanRule{
		name:      "manifest_test",
		category:  jackal.CategoryGeneral,
		platforms: []string{"darwin", "linux"},
		paths:     []string{cachePath},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	// Manifest reports the path exists but Lstat may fail (path doesn't actually exist).
	// Should still produce a finding via manifest fallback.
	if len(findings) != 1 {
		t.Errorf("expected 1 finding via manifest, got %d", len(findings))
	}
	if len(findings) > 0 && findings[0].SizeBytes != 4096 {
		t.Errorf("expected 4096 bytes from manifest, got %d", findings[0].SizeBytes)
	}
}

func TestBaseScanRule_ScanManifestZeroSize(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "empty-cache")

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			cachePath: {size: 0, count: 0}, // Empty in manifest
		},
		globs: map[string][]string{
			cachePath: {cachePath},
		},
		files: map[string]bool{
			cachePath: true,
		},
	}

	rule := &baseScanRule{
		name:      "empty_manifest_test",
		category:  jackal.CategoryGeneral,
		platforms: []string{"darwin", "linux"},
		paths:     []string{cachePath},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	// Zero-size entries should be skipped
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for zero-size manifest entry, got %d", len(findings))
	}
}

func TestBaseScanRule_ScanManifestWithRealStat(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "real-cache")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "data.bin"), make([]byte, 2048), 0o644)

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			cacheDir: {size: 2048, count: 1},
		},
		globs: map[string][]string{
			cacheDir: {cacheDir},
		},
		files: map[string]bool{
			cacheDir: true,
		},
	}

	rule := &baseScanRule{
		name:      "real_manifest_test",
		category:  jackal.CategoryGeneral,
		platforms: []string{"darwin", "linux"},
		paths:     []string{cacheDir},
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestFindRule_ScanWithManifest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	searchRoot := filepath.Join(tmp, "projects")
	nodeModules := filepath.Join(searchRoot, "app", "node_modules")
	os.MkdirAll(nodeModules, 0o755)
	os.WriteFile(filepath.Join(nodeModules, "pkg.js"), make([]byte, 1024), 0o644)
	// Make it old enough
	oldTime := time.Now().AddDate(0, 0, -30)
	os.Chtimes(nodeModules, oldTime, oldTime)

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			nodeModules: {size: 1024, count: 1},
		},
		findDirs: map[string][]string{
			searchRoot + "/node_modules": {nodeModules},
		},
	}

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{searchRoot},
		maxDepth:    4,
		minAgeDays:  14,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding via manifest, got %d", len(findings))
	}
}

func TestFindRule_ManifestWithMatchFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	searchRoot := filepath.Join(tmp, "projects")
	rustTarget := filepath.Join(searchRoot, "mylib", "target")
	os.MkdirAll(rustTarget, 0o755)
	os.WriteFile(filepath.Join(rustTarget, "build.o"), make([]byte, 2048), 0o644)
	os.WriteFile(filepath.Join(searchRoot, "mylib", "Cargo.toml"), []byte("[package]"), 0o644)
	oldTime := time.Now().AddDate(0, 0, -30)
	os.Chtimes(rustTarget, oldTime, oldTime)

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			rustTarget: {size: 2048, count: 1},
		},
		findDirs: map[string][]string{
			searchRoot + "/target": {rustTarget},
		},
	}

	rule := &findRule{
		name:        "rust_targets",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "target",
		searchPaths: []string{searchRoot},
		maxDepth:    3,
		minAgeDays:  7,
		matchFile:   "Cargo.toml",
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding via manifest, got %d", len(findings))
	}
}

func TestFindRule_ManifestMatchFileNotPresent(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	searchRoot := filepath.Join(tmp, "projects")
	fakeTarget := filepath.Join(searchRoot, "notrust", "target")
	os.MkdirAll(fakeTarget, 0o755)
	os.WriteFile(filepath.Join(fakeTarget, "data.bin"), make([]byte, 1024), 0o644)
	// No Cargo.toml in parent

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			fakeTarget: {size: 1024, count: 1},
		},
		findDirs: map[string][]string{
			searchRoot + "/target": {fakeTarget},
		},
	}

	rule := &findRule{
		name:        "rust_targets",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "target",
		searchPaths: []string{searchRoot},
		maxDepth:    3,
		matchFile:   "Cargo.toml",
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings (no Cargo.toml), got %d", len(findings))
	}
}

func TestFindRule_ManifestZeroSize(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	searchRoot := filepath.Join(tmp, "projects")
	emptyModules := filepath.Join(searchRoot, "app", "node_modules")
	os.MkdirAll(emptyModules, 0o755)

	manifest := &mockManifest{
		dirs: map[string]struct {
			size  int64
			count int
		}{
			emptyModules: {size: 0, count: 0},
		},
		findDirs: map[string][]string{
			searchRoot + "/node_modules": {emptyModules},
		},
	}

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{searchRoot},
		maxDepth:    4,
	}

	findings, err := rule.Scan(context.Background(), jackal.ScanOptions{
		HomeDir:  tmp,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for zero-size manifest entry, got %d", len(findings))
	}
}

func TestFindRule_ContextCancellation(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create structure
	projectDir := filepath.Join(tmp, "projects", "app")
	nodeModules := filepath.Join(projectDir, "node_modules")
	os.MkdirAll(nodeModules, 0o755)
	os.WriteFile(filepath.Join(nodeModules, "pkg.js"), make([]byte, 512), 0o644)

	rule := &findRule{
		name:        "node_modules",
		category:    jackal.CategoryDev,
		platforms:   []string{"darwin", "linux"},
		targetName:  "node_modules",
		searchPaths: []string{filepath.Join(tmp, "projects")},
		maxDepth:    4,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	findings, err := rule.Scan(ctx, jackal.ScanOptions{HomeDir: tmp})
	if err != nil {
		// Context cancellation during walk may return error or empty results — both acceptable
		t.Logf("Scan returned error (expected): %v", err)
	}
	// With immediate cancel, we should get 0 or few findings
	t.Logf("Findings with canceled context: %d", len(findings))
}
