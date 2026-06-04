package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Darwin implements Platform for macOS.
type Darwin struct{}

func (d *Darwin) Name() string { return "darwin" }

func (d *Darwin) Getenv(key string) string {
	return os.Getenv(key)
}

func (d *Darwin) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (d *Darwin) Getwd() (string, error) {
	return os.Getwd()
}

func (d *Darwin) Command(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (d *Darwin) Processes() ([]string, error) {
	out, err := d.Command("ps", "-eo", "comm")
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}

func (d *Darwin) SupportsTrash() bool { return true }

func (d *Darwin) MoveToTrash(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path for trash: %w", err)
	}
	// Native move to ~/.Trash — the app's OWN file operation. This needs NO
	// Automation/Finder permission, so it works from a background launchd agent
	// (the menubar), where `tell application "Finder"` is rejected by TCC. Items
	// remain fully recoverable from the Trash. If the native move fails, fail
	// closed instead of falling back to Finder automation; unattended cleanup must
	// never trigger authorization prompts.
	if home, herr := os.UserHomeDir(); herr == nil {
		trashDir := filepath.Join(home, ".Trash")
		if mkErr := os.MkdirAll(trashDir, 0o700); mkErr == nil {
			dest := filepath.Join(trashDir, filepath.Base(absPath))
			if _, statErr := os.Lstat(dest); statErr == nil {
				// Avoid clobbering an existing item of the same name in the Trash.
				dest = filepath.Join(trashDir, filepath.Base(absPath)+" "+time.Now().Format("2006-01-02 15.04.05"))
			}
			if renErr := os.Rename(absPath, dest); renErr == nil {
				return nil
			} else {
				return fmt.Errorf("move %q to trash: %w", absPath, renErr)
			}
		}
	}
	return fmt.Errorf("trash unavailable for %q", absPath)
}

func (d *Darwin) ProtectedPrefixes() []string {
	return []string{
		"/System/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/private/var/db/",
		"/Library/Extensions/",
		"/Library/Frameworks/",
	}
}

func (d *Darwin) PickFolder() (string, error) {
	cmd := exec.Command("osascript", "-e",
		`POSIX path of (choose folder with prompt "Select a folder to scan:")`)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("folder picker canceled or failed: %w", err)
	}
	return filepath.Clean(string(out)), nil
}

func (d *Darwin) OpenBrowser(url string) error {
	return exec.Command("open", url).Start()
}

func (d *Darwin) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

func (d *Darwin) Kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// Note: We use the system command "kill" to be more portable across environments
	// than syscall package on some platforms.
	_ = exec.Command("kill", "-15", fmt.Sprintf("%d", pid)).Run()
	return proc.Kill() // Force kill if it's still there
}
