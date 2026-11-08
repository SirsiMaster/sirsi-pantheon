package routerstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The relay fixture uses only a private spool and an in-process failing edge.
// It never contacts the canonical service or changes any real reservation.
func TestMaatReadNeverReachedIsNotQueued(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		err          error
		status       int
		outbox       bool
		failed       bool
	}{
		{"read_never_reached", "MaatRead", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, http.StatusServiceUnavailable, false, false},
		{"mutation_never_reached", "MaatMutate", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, http.StatusAccepted, true, false},
		{"read_post_send_unknown", "MaatRead", errors.New("response lost after send"), http.StatusBadGateway, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, lane, id := t.TempDir(), "fixture-lane", "fixture-1"
			for _, dir := range []string{"req", "res", "inflight"} {
				if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			request := filepath.Join(root, lane, "req", id+".json")
			if err := writeAtomic(request, spoolRequest{Method: tc.method, Body: []byte(`{"args":[]}`)}, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
				Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, tc.err })}}
			rl.handleOne(context.Background(), lane, request)
			body, err := os.ReadFile(filepath.Join(root, lane, "res", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var response spoolResponse
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatal(err)
			}
			if response.Status != tc.status {
				t.Fatalf("status=%d body=%s", response.Status, response.Body)
			}
			if tc.status == http.StatusServiceUnavailable && !strings.Contains(string(response.Body), "read not queued") {
				t.Fatalf("missing clear refusal: %s", response.Body)
			}
			for dir, want := range map[string]bool{"outbox": tc.outbox, "failed": tc.failed} {
				_, err := os.Stat(filepath.Join(root, lane, dir, id+".json"))
				if (err == nil) != want {
					t.Fatalf("%s custody present=%v want=%v err=%v", dir, err == nil, want, err)
				}
			}
			if tc.err != nil && tc.status != http.StatusBadGateway && calls != 2 {
				t.Fatalf("safe pre-send redial count=%d", calls)
			}
		})
	}
}

func TestLegacyQueuedMaatReadRemainsHeldOnOutage(t *testing.T) {
	root, lane, id := t.TempDir(), "fixture-lane", "fixture-old"
	for _, dir := range []string{"outbox", "inflight"} {
		if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, lane, "outbox", id+".json")
	if err := writeAtomic(path, spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
		t.Fatal(err)
	}
	rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		})}}
	if got := rl.retryOutboxOne(context.Background(), lane, path); got != outboxHeld {
		t.Fatalf("outcome=%v, want held", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original held request lost: %v", err)
	}
}

// Legacy queued reads retain their old ambiguous-error behavior on both wakes.
// This demonstrates compatibility, not that replay of such errors is safe.
func TestLegacyQueuedMaatReadAmbiguousErrorStaysHeldOnSecondWake(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"read_phase_nested_dns", &net.OpError{Op: "read", Net: "tcp", Err: &net.DNSError{Err: "fixture", Name: "fixture.invalid"}}},
		{"joined_dial_and_unknown", errors.Join(&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("fixture unreachable")}, errors.New("other outcome unknown"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !neverReachedService(tc.err) || maatReadNeverReached(tc.err) {
				t.Fatal("fixture does not demonstrate the legacy-to-conservative classification change")
			}
			root, lane := t.TempDir(), "fixture-lane"
			for _, dir := range []string{"outbox", "inflight"} {
				if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			first := filepath.Join(root, lane, "outbox", "001.json")
			later := filepath.Join(root, lane, "outbox", "002.json")
			for _, path := range []string{first, later} {
				if err := writeAtomic(path, spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
					t.Fatal(err)
				}
			}
			original, err := os.ReadFile(first)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			rl := &Relay{Spool: root, Base: "http://fixture.invalid", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
				Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, tc.err
				})}}
			for wake := 1; wake <= 2; wake++ {
				if got := rl.drainOutbox(context.Background(), lane); got != 2 || calls != 2*wake {
					t.Fatalf("wake=%d remaining=%d calls=%d", wake, got, calls)
				}
				held, err := os.ReadFile(first)
				if err != nil || !bytes.Equal(held, original) {
					t.Fatalf("wake=%d original held envelope changed: %v", wake, err)
				}
				if _, err := os.Stat(later); err != nil {
					t.Fatalf("wake=%d later entry lost: %v", wake, err)
				}
				if _, err := os.Stat(filepath.Join(root, lane, "failed", "001.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("wake=%d legacy read unexpectedly parked: %v", wake, err)
				}
			}
		})
	}
}

// Characterizes the unchanged mutation-custody defect; this is not an
// approval to discard rejected mutations or a release qualification.
func TestReadRepairDoesNotChangeQueuedMutationRefusalBehavior(t *testing.T) {
	root, lane, id := t.TempDir(), "fixture-lane", "fixture-mutation"
	for _, dir := range []string{"outbox", "inflight"} {
		if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, lane, "outbox", id+".json")
	if err := writeAtomic(path, spoolRequest{Method: "MaatMutate", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
		t.Fatal(err)
	}
	router := &Relay{Spool: root, Base: "http://fixture.invalid", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header),
				Body: io.NopCloser(strings.NewReader(`{"error":{"message":"fixture refused"}}`))}, nil
		})}}
	if got := router.retryOutboxOne(context.Background(), lane, path); got != outboxDelivered {
		t.Fatalf("legacy mutation response behavior changed: %v", got)
	}
	if _, err := os.Stat(filepath.Join(root, lane, "failed", id+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected known missing rejected-mutation custody, got %v", err)
	}
}

func TestMaatReadDialThenUnknownKeepsOutcomeCustody(t *testing.T) {
	root, lane, id := t.TempDir(), "fixture-lane", "fixture-mixed"
	for _, dir := range []string{"req", "res", "inflight"} {
		if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, lane, "req", id+".json")
	if err := writeAtomic(path, spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
			}
			return nil, errors.New("response lost after request left")
		})}}
	rl.handleOne(context.Background(), lane, path)
	if calls != 2 {
		t.Fatalf("attempts=%d", calls)
	}
	if _, err := os.Stat(filepath.Join(root, lane, "failed", id+".json")); err != nil {
		t.Fatalf("unknown outcome not retained: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, lane, "res", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var response spoolResponse
	if err := json.Unmarshal(data, &response); err != nil || response.Status != http.StatusBadGateway || !strings.Contains(string(response.Body), "OUTCOME UNKNOWN") {
		t.Fatalf("response=%s err=%v", data, err)
	}
}

func TestMaatReadAmbiguousWrappedErrorsRemainUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"read_phase_nested_dns", &net.OpError{Op: "read", Net: "tcp", Err: &net.DNSError{Err: "fixture", Name: "fixture.invalid"}}},
		{"joined_dial_and_unknown", errors.Join(&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("fixture unreachable")}, errors.New("other outcome unknown"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, lane, id := t.TempDir(), "fixture-lane", "fixture-ambiguous"
			for _, dir := range []string{"req", "res", "inflight"} {
				if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(root, lane, "req", id+".json")
			if err := writeAtomic(path, spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			router := &Relay{Spool: root, Base: "http://fixture.invalid", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
				Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, tc.err
				})}}
			router.handleOne(context.Background(), lane, path)
			if calls != 1 {
				t.Fatalf("ambiguous first attempt was re-forwarded: %d", calls)
			}
			if _, err := os.Stat(filepath.Join(root, lane, "failed", id+".json")); err != nil {
				t.Fatalf("unknown outcome custody missing: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(root, lane, "res", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var response spoolResponse
			if err := json.Unmarshal(data, &response); err != nil || response.Status != http.StatusBadGateway || !strings.Contains(string(response.Body), "OUTCOME UNKNOWN") {
				t.Fatalf("ambiguous response=%s, err=%v", data, err)
			}
		})
	}
}

func TestUnsentMaatReadResponseRemoteErrorContract(t *testing.T) {
	message := "relay: Ma'at read unavailable; service not reached and read not queued"
	body, err := json.Marshal(wireResponse{Error: &wireError{Name: "relay", Message: message}})
	if err != nil {
		t.Fatal(err)
	}
	rs := &RemoteStore{base: "http://fixture.invalid", client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}}
	var result []any
	err = rs.do(context.Background(), "MaatRead", []any{"reservation", "fixture"}, nil, &result)
	if !errors.Is(err, ErrServiceUnavailable) || err.Error() != "routerstore: router service unavailable (retry): "+message {
		t.Fatalf("unexpected rendered error: %v", err)
	}
}

// Exercise the real file transport and response consumption without starting a
// relay service or opening a network socket. The older outbox entry is outside
// this request's custody and must remain byte-for-byte unchanged.
func TestMaatReadPrivateSpoolRemoteErrorEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name        string
		edge        func(*http.Request) (*http.Response, error)
		want        string
		unavailable bool
		notOwner    bool
		calls       int
	}{
		{
			name: "never_reached",
			edge: func(*http.Request) (*http.Response, error) {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("fixture unreachable")}
			},
			want:        "routerstore: router service unavailable (retry): relay: Ma'at read unavailable; service not reached and read not queued",
			unavailable: true,
			calls:       2,
		},
		{
			name: "service_forbidden",
			edge: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"error":{"name":"fixture_denied","message":"scope denied"}}`))}, nil
			},
			want:  "routerstore: remote MaatRead: HTTP 403: scope denied",
			calls: 1,
		},
		{
			name: "service_forbidden_sentinel",
			edge: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header),
					Body: io.NopCloser(strings.NewReader(`{"error":{"name":"ErrNotOwner","message":"fixture denied"}}`))}, nil
			},
			want:     ErrNotOwner.Error(),
			notOwner: true,
			calls:    1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, lane := t.TempDir(), "fixture-lane"
			outbox := filepath.Join(root, lane, "outbox")
			if err := os.MkdirAll(outbox, 0700); err != nil {
				t.Fatal(err)
			}
			legacy := filepath.Join(outbox, "legacy.json")
			if err := writeAtomic(legacy, spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(legacy)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
				Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return tc.edge(r)
				})}}
			transport := newSpoolTransport(root, lane)
			transport.wait = 5 * time.Second
			rs := &RemoteStore{base: "http://spool", client: &http.Client{Transport: transport}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				var result []any
				finished <- rs.do(ctx, "MaatRead", []any{"reservation", "fixture"}, nil, &result)
			}()
			var request string
			for request == "" {
				files, err := filepath.Glob(filepath.Join(root, lane, "req", "*.json"))
				if err != nil {
					t.Fatal(err)
				}
				if len(files) == 1 {
					request = files[0]
					break
				}
				if len(files) > 1 {
					t.Fatalf("unexpected request files: %v", files)
				}
				select {
				case err := <-finished:
					t.Fatalf("caller returned before relay handling: %v", err)
				case <-ctx.Done():
					t.Fatal("timed out waiting for private spool request")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if !rl.handleOne(ctx, lane, request) {
				t.Fatal("relay failed to settle the new request")
			}
			select {
			case err := <-finished:
				if err == nil || err.Error() != tc.want || errors.Is(err, ErrServiceUnavailable) != tc.unavailable || errors.Is(err, ErrNotOwner) != tc.notOwner {
					t.Fatalf("remote error=%v; want %q, unavailable=%v", err, tc.want, tc.unavailable)
				}
			case <-ctx.Done():
				t.Fatal("timed out waiting for remote response")
			}
			if calls != tc.calls {
				t.Fatalf("edge calls=%d, want %d", calls, tc.calls)
			}
			after, err := os.ReadFile(legacy)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("legacy outbox changed: err=%v", err)
			}
			for _, dir := range []string{"req", "res", "inflight", "failed"} {
				files, err := filepath.Glob(filepath.Join(root, lane, dir, "*.json"))
				if err != nil || len(files) != 0 {
					t.Fatalf("unexpected %s custody: %v, err=%v", dir, files, err)
				}
			}
			files, err := filepath.Glob(filepath.Join(outbox, "*.json"))
			if err != nil || len(files) != 1 || files[0] != legacy {
				t.Fatalf("unexpected outbox custody: %v, err=%v", files, err)
			}
		})
	}
}

func TestUnsentMaatReadResponsePublicationFailureRetainsCustody(t *testing.T) {
	root, lane, id := t.TempDir(), "fixture-lane", "fixture-publish"
	for _, dir := range []string{"req", "inflight"} {
		if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, lane, "res"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, lane, "req", id+".json")
	if err := writeAtomic(path, spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`)}, 0600); err != nil {
		t.Fatal(err)
	}
	rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		})}}
	if rl.handleOne(context.Background(), lane, path) {
		t.Fatal("failed publication reported settled")
	}
	if !rl.settlementFailed.Load() {
		t.Fatal("publication failure did not retain settlement debt")
	}
	if _, err := os.Stat(filepath.Join(root, lane, "inflight", id+".json")); err != nil {
		t.Fatalf("request custody lost: %v", err)
	}
}
