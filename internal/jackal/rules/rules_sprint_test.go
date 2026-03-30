package rules

import (
	"runtime"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
)

// ── General Rules (general.go) ─────────────────────────────────────

func TestGeneralRules_Constructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]func() jackal.ScanRule{
		"system_caches":           NewSystemCachesRule,
		"system_logs":             NewSystemLogsRule,
		"crash_reports":           NewCrashReportsRule,
		"downloads_junk":          NewDownloadsJunkRule,
		"trash":                   NewTrashRule,
		"browser_caches":          NewBrowserCachesRule,
		"timemachine_local":      NewTimeMachineLocalRule,
		"mail_attachments":       NewMailAttachmentsCacheRule,
		"font_caches":             NewFontCachesRule,
	}
	for name, constructor := range constructors {
		rule := constructor()
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
		if rule.Category() != jackal.CategoryGeneral {
			t.Errorf("%q: Category() = %q, want 'general'", name, rule.Category())
		}
	}
}

// ── Virtualization Rules (virtualization.go) ─────────────────────────

func TestVirtualizationRules_Constructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]func() jackal.ScanRule{
		"parallels_full": NewParallelsFullRule,
		"vmware_fusion":  NewVMwareFusionRule,
		"utm":             NewUTMRule,
		"virtualbox":      NewVirtualBoxRule,
	}
	for name, constructor := range constructors {
		rule := constructor()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.Category() != jackal.CategoryVirtualization {
			t.Errorf("%q: Category() = %q, want 'vms'", name, rule.Category())
		}
	}
}

// ── AI Rules (ai.go) ────────────────────────────────────────────────

func TestAIRules_Constructors(t *testing.T) {
	t.Parallel()
	constructors := map[string]func() jackal.ScanRule{
		"mlx_cache":          NewMLXCacheRule,
		"metal_shaders":     NewMetalShaderCacheRule,
	}
	for name, constructor := range constructors {
		rule := constructor()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
		if rule.Category() != jackal.CategoryAI {
			t.Errorf("%q: Category() = %q, want 'ai'", name, rule.Category())
		}
	}
}

// ── Dev Rules (dev.go) ──────────────────────────────────────────────

func TestDevRules_AllHavePaths(t *testing.T) {
	t.Parallel()
	rules := crossPlatformRules()
	for _, r := range rules {
		base, ok := r.(*baseScanRule)
		if !ok {
			continue
		}
		if len(base.paths) == 0 {
			t.Errorf("rule %q has no scan paths", r.Name())
		}
	}
}

// ── Expanded Rules (expanded.go + ides_cloud.go) ─────────────────────

func TestExpandedRules_AllConstructors(t *testing.T) {
	t.Parallel()
	// IDE rules
	ideRules := map[string]func() jackal.ScanRule{
		"xcode_derived_data": NewXcodeDerivedDataRule,
		"vscode_caches":      NewVSCodeCachesRule,
		"jetbrains_caches":   NewJetBrainsCachesRule,
		"android_studio":     NewAndroidStudioRule,
		"cursor_caches":       NewCursorCacheRule,
		"windsurf_caches":   NewWindsurfCacheRule,
		"zed_caches":        NewZedCacheRule,
	}
	for name, constructor := range ideRules {
		rule := constructor()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
	}

	// Package manager rules
	pkgRules := map[string]func() jackal.ScanRule{
		"homebrew_cache":   NewHomebrewCacheRule,
		"cocoapods_cache":  NewCocoaPodsCacheRule,
		"spm_cache":        NewSPMCacheRule,
	}
	for name, constructor := range pkgRules {
		rule := constructor()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Name() != name {
			t.Errorf("%q: Name() = %q", name, rule.Name())
		}
	}

	// Cloud storage rules
	cloudRules := map[string]func() jackal.ScanRule{
		"onedrive_cache":       NewOneDriveCacheRule,
		"google_drive_cache":   NewGoogleDriveCacheRule,
		"dropbox_cache":        NewDropboxCacheRule,
		"icloud_cache":         NewICloudCacheRule,
	}
	for name, constructor := range cloudRules {
		rule := constructor()
		if rule == nil {
			t.Errorf("constructor for %q returned nil", name)
			continue
		}
		if rule.Category() != jackal.CategoryStorage {
			t.Errorf("%q: Category() = %q, want 'storage'", name, rule.Category())
		}
	}
}

// ── Platform distribution ────────────────────────────────────────────

func TestAllRules_PlatformDistribution(t *testing.T) {
	t.Parallel()
	rules := AllRules()
	darwinOnly := 0
	crossPlatform := 0
	for _, r := range rules {
		platforms := r.Platforms()
		hasDarwin := false
		hasLinux := false
		for _, p := range platforms {
			if p == "darwin" {
				hasDarwin = true
			}
			if p == "linux" {
				hasLinux = true
			}
		}
		if hasDarwin && !hasLinux {
			darwinOnly++
		}
		if hasDarwin && hasLinux {
			crossPlatform++
		}
	}

	if runtime.GOOS == "darwin" {
		if darwinOnly == 0 {
			t.Error("should have darwin-only rules on macOS")
		}
	}
	if crossPlatform == 0 {
		t.Error("should have cross-platform rules")
	}
}
