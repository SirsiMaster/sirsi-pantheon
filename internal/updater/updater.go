// Package updater provides version checking and advisory notifications.
// "Phone home" — checks GitHub Releases API for new versions and advisories.
// Rule A11: NO telemetry. NO tracking. Only checks public GitHub API.
package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	// GitHubReleasesAPI is the public endpoint for version checks.
	GitHubReleasesAPI = "https://api.github.com/repos/SirsiMaster/sirsi-anubis/releases/latest"

	// AdvisoryURL is checked for post-release roadblocks and known issues.
	AdvisoryURL = "https://raw.githubusercontent.com/SirsiMaster/sirsi-anubis/main/ADVISORY.json"

	// checkTimeout is the maximum time to wait for a response.
	checkTimeout = 3 * time.Second
)

// Release represents a GitHub release.
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	HTMLURL     string  `json:"html_url"`
	PublishedAt string  `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

// Asset is a downloadable binary attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Advisory represents a post-release notice (roadblocks, critical bugs, etc).
type Advisory struct {
	Version  string   `json:"version"`
	Severity string   `json:"severity"` // "info", "warning", "critical"
	Message  string   `json:"message"`
	Details  string   `json:"details,omitempty"`
	URL      string   `json:"url,omitempty"`
	Affects  []string `json:"affects,omitempty"` // versions affected
}

// AdvisoryFile is the structure of ADVISORY.json in the repo root.
type AdvisoryFile struct {
	Advisories []Advisory `json:"advisories"`
}

// UpdateResult contains the result of a version + advisory check.
type UpdateResult struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseURL      string
	DownloadURL     string // Platform-specific binary URL
	Advisories      []Advisory
	Error           error
}

// Check performs a non-blocking version and advisory check.
// It contacts only public GitHub APIs — no telemetry, no tracking.
// Times out after 3 seconds to never slow down the CLI.
func Check(currentVersion string) *UpdateResult {
	result := &UpdateResult{
		CurrentVersion: currentVersion,
	}

	// Check latest release
	release, err := fetchLatestRelease()
	if err != nil {
		result.Error = err
		return result
	}

	result.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	result.ReleaseURL = release.HTMLURL

	// Compare versions (simple string comparison for now)
	if result.LatestVersion != currentVersion && currentVersion != "dev" {
		result.UpdateAvailable = true
	}

	// Find platform-specific download
	result.DownloadURL = findPlatformAsset(release.Assets)

	// Check advisories
	advisories, _ := fetchAdvisories(currentVersion)
	result.Advisories = advisories

	return result
}

// fetchLatestRelease gets the latest release from GitHub.
func fetchLatestRelease() (*Release, error) {
	client := &http.Client{Timeout: checkTimeout}

	resp, err := client.Get(GitHubReleasesAPI)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	return &release, nil
}

// fetchAdvisories checks for post-release roadblocks and known issues.
func fetchAdvisories(currentVersion string) ([]Advisory, error) {
	client := &http.Client{Timeout: checkTimeout}

	resp, err := client.Get(AdvisoryURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil // No advisories file = no advisories
	}

	var file AdvisoryFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, err
	}

	// Filter advisories that affect the current version
	var relevant []Advisory
	for _, a := range file.Advisories {
		if len(a.Affects) == 0 {
			relevant = append(relevant, a) // Affects all versions
			continue
		}
		for _, v := range a.Affects {
			if v == currentVersion || v == "*" {
				relevant = append(relevant, a)
				break
			}
		}
	}

	return relevant, nil
}

// findPlatformAsset returns the download URL for the current OS/arch.
func findPlatformAsset(assets []Asset) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Match pattern: sirsi-anubis_*_darwin_arm64.tar.gz
	target := fmt.Sprintf("%s_%s", os, arch)

	for _, a := range assets {
		if strings.Contains(a.Name, target) && strings.HasSuffix(a.Name, ".tar.gz") {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

// FormatUpdateNotice returns a styled string for CLI display.
func FormatUpdateNotice(result *UpdateResult) string {
	if result.Error != nil || !result.UpdateAvailable {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n  𓂀 Update available: %s → %s\n", result.CurrentVersion, result.LatestVersion))
	if result.DownloadURL != "" {
		sb.WriteString(fmt.Sprintf("     Download: %s\n", result.DownloadURL))
	}
	sb.WriteString(fmt.Sprintf("     Release:  %s\n", result.ReleaseURL))

	return sb.String()
}

// FormatAdvisories returns styled advisory notices.
func FormatAdvisories(advisories []Advisory) string {
	if len(advisories) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, a := range advisories {
		icon := "ℹ️"
		switch a.Severity {
		case "warning":
			icon = "⚠️"
		case "critical":
			icon = "🚨"
		}
		sb.WriteString(fmt.Sprintf("\n  %s [%s] %s\n", icon, a.Severity, a.Message))
		if a.Details != "" {
			sb.WriteString(fmt.Sprintf("     %s\n", a.Details))
		}
		if a.URL != "" {
			sb.WriteString(fmt.Sprintf("     More info: %s\n", a.URL))
		}
	}
	return sb.String()
}
