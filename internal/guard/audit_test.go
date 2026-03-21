package guard

import (
	"os"
	"testing"
)

func TestClassifyProcess(t *testing.T) {
	tests := []struct {
		name     string
		process  ProcessInfo
		expected string
	}{
		{"node process", ProcessInfo{Name: "node", Command: "node index.js"}, "node"},
		{"npm", ProcessInfo{Name: "npm", Command: "/usr/local/bin/npm run dev"}, "node"},
		{"vite", ProcessInfo{Name: "vite", Command: "node ./node_modules/.bin/vite"}, "node"},
		{"gopls", ProcessInfo{Name: "gopls", Command: "/usr/local/bin/gopls"}, "lsp"},
		{"rust-analyzer", ProcessInfo{Name: "rust-analyzer", Command: "/usr/local/bin/rust-analyzer"}, "lsp"},
		{"docker backend", ProcessInfo{Name: "com.docker.backend", Command: "com.docker.backend"}, "docker"},
		{"electron helper", ProcessInfo{Name: "Electron Helper", Command: "/Apps/Code.app/Electron Helper"}, "electron"},
		{"ollama", ProcessInfo{Name: "ollama", Command: "ollama serve"}, "ai"},
		{"generic language server", ProcessInfo{Name: "some-language-server", Command: "some-language-server --stdio"}, "lsp"},
		{"unknown process", ProcessInfo{Name: "Safari", Command: "/Applications/Safari.app/Contents/MacOS/Safari"}, "other"},
		{"cargo build", ProcessInfo{Name: "cargo", Command: "cargo build --release"}, "build"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyProcess(&tt.process)
			if got != tt.expected {
				t.Errorf("classifyProcess(%q) = %q, want %q", tt.process.Name, got, tt.expected)
			}
		})
	}
}

func TestIsProtectedProcess(t *testing.T) {
	tests := []struct {
		name      string
		process   ProcessInfo
		protected bool
	}{
		{"root user", ProcessInfo{PID: 100, User: "root", Name: "sshd"}, true},
		{"kernel_task", ProcessInfo{PID: 0, User: "root", Name: "kernel_task"}, true},
		{"launchd", ProcessInfo{PID: 1, User: "root", Name: "launchd"}, true},
		{"WindowServer", ProcessInfo{PID: 200, User: "_windowserver", Name: "WindowServer"}, true},
		{"Finder", ProcessInfo{PID: 300, User: "user", Name: "Finder"}, true},
		{"Dock", ProcessInfo{PID: 301, User: "user", Name: "Dock"}, true},
		{"self", ProcessInfo{PID: os.Getpid(), User: "user", Name: "anubis"}, true},
		{"regular node", ProcessInfo{PID: 9999, User: "user", Name: "node"}, false},
		{"user docker", ProcessInfo{PID: 8888, User: "user", Name: "com.docker.backend"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProtectedProcess(tt.process)
			if got != tt.protected {
				t.Errorf("isProtectedProcess(%q, PID=%d) = %v, want %v",
					tt.process.Name, tt.process.PID, got, tt.protected)
			}
		})
	}
}

func TestIsValidTarget(t *testing.T) {
	tests := []struct {
		target string
		valid  bool
	}{
		{"node", true},
		{"lsp", true},
		{"docker", true},
		{"electron", true},
		{"build", true},
		{"ai", true},
		{"all", true},
		{"invalid", false},
		{"", false},
		{"Node", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := IsValidTarget(tt.target)
			if got != tt.valid {
				t.Errorf("IsValidTarget(%q) = %v, want %v", tt.target, got, tt.valid)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{500 * 1024 * 1024, "500.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{8 * 1024 * 1024 * 1024, "8.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.expected {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.expected)
			}
		})
	}
}

func TestDetectOrphans(t *testing.T) {
	processes := []ProcessInfo{
		{PID: 1, Name: "node", Group: "node", RSS: 100 * 1024 * 1024},    // 100 MB — orphan
		{PID: 2, Name: "node", Group: "node", RSS: 10 * 1024 * 1024},     // 10 MB — too small
		{PID: 3, Name: "safari", Group: "other", RSS: 500 * 1024 * 1024}, // "other" group — not orphan
		{PID: 4, Name: "gopls", Group: "lsp", RSS: 200 * 1024 * 1024},    // 200 MB — orphan
	}

	orphans := detectOrphans(processes)
	if len(orphans) != 2 {
		t.Fatalf("detectOrphans found %d orphans, want 2", len(orphans))
	}
	// Should be sorted by RSS descending
	if orphans[0].PID != 4 {
		t.Errorf("first orphan PID = %d, want 4 (gopls, 200MB)", orphans[0].PID)
	}
	if orphans[1].PID != 1 {
		t.Errorf("second orphan PID = %d, want 1 (node, 100MB)", orphans[1].PID)
	}
}

func TestAudit(t *testing.T) {
	// Integration test — runs on actual system
	result, err := Audit()
	if err != nil {
		t.Fatalf("Audit() error = %v", err)
	}
	if result.TotalRAM == 0 {
		t.Error("TotalRAM = 0")
	}
	if len(result.Groups) == 0 {
		t.Error("no process groups found")
	}
}
