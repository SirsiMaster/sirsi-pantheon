package desktopcustody

import (
	"context"
	"errors"
	"time"
)

type HostIdentity struct{ Host, PlatformUUID, BootID, ExecutableSHA256 string }

type ObserverEvidence struct {
	RequestID, Host, PlatformUUID, BootID string
	LostAt, ReturnedAt                    time.Time
	ContinuitySHA256                      string
}

// Participant is an exact enrolled service identity, not an agent-name role.
// Management binds these fields; the RPC server obtains them independently
// from the authenticated bearer/session and adopted machine record.
type Participant struct {
	TokenID, SessionID, MachineID, Host, Agent, ThreadID, RuntimeSHA256 string
}

// Delegation is created only through the canonical service's management
// backend. It authorizes one sleep request, never arbitrary power commands.
type Delegation struct {
	Request                               SleepRequest
	Admission                             SleepAdmission
	OwnerRequestRef, OwnerRequestSHA256   string
	ExecutableSHA256, CandidateSHA256     string
	InventorySHA256                       string
	ObserverMachineLabel                  string
	CustodianReservationID, CustodianWork string
	Coordinator, Custodian, Observer      Participant
}

type CustodyCall struct {
	Operation       string // inspect, ready, consume, returned
	ID              string
	Role            string // coordinator, custodian, observer
	Identity        HostIdentity
	InventorySHA256 string
	QuietDigest     string
	Return          *ReturnEvidence
	Observation     *ObserverEvidence
}

type CustodyReady struct {
	At     time.Time
	Digest string
}

type CustodyState struct {
	Delegation            Delegation
	Ready                 map[string]CustodyReady
	ConsumedAt, RevokedAt time.Time
	RevocationReason      string
	Return                *ReturnEvidence
	Observation           *ObserverEvidence
}

// OnlineAuthority must be the already enrolled canonical service transport.
// Production construction refuses local stores and caller-selected authorities.
type OnlineAuthority interface {
	DesktopSleep(context.Context, CustodyCall) (CustodyState, error)
}

// OnlineController implements the verifier and final custodian over one live
// authority. Inventory is independently re-read immediately before consume.
// An uncertain consume is never retried automatically or treated as permission.
type OnlineController struct {
	Authority   OnlineAuthority
	ID          string
	Identity    func(context.Context) (HostIdentity, error)
	Inventory   func(context.Context) (string, error)
	ReturnProof func(context.Context, SleepAdmission, WakeReceipt) (ReturnEvidence, error)
}

func (c *OnlineController) call(ctx context.Context, op string) (CustodyState, error) {
	if c == nil || c.Authority == nil || c.Identity == nil || c.Inventory == nil || c.ID == "" {
		return CustodyState{}, errors.New("sleep custody: canonical authority and native bindings required")
	}
	i, err := c.Identity(ctx)
	if err != nil {
		return CustodyState{}, err
	}
	inv, err := c.Inventory(ctx)
	if err != nil {
		return CustodyState{}, err
	}
	return c.Authority.DesktopSleep(ctx, CustodyCall{Operation: op, ID: c.ID, Role: "custodian", Identity: i, InventorySHA256: inv})
}

func (c *OnlineController) VerifySleep(ctx context.Context, req SleepRequest) (SleepAdmission, error) {
	s, err := c.call(ctx, "inspect")
	if err != nil {
		return SleepAdmission{}, err
	}
	r := s.Delegation.Request
	if r.ID != req.ID || r.Host != req.Host || !r.WakeAt.Equal(req.WakeAt) {
		return SleepAdmission{}, errors.New("sleep custody: exact request differs from live delegation")
	}
	return s.Delegation.Admission, nil
}

func (c *OnlineController) CurrentQuiet(ctx context.Context, a SleepAdmission) (QuietReceipt, error) {
	s, err := c.call(ctx, "ready")
	if err != nil {
		return QuietReceipt{}, err
	}
	if s.Delegation.Admission != a {
		return QuietReceipt{}, errors.New("sleep custody: admission changed")
	}
	r, ok := s.Ready["custodian"]
	if !ok {
		return QuietReceipt{}, errors.New("sleep custody: custodian receipt missing")
	}
	return QuietReceipt{ID: a.ID, Host: a.Host, BootID: a.BootID, Operation: SleepOperation, ExpiresAt: r.At.Add(15 * time.Second), Digest: r.Digest}, nil
}

func (c *OnlineController) AuthorizeSleep(ctx context.Context, a SleepAdmission, q QuietReceipt, action SleepAction) error {
	if action == nil || q.ID != a.ID {
		return errors.New("sleep custody: invalid native callback binding")
	}
	s, err := c.call(ctx, "consume")
	if err != nil {
		return err
	}
	if s.Delegation.Admission != a || s.ConsumedAt.IsZero() || s.Ready["custodian"].Digest != q.Digest {
		return errors.New("sleep custody: live consume differs from prepared custody")
	}
	return action(ctx)
}

func (c *OnlineController) VerifyReturn(ctx context.Context, a SleepAdmission, w WakeReceipt) (ReturnEvidence, error) {
	if c.ReturnProof == nil {
		return ReturnEvidence{}, errors.New("sleep custody: actual full-wake and independent observer adapter required")
	}
	e, err := c.ReturnProof(ctx, a, w)
	if err != nil {
		return ReturnEvidence{}, err
	}
	s, err := c.Authority.DesktopSleep(ctx, CustodyCall{Operation: "returned", ID: c.ID, Role: "custodian", Return: &e})
	if err != nil {
		return ReturnEvidence{}, err
	}
	if s.Return == nil || *s.Return != e {
		return ReturnEvidence{}, errors.New("sleep custody: canonical return proof not accepted")
	}
	return e, nil
}
