package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// These controls use private files and an in-process transport only.
func TestMaatReadDeadlineAndMutationIsolation(t *testing.T) {
	for _, tc := range []struct {
		name, method                         string
		deadline                             time.Time
		firstErr                             error
		alwaysFail                           bool
		wantCalls, wantStatus                int
		wantOutbox, wantFailed, wantDeadline bool
	}{
		{"expired-read", "MaatRead", time.Now().Add(-time.Minute), nil, false, 0, 503, false, false, false},
		{"bounded-read-redial", "MaatRead", time.Now().Add(time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, false, 2, 200, false, false, true},
		{"read-offline-not-queued", "MaatRead", time.Now().Add(time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, true, 2, 503, false, false, true},
		{"read-post-send-unknown", "MaatRead", time.Now().Add(time.Minute), errors.New("response lost"), false, 1, 502, false, true, true},
		{"legacy-read", "MaatRead", time.Time{}, nil, false, 1, 200, false, false, false},
		{"mutation-ignores-read-deadline", "MaatMutate", time.Now().Add(-time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, false, 2, 200, false, false, false},
		{"mutation-offline-still-queued", "MaatMutate", time.Now().Add(-time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, true, 2, 202, true, false, false},
		{"mutation-unknown", "MaatMutate", time.Now().Add(-time.Minute), errors.New("response lost"), false, 1, 502, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, lane, id := t.TempDir(), "lane", "request"
			for _, dir := range []string{"req", "res", "inflight"} {
				if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			req := spoolRequest{Method: tc.method, Body: []byte(`{"args":[]}`)}
			if !tc.deadline.IsZero() {
				req.ReadDeadlineUnixNano = json.RawMessage(strconv.FormatInt(tc.deadline.UnixNano(), 10))
			}
			if tc.name == "mutation-unknown" {
				// A formerly ignored extension of the wrong type must not reject a mutation.
				req.ReadDeadlineUnixNano = json.RawMessage(`"legacy-extension"`)
			}
			path := filepath.Join(root, lane, "req", id+".json")
			if err := writeAtomic(path, req, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			var firstDeadline time.Time
			rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(), Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				if ok != tc.wantDeadline {
					t.Errorf("deadline present=%v want=%v", ok, tc.wantDeadline)
				}
				if ok && calls == 1 {
					firstDeadline = deadline
				}
				if ok && calls == 2 && !deadline.Equal(firstDeadline) {
					t.Error("redial extended shared deadline")
				}
				if (calls == 1 || tc.alwaysFail) && tc.firstErr != nil {
					return nil, tc.firstErr
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"result":[]}`))}, nil
			})}}
			rl.handleOne(context.Background(), lane, path)
			data, err := os.ReadFile(filepath.Join(root, lane, "res", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var response spoolResponse
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if calls != tc.wantCalls || response.Status != tc.wantStatus {
				t.Fatalf("calls=%d status=%d body=%s", calls, response.Status, response.Body)
			}
			for dir, want := range map[string]bool{"outbox": tc.wantOutbox, "failed": tc.wantFailed} {
				_, err := os.Stat(filepath.Join(root, lane, dir, id+".json"))
				if (err == nil) != want {
					t.Fatalf("%s present=%v want=%v err=%v", dir, err == nil, want, err)
				}
			}
		})
	}
}

func TestMaatReadOutboxDoesNotApplyNewDeadline(t *testing.T) {
	root, lane, id := t.TempDir(), "lane", "old"
	for _, dir := range []string{"outbox", "inflight"} {
		if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, lane, "outbox", id+".json")
	if err := writeAtomic(path, spoolRequest{Method: "MaatRead", ReadDeadlineUnixNano: json.RawMessage(strconv.FormatInt(time.Now().Add(-time.Minute).UnixNano(), 10))}, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(), Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}
	})}}
	if got := rl.retryOutboxOne(context.Background(), lane, path); got != outboxHeld {
		t.Fatalf("outcome=%v", got)
	}
	if calls != 2 {
		t.Fatalf("attempts=%d", calls)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("outbox lost: %v", err)
	}
}

func TestMaatReadShortCallerDeadlineDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	transport := newSpoolTransport(root, "lane")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://spool/v1/call/MaatRead", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "request not published") {
		t.Fatalf("unexpected error: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(root, "lane", "req", "*.json"))
	if err != nil || len(files) != 0 {
		t.Fatalf("published=%v err=%v", files, err)
	}
}

func TestReadDeadlineEnvelopeCanBeDecodedByLegacyShape(t *testing.T) {
	req := spoolRequest{Method: "MaatRead", Body: []byte(`{"args":[]}`), ReadDeadlineUnixNano: json.RawMessage(`123`)}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    []byte            `json:"body"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Method != req.Method || string(legacy.Body) != string(req.Body) {
		t.Fatalf("legacy decode changed request: %+v", legacy)
	}
}

// These causal controls use real RoundTrip publication, handleOne, the actual
// context deadline/second-attempt path, response publication and RemoteStore.do
// consumption. Only the network edge is in-process. synctest advances the real
// timer APIs virtually; there are no sleeps, sockets, service or subprocesses.
func startBudgetRead(t *testing.T, ctx context.Context, root string) (string, <-chan error, time.Time) {
	t.Helper()
	transport := newSpoolTransport(root, "budget-lane")
	rs := &RemoteStore{base: "http://spool", client: &http.Client{Transport: transport}}
	done := make(chan error, 1)
	go func() {
		var result []any
		done <- rs.do(ctx, "MaatRead", []any{"reservation", "fixture"}, nil, &result)
	}()
	synctest.Wait() // caller has published and is blocked on its response poll
	files, err := filepath.Glob(filepath.Join(root, "budget-lane", "req", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("actual published requests=%v err=%v", files, err)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var wire spoolRequest
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	var nanos int64
	if err := json.Unmarshal(wire.ReadDeadlineUnixNano, &nanos); err != nil {
		t.Fatalf("actual caller omitted its read deadline: %v", err)
	}
	deadline := time.Unix(0, nanos)
	callerDeadline, _ := ctx.Deadline()
	if !deadline.Equal(callerDeadline.Add(-spoolReadSettlementReserve)) {
		t.Fatalf("published cutoff=%v caller=%v reserve=%v", deadline, callerDeadline, spoolReadSettlementReserve)
	}
	return files[0], done, deadline
}

func checkBudgetReadCustody(t *testing.T, root string, failed bool) {
	t.Helper()
	for _, dir := range []string{"req", "res", "inflight", "outbox", "failed"} {
		files, err := filepath.Glob(filepath.Join(root, "budget-lane", dir, "*.json"))
		want := 0
		if dir == "failed" && failed {
			want = 1
		}
		if err != nil || len(files) != want {
			t.Fatalf("%s custody=%v want=%d err=%v", dir, files, want, err)
		}
	}
}

func TestMaatReadSharedDeadlineCausal(t *testing.T) {
	for _, name := range []string{"two_dial_errors", "expired_before_redial", "generic_deadline_unknown", "postsend_deadline_unknown", "denial"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root := t.TempDir()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				request, done, wireDeadline := startBudgetRead(t, ctx, root)
				started := time.Now()
				wantDeadline := started.Add(spoolReadForwardBudget)
				if wireDeadline.Before(wantDeadline) {
					wantDeadline = wireDeadline
				}
				calls := 0
				var firstDeadline time.Time
				rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(),
					Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
						calls++
						deadline, ok := r.Context().Deadline()
						if !ok || !deadline.Equal(wantDeadline) {
							t.Errorf("attempt=%d actual deadline=%v want shared=%v", calls, deadline, wantDeadline)
							return nil, errors.New("missing or extended actual read context")
						}
						if calls == 1 {
							firstDeadline = deadline
						} else if !deadline.Equal(firstDeadline) {
							t.Error("second attempt reset the read budget")
						}
						if name == "denial" {
							return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header),
								Body: io.NopCloser(strings.NewReader(`{"error":{"message":"denied; relay: Ma'at read unavailable; service not reached and read not queued"}}`))}, nil
						}
						if name == "two_dial_errors" && calls == 1 {
							// First dial uses all but one second of the SAME cutoff.
							first, stop := context.WithDeadline(r.Context(), deadline.Add(-time.Second))
							<-first.Done()
							stop()
							return nil, &net.OpError{Op: "dial", Net: "tcp", Err: first.Err()}
						}
						<-r.Context().Done() // actual production cutoff, not a fabricated error/time snapshot
						if name == "generic_deadline_unknown" {
							return nil, r.Context().Err()
						}
						phase := "dial"
						if name == "postsend_deadline_unknown" {
							phase = "read"
						}
						return nil, &net.OpError{Op: phase, Net: "tcp", Err: r.Context().Err()}
					})}}
				rl.handleOne(context.Background(), "budget-lane", request)
				err := <-done // includes actual response-file poll and body consumption
				if err == nil || ctx.Err() != nil || time.Since(started) > spoolReadForwardBudget+spoolPoll {
					t.Fatalf("missed caller budget: err=%v caller=%v elapsed=%v", err, ctx.Err(), time.Since(started))
				}
				wantCalls := 1
				if name == "two_dial_errors" {
					wantCalls = 2
				}
				if calls != wantCalls {
					t.Fatalf("attempts=%d want=%d", calls, wantCalls)
				}
				unknown := strings.HasSuffix(name, "_unknown")
				unavailable := name == "two_dial_errors" || name == "expired_before_redial"
				if errors.Is(err, ErrServiceUnavailable) != unavailable || strings.Contains(err.Error(), "OUTCOME UNKNOWN") != unknown {
					t.Fatalf("deadline classification changed: %v", err)
				}
				if unavailable && err.Error() != "routerstore: router service unavailable (retry): relay: Ma'at read unavailable; service not reached and read not queued" {
					t.Fatalf("running-supervisor sentinel changed: %v", err)
				}
				if name == "denial" && !strings.Contains(err.Error(), "HTTP 403") {
					t.Fatalf("denial lost precedence: %v", err)
				}
				checkBudgetReadCustody(t, root, unknown)
			})
		})
	}
}

func TestMaatReadQueueDelayConsumesCallerDeadlineCausal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root := t.TempDir()
		// Earlier incoming context wins over the transport's default30 seconds.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		request, done, cutoff := startBudgetRead(t, ctx, root)
		queued, stop := context.WithDeadline(context.Background(), cutoff)
		<-queued.Done() // virtual lane wait until the forwarding cutoff, still BEFORE caller expiry
		stop()
		calls := 0
		rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(),
			Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("expired queued read must never reach the edge")
			})}}
		rl.handleOne(context.Background(), "budget-lane", request)
		err := <-done
		if calls != 0 || ctx.Err() != nil || !errors.Is(err, ErrServiceUnavailable) {
			t.Fatalf("queue delay was not charged: calls=%d caller=%v err=%v", calls, ctx.Err(), err)
		}
		checkBudgetReadCustody(t, root, false)
	})
}

func TestMaatReadInvalidDeadlineDoesNotAttemptOrQueue(t *testing.T) {
	for _, raw := range []string{`null`, `"later"`, `-1`, `0`, `9223372036854775808`, `1`} {
		t.Run(raw, func(t *testing.T) {
			root, lane, id := t.TempDir(), "budget-lane", "invalid"
			path := filepath.Join(root, lane, "req", id+".json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := writeAtomic(path, spoolRequest{Method: "MaatRead", ReadDeadlineUnixNano: json.RawMessage(raw)}, 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(),
				Client: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					calls++
					return nil, errors.New("invalid read deadline must not reach the edge")
				})}}
			rl.handleOne(context.Background(), lane, path)
			data, err := os.ReadFile(filepath.Join(root, lane, "res", id+".json"))
			var response spoolResponse
			if err != nil || json.Unmarshal(data, &response) != nil || calls != 0 || response.Status != http.StatusServiceUnavailable {
				t.Fatalf("invalid deadline forwarded or queued: calls=%d response=%s err=%v", calls, data, err)
			}
			for _, dir := range []string{"inflight", "outbox", "failed"} {
				files, err := filepath.Glob(filepath.Join(root, lane, dir, "*.json"))
				if err != nil || len(files) != 0 {
					t.Fatalf("unexpected %s custody=%v err=%v", dir, files, err)
				}
			}
		})
	}
}
