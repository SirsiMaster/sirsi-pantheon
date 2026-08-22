package dashboard

import (
	"testing"
	"time"
)

func TestBuildSNEOpenAIModelListRequiresExactVerifiedIdentity(t *testing.T) {
	model := SNEReadModel{
		Ready: true, ActiveModel: "gemma-4-12b-it-affine8-sne-v2", DeviceFamily: "Apple M5 Max", GeneratedAt: time.Unix(123, 0),
		Lifecycle:      SNELifecycleState{State: "ready", ModelID: "gemma-4-12b-it-affine8-sne-v2", RuntimeID: "sne-v2-mtp-shared-wide", RuntimeSHA256: string(make([]byte, 64)), ModelManifestSHA256: string(make([]byte, 64)), Profile: "interactive"},
		RuntimeCatalog: SNERuntimeCatalogStatus{State: "verified", SignedRequired: true, CatalogID: "sne-gemma4-v2", VersionSHA256: "abc123"},
		Catalog:        []SNECatalogItem{{CatalogEntry: "gemma-4-12b-affine8-mtp", ModelID: "gemma-4-12b-it-affine8-sne-v2", RuntimeID: "sne-v2-mtp-shared-wide", ExecutionMode: "mtp-shared-wide", WeightFormat: "affine", WeightBits: 8, Qualification: "admitted", SupportStatus: "release-supported", Active: true, CacheTopology: "fixed-4096", ServingCacheCapacity: 4096, PrefixSessionsMaximum: 2, PrefixSessionsSupported: false}},
	}
	model.Lifecycle.RuntimeSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	model.Lifecycle.ModelManifestSHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	response, ok := buildSNEOpenAIModelList(model)
	if !ok || response.Object != "list" || len(response.Data) != 1 {
		t.Fatalf("unexpected response: ok=%v response=%+v", ok, response)
	}
	got := response.Data[0]
	if got.ID != model.ActiveModel || got.OwnedBy != "sirsi" || got.SNE.RuntimeID != "sne-v2-mtp-shared-wide" || got.SNE.RuntimeSHA256 != model.Lifecycle.RuntimeSHA256 || got.SNE.ModelManifestSHA256 != model.Lifecycle.ModelManifestSHA256 || got.SNE.ExecutionMode != "mtp-shared-wide" || got.SNE.WeightBits != 8 || got.SNE.CatalogVersionSHA != "abc123" || got.SNE.SupportStatus != "release-supported" {
		t.Fatalf("governed model identity mismatch: %+v", got)
	}
	if got.SNE.ServingCacheCapacity != 4096 || got.SNE.PrefixSessionsMaximum != 2 || got.SNE.PrefixSessionsSupported {
		t.Fatalf("MTP prefix capability was misrepresented: %+v", got.SNE)
	}
	model.Lifecycle.RuntimeSHA256 = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	if _, ok := buildSNEOpenAIModelList(model); ok {
		t.Fatal("uppercase runtime SHA-256 was exposed through /v1/models")
	}
	model.Lifecycle.RuntimeSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	model.RuntimeCatalog.State = "invalid"
	if _, ok := buildSNEOpenAIModelList(model); ok {
		t.Fatal("invalid runtime catalog was exposed through /v1/models")
	}
}

func TestBuildSNEOpenAIModelListRejectsPilotCandidate(t *testing.T) {
	model := SNEReadModel{
		Ready: true, ActiveModel: "candidate", GeneratedAt: time.Unix(123, 0),
		Lifecycle:      SNELifecycleState{State: "ready", ModelID: "candidate", RuntimeID: "runtime", RuntimeSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ModelManifestSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Profile: "interactive"},
		RuntimeCatalog: SNERuntimeCatalogStatus{State: "verified", SignedRequired: true},
		Catalog:        []SNECatalogItem{{ModelID: "candidate", RuntimeID: "runtime", Active: true, SupportStatus: "pilot-candidate", NextGate: "fresh100"}},
	}
	item, admitted := activeReleaseSupportedSNETuple(model)
	if admitted || item.SupportStatus != "pilot-candidate" || item.NextGate != "fresh100" {
		t.Fatalf("pilot candidate admission = %v item=%+v", admitted, item)
	}
	if _, ok := buildSNEOpenAIModelList(model); ok {
		t.Fatal("pilot candidate was exposed through /v1/models")
	}
}

func TestPrefixSessionsRequireExplicitPlainCapacity(t *testing.T) {
	if !supportsPrefixSessions("plain", 2) {
		t.Fatal("plain paged capacity was not exposed")
	}
	for _, test := range []struct {
		mode    string
		maximum int
	}{{"plain", 0}, {"mtp", 2}, {"mtp-shared-wide", 2}} {
		if supportsPrefixSessions(test.mode, test.maximum) {
			t.Fatalf("unsupported prefix-session tuple admitted: %+v", test)
		}
	}
}
