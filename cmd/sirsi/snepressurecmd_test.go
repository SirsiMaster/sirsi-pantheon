package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func TestCLIPrefixCachePressureReadsSharedLoopbackView(t *testing.T) {
	oldBase, oldToken := snePressureBaseURL, snePressureLoadToken
	oldClient := snePressureHTTPClient
	t.Cleanup(func() { snePressureBaseURL, snePressureLoadToken, snePressureHTTPClient = oldBase, oldToken, oldClient })
	fixture := dashboard.PrefixCachePressureAuthorizationView{
		State: "owner-confirmation-required",
		Receipt: sne.PrefixCachePressureReceipt{Observation: sne.PrefixCachePressureObservation{
			RequestID: "pressure-test", HostID: "m5", ObservedAtUnix: 1, ExpiresAtUnix: 2,
		}, ObservationSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	snePressureHTTPClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/api/sne/prefix-cache-pressure" || request.Header.Get("Authorization") != "Bearer test-capability" {
			return &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(bytes.NewBufferString("wrong request")), Header: make(http.Header)}, nil
		}
		body, err := json.Marshal(fixture)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, err
	})}
	snePressureBaseURL = "http://127.0.0.1:9119"
	snePressureLoadToken = func() (string, error) { return "test-capability", nil }
	view, err := requestPrefixCachePressure(http.MethodGet, nil)
	if err != nil {
		t.Fatal(err)
	}
	if view.State != fixture.State || view.Receipt.Observation.RequestID != fixture.Receipt.Observation.RequestID {
		t.Fatalf("view = %+v", view)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
