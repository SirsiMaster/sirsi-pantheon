package guard

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

func TestAuditWith_MockDarwin(t *testing.T) {
	m := &platform.Mock{
		CommandResults: map[string]string{
			"sysctl -n hw.memsize": "17179869184", // 16 GB
			"vm_stat": `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                   100000.
Pages active:                 200000.
Pages inactive:               150000.
Pages wired:                  100000.
`,
			"ps -axo pid,rss,vsz,%cpu,user,command": `  PID   RSS   VSZ  %CPU USER     COMM
    1   1024  4096  0.1  root     /sbin/launchd
  100  51200  81920 1.5  user     /usr/local/bin/node
  200  102400 204800 2.0 user     /usr/local/bin/gopls
`,
		},
	}

	result, err := AuditWith(m)
	if err != nil {
		t.Fatalf("AuditWith failed: %v", err)
	}

	// 16 GB total
	if result.TotalRAM != 17179869184 {
		t.Errorf("TotalRAM = %d, want 17179869184", result.TotalRAM)
	}

	// Used = (active + wired) * 16384 = (200000 + 100000) * 16384 = 300000 * 16384 = 4,915,200,000
	if result.UsedRAM != 300000*16384 {
		t.Errorf("UsedRAM = %d, want %d", result.UsedRAM, 300000*16384)
	}

	// Groups: node (PID 100), lsp (PID 200), other (PID 1)
	if len(result.Groups) < 3 {
		t.Fatalf("expected at least 3 groups, got %d", len(result.Groups))
	}

	// gopls (lsp) should be first (sorted by RSS) — 100 MB vs 50 MB
	if result.Groups[0].Name != "lsp" {
		t.Errorf("first group = %s, want lsp", result.Groups[0].Name)
	}
}

func TestAuditWith_MockLinux(t *testing.T) {
	m := &platform.Mock{
		// Force Linux detection in mock
		Env: map[string]string{"GOOS": "linux"},
		CommandResults: map[string]string{
			"free -b": `              total        used        free      shared  buff/cache   available
Mem:    16000000000  8000000000  4000000000   100000000  4000000000  7000000000
Swap:    2000000000           0  2000000000
`,
			"ps -axo pid,rss,vsz,%cpu,user,command": `  PID   RSS   VSZ  %CPU USER     COMM
    1   1024  4096  0.0  root     systemd
  123 204800 500000 5.0  user     node
`,
		},
	}

	// Pass a mock that returns "linux" name
	m_linux := &linuxMock{m}

	result, err := AuditWith(m_linux)
	if err != nil {
		t.Fatalf("AuditWith failed: %v", err)
	}

	if result.TotalRAM != 16000000000 {
		t.Errorf("TotalRAM = %d, want 16000000000", result.TotalRAM)
	}
	if result.UsedRAM != 8000000000 {
		t.Errorf("UsedRAM = %d, want 8000000000", result.UsedRAM)
	}
}

// Minimal wrapper to override Name() for generic Mock
type linuxMock struct {
	*platform.Mock
}

func (l *linuxMock) Name() string { return "linux" }
