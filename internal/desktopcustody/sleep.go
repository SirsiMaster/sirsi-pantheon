// Package desktopcustody records a narrowly typed, externally authorized
// sleep lifecycle. It intentionally ships no native power implementation:
// SHA supplies that adapter only after issuer and custody qualification.
package desktopcustody

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

const (
	SleepOperation = "sleep"
	maxSleepWindow = 30 * time.Minute
)

var (
	ErrReturnVerificationPending = errors.New("sleep dispatched; independent post-wake verification is required")
	ErrSleepUnknown              = errors.New("sleep outcome is unknown and requires recovery review")
	identifierRE                 = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

type SleepRequest struct {
	ID     string
	Host   string
	WakeAt time.Time
}

// SleepAdmission is a distinct, typed capability. A display grant or a
// scheduler record cannot be adapted into authority to sleep a machine.
type SleepAdmission struct {
	ID                string
	Issuer            string
	Host              string
	Owner             string
	PlatformUUID      string
	BootID            string
	Operation         string
	RequestID         string
	Nonce             string
	ReviewDigest      string
	NotBefore         time.Time
	NotAfter          time.Time
	CustodyGeneration string
}

type QuietReceipt struct {
	ID        string
	Host      string
	BootID    string
	Operation string
	ExpiresAt time.Time
	Digest    string
}

type WakeReceipt struct {
	ID       string
	Host     string
	WakeAt   time.Time
	OwnerTag string
	Readback string
}

// ReturnEvidence is produced from actual powerd sleep/full-wake records and
// the independent M1 observer. Network reachability or DarkWake alone cannot
// satisfy it.
type ReturnEvidence struct {
	RequestID             string
	Host                  string
	PlatformUUID          string
	BootID                string
	SleepRecordID         string
	WakeRecordID          string
	FullWake              bool
	ObservedAt            time.Time
	ObserverReceiptDigest string
	Digest                string
}

type SleepVerifier interface {
	VerifySleep(context.Context, SleepRequest) (SleepAdmission, error)
}

type SleepAction func(context.Context) error

// SleepCustodian is the final managed boundary. Its production implementation
// rechecks boot identity, issuer/revocation, quiet state, owner return, and
// write safety immediately before calling action.
type SleepCustodian interface {
	CurrentQuiet(context.Context, SleepAdmission) (QuietReceipt, error)
	AuthorizeSleep(context.Context, SleepAdmission, QuietReceipt, SleepAction) error
	VerifyReturn(context.Context, SleepAdmission, WakeReceipt) (ReturnEvidence, error)
}

// SleepPower is the only native adapter seam. This package has no shell,
// pmset, GUI, cgo, power command, or fallback implementation.
type SleepPower interface {
	ScheduleWake(context.Context, SleepAdmission, SleepRequest) (WakeReceipt, error)
	ReadWake(context.Context, SleepAdmission, SleepRequest, WakeReceipt) error
	Sleep(context.Context) error
	CancelWake(context.Context, SleepAdmission, SleepRequest, WakeReceipt) error
}

type SleepPhase string

const (
	SleepPrepared       SleepPhase = "prepared"
	WakeScheduled       SleepPhase = "wake-scheduled"
	WakeReadBack        SleepPhase = "wake-readback"
	SleepRequested      SleepPhase = "sleep-requested"
	SleepInvoked        SleepPhase = "native-invoked"
	SleepNativeReturned SleepPhase = "native-returned"
	ReturnPending       SleepPhase = "returned-awaiting-verification"
	SleepVerified       SleepPhase = "verified-return"
	SleepFailed         SleepPhase = "failed"
	SleepUnknown        SleepPhase = "unknown-reconcile-required"
)

// SleepEvent is immutable and redundantly carries the complete typed binding.
// A mixed, partial, or replayed event set consequently fails closed.
type SleepEvent struct {
	Phase             SleepPhase `json:"phase"`
	RequestID         string     `json:"request_id"`
	Host              string     `json:"host"`
	Owner             string     `json:"owner"`
	AdmissionID       string     `json:"admission_id"`
	Issuer            string     `json:"issuer"`
	PlatformUUID      string     `json:"platform_uuid"`
	BootID            string     `json:"boot_id"`
	Operation         string     `json:"operation"`
	OwnerTag          string     `json:"owner_tag"`
	ReviewDigest      string     `json:"review_digest"`
	Nonce             string     `json:"nonce"`
	NotBefore         time.Time  `json:"not_before"`
	NotAfter          time.Time  `json:"not_after"`
	CustodyGeneration string     `json:"custody_generation"`
	NativeError       string     `json:"native_error,omitempty"`
	QuietDigest       string     `json:"quiet_digest,omitempty"`
	WakeID            string     `json:"wake_id,omitempty"`
	WakeAt            time.Time  `json:"wake_at,omitempty"`
	WakeReadback      string     `json:"wake_readback,omitempty"`
	ReturnDigest      string     `json:"return_digest,omitempty"`
	At                time.Time  `json:"at"`
	Detail            string     `json:"detail,omitempty"`
}

type SleepJournal interface {
	BeginSleep(SleepEvent) error
	AppendSleep(SleepEvent) error
	ReadSleep(string) ([]SleepEvent, error)
}

type SleepService struct {
	Verifier  SleepVerifier
	Custodian SleepCustodian
	Power     SleepPower
	Journal   SleepJournal
	Now       func() time.Time
}

func (s SleepService) Dispatch(ctx context.Context, req SleepRequest) error {
	if err := s.dependencies(); err != nil {
		return err
	}
	if err := s.validateDispatchRequest(req); err != nil {
		return err
	}
	a, err := s.Verifier.VerifySleep(ctx, req)
	if err != nil {
		return fmt.Errorf("sleep custody: enrolled issuer denied request: %w", err)
	}
	if err := s.validateAdmission(req, a); err != nil {
		return err
	}
	quiet, err := s.Custodian.CurrentQuiet(ctx, a)
	if err != nil {
		return fmt.Errorf("sleep custody: current workload review unavailable: %w", err)
	}
	if err := s.validateQuiet(a, quiet); err != nil {
		return fmt.Errorf("sleep custody: current workload review is invalid: %w", err)
	}
	if err := s.Journal.BeginSleep(s.event(SleepPrepared, req, a, quiet, WakeReceipt{}, "", "")); err != nil {
		return fmt.Errorf("sleep custody: persist prepared receipt: %w", err)
	}
	wake, err := s.Power.ScheduleWake(ctx, a, req)
	if err != nil {
		return s.fail(req, a, "wake schedule failed", err)
	}
	if err := s.validateWake(req, wake); err != nil {
		return s.unknown(req, a, "wake scheduler returned an invalid receipt", err)
	}
	if err := s.Journal.AppendSleep(s.event(WakeScheduled, req, a, quiet, wake, "", "")); err != nil {
		return s.unknown(req, a, "wake scheduling receipt could not persist", err)
	}
	if err := s.Power.ReadWake(ctx, a, req, wake); err != nil {
		return s.cancelAndFail(ctx, req, a, wake, "wake readback failed", err)
	}
	if err := s.Journal.AppendSleep(s.event(WakeReadBack, req, a, quiet, wake, "", "")); err != nil {
		return s.cancelAndUnknown(ctx, req, a, wake, "wake readback receipt could not persist", err)
	}
	quiet, err = s.Custodian.CurrentQuiet(ctx, a)
	if err != nil {
		return s.cancelAndFail(ctx, req, a, wake, "final workload review unavailable", err)
	}
	if err := s.validateQuiet(a, quiet); err != nil {
		return s.cancelAndFail(ctx, req, a, wake, "final workload review is invalid", err)
	}
	if err := s.Journal.AppendSleep(s.event(SleepRequested, req, a, quiet, wake, "", "")); err != nil {
		return s.cancelAndUnknown(ctx, req, a, wake, "sleep request receipt could not persist", err)
	}
	// AuthorizeSleep must invoke action synchronously and must not retain it.
	// Serialize the callback and close it on return, rejecting duplicate/late calls.
	var mu sync.Mutex
	invoked, closed := false, false
	var actionErr error
	authorizeErr := s.Custodian.AuthorizeSleep(ctx, a, quiet, func(actionCtx context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		if closed || invoked {
			return errors.New("sleep custody: native callback is closed or already invoked")
		}
		if err := s.Journal.AppendSleep(s.event(SleepInvoked, req, a, quiet, wake, "", "native invocation begins; physical outcome requires independent proof")); err != nil {
			actionErr = fmt.Errorf("persist native invocation: %w", err)
			return actionErr
		}
		invoked = true
		nativeErr := s.Power.Sleep(actionCtx)
		e := s.event(SleepNativeReturned, req, a, quiet, wake, "", "native adapter returned; physical outcome remains unverified")
		if nativeErr != nil {
			e.NativeError = nativeErr.Error()
		}
		journalErr := s.Journal.AppendSleep(e)
		actionErr = errors.Join(nativeErr, journalErr)
		return actionErr
	})
	mu.Lock()
	closed = true
	started, nativeErr := invoked, actionErr
	mu.Unlock()
	if err := errors.Join(authorizeErr, nativeErr); err != nil {
		if started {
			// The scheduled wake is a recovery safeguard once native invocation begins.
			return s.unknown(req, a, "native invocation has ambiguous outcome; owned wake retained", err)
		}
		return s.cancelAndFail(ctx, req, a, wake, "final custody fence denied sleep", err)
	}
	if !started {
		return s.cancelAndFail(ctx, req, a, wake, "custodian did not invoke sleep", errors.New("native callback not invoked"))
	}
	if err := s.Journal.AppendSleep(s.event(ReturnPending, req, a, quiet, wake, "", "native operation returned; independent full-wake proof required")); err != nil {
		return s.unknown(req, a, "return-pending receipt could not persist", err)
	}
	return ErrReturnVerificationPending
}

// VerifyReturn deliberately permits a past wake time: it is the post-wake
// half of the transaction, not a second dispatch.
func (s SleepService) VerifyReturn(ctx context.Context, req SleepRequest) error {
	if err := s.dependencies(); err != nil {
		return err
	}
	if err := s.validateReturnRequest(req); err != nil {
		return err
	}
	events, err := s.Journal.ReadSleep(req.ID)
	if err != nil {
		return fmt.Errorf("sleep custody: read lifecycle: %w", err)
	}
	if len(events) == 0 || events[len(events)-1].Phase != ReturnPending {
		return errors.New("sleep custody: request is not awaiting return verification")
	}
	a, err := admissionFromEvents(events)
	if err != nil {
		return s.unknownFromEvents(events, "journal admission binding is invalid", err)
	}
	if err := s.validateAdmissionIdentity(req, a); err != nil {
		return s.unknownFromEvents(events, "journal does not bind this return request", err)
	}
	wake, ok := wakeFromEvents(events)
	if !ok {
		return s.unknown(req, a, "journal lacks an exact wake receipt", ErrSleepUnknown)
	}
	evidence, err := s.Custodian.VerifyReturn(ctx, a, wake)
	if err != nil {
		return s.unknown(req, a, "independent post-wake proof unavailable", err)
	}
	if err := s.validateReturn(req, a, wake, evidence); err != nil {
		return s.unknown(req, a, "post-wake proof is invalid", err)
	}
	if err := s.Power.CancelWake(ctx, a, req, wake); err != nil {
		return s.unknown(req, a, "owned wake cleanup failed", err)
	}
	return s.Journal.AppendSleep(s.event(SleepVerified, req, a, QuietReceipt{}, wake, evidence.Digest, ""))
}

func (s SleepService) dependencies() error {
	if s.Verifier == nil || s.Custodian == nil || s.Power == nil || s.Journal == nil {
		return errors.New("sleep custody is unavailable: trusted issuer, custodian, power adapter, and journal are all required")
	}
	return nil
}
func (s SleepService) validateDispatchRequest(r SleepRequest) error {
	if err := s.validateReturnRequest(r); err != nil {
		return err
	}
	if !r.WakeAt.After(s.now()) || r.WakeAt.Sub(s.now()) > 24*time.Hour {
		return errors.New("sleep custody: wake time must be a bounded future instant before dispatch")
	}
	return nil
}
func (s SleepService) validateReturnRequest(r SleepRequest) error {
	if !identifierRE.MatchString(r.ID) || !identifierRE.MatchString(r.Host) || r.WakeAt.IsZero() {
		return errors.New("sleep custody: request is malformed")
	}
	return nil
}
func (s SleepService) validateAdmission(r SleepRequest, a SleepAdmission) error {
	if a.ID == "" || a.Issuer == "" || a.Host != r.Host || !identifierRE.MatchString(a.Owner) || a.Operation != SleepOperation || a.RequestID != r.ID || a.Nonce == "" || a.ReviewDigest == "" || a.PlatformUUID == "" || a.BootID == "" || a.CustodyGeneration == "" || a.NotBefore.IsZero() || a.NotAfter.IsZero() {
		return errors.New("sleep custody: admission does not have the required typed binding")
	}
	now := s.now()
	if now.Before(a.NotBefore.UTC()) || !now.Before(a.NotAfter.UTC()) || a.NotAfter.Sub(a.NotBefore) > maxSleepWindow || !r.WakeAt.Before(a.NotAfter.UTC()) {
		return errors.New("sleep custody: admission window is not active and bounded")
	}
	return nil
}
func (s SleepService) validateAdmissionIdentity(r SleepRequest, a SleepAdmission) error {
	if a.ID == "" || a.Host != r.Host || a.RequestID != r.ID || a.Operation != SleepOperation || a.Issuer == "" || !identifierRE.MatchString(a.Owner) || a.PlatformUUID == "" || a.BootID == "" || a.ReviewDigest == "" || a.CustodyGeneration == "" {
		return errors.New("sleep custody: admission identity mismatch")
	}
	return nil
}
func (s SleepService) validateQuiet(a SleepAdmission, q QuietReceipt) error {
	if q.ID == "" || q.Host != a.Host || q.BootID != a.BootID || q.Operation != SleepOperation || q.Digest == "" || !s.now().Before(q.ExpiresAt.UTC()) || q.ExpiresAt.After(a.NotAfter.UTC()) {
		return errors.New("quiet receipt does not bind current typed sleep admission")
	}
	return nil
}
func (s SleepService) validateWake(r SleepRequest, w WakeReceipt) error {
	if w.ID == "" || w.Host != r.Host || !w.WakeAt.Equal(r.WakeAt) || w.OwnerTag != "pantheon-sleep-"+r.ID || w.Readback == "" {
		return errors.New("wake receipt does not bind exact owned wake event")
	}
	return nil
}
func (s SleepService) validateReturn(r SleepRequest, a SleepAdmission, w WakeReceipt, e ReturnEvidence) error {
	if e.RequestID != r.ID || e.Host != a.Host || e.PlatformUUID != a.PlatformUUID || e.BootID != a.BootID || e.SleepRecordID == "" || e.WakeRecordID != w.ID || !e.FullWake || e.ObservedAt.IsZero() || e.ObservedAt.Before(w.WakeAt) || e.ObserverReceiptDigest == "" || e.Digest == "" {
		return errors.New("return evidence does not bind actual full wake and independent observer proof")
	}
	return nil
}
func (s SleepService) event(p SleepPhase, r SleepRequest, a SleepAdmission, q QuietReceipt, w WakeReceipt, returnDigest, detail string) SleepEvent {
	return SleepEvent{Phase: p, RequestID: r.ID, Host: a.Host, Owner: a.Owner, AdmissionID: a.ID, Issuer: a.Issuer, PlatformUUID: a.PlatformUUID, BootID: a.BootID, Operation: a.Operation, OwnerTag: "pantheon-sleep-" + r.ID, ReviewDigest: a.ReviewDigest, Nonce: a.Nonce, NotBefore: a.NotBefore, NotAfter: a.NotAfter, CustodyGeneration: a.CustodyGeneration, QuietDigest: q.Digest, WakeID: w.ID, WakeAt: w.WakeAt, WakeReadback: w.Readback, ReturnDigest: returnDigest, At: s.now(), Detail: detail}
}
func (s SleepService) cancelAndFail(c context.Context, r SleepRequest, a SleepAdmission, w WakeReceipt, d string, cause error) error {
	if err := s.Power.CancelWake(c, a, r, w); err != nil {
		return s.unknown(r, a, d+"; owned wake cleanup failed", errors.Join(cause, err))
	}
	return s.fail(r, a, d, cause)
}
func (s SleepService) cancelAndUnknown(c context.Context, r SleepRequest, a SleepAdmission, w WakeReceipt, d string, cause error) error {
	if err := s.Power.CancelWake(c, a, r, w); err != nil {
		cause = errors.Join(cause, err)
	}
	return s.unknown(r, a, d, cause)
}
func (s SleepService) fail(r SleepRequest, a SleepAdmission, d string, cause error) error {
	if err := s.Journal.AppendSleep(s.event(SleepFailed, r, a, QuietReceipt{}, WakeReceipt{}, "", d)); err != nil {
		return fmt.Errorf("sleep custody: %s: %w; failure receipt: %v", d, cause, err)
	}
	return fmt.Errorf("sleep custody: %s: %w", d, cause)
}
func (s SleepService) unknown(r SleepRequest, a SleepAdmission, d string, cause error) error {
	if err := s.Journal.AppendSleep(s.event(SleepUnknown, r, a, QuietReceipt{}, WakeReceipt{}, "", d)); err != nil {
		return fmt.Errorf("sleep custody: %s: %w; unknown-outcome receipt: %v", d, cause, err)
	}
	return fmt.Errorf("%w: %s: %v", ErrSleepUnknown, d, cause)
}
func (s SleepService) unknownFromEvents(events []SleepEvent, d string, cause error) error {
	if len(events) == 0 {
		return fmt.Errorf("%w: %s: %v", ErrSleepUnknown, d, cause)
	}
	e := events[0]
	e.Phase, e.At, e.Detail = SleepUnknown, s.now(), d
	err := s.Journal.AppendSleep(e)
	if err != nil {
		return fmt.Errorf("sleep custody: %s: %w; unknown-outcome receipt: %v", d, cause, err)
	}
	return fmt.Errorf("%w: %s: %v", ErrSleepUnknown, d, cause)
}
func (s SleepService) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}
func admissionFromEvents(events []SleepEvent) (SleepAdmission, error) {
	if len(events) == 0 {
		return SleepAdmission{}, errors.New("empty sleep journal")
	}
	if err := validateSleepEvents(events); err != nil {
		return SleepAdmission{}, err
	}
	e := events[0]
	if e.Phase != SleepPrepared {
		return SleepAdmission{}, errors.New("journal root is not prepared")
	}
	return SleepAdmission{ID: e.AdmissionID, Issuer: e.Issuer, Host: e.Host, Owner: e.Owner, PlatformUUID: e.PlatformUUID, BootID: e.BootID, Operation: e.Operation, RequestID: e.RequestID, ReviewDigest: e.ReviewDigest, Nonce: e.Nonce, NotBefore: e.NotBefore, NotAfter: e.NotAfter, CustodyGeneration: e.CustodyGeneration}, nil
}
func wakeFromEvents(events []SleepEvent) (WakeReceipt, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.Phase == WakeReadBack && e.WakeID != "" {
			return WakeReceipt{ID: e.WakeID, Host: e.Host, WakeAt: e.WakeAt, OwnerTag: e.OwnerTag, Readback: e.WakeReadback}, true
		}
	}
	return WakeReceipt{}, false
}
