package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

// SNEControlClient is the native SNE control surface consumed by Pantheon.
// The interface intentionally contains no process, shell, or path operations;
// lifecycle ownership stays in the native SNE supervisor.
type SNEControlClient interface {
	ReadinessIdentity(context.Context) (sne.ServiceReadinessIdentity, error)
	LoadModel(context.Context, string) error
	UnloadModel(context.Context, string) error
	ReloadModel(context.Context, string) error
}

type SNELifecycleAction string

const (
	SNELoad   SNELifecycleAction = "load"
	SNEUnload SNELifecycleAction = "unload"
	SNEReload SNELifecycleAction = "reload"
)

// SNEReadiness is a timestamped readback of the native service identity. A
// configured model name is not treated as proof of service state.
type SNEReadiness struct {
	ObservedAt  time.Time                    `json:"observed_at"`
	Identity    sne.ServiceReadinessIdentity `json:"identity"`
	ServedModel string                       `json:"served_model"`
	Ready       bool                         `json:"ready"`
}

// SNELifecycle is the result of one native lifecycle transition and both
// identity readbacks surrounding it.
type SNELifecycle struct {
	Action     SNELifecycleAction `json:"action"`
	ModelID    string             `json:"model_id"`
	StartedAt  time.Time          `json:"started_at"`
	FinishedAt time.Time          `json:"finished_at"`
	Before     SNEReadiness       `json:"before"`
	After      SNEReadiness       `json:"after"`
}

// SNERecovery is a plan-only, hash-bound recovery decision. Execution remains
// with the existing receipt-bound recovery/lifecycle owners.
type SNERecovery struct {
	Plan       sne.RecoveryPlan `json:"plan"`
	PlanSHA256 string           `json:"plan_sha256"`
}

// SNEBenchmarkSession is the Pantheon-facing name for the existing native
// benchmark provenance/session contract.
type SNEBenchmarkSession struct {
	Session sne.BenchmarkSession `json:"session"`
}

// SNEControl composes Pantheon's engine layer with the native SNE control
// surface. It never edits the native SNE package or invents a second lifecycle
// authority.
type SNEControl struct {
	client  SNEControlClient
	modelID string
	now     func() time.Time
}

func NewSNEControl(client SNEControlClient, modelID string) (*SNEControl, error) {
	if client == nil {
		return nil, fmt.Errorf("SNE control: client is required")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil, fmt.Errorf("SNE control: model is required")
	}
	return &SNEControl{client: client, modelID: modelID, now: time.Now}, nil
}

func (c *SNEControl) Readiness(ctx context.Context) (SNEReadiness, error) {
	if c == nil || c.client == nil {
		return SNEReadiness{}, fmt.Errorf("SNE control: client is required")
	}
	identity, err := c.client.ReadinessIdentity(ctx)
	if err != nil {
		return SNEReadiness{}, fmt.Errorf("SNE control: readiness: %w", err)
	}
	readiness := c.readiness(identity)
	if !readiness.Ready {
		return SNEReadiness{}, fmt.Errorf("SNE control: service/model is not ready: status=%q model=%q", identity.Status, readiness.ServedModel)
	}
	return readiness, nil
}

func (c *SNEControl) Apply(ctx context.Context, action SNELifecycleAction) (SNELifecycle, error) {
	if c == nil || c.client == nil {
		return SNELifecycle{}, fmt.Errorf("SNE control: client is required")
	}
	if action != SNELoad && action != SNEUnload && action != SNEReload {
		return SNELifecycle{}, fmt.Errorf("SNE control: unsupported lifecycle action %q", action)
	}
	started := c.clock().UTC()
	beforeIdentity, err := c.client.ReadinessIdentity(ctx)
	if err != nil {
		return SNELifecycle{}, fmt.Errorf("SNE control: preflight readiness: %w", err)
	}
	before := c.readiness(beforeIdentity)
	if !before.Ready || (before.ServedModel != "" && before.ServedModel != c.modelID) {
		return SNELifecycle{}, fmt.Errorf("SNE control: preflight identity is not admitted: status=%q model=%q", beforeIdentity.Status, before.ServedModel)
	}
	switch action {
	case SNELoad:
		err = c.client.LoadModel(ctx, c.modelID)
	case SNEUnload:
		err = c.client.UnloadModel(ctx, c.modelID)
	case SNEReload:
		err = c.client.ReloadModel(ctx, c.modelID)
	}
	if err != nil {
		return SNELifecycle{}, fmt.Errorf("SNE control: %s %s: %w", action, c.modelID, err)
	}
	afterIdentity, err := c.client.ReadinessIdentity(ctx)
	if err != nil {
		return SNELifecycle{}, fmt.Errorf("SNE control: postflight readiness: %w", err)
	}
	after := c.readiness(afterIdentity)
	if action != SNEUnload && (!after.Ready || after.ServedModel != c.modelID) {
		return SNELifecycle{}, fmt.Errorf("SNE control: %s did not restore admitted identity: status=%q model=%q", action, afterIdentity.Status, after.ServedModel)
	}
	if action == SNEUnload && after.ServedModel != "" && after.ServedModel != c.modelID {
		return SNELifecycle{}, fmt.Errorf("SNE control: unload readback served unexpected model %q", after.ServedModel)
	}
	return SNELifecycle{Action: action, ModelID: c.modelID, StartedAt: started, FinishedAt: c.clock().UTC(), Before: before, After: after}, nil
}

func (c *SNEControl) PlanRecovery(request sne.RecoveryRequest) (SNERecovery, error) {
	plan, err := sne.PlanRecovery(request)
	if err != nil {
		return SNERecovery{}, fmt.Errorf("SNE control: recovery plan: %w", err)
	}
	digest, err := sne.RecoveryPlanSHA256(plan)
	if err != nil {
		return SNERecovery{}, fmt.Errorf("SNE control: recovery plan digest: %w", err)
	}
	return SNERecovery{Plan: plan, PlanSHA256: digest}, nil
}

func NewSNEBenchmarkSession(provenance sne.BenchmarkProvenance) (SNEBenchmarkSession, error) {
	session, err := sne.NewBenchmarkSession(provenance)
	if err != nil {
		return SNEBenchmarkSession{}, fmt.Errorf("SNE control: benchmark session: %w", err)
	}
	return SNEBenchmarkSession{Session: session}, nil
}

func (s SNEBenchmarkSession) Validate() error {
	return s.Session.Validate()
}

func (c *SNEControl) readiness(identity sne.ServiceReadinessIdentity) SNEReadiness {
	servedModel := strings.TrimSpace(identity.ReadyModelID)
	if servedModel == "" {
		servedModel = strings.TrimSpace(identity.LoadedModel)
	}
	return SNEReadiness{
		ObservedAt:  c.clock().UTC(),
		Identity:    identity,
		ServedModel: servedModel,
		Ready:       strings.EqualFold(strings.TrimSpace(identity.Status), "ready") && servedModel == c.modelID,
	}
}

func (c *SNEControl) clock() time.Time {
	if c != nil && c.now != nil {
		return c.now()
	}
	return time.Now()
}
