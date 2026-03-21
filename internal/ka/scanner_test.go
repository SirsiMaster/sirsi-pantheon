package ka

import (
	"testing"
)

func TestExtractBundleID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plist file", "com.parallels.desktop.console.plist", "com.parallels.desktop.console"},
		{"directory name", "com.parallels.desktop.console", "com.parallels.desktop.console"},
		{"saved state", "com.apple.Safari.savedState", "com.apple.Safari"},
		{"group prefix", "group.com.docker", "com.docker"},
		{"org prefix", "org.mozilla.firefox", "org.mozilla.firefox"},
		{"io prefix", "io.github.nickvision.money", "io.github.nickvision.money"},
		{"net prefix", "net.sourceforge.skim-app.skim", "net.sourceforge.skim-app.skim"},
		{"dev prefix", "dev.zed.Zed", "dev.zed.Zed"},
		{"no dots - not bundle ID", "SomeAppName", ""},
		{"single dot - not bundle ID", "com.app", "com.app"},
		{"unknown TLD prefix", "xyz.something.app", ""},
		{"empty string", "", ""},
		{"de prefix", "de.appmaker.myapp", "de.appmaker.myapp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBundleID(tt.input)
			if got != tt.expected {
				t.Errorf("extractBundleID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGuessAppName(t *testing.T) {
	tests := []struct {
		name     string
		bundleID string
		expected string
	}{
		{"standard 3-part", "com.parallels.desktop", "Desktop"},
		{"4-part bundle", "com.parallels.desktop.console", "Desktop"},
		{"hyphenated name", "com.apple.dt.xcode-build", "Dt"},
		{"capitalization", "com.google.chrome", "Chrome"},
		{"2-part returns raw", "com.app", "com.app"},
		{"1-part returns raw", "something", "something"},
		{"org prefix", "org.mozilla.firefox", "Firefox"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessAppName(tt.bundleID)
			if got != tt.expected {
				t.Errorf("guessAppName(%q) = %q, want %q", tt.bundleID, got, tt.expected)
			}
		})
	}
}

func TestIsSystemBundleID(t *testing.T) {
	tests := []struct {
		name     string
		bundleID string
		expected bool
	}{
		{"apple system", "com.apple.Safari", true},
		{"apple subsystem", "com.apple.dt.Xcode", true},
		{"google drivefs", "com.google.drivefs.helper", true},
		{"google keystone", "com.google.keystone.agent", true},
		{"microsoft onedrive", "com.microsoft.OneDrive.FinderSync", true},
		{"microsoft autoupdate", "com.microsoft.autoupdate2", true},
		{"third party app", "com.parallels.desktop", false},
		{"user app", "com.mycompany.myapp", false},
		{"mozilla", "org.mozilla.firefox", false},
		{"docker", "com.docker.docker", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSystemBundleID(tt.bundleID)
			if got != tt.expected {
				t.Errorf("isSystemBundleID(%q) = %v, want %v", tt.bundleID, got, tt.expected)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		homeDir string
		want    string
	}{
		{"tilde expansion", "~/Library", "/Users/test", "/Users/test/Library"},
		{"no tilde", "/usr/local", "/Users/test", "/usr/local"},
		{"tilde only prefix", "~foo/bar", "/Users/test", "~foo/bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.path, tt.homeDir)
			if got != tt.want {
				t.Errorf("expandPath(%q, %q) = %q, want %q", tt.path, tt.homeDir, got, tt.want)
			}
		})
	}
}

func TestNewScanner(t *testing.T) {
	s := NewScanner()
	if s == nil {
		t.Fatal("NewScanner() returned nil")
	}
	if s.installedApps == nil {
		t.Error("installedApps map not initialized")
	}
	if s.installedNames == nil {
		t.Error("installedNames map not initialized")
	}
	if s.knownBundleIDs == nil {
		t.Error("knownBundleIDs map not initialized")
	}
}
