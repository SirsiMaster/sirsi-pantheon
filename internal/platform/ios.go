//go:build ios

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// IOS implements Platform for iOS devices.
// iOS sandboxing restricts filesystem access to the app container.
// Many macOS APIs (osascript, ps, networksetup) are unavailable.
type IOS struct {
	// ContainerPath is the app's sandbox root, set at init by Swift.
	ContainerPath string
}

func (p *IOS) Name() string { return "ios" }

func (p *IOS) Getenv(key string) string {
	return os.Getenv(key)
}

func (p *IOS) UserHomeDir() (string, error) {
	if p.ContainerPath != "" {
		return p.ContainerPath, nil
	}
	return os.UserHomeDir()
}

func (p *IOS) Getwd() (string, error) {
	return os.Getwd()
}

// Command is restricted on iOS. Only a subset of operations are supported
// via direct Go implementations rather than shelling out.
func (p *IOS) Command(name string, args ...string) ([]byte, error) {
	switch name {
	case "uname":
		return p.uname(args...)
	case "sysctl":
		return p.sysctl(args...)
	default:
		return nil, fmt.Errorf("command %q not available on iOS (sandboxed)", name)
	}
}

// Processes is not available on iOS — the sandbox prevents process enumeration.
func (p *IOS) Processes() ([]string, error) {
	return nil, fmt.Errorf("process enumeration not available on iOS")
}

// SupportsTrash returns false — iOS has no user-accessible Trash.
func (p *IOS) SupportsTrash() bool { return false }

// MoveToTrash is not supported on iOS.
func (p *IOS) MoveToTrash(path string) error {
	return fmt.Errorf("trash not supported on iOS — use direct deletion with confirmation")
}

// ProtectedPrefixes returns iOS system paths that must never be touched.
func (p *IOS) ProtectedPrefixes() []string {
	return []string{
		"/System/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/private/",
		"/var/",
	}
}

// OpenBrowser opens a URL — on iOS this is handled by the Swift layer via UIApplication.
// The Go layer signals intent; Swift performs the actual open.
func (p *IOS) OpenBrowser(url string) error {
	return fmt.Errorf("URL open must be handled by Swift layer: %s", url)
}

// PickFolder is handled by the Swift layer via UIDocumentPickerViewController.
func (p *IOS) PickFolder() (string, error) {
	return "", fmt.Errorf("folder picker must be handled by Swift layer")
}

func (p *IOS) ReadDir(dirname string) ([]os.DirEntry, error) {
	// Enforce sandbox — only allow reads within the container.
	if p.ContainerPath != "" {
		abs, err := filepath.Abs(dirname)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(abs, p.ContainerPath) {
			return nil, fmt.Errorf("access denied: %s is outside app sandbox", dirname)
		}
	}
	return os.ReadDir(dirname)
}

func (p *IOS) Kill(pid int) error {
	return fmt.Errorf("process termination not available on iOS")
}

// --- iOS-native implementations for system queries ---

func (p *IOS) uname(args ...string) ([]byte, error) {
	// On iOS, syscall.Uname is not available. Use runtime info instead.
	return []byte(runtime.GOARCH), nil
}

func (p *IOS) sysctl(args ...string) ([]byte, error) {
	// Provide basic hardware info available on iOS without shelling out.
	if len(args) >= 2 && args[0] == "-n" {
		switch args[1] {
		case "hw.ncpu":
			return []byte(fmt.Sprintf("%d", runtime.NumCPU())), nil
		case "hw.memsize":
			// Use Go's runtime for memory estimate — not exact but functional.
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			return []byte(fmt.Sprintf("%d", m.Sys)), nil
		case "machdep.cpu.brand_string":
			return []byte("Apple A-series (iOS)"), nil
		}
	}
	return nil, fmt.Errorf("sysctl %v not available on iOS", args)
}

// TrashContents: iOS has no trash Sirsi manages.
func (p *IOS) TrashContents() ([]TrashEntry, error) { return nil, nil }

// EmptyTrash: refused rather than silently no-op — a permanent-delete caller
// must never be told "done" by a platform that cannot do it.
func (p *IOS) EmptyTrash([]string) ([]string, int64, error) {
	return nil, 0, errTrashUnsupported
}
