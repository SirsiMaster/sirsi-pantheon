// Package updater provides version checking and advisory notifications.
// "Phone home" — checks GitHub Releases API for new versions and advisories.
// Rule A11: NO telemetry. NO tracking. Only checks public GitHub API.
package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// GitHubReleasesAPI is the endpoint for listing all releases.
	GitHubReleasesAPI = "https://api.github.com/repos/SirsiMaster/sirsi-pantheon/releases?per_page=10"

	// AdvisoryURL is checked for post-release roadblocks and known issues.
	AdvisoryURL = "https://raw.githubusercontent.com/SirsiMaster/sirsi-pantheon/main/ADVISORY.json"

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

type Client struct {
	ReleasesURL string
	AdvisoryURL string
	HTTPClient  *http.Client
}

func NewClient() *Client {
	return &Client{
		ReleasesURL: GitHubReleasesAPI,
		AdvisoryURL: AdvisoryURL,
		HTTPClient:  &http.Client{Timeout: checkTimeout},
	}
}

// Check performs a non-blocking version and advisory check.
// It contacts only public GitHub APIs — no telemetry, no tracking.
// Times out after 3 seconds to never slow down the CLI.
func Check(currentVersion string) *UpdateResult {
	return NewClient().Check(currentVersion)
}

func (c *Client) Check(currentVersion string) *UpdateResult {
	result := &UpdateResult{
		CurrentVersion: currentVersion,
	}

	// Fetch all releases and find the newest one
	release, err := c.fetchNewestRelease()
	if err != nil {
		result.Error = err
		return result
	}

	result.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	result.ReleaseURL = release.HTMLURL

	// Only signal update if the remote version is actually newer
	if currentVersion != "dev" && compareVersions(result.LatestVersion, currentVersion) > 0 {
		result.UpdateAvailable = true
	}

	// Find platform-specific download
	result.DownloadURL = findPlatformAsset(release.Assets)

	// Check advisories
	advisories, _ := c.fetchAdvisories(currentVersion)
	result.Advisories = advisories

	return result
}

// fetchNewestRelease fetches all releases and returns the one with the highest
// semver version. This correctly handles pre-releases (which GitHub's /latest
// endpoint skips), preventing false "downgrade" notifications.
func (c *Client) fetchNewestRelease() (*Release, error) {
	resp, err := c.HTTPClient.Get(c.ReleasesURL)
	if err != nil {
		return nil, fmt.Errorf("fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}

	if len(releases) == 0 {
		return nil, fmt.Errorf("no releases found")
	}

	// Find the release with the highest version.
	best := &releases[0]
	for i := 1; i < len(releases); i++ {
		r := &releases[i]
		rVer := strings.TrimPrefix(r.TagName, "v")
		bVer := strings.TrimPrefix(best.TagName, "v")
		if compareVersions(rVer, bVer) > 0 {
			best = r
		}
	}

	return best, nil
}

// fetchAdvisories checks for post-release roadblocks and known issues.
func (c *Client) fetchAdvisories(currentVersion string) ([]Advisory, error) {
	resp, err := c.HTTPClient.Get(c.AdvisoryURL)
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

	// Match pattern: sirsi-pantheon_*_darwin_arm64.tar.gz
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

// compareVersions compares two semver-like version strings (e.g., "0.4.0-alpha").
// Returns: positive if a > b, negative if a < b, 0 if equal.
// Handles major.minor.patch and optional pre-release suffix.
func compareVersions(a, b string) int {
	aParts, aPre := parseVersion(a)
	bParts, bPre := parseVersion(b)

	// Compare numeric parts (major, minor, patch)
	for i := 0; i < 3; i++ {
		av, bv := 0, 0
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av != bv {
			return av - bv
		}
	}

	// Numeric parts are equal — compare pre-release.
	// No pre-release > any pre-release (1.0.0 > 1.0.0-alpha).
	switch {
	case aPre == "" && bPre == "":
		return 0
	case aPre == "":
		return 1 // a is stable, b has pre-release
	case bPre == "":
		return -1 // b is stable, a has pre-release
	default:
		// Both have pre-release — compare alphabetically.
		// This gives correct ordering: alpha < beta < rc
		if aPre < bPre {
			return -1
		}
		if aPre > bPre {
			return 1
		}
		return 0
	}
}

// parseVersion splits "0.4.0-alpha" into ([0, 4, 0], "alpha").
func parseVersion(v string) ([]int, string) {
	preRelease := ""
	if idx := strings.IndexByte(v, '-'); idx != -1 {
		preRelease = v[idx+1:]
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		nums[i] = n
	}
	return nums, preRelease
}
