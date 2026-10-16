// Package desktopcustody protects disruptive desktop operations behind a
// durable, host-local, explicitly bounded unattended-use grant. A Ma'at
// reservation is intentionally not an input to this package: reservations
// coordinate capacity, whereas blanking an owner's display requires a
// separate affirmative authority that can be revoked immediately.
package desktopcustody

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

const maxUnattendedWindow = 30 * time.Minute

var (
	ErrAbsent    = errors.New("desktop custody is absent")
	ErrExpired   = errors.New("desktop custody has expired")
	ErrRevoked   = errors.New("desktop custody has been revoked")
	ErrContested = errors.New("desktop custody is contested")
)

var identifierRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Grant is the only authority accepted immediately before a disruptive display
// operation. It deliberately has no reservation field: a capacity reservation
// cannot become operator consent by association. Owner is a stable binding for
// the trusted admission issuer, not an authentication mechanism on its own;
// cross-privilege issuance belongs to a managed broker or signed admission.
type Grant struct {
	ID              string    `json:"id"`
	Host            string    `json:"host"`
	Owner           string    `json:"owner"`
	UnattendedStart time.Time `json:"unattended_start"`
	UnattendedEnd   time.Time `json:"unattended_end"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// Actuator is intentionally tiny. Production may wrap a platform operation,
// but the gate owns the authorization check and tests use a fake actuator. The
// package itself never calls pmset, osascript, or any display API.
type Actuator interface {
	DisruptDisplay(context.Context) error
}

// Store persists grants and one-way revocations. A production implementation
// must survive process restart; a memory implementation is test-only.
type Store interface {
	CreateGrant(Grant) error
	LoadGrant(string) (Grant, error)
	CreateRevocation(id, reason string, at time.Time) error
	Revocation(id string) (Revocation, bool, error)
}

// Locker is implemented by stores that can serialize custody decisions across
// Service instances and processes. The FileStore implementation takes an
// advisory lock on the retained root descriptor. A store without this boundary
// is suitable only for deterministic unit tests, never for a real actuator.
type Locker interface {
	WithLock(func() error) error
}

// Revocation records the owner-use or contention event that made a grant
// unusable. Revocation is one-way: a new explicitly bounded grant is required
// to resume any disruptive operation.
type Revocation struct {
	GrantID string    `json:"grant_id"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// Service serializes revocation and authorization through one lock. It checks
// durable state directly before calling the actuator; there is no earlier
// authorization result that callers can cache or reuse.
type Service struct {
	store Store
	now   func() time.Time
	mu    sync.Mutex
}

func New(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// WithClock supplies a deterministic clock for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// Arm creates one durable grant. The caller must supply an explicit bounded
// unattended window; a blank window, an overlong window, or a window extending
// past expiry is never silently normalized.
func (s *Service) Arm(g Grant) (Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateGrant(g, s.now().UTC()); err != nil {
		return Grant{}, err
	}
	if g.ID == "" {
		id, err := randomID()
		if err != nil {
			return Grant{}, fmt.Errorf("desktop custody: create grant id: %w", err)
		}
		g.ID = id
	}
	if err := s.store.CreateGrant(g); err != nil {
		return Grant{}, fmt.Errorf("desktop custody: persist grant: %w", err)
	}
	return g, nil
}

// Revoke records a durable one-way owner-use or contention revocation. It is
// safe to call repeatedly: the first durable record wins, while later callers
// learn that the grant was already revoked rather than changing its history.
func (s *Service) Revoke(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withStoreLock(func() error {
		if !identifierRE.MatchString(id) {
			return errors.New("desktop custody: invalid grant id")
		}
		if reason == "" {
			return errors.New("desktop custody: revocation reason is required")
		}
		if _, err := s.store.LoadGrant(id); err != nil {
			return fmt.Errorf("desktop custody: load grant for revocation: %w", err)
		}
		if _, exists, err := s.store.Revocation(id); err != nil {
			return fmt.Errorf("desktop custody: read revocation: %w", err)
		} else if exists {
			return nil
		}
		if err := s.store.CreateRevocation(id, reason, s.now().UTC()); err != nil {
			return fmt.Errorf("desktop custody: persist revocation: %w", err)
		}
		return nil
	})
}

// AuthorizeAndAct performs the final, serialized, durable check immediately
// before one disruptive operation. It is deliberately not an authorization
// token API: callers cannot separate this check from actuator invocation.
func (s *Service) AuthorizeAndAct(ctx context.Context, host, owner, grantID string, actuator Actuator) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withStoreLock(func() error {
		if actuator == nil {
			return errors.New("desktop custody: display actuator is required")
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("desktop custody: action context is not active: %w", err)
		}
		g, err := s.store.LoadGrant(grantID)
		if err != nil {
			return fmt.Errorf("desktop custody: load grant: %w", err)
		}
		if g.Host != host || g.Owner != owner {
			return ErrContested
		}
		if _, exists, err := s.store.Revocation(grantID); err != nil {
			return fmt.Errorf("desktop custody: read revocation: %w", err)
		} else if exists {
			return ErrRevoked
		}
		now := s.now().UTC()
		if !now.Before(g.ExpiresAt.UTC()) || now.Before(g.UnattendedStart.UTC()) || !now.Before(g.UnattendedEnd.UTC()) {
			return ErrExpired
		}
		// This call is intentionally adjacent to the actuator boundary. Revoke uses
		// the same retained-root lock, so a completed revocation always wins over a
		// later action even when another process owns the other Service instance.
		return actuator.DisruptDisplay(ctx)
	})
}

func (s *Service) withStoreLock(fn func() error) error {
	locker, ok := s.store.(Locker)
	if !ok {
		return errors.New("desktop custody: production store must provide cross-process serialization")
	}
	return locker.WithLock(fn)
}

func validateGrant(g Grant, now time.Time) error {
	if g.ID != "" && !identifierRE.MatchString(g.ID) {
		return errors.New("desktop custody: invalid grant id")
	}
	if !identifierRE.MatchString(g.Host) || !identifierRE.MatchString(g.Owner) {
		return errors.New("desktop custody: host and owner must be stable identifiers")
	}
	if g.UnattendedStart.IsZero() || g.UnattendedEnd.IsZero() || g.ExpiresAt.IsZero() {
		return errors.New("desktop custody: explicit unattended window and expiry are required")
	}
	start, end, expiry := g.UnattendedStart.UTC(), g.UnattendedEnd.UTC(), g.ExpiresAt.UTC()
	if !start.Before(end) || end.Sub(start) > maxUnattendedWindow || end.After(expiry) || !now.Before(expiry) {
		return errors.New("desktop custody: unattended window is not bounded by an active expiry")
	}
	return nil
}

func randomID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
