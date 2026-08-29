package sne

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrMigrateSupervisorProfileMigratesOnlyKnownLegacyPolicy(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sne-profile.yaml")
	legacy := `schema_version: pantheon.sne-supervisor.v0
product: nexus
sne:
  profile: interactive
  endpoint: http://127.0.0.1:8477
  health_path: /health/ready
  models_path: /v1/models
  restart_policy: on-failure
  memory_ceiling_bytes: 0
  max_concurrent_requests: 4
  yield_to_foreground: false
`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	profile, err := LoadOrMigrateSupervisorProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if profile.SNE.MaxConcurrentRequests != 1 || profile.SNE.MaxQueuedRequests != 8 || profile.SNE.QueueDiscipline != "fifo" || profile.SNE.RequestTimeoutMS != 120000 {
		t.Fatalf("unexpected migrated policy: %+v", profile.SNE)
	}
	backup, err := os.ReadFile(path + ".pre-serving-policy-v2")
	if err != nil || string(backup) != legacy {
		t.Fatalf("legacy backup mismatch: %v", err)
	}
}

func TestLoadOrMigrateSupervisorProfileRejectsUnknownDrift(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sne-profile.yaml")
	invalid := `schema_version: pantheon.sne-supervisor.v0
product: nexus
sne:
  profile: interactive
  endpoint: http://127.0.0.1:8477
  health_path: /health/ready
  models_path: /v1/models
  restart_policy: on-failure
  max_concurrent_requests: 2
  max_queued_requests: 8
  queue_discipline: fifo
  request_timeout_ms: 120000
`
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrMigrateSupervisorProfile(path); err == nil || !strings.Contains(err.Error(), "unsupported SNE supervisor serving policy") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(path + ".pre-serving-policy-v2"); !os.IsNotExist(err) {
		t.Fatalf("unknown drift created backup: %v", err)
	}
}
