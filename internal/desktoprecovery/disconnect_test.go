package desktoprecovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExplicitDisconnectReleasesSession(t *testing.T) {
	for _, origin := range []string{"https://approved.example", "https://forged.example"} {
		t.Run(origin, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			raw := make([]byte, 32)
			hash := sha256.Sum256(raw)
			g := &Gateway{now: time.Now, sessions: map[[sha256.Size]byte]session{hash: {nodeID: "m1", active: true, expiresAt: time.Now().Add(time.Minute), cancel: cancel}}}
			r := httptest.NewRequest(http.MethodPost, "https://approved.example/recovery/v1/nodes/m1/disconnect", nil)
			r.Header.Set("Origin", origin)
			r.AddCookie(&http.Cookie{Name: cookieName, Value: hex.EncodeToString(raw)})
			w := httptest.NewRecorder()
			g.disconnect(w, r, Node{ID: "m1", Origins: []string{"https://approved.example"}})
			if origin == "https://forged.example" {
				if ctx.Err() != nil || w.Code != http.StatusForbidden {
					t.Fatal("forged origin disconnected session")
				}
				return
			}
			if ctx.Err() == nil || len(g.sessions) != 0 || w.Code != http.StatusNoContent {
				t.Fatalf("session not released: %d", w.Code)
			}
			if len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
				t.Fatal("cookie not cleared")
			}
		})
	}
}

func TestGatewayCloseReleasesHijackedSessions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	g := &Gateway{sessions: map[[sha256.Size]byte]session{{}: {cancel: cancel}}}
	g.Close()
	if ctx.Err() == nil || len(g.sessions) != 0 {
		t.Fatal("shutdown retained session")
	}
}
