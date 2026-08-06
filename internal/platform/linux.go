package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Linux implements Platform for Linux distributions.
type Linux struct{}

func (l *Linux) Name() string { return "linux" }

func (l *Linux) Getenv(key string) string {
	return os.Getenv(key)
}

func (l *Linux) UserHomeDir() (string, error) {
	return os.UserHomeDir()
}

func (l *Linux) Getwd() (string, error) {
	return os.Getwd()
}

func (l *Linux) Command(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (l *Linux) Processes() ([]string, error) {
	out, err := l.Command("ps", "-eo", "comm")
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}

func (l *Linux) SupportsTrash() bool { return false } // TODO: freedesktop.org trash spec

func (l *Linux) MoveToTrash(path string) error {
	// TODO: Implement freedesktop.org trash spec
	// For now, use gio trash if available
	cmd := exec.Command("gio", "trash", path)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trash not available on this Linux system (install glib2): %w", err)
	}
	return nil
}

func (l *Linux) ProtectedPrefixes() []string {
	return []string{
		"/boot/",
		"/etc/",
		"/usr/",
		"/bin/",
		"/sbin/",
		"/lib/",
		"/lib64/",
		"/proc/",
		"/sys/",
		"/dev/",
		"/var/lib/dpkg/",
		"/var/lib/rpm/",
	}
}

func (l *Linux) PickFolder() (string, error) {
	// Try zenity first, then kdialog
	cmd := exec.Command("zenity", "--file-selection", "--directory",
		"--title=Select a folder to scan")
	out, err := cmd.Output()
	if err != nil {
		// Fallback to kdialog
		cmd = exec.Command("kdialog", "--getexistingdirectory", ".")
		out, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("no folder picker available (install zenity or kdialog): %w", err)
		}
	}
	return string(out), nil
}

func (l *Linux) OpenBrowser(url string) error {
	return exec.Command("xdg-open", url).Start()
}

func (l *Linux) ReadDir(dirname string) ([]os.DirEntry, error) {
	return os.ReadDir(dirname)
}

func (l *Linux) Kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	_ = exec.Command("kill", "-15", fmt.Sprintf("%d", pid)).Run()
	return proc.Kill()
}

// TrashContents: linux has no trash Sirsi manages.
func (l *Linux) TrashContents() ([]TrashEntry, error) { return nil, nil }

// EmptyTrash: refused rather than silently no-op — a permanent-delete caller
// must never be told "done" by a platform that cannot do it.
func (l *Linux) EmptyTrash([]string) ([]string, int64, error) {
	return nil, 0, errTrashUnsupported
}
