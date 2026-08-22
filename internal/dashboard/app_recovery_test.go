package dashboard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apprecovery"
)

type recoveryTestStore struct{ receipt apprecovery.Receipt }

func (s *recoveryTestStore) Save(receipt apprecovery.Receipt) error { s.receipt = receipt; return nil }
func (s *recoveryTestStore) Load(string) (apprecovery.Receipt, error) {
	if s.receipt.Schema == "" {
		return apprecovery.Receipt{}, errors.New("not found")
	}
	return s.receipt, nil
}

type recoveryTestDriver struct{ started bool }

func (d *recoveryTestDriver) Capture(context.Context, apprecovery.Target) (apprecovery.Snapshot, error) {
	return apprecovery.Snapshot{Files: map[string]string{"private": "hash"}}, nil
}
func (d *recoveryTestDriver) ClearTransientState(context.Context, apprecovery.Target) error {
	return nil
}
func (d *recoveryTestDriver) PID(context.Context, apprecovery.Target) (int, error) {
	if d.started {
		return 12, nil
	}
	return 11, nil
}
func (d *recoveryTestDriver) Stop(context.Context, apprecovery.Target, int) error { return nil }
func (d *recoveryTestDriver) Start(context.Context, apprecovery.Target, apprecovery.Snapshot) error {
	d.started = true
	return nil
}
func (d *recoveryTestDriver) Ready(context.Context, apprecovery.Target) error { return nil }

func TestRecoveryAPIRequiresSameOriginAndProjectsPrivacySafeReceipt(t *testing.T) {
	store := &recoveryTestStore{}
	manager, err := apprecovery.NewManager([]apprecovery.Target{{ID: "browser", Kind: apprecovery.KindAppSavedState, BundleID: "com.example.browser", ExecutablePath: "/Applications/Browser.app/Contents/MacOS/Browser", StatePaths: []string{"/private/state"}}}, &recoveryTestDriver{}, store)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{AppRecovery: manager})

	crossOrigin := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/recovery/restart", strings.NewReader(`{"target_id":"browser","mode":"restore"}`))
	crossOrigin.Host = "pantheon.test"
	crossOrigin.Header.Set("Origin", "https://attacker.example")
	crossResult := httptest.NewRecorder()
	server.apiRecoveryRestart(crossResult, crossOrigin)
	if crossResult.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", crossResult.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "http://pantheon.test/api/recovery/restart", strings.NewReader(`{"target_id":"browser","mode":"restore"}`))
	request.Host = "pantheon.test"
	request.Header.Set("Origin", "http://pantheon.test")
	result := httptest.NewRecorder()
	server.apiRecoveryRestart(result, request)
	if result.Code != http.StatusOK {
		t.Fatalf("restart status = %d body=%s", result.Code, result.Body.String())
	}
	if strings.Contains(result.Body.String(), "private") || strings.Contains(result.Body.String(), "hash") || strings.Contains(result.Body.String(), "Applications") {
		t.Fatalf("private recovery material escaped: %s", result.Body.String())
	}
}
