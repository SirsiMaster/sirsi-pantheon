package rules

import "github.com/SirsiMaster/sirsi-pantheon/internal/jackal"

// ═══════════════════════════════════════════
// GENERAL MAC — System caches, logs, junk
// ═══════════════════════════════════════════

// NewSystemCachesRule scans for system and application caches.
func NewSystemCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "system_caches",
		displayName: "System & App Caches",
		category:    jackal.CategoryGeneral,
		description: "System and application cache files that can be safely regenerated",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/*",
		},
		excludes: []string{
			"~/Library/Caches/com.apple.iconservices*",
			"~/Library/Caches/com.apple.Safari*",
		},
		minAgeDays: 7,
	}
}

// NewSystemLogsRule scans for old system and application logs.
func NewSystemLogsRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "system_logs",
		displayName: "System & App Logs",
		category:    jackal.CategoryGeneral,
		description: "Old system and application log files",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Logs/*",
		},
		minAgeDays: 14,
	}
}

// NewCrashReportsRule scans for crash reports and diagnostic data.
func NewCrashReportsRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "crash_reports",
		displayName: "Crash Reports",
		category:    jackal.CategoryGeneral,
		description: "Crash reports and diagnostic data",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Logs/DiagnosticReports/*",
		},
		minAgeDays: 3,
		severity:   jackal.SeverityCaution,
	}
}

// NewDownloadsJunkRule scans for old installer files in Downloads.
func NewDownloadsJunkRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "downloads_junk",
		displayName: "Downloads Installers",
		category:    jackal.CategoryGeneral,
		description: "Old .dmg, .pkg, and .zip files in Downloads",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Downloads/*.dmg",
			"~/Downloads/*.pkg",
			"~/Downloads/*.zip",
		},
		minAgeDays: 30,
	}
}

// NewTrashRule scans the user's Trash.
func NewTrashRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "trash",
		displayName: "Trash",
		category:    jackal.CategoryGeneral,
		description: "Files in the user's Trash folder",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/.Trash/*",
		},
		minAgeDays: 3,
	}
}

// NewBrowserCachesRule scans for browser caches (Chrome, Firefox, Safari).
func NewBrowserCachesRule() jackal.ScanRule {
	return &baseScanRule{
		name:        "browser_caches",
		displayName: "Browser Caches",
		category:    jackal.CategoryGeneral,
		description: "Chrome, Firefox, Arc, and other browser caches",
		platforms:   []string{"darwin"},
		paths: []string{
			"~/Library/Caches/Google/Chrome/Default/Cache/*",
			"~/Library/Caches/com.google.Chrome/*",
			"~/Library/Caches/Firefox/Profiles/*/cache2/*",
			"~/Library/Caches/com.operasoftware.Opera/*",
			"~/Library/Caches/company.thebrowser.Browser/*",
		},
		minAgeDays: 3,
	}
}
