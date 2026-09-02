package routerstore

// ADR-062 §2 (rs-08): the router service exposes a Store over HTTP, one route
// per method, resolved by reflection against the Store interface so a method
// added to the interface is served without a hand-written route (and a method
// removed disappears from the wire in the same commit). remote.go documents
// the JSON contract.

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"
)

var (
	ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()
	errType = reflect.TypeOf((*error)(nil)).Elem()
	// storeType is the interface itself; reflection walks it, not the concrete
	// store, so unexported helpers on SQLiteStore are never reachable.
	storeType = reflect.TypeOf((*Store)(nil)).Elem()
)

// ServerOptions configure Handler.
type ServerOptions struct {
	// Token is the bearer token every request must present. Empty means the
	// handler refuses to serve at all — an unauthenticated ledger is not a
	// configuration, it is a bug (ADR-062 §3). Per-host tokens and sessions
	// arrive in rs-10/rs-11; this is the floor.
	Token string
	// MaxWait caps a Wait long-poll regardless of what the client asked for.
	MaxWait time.Duration
	// CallTimeout bounds every non-Wait call server-side.
	CallTimeout time.Duration
}

// Handler serves store over HTTP at /v1/call/{Method}.
func Handler(store Store, opts ServerOptions) (http.Handler, error) {
	if strings.TrimSpace(opts.Token) == "" {
		return nil, errors.New("routerstore: serve: a bearer token is required (ADR-062 §3); refusing to serve an unauthenticated ledger")
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = 60 * time.Second
	}
	if opts.CallTimeout <= 0 {
		opts.CallTimeout = 5 * time.Second
	}
	sv := reflect.ValueOf(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/v1/call/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, http.StatusMethodNotAllowed, "", "POST only")
			return
		}
		if !authorized(r, opts.Token) {
			writeErr(w, http.StatusUnauthorized, "", "missing or invalid bearer token")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/v1/call/")
		m, ok := storeType.MethodByName(name)
		if !ok {
			writeErr(w, http.StatusNotFound, "", "no such store method: "+name)
			return
		}
		var req wireRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20)).Decode(&req); err != nil && !errors.Is(err, errEmptyBody) {
			// An empty body is a zero-arg call.
			if r.ContentLength != 0 {
				writeErr(w, http.StatusBadRequest, "", "bad JSON: "+err.Error())
				return
			}
		}

		// Build the argument list. Interface method types have no receiver, so
		// NumIn is exactly the Go parameter count. A leading context.Context is
		// supplied from the request; everything else decodes from args in order.
		mt := m.Type
		in := make([]reflect.Value, 0, mt.NumIn())
		ai := 0
		timeout := opts.CallTimeout
		if name == "Wait" {
			timeout = opts.MaxWait
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		for i := 0; i < mt.NumIn(); i++ {
			pt := mt.In(i)
			if pt == ctxType {
				in = append(in, reflect.ValueOf(ctx))
				continue
			}
			if ai >= len(req.Args) {
				writeErr(w, http.StatusBadRequest, "", fmt.Sprintf("%s: expected %d args, got %d", name, mt.NumIn(), len(req.Args)))
				return
			}
			pv := reflect.New(pt)
			if err := json.Unmarshal(req.Args[ai], pv.Interface()); err != nil {
				writeErr(w, http.StatusBadRequest, "", fmt.Sprintf("%s: arg %d: %v", name, ai, err))
				return
			}
			in = append(in, pv.Elem())
			ai++
		}
		if name == "Wait" && len(in) == 3 {
			// Clamp the client's requested timeout to the server's ceiling.
			if d, ok := in[2].Interface().(time.Duration); ok && (d <= 0 || d > opts.MaxWait) {
				in[2] = reflect.ValueOf(opts.MaxWait)
			}
		}

		outs := sv.MethodByName(name).Call(in)

		// Last return is the error (if the method returns one).
		var callErr error
		results := outs
		if mt.NumOut() > 0 && mt.Out(mt.NumOut()-1) == errType {
			if e, _ := outs[len(outs)-1].Interface().(error); e != nil {
				callErr = e
			}
			results = outs[:len(outs)-1]
		}
		if callErr != nil {
			if sn := sentinelName(callErr); sn != "" {
				writeJSON(w, http.StatusOK, wireResponse{Error: &wireError{Name: sn, Message: callErr.Error()}})
				return
			}
			writeErr(w, http.StatusInternalServerError, "", callErr.Error())
			return
		}
		resp := wireResponse{Result: make([]json.RawMessage, 0, len(results))}
		for _, rv := range results {
			b, err := json.Marshal(rv.Interface())
			if err != nil {
				writeErr(w, http.StatusInternalServerError, "", "encode result: "+err.Error())
				return
			}
			resp.Result = append(resp.Result, b)
		}
		writeJSON(w, http.StatusOK, resp)
	})
	return mux, nil
}

var errEmptyBody = errors.New("empty body")

func authorized(r *http.Request, token string) bool {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if !strings.HasPrefix(h, p) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(h, p))
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, name, msg string) {
	writeJSON(w, code, wireResponse{Error: &wireError{Name: name, Message: msg}})
}
