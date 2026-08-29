package dashboard

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestSNEReadinessProjectionRequiresExactSupervisedTuple(t *testing.T) {
	runtimeSHA := strings.Repeat("a", 64)
	nativeRuntimeSHA := strings.Repeat("c", 64)
	manifestSHA := strings.Repeat("b", 64)
	lifecycle := SNELifecycleState{
		State: "ready", ModelID: "gemma-test", Profile: "interactive",
		RuntimeSHA256: runtimeSHA, NativeRuntimeSHA256: nativeRuntimeSHA, ModelManifestSHA256: manifestSHA,
	}
	identity := sne.ServiceReadinessIdentity{
		Status: "ready", APIVersion: "v0", APIContract: "sne.openai-chat.v2", ReadyAPIContract: "sne.openai-chat.v2",
		Profile: "interactive", ReadyProfile: "interactive", RuntimeSHA256: runtimeSHA, ReadyRuntimeSHA256: runtimeSHA, NativeRuntimeSHA256: nativeRuntimeSHA, ReadyNativeRuntimeSHA256: nativeRuntimeSHA,
		LoadedModel: "gemma-test", ReadyModelID: "gemma-test", ReadyManifestSHA256: manifestSHA,
		Models:                []sne.Model{{ID: "gemma-test", ManifestSHA256: manifestSHA}},
		MaxConcurrentRequests: 1, ReadyMaxConcurrentRequests: 1,
		MaxQueuedRequests: 8, ReadyMaxQueuedRequests: 8,
		QueueDiscipline: "fifo", ReadyQueueDiscipline: "fifo",
		RequestTimeoutMS: 120000, ReadyRequestTimeoutMS: 120000,
	}
	if !sneReadinessMatchesLifecycle(identity, lifecycle) {
		t.Fatal("exact supervised tuple was not projected ready")
	}
	if recovery := sneReadinessMismatchRecovery(identity, lifecycle); recovery != "" {
		t.Fatalf("exact supervised tuple received mismatch recovery: %q", recovery)
	}

	tests := map[string]func(*sne.ServiceReadinessIdentity, *SNELifecycleState){
		"unsupervised": func(_ *sne.ServiceReadinessIdentity, state *SNELifecycleState) { state.State = "stopped" },
		"runtime drift": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) {
			id.RuntimeSHA256 = strings.Repeat("d", 64)
		},
		"native runtime drift": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) {
			id.NativeRuntimeSHA256 = strings.Repeat("d", 64)
		},
		"model drift": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) { id.LoadedModel = "rogue-model" },
		"manifest drift": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) {
			id.Models[0].ManifestSHA256 = strings.Repeat("d", 64)
		},
		"profile drift":  func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) { id.Profile = "fleet" },
		"contract drift": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) { id.APIContract = "sne.openai-chat.v1" },
		"multiple models": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) {
			id.Models = append(id.Models, sne.Model{ID: "other"})
		},
		"queue drift":   func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) { id.ReadyMaxQueuedRequests++ },
		"timeout drift": func(id *sne.ServiceReadinessIdentity, _ *SNELifecycleState) { id.ReadyRequestTimeoutMS++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidateIdentity := identity
			candidateIdentity.Models = append([]sne.Model(nil), identity.Models...)
			candidateLifecycle := lifecycle
			mutate(&candidateIdentity, &candidateLifecycle)
			if sneReadinessMatchesLifecycle(candidateIdentity, candidateLifecycle) {
				t.Fatal("drifted or unsupervised service was projected ready")
			}
			if candidateIdentity.Status == "ready" && sneReadinessMismatchRecovery(candidateIdentity, candidateLifecycle) == "" {
				t.Fatal("ready but drifted service omitted actionable mismatch recovery")
			}
		})
	}
}

func TestApplySNELifecycleRecoveryProjectsWaitingForUnlock(t *testing.T) {
	model := SNEReadModel{ServiceState: "failed", Lifecycle: SNELifecycleState{
		State: "failed", ErrorCode: sneMetalSessionLockedCode,
		Recovery: "Unlock this Mac. Pantheon will retry the same verified model and runtime when the graphical session becomes active.",
	}}
	applySNELifecycleRecovery(&model)
	if model.ServiceState != "waiting-for-unlock" || !strings.Contains(model.Recovery, "Unlock this Mac") {
		t.Fatalf("locked-session projection = %+v", model)
	}
	model.Ready = true
	model.ServiceState = "ready"
	applySNELifecycleRecovery(&model)
	if model.ServiceState != "ready" {
		t.Fatalf("healthy service was overwritten: %+v", model)
	}
}
