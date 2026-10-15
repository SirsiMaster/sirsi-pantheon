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

func testRequest() JobRequest {
	return JobRequest{
		Contract: ClusterContractV1, RequestID: "request-1", IdempotencyKey: "idem-1", SessionID: "session-1",
		PlanSHA256: testDigest, ModelID: "gemma", CheckpointSHA256: testDigest, TokenizerSHA256: testDigest,
		TemplateSHA256: testDigest, Mode: "plain", Precision: "Q8", InputTokens: []uint32{2, 105}, MaxOutputTokens: 32,
	}
}

func TestClusterClientRejectsHTTPOutsideTest(t *testing.T) {
	if _, err := NewClusterClient("http://example.invalid", nil); err == nil {
		t.Fatal("accepted unencrypted endpoint")
	}
	if _, err := NewClusterClient("https://example.invalid?redirect=elsewhere", nil); err == nil {
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
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testRequest()); err == nil || !strings.Contains(err.Error(), "all-rank") {
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
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	job, err := client.CreateJob(context.Background(), testRequest())
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
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateJob(context.Background(), testRequest()); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("CreateJob error = %v, want planned service rejection", err)
	}
}

func TestEventsRejectForeignOrNonMonotonicSequence(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/apollo/jobs/job-1/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"exec\",\"epoch\":9,\"sequence\":1,\"type\":\"admitted\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"contract\":\"apollo.cluster-inference.v1\",\"job_id\":\"job-1\",\"execution_id\":\"exec\",\"epoch\":9,\"sequence\":1,\"type\":\"token\"}\n\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	client, err := newClusterClient(srv.URL, srv.Client(), true)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	err = client.Events(context.Background(), "job-1", 0, func(Event) error { seen++; return nil })
	if err == nil || !strings.Contains(err.Error(), "non-monotonic") || seen != 1 {
		t.Fatalf("Events error=%v seen=%d", err, seen)
	}
}
