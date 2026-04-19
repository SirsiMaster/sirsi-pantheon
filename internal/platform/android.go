//go:build android

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Android implements Platform for Android devices.
// Android sandboxing restricts filesystem access to the app's internal/external storage.
// Shell commands and process enumeration are unavailable.
type Android struct {
	// DataDir is the app's internal data directory, set at init by the Kotlin layer.
	DataDir string
}

func (p *Android) Name() string { return "android" }

func (p *Android) Getenv(key string) string {
	return os.Getenv(key)
}

func (p *Android) UserHomeDir() (string, error) {
	if p.DataDir != "" {
		return p.DataDir, nil
	}
	return os.UserHomeDir()
}

func (p *Android) Getwd() (string, error) {
	return os.Getwd()
}

// Command is restricted on Android. Only a subset of operations are supported
// via direct Go implementations rather than shelling out.
func (p *Android) Command(name string, args ...string) ([]byte, error) {
	switch name {
	case "uname":
		return p.uname(args...)
	case "getprop":
		return p.getprop(args...)
	default:
		return nil, fmt.Errorf("command %q not available on Android (sandboxed)", name)
	}
}

// Processes is not available on Android — the sandbox prevents process enumeration.
func (p *Android) Processes() ([]string, error) {
	return nil, fmt.Errorf("process enumeration not available on Android")
}

// SupportsTrash returns false — Android has no user-accessible Trash from the app sandbox.
func (p *Android) SupportsTrash() bool { return false }

// MoveToTrash is not supported on Android.
func (p *Android) MoveToTrash(path string) error {
	return fmt.Errorf("trash not supported on Android — use direct deletion with confirmation")
}

// ProtectedPrefixes returns Android system paths that must never be touched.
func (p *Android) ProtectedPrefixes() []string {
	return []string{
		"/system/",
		"/vendor/",
		"/oem/",
		"/odm/",
		"/product/",
		"/apex/",
		"/bin/",
		"/sbin/",
		"/dev/",
		"/proc/",
		"/sys/",
	}
}

// OpenBrowser opens a URL — on Android this is handled by the Kotlin layer via Intent.
// The Go layer signals intent; Kotlin performs the actual open.
func (p *Android) OpenBrowser(url string) error {
	return fmt.Errorf("URL open must be handled by Kotlin layer: %s", url)
}

// PickFolder is handled by the Kotlin layer via Storage Access Framework.
func (p *Android) PickFolder() (string, error) {
	return "", fmt.Errorf("folder picker must be handled by Kotlin layer")
}

func (p *Android) ReadDir(dirname string) ([]os.DirEntry, error) {
	// Enforce sandbox — only allow reads within the data directory.
	if p.DataDir != "" {
		abs, err := filepath.Abs(dirname)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(abs, p.DataDir) {
			return nil, fmt.Errorf("access denied: %s is outside app sandbox", dirname)
		}
	}
	return os.ReadDir(dirname)
}

func (p *Android) Kill(pid int) error {
	return fmt.Errorf("process termination not available on Android")
}

// --- Android-native implementations for system queries ---

func (p *Android) uname(args ...string) ([]byte, error) {
	return []byte(runtime.GOARCH), nil
}

func (p *Android) getprop(args ...string) ([]byte, error) {
	// Provide basic info available without shelling out.
	if len(args) >= 1 {
		switch args[0] {
		case "ro.product.model":
			return []byte("Android Device"), nil
		case "ro.build.version.release":
			return []byte("unknown"), nil
		case "ro.product.cpu.abi":
			return []byte(runtime.GOARCH), nil
		}
	}
	return nil, fmt.Errorf("getprop %v not available in sandbox", args)
}
