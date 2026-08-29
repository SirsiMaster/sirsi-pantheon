package sne

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var supervisorProfileMigrationMu sync.Mutex

// LoadOrMigrateSupervisorProfile accepts the current strict profile or upgrades
// the one pre-serving-policy interactive profile emitted by older Pantheon
// installers. Every other invalid policy continues to fail closed.
func LoadOrMigrateSupervisorProfile(path string) (SupervisorProfile, error) {
	profile, err := LoadSupervisorProfile(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported SNE supervisor serving policy") {
		return profile, err
	}

	supervisorProfileMigrationMu.Lock()
	defer supervisorProfileMigrationMu.Unlock()
	if profile, err = LoadSupervisorProfile(path); err == nil {
		return profile, nil
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return SupervisorProfile{}, fmt.Errorf("read legacy SNE supervisor profile: %w", readErr)
	}
	var legacy SupervisorProfile
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if decodeErr := decoder.Decode(&legacy); decodeErr != nil {
		return SupervisorProfile{}, fmt.Errorf("decode legacy SNE supervisor profile: %w", decodeErr)
	}
	if legacy.SchemaVersion != "pantheon.sne-supervisor.v0" || legacy.SNE.Profile != "interactive" || legacy.SNE.RestartPolicy != "on-failure" || legacy.SNE.MaxConcurrentRequests != 4 || legacy.SNE.MaxQueuedRequests != 0 || legacy.SNE.QueueDiscipline != "" || legacy.SNE.RequestTimeoutMS != 0 {
		return SupervisorProfile{}, fmt.Errorf("unsupported SNE supervisor serving policy")
	}

	expected := expectedServingPolicy("interactive")
	legacy.SNE.MaxConcurrentRequests = expected.MaxConcurrentRequests
	legacy.SNE.MaxQueuedRequests = expected.MaxQueuedRequests
	legacy.SNE.QueueDiscipline = expected.QueueDiscipline
	legacy.SNE.RequestTimeoutMS = expected.RequestTimeoutMS
	encoded, encodeErr := yaml.Marshal(&legacy)
	if encodeErr != nil {
		return SupervisorProfile{}, fmt.Errorf("encode migrated SNE supervisor profile: %w", encodeErr)
	}
	info, statErr := os.Lstat(path)
	if statErr != nil || !info.Mode().IsRegular() {
		return SupervisorProfile{}, fmt.Errorf("SNE supervisor profile must be a regular file")
	}
	backup := path + ".pre-serving-policy-v2"
	if _, backupErr := os.Stat(backup); os.IsNotExist(backupErr) {
		if writeErr := os.WriteFile(backup, data, info.Mode().Perm()); writeErr != nil {
			return SupervisorProfile{}, fmt.Errorf("backup legacy SNE supervisor profile: %w", writeErr)
		}
	}
	temporary, tempErr := os.CreateTemp(filepath.Dir(path), ".sne-profile-migrate-*")
	if tempErr != nil {
		return SupervisorProfile{}, fmt.Errorf("stage migrated SNE supervisor profile: %w", tempErr)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if chmodErr := temporary.Chmod(info.Mode().Perm()); chmodErr != nil {
		temporary.Close()
		return SupervisorProfile{}, fmt.Errorf("protect migrated SNE supervisor profile: %w", chmodErr)
	}
	if _, writeErr := temporary.Write(encoded); writeErr != nil {
		temporary.Close()
		return SupervisorProfile{}, fmt.Errorf("write migrated SNE supervisor profile: %w", writeErr)
	}
	if syncErr := temporary.Sync(); syncErr != nil {
		temporary.Close()
		return SupervisorProfile{}, fmt.Errorf("sync migrated SNE supervisor profile: %w", syncErr)
	}
	if closeErr := temporary.Close(); closeErr != nil {
		return SupervisorProfile{}, fmt.Errorf("close migrated SNE supervisor profile: %w", closeErr)
	}
	if renameErr := os.Rename(temporaryPath, path); renameErr != nil {
		return SupervisorProfile{}, fmt.Errorf("commit migrated SNE supervisor profile: %w", renameErr)
	}
	return LoadSupervisorProfile(path)
}
