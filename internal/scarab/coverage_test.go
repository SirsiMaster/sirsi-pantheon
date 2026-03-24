package scarab

import (
	"fmt"
	"testing"
)

// ═══════════════════════════════════════════
// AuditContainers — parsing logic coverage
// ═══════════════════════════════════════════

func TestParseContainerLine_Running(t *testing.T) {
	line := "abc123def456\tmy-container\tnginx:latest\tUp 3 hours\t0.0.0.0:80->80/tcp"
	parts := splitContainerLine(line)
	if parts == nil {
		t.Fatal("expected non-nil parts")
	}
	if parts.ID != "abc123def456" {
		t.Errorf("ID = %q, want %q", parts.ID, "abc123def456")
	}
	if parts.Name != "my-container" {
		t.Errorf("Name = %q, want %q", parts.Name, "my-container")
	}
	if parts.Image != "nginx:latest" {
		t.Errorf("Image = %q, want %q", parts.Image, "nginx:latest")
	}
	if !parts.Running {
		t.Error("expected Running = true for 'Up' status")
	}
	if parts.Ports != "0.0.0.0:80->80/tcp" {
		t.Errorf("Ports = %q", parts.Ports)
	}
}

func TestParseContainerLine_Stopped(t *testing.T) {
	line := "def789abc012\told-app\tredis:7\tExited (0) 2 days ago\t"
	parts := splitContainerLine(line)
	if parts == nil {
		t.Fatal("expected non-nil parts")
	}
	if parts.Running {
		t.Error("expected Running = false for 'Exited' status")
	}
	if parts.Name != "old-app" {
		t.Errorf("Name = %q, want %q", parts.Name, "old-app")
	}
}

func TestParseContainerLine_Empty(t *testing.T) {
	if parts := splitContainerLine(""); parts != nil {
		t.Error("empty line should return nil")
	}
}

func TestParseContainerLine_TooFewFields(t *testing.T) {
	if parts := splitContainerLine("only\ttwo"); parts != nil {
		t.Error("line with < 4 fields should return nil")
	}
}

func TestParseContainerLine_NoPorts(t *testing.T) {
	line := "abc123\tno-ports\talpine:latest\tUp 1 hour"
	parts := splitContainerLine(line)
	if parts == nil {
		t.Fatal("expected non-nil parts")
	}
	if parts.Ports != "" {
		t.Errorf("expected empty Ports, got %q", parts.Ports)
	}
	if !parts.Running {
		t.Error("expected Running = true")
	}
}

func TestCountLines_DanglingImages(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"three images", "sha1\nsha2\nsha3", 3},
		{"one image", "sha1", 1},
		{"empty", "", 0},
		{"whitespace only", "  \n  ", 0},
		{"mixed", "sha1\n\nsha2\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countNonEmptyLines(tt.input)
			if got != tt.want {
				t.Errorf("countNonEmptyLines(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestContainerAudit_Aggregation(t *testing.T) {
	audit := &ContainerAudit{}
	containers := []Container{
		{ID: "a", Name: "web", Status: "Up 1h", Running: true},
		{ID: "b", Name: "db", Status: "Up 2h", Running: true},
		{ID: "c", Name: "old", Status: "Exited (0)", Running: false},
	}
	for _, c := range containers {
		if c.Running {
			audit.RunningCount++
		} else {
			audit.StoppedCount++
		}
		audit.Containers = append(audit.Containers, c)
	}
	if audit.RunningCount != 2 {
		t.Errorf("RunningCount = %d, want 2", audit.RunningCount)
	}
	if audit.StoppedCount != 1 {
		t.Errorf("StoppedCount = %d, want 1", audit.StoppedCount)
	}
	if len(audit.Containers) != 3 {
		t.Errorf("Containers len = %d, want 3", len(audit.Containers))
	}
}

func TestContainer_Fields(t *testing.T) {
	c := Container{
		ID:      "abc",
		Name:    "test",
		Image:   "nginx",
		Status:  "Up 1h",
		Size:    "5MB",
		Ports:   "80/tcp",
		Running: true,
	}
	if c.ID != "abc" {
		t.Errorf("ID = %q, want %q", c.ID, "abc")
	}
	if c.Name != "test" {
		t.Errorf("Name = %q, want %q", c.Name, "test")
	}
	if c.Image != "nginx" {
		t.Errorf("Image = %q, want %q", c.Image, "nginx")
	}
	if c.Status != "Up 1h" {
		t.Errorf("Status = %q, want %q", c.Status, "Up 1h")
	}
	if c.Size != "5MB" {
		t.Errorf("Size = %q, want %q", c.Size, "5MB")
	}
	if c.Ports != "80/tcp" {
		t.Errorf("Ports = %q, want %q", c.Ports, "80/tcp")
	}
	if !c.Running {
		t.Error("Running should be true")
	}
}

// === MOCKED DEPENDENCY TESTS ===

func saveAndRestoreDocker(t *testing.T) {
	t.Helper()
	origInfo := runDockerInfo
	origPS := runDockerPS
	origImages := runDockerImages
	origVols := runDockerVols
	t.Cleanup(func() {
		runDockerInfo = origInfo
		runDockerPS = origPS
		runDockerImages = origImages
		runDockerVols = origVols
	})
}

func TestAuditContainers_DockerNotRunning(t *testing.T) {
	saveAndRestoreDocker(t)
	runDockerInfo = func() error { return fmt.Errorf("docker not running") }

	audit, err := AuditContainers()
	if err != nil {
		t.Fatalf("AuditContainers: %v", err)
	}
	if audit.DockerRunning {
		t.Error("should not be running")
	}
	if len(audit.Containers) != 0 {
		t.Error("should have no containers")
	}
}

func TestAuditContainers_WithContainers(t *testing.T) {
	saveAndRestoreDocker(t)
	runDockerInfo = func() error { return nil }
	runDockerPS = func() ([]byte, error) {
		return []byte("abc123\tmy-nginx\tnginx:latest\tUp 3 hours\t80/tcp\n" +
			"def456\tmy-redis\tredis:7\tExited (0) 1 day ago\t\n"), nil
	}
	runDockerImages = func() ([]byte, error) {
		return []byte("sha256:aaa\nsha256:bbb\n"), nil
	}
	runDockerVols = func() ([]byte, error) {
		return []byte("vol1\n"), nil
	}

	audit, err := AuditContainers()
	if err != nil {
		t.Fatalf("AuditContainers: %v", err)
	}
	if !audit.DockerRunning {
		t.Error("should be running")
	}
	if audit.RunningCount != 1 {
		t.Errorf("RunningCount = %d, want 1", audit.RunningCount)
	}
	if audit.StoppedCount != 1 {
		t.Errorf("StoppedCount = %d, want 1", audit.StoppedCount)
	}
	if len(audit.Containers) != 2 {
		t.Errorf("Containers len = %d, want 2", len(audit.Containers))
	}
	if audit.DanglingImages != 2 {
		t.Errorf("DanglingImages = %d, want 2", audit.DanglingImages)
	}
	if audit.UnusedVolumes != 1 {
		t.Errorf("UnusedVolumes = %d, want 1", audit.UnusedVolumes)
	}
}

func TestAuditContainers_PSError(t *testing.T) {
	saveAndRestoreDocker(t)
	runDockerInfo = func() error { return nil }
	runDockerPS = func() ([]byte, error) { return nil, fmt.Errorf("ps failed") }
	runDockerImages = func() ([]byte, error) { return nil, fmt.Errorf("images failed") }
	runDockerVols = func() ([]byte, error) { return nil, fmt.Errorf("vols failed") }

	audit, err := AuditContainers()
	if err != nil {
		t.Fatalf("AuditContainers: %v", err)
	}
	if !audit.DockerRunning {
		t.Error("Docker should be running even if ps fails")
	}
	if len(audit.Containers) != 0 {
		t.Error("should have no containers on PS error")
	}
}

func TestAuditContainers_EmptyOutput(t *testing.T) {
	saveAndRestoreDocker(t)
	runDockerInfo = func() error { return nil }
	runDockerPS = func() ([]byte, error) { return []byte(""), nil }
	runDockerImages = func() ([]byte, error) { return []byte(""), nil }
	runDockerVols = func() ([]byte, error) { return []byte(""), nil }

	audit, err := AuditContainers()
	if err != nil {
		t.Fatalf("AuditContainers: %v", err)
	}
	if audit.DanglingImages != 0 {
		t.Errorf("DanglingImages = %d, want 0", audit.DanglingImages)
	}
}

// --- FormatContainerStatus ---

func TestFormatContainerStatus_Running_Mocked(t *testing.T) {
	s := FormatContainerStatus(Container{Running: true, Status: "Up 3 hours"})
	if s != "🟢 Up 3 hours" {
		t.Errorf("got %q", s)
	}
}

func TestFormatContainerStatus_Stopped_Mocked(t *testing.T) {
	s := FormatContainerStatus(Container{Running: false, Status: "Exited (0)"})
	if s != "🔴 Exited (0)" {
		t.Errorf("got %q", s)
	}
}

// --- countNonEmptyLines edge cases ---

func TestCountNonEmptyLines_Mixed(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"a", 1},
		{"a\nb\nc", 3},
		{"a\n\n\nb", 2},
		{"\n\n", 0},
		{"  \n  \n", 0},
	}
	for _, tt := range tests {
		got := countNonEmptyLines(tt.input)
		if got != tt.want {
			t.Errorf("countNonEmptyLines(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
