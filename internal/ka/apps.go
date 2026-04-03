// apps.go implements the full application enumerator for macOS.
// This is Ka's domain — the Spirit Hunter sees all software on the machine,
// whether alive or dead.
package ka

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/logging"
)

// InstalledApp represents a fully enumerated application on the system.
type InstalledApp struct {
	Name       string    `json:"name"`
	BundleID   string    `json:"bundle_id"`
	Path       string    `json:"path"`
	Version    string    `json:"version"`
	Source     string    `json:"source"` // "applications", "user-applications", "homebrew", "appstore", "ghost", "orphan"
	Size       int64     `json:"size"`   // bytes
	LastUsed   time.Time `json:"last_used"`
	IsRunning  bool      `json:"is_running"`
	HasGhosts  bool      `json:"has_ghosts"`
	GhostCount int       `json:"ghost_count"`
	GhostSize  int64     `json:"ghost_size"`
}

// systemProfilerApp represents a single app entry from system_profiler JSON.
type systemProfilerApp struct {
	Name     string `json:"_name"`
	Version  string `json:"version"`
	Path     string `json:"path"`
	BundleID string `json:"info"`          // sometimes populated
	Source   string `json:"obtained_from"` // "identified_developer", "apple", "mac_app_store", etc.
	LastUsed string `json:"lastModified"`
}

// systemProfilerResult wraps the top-level JSON structure from system_profiler.
type systemProfilerResult struct {
	Items []systemProfilerApp `json:"_items"`
}

// EnumerateApps discovers all software on macOS from multiple sources.
// This function has ZERO side effects — it only reads.
func EnumerateApps(ctx context.Context) ([]InstalledApp, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("ka apps: only supported on macOS")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("ka apps: cannot resolve home dir: %w", err)
	}

	logging.Info("Ka: starting app enumeration")
	start := time.Now()

	// Collect apps from all sources in parallel.
	type sourceResult struct {
		apps []InstalledApp
		err  error
	}

	profilerCh := make(chan sourceResult, 1)
	brewCh := make(chan sourceResult, 1)

	// Source 1: system_profiler (covers /Applications, ~/Applications, App Store, etc.)
	go func() {
		apps, err := enumerateSystemProfiler(ctx)
		profilerCh <- sourceResult{apps, err}
	}()

	// Source 2: Homebrew casks
	go func() {
		apps, err := enumerateHomebrew(ctx, homeDir)
		brewCh <- sourceResult{apps, err}
	}()

	// Source 3+4: Direct .app scanning for anything system_profiler missed
	directApps := enumerateAppDirs(ctx, homeDir)

	// Collect parallel results
	profilerRes := <-profilerCh
	brewRes := <-brewCh

	// Merge: use a map keyed by path to deduplicate
	appMap := make(map[string]*InstalledApp)

	// system_profiler is the richest source — add first
	if profilerRes.err == nil {
		for i := range profilerRes.apps {
			app := &profilerRes.apps[i]
			appMap[app.Path] = app
		}
	}

	// Direct .app scans fill gaps
	for i := range directApps {
		app := &directApps[i]
		if _, exists := appMap[app.Path]; !exists {
			appMap[app.Path] = app
		}
	}

	// Homebrew casks — enrich or add
	if brewRes.err == nil {
		for i := range brewRes.apps {
			bApp := &brewRes.apps[i]
			if existing, exists := appMap[bApp.Path]; exists {
				existing.Source = "homebrew" // Override source to indicate brew-managed
			} else {
				appMap[bApp.Path] = bApp
			}
		}
	}

	// Convert to slice
	apps := make([]InstalledApp, 0, len(appMap))
	for _, app := range appMap {
		apps = append(apps, *app)
	}

	// Enrich each app with runtime status, size, last-used, and ghost data
	scanner := NewScanner()
	scanner.SkipLaunchServices = true // We'll fold LS ghosts via the ghost scan below
	ghosts, _ := scanner.Scan(ctx, false)

	// Build ghost index by bundle ID and by app name
	ghostByBundleID := make(map[string]Ghost)
	ghostByName := make(map[string]Ghost)
	for _, g := range ghosts {
		if g.BundleID != "" {
			ghostByBundleID[g.BundleID] = g
		}
		ghostByName[strings.ToLower(g.AppName)] = g
	}

	for i := range apps {
		enrichApp(ctx, &apps[i], ghostByBundleID, ghostByName)
	}

	// Add pure ghost apps (apps no longer installed but with residuals).
	// Multi-layer matching prevents false positives on active apps:
	//   Layer 1: Exact bundle ID match
	//   Layer 2: Bundle ID prefix/family (com.adobe.* matches all Adobe residuals)
	//   Layer 3: Normalized name substring (WhatsApp matches WhatsAppSMB)
	//   Layer 4: Vendor prefix grouping (known vendor domains)
	existingBundles := make(map[string]bool)
	existingNames := make(map[string]bool)
	existingBundlePrefixes := make(map[string]bool)
	existingNameNorms := make(map[string]bool)
	for _, app := range apps {
		if app.BundleID != "" {
			existingBundles[app.BundleID] = true
			// Layer 2: Extract bundle ID family prefix (e.g. "com.adobe" from "com.adobe.Acrobat")
			prefix := bundleIDPrefix(app.BundleID)
			if prefix != "" {
				existingBundlePrefixes[prefix] = true
			}
		}
		norm := strings.ToLower(app.Name)
		existingNames[norm] = true
		// Layer 3: Store normalized variants for substring matching
		existingNameNorms[normalizeAppName(norm)] = true
	}

	for _, g := range ghosts {
		if ghostBelongsToInstalledApp(g, existingBundles, existingBundlePrefixes, existingNames, existingNameNorms) {
			continue
		}
		apps = append(apps, InstalledApp{
			Name:       g.AppName,
			BundleID:   g.BundleID,
			Source:     "ghost",
			HasGhosts:  true,
			GhostCount: g.TotalFiles,
			GhostSize:  g.TotalSize,
		})
	}

	// Sort by name
	sort.Slice(apps, func(i, j int) bool {
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})

	logging.Info("Ka: app enumeration complete", "total", len(apps), "duration", time.Since(start))
	return apps, nil
}

// enumerateSystemProfiler uses system_profiler to get all registered apps.
func enumerateSystemProfiler(ctx context.Context) ([]InstalledApp, error) {
	cmd := exec.CommandContext(ctx, "system_profiler", "SPApplicationsDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("system_profiler failed: %w", err)
	}

	var results []systemProfilerResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("system_profiler JSON parse failed: %w", err)
	}

	var apps []InstalledApp
	for _, result := range results {
		for _, item := range result.Items {
			source := classifySource(item.Path, item.Source)
			app := InstalledApp{
				Name:    item.Name,
				Path:    item.Path,
				Version: item.Version,
				Source:  source,
			}
			if item.BundleID != "" {
				app.BundleID = item.BundleID
			}
			apps = append(apps, app)
		}
	}

	return apps, nil
}

// enumerateHomebrew lists all Homebrew cask-installed apps.
func enumerateHomebrew(ctx context.Context, homeDir string) ([]InstalledApp, error) {
	cmd := exec.CommandContext(ctx, "brew", "list", "--cask", "-1")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew list failed: %w", err)
	}

	var apps []InstalledApp
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		cask := strings.TrimSpace(sc.Text())
		if cask == "" {
			continue
		}

		// Try to find the .app path — most casks install to /Applications
		appName := caskToAppName(cask)
		appPath := filepath.Join("/Applications", appName+".app")
		if _, err := os.Stat(appPath); os.IsNotExist(err) {
			appPath = filepath.Join(homeDir, "Applications", appName+".app")
			if _, err := os.Stat(appPath); os.IsNotExist(err) {
				appPath = "" // Can't find it
			}
		}

		app := InstalledApp{
			Name:   appName,
			Path:   appPath,
			Source: "homebrew",
		}

		// Try reading version from brew info
		if version := brewCaskVersion(ctx, cask); version != "" {
			app.Version = version
		}

		// Read bundle ID from the .app if we found it
		if appPath != "" {
			if bid, err := readBundleIDDefault(ctx, appPath); err == nil && bid != "" {
				app.BundleID = bid
			}
		}

		apps = append(apps, app)
	}

	return apps, nil
}

// enumerateAppDirs directly scans /Applications and ~/Applications.
func enumerateAppDirs(ctx context.Context, homeDir string) []InstalledApp {
	dirs := []struct {
		path   string
		source string
	}{
		{"/Applications", "applications"},
		{filepath.Join(homeDir, "Applications"), "user-applications"},
	}

	var apps []InstalledApp
	for _, d := range dirs {
		entries, err := os.ReadDir(d.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			entryPath := filepath.Join(d.path, entry.Name())
			if strings.HasSuffix(entry.Name(), ".app") {
				// Direct .app at top level
			} else if entry.IsDir() {
				// Scan one level deeper for .app inside subdirs
				// Handles: WhatsApp.localized/WhatsApp.app, Adobe Acrobat DC/Adobe Acrobat.app
				subEntries, err := os.ReadDir(entryPath)
				if err != nil {
					continue
				}
				for _, sub := range subEntries {
					if strings.HasSuffix(sub.Name(), ".app") {
						subAppPath := filepath.Join(entryPath, sub.Name())
						subAppName := strings.TrimSuffix(sub.Name(), ".app")
						subApp := InstalledApp{
							Name:   subAppName,
							Path:   subAppPath,
							Source: d.source,
						}
						if bid, err := readBundleIDDefault(ctx, subAppPath); err == nil && bid != "" {
							subApp.BundleID = bid
						}
						apps = append(apps, subApp)
					}
				}
				continue
			} else {
				continue
			}
			appPath := entryPath
			appName := strings.TrimSuffix(entry.Name(), ".app")

			app := InstalledApp{
				Name:   appName,
				Path:   appPath,
				Source: d.source,
			}

			// Read bundle ID
			if bid, err := readBundleIDDefault(ctx, appPath); err == nil && bid != "" {
				app.BundleID = bid
			}

			// Read version from Info.plist
			if version := readAppVersion(ctx, appPath); version != "" {
				app.Version = version
			}

			apps = append(apps, app)
		}
	}

	return apps
}

// enrichApp fills in runtime status, size, last-used date, and ghost info.
func enrichApp(ctx context.Context, app *InstalledApp, ghostByBundleID map[string]Ghost, ghostByName map[string]Ghost) {
	if app.Path == "" {
		return
	}

	// Read bundle ID if missing
	if app.BundleID == "" {
		if bid, err := readBundleIDDefault(ctx, app.Path); err == nil && bid != "" {
			app.BundleID = bid
		}
	}

	// Read version if missing
	if app.Version == "" {
		if v := readAppVersion(ctx, app.Path); v != "" {
			app.Version = v
		}
	}

	// Check if running
	app.IsRunning = isAppRunning(ctx, app.Name)

	// Get size via du
	app.Size = appSize(ctx, app.Path)

	// Get last used date
	app.LastUsed = appLastUsed(ctx, app.Path)

	// Check for ghost residuals
	if app.BundleID != "" {
		if g, ok := ghostByBundleID[app.BundleID]; ok {
			app.HasGhosts = true
			app.GhostCount = g.TotalFiles
			app.GhostSize = g.TotalSize
		}
	}
	if !app.HasGhosts {
		if g, ok := ghostByName[strings.ToLower(app.Name)]; ok {
			app.HasGhosts = true
			app.GhostCount = g.TotalFiles
			app.GhostSize = g.TotalSize
		}
	}
}

// isAppRunning checks if an app is currently running via pgrep.
func isAppRunning(ctx context.Context, appName string) bool {
	cmd := exec.CommandContext(ctx, "pgrep", "-xiq", appName)
	return cmd.Run() == nil
}

// appSize returns the size of an .app bundle in bytes.
func appSize(ctx context.Context, appPath string) int64 {
	cmd := exec.CommandContext(ctx, "du", "-sk", appPath)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	// du -sk outputs "<size_kb>\t<path>"
	fields := strings.Fields(string(out))
	if len(fields) < 1 {
		return 0
	}
	var kb int64
	_, _ = fmt.Sscanf(fields[0], "%d", &kb)
	return kb * 1024
}

// appLastUsed reads the kMDItemLastUsedDate from Spotlight metadata.
func appLastUsed(ctx context.Context, appPath string) time.Time {
	cmd := exec.CommandContext(ctx, "mdls", "-name", "kMDItemLastUsedDate", "-raw", appPath)
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}
	}
	raw := strings.TrimSpace(string(out))
	if raw == "(null)" || raw == "" {
		return time.Time{}
	}
	// Format: "2024-03-15 10:30:45 +0000"
	t, err := time.Parse("2006-01-02 15:04:05 +0000", raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

// readAppVersion reads CFBundleShortVersionString from Info.plist.
func readAppVersion(ctx context.Context, appPath string) string {
	plistPath := filepath.Join(appPath, "Contents", "Info.plist")
	cmd := exec.CommandContext(ctx, "defaults", "read", plistPath, "CFBundleShortVersionString")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// classifySource determines the source category from system_profiler data.
func classifySource(appPath, obtainedFrom string) string {
	homeDir, _ := os.UserHomeDir()
	switch {
	case obtainedFrom == "mac_app_store":
		return "appstore"
	case strings.HasPrefix(appPath, filepath.Join(homeDir, "Applications")):
		return "user-applications"
	default:
		return "applications"
	}
}

// caskToAppName converts a brew cask name (e.g. "visual-studio-code") to
// an app name (e.g. "Visual Studio Code"). This is a best-effort heuristic.
func caskToAppName(cask string) string {
	parts := strings.Split(cask, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// brewCaskVersion attempts to read the version of a brew cask.
func brewCaskVersion(ctx context.Context, cask string) string {
	cmd := exec.CommandContext(ctx, "brew", "info", "--cask", "--json=v2", cask)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Parse minimal JSON to extract version
	var info struct {
		Casks []struct {
			Version string `json:"version"`
		} `json:"casks"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return ""
	}
	if len(info.Casks) > 0 {
		return info.Casks[0].Version
	}
	return ""
}

// ScanLaunchServicesGhosts folds the sight/launchservices logic into Ka.
// Scans the Launch Services database for apps that are registered but
// whose .app bundle no longer exists on disk.
func ScanLaunchServicesGhosts(ctx context.Context) ([]GhostRegistration, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("ka: Launch Services scan only supported on macOS")
	}

	lsregister := "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
	cmd := exec.CommandContext(ctx, lsregister, "-dump")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ka: lsregister pipe failed: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("ka: lsregister start failed: %w", err)
	}

	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 256*1024), 1024*1024)

	var ghosts []GhostRegistration
	seen := make(map[string]bool)

	var currentBundle, currentPath, currentName string
	separator := "--------------------------------------------------------------------------------"

	processBlock := func() {
		if currentBundle == "" || currentPath == "" {
			return
		}
		if strings.HasPrefix(currentBundle, "com.apple.") {
			return
		}
		if !strings.Contains(currentPath, ".app") {
			return
		}
		appPath := currentPath
		if idx := strings.Index(currentPath, ".app"); idx > 0 {
			appPath = currentPath[:idx+4]
		}
		if seen[currentBundle] {
			return
		}
		if _, err := os.Stat(appPath); os.IsNotExist(err) {
			seen[currentBundle] = true
			name := currentName
			if name == "" {
				name = currentBundle
			}
			ghosts = append(ghosts, GhostRegistration{
				BundleID: currentBundle,
				Path:     appPath,
				Name:     name,
			})
		}
	}

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if line == separator {
			processBlock()
			currentBundle = ""
			currentPath = ""
			currentName = ""
			continue
		}

		if strings.HasPrefix(line, "bundle id:") {
			currentBundle = strings.TrimSpace(strings.TrimPrefix(line, "bundle id:"))
		} else if strings.HasPrefix(line, "path:") {
			currentPath = strings.TrimSpace(strings.TrimPrefix(line, "path:"))
		} else if strings.HasPrefix(line, "name:") {
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		}
	}
	processBlock() // final block

	_ = cmd.Wait()

	logging.Debug("ka: Launch Services ghost scan complete", "ghosts", len(ghosts))
	return ghosts, nil
}

// GhostRegistration represents an app registered in Launch Services
// whose .app bundle no longer exists on disk.
// (Folded from internal/sight — this is Ka's domain.)
type GhostRegistration struct {
	BundleID string `json:"bundle_id"`
	Path     string `json:"path"`
	Name     string `json:"name"`
}

// ── Multi-layer ghost matching ─────────────────────────────────────────
// Prevents false positives on active apps while maintaining full ghost detection.

// ghostBelongsToInstalledApp returns true if a ghost residual belongs to
// an app that is currently installed. Uses 4 matching layers.
func ghostBelongsToInstalledApp(g Ghost, bundles, bundlePrefixes, names, nameNorms map[string]bool) bool {
	// Layer 1: Exact bundle ID
	if g.BundleID != "" && bundles[g.BundleID] {
		return true
	}

	// Layer 2: Bundle ID family prefix
	// e.g. ghost has "com.adobe.AdobeCRDaemon" → prefix "com.adobe" matches installed "com.adobe.Acrobat"
	if g.BundleID != "" {
		prefix := bundleIDPrefix(g.BundleID)
		if prefix != "" && bundlePrefixes[prefix] {
			return true
		}
	}

	// Layer 3: Exact name match (case-insensitive)
	ghostName := strings.ToLower(g.AppName)
	if names[ghostName] {
		return true
	}

	// Layer 4: Normalized name substring matching
	// e.g. "WhatsAppSMB" normalizes to "whatsappsmb", which contains "whatsapp"
	// e.g. "CleanMyMac4" normalizes to "cleanmymac", which matches "cleanmymac"
	ghostNorm := normalizeAppName(ghostName)
	for installedNorm := range nameNorms {
		if installedNorm == "" || ghostNorm == "" {
			continue
		}
		// Check bidirectional substring: ghost contains installed OR installed contains ghost
		if strings.Contains(ghostNorm, installedNorm) || strings.Contains(installedNorm, ghostNorm) {
			return true
		}
	}

	return false
}

// bundleIDPrefix extracts the vendor prefix from a bundle ID.
// "com.adobe.Acrobat" → "com.adobe"
// "net.whatsapp.WhatsApp" → "net.whatsapp"
// Single-component IDs return empty.
func bundleIDPrefix(bundleID string) string {
	parts := strings.Split(bundleID, ".")
	if len(parts) < 3 {
		return ""
	}
	return strings.Join(parts[:2], ".")
}

// normalizeAppName strips version numbers, underscores, hyphens, and
// trailing digits to produce a canonical name for fuzzy matching.
// "CleanMyMac4" → "cleanmymac"
// "WhatsAppSMB" → "whatsappsmb" (still matches "whatsapp" via substring)
// "Acrobat_webcapture" → "acrobatweb capture"
func normalizeAppName(name string) string {
	name = strings.ToLower(name)
	// Remove common separators
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, " ", "")
	// Strip trailing version numbers (e.g. "cleanmymac4" → "cleanmymac")
	for len(name) > 0 && name[len(name)-1] >= '0' && name[len(name)-1] <= '9' {
		name = name[:len(name)-1]
	}
	return name
}
