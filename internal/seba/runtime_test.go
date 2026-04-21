package seba

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── Phase 2 Runtime Mapper Tests ────────────────────────────────────

func TestRuntimeMapperRegistration(t *testing.T) {
	t.Parallel()
	// Phase 2 adds 8 runtime mappers
	rtTypes := []DiagramType{
		DiagramMemoryPressure,
		DiagramCPUTopology,
		DiagramGPUArchitecture,
		DiagramProcessMap,
		DiagramNetworkPorts,
		DiagramSSHConnections,
		DiagramDiskUsage,
		DiagramSystemOverview,
	}
	for _, dt := range rtTypes {
		if _, ok := mapperRegistry[dt]; !ok {
			t.Errorf("runtime mapper %s not registered", dt)
		}
	}
}

func TestAllDiagramTypes_IncludesRuntime(t *testing.T) {
	t.Parallel()
	all := AllDiagramTypes()
	if len(all) < 23 {
		t.Errorf("AllDiagramTypes = %d, want at least 23 (6 core + 9 Phase1 + 8 Phase2)", len(all))
	}
}

func TestGenerateMemoryPressure(t *testing.T) {
	if testing.Short() {
		t.Skip("memory pressure requires live system")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported platform")
	}

	r, err := GenerateDiagram(".", DiagramMemoryPressure)
	if err != nil {
		t.Fatalf("mempress: %v", err)
	}
	if !strings.Contains(r.Mermaid, "System RAM") {
		t.Error("should mention System RAM")
	}
	if !strings.Contains(r.Mermaid, "Used") {
		t.Error("should show used memory")
	}
	if !strings.Contains(r.Title, "Memory Pressure") {
		t.Error("title should mention Memory Pressure")
	}
}

func TestGenerateCPUTopology(t *testing.T) {
	if testing.Short() {
		t.Skip("CPU topology requires live system")
	}

	r, err := GenerateDiagram(".", DiagramCPUTopology)
	if err != nil {
		t.Fatalf("cpu: %v", err)
	}
	if !strings.Contains(r.Mermaid, "CPU") {
		t.Error("should mention CPU")
	}
	if !strings.Contains(r.Mermaid, "cores") {
		t.Error("should mention cores")
	}
}

func TestGenerateGPUArchitecture(t *testing.T) {
	if testing.Short() {
		t.Skip("GPU detection requires live system")
	}

	r, err := GenerateDiagram(".", DiagramGPUArchitecture)
	if err != nil {
		t.Fatalf("gpu: %v", err)
	}
	if !strings.Contains(r.Mermaid, "GPU") || !strings.Contains(r.Mermaid, "SoC") {
		t.Error("should show GPU and SoC")
	}
}

func TestGenerateProcessMap(t *testing.T) {
	if testing.Short() {
		t.Skip("process map requires live system")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported platform")
	}

	r, err := GenerateDiagram(".", DiagramProcessMap)
	if err != nil {
		t.Fatalf("procmap: %v", err)
	}
	if !strings.Contains(r.Mermaid, "PID") {
		t.Error("should show PIDs")
	}
	if !strings.Contains(r.Mermaid, "MB") {
		t.Error("should show MB")
	}
}

func TestGenerateNetworkPorts(t *testing.T) {
	if testing.Short() {
		t.Skip("port scan requires live system")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported platform")
	}

	r, err := GenerateDiagram(".", DiagramNetworkPorts)
	if err != nil {
		t.Fatalf("ports: %v", err)
	}
	if !strings.Contains(r.Mermaid, "Machine") {
		t.Error("should show Machine node")
	}
}

func TestGenerateSSHConnections(t *testing.T) {
	// Not parallel — mutates HOME env var.
	tmp := t.TempDir()
	sshDir := filepath.Join(tmp, ".ssh")
	os.MkdirAll(sshDir, 0o700)
	os.WriteFile(filepath.Join(sshDir, "config"), []byte(`
Host production
    HostName 10.0.1.50
    User deploy
    Port 2222

Host staging
    HostName staging.example.com
    User admin
`), 0o600)

	t.Setenv("HOME", tmp)

	r, err := generateSSHConnections("")
	if err != nil {
		t.Fatalf("ssh: %v", err)
	}
	if !strings.Contains(r.Mermaid, "production") {
		t.Error("should show production host")
	}
	if !strings.Contains(r.Mermaid, "staging") {
		t.Error("should show staging host")
	}
	if !strings.Contains(r.Mermaid, "2222") {
		t.Error("should show custom port")
	}
}

func TestGenerateSSHConnections_NoConfig(t *testing.T) {
	// Not parallel — mutates HOME env var.
	tmp := t.TempDir()

	t.Setenv("HOME", tmp)

	_, err := generateSSHConnections("")
	if err == nil {
		t.Error("should error when no SSH config")
	}
}

func TestGenerateDiskUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("disk usage requires live system")
	}

	r, err := GenerateDiagram(".", DiagramDiskUsage)
	if err != nil {
		t.Fatalf("disk: %v", err)
	}
	if !strings.Contains(r.Mermaid, "disk") {
		t.Error("should have disk nodes")
	}
}

func TestGenerateSystemOverview(t *testing.T) {
	if testing.Short() {
		t.Skip("system overview requires live system")
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("unsupported platform")
	}

	r, err := GenerateDiagram(".", DiagramSystemOverview)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if !strings.Contains(r.Mermaid, "CPU") {
		t.Error("should show CPU")
	}
	if !strings.Contains(r.Mermaid, "RAM") {
		t.Error("should show RAM")
	}
	if !strings.Contains(r.Mermaid, "GPU") {
		t.Error("should show GPU")
	}
}
