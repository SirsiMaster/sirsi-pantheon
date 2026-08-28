package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestPrefixCachePressureAuthorizationRequiresVisibleBoundConfirmation(t *testing.T) {
	now := time.Unix(1_788_000_000, 0).UTC()
	confirm := NewConfirmGuard()
	confirm.now = func() time.Time { return now }
	manager := newPrefixCachePressureAuthorizationManager(confirm)
	manager.now = func() time.Time { return now }
	manager.hostID = func() (string, error) { return "m5", nil }
	manager.sample = func() sne.ResourceAdmission {
		return sne.ResourceAdmission{TotalRAMBytes: 48 << 30, AvailableRAMBytes: 20 << 30, SwapLimitBytes: 3 << 30, Pressure: "warning", PressureSource: "host_statistics64"}
	}
	prepared, err := manager.prepare()
	if err != nil {
		t.Fatal(err)
	}
	if prepared.State != "owner-confirmation-required" || prepared.Confirmation == nil || prepared.Receipt.Observation.RequestID == "" {
		t.Fatalf("prepared view = %+v", prepared)
	}
	accepted, err := manager.authorize(prefixCachePressureAuthorizeRequest{
		RequestID: prepared.Receipt.Observation.RequestID, ObservationSHA256: prepared.Receipt.ObservationSHA256,
		ConfirmToken: prepared.Confirmation.ConfirmToken, ActionHash: prepared.Confirmation.ActionHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.State != "authorization-accepted" || accepted.Authorization == nil || accepted.Authorization.Operation != sne.PrefixCachePressureOperation {
		t.Fatalf("accepted view = %+v", accepted)
	}
	if _, err := manager.authorize(prefixCachePressureAuthorizeRequest{RequestID: prepared.Receipt.Observation.RequestID, ObservationSHA256: prepared.Receipt.ObservationSHA256, ConfirmToken: prepared.Confirmation.ConfirmToken, ActionHash: prepared.Confirmation.ActionHash}); err == nil {
		t.Fatal("replayed confirmation was accepted")
	}
}

func TestPrefixCachePressureEndpointReturnsOnlyBoundAuthorization(t *testing.T) {
	now := time.Unix(1_788_000_000, 0).UTC()
	server := New(Config{SNELocalAccessToken: "test-capability"})
	server.confirm.now = func() time.Time { return now }
	server.snePressure.now = func() time.Time { return now }
	server.snePressure.hostID = func() (string, error) { return "m5", nil }
	server.snePressure.sample = func() sne.ResourceAdmission {
		return sne.ResourceAdmission{TotalRAMBytes: 48 << 30, AvailableRAMBytes: 20 << 30, SwapLimitBytes: 3 << 30, Pressure: "normal", PressureSource: "host_statistics64"}
	}
	prepareRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/sne/prefix-cache-pressure", nil)
	prepareRequest.Header.Set("Authorization", "Bearer test-capability")
	prepareRecorder := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(prepareRecorder, prepareRequest)
	if prepareRecorder.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", prepareRecorder.Code, prepareRecorder.Body.String())
	}
	var prepared PrefixCachePressureAuthorizationView
	if err := json.NewDecoder(prepareRecorder.Body).Decode(&prepared); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(prefixCachePressureAuthorizeRequest{RequestID: prepared.Receipt.Observation.RequestID, ObservationSHA256: prepared.Receipt.ObservationSHA256, ConfirmToken: prepared.Confirmation.ConfirmToken, ActionHash: prepared.Confirmation.ActionHash})
	if err != nil {
		t.Fatal(err)
	}
	commitRequest := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/sne/prefix-cache-pressure", bytes.NewReader(body))
	commitRequest.Header.Set("Authorization", "Bearer test-capability")
	commitRecorder := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(commitRecorder, commitRequest)
	if commitRecorder.Code != http.StatusOK {
		t.Fatalf("commit status=%d body=%s", commitRecorder.Code, commitRecorder.Body.String())
	}
	var accepted PrefixCachePressureAuthorizationView
	if err := json.NewDecoder(commitRecorder.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.Authorization == nil || accepted.Authorization.ArtifactSHA256 != prepared.Receipt.ObservationSHA256 {
		t.Fatalf("accepted view=%+v", accepted)
	}
}

func TestPrefixCachePressureAuthorizationPreservesQuarantine(t *testing.T) {
	manager := newPrefixCachePressureAuthorizationManager(NewConfirmGuard())
	manager.containment = func() string { return "quarantined" }
	if _, err := manager.prepare(); err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("quarantine prepare error = %v", err)
	}
}
