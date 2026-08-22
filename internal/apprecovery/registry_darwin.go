//go:build darwin

package apprecovery

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
)

func DefaultRegistryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sirsi", "recovery-targets.json"), nil
}

func RegisterDefaultTarget(target Target) error {
	if err := target.validate(); err != nil {
		return err
	}
	if err := validateDarwinRegistration(target); err != nil {
		return err
	}
	path, err := DefaultRegistryPath()
	if err != nil {
		return err
	}
	registry, err := loadMutableRegistry(path)
	if err != nil {
		return err
	}
	for _, existing := range registry.Targets {
		if existing.ID == target.ID {
			return errors.New("recovery target already exists")
		}
	}
	registry.Targets = append(registry.Targets, registryTarget{
		ID: target.ID, Kind: target.Kind, BundleID: target.BundleID,
		ExecutablePath: target.ExecutablePath, LaunchdTarget: target.LaunchdTarget,
		StatePaths: target.StatePaths, FreshStatePaths: target.FreshStatePaths,
		ReadinessURL: target.ReadinessURL, ReadyTimeoutMS: target.ReadyTimeout.Milliseconds(),
		StartArguments: target.StartArguments, AutoResume: target.AutoResume,
	})
	return writeMutableRegistry(path, registry)
}

func RemoveDefaultTarget(targetID string) error {
	if !targetIDPattern.MatchString(targetID) {
		return errors.New("invalid recovery target id")
	}
	path, err := DefaultRegistryPath()
	if err != nil {
		return err
	}
	registry, err := loadMutableRegistry(path)
	if err != nil {
		return err
	}
	kept := registry.Targets[:0]
	found := false
	for _, target := range registry.Targets {
		if target.ID == targetID {
			found = true
			continue
		}
		kept = append(kept, target)
	}
	if !found {
		return errors.New("unknown recovery target")
	}
	registry.Targets = kept
	return writeMutableRegistry(path, registry)
}

func loadMutableRegistry(path string) (registryFile, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return registryFile{Schema: defaultRegistrySchema, Targets: []registryTarget{}}, nil
	}
	if err != nil {
		return registryFile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return registryFile{}, errors.New("existing recovery registry failed file safety admission")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return registryFile{}, errors.New("existing recovery registry must be owned by the current user")
	}
	file, err := os.Open(path)
	if err != nil {
		return registryFile{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var registry registryFile
	if err := decoder.Decode(&registry); err != nil {
		return registryFile{}, err
	}
	if registry.Schema != defaultRegistrySchema {
		return registryFile{}, errors.New("unsupported recovery target registry schema")
	}
	return registry, nil
}

func validateDarwinRegistration(target Target) error {
	if !filepath.IsAbs(target.ExecutablePath) {
		return errors.New("recovery executable path must be absolute")
	}
	executable, err := os.Lstat(target.ExecutablePath)
	if err != nil {
		return errors.New("recovery executable is unavailable")
	}
	if executable.Mode()&os.ModeSymlink != 0 || !executable.Mode().IsRegular() || executable.Mode().Perm()&0111 == 0 {
		return errors.New("recovery executable must be a non-symlink executable file")
	}
	if target.Kind == KindAppSavedState && !bundlePattern.MatchString(target.BundleID) {
		return errors.New("invalid application bundle id")
	}
	if target.Kind == KindLaunchd && !launchdPattern.MatchString(target.LaunchdTarget) {
		return errors.New("invalid launchd target")
	}
	for _, path := range append(append([]string{}, target.StatePaths...), target.FreshStatePaths...) {
		if !filepath.IsAbs(path) {
			return errors.New("recovery state paths must be absolute")
		}
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && containsPath(target.FreshStatePaths, path) {
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("recovery state path must be an available non-symlink regular file")
		}
	}
	if target.ReadinessURL != "" {
		parsed, err := url.Parse(target.ReadinessURL)
		if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
			return errors.New("readiness URL must be loopback HTTP")
		}
		ip := net.ParseIP(parsed.Hostname())
		if parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("readiness URL must be loopback HTTP")
		}
	}
	return nil
}

func containsPath(paths []string, wanted string) bool {
	for _, path := range paths {
		if path == wanted {
			return true
		}
	}
	return false
}

func writeMutableRegistry(path string, registry registryFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".recovery-targets-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
