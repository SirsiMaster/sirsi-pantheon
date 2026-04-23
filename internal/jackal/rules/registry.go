package rules

import (
	"runtime"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
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
		// General Mac (9 rules)
		NewSystemCachesRule(),
		NewSystemLogsRule(),
		NewCrashReportsRule(),
		NewDownloadsJunkRule(),
		NewTrashRule(),
		NewBrowserCachesRule(),
		NewTimeMachineLocalRule(),
		NewMailAttachmentsCacheRule(),
		NewFontCachesRule(),

		// Virtualization (4 rules)
		NewParallelsFullRule(),
		NewVMwareFusionRule(),
		NewUTMRule(),
		NewVirtualBoxRule(),

		// AI/ML — macOS only (2 rules)
		NewMLXCacheRule(),
		NewMetalShaderCacheRule(),

		// IDEs — macOS only (8 rules)
		NewXcodeDerivedDataRule(),
		NewVSCodeCachesRule(),
		NewJetBrainsCachesRule(),
		NewAndroidStudioRule(),
		NewCursorCacheRule(),
		NewWindsurfCacheRule(),
		NewZedCacheRule(),

		// Package managers — macOS only (3 rules)
		NewHomebrewCacheRule(),
		NewCocoaPodsCacheRule(),
		NewSPMCacheRule(),

		// Cloud Storage — macOS only (4 rules)
		NewOneDriveCacheRule(),
		NewGoogleDriveCacheRule(),
		NewDropboxCacheRule(),
		NewICloudCacheRule(),
	}
}

// linuxRules returns Linux-specific scan rules.
func linuxRules() []jackal.ScanRule {
	// TODO: Phase 2 — systemd journals, apt/dpkg cache, snap cache
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

		// Package managers — cross-platform (5 rules)
		NewNpmGlobalCacheRule(),
		NewGradleCacheRule(),
		NewMavenCacheRule(),
		NewComposerCacheRule(),
		NewRubyGemsCacheRule(),

		// Ghost cleanup (registered for Clean dispatch — Scan is no-op)
		NewKaGhostRule(),

		// Git & Repo Hygiene (7 rules)
		NewStaleBranchesRule(),
		NewGitMergedBranchesRule(),
		NewGitLargeObjectsRule(),
		NewGitOrphanedWorktreesRule(),
		NewGitUntrackedArtifactsRule(),
		NewGitRerereCacheRule(),
		NewGitReflogBloatRule(),

		// CI/CD & Build (7 rules)
		NewGitHubActionsCacheRule(),
		NewActRunnerCacheRule(),
		NewBuildOutputRule(),
		NewNextJSCacheRule(),
		NewTurborepoCache(),
		NewDanglingDockerImagesRule(),
		NewDockerBuildCacheRule(),

		// Repo Hygiene (8 rules)
		NewEnvFileRule(),
		NewStaleLockFilesRule(),
		NewDeadSymlinksRule(),
		NewOversizedReposRule(),
		NewCoverageReportsRule(),
		NewLogFilesRule(),
		NewVenvRule(),
		NewDotEnvVenvRule(),

		// AI/ML — cross-platform (9 rules)
		NewHuggingFaceCacheRule(),
		NewOllamaModelsRule(),
		NewPyTorchCacheRule(),
		NewTensorFlowCacheRule(),
		NewOnnxCacheRule(),
		NewVLLMCacheRule(),
		NewJaxCacheRule(),
		NewStableDiffusionModelsRule(),
		NewLangChainCacheRule(),

		// IDEs — cross-platform (4 rules)
		NewClaudeCodeRule(),
		NewGeminiCLIRule(),
		NewNeovimCacheRule(),
		NewCodexCLIRule(),

		// Cloud — cross-platform (6 rules)
		NewKubernetesCachesRule(),
		NewTerraformCachesRule(),
		NewGCloudCachesRule(),
		NewFirebaseCachesRule(),
		NewNginxLogsRule(),
		NewAWSCLICacheRule(),
	}
}
