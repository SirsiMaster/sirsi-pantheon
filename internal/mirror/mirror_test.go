package mirror

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyMedia(t *testing.T) {
	tests := []struct {
		ext      string
		expected MediaType
	}{
		{".jpg", MediaPhoto},
		{".jpeg", MediaPhoto},
		{".png", MediaPhoto},
		{".heic", MediaPhoto},
		{".raw", MediaPhoto},
		{".mp3", MediaMusic},
		{".flac", MediaMusic},
		{".m4a", MediaMusic},
		{".mp4", MediaVideo},
		{".mov", MediaVideo},
		{".pdf", MediaDocument},
		{".docx", MediaDocument},
		{".xyz", MediaOther},
		{"", MediaOther},
		{".go", MediaOther},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := ClassifyMedia(tt.ext)
			if got != tt.expected {
				t.Errorf("ClassifyMedia(%q) = %q, want %q", tt.ext, got, tt.expected)
			}
		})
	}
}

func TestScan_EmptyPaths(t *testing.T) {
	_, err := Scan(ScanOptions{})
	if err == nil {
		t.Error("Scan with empty paths should error")
	}
}

func TestScan_NoDuplicates(t *testing.T) {
	dir := t.TempDir()

	// Create unique files
	writeFile(t, filepath.Join(dir, "file1.txt"), "content one")
	writeFile(t, filepath.Join(dir, "file2.txt"), "content two")
	writeFile(t, filepath.Join(dir, "file3.txt"), "content three")

	result, err := Scan(ScanOptions{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.TotalScanned != 3 {
		t.Errorf("TotalScanned = %d, want 3", result.TotalScanned)
	}
	if result.TotalDuplicates != 0 {
		t.Errorf("TotalDuplicates = %d, want 0", result.TotalDuplicates)
	}
	if len(result.Groups) != 0 {
		t.Errorf("Groups = %d, want 0", len(result.Groups))
	}
}

func TestScan_FindsDuplicates(t *testing.T) {
	dir := t.TempDir()
	subA := filepath.Join(dir, "a")
	subB := filepath.Join(dir, "b")
	os.MkdirAll(subA, 0755)
	os.MkdirAll(subB, 0755)

	// Same content in two places
	content := "this is duplicate content that should be detected"
	writeFile(t, filepath.Join(subA, "original.txt"), content)
	writeFile(t, filepath.Join(subB, "copy.txt"), content)

	// Plus a unique file
	writeFile(t, filepath.Join(dir, "unique.txt"), "totally different")

	result, err := Scan(ScanOptions{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.TotalScanned != 3 {
		t.Errorf("TotalScanned = %d, want 3", result.TotalScanned)
	}
	if result.TotalDuplicates != 1 {
		t.Errorf("TotalDuplicates = %d, want 1", result.TotalDuplicates)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("Groups = %d, want 1", len(result.Groups))
	}

	group := result.Groups[0]
	if len(group.Files) != 2 {
		t.Errorf("Group files = %d, want 2", len(group.Files))
	}
	if group.MatchType != MatchExact {
		t.Errorf("MatchType = %q, want %q", group.MatchType, MatchExact)
	}
	if group.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", group.Confidence)
	}
	if group.WasteBytes == 0 {
		t.Error("WasteBytes should be > 0")
	}
}

func TestScan_ProtectedDirs(t *testing.T) {
	dir := t.TempDir()
	origDir := filepath.Join(dir, "originals")
	copyDir := filepath.Join(dir, "copies")
	os.MkdirAll(origDir, 0755)
	os.MkdirAll(copyDir, 0755)

	content := "protect this important content"
	writeFile(t, filepath.Join(origDir, "important.txt"), content)
	writeFile(t, filepath.Join(copyDir, "copy.txt"), content)

	result, err := Scan(ScanOptions{
		Paths:       []string{dir},
		ProtectDirs: []string{origDir},
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(result.Groups) != 1 {
		t.Fatalf("Groups = %d, want 1", len(result.Groups))
	}

	group := result.Groups[0]
	// The protected file should be recommended (index 0)
	keeper := group.Files[group.Recommended]
	if !keeper.IsProtected {
		t.Error("Recommended keeper should be the protected file")
	}
}

func TestScan_MinSizeFilter(t *testing.T) {
	dir := t.TempDir()

	// Small file (below threshold)
	writeFile(t, filepath.Join(dir, "tiny.txt"), "sm")
	// Larger file
	writeFile(t, filepath.Join(dir, "big.txt"), "this is a larger file with more content than the small one to exceed minimum")

	result, err := Scan(ScanOptions{
		Paths:   []string{dir},
		MinSize: 50, // 50 bytes minimum
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.TotalScanned != 1 {
		t.Errorf("TotalScanned = %d, want 1 (small file should be filtered)", result.TotalScanned)
	}
}

func TestScan_MediaFilter(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "photo.jpg"), "fake jpg content here!!")
	writeFile(t, filepath.Join(dir, "song.mp3"), "fake mp3 content here!!")
	writeFile(t, filepath.Join(dir, "doc.txt"), "some text document stuff!")

	result, err := Scan(ScanOptions{
		Paths:       []string{dir},
		MediaFilter: MediaPhoto,
	})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.TotalScanned != 1 {
		t.Errorf("TotalScanned = %d, want 1 (only .jpg)", result.TotalScanned)
	}
}

func TestScan_MultipleDuplicateGroups(t *testing.T) {
	dir := t.TempDir()

	// Group 1: two copies
	writeFile(t, filepath.Join(dir, "a1.txt"), "content alpha alpha")
	writeFile(t, filepath.Join(dir, "a2.txt"), "content alpha alpha")

	// Group 2: three copies
	writeFile(t, filepath.Join(dir, "b1.txt"), "content beta beta!!")
	writeFile(t, filepath.Join(dir, "b2.txt"), "content beta beta!!")
	writeFile(t, filepath.Join(dir, "b3.txt"), "content beta beta!!")

	// Unique
	writeFile(t, filepath.Join(dir, "unique.txt"), "i am unique content")

	result, err := Scan(ScanOptions{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.TotalScanned != 6 {
		t.Errorf("TotalScanned = %d, want 6", result.TotalScanned)
	}
	if len(result.Groups) != 2 {
		t.Errorf("Groups = %d, want 2", len(result.Groups))
	}
	if result.TotalDuplicates != 3 {
		t.Errorf("TotalDuplicates = %d, want 3 (1 from group1 + 2 from group2)", result.TotalDuplicates)
	}
}

func TestScan_SkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "empty.txt"), "")
	writeFile(t, filepath.Join(dir, "real.txt"), "has content")

	result, err := Scan(ScanOptions{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if result.TotalScanned != 1 {
		t.Errorf("TotalScanned = %d, want 1 (empty file should be skipped)", result.TotalScanned)
	}
}

func TestScan_SortByWaste(t *testing.T) {
	dir := t.TempDir()

	// Small duplicates
	writeFile(t, filepath.Join(dir, "s1.txt"), "tiny")
	writeFile(t, filepath.Join(dir, "s2.txt"), "tiny")

	// Large duplicates (more waste)
	bigContent := make([]byte, 10000)
	for i := range bigContent {
		bigContent[i] = byte('A' + i%26)
	}
	writeFile(t, filepath.Join(dir, "big1.dat"), string(bigContent))
	writeFile(t, filepath.Join(dir, "big2.dat"), string(bigContent))

	result, err := Scan(ScanOptions{Paths: []string{dir}})
	if err != nil {
		t.Fatalf("Scan error: %v", err)
	}

	if len(result.Groups) != 2 {
		t.Fatalf("Groups = %d, want 2", len(result.Groups))
	}

	// First group should have more waste
	if result.Groups[0].WasteBytes < result.Groups[1].WasteBytes {
		t.Error("Groups should be sorted by waste (largest first)")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHashFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	writeFile(t, path, "hello mirror")

	hash, err := hashFileSHA256(path)
	if err != nil {
		t.Fatalf("hashFileSHA256 error: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("Hash length = %d, want 64", len(hash))
	}

	// Same content = same hash
	path2 := filepath.Join(dir, "test2.txt")
	writeFile(t, path2, "hello mirror")
	hash2, _ := hashFileSHA256(path2)
	if hash != hash2 {
		t.Error("Same content should produce same hash")
	}

	// Different content = different hash
	path3 := filepath.Join(dir, "test3.txt")
	writeFile(t, path3, "different content")
	hash3, _ := hashFileSHA256(path3)
	if hash == hash3 {
		t.Error("Different content should produce different hash")
	}
}

func TestHashFileSHA256_NotExists(t *testing.T) {
	_, err := hashFileSHA256("/nonexistent/file.txt")
	if err == nil {
		t.Error("Should error on nonexistent file")
	}
}

// writeFile is a test helper that creates a file with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}
