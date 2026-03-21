package cleaner

import (
	"runtime"
	"testing"
)

func TestValidatePath_ProtectedPrefixes(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("platform-specific test")
	}

	var protectedPaths []string
	if runtime.GOOS == "darwin" {
		protectedPaths = []string{
			"/System/Library/Frameworks",
			"/usr/bin/ls",
			"/bin/sh",
			"/sbin/mount",
			"/private/var/db/receipts",
			"/Library/Extensions/SomeDriver.kext",
			"/Library/Frameworks/SomeFramework.framework",
		}
	} else {
		protectedPaths = []string{
			"/boot/vmlinuz",
			"/etc/passwd",
			"/usr/bin/ls",
			"/bin/sh",
			"/sbin/init",
			"/proc/1/status",
			"/sys/class/net",
			"/dev/null",
		}
	}

	for _, path := range protectedPaths {
		t.Run(path, func(t *testing.T) {
			err := ValidatePath(path)
			if err == nil {
				t.Errorf("ValidatePath(%q) = nil, want BLOCKED error", path)
			}
		})
	}
}

func TestValidatePath_ProtectedSuffixes(t *testing.T) {
	tests := []struct {
		path string
	}{
		{"/Users/test/Library/Keychains/login.keychain-db"},
		{"/Users/test/Library/Keychains/System.keychain"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if err == nil {
				t.Errorf("ValidatePath(%q) = nil, want BLOCKED error", tt.path)
			}
		})
	}
}

func TestValidatePath_ProtectedNames(t *testing.T) {
	tests := []struct {
		path string
	}{
		{"/Users/test/project/.git"},
		{"/Users/test/project/.env"},
		{"/Users/test/.ssh"},
		{"/Users/test/.gnupg"},
		{"/Users/test/.ssh/id_rsa"},
		{"/Users/test/.ssh/id_ed25519"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if err == nil {
				t.Errorf("ValidatePath(%q) = nil, want BLOCKED error", tt.path)
			}
		})
	}
}

func TestValidatePath_AllowedPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"user cache", "/Users/test/Library/Caches/com.example.app"},
		{"user logs", "/Users/test/Library/Logs/SomeApp"},
		{"tmp file", "/tmp/somefile.txt"},
		{"user downloads", "/Users/test/Downloads/old-installer.dmg"},
		{"node_modules", "/Users/test/Development/project/node_modules"},
		{"derived data", "/Users/test/Library/Developer/Xcode/DerivedData/MyApp-abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if err != nil {
				t.Errorf("ValidatePath(%q) = %v, want nil (should be allowed)", tt.path, err)
			}
		})
	}
}

func TestValidatePath_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantError bool
	}{
		{"root path", "/", false},
		{"home dir itself", "/Users/test", false},
		{"relative path", "relative/path/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if tt.wantError && err == nil {
				t.Errorf("ValidatePath(%q) = nil, want error", tt.path)
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidatePath(%q) = %v, want nil", tt.path, err)
			}
		})
	}
}

func TestDirSize(t *testing.T) {
	// DirSize on a non-existent directory should return 0
	size := DirSize("/nonexistent/path/that/doesnt/exist")
	if size != 0 {
		t.Errorf("DirSize(nonexistent) = %d, want 0", size)
	}
}
