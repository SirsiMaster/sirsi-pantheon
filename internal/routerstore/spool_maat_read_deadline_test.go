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
		{"expired-capacity-read", "RecipientLoad", time.Now().Add(-time.Minute), nil, false, 0, 503, false, false, false},
		{"capacity-read-offline-not-queued", "RecipientLoad", time.Now().Add(time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, true, 2, 503, false, false, true},
		{"capacity-read-post-send-unknown", "RecipientLoad", time.Now().Add(time.Minute), errors.New("response lost"), false, 1, 502, false, true, true},
		{"bounded-capacity-redial", "RecipientLoad", time.Now().Add(time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, false, 2, 200, false, false, true},
		{"legacy-capacity-read", "RecipientLoad", time.Time{}, nil, false, 1, 200, false, false, false},
		{"bounded-read-redial", "MaatRead", time.Now().Add(time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, false, 2, 200, false, false, true},
		{"read-offline-not-queued", "MaatRead", time.Now().Add(time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, true, 2, 503, false, false, true},
		{"read-post-send-unknown", "MaatRead", time.Now().Add(time.Minute), errors.New("response lost"), false, 1, 502, false, true, true},
		{"legacy-read", "MaatRead", time.Time{}, nil, false, 1, 200, false, false, false},
		{"mutation-ignores-read-deadline", "MaatMutate", time.Now().Add(-time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, false, 2, 200, false, false, false},
		{"mutation-offline-still-queued", "MaatMutate", time.Now().Add(-time.Minute), &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}, true, 2, 202, true, false, false},
		{"mutation-unknown", "MaatMutate", time.Now().Add(-time.Minute), errors.New("response lost"), false, 1, 502, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			read := spoolReadFixtureForMethod(tc.method)
			root, lane, id := t.TempDir(), "lane", "request"
			for _, dir := range []string{"req", "res", "inflight"} {
				if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
					t.Fatal(err)
				}
			}
			req := spoolRequest{Method: tc.method, Body: []byte(`{"args":[]}`)}
			if read.label != "" {
				req.Body = spoolReadFixtureBody(t, read)
			}
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
				if read.label != "" {
					checkSpoolReadFixtureRequest(t, r, read)
				}
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
	withSpoolReadFixture(t, func(t *testing.T, read spoolReadFixture) {
		for _, cutoff := range []struct {
			name string
			raw  json.RawMessage
		}{
			{"expired", json.RawMessage(strconv.FormatInt(time.Now().Add(-time.Minute).UnixNano(), 10))},
			{"absent", nil},
			{"malformed", json.RawMessage(`"legacy-extension"`)},
		} {
			t.Run(cutoff.name, func(t *testing.T) {
				root, lane, id := t.TempDir(), "lane", "001"
				for _, dir := range []string{"outbox", "inflight"} {
					if err := os.MkdirAll(filepath.Join(root, lane, dir), 0700); err != nil {
						t.Fatal(err)
					}
				}
				path := filepath.Join(root, lane, "outbox", id+".json")
				later := filepath.Join(root, lane, "outbox", "002.json")
				envelope := spoolRequest{Method: read.method, Body: spoolReadFixtureBody(t, read), ReadDeadlineUnixNano: cutoff.raw}
				for _, file := range []string{path, later} {
					if err := writeAtomic(file, envelope, 0600); err != nil {
						t.Fatal(err)
					}
				}
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				tail, err := os.ReadFile(later)
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(), Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					checkSpoolReadFixtureRequest(t, r, read)
					if _, ok := r.Context().Deadline(); ok {
						t.Error("already-owned outbox read acquired client cutoff")
					}
					calls++
					return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("offline")}
				})}}
				for wake := 1; wake <= 2; wake++ {
					if wake == 1 {
						if got := rl.retryOutboxOne(context.Background(), lane, path); got != outboxHeld {
							t.Fatalf("outcome=%v", got)
						}
					} else if got := rl.drainOutbox(context.Background(), lane); got != 2 {
						t.Fatalf("ordered frontier remaining=%d want=2", got)
					}
					if calls != 2*wake {
						t.Fatalf("wake=%d attempts=%d want=%d", wake, calls, 2*wake)
					}
					if held, err := os.ReadFile(path); err != nil || string(held) != string(original) {
						t.Fatalf("outbox lost or changed: %v", err)
					}
					if held, err := os.ReadFile(later); err != nil || string(held) != string(tail) {
						t.Fatalf("later ordered envelope lost or changed: %v", err)
					}
					for _, dir := range []string{"req", "res", "inflight", "failed"} {
						files, err := filepath.Glob(filepath.Join(root, lane, dir, "*.json"))
						if err != nil || len(files) != 0 {
							t.Fatalf("legacy wake=%d unexpected %s custody=%v err=%v", wake, dir, files, err)
						}
					}
				}
			})
		}
	})
}

func TestMaatReadShortCallerDeadlineDoesNotPublish(t *testing.T) {
	withSpoolReadFixture(t, func(t *testing.T, read spoolReadFixture) {
		root := t.TempDir()
		transport := newSpoolTransport(root, "lane")
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://spool/v1/call/"+read.method, strings.NewReader(string(spoolReadFixtureBody(t, read))))
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
	})
}

func TestReadDeadlineEnvelopeCanBeDecodedByLegacyShape(t *testing.T) {
	withSpoolReadFixture(t, func(t *testing.T, read spoolReadFixture) {
		req := spoolRequest{Method: read.method, Body: spoolReadFixtureBody(t, read), ReadDeadlineUnixNano: json.RawMessage(`123`)}
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
	})
}

// These causal controls use real RoundTrip publication, handleOne, the actual
// context deadline/second-attempt path, response publication and RemoteStore.do
// consumption. Only the network edge is in-process. synctest advances the real
// timer APIs virtually; there are no sleeps, sockets, service or subprocesses.
func startBudgetRead(t *testing.T, ctx context.Context, root string, read spoolReadFixture) (string, <-chan error, time.Time) {
	t.Helper()
	transport := newSpoolTransport(root, "budget-lane")
	rs := &RemoteStore{base: "http://spool", client: &http.Client{Transport: transport}}
	done := make(chan error, 1)
	go func() {
		if read.method == "RecipientLoad" {
			var result RecipientLoad
			err := rs.do(ctx, read.method, read.args, nil, &result)
			if err == nil && result != (RecipientLoad{Unacked: 3, Stalled: 4}) {
				err = errors.New("actual capacity result was not decoded")
			}
			done <- err
			return
		}
		var result []any
		done <- rs.do(ctx, read.method, read.args, nil, &result)
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
	if wire.Method != read.method {
		t.Fatalf("published method=%s want=%s", wire.Method, read.method)
	}
	checkSpoolReadFixtureArgs(t, wire.Body, read)
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
	withSpoolReadFixture(t, func(t *testing.T, read spoolReadFixture) {
		for _, name := range []string{"two_dial_errors", "expired_before_redial", "generic_deadline_unknown", "postsend_deadline_unknown", "denial", "redial_success"} {
			t.Run(name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					root := t.TempDir()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					request, done, wireDeadline := startBudgetRead(t, ctx, root, read)
					started := time.Now()
					wantDeadline := started.Add(spoolReadForwardBudget)
					if wireDeadline.Before(wantDeadline) {
						wantDeadline = wireDeadline
					}
					calls := 0
					var firstDeadline time.Time
					rl := &Relay{Spool: root, Base: "http://fixture.invalid", Token: "fixture", Log: discardLog(),
						Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
							checkSpoolReadFixtureRequest(t, r, read)
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
							if name == "redial_success" {
								if calls == 1 {
									return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("fixture unavailable")}
								}
								body := `{"result":[[]]}`
								if read.method == "RecipientLoad" {
									body = `{"result":[{"unacked":3,"stalled":4}]}`
								}
								return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
									Body: io.NopCloser(strings.NewReader(body))}, nil
							}
							if name == "denial" {
								return &http.Response{StatusCode: http.StatusForbidden, Header: make(http.Header),
									Body: io.NopCloser(strings.NewReader(`{"error":{"message":"denied; relay: ` + read.label + ` unavailable; service not reached and read not queued"}}`))}, nil
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
					success := name == "redial_success"
					if (err == nil) != success || ctx.Err() != nil || time.Since(started) > spoolReadForwardBudget+spoolPoll {
						t.Fatalf("missed caller budget: err=%v caller=%v elapsed=%v", err, ctx.Err(), time.Since(started))
					}
					wantCalls := 1
					if name == "two_dial_errors" || success {
						wantCalls = 2
					}
					if calls != wantCalls {
						t.Fatalf("attempts=%d want=%d", calls, wantCalls)
					}
					if success {
						checkBudgetReadCustody(t, root, false)
						return
					}
					unknown := strings.HasSuffix(name, "_unknown")
					unavailable := name == "two_dial_errors" || name == "expired_before_redial"
					if errors.Is(err, ErrServiceUnavailable) != unavailable || strings.Contains(err.Error(), "OUTCOME UNKNOWN") != unknown {
						t.Fatalf("deadline classification changed: %v", err)
					}
					if unavailable && err.Error() != "routerstore: router service unavailable (retry): relay: "+read.label+" unavailable; service not reached and read not queued" {
						t.Fatalf("running-supervisor sentinel changed: %v", err)
					}
					if name == "denial" && !strings.Contains(err.Error(), "HTTP 403") {
						t.Fatalf("denial lost precedence: %v", err)
					}
					checkBudgetReadCustody(t, root, unknown)
				})
			})
		}
	})
}

func TestMaatReadQueueDelayConsumesCallerDeadlineCausal(t *testing.T) {
	withSpoolReadFixture(t, func(t *testing.T, read spoolReadFixture) {
		synctest.Test(t, func(t *testing.T) {
			root := t.TempDir()
			// Earlier incoming context wins over the transport's default30 seconds.
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			request, done, cutoff := startBudgetRead(t, ctx, root, read)
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
			if err.Error() != "routerstore: router service unavailable (retry): relay: "+read.label+" unavailable; service not reached and read not queued" {
				t.Fatalf("queued-delay read label changed: %v", err)
			}
			checkBudgetReadCustody(t, root, false)
		})
	})
}

func TestMaatReadInvalidDeadlineDoesNotAttemptOrQueue(t *testing.T) {
	withSpoolReadFixture(t, func(t *testing.T, read spoolReadFixture) {
		for _, raw := range []string{`null`, `"later"`, `-1`, `0`, `9223372036854775808`, `1`} {
			t.Run(raw, func(t *testing.T) {
				root, lane, id := t.TempDir(), "budget-lane", "invalid"
				path := filepath.Join(root, lane, "req", id+".json")
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := writeAtomic(path, spoolRequest{Method: read.method, Body: spoolReadFixtureBody(t, read), ReadDeadlineUnixNano: json.RawMessage(raw)}, 0600); err != nil {
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
				var rendered wireResponse
				if err := json.Unmarshal(response.Body, &rendered); err != nil || rendered.Error == nil || rendered.Error.Message != "relay: "+read.label+" unavailable; service not reached and read not queued" {
					t.Fatalf("invalid-deadline read label changed: %s err=%v", response.Body, err)
				}
				for _, dir := range []string{"inflight", "outbox", "failed"} {
					files, err := filepath.Glob(filepath.Join(root, lane, dir, "*.json"))
					if err != nil || len(files) != 0 {
						t.Fatalf("unexpected %s custody=%v err=%v", dir, files, err)
					}
				}
			})
		}
	})
}
