package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCLITarballAsset(t *testing.T) {
	want := "sirsi-pantheon_0.23.2_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	rel := &Release{Assets: []Asset{
		{Name: "checksums.txt"},
		{Name: "sirsi-anubis_0.23.2_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"},
		{Name: want, BrowserDownloadURL: "https://x/tgz"},
	}}
	got := CLITarballAsset(rel)
	if got == nil || got.Name != want {
		t.Fatalf("CLITarballAsset = %v, want %s", got, want)
	}
}

func TestAppDMGAsset(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("DMG asset is darwin-only")
	}
	rel := &Release{Assets: []Asset{
		{Name: "sirsi-pantheon_0.23.2_darwin_" + runtime.GOARCH + ".tar.gz"},
		{Name: "SirsiPantheon-0.23.2-windows-setup.zip"},
		{Name: "SirsiPantheon-0.23.2-" + runtime.GOARCH + ".dmg", BrowserDownloadURL: "https://x/dmg"},
	}}
	got := AppDMGAsset(rel)
	if got == nil || got.BrowserDownloadURL != "https://x/dmg" {
		t.Fatalf("AppDMGAsset = %v, want the %s dmg", got, runtime.GOARCH)
	}
}

func TestAppDMGAsset_NoneWhenMissing(t *testing.T) {
	rel := &Release{Assets: []Asset{{Name: "checksums.txt"}}}
	if got := AppDMGAsset(rel); got != nil {
		t.Fatalf("AppDMGAsset = %v, want nil when no dmg present", got)
	}
}

// TestExtractSirsiBinary builds a tarball with a `sirsi` binary plus a decoy and
// confirms only the binary is extracted, executable.
func TestExtractSirsiBinary(t *testing.T) {
	dir := t.TempDir()
	tgz := filepath.Join(dir, "sirsi-pantheon_test.tar.gz")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range []struct {
		name string
		body string
		mode int64
	}{
		{"README.md", "noise", 0o644},
		{"sirsi", "#!/bin/sh\necho hi\n", 0o755},
	} {
		_ = tw.WriteHeader(&tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(e.body))
	}
	_ = tw.Close()
	_ = gz.Close()
	if err := os.WriteFile(tgz, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := ExtractSirsiBinary(tgz, dir)
	if err != nil {
		t.Fatalf("ExtractSirsiBinary: %v", err)
	}
	if filepath.Base(out) != "sirsi" {
		t.Errorf("extracted %q, want sirsi", out)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Error("extracted sirsi is not executable")
	}
}

func TestExtractSirsiBinary_Missing(t *testing.T) {
	dir := t.TempDir()
	tgz := filepath.Join(dir, "empty.tar.gz")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "LICENSE", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte("x"))
	_ = tw.Close()
	_ = gz.Close()
	_ = os.WriteFile(tgz, buf.Bytes(), 0o644)

	if _, err := ExtractSirsiBinary(tgz, dir); err == nil {
		t.Error("expected error when no sirsi binary is present")
	}
}

// TestExtractSirsiBinary_RejectsOversize proves the fix for codex's #98 finding:
// a binary larger than the cap must ERROR, not be silently truncated.
func TestExtractSirsiBinary_RejectsOversize(t *testing.T) {
	orig := maxCLIBinarySize
	maxCLIBinarySize = 10 // shrink the cap so a tiny fixture trips it
	defer func() { maxCLIBinarySize = orig }()

	dir := t.TempDir()
	tgz := filepath.Join(dir, "big.tar.gz")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := bytes.Repeat([]byte("X"), 50) // 50 bytes > 10-byte cap
	_ = tw.WriteHeader(&tar.Header{Name: "sirsi", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()
	if err := os.WriteFile(tgz, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := ExtractSirsiBinary(tgz, dir)
	if err == nil {
		t.Fatalf("expected error for oversized binary, got out=%q (silent truncation regression)", out)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sirsi")); statErr == nil {
		t.Error("a truncated sirsi file was left behind; an oversized extract must write nothing")
	}
}
