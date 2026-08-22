//go:build darwin

package apprecovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const defaultRegistrySchema = "pantheon.app-recovery-targets.v1"

type registryFile struct {
	Schema  string           `json:"schema"`
	Targets []registryTarget `json:"targets"`
}

type registryTarget struct {
	ID              string   `json:"id"`
	Kind            Kind     `json:"kind"`
	BundleID        string   `json:"bundle_id,omitempty"`
	ExecutablePath  string   `json:"executable_path"`
	LaunchdTarget   string   `json:"launchd_target,omitempty"`
	StatePaths      []string `json:"state_paths,omitempty"`
	FreshStatePaths []string `json:"fresh_state_paths,omitempty"`
	ReadinessURL    string   `json:"readiness_url,omitempty"`
	ReadyTimeoutMS  int64    `json:"ready_timeout_ms,omitempty"`
	StartArguments  []string `json:"start_arguments,omitempty"`
	AutoResume      bool     `json:"auto_resume,omitempty"`
}

func LoadDefaultManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configRoot := filepath.Join(home, ".config", "sirsi")
	registryPath := filepath.Join(configRoot, "recovery-targets.json")
	info, err := os.Lstat(registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("recovery target registry must be a regular file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("recovery target registry must not be accessible by group or other users")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("recovery target registry must be owned by the current user")
	}
	file, err := os.Open(registryPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var registry registryFile
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("decode recovery target registry: %w", err)
	}
	if registry.Schema != defaultRegistrySchema {
		return nil, errors.New("unsupported recovery target registry schema")
	}
	targets := make([]Target, 0, len(registry.Targets))
	for _, item := range registry.Targets {
		if item.ReadyTimeoutMS < 0 || item.ReadyTimeoutMS > int64((10*time.Minute)/time.Millisecond) {
			return nil, fmt.Errorf("target %q has invalid readiness timeout", item.ID)
		}
		targets = append(targets, Target{
			ID: item.ID, Kind: item.Kind, BundleID: item.BundleID,
			ExecutablePath: item.ExecutablePath, LaunchdTarget: item.LaunchdTarget,
			StatePaths: item.StatePaths, FreshStatePaths: item.FreshStatePaths,
			ReadinessURL: item.ReadinessURL, ReadyTimeout: time.Duration(item.ReadyTimeoutMS) * time.Millisecond,
			StartArguments: item.StartArguments, AutoResume: item.AutoResume,
		})
	}
	return NewManager(targets, DarwinDriver{}, FileStore{Root: filepath.Join(configRoot, "recovery-receipts")})
}
