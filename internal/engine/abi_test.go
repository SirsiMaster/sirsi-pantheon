package engine

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testIdentity() Identity {
	return Identity{Engine: KindSNE, EngineVersion: "native-1", ModelID: "model-a", ModelSHA256: testSHA, TokenizerID: "tok-a", TokenizerSHA256: testSHA, Precision: "int4", CacheNamespace: "cache-a"}
}

func TestBackendVariantsAreClosedAndEngineCompatible(t *testing.T) {
	cases := []struct {
		variant BackendVariant
		engine  Kind
	}{
		{VariantMLXRaw, KindMLX}, {VariantMLXPatched, KindMLX},
		{VariantOMLXPublic, KindOMLX}, {VariantSNEPlain, KindSNE}, {VariantSNEMTP, KindSNE},
	}
	for _, tc := range cases {
		if err := tc.variant.Validate(); err != nil {
			t.Errorf("variant %q rejected: %v", tc.variant, err)
		}
		if err := tc.variant.ValidateForEngine(tc.engine); err != nil {
			t.Errorf("variant %q rejected for %q: %v", tc.variant, tc.engine, err)
		}
	}
	for _, tc := range []struct {
		variant BackendVariant
		engine  Kind
	}{{VariantMLXRaw, KindOMLX}, {VariantOMLXPublic, KindSNE}, {VariantSNEMTP, KindMLX}} {
		if err := tc.variant.ValidateForEngine(tc.engine); err == nil {
			t.Errorf("incompatible variant %q accepted for %q", tc.variant, tc.engine)
		}
	}
	if _, err := ParseVariant("not-a-variant"); err == nil {
		t.Fatal("unknown backend variant accepted")
	}
}

func TestRouteDisplayNamesMatchCanonicalProductAndConnectorIdentities(t *testing.T) {
	for _, test := range []struct {
		kind    Kind
		variant BackendVariant
		want    string
	}{
		{KindMLX, VariantMLXRaw, "MLX · Raw"},
		{KindMLX, VariantMLXPatched, "MLX · Patched"},
		{KindOMLX, VariantOMLXPublic, "oMLX · Public"},
		{KindSNE, VariantSNEPlain, "Apollo (Plain)"},
		{KindSNE, VariantSNEMTP, "Apollo Flash (Speculative)"},
	} {
		if got := RouteDisplayName(test.kind, test.variant); got != test.want {
			t.Errorf("RouteDisplayName(%q, %q) = %q, want %q", test.kind, test.variant, got, test.want)
		}
	}
	if got := RouteDisplayName(KindSNE, "unknown-route"); got != "sne · unknown-route" {
		t.Fatalf("unknown route fallback = %q", got)
	}
}

func TestIdentityVariantDefaultsAreExplicitAndDigestBound(t *testing.T) {
	legacy := testIdentity()
	if legacy.Variant != "" || legacy.EffectiveVariant() != VariantSNEPlain {
		t.Fatalf("legacy identity variant = %q/%q", legacy.Variant, legacy.EffectiveVariant())
	}
	explicit := legacy
	explicit.Variant = VariantSNEMTP
	explicit.Assistant = testAssistantIdentity()
	legacyDigest, err := legacy.Digest()
	if err != nil {
		t.Fatal(err)
	}
	explicitDigest, err := explicit.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if legacyDigest == explicitDigest {
		t.Fatal("variant change did not change identity digest")
	}
	if !legacy.Equal(legacy.canonical()) {
		t.Fatal("legacy identity did not compare equal to its effective identity")
	}
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"variant":"sne-plain"`) {
		t.Fatalf("legacy identity did not emit its effective variant: %s", encoded)
	}
}

func TestIdentityValidationUsesStableFieldOrder(t *testing.T) {
	identity := testIdentity()
	identity.EngineVersion = ""
	identity.ModelID = ""
	if err := identity.Validate(); err == nil || !strings.Contains(err.Error(), "engine_version is required") {
		t.Fatalf("missing identity fields should report engine_version first, got %v", err)
	}

	identity = testIdentity()
	identity.ModelSHA256 = "invalid"
	identity.TokenizerSHA256 = "also-invalid"
	if err := identity.Validate(); err == nil || !strings.Contains(err.Error(), "model_sha256 must be lowercase SHA-256") {
		t.Fatalf("malformed identity hashes should report model_sha256 first, got %v", err)
	}
}

func testSession() Session {
	return Session{ID: "session-1", Identity: testIdentity(), CreatedAt: "2026-09-07T16:00:00Z"}
}

func TestIdentityDigestBindsAllExecutionTupleFields(t *testing.T) {
	a := testIdentity()
	b := a
	b.CacheNamespace = "cache-b"
	da, err := a.Digest()
	if err != nil {
		t.Fatal(err)
	}
	db, err := b.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if da == db {
		t.Fatal("cache namespace change did not change identity digest")
	}
	if len(da) != 64 || strings.ToLower(da) != da {
		t.Fatalf("digest = %q, want lowercase SHA-256", da)
	}
}

func TestGenerateRequestRejectsSilentIdentityAndCapabilityChanges(t *testing.T) {
	s := testSession()
	req := GenerateRequest{SessionID: s.ID, Identity: s.Identity, Prompt: "hello", MaxTokens: 8, Stream: true, CacheNamespace: s.Identity.CacheNamespace}
	if err := req.Validate(s, Capabilities{Streaming: true, Cancellation: true, Receipts: true}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	bad := req
	bad.Identity.ModelID = "different-model"
	if err := bad.Validate(s, Capabilities{Streaming: true, Cancellation: true, Receipts: true}); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("identity drift was accepted: %v", err)
	}
	bad = req
	bad.Stream = true
	if err := bad.Validate(s, Capabilities{}); err == nil || !strings.Contains(err.Error(), "streaming") {
		t.Fatalf("unsupported streaming was accepted: %v", err)
	}
	if err := req.Validate(s, Capabilities{Streaming: true, Cancellation: true}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "receipts") {
		t.Fatalf("streaming request without receipt capability was accepted: %v", err)
	}
	if err := req.Validate(s, Capabilities{Streaming: true, Receipts: true}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "cancellation") {
		t.Fatalf("streaming without cancellation capability was accepted: %v", err)
	}
	bad = req
	bad.CacheNamespace = "other-cache"
	if err := bad.Validate(s, Capabilities{Streaming: true, Cancellation: true, Receipts: true}); err == nil || !strings.Contains(err.Error(), "cache namespace") {
		t.Fatalf("cache drift was accepted: %v", err)
	}
}

func TestGenerateRequestRejectsUnsupportedRequiredCapabilitiesBeforeTransport(t *testing.T) {
	s := testSession()
	req := GenerateRequest{
		SessionID: s.ID, Identity: s.Identity, Prompt: "hello", MaxTokens: 8,
		CacheNamespace:       s.Identity.CacheNamespace,
		RequiredCapabilities: []Capability{CapabilityMTP, CapabilityKVState},
	}
	if err := req.Validate(s, Capabilities{KVState: true}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "mtp") {
		t.Fatalf("unsupported MTP capability was not rejected explicitly: %v", err)
	}
	req.RequiredCapabilities = []Capability{CapabilityKVState, CapabilityKVState}
	if err := req.Validate(s, Capabilities{KVState: true}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate capability requirement was accepted: %v", err)
	}
}

func TestGenerateRequestRequiresReceiptCapability(t *testing.T) {
	s := testSession()
	req := GenerateRequest{SessionID: s.ID, Identity: s.Identity, Prompt: "hello", MaxTokens: 8, CacheNamespace: s.Identity.CacheNamespace}
	if err := req.Validate(s, Capabilities{Receipts: true}); err != nil {
		t.Fatalf("request with receipt-capable connector rejected: %v", err)
	}
	if err := req.Validate(s, Capabilities{}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "receipts") {
		t.Fatalf("request accepted without receipt capability: %v", err)
	}
}

func TestMTPIdentityRequiresMTPCapability(t *testing.T) {
	session := testSession()
	session.Identity.Variant = VariantSNEMTP
	session.Identity.Assistant = testAssistantIdentity()
	request := GenerateRequest{
		SessionID: session.ID, Identity: session.Identity, Prompt: "hello", MaxTokens: 8,
		CacheNamespace: session.Identity.CacheNamespace,
	}
	if err := request.Validate(session, Capabilities{Receipts: true}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "mtp") {
		t.Fatalf("MTP identity without MTP capability error = %v", err)
	}
	if err := request.Validate(session, Capabilities{Receipts: true, MTP: true}); err != nil {
		t.Fatalf("MTP identity with declared capability rejected: %v", err)
	}
}

func testAssistantIdentity() *AssistantIdentity {
	return &AssistantIdentity{ModelID: "assistant-a", Revision: "rev-1", CheckpointSHA256: testSHA, Precision: "int8"}
}

func TestMTPAssistantIdentityIsRequiredAndDigestBound(t *testing.T) {
	identity := testIdentity()
	identity.Variant = VariantSNEMTP
	if err := identity.Validate(); err == nil || !strings.Contains(err.Error(), "assistant identity is required") {
		t.Fatalf("MTP identity without assistant accepted: %v", err)
	}
	identity.Assistant = testAssistantIdentity()
	first, err := identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	changed := identity.canonical()
	changed.Assistant.Revision = "rev-2"
	second, err := changed.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || identity.Equal(changed) {
		t.Fatal("assistant revision change did not alter engine identity")
	}
}

func TestSnapshotGenerateRequestDetachesCallerOwnedInputs(t *testing.T) {
	temperature := 0.4
	topP := 0.8
	seed := int64(17)
	choice := "first"
	request := GenerateRequest{
		SessionID: "snapshot", Identity: testIdentity(), Prompt: "hello", MaxTokens: 8,
		Temperature: &temperature, TopP: &topP, Seed: &seed,
		Tools:          []ToolSpec{{Name: "choose", Schema: map[string]any{"choices": []any{choice}}}},
		CacheNamespace: "cache-a", RequiredCapabilities: []Capability{CapabilityReceipts},
	}
	snapshot, digest, err := SnapshotGenerateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	temperature = 1.7
	topP = 0.2
	seed = 99
	choice = "changed"
	request.Prompt = "changed"
	request.RequiredCapabilities[0] = CapabilityTools
	request.Tools[0].Schema["choices"].([]any)[0] = "changed"
	if *snapshot.Temperature != 0.4 || *snapshot.TopP != 0.8 || *snapshot.Seed != 17 || snapshot.Prompt != "hello" {
		t.Fatalf("snapshot retained caller-owned scalar state: %+v", snapshot)
	}
	if got := snapshot.Tools[0].Schema["choices"].([]any)[0]; got != "first" {
		t.Fatalf("snapshot retained caller-owned nested schema: %v", got)
	}
	if snapshot.RequiredCapabilities[0] != CapabilityReceipts {
		t.Fatalf("snapshot retained caller-owned capability slice: %+v", snapshot.RequiredCapabilities)
	}
	if after, err := GenerateRequestDigest(snapshot); err != nil || after != digest {
		t.Fatalf("snapshot digest changed after caller mutation: before=%q after=%q err=%v", digest, after, err)
	}
}

func TestSnapshotGenerateRequestDigestUsesNormalizedJSONValues(t *testing.T) {
	request := GenerateRequest{Tools: []ToolSpec{{Name: "numeric", Schema: map[string]any{
		"limit": json.Number("9007199254740993"),
	}}}}
	originalDigest, err := GenerateRequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, snapshotDigest, err := SnapshotGenerateRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	backendDigest, err := GenerateRequestDigest(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotDigest != backendDigest {
		t.Fatalf("snapshot digest %q does not bind normalized backend input %q", snapshotDigest, backendDigest)
	}
	if snapshotDigest == originalDigest {
		t.Fatal("fixture did not distinguish caller JSON from normalized backend JSON")
	}
}

func TestGenerateRequestRejectsToolsWhenConnectorDoesNotAdvertiseThem(t *testing.T) {
	s := testSession()
	req := GenerateRequest{
		SessionID: s.ID, Identity: s.Identity, Prompt: "hello", MaxTokens: 8,
		CacheNamespace: s.Identity.CacheNamespace,
		Tools:          []ToolSpec{{Name: "inspect", Description: "inspect state"}},
	}
	if err := req.Validate(s, Capabilities{}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), "tools") {
		t.Fatalf("unsupported tools were not rejected explicitly: %v", err)
	}
}

func TestGenerateRequestRejectsUnadvertisedSamplingControl(t *testing.T) {
	s := testSession()
	cases := []struct {
		name string
		edit func(*GenerateRequest)
		want string
	}{
		{name: "temperature", edit: func(r *GenerateRequest) { v := 0.7; r.Temperature = &v }, want: "temperature"},
		{name: "top_p", edit: func(r *GenerateRequest) { v := 0.8; r.TopP = &v }, want: "top_p"},
		{name: "seed", edit: func(r *GenerateRequest) { v := int64(42); r.Seed = &v }, want: "seed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := GenerateRequest{SessionID: s.ID, Identity: s.Identity, Prompt: "hello", MaxTokens: 8, CacheNamespace: s.Identity.CacheNamespace}
			tc.edit(&req)
			if err := req.Validate(s, Capabilities{}); err == nil || !errors.Is(err, ErrUnsupportedCapability) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unadvertised %s was not rejected explicitly: %v", tc.name, err)
			}
		})
	}
}

func TestGenerateRequestRejectsNonFiniteSamplingControls(t *testing.T) {
	s := testSession()
	cases := []struct {
		name        string
		temperature *float64
		topP        *float64
	}{
		{name: "temperature NaN", temperature: testFloat(math.NaN())},
		{name: "temperature infinity", temperature: testFloat(math.Inf(1))},
		{name: "top_p NaN", topP: testFloat(math.NaN())},
		{name: "top_p infinity", topP: testFloat(math.Inf(-1))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := GenerateRequest{
				SessionID: s.ID, Identity: s.Identity, Prompt: "hello", MaxTokens: 8,
				CacheNamespace: s.Identity.CacheNamespace, Temperature: tc.temperature, TopP: tc.topP,
			}
			if err := req.Validate(s, Capabilities{Temperature: true, TopP: true}); err == nil {
				t.Fatal("non-finite sampling control was accepted")
			}
		})
	}
}

func testFloat(value float64) *float64 { return &value }

func TestEventSequenceAndReceiptIdentityAreFailClosed(t *testing.T) {
	e := Event{Kind: EventDelta, SessionID: "session-1", Sequence: 1, Text: "hi"}
	if err := e.Validate(0); err != nil {
		t.Fatal(err)
	}
	if err := (Event{Kind: EventCompleted, SessionID: "session-1", Sequence: 1}).Validate(1); err == nil {
		t.Fatal("duplicate event sequence accepted")
	}
	if err := (Event{Kind: EventCompleted, SessionID: "session-1", Sequence: 2}).Validate(1); err == nil || !strings.Contains(err.Error(), "receipt") {
		t.Fatal("completed event without receipt accepted")
	}
	if err := (Event{Kind: EventCompleted, SessionID: "session-1", Sequence: 2, Receipt: &Receipt{SessionID: "other-session"}}).Validate(1); err == nil || !strings.Contains(err.Error(), "receipt session") {
		t.Fatal("cross-session receipt accepted")
	}
	if err := (Event{Kind: EventCompleted, SessionID: "session-1", Sequence: 2, Receipt: &Receipt{SessionID: "session-1"}}).Validate(1); err == nil || !strings.Contains(err.Error(), "served model identity") {
		t.Fatal("completed event without served model identity accepted")
	}
	if err := (Event{Kind: EventError, SessionID: "session-1", Sequence: 2, ErrorCode: "cancelled"}).Validate(1); err == nil || !strings.Contains(err.Error(), "cancelled receipt") {
		t.Fatal("cancellation event without a cancelled receipt was accepted")
	}
	if err := (Event{Kind: EventError, SessionID: "session-1", Sequence: 2, ErrorCode: "provider_error", Receipt: &Receipt{SessionID: "session-1", Cancelled: true}}).Validate(1); err == nil || !strings.Contains(err.Error(), "non-cancellation errors") {
		t.Fatal("non-cancellation error with a cancelled receipt was accepted")
	}
	s := testSession()
	digest, err := s.Identity.Digest()
	if err != nil {
		t.Fatal(err)
	}
	r := Receipt{ABIVersion: ABIVersion, SessionID: s.ID, Identity: s.Identity, IdentityDigest: digest, RequestSHA256: testSHA, CompletionSHA256: testSHA, StartedAt: s.CreatedAt, FinishedAt: "2026-09-07T16:00:01Z"}
	if err := r.Validate(s); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
	r.StartedAt = "2026-09-07T16:00:02Z"
	if err := r.Validate(s); err == nil || !strings.Contains(err.Error(), "finished_at precedes started_at") {
		t.Fatalf("receipt with reversed timestamps was accepted: %v", err)
	}
	r.StartedAt = s.CreatedAt
	r.IdentityDigest = testSHA
	if err := r.Validate(s); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatal("receipt with mismatched identity digest accepted")
	}
	r.IdentityDigest = digest
	route := RouteDecision{Requested: KindMLX, Selected: KindSNE, DataBoundary: DataBoundaryNotDisclosed, Fallback: true, Rationale: "preferred mlx unavailable; explicit fallback selected sne"}
	r.Route = &route
	route.DataBoundary = ""
	if err := r.Validate(s); err == nil || !strings.Contains(err.Error(), "route data boundary is required") {
		t.Fatalf("route receipt without endpoint classification was accepted: %v", err)
	}
	route.DataBoundary = DataBoundaryNotDisclosed
	if err := r.Validate(s); err != nil {
		t.Fatalf("valid fallback route receipt rejected: %v", err)
	}
	r.Route.Selected = KindMLX
	if err := r.Validate(s); err == nil || !strings.Contains(err.Error(), "selected engine") {
		t.Fatal("route identity drift accepted")
	}
}
