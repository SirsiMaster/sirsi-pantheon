package routerstore

import (
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"
)

// ADR-069: the durable-outbox safety hinge. A "never reached the service" error
// (no connection established) is safe to HOLD and retry in order; a post-send
// failure (maybe committed) must NOT be — it stays OUTCOME UNKNOWN. Both
// directions are asserted: the classifier is worthless if it can't say NO.
func TestNeverReachedService(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"conn refused (dial)", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, true},
		{"dial timeout", &net.OpError{Op: "dial", Err: errors.New("i/o timeout")}, true},
		{"dns failure", &net.DNSError{Err: "no such host", Name: "sirsi-router"}, true},
		{"wrapped url.Error dial-refused", &url.Error{Op: "Post", URL: "http://svc/v1/call/SendGuarded", Err: &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}}, true},
		// NEGATIVE controls — must be false: the request may have reached the service.
		// SSA #794 (2026-09-27): classification requires the DIAL phase. A bare
		// ECONNREFUSED carries no phase, and a read/write-phase op error carrying
		// ECONNREFUSED/reset happened AFTER connect — the request may have crossed
		// the boundary, so both must stay UNKNOWN (fail closed), never auto-retried.
		{"bare ECONNREFUSED (no phase → unknown)", syscall.ECONNREFUSED, false},
		{"post-connect ECONNREFUSED on write (maybe committed)", &net.OpError{Op: "write", Err: syscall.ECONNREFUSED}, false},
		{"post-connect ECONNREFUSED on read (maybe committed)", &net.OpError{Op: "read", Err: syscall.ECONNREFUSED}, false},
		{"post-send read reset (maybe committed)", &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}, false},
		{"write op after connect", &net.OpError{Op: "write", Err: errors.New("broken pipe")}, false},
		{"generic error", errors.New("service response over limit"), false},
	}
	for _, c := range cases {
		if got := neverReachedService(c.err); got != c.want {
			t.Errorf("%s: neverReachedService(%v) = %v, want %v", c.name, c.err, got, c.want)
		}
	}
}
