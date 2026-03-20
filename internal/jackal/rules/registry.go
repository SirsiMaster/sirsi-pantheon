package rules

import (
	"runtime"

	"github.com/SirsiMaster/sirsi-anubis/internal/jackal"
)

// AllRules returns all built-in scan rules for the current platform.
// New rules are added here as they're implemented.
func AllRules() []jackal.ScanRule {
	var rules []jackal.ScanRule

	// Platform-specific rules
	switch runtime.GOOS {
	case "darwin":
		rules = append(rules, darwinRules()...)
	case "linux":
		rules = append(rules, linuxRules()...)
	}

	// Cross-platform rules
	rules = append(rules, crossPlatformRules()...)

	return rules
}

// darwinRules returns macOS-specific scan rules.
func darwinRules() []jackal.ScanRule {
	return []jackal.ScanRule{
		// General Mac (6 rules)
		NewSystemCachesRule(),
		NewSystemLogsRule(),
		NewCrashReportsRule(),
		NewDownloadsJunkRule(),
		NewTrashRule(),
		NewBrowserCachesRule(),

		// Virtualization (4 rules)
		NewParallelsFullRule(),
		NewVMwareFusionRule(),
		NewUTMRule(),
		NewVirtualBoxRule(),

		// AI/ML — macOS only (2 rules)
		NewMLXCacheRule(),
		NewMetalShaderCacheRule(),

		// IDEs — macOS only (4 rules)
		NewXcodeDerivedDataRule(),
		NewVSCodeCachesRule(),
		NewJetBrainsCachesRule(),
		NewAndroidStudioRule(),

		// Cloud Storage — macOS only (3 rules)
		NewOneDriveCacheRule(),
		NewGoogleDriveCacheRule(),
		NewDropboxCacheRule(),
	}
}

// linuxRules returns Linux-specific scan rules.
func linuxRules() []jackal.ScanRule {
	// TODO: Phase 2
	return []jackal.ScanRule{}
}

// crossPlatformRules returns rules that work on all platforms.
func crossPlatformRules() []jackal.ScanRule {
	return []jackal.ScanRule{
		// Developer Frameworks (5 rules)
		NewNodeModulesRule(),
		NewGoModCacheRule(),
		NewPythonCachesRule(),
		NewRustTargetRule(),
		NewDockerRule(),

		// AI/ML — cross-platform (4 rules)
		NewHuggingFaceCacheRule(),
		NewOllamaModelsRule(),
		NewPyTorchCacheRule(),
		NewTensorFlowCacheRule(),

		// IDEs — cross-platform (2 rules)
		NewClaudeCodeRule(),
		NewGeminiCLIRule(),

		// Cloud — cross-platform (4 rules)
		NewKubernetesCachesRule(),
		NewTerraformCachesRule(),
		NewGCloudCachesRule(),
		NewFirebaseCachesRule(),
	}
}
