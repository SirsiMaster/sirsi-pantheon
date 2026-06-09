package ka

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
)

// DarwinProvider implements GhostProvider for macOS.
// It scans .app bundles, reads Info.plist for bundle IDs,
// queries Launch Services (lsregister), and checks Homebrew casks.
type DarwinProvider struct{}

func (d *DarwinProvider) ResidualLocations(includeSudo bool) []residualLocation {
	locations := []residualLocation{
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
	if includeSudo {
		locations = append(locations,
			residualLocation{ResidualPreferences, "/Library/Preferences", true},
			residualLocation{ResidualLaunchAgent, "/Library/LaunchAgents", true},
			residualLocation{ResidualLaunchDaemon, "/Library/LaunchDaemons", true},
			residualLocation{ResidualReceipts, "/var/db/receipts", true},
			residualLocation{ResidualAppSupport, "/Library/Application Support", true},
		)
	}
	return locations
}

func (d *DarwinProvider) BuildInstalledIndex(ctx context.Context, s *Scanner) error {
	for _, dir := range s.appDirs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		d.indexAppsIn(ctx, s, dir, 3)
	}

	if !s.SkipBrew {
		s.indexHomebrewCasks(ctx)
	}

	return nil
}

// indexAppsIn recursively indexes .app bundles under dir, up to maxDepth levels
// deep. Records every .app it finds AND descends into non-.app subdirectories
// — /Applications routinely contains both direct .app entries (Microsoft Word.app)
// AND subfolders that hold apps (WhatsApp.localized/, Adobe Acrobat DC/,
// Utilities/Adobe Sync/CoreSync/), and we want BOTH. Never descends into a
// .app bundle's internal directories.
func (d *DarwinProvider) indexAppsIn(ctx context.Context, s *Scanner, dir string, maxDepth int) {
	if maxDepth <= 0 {
		return
	}
	entries, err := s.DirReader(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".app") {
			appPath := filepath.Join(dir, name)
			appName := strings.TrimSuffix(name, ".app")
			s.installedNames[strings.ToLower(appName)] = true
			if bundleID, err := s.ReadBundleIDFn(ctx, appPath); err == nil && bundleID != "" {
				s.installedApps[bundleID] = true
				s.knownBundleIDs[bundleID] = appName
			}
			continue // do not descend into the .app
		}
		if entry.IsDir() {
			d.indexAppsIn(ctx, s, filepath.Join(dir, name), maxDepth-1)
		}
	}
}

func (d *DarwinProvider) ScanRegistry(ctx context.Context, s *Scanner) map[string]bool {
	ghosts := make(map[string]bool)

	path := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	cmd := s.ExecCommand(ctx, path, "-dump")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ghosts
	}
	if err := cmd.Start(); err != nil {
		return ghosts
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)
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

		if currentBundle != "" && currentPath != "" {
			if strings.HasSuffix(currentPath, ".app") {
				if _, err := os.Stat(currentPath); os.IsNotExist(err) {
					if !s.installedApps[currentBundle] {
						ghosts[currentBundle] = true
					}
				}
			}
			currentBundle = ""
			currentPath = ""
		}
	}

	_ = cmd.Wait()

	logging.Debug("darwin: scanned Launch Services", "ghosts", len(ghosts))
	return ghosts
}

func (d *DarwinProvider) ExtractAppID(name string) string {
	return extractBundleID(name)
}

func (d *DarwinProvider) IsSystemID(id string) bool {
	return isSystemBundleID(id)
}
