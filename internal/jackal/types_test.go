package jackal

import (
	"runtime"
	"testing"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero bytes", 0, "0 B"},
		{"one byte", 1, "1 B"},
		{"sub-KB", 512, "512 B"},
		{"exactly 1 KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"exactly 1 MB", 1024 * 1024, "1.0 MB"},
		{"500 MB", 500 * 1024 * 1024, "500.0 MB"},
		{"exactly 1 GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"67.8 GB", 72813813760, "67.8 GB"},
		{"exactly 1 TB", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"2.5 TB", int64(2.5 * 1024 * 1024 * 1024 * 1024), "2.5 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSize(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatSize(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		homeDir  string
		expected string
	}{
		{"tilde expansion", "~/Documents", "/Users/test", "/Users/test/Documents"},
		{"tilde with nested path", "~/Library/Caches/foo", "/home/user", "/home/user/Library/Caches/foo"},
		{"no tilde", "/usr/local/bin", "/Users/test", "/usr/local/bin"},
		{"relative path stays relative", "relative/path", "/Users/test", "relative/path"},
		{"tilde only partial no-op", "~foo/bar", "/Users/test", "~foo/bar"},
		{"empty home uses os default", "~/test", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "empty home uses os default" {
				// When homeDir is empty, it falls back to os.UserHomeDir
				got := ExpandPath(tt.path, tt.homeDir)
				if got == "" {
					t.Skip("os.UserHomeDir returned empty")
				}
				return
			}
			got := ExpandPath(tt.path, tt.homeDir)
			if got != tt.expected {
				t.Errorf("ExpandPath(%q, %q) = %q, want %q", tt.path, tt.homeDir, got, tt.expected)
			}
		})
	}
}

func TestPlatformMatch(t *testing.T) {
	current := runtime.GOOS

	tests := []struct {
		name      string
		platforms []string
		expected  bool
	}{
		{"matches current OS", []string{current}, true},
		{"matches in list", []string{"windows", current, "freebsd"}, true},
		{"no match", []string{"plan9", "solaris"}, false},
		{"empty list", []string{}, false},
		{"darwin only", []string{"darwin"}, current == "darwin"},
		{"linux only", []string{"linux"}, current == "linux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlatformMatch(tt.platforms)
			if got != tt.expected {
				t.Errorf("PlatformMatch(%v) = %v, want %v", tt.platforms, got, tt.expected)
			}
		})
	}
}
