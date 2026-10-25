package authority

// ADR-120 §2/§6 (step 3, adr120-s3-01-server-cmd): `sirsi maat authority
// serve` wraps Authority.Reserve/Renew/CatchUp over HTTP for pod members,
// following the rs-08-serve pattern (ADR-062, internal/routerstore/serve.go)
// at the scope ADR-120 §6 actually asks for: a single pod secret bearer —
// not the router service's full session/signature/nonce/runtime chain, which
// is ADR-062's own node credential and never leaves its Mac (§6: "The cloud
// token never leaves a Mac") — plus a bounded per-call deadline, an
// in-flight cap, and a server-decided (never client-trusted) lease TTL.

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// ServerOptions configure Handler.
type ServerOptions struct {
	// PodSecret every request must present as "Bearer <secret>" (ADR-120 §6).
	PodSecret string
	// CallTimeout bounds every Reserve/Renew/CatchUp call server-side.
	CallTimeout time.Duration
	// MaxInFlight caps concurrent calls; a request past the cap gets 503
	// immediately instead of queuing unbounded goroutines (rs-08's in-flight cap).
	MaxInFlight int
	// MaxLeaseTTLSec bounds the LeaseTTLSec a caller may request. 0 or a
	// value over the cap is clamped down to it server-side — the authority
	// decides the lease, never the caller (rs-08's server-issued lease
	// expiry, e.g. Wait clamped to ServerOptions.MaxWait).
	MaxLeaseTTLSec int
}

const (
	defaultCallTimeout    = 10 * time.Second
	defaultMaxInFlight    = 8
	defaultMaxLeaseTTLSec = 600
)

type reserveRequest struct {
	Reservation schedule.Reservation `json:"reservation"`
	Queue       bool                 `json:"queue"`
}

type wireError struct {
	Error string `json:"error"`
}

// Handler serves auth's Reserve/Renew/CatchUp over HTTP for pod members
// (ADR-120 §2, §6). One instance per Authority — "one authority, no
// failover" (§2): a second process only ever wraps a second Authority, never
// shares this one.
func Handler(auth *Authority, opts ServerOptions) (http.Handler, error) {
	if strings.TrimSpace(opts.PodSecret) == "" {
		return nil, errors.New("maat: authority serve: a pod secret is required (ADR-120 §6); refusing to serve unauthenticated")
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = defaultCallTimeout
	}
	if opts.MaxInFlight <= 0 {
		opts.MaxInFlight = defaultMaxInFlight
	}
	if opts.MaxLeaseTTLSec <= 0 {
		opts.MaxLeaseTTLSec = defaultMaxLeaseTTLSec
	}
	s := &server{auth: auth, opts: opts, inflight: make(chan struct{}, opts.MaxInFlight)}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/v1/authority/reserve", s.withAuth(s.handleReserve))
	mux.HandleFunc("/v1/authority/renew", s.withAuth(s.handleRenew))
	mux.HandleFunc("/v1/authority/catchup", s.withAuth(s.handleCatchUp))
	return mux, nil
}

type server struct {
	auth     *Authority
	opts     ServerOptions
	inflight chan struct{}
}

// withAuth checks the pod secret, enforces the in-flight cap, and derives a
// bounded context before calling h — every route goes through all three.
func (s *server) withAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.opts.PodSecret)) != 1 {
			writeErr(w, http.StatusUnauthorized, "missing or invalid pod secret")
			return
		}
		select {
		case s.inflight <- struct{}{}:
			defer func() { <-s.inflight }()
		default:
			writeErr(w, http.StatusServiceUnavailable, "pod authority at capacity; retry")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.opts.CallTimeout)
		defer cancel()
		h(w, r.WithContext(ctx))
	}
}

func (s *server) handleReserve(w http.ResponseWriter, r *http.Request) {
	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request body: "+err.Error())
		return
	}
	if req.Reservation.LeaseTTLSec <= 0 || req.Reservation.LeaseTTLSec > s.opts.MaxLeaseTTLSec {
		req.Reservation.LeaseTTLSec = s.opts.MaxLeaseTTLSec
	}
	result, err := callBounded(r.Context(), func() (schedule.ReserveResult, error) {
		return s.auth.Reserve(req.Reservation, req.Queue)
	})
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *server) handleRenew(w http.ResponseWriter, r *http.Request) {
	if _, err := callBounded(r.Context(), func() (struct{}, error) { return struct{}{}, s.auth.Renew() }); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

func (s *server) handleCatchUp(w http.ResponseWriter, r *http.Request) {
	if _, err := callBounded(r.Context(), func() (struct{}, error) { return struct{}{}, s.auth.CatchUp() }); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct{}{})
}

// callBounded runs fn on its own goroutine and returns ctx.Err() if ctx ends
// first. fn itself keeps running to completion in the background — Reserve,
// Renew and CatchUp have no cancellation points of their own — so this
// bounds how long the HTTP caller waits, not the authority's own commit.
// ponytail: fn leaks until it finishes if ctx fires first; fine at this
// call's own CallTimeout scale (seconds), revisit if fn ever blocks on an
// unbounded external call.
func callBounded[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	ch := make(chan struct {
		v   T
		err error
	}, 1)
	go func() {
		v, err := fn()
		ch <- struct {
			v   T
			err error
		}{v, err}
	}()
	select {
	case o := <-ch:
		return o.v, o.err
	case <-ctx.Done():
		var zero T
		return zero, fmt.Errorf("maat: authority serve: %w", ctx.Err())
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, wireError{Error: msg})
}
