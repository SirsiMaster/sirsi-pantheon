//go:build darwin

package apprecovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	bundlePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{1,255}$`)
	launchdPattern = regexp.MustCompile(`^(gui/[0-9]+|user/[0-9]+|system)/[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
)

type DarwinDriver struct {
	HTTPClient *http.Client
}

func (d DarwinDriver) ClearTransientState(_ context.Context, target Target) error {
	for _, path := range target.FreshStatePaths {
		if !filepath.IsAbs(path) {
			return errors.New("transient state path must be absolute")
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("transient state path must not be a symbolic link")
		}
		if info.IsDir() {
			return errors.New("transient state clearing accepts files only")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func (d DarwinDriver) Capture(_ context.Context, target Target) (Snapshot, error) {
	snapshot := Snapshot{Files: make(map[string]string, len(target.StatePaths))}
	for _, path := range target.StatePaths {
		if !filepath.IsAbs(path) {
			return Snapshot{}, errors.New("recovery state path must be absolute")
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return Snapshot{}, errors.New("declared recovery state must be a non-symlink regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("declared recovery state unavailable: %w", err)
		}
		hash := sha256.Sum256(data)
		snapshot.Files[path] = hex.EncodeToString(hash[:])
	}
	return snapshot, nil
}

func (d DarwinDriver) PID(ctx context.Context, target Target) (int, error) {
	if !filepath.IsAbs(target.ExecutablePath) {
		return 0, errors.New("executable path must be absolute")
	}
	executablePath, err := filepath.EvalSymlinks(target.ExecutablePath)
	if err != nil {
		return 0, errors.New("resolve executable path")
	}
	paths := regexp.QuoteMeta(target.ExecutablePath)
	if executablePath != target.ExecutablePath {
		paths = "(" + paths + "|" + regexp.QuoteMeta(executablePath) + ")"
	}
	out, err := exec.CommandContext(ctx, "/usr/bin/pgrep", "-f", "^"+paths+"( |$)").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, nil
		}
		return 0, err
	}
	first := strings.Fields(string(out))
	if len(first) == 0 {
		return 0, nil
	}
	return strconv.Atoi(first[0])
}

func (d DarwinDriver) Stop(ctx context.Context, target Target, pid int) error {
	if pid <= 0 {
		return nil
	}
	switch target.Kind {
	case KindAppSavedState:
		if !bundlePattern.MatchString(target.BundleID) {
			return errors.New("invalid application bundle id")
		}
		return exec.CommandContext(ctx, "/usr/bin/osascript", "-e", `tell application id "`+target.BundleID+`" to quit`).Run()
	case KindLaunchd:
		return nil
	case KindCheckpointed:
		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return process.Signal(syscall.SIGTERM)
	default:
		return ErrUnsupported
	}
}

func (d DarwinDriver) Start(ctx context.Context, target Target, snapshot Snapshot) error {
	for path, expected := range snapshot.Files {
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.New("captured recovery state disappeared before relaunch")
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != expected {
			return errors.New("captured recovery state changed before relaunch")
		}
	}
	switch target.Kind {
	case KindAppSavedState:
		if !bundlePattern.MatchString(target.BundleID) {
			return errors.New("invalid application bundle id")
		}
		appPath, err := appBundlePath(target.ExecutablePath)
		if err != nil {
			return err
		}
		return exec.CommandContext(ctx, "/usr/bin/open", appPath).Run()
	case KindLaunchd:
		if !launchdPattern.MatchString(target.LaunchdTarget) {
			return errors.New("invalid launchd target")
		}
		return exec.CommandContext(ctx, "/bin/launchctl", "kickstart", "-k", target.LaunchdTarget).Run()
	case KindCheckpointed:
		command := exec.Command(target.ExecutablePath, target.StartArguments...)
		command.Env = append(os.Environ(), "PANTHEON_RECOVERY_STATE=verified")
		return command.Start()
	default:
		return ErrUnsupported
	}
}

func appBundlePath(executablePath string) (string, error) {
	macOSDir := filepath.Dir(executablePath)
	contentsDir := filepath.Dir(macOSDir)
	appPath := filepath.Dir(contentsDir)
	if filepath.Base(macOSDir) != "MacOS" || filepath.Base(contentsDir) != "Contents" || filepath.Ext(appPath) != ".app" {
		return "", errors.New("application executable is not inside a macOS app bundle")
	}
	return appPath, nil
}

func (d DarwinDriver) Ready(ctx context.Context, target Target) error {
	if target.ReadinessURL == "" {
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.ReadinessURL, nil)
	if err != nil {
		return err
	}
	if request.URL.Scheme != "http" || request.URL.Hostname() == "" {
		return errors.New("readiness URL must be loopback HTTP")
	}
	ip := net.ParseIP(request.URL.Hostname())
	if request.URL.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("readiness URL must be loopback HTTP")
	}
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("readiness returned HTTP %d", response.StatusCode)
	}
	return nil
}
