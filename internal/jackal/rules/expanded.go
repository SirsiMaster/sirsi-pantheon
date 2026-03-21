package rules

import "github.com/SirsiMaster/sirsi-anubis/internal/jackal"

// ═══════════════════════════════════════════
// JAVA / JVM ECOSYSTEM
// ═══════════════════════════════════════════

// NewGradleCacheRule scans Gradle build system caches.
func NewGradleCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "gradle_cache",
		displayName: "Gradle Cache",
		category:    jackal.CategoryDev,
		description: "Gradle wrapper distributions, dependency cache, and build outputs",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.gradle/caches",
			"~/.gradle/wrapper/dists",
			"~/.gradle/daemon",
		},
		minAgeDays: 14,
	}
}

// NewMavenCacheRule scans Maven repository cache.
func NewMavenCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "maven_cache",
		displayName: "Maven Repository",
		category:    jackal.CategoryDev,
		description: "Maven local repository (~/.m2/repository) and wrapper",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.m2/repository",
		},
	}
}

// ═══════════════════════════════════════════
// PACKAGE MANAGER CACHES
// ═══════════════════════════════════════════

// NewNpmGlobalCacheRule scans npm/yarn/pnpm global caches.
func NewNpmGlobalCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "npm_global_cache",
		displayName: "npm/yarn/pnpm Cache",
		category:    jackal.CategoryDev,
		description: "npm _cacache, yarn cache, and pnpm content-addressable store",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.npm/_cacache",
			"~/.yarn/cache",
			"~/.cache/yarn",
			"~/.local/share/pnpm/store",
			"~/Library/pnpm/store",
		},
	}
}

// NewHomebrewCacheRule scans Homebrew download and formula caches.
func NewHomebrewCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "homebrew_cache",
		displayName: "Homebrew Cache",
		category:    jackal.CategoryDev,
		description: "Homebrew downloaded bottles, source tarballs, and old formula versions",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/Homebrew",
		},
	}
}

// NewCocoaPodsCacheRule scans CocoaPods caches.
func NewCocoaPodsCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "cocoapods_cache",
		displayName: "CocoaPods Cache",
		category:    jackal.CategoryDev,
		description: "CocoaPods spec repos and download cache",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/CocoaPods",
			"~/.cocoapods/repos",
		},
	}
}

// NewSPMCacheRule scans Swift Package Manager caches.
func NewSPMCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "spm_cache",
		displayName: "Swift Package Manager",
		category:    jackal.CategoryDev,
		description: "SPM cloned repositories and build products",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Developer/Xcode/SPMRepositories",
			"~/Library/org.swift.swiftpm",
		},
	}
}

// NewComposerCacheRule scans PHP Composer caches.
func NewComposerCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "composer_cache",
		displayName: "Composer (PHP) Cache",
		category:    jackal.CategoryDev,
		description: "PHP Composer package cache and downloaded files",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.composer/cache",
			"~/.cache/composer",
		},
	}
}

// NewRubyGemsCacheRule scans Ruby gem caches.
func NewRubyGemsCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "rubygems_cache",
		displayName: "Ruby Gems Cache",
		category:    jackal.CategoryDev,
		description: "Ruby gem download cache and doc generation",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.gem/ruby",
			"~/Library/Caches/gem",
		},
	}
}

// ═══════════════════════════════════════════
// ADDITIONAL AI/ML CACHES
// ═══════════════════════════════════════════

// NewOnnxCacheRule scans ONNX Runtime caches.
func NewOnnxCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "onnx_cache",
		displayName: "ONNX Runtime Cache",
		category:    jackal.CategoryAI,
		description: "ONNX Runtime model cache and optimization output",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/onnxruntime",
		},
	}
}

// NewVLLMCacheRule scans vLLM engine caches.
func NewVLLMCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "vllm_cache",
		displayName: "vLLM Cache",
		category:    jackal.CategoryAI,
		description: "vLLM compiled kernels and model weight cache",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/vllm",
		},
	}
}

// NewJaxCacheRule scans JAX/Flax caches.
func NewJaxCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "jax_cache",
		displayName: "JAX / Flax Cache",
		category:    jackal.CategoryAI,
		description: "JAX compiled XLA caches and Flax checkpoints",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/jax",
			"~/.cache/flax",
		},
	}
}

// NewStableDiffusionModelsRule scans Stable Diffusion model caches.
func NewStableDiffusionModelsRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "stable_diffusion",
		displayName: "Stable Diffusion Models",
		category:    jackal.CategoryAI,
		description: "Stable Diffusion model checkpoints, LoRAs, and ComfyUI/Automatic1111 outputs",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/stable-diffusion",
			"~/stable-diffusion-webui/models",
		},
	}
}

// NewLangChainCacheRule scans LangChain caches.
func NewLangChainCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "langchain_cache",
		displayName: "LangChain Cache",
		category:    jackal.CategoryAI,
		description: "LangChain embedding caches and vector store temp files",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.langchain",
			"~/.cache/langchain",
		},
	}
}

// ═══════════════════════════════════════════
// ADDITIONAL IDE CACHES
// ═══════════════════════════════════════════

// NewCursorCacheRule scans Cursor IDE caches.
func NewCursorCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "cursor_caches",
		displayName: "Cursor IDE",
		category:    jackal.CategoryIDEs,
		description: "Cursor AI IDE caches, workspace storage, and extension data",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Application Support/Cursor/Cache",
			"~/Library/Application Support/Cursor/CachedData",
			"~/Library/Application Support/Cursor/CachedExtensions",
			"~/Library/Application Support/Cursor/logs",
		},
		minAgeDays: 7,
	}
}

// NewWindsurfCacheRule scans Windsurf IDE caches.
func NewWindsurfCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "windsurf_caches",
		displayName: "Windsurf IDE",
		category:    jackal.CategoryIDEs,
		description: "Windsurf (Codeium) IDE caches and workspace storage",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Application Support/Windsurf/Cache",
			"~/Library/Application Support/Windsurf/CachedData",
			"~/Library/Application Support/Windsurf/logs",
		},
		minAgeDays: 7,
	}
}

// NewZedCacheRule scans Zed editor caches.
func NewZedCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "zed_caches",
		displayName: "Zed Editor",
		category:    jackal.CategoryIDEs,
		description: "Zed editor caches, language server downloads, and log files",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Application Support/Zed/logs",
			"~/Library/Caches/dev.zed.Zed",
			"~/Library/Application Support/Zed/languages",
		},
		minAgeDays: 7,
	}
}

// NewNeovimCacheRule scans Neovim caches.
func NewNeovimCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "neovim_caches",
		displayName: "Neovim Cache",
		category:    jackal.CategoryIDEs,
		description: "Neovim plugin caches, swap files, and undo history",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.local/share/nvim",
			"~/.cache/nvim",
		},
		minAgeDays: 14,
	}
}

// NewCodexCLIRule scans OpenAI Codex CLI caches.
func NewCodexCLIRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "codex_cli",
		displayName: "Codex CLI",
		category:    jackal.CategoryIDEs,
		description: "OpenAI Codex CLI session logs and caches",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.codex",
			"~/.cache/codex",
		},
		minAgeDays: 14,
	}
}

// ═══════════════════════════════════════════
// ADDITIONAL CLOUD / INFRA
// ═══════════════════════════════════════════

// NewNginxLogsRule scans nginx access/error logs.
func NewNginxLogsRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "nginx_logs",
		displayName: "nginx Logs",
		category:    jackal.CategoryCloud,
		description: "nginx access and error logs (local development installs)",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"/usr/local/var/log/nginx",
			"/opt/homebrew/var/log/nginx",
			"/var/log/nginx",
		},
		minAgeDays: 7,
	}
}

// NewAWSCLICacheRule scans AWS CLI caches.
func NewAWSCLICacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "aws_cli_cache",
		displayName: "AWS CLI Cache",
		category:    jackal.CategoryCloud,
		description: "AWS CLI credential cache and SSO session data",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.aws/cli/cache",
			"~/.aws/sso/cache",
		},
		minAgeDays: 7,
	}
}

// ═══════════════════════════════════════════
// ADDITIONAL CLOUD STORAGE
// ═══════════════════════════════════════════

// NewICloudCacheRule scans iCloud Drive caches.
func NewICloudCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "icloud_cache",
		displayName: "iCloud Drive Cache",
		category:    jackal.CategoryStorage,
		description: "iCloud Drive local cache and download staging",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Mobile Documents/com~apple~CloudDocs/.iCloud",
			"~/Library/Caches/com.apple.cloudd",
		},
	}
}

// ═══════════════════════════════════════════
// GENERAL MAC — ADDITIONAL
// ═══════════════════════════════════════════

// NewTimeMachineLocalRule scans Time Machine local snapshots.
func NewTimeMachineLocalRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "timemachine_local",
		displayName: "Time Machine Local Snapshots",
		category:    jackal.CategoryGeneral,
		description: "Time Machine local snapshots consuming disk space",
		platforms:   []string{"darwin"},
		paths: []string{
			"/Volumes/com.apple.TimeMachine.localsnapshots",
		},
	}
}

// NewMailAttachmentsCacheRule scans Mail.app attachment cache.
func NewMailAttachmentsCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "mail_attachments",
		displayName: "Mail Attachments Cache",
		category:    jackal.CategoryGeneral,
		description: "Apple Mail downloaded attachment cache",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Containers/com.apple.mail/Data/Library/Mail Downloads",
		},
		minAgeDays: 30,
	}
}

// NewFontCachesRule scans font validation caches.
func NewFontCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "font_caches",
		displayName: "Font Caches",
		category:    jackal.CategoryGeneral,
		description: "macOS font validation caches and Font Book data",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/com.apple.FontRegistry",
			"~/Library/Caches/com.apple.ats",
		},
		minAgeDays: 14,
	}
}
