package desktoprecovery

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderedEntryKeepsNodeInFormAction(t *testing.T) {
	g := &Gateway{}
	for _, tc := range []struct {
		name, message string
		status        int
	}{
		{"fresh", "", http.StatusOK},
		{"denied", "Request a fresh recovery admission.", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			g.renderEntry(w, Node{ID: "m1"}, tc.status, tc.message)
			body := w.Body.String()
			if w.Code != tc.status || !strings.Contains(body, `action="/recovery/v1/nodes/m1/sessions"`) || strings.Contains(body, `nodes//sessions`) {
				t.Fatalf("entry status/form target failed: status=%d", w.Code)
			}
			if tc.message != "" && !strings.Contains(body, tc.message) {
				t.Fatal("missing admission resolution message")
			}
		})
	}
}
