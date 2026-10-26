package routerstore

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
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
