package dashboard

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

type fakeSNESupervisor struct {
	started  bool
	stopped  bool
	startErr error
	resource sne.ResourceAdmission
	start    func() error
}

func TestRuntimePackageKeepsServiceAndNativeIdentitiesDistinct(t *testing.T) {
	packaged := sne.RuntimePackage{
		RuntimeSHA256:       strings.Repeat("a", 64),
		NativeRuntimeSHA256: strings.Repeat("b", 64),
	}
	if packaged.RuntimeSHA256 == packaged.NativeRuntimeSHA256 {
		t.Fatal("service and native runtime identities collapsed")
	}
}

func TestFindSNECheckoutAcceptsCanonicalLicenseURL(t *testing.T) {
	root := t.TempDir()
	checkpoint := filepath.Join(root, "models", "gemma-test", "revision", "checkpoint")
	if err := os.MkdirAll(checkpoint, 0o755); err != nil {
		t.Fatal(err)
	}
	receipt := `{"schema":"sne.model-checkout.v1","model_id":"gemma-test","revision":"revision","license_identifier":"gemma-terms","license_url":"https://ai.google.dev/gemma/terms","license_accepted":true,"source_uri":"huggingface://google/gemma-test@revision","checkpoint_sha256":"checkpoint","artifact_set_sha256":"artifacts","completed_at":"2026-08-17T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(checkpoint, ".sne-checkout.json"), []byte(receipt), 0o644); err != nil {
		t.Fatal(err)
	}

	resolved, parsed, err := findSNECheckout(root, "gemma-test")
	if err != nil {
		t.Fatalf("find checkout: %v", err)
	}
	if resolved != checkpoint || parsed.LicenseURL != "https://ai.google.dev/gemma/terms" {
		t.Fatalf("resolved=%q receipt=%+v", resolved, parsed)
	}
}

func (fake *fakeSNESupervisor) Start(context.Context) error {
	fake.started = true
	if fake.start != nil {
		return fake.start()
	}
	return fake.startErr
}
func (fake *fakeSNESupervisor) WaitReady(context.Context) error { return nil }
func (fake *fakeSNESupervisor) Stop(context.Context) error      { fake.stopped = true; return nil }
func (fake *fakeSNESupervisor) ResourceAdmission() sne.ResourceAdmission {
	return fake.resource
}

func TestSNELifecycleTransitions(t *testing.T) {
	fake := &fakeSNESupervisor{}
	var gotAllowResearch bool
	var gotRuntimeID string
	manager := NewSNELifecycleManager(SNELifecycleConfig{
		CatalogStoreRoot: t.TempDir(),
		factory:          func(sne.SupervisorProfile, sne.LaunchConfig) (sneSupervisor, error) { return fake, nil },
		resolve: func(_ string, runtimeID string, allowResearch bool) (sne.SupervisorProfile, sne.LaunchConfig, error) {
			gotRuntimeID = runtimeID
			gotAllowResearch = allowResearch
			return sne.SupervisorProfile{}, sne.LaunchConfig{StartupTimeout: time.Second}, nil
		},
	})
	state, err := manager.Start(SNELifecycleRequest{ModelID: "gemma-test", RuntimeID: "candidate-v26", AllowResearch: true})
	if err != nil || state.State != "starting" {
		t.Fatalf("start state=%+v err=%v", state, err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.Snapshot().State != "ready" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state = manager.Snapshot(); state.State != "ready" || state.RuntimeID != "candidate-v26" || !fake.started || !gotAllowResearch || gotRuntimeID != "candidate-v26" {
		t.Fatalf("ready state=%+v started=%v", state, fake.started)
	}
	if _, err = manager.RollbackCatalog(strings.Repeat("a", 64)); err == nil || !strings.Contains(err.Error(), "stop SNE") {
		t.Fatalf("catalog rollback while ready was not rejected: %v", err)
	}
	state, err = manager.Stop(context.Background())
	if err != nil || state.State != "stopped" || !fake.stopped {
		t.Fatalf("stop state=%+v stopped=%v err=%v", state, fake.stopped, err)
	}
	manager.mu.Lock()
	manager.catalogMutating = true
	manager.mu.Unlock()
	if _, err := manager.Start(SNELifecycleRequest{ModelID: "gemma-test"}); err == nil || !strings.Contains(err.Error(), "catalog mutation") {
		t.Fatalf("start during catalog mutation was not rejected: %v", err)
	}
	manager.mu.Lock()
	manager.catalogMutating = false
	manager.mu.Unlock()
}

func TestSNELifecycleRejectsUnrecoveredModelStore(t *testing.T) {
	manager := NewSNELifecycleManager(SNELifecycleConfig{
		StoreRoot:       t.TempDir(),
		RequireRecovery: true,
		recover: func(context.Context, string, string) error {
			return fmt.Errorf("interrupted removal receipt is invalid")
		},
	})
	if manager.Snapshot().State != "failed" || len(manager.Available()) != 0 || len(manager.RuntimeSelections()) != 0 {
		t.Fatalf("unrecovered lifecycle state = %+v", manager.Snapshot())
	}
	if _, err := manager.Start(SNELifecycleRequest{ModelID: "gemma-test"}); err == nil || !strings.Contains(err.Error(), "not recovered") {
		t.Fatalf("lifecycle did not fail closed on recovery: %v", err)
	}
}

func TestSNELifecyclePublishesActionableResourceAdmission(t *testing.T) {
	fake := &fakeSNESupervisor{
		startErr: &sne.ResourceAdmissionError{
			Code:     "swap_cleanup_required",
			Message:  "SNE resource admission requires swap cleanup",
			Recovery: "Restart the Mac and retry.",
		},
		resource: sne.ResourceAdmission{
			RequiredBytes:     25_545_459_702,
			TotalRAMBytes:     48 << 30,
			AvailableRAMBytes: 28 << 30,
			LifecycleReserve:  4 << 30,
			SwapUsedBytes:     4 << 30,
			SwapLimitBytes:    3 << 30,
			Pressure:          "normal",
			PressureSource:    "host_statistics64",
		},
	}
	manager := NewSNELifecycleManager(SNELifecycleConfig{
		CatalogStoreRoot: t.TempDir(),
		factory:          func(sne.SupervisorProfile, sne.LaunchConfig) (sneSupervisor, error) { return fake, nil },
		hostIdentity:     func() (string, error) { return "m5", nil },
		resolve: func(string, string, bool) (sne.SupervisorProfile, sne.LaunchConfig, error) {
			return sne.SupervisorProfile{}, sne.LaunchConfig{StartupTimeout: time.Second}, nil
		},
	})
	state, err := manager.Start(SNELifecycleRequest{ModelID: "gemma-test"})
	if err != nil || state.State != "starting" {
		t.Fatalf("start state=%+v err=%v", state, err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.Snapshot().State != "failed" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state = manager.Snapshot()
	if state.ErrorCode != "swap_cleanup_required" || state.Recovery != "Restart the Mac and retry." || state.ResourceAdmission == nil {
		t.Fatalf("actionable admission state = %+v", state)
	}
	if state.ResourceAdmission.RequiredBytes != fake.resource.RequiredBytes || state.ResourceAdmission.SwapUsedBytes != fake.resource.SwapUsedBytes {
		t.Fatalf("measured admission lost: %+v", state.ResourceAdmission)
	}
	if state.PrefixCachePressure == nil || state.PrefixCachePressure.ObservationSHA256 == "" {
		t.Fatalf("prefix-cache pressure receipt missing: %+v", state.PrefixCachePressure)
	}
}

func TestSNELifecyclePublishesLockedMetalSessionRecovery(t *testing.T) {
	fake := &fakeSNESupervisor{startErr: fmt.Errorf("SNE Metal session preflight: console is locked; authenticate the console before GPU work")}
	manager := NewSNELifecycleManager(SNELifecycleConfig{
		CatalogStoreRoot: t.TempDir(),
		factory:          func(sne.SupervisorProfile, sne.LaunchConfig) (sneSupervisor, error) { return fake, nil },
		resolve: func(string, string, bool) (sne.SupervisorProfile, sne.LaunchConfig, error) {
			return sne.SupervisorProfile{}, sne.LaunchConfig{RuntimeID: "runtime-v2", RuntimeSHA256: strings.Repeat("a", 64), ModelManifestSHA256: strings.Repeat("b", 64), StartupTimeout: time.Second}, nil
		},
	})
	if _, err := manager.Start(SNELifecycleRequest{ModelID: "gemma-test", RuntimeID: "runtime-v2"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.Snapshot().State != "failed" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := manager.Snapshot()
	if state.ErrorCode != sneMetalSessionLockedCode || !strings.Contains(state.Recovery, "same verified model and runtime") {
		t.Fatalf("locked-session recovery = %+v", state)
	}
}

func TestSNELifecycleRetriesPreservedTupleAfterGraphicalSessionUnlock(t *testing.T) {
	var unlocked atomic.Bool
	var starts atomic.Int32
	fake := &fakeSNESupervisor{start: func() error {
		starts.Add(1)
		if !unlocked.Load() {
			return fmt.Errorf("SNE Metal session preflight: console is locked")
		}
		return nil
	}}
	manager := NewSNELifecycleManager(SNELifecycleConfig{
		CatalogStoreRoot: t.TempDir(), unlockRetryInterval: time.Millisecond,
		graphicalSessionReady: func(context.Context) (bool, error) { return unlocked.Load(), nil },
		factory:               func(sne.SupervisorProfile, sne.LaunchConfig) (sneSupervisor, error) { return fake, nil },
		resolve: func(string, string, bool) (sne.SupervisorProfile, sne.LaunchConfig, error) {
			return sne.SupervisorProfile{}, sne.LaunchConfig{RuntimeID: "runtime-v2", RuntimeSHA256: strings.Repeat("a", 64), ModelManifestSHA256: strings.Repeat("b", 64), StartupTimeout: time.Second}, nil
		},
	})
	request := SNELifecycleRequest{ModelID: "gemma-test", RuntimeID: "runtime-v2"}
	if _, err := manager.Start(request); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.Snapshot().ErrorCode != sneMetalSessionLockedCode && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	unlocked.Store(true)
	deadline = time.Now().Add(time.Second)
	for manager.Snapshot().State != "ready" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := manager.Snapshot()
	if state.State != "ready" || state.ModelID != request.ModelID || state.RuntimeID != request.RuntimeID || starts.Load() != 2 {
		t.Fatalf("automatic unlock retry state=%+v starts=%d", state, starts.Load())
	}
	if _, err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSNELifecycleStopCancelsLockedSessionRetry(t *testing.T) {
	var unlocked atomic.Bool
	var starts atomic.Int32
	fake := &fakeSNESupervisor{start: func() error {
		starts.Add(1)
		return fmt.Errorf("SNE Metal session preflight: console is locked")
	}}
	manager := NewSNELifecycleManager(SNELifecycleConfig{
		CatalogStoreRoot: t.TempDir(), unlockRetryInterval: time.Millisecond,
		graphicalSessionReady: func(context.Context) (bool, error) { return unlocked.Load(), nil },
		factory:               func(sne.SupervisorProfile, sne.LaunchConfig) (sneSupervisor, error) { return fake, nil },
		resolve: func(string, string, bool) (sne.SupervisorProfile, sne.LaunchConfig, error) {
			return sne.SupervisorProfile{}, sne.LaunchConfig{RuntimeID: "runtime-v2", StartupTimeout: time.Second}, nil
		},
	})
	if _, err := manager.Start(SNELifecycleRequest{ModelID: "gemma-test", RuntimeID: "runtime-v2"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for manager.Snapshot().ErrorCode != sneMetalSessionLockedCode && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	unlocked.Store(true)
	time.Sleep(10 * time.Millisecond)
	if state := manager.Snapshot(); state.State != "stopped" || starts.Load() != 1 {
		t.Fatalf("canceled unlock retry state=%+v starts=%d", state, starts.Load())
	}
}

func TestParseIOConsoleLockedIsNarrowAndFailClosed(t *testing.T) {
	tests := []struct {
		name  string
		input string
		ready bool
		fail  bool
	}{
		{name: "unlocked", input: `    "IOConsoleLocked" = No`, ready: true},
		{name: "locked", input: `    "IOConsoleLocked" = Yes`, ready: false},
		{name: "missing", input: `"IOConsoleUsers" = ()`, fail: true},
		{name: "invalid", input: `"IOConsoleLocked" = Maybe`, fail: true},
		{name: "embedded statistics are ignored", input: `"Stats" = {"IOConsoleLocked"=No}`, fail: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ready, err := parseIOConsoleLocked([]byte(test.input))
			if (err != nil) != test.fail || ready != test.ready {
				t.Fatalf("ready=%v err=%v", ready, err)
			}
		})
	}
}

func TestRealDefaultSNELifecycleLoadsSignedCatalog(t *testing.T) {
	if os.Getenv("SNE_REAL_DEFAULT_SIGNED_CATALOG") != "1" {
		t.Skip("real default signed catalog gate not requested")
	}
	cfg := DefaultSNELifecycleConfig()
	if cfg == nil || !cfg.RequireSignedCatalog {
		t.Fatal("default SNE lifecycle does not require a signed catalog")
	}
	manager := NewSNELifecycleManager(*cfg)
	catalog, err := manager.loadRuntimeCatalog()
	if err != nil {
		t.Fatal(err)
	}
	status := manager.CatalogStatus()
	if status.State != "verified" || !status.SignedRequired || status.CatalogID != catalog.CatalogID ||
		len(status.VersionSHA256) != 64 || status.Entries != len(catalog.Entries) || status.Versions < 1 ||
		len(status.RetainedVersions) != status.Versions || status.RollbackAvailable != (status.Versions > 1) {
		t.Fatalf("unexpected default signed catalog status: %+v", status)
	}
	selections := manager.RuntimeSelections()
	if len(selections) != len(catalog.Entries) {
		t.Fatalf("unexpected default runtime selections: %d", len(selections))
	}
	t.Logf("pantheon_default_sne_signed_catalog accepted=true catalog_id=%s entries=%d", catalog.CatalogID, len(catalog.Entries))
}

func TestRealDefaultSignedPortableSNELifecycle(t *testing.T) {
	modelID := os.Getenv("SNE_REAL_DEFAULT_PORTABLE_MODEL_ID")
	if modelID == "" {
		t.Skip("real default signed portable lifecycle model not supplied")
	}
	cfg := DefaultSNELifecycleConfig()
	if cfg == nil || !cfg.RequireSignedCatalog {
		t.Fatal("default SNE lifecycle does not require a signed catalog")
	}
	manager := NewSNELifecycleManager(*cfg)
	state, err := manager.Start(SNELifecycleRequest{ModelID: modelID})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		state = manager.Snapshot()
		if state.State == "ready" || state.State == "failed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if state.State != "ready" {
		t.Fatalf("portable signed lifecycle did not become ready: %+v", state)
	}
	stopped, err := manager.Stop(context.Background())
	if err != nil || stopped.State != "stopped" {
		t.Fatalf("portable signed lifecycle stop=%+v err=%v", stopped, err)
	}
	t.Logf("pantheon_default_signed_portable_lifecycle accepted=true model=%s lifecycle_id=%s", modelID, state.ID)
}
