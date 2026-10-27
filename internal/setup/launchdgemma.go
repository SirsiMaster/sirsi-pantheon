package setup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"time"
)

const GemmaNativeWorkerLabel = "ai.sirsi.gemma-worker.native"

// GemmaWorkerPlan selects one observed native instance, never an arbitrary PID.
// These are public pins; the barrier capability lives only in the private debt.
type GemmaWorkerPlan struct {
	Instance          ServiceInstance `json:"instance"`
	Thread            string          `json:"thread"`
	Machine           string          `json:"machine"`
	KernelStart       string          `json:"kernel_start"`
	CDHash            string          `json:"cdhash"`
	ReplacementCDHash string          `json:"replacement_cdhash"`
	Legacy140         bool            `json:"legacy140"`
	Parent            int             `json:"parent"`
	Digest            string          `json:"digest"`
}

func gemmaPlanDigest(p GemmaWorkerPlan) string {
	p.Digest = ""
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validateGemmaWorkerPlan(p GemmaWorkerPlan) error {
	if validateServiceInstance(p.Instance) != nil || p.Instance.Label != GemmaNativeWorkerLabel || p.Parent != 1 || p.Thread == "" || p.Machine == "" || p.KernelStart == "" || len(p.CDHash) != 40 || len(p.ReplacementCDHash) != 40 || p.Digest != gemmaPlanDigest(p) {
		return errors.New("Gemma lifecycle: incomplete exact native worker plan")
	}
	if _, e := hex.DecodeString(p.CDHash); e != nil {
		return e
	}
	if p.Legacy140 && p.CDHash != "c9fa4a594cab29a4a95c2180c0ca64e5dcd0de59" {
		return errors.New("Gemma lifecycle: only independently identified PUBLIC140 legacy image supported")
	}
	return nil
}

// Acknowledgment is written by the actual worker only after its synchronous
// work turn and lease-maintainer goroutine have joined and the typed claim gate
// returned ErrLanePaused. It contains no lease or controller capability.
type GemmaWorkerPauseAck struct {
	Plan GemmaWorkerPlan `json:"plan"`
	At   time.Time       `json:"at"`
}
type gemmaWorkerDebt struct {
	Plan                   GemmaWorkerPlan
	OwnerThread            string
	PreviousThread         string
	Capability             string // private 0600 durable controller record ONLY
	Began                  time.Time
	Phase                  string
	LegacyRetirementIssued bool
	OldOutcome             string
}

// All native operations are closed over the fixed worker profile by the Darwin
// adapter. Tests inject inert operations; callers cannot supply commands/labels.
type gemmaWorkerOperations struct {
	inspect      func(context.Context, string) (GemmaWorkerPlan, error)
	barrier      func(context.Context, routerstore.LaneClaimBarrierRequest) (routerstore.LaneClaimBarrierResult, error)
	ack          func(string) (GemmaWorkerPauseAck, error)
	joined       func(GemmaWorkerPlan) error
	save         func(gemmaWorkerDebt) error
	load         func() (gemmaWorkerDebt, error)
	start        func(context.Context) error
	wait         func(context.Context) error
	now          func() time.Time
	legacyRetire func(context.Context, GemmaWorkerPlan) error
	startLegacy  func(context.Context) error
}

func gemmaWorkerAdvance(d gemmaWorkerOperations, s *gemmaWorkerDebt, phase string) error {
	s.Phase = phase
	return d.save(*s)
}

// No defer releases a barrier. Every uncertain/auth/timeout path retains debt.
// There is no retirement signal: only the selected worker may acknowledge and
// exit at its own joined boundary. No lease expiry is accepted as quiescence.
func applyGemmaWorker(ctx context.Context, d gemmaWorkerOperations, p GemmaWorkerPlan, owner string) error {
	if e := validateGemmaWorkerPlan(p); e != nil {
		return e
	}
	if owner == "" {
		return errors.New("Gemma lifecycle: registered SSA controller required")
	}
	if old, e := d.load(); e == nil && old.Phase != "done" {
		return errors.New("Gemma lifecycle: retained debt requires recovery")
	} else if e != nil && !errors.Is(e, ErrGemmaNoDebt) {
		return e
	}
	actual, e := d.inspect(ctx, p.Thread)
	if e != nil {
		return e
	}
	// Requested protocol mode is not a physical observation. p was validated
	// above, including the exact140 loaded-image gate. Preserve every other pin.
	actual.Legacy140 = p.Legacy140
	actual.Digest = gemmaPlanDigest(actual)
	if actual != p {
		return errors.New("Gemma lifecycle: selected identity changed")
	}
	var secret [16]byte
	if _, e = rand.Read(secret[:]); e != nil {
		return e
	}
	s := gemmaWorkerDebt{Plan: p, OwnerThread: owner, Capability: hex.EncodeToString(secret[:]), Began: d.now(), Phase: "acquire-intent"}
	if e = d.save(s); e != nil {
		return e
	}
	b, e := d.barrier(ctx, routerstore.LaneClaimBarrierRequest{Action: "acquire", Lane: "gemma-pantheon", Token: s.Capability})
	if e != nil {
		return errors.New("Gemma lifecycle: barrier acquisition unknown; debt retained")
	}
	if !b.Paused {
		return errors.New("Gemma lifecycle: barrier not established")
	}
	if e = gemmaWorkerAdvance(d, &s, "paused"); e != nil {
		return e
	}
	return continueGemmaWorker(ctx, d, &s)
}

var ErrGemmaNoDebt = errors.New("Gemma lifecycle: no retained debt")

func recoverGemmaWorker(ctx context.Context, d gemmaWorkerOperations, owner string) error {
	s, e := d.load()
	if e != nil {
		return e
	}
	if e = validateGemmaWorkerPlan(s.Plan); e != nil {
		return e
	}
	if s.Phase == "done" {
		return nil
	}
	// An issued start is never replayed. Read-only successful replacement may be
	// confirmed; an outage/absence remains explicit unknown recovery debt.
	if s.Phase == "start-intent" {
		p, e := d.inspect(ctx, "")
		if e != nil {
			return errors.New("Gemma lifecycle: start outcome unknown; no replay")
		}
		if !sameGemmaArtifacts(s.Plan, p) || p.KernelStart == s.Plan.KernelStart {
			return errors.New("Gemma lifecycle: replacement not confirmed")
		}
		return gemmaWorkerAdvance(d, &s, "done")
	}
	if s.Phase == "release-intent" {
		return errors.New("Gemma lifecycle: release outcome unknown; no start or automatic release replay")
	}
	if owner != s.OwnerThread || s.Phase == "recover-intent" {
		if s.Phase != "recover-intent" {
			var secret [16]byte
			if _, e = rand.Read(secret[:]); e != nil {
				return e
			}
			s.PreviousThread = s.OwnerThread
			s.OwnerThread = owner
			s.Capability = hex.EncodeToString(secret[:])
			if e = gemmaWorkerAdvance(d, &s, "recover-intent"); e != nil {
				return e
			}
		}
		if s.OwnerThread != owner {
			return errors.New("Gemma lifecycle: recovery owner changed")
		}
		_, e = d.barrier(ctx, routerstore.LaneClaimBarrierRequest{Action: "recover", Lane: "gemma-pantheon", Token: s.Capability, PreviousThread: s.PreviousThread})
		if e != nil {
			return errors.New("Gemma lifecycle: exact closed prior controller recovery refused; debt retained")
		}
	} else if s.Phase == "acquire-intent" {
		_, e = d.barrier(ctx, routerstore.LaneClaimBarrierRequest{Action: "acquire", Lane: "gemma-pantheon", Token: s.Capability})
		if e != nil {
			return errors.New("Gemma lifecycle: acquire recovery refused")
		}
	}
	if e = gemmaWorkerAdvance(d, &s, "paused"); e != nil {
		return e
	}
	return continueGemmaWorker(ctx, d, &s)
}
func sameGemmaArtifacts(a, b GemmaWorkerPlan) bool {
	x, y := a.Instance, b.Instance
	return x.Label == y.Label && x.Target == y.Target && x.Plist == y.Plist && x.Program == y.Program && x.PlistSHA256 == y.PlistSHA256 && x.ProgramSHA256 == y.ProgramSHA256 && a.ReplacementCDHash == b.CDHash && a.ReplacementCDHash == b.ReplacementCDHash && a.Machine == b.Machine && b.Parent == 1
}
func continueGemmaWorker(ctx context.Context, d gemmaWorkerOperations, s *gemmaWorkerDebt) error {
	for {
		b, e := d.barrier(ctx, routerstore.LaneClaimBarrierRequest{Action: "status", Lane: "gemma-pantheon", Token: s.Capability})
		if e != nil || !b.Paused {
			return errors.New("Gemma lifecycle: authoritative barrier status unavailable; debt retained")
		}
		if b.ActiveItems == 0 && b.ActiveTasks == 0 {
			ack, ackErr := d.ack(s.Plan.Thread)
			eligible := ackErr == nil && ack.Plan == s.Plan && !ack.At.Before(s.Began)
			if s.Plan.Legacy140 {
				if s.Plan.CDHash != "c9fa4a594cab29a4a95c2180c0ca64e5dcd0de59" || d.legacyRetire == nil || d.startLegacy == nil {
					return errors.New("Gemma lifecycle: recognized legacy140 adapter required")
				}
				if !s.LegacyRetirementIssued {
					s.LegacyRetirementIssued = true
					s.OldOutcome = "legacy140 retirement requested; old outcome not attested"
					if e = gemmaWorkerAdvance(d, s, "legacy-retire-intent"); e != nil {
						return e
					}
					if e = d.legacyRetire(ctx, s.Plan); e != nil {
						return errors.New("Gemma lifecycle: legacy140 retirement unknown; retain barrier/debt, never replay")
					}
				}
				eligible = true // process retirement proof below, NEVER a pause acknowledgment
			}
			if eligible {
				if e = d.joined(s.Plan); e != nil {
					return fmt.Errorf("Gemma lifecycle: selected worker/children not joined; barrier retained: %w", e)
				}
				if e = gemmaWorkerAdvance(d, s, "release-intent"); e != nil {
					return e
				}
				b, e = d.barrier(ctx, routerstore.LaneClaimBarrierRequest{Action: "release", Lane: "gemma-pantheon", Token: s.Capability})
				if e != nil || b.Paused {
					return errors.New("Gemma lifecycle: release unknown; no start performed")
				}
				if e = gemmaWorkerAdvance(d, s, "start-intent"); e != nil {
					return e
				}
				start := d.start
				if s.Plan.Legacy140 {
					start = d.startLegacy
					s.OldOutcome = "legacy140 kernel retirement confirmed; no cooperative pause acknowledgment; original outcome/history retained"
				}
				if e = start(ctx); e != nil {
					return errors.New("Gemma lifecycle: start outcome unknown; no automatic replay")
				}
				p, e := d.inspect(ctx, "")
				if e != nil {
					return errors.New("Gemma lifecycle: replacement readback unavailable; debt retained")
				}
				if !sameGemmaArtifacts(s.Plan, p) || p.KernelStart == s.Plan.KernelStart {
					return errors.New("Gemma lifecycle: replacement identity mismatch")
				}
				return gemmaWorkerAdvance(d, s, "done")
			}
		}
		if e = d.wait(ctx); e != nil {
			return errors.New("Gemma lifecycle: joined pause not observed within bound; barrier retained")
		}
	}
}
