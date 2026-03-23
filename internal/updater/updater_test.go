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
		if strings.Contains(r.URL.Path, "latest") {
			json.NewEncoder(w).Encode(Release{
				TagName: "v0.5.0",
				Assets: []Asset{
					{Name: "sirsi-pantheon_0.5.0_darwin_arm64.tar.gz", BrowserDownloadURL: "https://dl"},
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
		ReleasesURL: server.URL + "/latest",
		AdvisoryURL: server.URL + "/advisory",
		HTTPClient:  server.Client(),
	}

	result := client.Check("0.4.0")
	if !result.UpdateAvailable {
		t.Error("Update should be available")
	}
	if result.LatestVersion != "0.5.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "0.5.0")
	}
	if len(result.Advisories) != 1 {
		t.Error("Should have 1 advisory")
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
