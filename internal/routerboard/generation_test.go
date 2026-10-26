package routerboard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type generationStreamRecorder struct {
	*httptest.ResponseRecorder
	onFlush func()
}

func (w generationStreamRecorder) Flush() { w.ResponseRecorder.Flush(); w.onFlush() }

func TestFabricGenerationBoardFailureRetainsBytesAndSSEInvalidates(t *testing.T) {
	failed := false
	input := ledger.FabricRead{Snapshot: ledger.Snapshot{GeneratedAt: "2026-10-25T12:00:00Z", Agents: []ledger.Agent{{AgentID: "lane", Tasks: []routerstore.Task{{TaskID: "task", Status: "pending", Subject: "inert"}}}}}}
	producer := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) {
		if failed {
			return ledger.FabricRead{}, errors.New("inert unavailable")
		}
		return input, nil
	}, "fixture")
	b := NewWithProducer("/not-executable", "/not-readable", "ui-fixture", producer.Read)
	h := NewHandler(b, "ui")
	initial := httptest.NewRecorder()
	h.snapshot(initial, httptest.NewRequest(http.MethodGet, "/api/router/v1/snapshot", nil))
	if initial.Code != 503 {
		t.Fatal("pre-first frame fabricated success")
	}
	b.Poll(context.Background())
	before, version, valid := b.SnapshotState()
	if !valid || version == 0 {
		t.Fatal("positive generation not published")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	flushes := 0
	w := generationStreamRecorder{ResponseRecorder: httptest.NewRecorder(), onFlush: func() {
		flushes++
		if flushes == 1 {
			failed = true
			b.Poll(context.Background())
		} else {
			cancel()
		}
	}}
	h.stream(w, httptest.NewRequest(http.MethodGet, "/api/router/v1/stream", nil).WithContext(ctx))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(w.Body.String(), "data: "+string(before)) || !strings.Contains(w.Body.String(), "event: invalid") {
		t.Fatalf("SSE change/invalid contract lost: %s", w.Body.String())
	}
	after, failedVersion, valid := b.SnapshotState()
	if valid || string(after) != string(before) || failedVersion <= version {
		t.Fatal("failed read overwrote retained bytes or failed to invalidate")
	}
	if _, _, err := b.SnapshotFleetShape(); err == nil {
		t.Fatal("once/fleet printed invalid retained generation")
	}
	for _, path := range []string{"/api/ledger", "/api/tasks", "/api/router/v1/ledger", "/api/router/v1/tasks", "/api/router/v1/snapshot", "/api/router/v1/stream"} {
		mux := http.NewServeMux()
		h.RegisterReadOnly(mux)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != 503 {
			t.Fatal("failed source served healthy empty/retained DTO", path, response.Code)
		}
	}
	failed = false
	b.Poll(context.Background())
	_, newVersion, valid := b.SnapshotState()
	if !valid || newVersion <= failedVersion {
		t.Fatal("fresh recovery publication unavailable")
	}
}
