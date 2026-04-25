package seshat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ChromeProfile represents a Chrome profile parsed from the Local State file.
type ChromeProfile struct {
	// DirName is the profile directory name (e.g., "Default", "Profile 1").
	DirName string `json:"dir_name"`
	// DisplayName is the user-visible profile name (e.g., "SirsiMaster").
	DisplayName string `json:"display_name"`
	// GaiaName is the Google account display name.
	GaiaName string `json:"gaia_name"`
	// Email is the Google account email address.
	Email string `json:"email"`
	// AvatarIcon is the Chrome avatar identifier.
	AvatarIcon string `json:"avatar_icon"`
}

// chromeLocalState is the partial structure of Chrome's Local State JSON file.
type chromeLocalState struct {
	Profile struct {
		InfoCache map[string]chromeProfileInfo `json:"info_cache"`
	} `json:"profile"`
}

type chromeProfileInfo struct {
	Name       string `json:"name"`
	GaiaName   string `json:"gaia_name"`
	UserName   string `json:"user_name"`
	AvatarIcon string `json:"avatar_icon"`
}

// ChromeBaseDir returns the Chrome user data directory for the current platform.
func ChromeBaseDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	case "linux":
		return filepath.Join(home, ".config", "google-chrome")
	case "windows":
		return filepath.Join(home, "AppData", "Local", "Google", "Chrome", "User Data")
	default:
		return filepath.Join(home, ".config", "google-chrome")
	}
}

// ResolveProfileDir resolves a profile name to the full filesystem path.
// The profileName can be either a directory name (e.g., "Default", "Profile 1")
// or a display name (e.g., "SirsiMaster"). Directory names are checked first.
func ResolveProfileDir(profileName string) (string, error) {
	baseDir := ChromeBaseDir()
	dirPath := filepath.Join(baseDir, profileName)

	// Check if it's already a valid directory name.
	if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
		return dirPath, nil
	}

	// Try resolving as a display name.
	profiles, err := ListChromeProfiles()
	if err != nil {
		return "", fmt.Errorf("resolve profile '%s': %w", profileName, err)
	}

	for _, p := range profiles {
		if p.DisplayName == profileName || p.GaiaName == profileName {
			return filepath.Join(baseDir, p.DirName), nil
		}
	}

	return "", fmt.Errorf("Chrome profile '%s' not found — run 'sirsi seshat profiles chrome' to list available profiles", profileName)
}

// ListChromeProfiles reads Chrome's Local State file and returns all profiles.
func ListChromeProfiles() ([]ChromeProfile, error) {
	localStatePath := filepath.Join(ChromeBaseDir(), "Local State")

	data, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, fmt.Errorf("read Chrome Local State: %w", err)
	}

	var state chromeLocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse Chrome Local State: %w", err)
	}

	var profiles []ChromeProfile
	for dirName, info := range state.Profile.InfoCache {
		profiles = append(profiles, ChromeProfile{
			DirName:     dirName,
			DisplayName: info.Name,
			GaiaName:    info.GaiaName,
			Email:       info.UserName,
			AvatarIcon:  info.AvatarIcon,
		})
	}

	return profiles, nil
}

// ResolveDirNameByDisplayName looks up the directory name for a display name.
// Returns the display name itself if no mapping is found (assumes it IS the dir name).
func ResolveDirNameByDisplayName(displayName string) (string, error) {
	profiles, err := ListChromeProfiles()
	if err != nil {
		return "", fmt.Errorf("resolve display name '%s': %w", displayName, err)
	}

	// First check if displayName matches a directory name directly.
	for _, p := range profiles {
		if p.DirName == displayName {
			return p.DirName, nil
		}
	}

	// Then check display names and gaia names.
	for _, p := range profiles {
		if p.DisplayName == displayName || p.GaiaName == displayName {
			return p.DirName, nil
		}
	}

	return "", fmt.Errorf("Chrome profile '%s' not found", displayName)
}

// OpenChromeWithProfile launches Google Chrome with the specified profile.
// The profileName can be a display name (e.g., "SirsiMaster") or directory name (e.g., "Profile 1").
// If url is non-empty, Chrome opens to that URL.
func OpenChromeWithProfile(profileName string, url string) (string, error) {
	dirName, err := ResolveDirNameByDisplayName(profileName)
	if err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		args := []string{"-na", "Google Chrome", "--args", fmt.Sprintf("--profile-directory=%s", dirName)}
		if url != "" {
			args = append(args, url)
		}
		cmd = exec.Command("open", args...)
	case "linux":
		args := []string{fmt.Sprintf("--profile-directory=%s", dirName)}
		if url != "" {
			args = append(args, url)
		}
		cmd = exec.Command("google-chrome", args...)
	case "windows":
		chromePath := filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe")
		args := []string{fmt.Sprintf("--profile-directory=%s", dirName)}
		if url != "" {
			args = append(args, url)
		}
		cmd = exec.Command(chromePath, args...)
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("launch Chrome with profile '%s' (dir: %s): %w", profileName, dirName, err)
	}

	return dirName, nil
}
