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
	"runtime"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/cleaner"
	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
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
	ResidualPreferences     ResidualType = "Preferences"
	ResidualAppSupport      ResidualType = "Application Support"
	ResidualCaches          ResidualType = "Caches"
	ResidualContainers      ResidualType = "Containers"
	ResidualGroupContainers ResidualType = "Group Containers"
	ResidualSavedState      ResidualType = "Saved State"
	ResidualHTTPStorages    ResidualType = "HTTP Storages"
	ResidualWebKit          ResidualType = "WebKit Data"
	ResidualCookies         ResidualType = "Cookies"
	ResidualAppScripts      ResidualType = "Application Scripts"
	ResidualLogs            ResidualType = "Logs"
	ResidualLaunchAgent     ResidualType = "Launch Agent"
	ResidualLaunchDaemon    ResidualType = "Launch Daemon"
	ResidualReceipts        ResidualType = "Package Receipts"
	ResidualLoginItems      ResidualType = "Login Items"
	ResidualCrashReports    ResidualType = "Crash Reports"
	ResidualGhostApp        ResidualType = "Ghost App (Spotlight)"
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

// HorusManifest is the interface for the shared filesystem index.
type HorusManifest interface {
	DirSizeAndCount(dir string) (int64, int)
	Exists(path string) bool
}

// Scanner is the Ka ghost detection engine.
type Scanner struct {
	homeDir            string
	appDirs            []string          // Directories to scan for .app bundles
	installedApps      map[string]bool   // Bundle IDs of currently installed apps
	installedNames     map[string]bool   // Names of currently installed apps (lowercase)
	knownBundleIDs     map[string]string // Bundle ID → app name mapping
	DirReader          func(string) ([]os.DirEntry, error)
	ExecCommand        func(string, ...string) *exec.Cmd
	ReadBundleIDFn     func(string) (string, error)
	SkipLaunchServices bool
	SkipBrew           bool
	Manifest           HorusManifest
}

// NewScanner creates a new Ka scanner.
func NewScanner() *Scanner {
	homeDir, _ := os.UserHomeDir()
	s := &Scanner{
		homeDir: homeDir,
		appDirs: []string{
			"/Applications",
			filepath.Join(homeDir, "Applications"),
		},
		installedApps:  make(map[string]bool),
		installedNames: make(map[string]bool),
		knownBundleIDs: make(map[string]string),
		DirReader:      os.ReadDir,          // Default implementation
		ExecCommand:    exec.Command,        // Default implementation
		ReadBundleIDFn: readBundleIDDefault, // Default implementation
	}
	return s
}

// readBundleIDDefault reads the CFBundleIdentifier from an app's Info.plist.
func readBundleIDDefault(appPath string) (string, error) {
	plistPath := filepath.Join(appPath, "Contents", "Info.plist")

	cmd := exec.Command("defaults", "read", plistPath, "CFBundleIdentifier")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// Scan discovers all ghosts (Kas) on the system.
// This method has ZERO side effects — it only reads.
func (s *Scanner) Scan(includeSudo bool) ([]Ghost, error) {
	logging.Info("Ka: starting ghost scan", "sudo", includeSudo)
	start := time.Now()

	// Step 1: Build inventory of currently installed apps
	logging.Debug("ka scan starting", "includeSudo", includeSudo)
	if err := s.buildInstalledAppIndex(); err != nil {
		return nil, fmt.Errorf("failed to index installed apps: %w", err)
	}
	logging.Debug("installed apps indexed", "bundleIDs", len(s.installedApps), "names", len(s.installedNames))

	// Step 2+3: Run filesystem scan and Launch Services scan in parallel.
	// lsregister -dump takes ~5s alone — don't block the filesystem scan.
	var orphans map[string][]Residual
	var lsGhosts map[string]bool

	if s.SkipLaunchServices {
		// Fast path: filesystem-only scan (finds 95%+ of ghosts).
		orphans = s.scanForOrphans(includeSudo)
		lsGhosts = make(map[string]bool)
	} else {
		// Full path: parallel filesystem + lsregister.
		done := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			lsGhosts = s.scanLaunchServices()
			close(done)
		}()
		orphans = s.scanForOrphans(includeSudo)
		<-done
	}

	// Step 4: Merge orphans into Ghost structures
	ghosts := s.mergeOrphans(orphans, lsGhosts)
	logging.Debug("ka scan complete", "ghosts", len(ghosts), "orphans", len(orphans), "lsGhosts", len(lsGhosts))

	logging.Info("Ka: scan complete", "ghosts", len(ghosts), "duration", time.Since(start))
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

// buildInstalledAppIndex scans configured app directories for .app bundles.
func (s *Scanner) buildInstalledAppIndex() error {
	for _, dir := range s.appDirs {
		entries, err := s.DirReader(dir)
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
			bundleID, err := s.ReadBundleIDFn(appPath)
			if err != nil {
				continue
			}
			if bundleID != "" {
				s.installedApps[bundleID] = true
				s.knownBundleIDs[bundleID] = appName
			}
		}
	}

	// Also index Homebrew casks if not skipped
	if !s.SkipBrew {
		s.indexHomebrewCasks()
	}

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
		entries, err := s.DirReader(dir)
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
				if s.Manifest != nil {
					// Horus: O(1) hash lookup
					size, fileCount = s.Manifest.DirSizeAndCount(path)
				} else {
					// Fallback: combined walk
					size, fileCount = dirSizeAndCount(path)
				}
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

	path := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	cmd := s.ExecCommand(path, "-dump")
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
			AppName:          guessAppName(bundleID),
			BundleID:         bundleID,
			Residuals:        residuals,
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
func (s *Scanner) indexHomebrewCasks() map[string]bool {
	casks := make(map[string]bool)

	if s.SkipBrew {
		return casks
	}

	command := s.ExecCommand("brew", "list", "--cask", "-1")
	output, err := command.Output()
	if err != nil {
		logging.Debug("brew list --cask error", "err", err)
		return casks
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		cask := strings.TrimSpace(scanner.Text())
		if cask != "" {
			casks[cask] = true
			s.installedNames[strings.ToLower(cask)] = true
		}
	}

	return casks
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
	validPrefixes := []string{"com", "org", "net", "io", "dev", "me", "co", "app", "de", "uk", "fr", "jp", "br", "au", "edu"}
	for _, prefix := range validPrefixes {
		if parts[0] == prefix {
			// Ensure there's a non-empty part after the prefix
			if len(parts) > 1 && parts[1] != "" {
				return name
			}
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

func expandPath(path string, homeDir string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// dirSizeAndCount walks a directory once and returns total size and file count.
func dirSizeAndCount(dir string) (int64, int) {
	var totalSize int64
	count := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			count++
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
			}
		}
		return nil
	})
	return totalSize, count
}
