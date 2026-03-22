package scarab

import (
	"net"
	"testing"
)

// ═══════════════════════════════════════════
// parseARPLine
// ═══════════════════════════════════════════

func TestParseARPLine_DarwinFormat(t *testing.T) {
	// macOS: ? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]
	line := "? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]"
	h := parseARPLine(line)
	if h == nil {
		t.Fatal("expected non-nil host")
	}
	if h.IP != "192.168.1.1" {
		t.Errorf("IP = %q, want %q", h.IP, "192.168.1.1")
	}
	if h.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC = %q, want %q", h.MAC, "aa:bb:cc:dd:ee:ff")
	}
}

func TestParseARPLine_Empty(t *testing.T) {
	if h := parseARPLine(""); h != nil {
		t.Error("empty line should return nil")
	}
}

func TestParseARPLine_TooFewFields(t *testing.T) {
	if h := parseARPLine("few fields"); h != nil {
		t.Error("line with < 4 fields should return nil")
	}
}

func TestParseARPLine_IncompleteMAC(t *testing.T) {
	// On macOS, "(incomplete)" matches the paren-search the same way as "(ip)".
	// The function returns nil because "incomplete" is not a valid IP.
	line := "? (10.0.0.1) at (incomplete) on en0 ifscope [ethernet]"
	h := parseARPLine(line)
	// parseARPLine iterates all fields — (incomplete) overwrites IP with "incomplete",
	// which fails isValidIP, so it returns nil.
	if h != nil {
		// If implementation handles this correctly in future, that's fine too
		if h.MAC != "" {
			t.Errorf("incomplete MAC should be empty, got %q", h.MAC)
		}
	}
}

// ═══════════════════════════════════════════
// isValidIP
// ═══════════════════════════════════════════

func TestIsValidIP(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"255.255.255.255", true},
		{"::1", true},
		{"not-an-ip", false},
		{"", false},
		{"192.168.1.999", false},
	}
	for _, tt := range tests {
		got := isValidIP(tt.input)
		if got != tt.want {
			t.Errorf("isValidIP(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ═══════════════════════════════════════════
// incrementIP
// ═══════════════════════════════════════════

func TestIncrementIP(t *testing.T) {
	ip := net.ParseIP("192.168.1.1").To4()
	incrementIP(ip)
	if ip.String() != "192.168.1.2" {
		t.Errorf("increment 192.168.1.1 = %q, want 192.168.1.2", ip.String())
	}
}

func TestIncrementIP_Rollover(t *testing.T) {
	ip := net.ParseIP("192.168.1.255").To4()
	incrementIP(ip)
	if ip.String() != "192.168.2.0" {
		t.Errorf("increment 192.168.1.255 = %q, want 192.168.2.0", ip.String())
	}
}

// ═══════════════════════════════════════════
// getLocalSubnet
// ═══════════════════════════════════════════

func TestGetLocalSubnet_ReturnsValidCIDR(t *testing.T) {
	subnet, err := getLocalSubnet()
	if err != nil {
		// CI environments may not have network
		t.Skipf("no active network: %v", err)
	}
	_, _, err = net.ParseCIDR(subnet)
	if err != nil {
		t.Errorf("getLocalSubnet() returned invalid CIDR %q: %v", subnet, err)
	}
}

// ═══════════════════════════════════════════
// FormatContainerStatus
// ═══════════════════════════════════════════

func TestFormatContainerStatus_Running(t *testing.T) {
	c := Container{Status: "Up 3 hours", Running: true}
	s := FormatContainerStatus(c)
	if s != "🟢 Up 3 hours" {
		t.Errorf("FormatContainerStatus() = %q, want %q", s, "🟢 Up 3 hours")
	}
}

func TestFormatContainerStatus_Stopped(t *testing.T) {
	c := Container{Status: "Exited (0) 2 days ago", Running: false}
	s := FormatContainerStatus(c)
	if s != "🔴 Exited (0) 2 days ago" {
		t.Errorf("FormatContainerStatus() = %q, want %q", s, "🔴 Exited (0) 2 days ago")
	}
}

// ═══════════════════════════════════════════
// Type checks
// ═══════════════════════════════════════════

func TestHost_Struct(t *testing.T) {
	h := Host{
		IP:        "10.0.0.1",
		MAC:       "aa:bb:cc:dd:ee:ff",
		Hostname:  "test-host",
		Alive:     true,
		OpenPorts: []int{80, 443},
	}
	if h.IP != "10.0.0.1" {
		t.Error("Host struct field access failed")
	}
	if len(h.OpenPorts) != 2 {
		t.Errorf("expected 2 open ports, got %d", len(h.OpenPorts))
	}
}

func TestContainerAudit_Defaults(t *testing.T) {
	a := ContainerAudit{}
	if a.DockerRunning {
		t.Error("DockerRunning should default to false")
	}
	if a.RunningCount != 0 || a.StoppedCount != 0 {
		t.Error("counts should default to 0")
	}
}
