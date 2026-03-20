// Package ka implements the Ghost Hunter module for Sirsi Anubis.
//
// 𓂓 Ka — the spiritual double that persists after the body dies.
// In Egyptian belief, the Ka remains after death and needs sustenance.
// An uninstalled app's remnants ARE its Ka — the spirit lingers on
// your filesystem, consuming resources, polluting Spotlight, and
// haunting your system. Ka Detection finds these spirits and releases them.
//
// This module is a CENTRAL TENET of Sirsi Anubis — not a feature,
// but a foundational capability that distinguishes Anubis from every
// other cleaning tool.
package ka

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-anubis/internal/cleaner"
)

// Ghost represents a detected Ka — the spirit of a dead application.
type Ghost struct {
	// The name of the dead application
	AppName string

	// The bundle identifier (e.g., "com.parallels.desktop.console")
	BundleID string

	// All residual paths found for this ghost
	Residuals []Residual

	// Total size of all residuals in bytes
	TotalSize int64

	// Total number of files across all residuals
	TotalFiles int

	// Whether this ghost is registered in Launch Services
	InLaunchServices bool

	// How it was detected
	DetectionMethod string
}

// Residual represents a single residual path left by a dead app.
type Residual struct {
	// Absolute path to the residual
	Path string

	// What type of residual this is
	Type ResidualType

	// Size in bytes
	SizeBytes int64

	// Number of files (if directory)
	FileCount int

	// Whether this requires sudo to remove
	RequiresSudo bool
}

// ResidualType categorizes what kind of residual was found.
type ResidualType string

const (
	ResidualPreferences    ResidualType = "Preferences"
	ResidualAppSupport     ResidualType = "Application Support"
	ResidualCaches         ResidualType = "Caches"
	ResidualContainers     ResidualType = "Containers"
	ResidualGroupContainers ResidualType = "Group Containers"
	ResidualSavedState     ResidualType = "Saved State"
	ResidualHTTPStorages   ResidualType = "HTTP Storages"
	ResidualWebKit         ResidualType = "WebKit Data"
	ResidualCookies        ResidualType = "Cookies"
	ResidualAppScripts     ResidualType = "Application Scripts"
	ResidualLogs           ResidualType = "Logs"
	ResidualLaunchAgent    ResidualType = "Launch Agent"
	ResidualLaunchDaemon   ResidualType = "Launch Daemon"
	ResidualReceipts       ResidualType = "Package Receipts"
	ResidualLoginItems     ResidualType = "Login Items"
	ResidualCrashReports   ResidualType = "Crash Reports"
	ResidualGhostApp       ResidualType = "Ghost App (Spotlight)"
)

// residualLocation defines where to search for a specific type of residual.
type residualLocation struct {
	Type         ResidualType
	Dir          string // Parent directory (supports ~)
	RequiresSudo bool
}

// userResidualLocations are directories under ~/Library where app remnants hide.
var userResidualLocations = []residualLocation{
	{ResidualPreferences, "~/Library/Preferences", false},
	{ResidualAppSupport, "~/Library/Application Support", false},
	{ResidualCaches, "~/Library/Caches", false},
	{ResidualContainers, "~/Library/Containers", false},
	{ResidualGroupContainers, "~/Library/Group Containers", false},
	{ResidualSavedState, "~/Library/Saved Application State", false},
	{ResidualHTTPStorages, "~/Library/HTTPStorages", false},
	{ResidualWebKit, "~/Library/WebKit", false},
	{ResidualCookies, "~/Library/Cookies", false},
	{ResidualAppScripts, "~/Library/Application Scripts", false},
	{ResidualLogs, "~/Library/Logs", false},
	{ResidualCrashReports, "~/Library/Logs/DiagnosticReports", false},
}

// systemResidualLocations require sudo access.
var systemResidualLocations = []residualLocation{
	{ResidualPreferences, "/Library/Preferences", true},
	{ResidualLaunchAgent, "/Library/LaunchAgents", true},
	{ResidualLaunchDaemon, "/Library/LaunchDaemons", true},
	{ResidualReceipts, "/var/db/receipts", true},
	{ResidualAppSupport, "/Library/Application Support", true},
}

// Scanner is the Ka ghost detection engine.
type Scanner struct {
	homeDir         string
	installedApps   map[string]bool   // Bundle IDs of currently installed apps
	installedNames  map[string]bool   // Names of currently installed apps (lowercase)
	knownBundleIDs  map[string]string // Bundle ID → app name mapping
}

// NewScanner creates a new Ka scanner.
func NewScanner() *Scanner {
	homeDir, _ := os.UserHomeDir()
	return &Scanner{
		homeDir:        homeDir,
		installedApps:  make(map[string]bool),
		installedNames: make(map[string]bool),
		knownBundleIDs: make(map[string]string),
	}
}

// Scan discovers all ghosts (Kas) on the system.
// This method has ZERO side effects — it only reads.
func (s *Scanner) Scan(includeSudo bool) ([]Ghost, error) {
	// Step 1: Build inventory of currently installed apps
	if err := s.buildInstalledAppIndex(); err != nil {
		return nil, fmt.Errorf("failed to index installed apps: %w", err)
	}

	// Step 2: Scan all residual locations for orphaned entries
	orphans := s.scanForOrphans(includeSudo)

	// Step 3: Check Launch Services for ghost app registrations
	lsGhosts := s.scanLaunchServices()

	// Step 4: Merge orphans into Ghost structures
	ghosts := s.mergeOrphans(orphans, lsGhosts)

	return ghosts, nil
}

// Clean removes all residuals for a specific ghost.
func (s *Scanner) Clean(ghost Ghost, dryRun bool, useTrash bool) (int64, int, error) {
	var totalFreed int64
	var totalCleaned int

	for _, r := range ghost.Residuals {
		freed, err := cleaner.DeleteFile(r.Path, dryRun, useTrash)
		if err != nil {
			continue // Skip protected paths, log elsewhere
		}
		totalFreed += freed
		totalCleaned++
	}

	return totalFreed, totalCleaned, nil
}

// buildInstalledAppIndex scans /Applications and ~/Applications for .app bundles.
func (s *Scanner) buildInstalledAppIndex() error {
	appDirs := []string{
		"/Applications",
		filepath.Join(s.homeDir, "Applications"),
	}

	for _, dir := range appDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".app") {
				continue
			}

			appPath := filepath.Join(dir, entry.Name())
			appName := strings.TrimSuffix(entry.Name(), ".app")
			s.installedNames[strings.ToLower(appName)] = true

			// Read bundle ID from Info.plist
			bundleID := readBundleID(appPath)
			if bundleID != "" {
				s.installedApps[bundleID] = true
				s.knownBundleIDs[bundleID] = appName
			}
		}
	}

	// Also index Homebrew casks
	s.indexHomebrewCasks()

	return nil
}

// scanForOrphans scans residual locations for entries that don't match installed apps.
func (s *Scanner) scanForOrphans(includeSudo bool) map[string][]Residual {
	orphans := make(map[string][]Residual)

	locations := userResidualLocations
	if includeSudo {
		locations = append(locations, systemResidualLocations...)
	}

	for _, loc := range locations {
		dir := expandPath(loc.Dir, s.homeDir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()
			bundleID := extractBundleID(name)

			if bundleID == "" {
				continue // Can't identify — skip
			}

			// Skip known system/platform bundle IDs — these are NOT ghosts
			if isSystemBundleID(bundleID) {
				continue
			}

			// Is this app still installed?
			if s.isInstalled(bundleID, name) {
				continue // Not a ghost
			}

			path := filepath.Join(dir, name)
			size := int64(0)
			fileCount := 1

			info, err := os.Lstat(path)
			if err != nil {
				continue
			}
			if info.IsDir() {
				size = cleaner.DirSize(path)
				fileCount = countFiles(path)
			} else {
				size = info.Size()
			}

			if size == 0 {
				continue
			}

			residual := Residual{
				Path:         path,
				Type:         loc.Type,
				SizeBytes:    size,
				FileCount:    fileCount,
				RequiresSudo: loc.RequiresSudo,
			}

			// Group by bundle ID
			orphans[bundleID] = append(orphans[bundleID], residual)
		}
	}

	return orphans
}

// scanLaunchServices queries the Launch Services database for registered ghost apps.
func (s *Scanner) scanLaunchServices() map[string]bool {
	ghosts := make(map[string]bool)

	// Use lsregister to dump registered apps
	cmd := exec.Command("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-dump")
	output, err := cmd.Output()
	if err != nil {
		return ghosts
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	var currentBundle string
	var currentPath string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "bundle id:") {
			currentBundle = strings.TrimSpace(strings.TrimPrefix(line, "bundle id:"))
		}
		if strings.HasPrefix(line, "path:") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "path:"))
		}

		// When we have both bundle and path, check if the app exists
		if currentBundle != "" && currentPath != "" {
			if strings.HasSuffix(currentPath, ".app") {
				if _, err := os.Stat(currentPath); os.IsNotExist(err) {
					// App path doesn't exist — it's a ghost in Launch Services
					if !s.installedApps[currentBundle] {
						ghosts[currentBundle] = true
					}
				}
			}
			currentBundle = ""
			currentPath = ""
		}
	}

	return ghosts
}

// mergeOrphans combines filesystem orphans and Launch Services ghosts into Ghost structs.
func (s *Scanner) mergeOrphans(orphans map[string][]Residual, lsGhosts map[string]bool) []Ghost {
	var ghosts []Ghost

	// Create ghost for each orphan group
	seen := make(map[string]bool)
	for bundleID, residuals := range orphans {
		ghost := Ghost{
			AppName:         guessAppName(bundleID),
			BundleID:        bundleID,
			Residuals:       residuals,
			InLaunchServices: lsGhosts[bundleID],
			DetectionMethod:  "filesystem",
		}

		for _, r := range residuals {
			ghost.TotalSize += r.SizeBytes
			ghost.TotalFiles += r.FileCount
		}

		ghosts = append(ghosts, ghost)
		seen[bundleID] = true
	}

	// Add Launch Services ghosts that weren't found via filesystem
	for bundleID := range lsGhosts {
		if seen[bundleID] {
			continue
		}
		ghosts = append(ghosts, Ghost{
			AppName:          guessAppName(bundleID),
			BundleID:         bundleID,
			InLaunchServices: true,
			DetectionMethod:  "launch_services",
		})
	}

	return ghosts
}

// isInstalled checks if a bundle ID or name matches an installed app.
func (s *Scanner) isInstalled(bundleID, fileName string) bool {
	// Check by bundle ID
	if s.installedApps[bundleID] {
		return true
	}

	// Check by name (for entries without standard bundle IDs)
	nameLower := strings.ToLower(fileName)
	// Extract app name from various formats
	for installed := range s.installedNames {
		if strings.Contains(nameLower, installed) {
			return true
		}
	}

	return false
}

// indexHomebrewCasks adds Homebrew cask apps to the installed app index.
func (s *Scanner) indexHomebrewCasks() {
	cmd := exec.Command("brew", "list", "--cask", "-1")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			s.installedNames[strings.ToLower(name)] = true
		}
	}
}

// isSystemBundleID returns true if the bundle ID belongs to a macOS system
// component or known platform service that should NOT be flagged as a ghost.
// These are system services, not user-installed apps.
func isSystemBundleID(bundleID string) bool {
	// Apple system components (macOS, iCloud, Siri, etc.)
	if strings.HasPrefix(bundleID, "com.apple.") {
		return true
	}

	// Known system/platform prefixes that create preferences but
	// are not traditional "apps" in /Applications
	systemPrefixes := []string{
		"com.google.drivefs",       // Google Drive File Stream (background service)
		"com.google.keystone",      // Google update agent
		"com.microsoft.OneDrive",   // OneDrive extension services
		"com.microsoft.autoupdate", // Microsoft AutoUpdate
	}

	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(bundleID, prefix) {
			return true
		}
	}

	return false
}

// extractBundleID attempts to extract a bundle ID from a filename.
// Handles patterns like:
//   - com.parallels.desktop.console.plist → com.parallels.desktop.console
//   - com.parallels.desktop.console (directory name)
//   - group.com.docker → com.docker
func extractBundleID(name string) string {
	// Strip common extensions
	name = strings.TrimSuffix(name, ".plist")
	name = strings.TrimSuffix(name, ".savedState")

	// Must look like a bundle ID (contains dots)
	if !strings.Contains(name, ".") {
		return ""
	}

	// Strip "group." prefix
	name = strings.TrimPrefix(name, "group.")

	// Must start with com., org., net., io., etc.
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return ""
	}

	// Common TLD prefixes for bundle IDs
	validPrefixes := []string{"com", "org", "net", "io", "dev", "me", "co", "app", "de", "uk", "fr", "jp"}
	for _, prefix := range validPrefixes {
		if parts[0] == prefix {
			return name
		}
	}

	return ""
}

// guessAppName tries to derive a human-readable app name from a bundle ID.
func guessAppName(bundleID string) string {
	parts := strings.Split(bundleID, ".")
	if len(parts) < 3 {
		return bundleID
	}

	// Take the third part as the app name, capitalize it
	name := parts[2]

	// Handle common patterns
	name = strings.ReplaceAll(name, "-", " ")

	// Capitalize first letter
	if len(name) > 0 {
		name = strings.ToUpper(name[:1]) + name[1:]
	}

	return name
}

// readBundleID reads the CFBundleIdentifier from an app's Info.plist.
func readBundleID(appPath string) string {
	plistPath := filepath.Join(appPath, "Contents", "Info.plist")

	cmd := exec.Command("defaults", "read", plistPath, "CFBundleIdentifier")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

func expandPath(path string, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func countFiles(dir string) int {
	count := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}
