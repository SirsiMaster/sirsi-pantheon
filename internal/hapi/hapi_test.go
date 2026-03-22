package hapi

import (
	"os"
	"path/filepath"
	"testing"
)

// ═══════════════════════════════════════════
// GPU Detection — types and formatting
// ═══════════════════════════════════════════

func TestGPUType_Constants(t *testing.T) {
	// Ensure GPUType constants are distinct
	types := []GPUType{GPUAppleMetal, GPUNVIDIA, GPUAMD, GPUIntel, GPUNone}
	seen := make(map[GPUType]bool)
	for _, gt := range types {
		if seen[gt] {
			t.Errorf("duplicate GPUType constant: %q", gt)
		}
		seen[gt] = true
	}
}

func TestFormatGPUType(t *testing.T) {
	tests := []struct {
		input GPUType
		want  string
	}{
		{GPUAppleMetal, "Apple Metal"},
		{GPUNVIDIA, "NVIDIA CUDA"},
		{GPUAMD, "AMD ROCm"},
		{GPUIntel, "Intel"},
		{GPUNone, "CPU-only"},
		{GPUType("unknown"), "CPU-only"}, // fallback
	}

	for _, tt := range tests {
		got := FormatGPUType(tt.input)
		if got != tt.want {
			t.Errorf("FormatGPUType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
		{34359738368, "32.0 GB"},
	}

	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ═══════════════════════════════════════════
// DetectHardware — smoke test (doesn't crash)
// ═══════════════════════════════════════════

func TestDetectHardware_ReturnsProfile(t *testing.T) {
	profile, err := DetectHardware()
	if err != nil {
		t.Fatalf("DetectHardware() error: %v", err)
	}
	if profile == nil {
		t.Fatal("DetectHardware() returned nil profile")
	}
	if profile.CPUCores <= 0 {
		t.Errorf("CPUCores = %d, expected positive", profile.CPUCores)
	}
	if profile.CPUArch == "" {
		t.Error("CPUArch should not be empty")
	}
	if profile.OS == "" {
		t.Error("OS should not be empty")
	}
}

// ═══════════════════════════════════════════
// parseDisplayProfile
// ═══════════════════════════════════════════

func TestParseDisplayProfile_AppleSilicon(t *testing.T) {
	sampleOutput := `Graphics/Displays:

    Apple M1 Max:

      Chipset Model: Apple M1 Max
      Type: GPU
      Bus: Built-In
      Total Number of Cores: 32
      Vendor: Apple (0x106b)
      Metal Family: Supported, Metal GPUFamily Apple 8
`

	info := parseDisplayProfile(sampleOutput)
	if info.Name != "Apple M1 Max" {
		t.Errorf("Name = %q, want %q", info.Name, "Apple M1 Max")
	}
	if info.Type != GPUAppleMetal {
		t.Errorf("Type = %q, want %q", info.Type, GPUAppleMetal)
	}
	if info.MetalFamily == "" {
		t.Error("MetalFamily should be populated")
	}
}

func TestParseDisplayProfile_Empty(t *testing.T) {
	info := parseDisplayProfile("")
	if info.Name != "Unknown GPU" {
		t.Errorf("Name = %q, want %q for empty input", info.Name, "Unknown GPU")
	}
	if info.Type != GPUNone {
		t.Errorf("Type = %q, want %q for empty input", info.Type, GPUNone)
	}
}

// ═══════════════════════════════════════════
// parseNvidiaSmi
// ═══════════════════════════════════════════

func TestParseNvidiaSmi_FullOutput(t *testing.T) {
	sampleOutput := "NVIDIA GeForce RTX 4090, 24564, 550.54.14, 8.9"
	info := parseNvidiaSmi(sampleOutput)

	if info.Type != GPUNVIDIA {
		t.Errorf("Type = %q, want %q", info.Type, GPUNVIDIA)
	}
	if info.Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("Name = %q, want %q", info.Name, "NVIDIA GeForce RTX 4090")
	}
	if info.VRAM != 24564*1024*1024 {
		t.Errorf("VRAM = %d, want %d", info.VRAM, 24564*1024*1024)
	}
	if info.DriverVer != "550.54.14" {
		t.Errorf("DriverVer = %q, want %q", info.DriverVer, "550.54.14")
	}
	if info.Compute != "8.9" {
		t.Errorf("Compute = %q, want %q", info.Compute, "8.9")
	}
}

func TestParseNvidiaSmi_PartialOutput(t *testing.T) {
	info := parseNvidiaSmi("GTX 1080")
	if info.Name != "GTX 1080" {
		t.Errorf("Name = %q, want %q", info.Name, "GTX 1080")
	}
	if info.VRAM != 0 {
		t.Errorf("VRAM = %d, want 0 for partial output", info.VRAM)
	}
}

// ═══════════════════════════════════════════
// Deduplication
// ═══════════════════════════════════════════

func TestFindDuplicates_NoDups(t *testing.T) {
	dir := t.TempDir()
	// Create unique files of minimum size
	for i := 0; i < 3; i++ {
		data := make([]byte, 1024) // 1 KB
		for j := range data {
			data[j] = byte(i)
		}
		if err := os.WriteFile(filepath.Join(dir, string(rune('a'+i))+".bin"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := FindDuplicates([]string{dir}, 1024)
	if err != nil {
		t.Fatalf("FindDuplicates() error: %v", err)
	}
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 duplicate groups, got %d", len(result.Groups))
	}
}

func TestFindDuplicates_WithDups(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 2048) // 2 KB
	for i := range data {
		data[i] = 0xAB
	}

	// Write 3 identical files
	for _, name := range []string{"file1.bin", "file2.bin", "file3.bin"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := FindDuplicates([]string{dir}, 1024)
	if err != nil {
		t.Fatalf("FindDuplicates() error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(result.Groups))
	}

	group := result.Groups[0]
	if len(group.Files) != 3 {
		t.Errorf("expected 3 files in group, got %d", len(group.Files))
	}
	if group.Size != 2048 {
		t.Errorf("group size = %d, want 2048", group.Size)
	}
	// Wasted = (3-1) * 2048 = 4096
	if group.Wasted != 4096 {
		t.Errorf("wasted = %d, want 4096", group.Wasted)
	}
	if result.TotalWasted != 4096 {
		t.Errorf("total wasted = %d, want 4096", result.TotalWasted)
	}
}

func TestFindDuplicates_MinSizeFilter(t *testing.T) {
	dir := t.TempDir()
	// Write tiny identical files below threshold
	data := []byte("small")
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := FindDuplicates([]string{dir}, 1024) // min 1KB
	if err != nil {
		t.Fatalf("FindDuplicates() error: %v", err)
	}
	if result.Scanned != 0 {
		t.Errorf("expected 0 scanned files (below minSize), got %d", result.Scanned)
	}
}

func TestFindDuplicates_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := FindDuplicates([]string{dir}, 1024)
	if err != nil {
		t.Fatalf("FindDuplicates() error: %v", err)
	}
	if len(result.Groups) != 0 {
		t.Errorf("expected 0 groups for empty dir, got %d", len(result.Groups))
	}
}

func TestFindDuplicates_MultipleDirectories(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	data := make([]byte, 2048)
	for i := range data {
		data[i] = 0xCD
	}

	if err := os.WriteFile(filepath.Join(dir1, "dup.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "dup.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := FindDuplicates([]string{dir1, dir2}, 1024)
	if err != nil {
		t.Fatalf("FindDuplicates() error: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Errorf("expected 1 duplicate group across dirs, got %d", len(result.Groups))
	}
}

func TestFindDuplicates_SortedByWasted(t *testing.T) {
	dir := t.TempDir()

	// Group A: 2 files × 2KB = 2KB wasted
	dataA := make([]byte, 2048)
	for i := range dataA {
		dataA[i] = 0xAA
	}
	os.WriteFile(filepath.Join(dir, "a1.bin"), dataA, 0644)
	os.WriteFile(filepath.Join(dir, "a2.bin"), dataA, 0644)

	// Group B: 2 files × 4KB = 4KB wasted
	dataB := make([]byte, 4096)
	for i := range dataB {
		dataB[i] = 0xBB
	}
	os.WriteFile(filepath.Join(dir, "b1.bin"), dataB, 0644)
	os.WriteFile(filepath.Join(dir, "b2.bin"), dataB, 0644)

	result, err := FindDuplicates([]string{dir}, 1024)
	if err != nil {
		t.Fatalf("FindDuplicates() error: %v", err)
	}
	if len(result.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(result.Groups))
	}
	// First group should have more wasted space
	if result.Groups[0].Wasted < result.Groups[1].Wasted {
		t.Errorf("groups not sorted by wasted desc: %d < %d",
			result.Groups[0].Wasted, result.Groups[1].Wasted)
	}
}

// ═══════════════════════════════════════════
// hashFile
// ═══════════════════════════════════════════

func TestHashFile_Deterministic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	if err := os.WriteFile(path, []byte("deterministic content"), 0644); err != nil {
		t.Fatal(err)
	}

	h1, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile() error: %v", err)
	}
	h2, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile() second call error: %v", err)
	}
	if h1 != h2 {
		t.Errorf("hashFile not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 { // SHA-256 hex
		t.Errorf("expected 64 char SHA-256 hex, got %d chars", len(h1))
	}
}

func TestHashFile_DifferentContent(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.bin")
	f2 := filepath.Join(dir, "b.bin")
	os.WriteFile(f1, []byte("content A"), 0644)
	os.WriteFile(f2, []byte("content B"), 0644)

	h1, _ := hashFile(f1)
	h2, _ := hashFile(f2)
	if h1 == h2 {
		t.Error("different files should produce different hashes")
	}
}

func TestHashFile_NonExistent(t *testing.T) {
	_, err := hashFile("/nonexistent/path")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// ═══════════════════════════════════════════
// Snapshots
// ═══════════════════════════════════════════

func TestPruneSnapshot_DryRun(t *testing.T) {
	// Dry run should return nil without executing anything
	err := PruneSnapshot("com.apple.TimeMachine.2026-03-21-120000.local", true)
	if err != nil {
		t.Errorf("PruneSnapshot dry run should succeed, got: %v", err)
	}
}

func TestListSnapshots_DoesNotCrash(t *testing.T) {
	// ListSnapshots should return a valid result on any platform
	result, err := ListSnapshots()
	if err != nil {
		t.Fatalf("ListSnapshots() error: %v", err)
	}
	if result == nil {
		t.Fatal("ListSnapshots() returned nil")
	}
	if result.Total != len(result.Snapshots) {
		t.Errorf("Total=%d doesn't match len(Snapshots)=%d", result.Total, len(result.Snapshots))
	}
}

// ═══════════════════════════════════════════
// HardwareProfile struct fields
// ═══════════════════════════════════════════

func TestHardwareProfile_Defaults(t *testing.T) {
	p := HardwareProfile{}
	if p.NeuralEngine {
		t.Error("NeuralEngine should default to false")
	}
	if p.GPU.Type != "" {
		t.Errorf("GPU.Type should default to empty, got %q", p.GPU.Type)
	}
}
