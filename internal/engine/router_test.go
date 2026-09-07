package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type routerFixtureConnector struct {
	kind      Kind
	identity  Identity
	caps      Capabilities
	available bool
}

func (f routerFixtureConnector) Kind() Kind                 { return f.kind }
func (f routerFixtureConnector) Capabilities() Capabilities { return f.caps }
func (f routerFixtureConnector) OpenSession(_ context.Context, id string) (Session, error) {
	if !f.available {
		return Session{}, errors.New("fixture unavailable")
	}
	return Session{ID: id, Identity: f.identity, CreatedAt: "2026-09-07T16:00:00Z"}, nil
}
func (f routerFixtureConnector) Complete(_ context.Context, _ Session, _ GenerateRequest) (Completion, Receipt, error) {
	return Completion{Text: "ok", Model: f.identity.ModelID, FinishReason: "stop"}, Receipt{}, nil
}
func (f routerFixtureConnector) Stream(_ context.Context, _ Session, _ GenerateRequest) (<-chan Event, error) {
	return nil, nil
}

func identityFor(kind Kind) Identity {
	i := testIdentity()
	i.Engine = kind
	return i
}

func TestRouterRequiresExplicitFallbackAndPreservesSelectedIdentity(t *testing.T) {
	r, err := NewRouter(
		routerFixtureConnector{kind: KindMLX, identity: identityFor(KindMLX), caps: Capabilities{Sessions: true}, available: false},
		routerFixtureConnector{kind: KindSNE, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true, MTP: true}, available: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.OpenSession(context.Background(), "no-fallback", RoutePolicy{Preferred: KindMLX}); err == nil {
		t.Fatal("router silently fell back without explicit permission")
	}
	session, decision, err := r.OpenSession(context.Background(), "fallback", RoutePolicy{Preferred: KindMLX, AllowFallback: true, RequiredCapabilities: []Capability{CapabilityMTP}})
	if err != nil {
		t.Fatal(err)
	}
	if session.Identity.Engine != KindSNE || decision.Selected != KindSNE || !decision.Fallback {
		t.Fatalf("fallback session/decision = %+v/%+v", session, decision)
	}
	if !strings.Contains(decision.Rationale, "explicit fallback") {
		t.Fatalf("fallback rationale = %q", decision.Rationale)
	}
}

func TestRouterRejectsCapabilityGapBeforeConnectorAdmission(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{kind: KindSNE, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.OpenSession(context.Background(), "cap-gap", RoutePolicy{Preferred: KindSNE, RequiredCapabilities: []Capability{CapabilityKVState}})
	if err == nil || !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("capability gap = %v", err)
	}
}

func TestRouterRejectsDecisionSessionEngineMismatch(t *testing.T) {
	r, err := NewRouter(routerFixtureConnector{kind: KindSNE, identity: identityFor(KindSNE), caps: Capabilities{Sessions: true}, available: true})
	if err != nil {
		t.Fatal(err)
	}
	_, decision, err := r.OpenSession(context.Background(), "match", RoutePolicy{Preferred: KindSNE})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{ID: "match", Identity: identityFor(KindMLX), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, _, err := r.Complete(context.Background(), session, GenerateRequest{}, decision); err == nil {
		t.Fatal("decision/session engine mismatch was accepted")
	}
}
