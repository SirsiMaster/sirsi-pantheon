package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_Check(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases") {
			// Return an array of releases — fetchNewestRelease expects a list.
			json.NewEncoder(w).Encode([]Release{
				{
					TagName: "v0.3.0-alpha",
					Assets: []Asset{
						{Name: "sirsi-pantheon_0.3.0-alpha_darwin_arm64.tar.gz", BrowserDownloadURL: "https://dl/old"},
					},
				},
				{
					TagName: "v0.5.0",
					Assets: []Asset{
						{Name: "sirsi-pantheon_0.5.0_darwin_arm64.tar.gz", BrowserDownloadURL: "https://dl"},
					},
				},
				{
					TagName: "v0.2.0-alpha",
					Assets: []Asset{
						{Name: "sirsi-pantheon_0.2.0-alpha_darwin_arm64.tar.gz", BrowserDownloadURL: "https://dl/oldest"},
					},
				},
			})
		} else {
			json.NewEncoder(w).Encode(AdvisoryFile{
				Advisories: []Advisory{
					{Message: "Test Advisory", Severity: "warning"},
				},
			})
		}
	}))
	defer server.Close()

	client := &Client{
		ReleasesURL: server.URL + "/releases",
		AdvisoryURL: server.URL + "/advisory",
		HTTPClient:  server.Client(),
	}

	result := client.Check("0.4.0-alpha")
	if !result.UpdateAvailable {
		t.Error("Update should be available (0.5.0 > 0.4.0-alpha)")
	}
	if result.LatestVersion != "0.5.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "0.5.0")
	}
	if len(result.Advisories) != 1 {
		t.Error("Should have 1 advisory")
	}
}

func TestClient_Check_NoUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases") {
			json.NewEncoder(w).Encode([]Release{
				{TagName: "v0.4.0-alpha"},
			})
		} else {
			json.NewEncoder(w).Encode(AdvisoryFile{})
		}
	}))
	defer server.Close()

	client := &Client{
		ReleasesURL: server.URL + "/releases",
		AdvisoryURL: server.URL + "/advisory",
		HTTPClient:  server.Client(),
	}

	result := client.Check("0.4.0-alpha")
	if result.UpdateAvailable {
		t.Error("Should NOT show update when versions are equal")
	}
}

func TestClient_Check_OlderRelease(t *testing.T) {
	// This was the original bug: GitHub returned v0.2.0-alpha as "latest"
	// because pre-releases are skipped. The updater should NOT suggest a downgrade.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "releases") {
			json.NewEncoder(w).Encode([]Release{
				{TagName: "v0.2.0-alpha"},
			})
		} else {
			json.NewEncoder(w).Encode(AdvisoryFile{})
		}
	}))
	defer server.Close()

	client := &Client{
		ReleasesURL: server.URL + "/releases",
		AdvisoryURL: server.URL + "/advisory",
		HTTPClient:  server.Client(),
	}

	result := client.Check("0.4.0-alpha")
	if result.UpdateAvailable {
		t.Error("Should NOT suggest downgrade from 0.4.0-alpha to 0.2.0-alpha")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int // positive = a > b, negative = a < b, 0 = equal
	}{
		// Identical
		{"0.4.0-alpha", "0.4.0-alpha", 0},
		{"1.0.0", "1.0.0", 0},

		// Major differences
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},

		// Minor differences
		{"0.5.0", "0.4.0", 1},
		{"0.3.0", "0.4.0", -1},

		// Patch differences
		{"0.4.1", "0.4.0", 1},
		{"0.4.0", "0.4.1", -1},

		// Pre-release vs stable (stable > pre-release)
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0", -1},

		// Pre-release ordering (alpha < beta < rc)
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-rc", "1.0.0-beta", 1},

		// Different major with pre-release
		{"0.5.0-alpha", "0.4.0-alpha", 1},
		{"0.4.0-alpha", "0.2.0-alpha", 1},
		{"0.2.0-alpha", "0.4.0-alpha", -1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			switch {
			case tt.want > 0 && got <= 0:
				t.Errorf("compareVersions(%q, %q) = %d, want positive", tt.a, tt.b, got)
			case tt.want < 0 && got >= 0:
				t.Errorf("compareVersions(%q, %q) = %d, want negative", tt.a, tt.b, got)
			case tt.want == 0 && got != 0:
				t.Errorf("compareVersions(%q, %q) = %d, want 0", tt.a, tt.b, got)
			}
		})
	}
}

func TestFindPlatformAsset(t *testing.T) {
	assets := []Asset{
		{Name: "sirsi-pantheon_0.3.0_darwin_arm64.tar.gz", BrowserDownloadURL: "https://dl/mac-arm64"},
		{Name: "sirsi-pantheon_0.3.0_darwin_amd64.tar.gz", BrowserDownloadURL: "https://dl/mac-amd64"},
		{Name: "sirsi-pantheon_0.3.0_linux_arm64.tar.gz", BrowserDownloadURL: "https://dl/linux-arm64"},
		{Name: "sirsi-pantheon_0.3.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://dl/linux-amd64"},
	}

	url := findPlatformAsset(assets)
	if url == "" {
		t.Log("Testing on platform not in mockup list (windows?)")
	} else {
		if !strings.HasPrefix(url, "https://dl/") {
			t.Errorf("Unexpected download URL: %q", url)
		}
	}
}

func TestFormatUpdateNotice(t *testing.T) {
	result := &UpdateResult{
		CurrentVersion:  "0.3.0",
		LatestVersion:   "0.4.0",
		UpdateAvailable: true,
		ReleaseURL:      "https://rel",
		DownloadURL:     "https://dl",
	}

	notice := FormatUpdateNotice(result)
	if !strings.Contains(notice, "0.3.0") || !strings.Contains(notice, "0.4.0") {
		t.Errorf("Unexpected notice: %q", notice)
	}

	result.UpdateAvailable = false
	if FormatUpdateNotice(result) != "" {
		t.Error("Should return empty for no update")
	}
}

func TestFormatAdvisories(t *testing.T) {
	advisories := []Advisory{
		{Severity: "warning", Message: "Slow scan", Details: "Avoid /tmp"},
		{Severity: "critical", Message: "Security fix", URL: "https://vuln"},
		{Severity: "info", Message: "Welcome"},
	}

	output := FormatAdvisories(advisories)
	if !strings.Contains(output, "⚠️ [warning] Slow scan") {
		t.Errorf("Missing warning: %q", output)
	}
	if !strings.Contains(output, "🚨 [critical] Security fix") {
		t.Errorf("Missing critical: %q", output)
	}
	if !strings.Contains(output, "ℹ️ [info] Welcome") {
		t.Errorf("Missing info: %q", output)
	}

	if FormatAdvisories(nil) != "" {
		t.Error("Should return empty for nil advisories")
	}
}
