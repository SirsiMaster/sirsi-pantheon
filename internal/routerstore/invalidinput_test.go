package routerstore

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// router-client-errors-not-500: a caller mistake (bad message type, missing
// required field, ...) must surface as a 4xx the caller can act on, not a
// 500 that looks like a server/store fault and gets retried forever.

func TestValidateMessageTypeRejectsUnknownTypeAsInvalidInput(t *testing.T) {
	if err := validateMessageType("not-a-real-type"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown message type must wrap ErrInvalidInput, got %v", err)
	}
	for _, ok := range []string{"", "proposal", "review", "decision"} {
		if err := validateMessageType(ok); err != nil {
			t.Fatalf("documented type %q must be accepted, got %v", ok, err)
		}
	}
}

func TestWriteCallErrMapsInvalidInputToBadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	(&server{}).writeCallErr(w, "Send", fmt.Errorf("Send: %w", ErrInvalidInput))
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("ErrInvalidInput")) {
		t.Fatalf("invalid input must be 400, not a retryable server fault: %d %s", w.Code, w.Body.String())
	}
}

// TestStoreCallerValidationWrapsInvalidInput is a representative sample of the
// router-client-errors-not-500 slice: each of these call sites used to return
// a plain fmt.Errorf that fell through writeCallErr to a 500. One site per
// family (required field, invalid enum, invalid format, unknown action) is
// enough to prove the pattern holds across the batch without re-testing every
// one of the ~320 sites the task tracks.
func TestStoreCallerValidationWrapsInvalidInput(t *testing.T) {
	s := newTestStore(t)
	if err := s.Put(Item{ID: "x"}); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	cases := []struct {
		name string
		err  error
	}{
		{"RegisterAgent empty id", s.RegisterAgent("", 1)},
		{"SetState empty key", s.SetState("", "v")},
		{"Put empty id", s.Put(Item{})},
		{"Put invalid status", s.Put(Item{ID: "x", Status: "not-a-status"})},
		{"AckItem empty id", s.AckItem("")},
		{"ReassignItem missing fields", s.ReassignItem("", "", "", "")},
		{"ClaimItem empty agent", func() error { _, err := s.ClaimItem("", "x", time.Minute); return err }()},
		{"ResetBreaker empty domain", s.ResetBreaker("")},
		{"ForceOwner unknown action", s.ForceOwner("x", "bogus", "because")},
		{"ClassifyLane empty agent", func() error { _, err := s.ClassifyLane("", true, time.Minute); return err }()},
		{"AllocateIdentifier bad namespace", func() error { _, err := s.AllocateIdentifier("bad ns", "t", "o"); return err }()},
		{"Waive invalid req id", s.Waive("not-an-id", "reason", "owner-ref")},
		{"MaatMutate unknown action", func() error { _, err := s.MaatMutate("bogus", "x", ""); return err }()},
	}
	for _, tc := range cases {
		if !errors.Is(tc.err, ErrInvalidInput) {
			t.Errorf("%s: want ErrInvalidInput, got %v", tc.name, tc.err)
		}
	}
	if err := s.Put(Item{ID: "control-valid-item"}); err != nil {
		t.Errorf("control case (valid Put) must not error, got %v", err)
	}
}
