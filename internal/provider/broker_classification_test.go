package provider

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestUnknown503NotBackpressureAndContextCapacityPermanent(t *testing.T) {
	for _, c := range []struct {
		name, body, after string
		busy, permanent   bool
	}{
		{"unknown503", `{"error":{"code":"runtime_failure","type":"native_error","message":"native runtime failed"}}`, "", false, false},
		{"bareBusyCodeNotContract", `{"error":{"code":"BUSY","message":"busy"}}`, "", false, false},
		{"retryAfterBusy", `{"error":{"code":"BUSY","message":"capacity busy"}}`, "2", true, false},
		{"contextCapacitySynthetic", `{"error":{"code":"context_capacity_exceeded","type":"invalid_request_error","message":"input exceeds context capacity"}}`, "", false, true},
		{"contextMismatchOverridesRetryAfter", `{"error":{"code":"context_mismatch","message":"context mismatch"}}`, "2", false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(c.body))}
			if c.after != "" {
				r.Header.Set("Retry-After", c.after)
			}
			e := BrokerError(r, "")
			var b *InferenceError
			if !errors.As(e, &b) || b.Busy != c.busy || b.Permanent != c.permanent || b.Message == "" || b.Code == "" {
				t.Fatalf("incorrect503classification: %v", e)
			}
			if !c.busy && b.RetryAfter != 0 {
				t.Fatal("unknown/permanent failure received busy delay")
			}
		})
	}
}
