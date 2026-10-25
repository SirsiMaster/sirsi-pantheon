package dashboard

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mercury"
)

func TestMercuryExistingEntryPointsBindOnlyCanonicalReader(t *testing.T) {
	for _, path := range []string{"cmd/sirsi/dashboard.go", "cmd/sirsi-menubar/main.go", "cmd/sirsi-gui/main_darwin.go"} {
		t.Run(path, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "..", filepath.FromSlash(path)), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			bindings := 0
			ast.Inspect(file, func(node ast.Node) bool {
				kv, ok := node.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "MercuryFn" {
					return true
				}
				bindings++
				method, ok := kv.Value.(*ast.SelectorExpr)
				if !ok || method.Sel.Name != "Read" {
					t.Fatal("entrypoint does not bind the canonical Read method")
				}
				call, ok := method.X.(*ast.CallExpr)
				if !ok || len(call.Args) != 0 {
					t.Fatal("entrypoint must use configured authority without caller URL arguments")
				}
				constructor, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || constructor.Sel.Name != "NewConfigured" {
					t.Fatal("entrypoint bypassed the canonical configured reader")
				}
				pkg, ok := constructor.X.(*ast.Ident)
				if !ok || pkg.Name != "mercury" {
					t.Fatal("entrypoint bound another reader")
				}
				return true
			})
			if bindings != 1 {
				t.Fatalf("canonical Mercury bindings = %d, want one", bindings)
			}
		})
	}
}

func TestMercuryReadPassesExactRequestCancellationAndTypedState(t *testing.T) {
	type fixtureKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), fixtureKey{}, "owned-request"))
	cancel()
	calls := 0
	s := New(Config{MercuryFn: func(observed context.Context) mercury.Read {
		calls++
		if observed.Value(fixtureKey{}) != "owned-request" || observed.Err() != context.Canceled {
			t.Fatal("request cancellation did not reach the shared reader")
		}
		return mercury.Read{State: "offline", Message: "canceled fixture"}
	}})
	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/mercury", nil).WithContext(ctx))
	var read mercury.Read
	if err := json.Unmarshal(w.Body.Bytes(), &read); err != nil || calls != 1 || read.State != "offline" || read.Snapshot != nil || read.Total != nil {
		t.Fatalf("typed cancellation changed: calls=%d read=%+v err=%v", calls, read, err)
	}
}

func TestMercuryReadOnlyAPIAndConsumerView(t *testing.T) {
	calls := 0
	s := New(Config{MercuryFn: func(ctx context.Context) mercury.Read {
		calls++
		return mercury.Read{State: "unconfigured", Message: "Configure monitor"}
	}})
	for _, method := range []string{"GET", "POST", "DELETE"} {
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, httptest.NewRequest(method, "/api/mercury", nil))
		want := http.StatusMethodNotAllowed
		if method == "GET" {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("%s: %d", method, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache")
		}
	}
	if calls != 1 {
		t.Fatal("mutating request reached reader")
	}
	w := httptest.NewRecorder()
	s.handler.ServeHTTP(w, httptest.NewRequest("GET", "/?view=mercury", nil))
	if !strings.Contains(w.Body.String(), `src="/assets/mercury.js"`) {
		t.Fatal("missing mounted consumer")
	}
	if strings.Index(w.Body.String(), `src="/assets/mercury.js"`) > strings.Index(w.Body.String(), `src="/assets/app.js"`) {
		t.Fatal("consumer loaded too late")
	}
	disabled := New(Config{})
	w = httptest.NewRecorder()
	disabled.handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/mercury", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

// Both public doors must retain one console, while the router's CSP continues
// to require external JavaScript. Exercise the handler without host sockets.
func TestMercuryConsoleParityAndCSPAssets(t *testing.T) {
	s := New(Config{})
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	home, router := get("/"), get("/router")
	if home.Code != 200 || router.Code != 200 || home.Body.String() != router.Body.String() {
		t.Fatal("console doors diverged")
	}
	csp := router.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self';") {
		t.Fatal("router script policy weakened", csp)
	}
	if strings.Contains(router.Body.String(), "window.mountMercury") {
		t.Fatal("inline executable script bypasses router CSP")
	}
	for _, tc := range []struct{ path, mime, content string }{
		{"/assets/mercury.js", "application/javascript", "window.mountMercury"},
		{"/assets/mercury.css", "text/css", "mc-scene"},
	} {
		w := get(tc.path)
		if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), tc.mime) || !strings.Contains(w.Body.String(), tc.content) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("missing uncached external asset: %s", tc.path)
		}
	}
}
