package updater

// install.go extends the version checker with the ability to actually fetch
// and stage a release artifact. Rule A11 still holds: we only read public
// GitHub release assets — no telemetry. The CLI binary is staged here; the
// caller performs the AMFI-safe in-place replace. The macOS .app is delivered
// as the notarized DMG so its Developer-ID signature (and therefore the user's
// Full Disk Access grant) survives the update — ad-hoc local rebuilds cannot.

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// downloadTimeout is generous — release artifacts are tens of MB, unlike the
// 3s version check.
const downloadTimeout = 5 * time.Minute

// maxCLIBinarySize caps the extracted sirsi binary — defends against a malicious
// archive exhausting disk, while being far above any real CLI (tens of MB). A
// var (not const) so tests can shrink it without a multi-hundred-MB fixture.
var maxCLIBinarySize int64 = 512 << 20 // 512 MB

// NewestRelease exposes the newest release (highest semver, pre-releases
// included) for callers that need its assets, not just the version compare.
func (c *Client) NewestRelease() (*Release, error) {
	return c.fetchNewestRelease()
}

// CLITarballAsset returns the darwin/arch sirsi CLI tarball for this release.
func CLITarballAsset(rel *Release) *Asset {
	want := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH) // e.g. darwin_arm64
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if strings.Contains(a.Name, "sirsi-pantheon_") &&
			strings.Contains(a.Name, want) && strings.HasSuffix(a.Name, ".tar.gz") {
			return a
		}
	}
	return nil
}

// AppDMGAsset returns the macOS .app DMG for this release (arch-matched). The
// DMG carries the notarized Developer-ID signature, so installing from it keeps
// the app's identity (and TCC/Full-Disk-Access grant) stable across updates.
func AppDMGAsset(rel *Release) *Asset {
	if runtime.GOOS != "darwin" {
		return nil
	}
	suffix := "-" + runtime.GOARCH + ".dmg" // -arm64.dmg
	for i := range rel.Assets {
		a := &rel.Assets[i]
		if strings.HasPrefix(a.Name, "SirsiPantheon-") && strings.HasSuffix(a.Name, suffix) {
			return a
		}
	}
	return nil
}

// Download streams url to destPath (created/truncated), returning the bytes
// written. A non-200 status is an error and never leaves a partial file behind.
func Download(url, destPath string) (int64, error) {
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return 0, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("create %s: %w", destPath, err)
	}
	n, err := io.Copy(f, resp.Body)
	cerr := f.Close()
	if err != nil {
		_ = os.Remove(destPath)
		return 0, fmt.Errorf("write %s: %w", destPath, err)
	}
	if cerr != nil {
		_ = os.Remove(destPath)
		return 0, cerr
	}
	return n, nil
}

// ExtractSirsiBinary pulls the `sirsi` executable out of a goreleaser .tar.gz
// into destDir and returns its path. It rejects path-traversal entries and
// requires the binary to be present.
func ExtractSirsiBinary(tarballPath, destDir string) (string, error) {
	f, err := os.Open(tarballPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("tar: %w", err)
		}
		base := filepath.Base(hdr.Name)
		// Only the sirsi CLI binary; ignore READMEs, licenses, other deities.
		if hdr.Typeflag != tar.TypeReg || base != "sirsi" {
			continue
		}
		// Reject an implausibly large binary UP FRONT rather than silently
		// truncating it (codex review of #98). The cap is far above any real
		// sirsi CLI (tens of MB); anything larger is bogus, so error instead of
		// installing a half-written binary.
		if hdr.Size > maxCLIBinarySize {
			return "", fmt.Errorf("`sirsi` binary in %s is %d bytes, over the %d-byte safety cap — refusing to install a truncated binary", filepath.Base(tarballPath), hdr.Size, maxCLIBinarySize)
		}
		out := filepath.Join(destDir, "sirsi")
		dst, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		// Copy EXACTLY the declared size — io.CopyN never truncates a valid
		// binary and errors on a short/corrupt stream.
		if _, err := io.CopyN(dst, tr, hdr.Size); err != nil {
			dst.Close()
			_ = os.Remove(out)
			return "", fmt.Errorf("extract sirsi: %w", err)
		}
		if err := dst.Close(); err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("no `sirsi` binary found in %s", filepath.Base(tarballPath))
}
