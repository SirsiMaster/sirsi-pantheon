package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Explicit bounded local preview of the production handlers and embedded UI.
// Every row is synthetic, no router store or credential is opened, and no
// operational action provider is configured. Default suites do not bind it.
func TestFabricVisualFixture(t *testing.T) {
	file := os.Getenv("PANTHEON_FABRIC_VISUAL_FIXTURE")
	if file == "" {
		t.Skip("explicit bounded loopback visual fixture only")
	}
	snapshot := ledger.Snapshot{Agents: []ledger.Agent{{AgentID: "fixture-lane"}}}
	for group, status := range []string{"done", "pending", "in-progress", "in-progress", "blocked"} {
		for i := 0; i < 70; i++ {
			live := "active"
			if group == 3 {
				live = "stalled"
			}
			snapshot.Agents[0].Tasks = append(snapshot.Agents[0].Tasks, routerstore.Task{Agent: "fixture-lane", TaskID: fmt.Sprintf("fixture-%d-%03d", group, i), Subject: fmt.Sprintf("Development fixture task %d/%03d", group, i), Status: status, Liveness: live})
		}
	}
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) {
		snapshot.GeneratedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return ledger.FabricRead{Snapshot: snapshot}, nil
	}, RouterFn: func() (RouterSnapshot, error) {
		return RouterSnapshot{Version: "DEVELOPMENT FIXTURE — NOT LIVE", GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Lanes: RouterLanes{Counts: map[string]int{}}}, nil
	}})
	done := make(chan struct{})
	var once sync.Once
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture_done" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			once.Do(func() { close(done) })
			return
		}
		s.srv.Handler.ServeHTTP(w, r)
	}))
	defer ts.Close()
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(map[string]any{"url": ts.URL, "fixture": true, "rows": 350, "blocked": 70, "maximum_seconds": 180}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(180 * time.Second)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
}
