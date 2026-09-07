package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

type fakeSNEControlClient struct {
	identities []sne.ServiceReadinessIdentity
	loads      []string
	unloads    []string
	reloads    []string
	loadErr    error
	onLoad     func()
}

func (f *fakeSNEControlClient) ReadinessIdentity(context.Context) (sne.ServiceReadinessIdentity, error) {
	if len(f.identities) == 0 {
		return sne.ServiceReadinessIdentity{}, errors.New("missing fake identity")
	}
	identity := f.identities[0]
	if len(f.identities) > 1 {
		f.identities = f.identities[1:]
	}
	return identity, nil
}

func (f *fakeSNEControlClient) LoadModel(_ context.Context, model string) error {
	f.loads = append(f.loads, model)
	if f.onLoad != nil {
		f.onLoad()
	}
	return f.loadErr
}

func (f *fakeSNEControlClient) UnloadModel(_ context.Context, model string) error {
	f.unloads = append(f.unloads, model)
	return nil
}

func (f *fakeSNEControlClient) ReloadModel(_ context.Context, model string) error {
	f.reloads = append(f.reloads, model)
	return nil
}

func sneIdentity(status, model string) sne.ServiceReadinessIdentity {
	return sne.ServiceReadinessIdentity{Status: status, ReadyModelID: model}
}

func sneFullIdentity(status, model, runtime, native, manifest string) sne.ServiceReadinessIdentity {
	return sne.ServiceReadinessIdentity{
		Status: status, ReadyModelID: model,
		RuntimeSHA256: runtime, ReadyRuntimeSHA256: runtime,
		NativeRuntimeSHA256: native, ReadyNativeRuntimeSHA256: native,
		ReadyManifestSHA256: manifest,
	}
}

func TestSNEControlReadinessAndLifecycleBindIdentity(t *testing.T) {
	client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
		sneIdentity("ready", "model-a"),
		sneIdentity("ready", "model-a"),
	}}
	control, err := NewSNEControl(client, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, time.January, 1, 1, 2, 3, 0, time.UTC)
	control.now = func() time.Time { return when }
	readiness, err := control.Readiness(context.Background())
	if err != nil || !readiness.Ready || readiness.ServedModel != "model-a" {
		t.Fatalf("readiness = %+v, err=%v", readiness, err)
	}
	result, err := control.Apply(context.Background(), SNEReload)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != SNEReload || result.ModelID != "model-a" || len(client.reloads) != 1 || result.Before.ServedModel != result.After.ServedModel {
		t.Fatalf("unexpected lifecycle result: %+v client=%+v", result, client)
	}
}

func TestSNEControlRejectsDriftAndDoesNotMutate(t *testing.T) {
	client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
		sneIdentity("ready", "other-model"),
	}}
	control, err := NewSNEControl(client, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Readiness(context.Background()); err == nil {
		t.Fatal("expected readiness drift rejection")
	}
	if _, err := control.Apply(context.Background(), SNELoad); err == nil {
		t.Fatal("expected lifecycle preflight drift rejection")
	}
	if len(client.loads) != 0 {
		t.Fatalf("load mutated after drift: %v", client.loads)
	}
}

func TestSNEControlReadinessRejectsCancelledContext(t *testing.T) {
	client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
		sneIdentity("ready", "model-a"),
	}}
	control, err := NewSNEControl(client, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := control.Readiness(ctx); err == nil {
		t.Fatal("cancelled readiness was reported successful")
	}
}

func TestSNEControlLoadAllowsStoppedPreflightAndRequiresReadyPostflight(t *testing.T) {
	client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
		sneIdentity("stopped", ""),
		sneIdentity("ready", "model-a"),
	}}
	control, err := NewSNEControl(client, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	result, err := control.Apply(context.Background(), SNELoad)
	if err != nil {
		t.Fatal(err)
	}
	if result.Before.Ready || result.Before.ServedModel != "" || !result.After.Ready || len(client.loads) != 1 {
		t.Fatalf("load lifecycle did not bind stopped-to-ready readback: %+v client=%+v", result, client)
	}
}

func TestSNEControlFullIdentityRejectsRuntimeDriftBeforeLoad(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	other := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
		sneFullIdentity("ready", "model-a", other, sha, sha),
	}}
	control, err := NewSNEControlWithIdentity(client, SNEControlIdentity{ModelID: "model-a", RuntimeSHA256: sha, NativeRuntimeSHA256: sha, ManifestSHA256: sha})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Readiness(context.Background()); err == nil {
		t.Fatal("runtime drift was accepted")
	}
	if _, err := control.Apply(context.Background(), SNELoad); err == nil {
		t.Fatal("load mutated after ready-state runtime drift")
	}
	if len(client.loads) != 0 {
		t.Fatalf("load calls after identity drift: %v", client.loads)
	}
}

func TestSNEControlRefusesMutationAfterContextCancellation(t *testing.T) {
	client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
		sneIdentity("stopped", ""),
	}}
	control, err := NewSNEControl(client, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := control.Apply(ctx, SNELoad); err == nil {
		t.Fatal("cancelled lifecycle unexpectedly mutated")
	}
	if len(client.loads) != 0 {
		t.Fatalf("load calls after cancellation: %v", client.loads)
	}
}

func TestSNEControlRejectsCancellationAfterMutation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeSNEControlClient{
		identities: []sne.ServiceReadinessIdentity{
			sneIdentity("stopped", ""),
			sneIdentity("ready", "model-a"),
		},
		onLoad: cancel,
	}
	control, err := NewSNEControl(client, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := control.Apply(ctx, SNELoad); err == nil {
		t.Fatal("cancelled lifecycle was reported successful after mutation")
	}
	if len(client.loads) != 1 {
		t.Fatalf("expected exactly one attempted load, got %v", client.loads)
	}
}

func TestSNEControlRequiresCompleteRuntimeIdentityTuple(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	client := &fakeSNEControlClient{}
	if _, err := NewSNEControlWithIdentity(client, SNEControlIdentity{ModelID: "model-a", RuntimeSHA256: sha}); err == nil {
		t.Fatal("accepted partial runtime identity tuple")
	}
	control, err := NewSNEControlWithIdentity(client, SNEControlIdentity{
		ModelID:             "model-a",
		RuntimeSHA256:       " " + sha + " ",
		NativeRuntimeSHA256: sha,
		ManifestSHA256:      sha,
	})
	if err != nil {
		t.Fatalf("trimmed complete identity tuple rejected: %v", err)
	}
	if control.expectation.RuntimeSHA256 != sha || control.expectation.NativeRuntimeSHA256 != sha || control.expectation.ManifestSHA256 != sha {
		t.Fatalf("identity tuple was not canonicalized: %+v", control.expectation)
	}
}

func TestSNEControlUnloadRequiresClearedPostflightIdentity(t *testing.T) {
	tests := []struct {
		name        string
		after       sne.ServiceReadinessIdentity
		wantErr     bool
		wantUnloads int
	}{
		{name: "cleared", after: sneIdentity("stopped", ""), wantUnloads: 1},
		{name: "still-ready", after: sneIdentity("ready", "model-a"), wantErr: true, wantUnloads: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeSNEControlClient{identities: []sne.ServiceReadinessIdentity{
				sneIdentity("ready", "model-a"), tc.after,
			}}
			control, err := NewSNEControl(client, "model-a")
			if err != nil {
				t.Fatal(err)
			}
			_, err = control.Apply(context.Background(), SNEUnload)
			if tc.wantErr != (err != nil) || len(client.unloads) != tc.wantUnloads {
				t.Fatalf("unload err=%v calls=%v, wantErr=%v calls=%d", err, client.unloads, tc.wantErr, tc.wantUnloads)
			}
		})
	}
}

func TestSNEControlRecoveryAndBenchmarkAreHashBound(t *testing.T) {
	control, err := NewSNEControl(&fakeSNEControlClient{}, "model-a")
	if err != nil {
		t.Fatal(err)
	}
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	request := sne.RecoveryRequest{
		Action: sne.RecoveryRetry, ReceiptID: "receipt-1",
		Current: sne.RecoveryIdentity{ModelID: "model-a", RuntimeID: "runtime", PackageID: "package", PackageSHA256: sha, RuntimeSHA256: sha, ManifestSHA256: sha, ArtifactSetSHA256: sha},
		Lease:   sne.RecoveryArtifactLease{LeaseID: "lease-1", SHA256: sha}, OperatorState: sne.RecoveryOperatorActive,
	}
	recovery, err := control.PlanRecovery(request)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.PlanSHA256 == "" || recovery.PlanSHA256 == sha || recovery.Plan.ReceiptID != request.ReceiptID {
		t.Fatalf("unexpected recovery binding: %+v", recovery)
	}
	benchmark, err := NewSNEBenchmarkSession(sne.BenchmarkProvenance{
		SessionID: "session", ClaimClass: "performance", CleanRoom: true, DeviceIdentity: "device", OSVersion: "macOS", SourceRevision: "source", ModelID: "model-a", RuntimeID: "runtime", ArtifactSetSHA256: sha, ManifestSHA256: sha, CorpusSHA256: sha, PowerSource: "AC", ThermalState: "nominal", InputTokens: 8, OutputTokens: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := benchmark.Validate(); err != nil {
		t.Fatal(err)
	}
}
