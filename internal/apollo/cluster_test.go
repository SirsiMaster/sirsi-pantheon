package apollo

import (
	"context"
	"encoding/json"
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

func testPreparation() Preparation {
	return Preparation{
		Contract: ClusterContractV1, PreparationID: "preparation-1", RequestID: "request-1", SessionID: "session-1",
		ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), ModelID: "gemma",
		PlanSHA256: testDigest, CheckpointSHA256: testDigest, TokenizerSHA256: testDigest, TemplateSHA256: testDigest,
		Mode: "plain", Precision: "Q8", InputTokens: []uint32{2, 105}, StopTokens: []uint32{1}, MaxOutputTokens: 32, RankCount: 2,
	}
}

func TestClusterClientRejectsHTTPOutsideTest(t *testing.T) {
	if _, err := NewClusterClient("http://example.invalid", &http.Client{}); err == nil {
		t.Fatal("accepted unencrypted endpoint")
	}
	if _, err := NewClusterClient("https://example.invalid?redirect=elsewhere", &http.Client{}); err == nil {
		t.Fatal("accepted endpoint query")
	}
	if _, err := NewClusterClient("https://example.invalid", nil); err == nil {
		t.Fatal("accepted missing enrolled transport")
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
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testPreparation(), "idem-1"); err == nil || !strings.Contains(err.Error(), "all-rank") {
		t.Fatalf("CreateJob error = %v, want all-rank admission rejection", err)
	}
}

func TestCreateJobBindsFrozenRequestAndRankFences(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain"}})
	})
	mux.HandleFunc("/v1/apollo/jobs", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("client invented authorization header %q", got)
		}
		var request JobRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.PreparationID != "preparation-1" || request.SessionID != "session-1" || request.PlanSHA256 != testDigest || len(request.InputTokens) != 2 {
			t.Fatalf("frozen request = %#v", request)
		}
		_ = json.NewEncoder(w).Encode(testJob("admitting", []RankFence{testFence(0), {Rank: 1, HostID: "m1", Fence: "fence-2", Generation: "generation-2", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 2 << 30}}))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.CreateJob(context.Background(), testPreparation(), "idem-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(job.RankFences) != 2 || job.RankFences[1].HostID != "m1" {
		t.Fatalf("rank fences = %#v", job.RankFences)
	}
}

func TestPrepareBindsServiceOwnedTokenization(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain"}})
	})
	mux.HandleFunc("/v1/apollo/prepare", func(w http.ResponseWriter, r *http.Request) {
		var request PrepareRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Messages[0].Content != "hello" || request.ModelID != "gemma" || request.SessionID != "session-1" {
			t.Fatalf("prepare request = %#v", request)
		}
		out := testPreparation()
		_ = json.NewEncoder(w).Encode(out)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := client.Prepare(context.Background(), PrepareRequest{Contract: ClusterContractV1, RequestID: "request-1", SessionID: "session-1", ModelID: "gemma", Machines: []string{"auto"}, Mode: "plain", Precision: "Q8", Messages: []ChatMessage{{Role: "user", Content: "hello"}}, MaxOutputTokens: 32})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.PreparationID != "preparation-1" || len(prepared.InputTokens) != 2 || prepared.PlanSHA256 != testDigest {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestCreateJobRejectsNonContiguousRankAdmission(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, LaunchAvailable: true, StreamAvailable: true, CancelAvailable: true, Modes: []string{"plain"}})
	})
	mux.HandleFunc("/v1/apollo/jobs", func(w http.ResponseWriter, r *http.Request) {
		bad := testJob("admitting", []RankFence{{Rank: 7, HostID: "m5", Fence: "fence", Generation: "generation", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 1 << 30}, {Rank: 8, HostID: "m1", Fence: "fence2", Generation: "generation2", ExpiresAt: time.Now().UTC().Add(time.Minute).Format(time.RFC3339), MemoryAdmissionBytes: 1 << 30}})
		_ = json.NewEncoder(w).Encode(bad)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testPreparation(), "idem-1"); err == nil || !strings.Contains(err.Error(), "contiguous") {
		t.Fatalf("CreateJob error = %v, want rank topology refusal", err)
	}
}

func TestCreateJobRejectsPlannedCapability(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/capabilities", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Capability{Contract: ClusterContractV1, Modes: []string{"plain"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testPreparation(), "idem-1"); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("CreateJob error = %v, want planned service rejection", err)
	}
}

func TestEventsRejectForeignOrNonMonotonicSequence(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/jobs/job-1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"0123456789abcdef0123456789abcdef\",\"request_id\":\"request-1\",\"plan_sha256\":\"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\",\"epoch\":9,\"sequence\":1,\"type\":\"admitted\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"0123456789abcdef0123456789abcdef\",\"request_id\":\"request-1\",\"plan_sha256\":\"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\",\"epoch\":9,\"sequence\":1,\"type\":\"token\"}\n\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	err = client.Events(context.Background(), testJob("admitting", []RankFence{testFence(0)}), 0, func(Event) error { seen++; return nil })
	if err == nil || !strings.Contains(err.Error(), "non-monotonic") || seen != 1 {
		t.Fatalf("Events error=%v seen=%d", err, seen)
	}
}

func TestEventsPreserveAuthorizedTokenPayload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/jobs/job-1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"0123456789abcdef0123456789abcdef\",\"request_id\":\"request-1\",\"plan_sha256\":\"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\",\"epoch\":9,\"sequence\":1,\"type\":\"token\",\"text\":\"hello\",\"token_id\":42}\n\n"))
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"0123456789abcdef0123456789abcdef\",\"request_id\":\"request-1\",\"plan_sha256\":\"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\",\"epoch\":9,\"sequence\":2,\"type\":\"released\"}\n\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	err = client.Events(context.Background(), testJob("admitting", []RankFence{testFence(0)}), 0, func(event Event) error {
		raw, err = json.Marshal(event)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"text":"hello"`) || !strings.Contains(string(raw), `"token_id":42`) {
		t.Fatalf("token payload was lost: %s", raw)
	}
}

func TestEventsRejectsEOFBeforeTerminalEvidence(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/jobs/job-1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"0123456789abcdef0123456789abcdef\",\"request_id\":\"request-1\",\"plan_sha256\":\"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\",\"epoch\":9,\"sequence\":1,\"type\":\"token\",\"text\":\"partial\"}\n\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Events(context.Background(), testJob("admitting", []RankFence{testFence(0)}), 0, func(Event) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "without release") {
		t.Fatalf("Events error = %v, want nonterminal EOF refusal", err)
	}
}
