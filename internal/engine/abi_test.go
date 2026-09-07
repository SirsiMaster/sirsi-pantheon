package engine

import (
	"errors"
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testIdentity() Identity {
	return Identity{Engine: KindSNE, EngineVersion: "native-1", ModelID: "model-a", ModelSHA256: testSHA, TokenizerID: "tok-a", TokenizerSHA256: testSHA, Precision: "int4", CacheNamespace: "cache-a"}
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
	if err := req.Validate(s, Capabilities{Streaming: true}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	bad := req
	bad.Identity.ModelID = "different-model"
	if err := bad.Validate(s, Capabilities{Streaming: true}); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("identity drift was accepted: %v", err)
	}
	bad = req
	bad.Stream = true
	if err := bad.Validate(s, Capabilities{}); err == nil || !strings.Contains(err.Error(), "streaming") {
		t.Fatalf("unsupported streaming was accepted: %v", err)
	}
	bad = req
	bad.CacheNamespace = "other-cache"
	if err := bad.Validate(s, Capabilities{Streaming: true}); err == nil || !strings.Contains(err.Error(), "cache namespace") {
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

func TestEventSequenceAndReceiptIdentityAreFailClosed(t *testing.T) {
	e := Event{Kind: EventDelta, SessionID: "session-1", Sequence: 1, Text: "hi"}
	if err := e.Validate(0); err != nil {
		t.Fatal(err)
	}
	if err := (Event{Kind: EventCompleted, SessionID: "session-1", Sequence: 1}).Validate(1); err == nil {
		t.Fatal("duplicate event sequence accepted")
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
	r.IdentityDigest = testSHA
	if err := r.Validate(s); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatal("receipt with mismatched identity digest accepted")
	}
}
