package apollo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testJob(state string, fences []RankFence) Job {
	return Job{
		Contract: ClusterContractV1, JobID: "job-1", ExecutionID: "0123456789abcdef0123456789abcdef",
		RequestID: "request-1", PlanSHA256: testDigest, Epoch: 9, State: state, RankFences: fences,
	}
}

func testFence(rank int) RankFence {
	return RankFence{Rank: rank, HostID: "m5", Fence: "fence-1", Generation: "generation-1", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 1 << 30}
}

func testRequest() JobRequest {
	return JobRequest{
		Contract: ClusterContractV1, RequestID: "request-1", IdempotencyKey: "idem-1", SessionID: "session-1",
		PlanSHA256: testDigest, ModelID: "gemma", CheckpointSHA256: testDigest, TokenizerSHA256: testDigest,
		TemplateSHA256: testDigest, Mode: "plain", Precision: "Q8", InputTokens: []uint32{2, 105}, MaxOutputTokens: 32,
	}
}

func TestClusterClientRejectsHTTPOutsideTest(t *testing.T) {
	if _, err := NewClusterClient("http://example.invalid", &http.Client{}); err == nil {
		t.Fatal("accepted unencrypted endpoint")
	}
	if _, err := NewClusterClient("https://example.invalid?redirect=elsewhere", &http.Client{}); err == nil {
		t.Fatal("accepted endpoint query")
	}
}

func TestCreateJobRequiresLiveAllRankFences(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain"}})
	})
	mux.HandleFunc("/v1/apollo/jobs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testJob("admitting", nil))
	})
	client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Result(), nil
	})}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testRequest(), []RankPlacement{{Rank: 0, HostID: "m5"}, {Rank: 1, HostID: "m1"}}); err == nil || !strings.Contains(err.Error(), "all-rank") {
		t.Fatalf("CreateJob error = %v, want all-rank admission rejection", err)
	}
}

func TestCreateJobBindsFrozenRequestAndRankFences(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain", "mtp"}})
	})
	mux.HandleFunc("/v1/apollo/jobs", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("client invented authorization header %q", got)
		}
		var request JobRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.SessionID != "session-1" || request.PlanSHA256 != testDigest || len(request.InputTokens) != 2 {
			t.Fatalf("frozen request = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(testJob("admitting", []RankFence{testFence(0), {Rank: 1, HostID: "m1", Fence: "fence-2", Generation: "generation-2", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 2 << 30}}))
	})
	client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Result(), nil
	})}, true)
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.CreateJob(context.Background(), testRequest(), []RankPlacement{{Rank: 0, HostID: "m5"}, {Rank: 1, HostID: "m1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(job.RankFences) != 2 || job.RankFences[1].HostID != "m1" {
		t.Fatalf("rank fences = %#v", job.RankFences)
	}
}

func TestCreateJobRejectsPlannedCapability(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, Modes: []string{"plain"}})
	})
	client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w.Result(), nil
	})}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testRequest(), []RankPlacement{{Rank: 0, HostID: "m5"}, {Rank: 1, HostID: "m1"}}); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("CreateJob error = %v, want planned service rejection", err)
	}
}

func TestEventsBindFrozenTupleAndPreserveReply(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Event)
		reject bool
	}{
		{"reply", func(*Event) {}, false},
		{"execution", func(e *Event) { e.ExecutionID = "other" }, true},
		{"empty-execution", func(e *Event) { e.ExecutionID = "" }, true},
		{"request", func(e *Event) { e.RequestID = "other" }, true},
		{"empty-request", func(e *Event) { e.RequestID = "" }, true},
		{"plan", func(e *Event) { e.PlanSHA256 = strings.Repeat("b", 64) }, true},
		{"empty-plan", func(e *Event) { e.PlanSHA256 = "" }, true},
		{"epoch", func(e *Event) { e.Epoch++ }, true},
		{"zero-epoch", func(e *Event) { e.Epoch = 0 }, true},
		{"job", func(e *Event) { e.JobID = "other" }, true},
		{"replayed-sequence", func(e *Event) { e.Sequence = 0 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			admitted := testJob("decoding", nil)
			text := "hello"
			token := uint32(0)
			position := uint64(0)
			committed := uint64(1)
			event := Event{Contract: ClusterContractV1, JobID: admitted.JobID, ExecutionID: admitted.ExecutionID, RequestID: admitted.RequestID, PlanSHA256: admitted.PlanSHA256, Epoch: admitted.Epoch, Sequence: 1, Type: "token", Text: &text, TokenID: &token, Position: &position, CommittedTokens: &committed}
			tc.change(&event)
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			terminal := Event{Contract: admitted.Contract, JobID: admitted.JobID, ExecutionID: admitted.ExecutionID, RequestID: admitted.RequestID, PlanSHA256: admitted.PlanSHA256, Epoch: admitted.Epoch, Sequence: 2, Type: "released"}
			done, err := json.Marshal(terminal)
			if err != nil {
				t.Fatal(err)
			}
			client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: " + string(data) + "\n\ndata: " + string(done) + "\n\n"))}, nil
			})}, true)
			if err != nil {
				t.Fatal(err)
			}
			seen := 0
			err = client.Events(context.Background(), admitted, 0, func(e Event) error {
				if e.Type != "token" {
					return nil
				}
				seen++
				encoded, err := json.Marshal(e)
				if err != nil {
					return err
				}
				for _, field := range []string{`"text":"hello"`, `"token_id":0`, `"position":0`, `"committed_tokens":1`} {
					if !strings.Contains(string(encoded), field) {
						t.Errorf("missing %s: %s", field, encoded)
					}
				}
				return nil
			})
			if tc.reject {
				if err == nil || seen != 0 {
					t.Fatalf("foreign event delivered: seen=%d err=%v", seen, err)
				}
			} else if err != nil || seen != 1 {
				t.Fatalf("reply lost: seen=%d err=%v", seen, err)
			}
		})
	}
}

type clusterRoundTrip func(*http.Request) (*http.Response, error)

func (f clusterRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRankAdmissionMatchesCompleteFrozenPlan(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ranks  []RankFence
		reject bool
	}{
		{"complete", []RankFence{testFence(0), testFence(1)}, false},
		{"missing", []RankFence{testFence(0)}, true},
		{"extra", []RankFence{testFence(0), testFence(1), testFence(2)}, true},
		{"substituted", []RankFence{testFence(0), testFence(2)}, true},
		{"foreign-host", []RankFence{testFence(0), {Rank: 1, HostID: "foreign", Fence: "f", Generation: "g", MemoryAdmissionBytes: 1, ExpiresAt: time.Now().Add(time.Minute).Format(time.RFC3339)}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(r *http.Request) (*http.Response, error) {
				var response any = Capability{Contract: ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain"}}
				if r.Method == http.MethodPost {
					response = testJob("admitting", tc.ranks)
				}
				data, err := json.Marshal(response)
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
			})}, true)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.CreateJob(context.Background(), testRequest(), []RankPlacement{{Rank: 0, HostID: "m5"}, {Rank: 1, HostID: "m5"}})
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v err=%v", tc.reject, err)
			}
		})
	}
}

func TestMissingAuthenticationAndPlanRefuseBeforeNetwork(t *testing.T) {
	if _, err := NewClusterClient("https://fixture.invalid", nil); err == nil {
		t.Fatal("nil transport accepted")
	}
	calls := 0
	client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected network") })}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testRequest(), nil); err == nil || calls != 0 {
		t.Fatalf("missing plan: calls=%d err=%v", calls, err)
	}
}

func TestLifecycleBindsFrozenTuple(t *testing.T) {
	for _, operation := range []string{"status", "cancel"} {
		for _, field := range []string{"valid", "execution", "request", "plan", "epoch", "empty-execution", "empty-request", "empty-plan"} {
			t.Run(operation+"/"+field, func(t *testing.T) {
				admitted := testJob("decoding", nil)
				response := testJob("released", nil)
				switch field {
				case "execution":
					response.ExecutionID = "other"
				case "request":
					response.RequestID = "other"
				case "plan":
					response.PlanSHA256 = strings.Repeat("b", 64)
				case "epoch":
					response.Epoch++
				case "empty-execution":
					response.ExecutionID = ""
				case "empty-request":
					response.RequestID = ""
				case "empty-plan":
					response.PlanSHA256 = ""
				}
				data, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
				})}, true)
				if err != nil {
					t.Fatal(err)
				}
				if operation == "status" {
					_, err = client.Status(context.Background(), admitted)
				} else {
					_, err = client.Cancel(context.Background(), admitted)
				}
				if (err == nil) != (field == "valid") {
					t.Fatalf("field=%s err=%v", field, err)
				}
			})
		}
	}
}

func TestExpiredOriginalReceiptStillAllowsRecovery(t *testing.T) {
	oldFence := testFence(0)
	oldFence.ExpiresAt = time.Now().Add(-time.Hour).Format(time.RFC3339)
	admitted := testJob("decoding", []RankFence{oldFence})
	if err := admitted.validate(admitted.RequestID, admitted.PlanSHA256, true); err == nil {
		t.Fatal("expired fence authorized fresh admission")
	}
	for _, operation := range []string{"status-released", "status-orphan", "cancel", "events"} {
		t.Run(operation, func(t *testing.T) {
			calls := 0
			client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				response := testJob("released", nil)
				if operation == "status-orphan" {
					response.State = "orphan_unknown"
				}
				data, err := json.Marshal(response)
				if err != nil {
					return nil, err
				}
				header := http.Header{}
				if operation == "events" {
					event := Event{Contract: admitted.Contract, JobID: admitted.JobID, ExecutionID: admitted.ExecutionID, RequestID: admitted.RequestID, PlanSHA256: admitted.PlanSHA256, Epoch: admitted.Epoch, Sequence: 5, Type: "released"}
					data, err = json.Marshal(event)
					if err != nil {
						return nil, err
					}
					data = []byte("data: " + string(data) + "\n\n")
					header.Set("Content-Type", "text/event-stream")
				}
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
			})}, true)
			if err != nil {
				t.Fatal(err)
			}
			switch operation {
			case "cancel":
				_, err = client.Cancel(context.Background(), admitted)
			case "events":
				err = client.Events(context.Background(), admitted, 4, func(Event) error { return nil })
			default:
				_, err = client.Status(context.Background(), admitted)
			}
			if err != nil || calls != 1 {
				t.Fatalf("historical receipt blocked recovery: calls=%d err=%v", calls, err)
			}
		})
	}
}

type streamFailure struct{}

func (streamFailure) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestStreamRequiresTerminalEvidenceAndRetainsCursor(t *testing.T) {
	admitted := testJob("decoding", nil)
	event := Event{Contract: admitted.Contract, JobID: admitted.JobID, ExecutionID: admitted.ExecutionID, RequestID: admitted.RequestID, PlanSHA256: admitted.PlanSHA256, Epoch: admitted.Epoch, Sequence: 1, Type: "token"}
	token, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	tokenLine := "data: " + string(token) + "\n\n"
	event.Sequence = 2
	event.Type = "released"
	terminal, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		body       io.Reader
		incomplete bool
		cursor     uint64
		seen       int
	}{
		{"empty", strings.NewReader(""), true, 0, 0},
		{"early-eof", strings.NewReader(tokenLine), true, 1, 1},
		{"transport-break", io.MultiReader(strings.NewReader(tokenLine), streamFailure{}), true, 1, 1},
		{"oversized-event", strings.NewReader("data: " + strings.Repeat("x", maxEventLine)), true, 0, 0},
		{"terminal-after-4MiB", strings.NewReader(strings.Repeat(": keepalive\n", 400000) + tokenLine + "data: " + string(terminal) + "\n\n"), false, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := newClusterClient("http://fixture.invalid", &http.Client{Transport: clusterRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(tc.body)}, nil
			})}, true)
			if err != nil {
				t.Fatal(err)
			}
			seen := 0
			err = client.Events(context.Background(), admitted, 0, func(Event) error { seen++; return nil })
			if tc.incomplete {
				var outcome *IncompleteStreamError
				if !errors.Is(err, ErrClusterIncomplete) || !errors.As(err, &outcome) || outcome.LastSequence != tc.cursor {
					t.Fatalf("lost incomplete/cursor outcome: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if seen != tc.seen {
				t.Fatalf("seen=%d want=%d", seen, tc.seen)
			}
		})
	}
}
