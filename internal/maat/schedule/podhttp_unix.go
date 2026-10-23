//go:build darwin || linux

package schedule

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PodHTTPHandler authenticates existing native mTLS identities. The map binds
// exact leaf certificate fingerprints to registered holders; caller JSON never
// chooses its principal. No router bearer credential is copied to this service.
func PodHTTPHandler(j *PodJournal, nativeHolders map[string]string) (http.Handler, error) {
	if j == nil || len(nativeHolders) == 0 {
		return nil, fmt.Errorf("maat: native journal and admitted client identities required")
	}
	holders := map[string]string{}
	for k, v := range nativeHolders {
		if len(k) != 64 || strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("maat: invalid native client identity")
		}
		holders[k] = v
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/maat/pod" {
			http.Error(w, "unsupported pod operation", http.StatusNotFound)
			return
		}
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "native mutual TLS required", http.StatusForbidden)
			return
		}
		fingerprint := sha256.Sum256(r.TLS.PeerCertificates[0].Raw)
		holder := holders[hex.EncodeToString(fingerprint[:])]
		if holder == "" {
			http.Error(w, "native identity not admitted", http.StatusForbidden)
			return
		}
		var req podRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		dec.DisallowUnknownFields()
		if e := dec.Decode(&req); e != nil {
			http.Error(w, "invalid bounded request", http.StatusBadRequest)
			return
		}
		if dec.Decode(&struct{}{}) != io.EOF {
			http.Error(w, "trailing request data", http.StatusBadRequest)
			return
		}
		if req.Pod != j.grant.Pod || req.Epoch != j.grant.Epoch {
			http.Error(w, "authority epoch mismatch", http.StatusConflict)
			return
		}
		var out any
		var err error
		if req.Action == "reserve" {
			if req.Reservation.Holder != holder || req.Reservation.ID == "" {
				http.Error(w, "own holder and stable request ID required", http.StatusForbidden)
				return
			}
			out, err = j.Reserve(req.Reservation, req.Queue)
		} else {
			out, err = j.Mutate(holder, req.Action, req.ID, req.Value)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			return
		}
	}), nil
}

type podRequest struct {
	Pod         string      `json:"pod"`
	Epoch       int         `json:"epoch"`
	Action      string      `json:"action"`
	Reservation Reservation `json:"reservation"`
	Queue       bool        `json:"queue"`
	ID          string      `json:"id,omitempty"`
	Value       string      `json:"value,omitempty"`
}

// NativePodServer serves only with verified client certificates and bounded
// request handling. The native owner supplies its existing approved identities.
func NativePodServer(address string, config *tls.Config, handler http.Handler) (*http.Server, error) {
	if config == nil || config.ClientCAs == nil || config.ClientAuth != tls.RequireAndVerifyClientCert || len(config.Certificates) == 0 || handler == nil {
		return nil, fmt.Errorf("maat: approved mutual TLS configuration required")
	}
	c := config.Clone()
	c.MinVersion = tls.VersionTLS13
	return &http.Server{Addr: address, Handler: handler, TLSConfig: c, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192}, nil
}

// HTTPPodTransport is the actual authenticated native client. It accepts one
// exact delegation address and never retries a mutation or falls back to cloud.
type HTTPPodTransport struct {
	client *http.Client
	base   string
}

func NewHTTPPodTransport(address string, config *tls.Config) (*HTTPPodTransport, error) {
	if !strings.HasPrefix(address, "https://") || config == nil || config.InsecureSkipVerify || config.RootCAs == nil || len(config.Certificates) == 0 {
		return nil, fmt.Errorf("maat: verified native mutual TLS endpoint required")
	}
	c := config.Clone()
	c.MinVersion = tls.VersionTLS13
	return &HTTPPodTransport{base: strings.TrimRight(address, "/"), client: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: c}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (t *HTTPPodTransport) call(ctx context.Context, d Delegation, req podRequest, out any) error {
	allowed := false
	for _, a := range d.AuthorityAddrs {
		if strings.TrimRight(a, "/") == t.base {
			allowed = true
		}
	}
	if !allowed || d.expired(time.Now()) {
		return fmt.Errorf("maat: finite native endpoint not delegated")
	}
	req.Pod = d.Pod
	req.Epoch = d.Epoch
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, t.base+"/v1/maat/pod", bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	response, err := t.client.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("maat: native authority refused HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
}
func (t *HTTPPodTransport) ReservePod(ctx context.Context, d Delegation, req Reservation, queue bool) (ReserveResult, error) {
	var out ReserveResult
	err := t.call(ctx, d, podRequest{Action: "reserve", Reservation: req, Queue: queue}, &out)
	return out, err
}
func (t *HTTPPodTransport) MutatePod(ctx context.Context, d Delegation, action, id, value string) (*Reservation, error) {
	var out Reservation
	err := t.call(ctx, d, podRequest{Action: action, ID: id, Value: value}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
