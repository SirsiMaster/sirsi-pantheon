package rules

import "github.com/SirsiMaster/sirsi-anubis/internal/jackal"

// ═══════════════════════════════════════════
// IDES & AI CODING TOOLS
// ═══════════════════════════════════════════

// NewVSCodeCachesRule scans for VS Code caches and workspace storage.
func NewVSCodeCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "vscode_caches",
		displayName: "VS Code Caches",
		category:    jackal.CategoryIDEs,
		description: "VS Code extension cache, cached data, and workspace storage",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Application Support/Code/Cache",
			"~/Library/Application Support/Code/CachedData",
			"~/Library/Application Support/Code/CachedExtensions",
			"~/Library/Application Support/Code/CachedExtensionVSIXs",
			"~/Library/Application Support/Code/logs",
		},
		minAgeDays: 7,
	}
}

// NewJetBrainsCachesRule scans for JetBrains IDE caches.
func NewJetBrainsCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "jetbrains_caches",
		displayName: "JetBrains Caches",
		category:    jackal.CategoryIDEs,
		description: "IntelliJ, GoLand, WebStorm, PyCharm caches and indices",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/JetBrains",
		},
	}
}

// NewClaudeCodeRule scans for Claude Code / Antigravity caches.
func NewClaudeCodeRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "claude_code",
		displayName: "Claude Code (Antigravity)",
		category:    jackal.CategoryIDEs,
		description: "Claude Code conversation logs and session caches",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.claude/logs",
		},
		minAgeDays: 14,
	}
}

// NewGeminiCLIRule scans for Gemini CLI caches.
func NewGeminiCLIRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "gemini_cli",
		displayName: "Gemini CLI",
		category:    jackal.CategoryIDEs,
		description: "Gemini CLI session data and caches",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.gemini/cache",
		},
		excludes: []string{
			"~/.gemini/antigravity/skills", // Don't touch installed skills
		},
		minAgeDays: 14,
	}
}

// NewAndroidStudioRule scans for Android Studio caches and SDK.
func NewAndroidStudioRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "android_studio",
		displayName: "Android Studio",
		category:    jackal.CategoryIDEs,
		description: "Android Studio caches, AVD images, and Gradle wrapper",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/Google/AndroidStudio*",
			"~/.android/avd",
			"~/.android/cache",
		},
	}
}

// ═══════════════════════════════════════════
// CLOUD & INFRASTRUCTURE
// ═══════════════════════════════════════════

// NewKubernetesCachesRule scans for Kubernetes/Minikube caches.
func NewKubernetesCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "kubernetes_caches",
		displayName: "Kubernetes / Minikube",
		category:    jackal.CategoryCloud,
		description: "Minikube VM images, kubectl cache, and Helm charts",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.minikube",
			"~/.cache/helm",
			"~/.kube/cache",
		},
	}
}

// NewTerraformCachesRule scans for Terraform provider caches.
func NewTerraformCachesRule() jackal.ScanRule {
	return &findRule{
		name:        "terraform_providers",
		displayName: "Terraform Providers",
		category:    jackal.CategoryCloud,
		description: "Terraform .terraform directories with cached providers",
		platforms:   []string{"darwin", "linux"},
		targetName:  ".terraform",
		searchPaths: []string{
			"~/Development",
			"~/code",
			"~/projects",
		},
		maxDepth:   3,
		minAgeDays: 14,
	}
}

// NewGCloudCachesRule scans for Google Cloud SDK caches.
func NewGCloudCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "gcloud_caches",
		displayName: "Google Cloud SDK",
		category:    jackal.CategoryCloud,
		description: "gcloud CLI logs, cache, and credential helpers",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.config/gcloud/logs",
			"~/.config/gcloud/cache",
		},
		minAgeDays: 7,
	}
}

// NewFirebaseCachesRule scans for Firebase CLI caches.
func NewFirebaseCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "firebase_caches",
		displayName: "Firebase CLI",
		category:    jackal.CategoryCloud,
		description: "Firebase CLI caches and emulator JARs",
		platforms:   []string{"darwin", "linux"},
		paths: []string{
			"~/.cache/firebase",
		},
	}
}

// ═══════════════════════════════════════════
// CLOUD STORAGE
// ═══════════════════════════════════════════

// NewOneDriveCacheRule scans for OneDrive caches.
func NewOneDriveCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "onedrive_cache",
		displayName: "OneDrive Cache",
		category:    jackal.CategoryStorage,
		description: "Microsoft OneDrive local cache and logs",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/com.microsoft.OneDrive",
			"~/Library/Logs/OneDrive",
		},
	}
}

// NewGoogleDriveCacheRule scans for Google Drive caches.
func NewGoogleDriveCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "google_drive_cache",
		displayName: "Google Drive Cache",
		category:    jackal.CategoryStorage,
		description: "Google Drive File Stream logs and cache",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Application Support/Google/DriveFS/Logs",
			"~/Library/Caches/com.google.drivefs",
		},
	}
}

// NewDropboxCacheRule scans for Dropbox caches.
func NewDropboxCacheRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "dropbox_cache",
		displayName: "Dropbox Cache",
		category:    jackal.CategoryStorage,
		description: "Dropbox cache and landfill files",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/.dropbox/cache",
		},
	}
}
