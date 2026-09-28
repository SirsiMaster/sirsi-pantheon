package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
)

func TestApolloTelemetryProjection(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{ApolloTelemetryFn: func() (apollo.TelemetryRead, error) {
		return apollo.TelemetryRead{State: "awaiting_session", Reason: "no qualified session"}, nil
	}})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/apollo/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET telemetry = %d", resp.StatusCode)
	}
	var got apollo.TelemetryRead
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.State != "awaiting_session" || got.Reason == "" {
		t.Fatalf("telemetry = %+v", got)
	}
}

func TestApolloTelemetryUnavailableIsHonest(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{})
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/apollo/telemetry")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET telemetry = %d", resp.StatusCode)
	}
}

func TestApolloTelemetryOnlyAcceptsReadRequests(t *testing.T) {
	t.Parallel()
	ts := testServer(t, Config{ApolloTelemetryFn: func() (apollo.TelemetryRead, error) {
		return apollo.TelemetryRead{State: "awaiting_session"}, nil
	}})
	defer ts.Close()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/apollo/telemetry", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST telemetry = %d", resp.StatusCode)
	}
}
