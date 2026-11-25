package routerstore

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

type maatStatusRecorder struct {
	base  http.RoundTripper
	codes chan int
}

func (r *maatStatusRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err == nil && req.URL.Path == "/v1/call/MaatMutate" {
		r.codes <- resp.StatusCode
	}
	return resp, err
}

func maatStatusClient(t *testing.T, s *SQLiteStore) (*RemoteStore, chan int) {
	t.Helper()
	h, err := Handler(s, ServerOptions{Token: "fixture-bootstrap", RuleOfRa: "off"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	token, _, err := s.MintHostToken("fixture-host", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	codes := make(chan int, 8)
	c := NewMemoryOnlyRemoteStore(srv.URL, token)
	c.host, c.agent, c.threadID = "fixture-host", "ra", "fixture-thread"
	c.client.Transport = &maatStatusRecorder{base: transport, codes: codes}
	t.Cleanup(func() { _ = c.Close() })
	return c, codes
}

func wantMaatStatus(t *testing.T, codes chan int, want int) {
	t.Helper()
	select {
	case got := <-codes:
		if got != want {
			t.Errorf("MaatMutate HTTP %d, want %d", got, want)
		}
	default:
		t.Fatal("no MaatMutate HTTP response observed")
	}
}

// Legitimate refusals travel as HTTP 409 with errors.Is(ErrReservationRefused)
// and the exact historical text; a store failure stays a 500.
func TestMaatMutateRefusalsAreHTTP409WithText(t *testing.T) {
	s, released, _ := archivedReceiptFixture(t)
	c, codes := maatStatusClient(t, s)
	for _, tc := range []struct{ name, action, id, text string }{
		{"heartbeat-terminal", "heartbeat", released, "is released, not active"},
		{"release-terminal", "release", released, "already released"},
		{"release-exit-terminal", "release", released, "already released"},
		{"extend-terminal", "extend", released, "is released, not active"},
		{"heartbeat-unknown", "heartbeat", "no-such-id", "not found"},
		{"release-unknown", "release", "no-such-id", "not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := ""
			switch tc.name {
			case "release-exit-terminal":
				value = "exit=0"
			case "extend-terminal":
				value = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
			}
			_, err := c.MaatMutate(tc.action, tc.id, value)
			if !errors.Is(err, schedule.ErrReservationRefused) || !strings.Contains(err.Error(), tc.text) || !strings.HasPrefix(err.Error(), "maat: reservation ") {
				t.Fatalf("want refusal with exact text %q, got %v", tc.text, err)
			}
			wantMaatStatus(t, codes, http.StatusConflict)
		})
	}
}

// Negative control: a database failure under the same call must NOT be mapped
// to the refusal sentinel or to 409.
func TestMaatMutateStoreFailureStaysHTTP500(t *testing.T) {
	s := openBackendStore(t, t.TempDir()+"/test.db")
	t.Cleanup(func() { _ = s.Close() })
	if s.dialect().name == "postgres" {
		t.Skip("SQLite trigger fixture")
	}
	now := s.clock().UTC()
	active := hotRowRow(now, "active", time.Minute, -1, nil)
	seedMaatRows(t, s, active)
	if _, err := s.exec(`CREATE TRIGGER fail_maat BEFORE UPDATE ON state WHEN NEW.key='maat:reservations' BEGIN SELECT RAISE(ABORT,'fixture db failure'); END`); err != nil {
		t.Fatal(err)
	}
	c, codes := maatStatusClient(t, s)
	_, err := c.MaatMutate("heartbeat", active["id"].(string), "")
	if err == nil || errors.Is(err, schedule.ErrReservationRefused) || !strings.Contains(err.Error(), "fixture db failure") {
		t.Fatalf("store failure must surface as a plain error, got %v", err)
	}
	wantMaatStatus(t, codes, http.StatusInternalServerError)
}
