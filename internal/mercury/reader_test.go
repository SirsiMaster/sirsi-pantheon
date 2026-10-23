package mercury

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

var fixtureBodies = func() map[string]json.RawMessage {
	var x map[string]json.RawMessage
	if err := json.Unmarshal([]byte(fixtureJSON), &x); err != nil {
		panic(err)
	}
	return x
}()

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureClient(body []byte) *Client {
	c := New("http://127.0.0.1:8099/api/v1/dashboard")
	c.Now = func() time.Time { return time.Unix(1792339200, 0) }
	c.HTTP = doerFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "X-Mercury-Dashboard-Schema": {"1"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	return c
}

func TestNewConfiguredDistinguishesUnsetFromExplicitEmpty(t *testing.T) {
	t.Setenv(DashboardURLEnv, "")
	if err := os.Unsetenv(DashboardURLEnv); err != nil {
		t.Fatal(err)
	}
	defaultClient := NewConfigured()
	if defaultClient.Endpoint != DefaultEndpoint {
		t.Fatalf("unset endpoint = %q, want fixed loopback %q", defaultClient.Endpoint, DefaultEndpoint)
	}
	if err := os.Setenv(DashboardURLEnv, ""); err != nil {
		t.Fatal(err)
	}
	explicitEmpty := NewConfigured()
	if explicitEmpty.Endpoint != "" {
		t.Fatalf("explicit empty endpoint = %q, want unconfigured", explicitEmpty.Endpoint)
	}
	called := false
	explicitEmpty.HTTP = doerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, nil
	})
	if read := explicitEmpty.Read(context.Background()); called || read.State != "unconfigured" {
		t.Fatalf("explicit empty endpoint performed a read or changed state: called=%v state=%q", called, read.State)
	}
	if err := os.Setenv(DashboardURLEnv, "http://127.0.0.1:9000/api/v1/dashboard"); err != nil {
		t.Fatal(err)
	}
	if got := NewConfigured().Endpoint; got != "http://127.0.0.1:9000/api/v1/dashboard" {
		t.Fatalf("explicit endpoint changed: %q", got)
	}
}

func TestProducerFixtures(t *testing.T) {
	for name, b := range fixtureBodies {
		t.Run(name, func(t *testing.T) {
			d, err := Decode(b)
			if err != nil {
				t.Fatal(err)
			}
			if len(d.Nodes) == 0 {
				t.Fatal("nodes lost")
			}
			r := fixtureClient(b).Read(context.Background())
			if r.State != d.Status {
				t.Fatalf("state %s", r.State)
			}
			if name == "uncalibrated" && d.Nodes[0].GPU.WorkPct != nil {
				t.Fatal("unknown became zero")
			}
			if name == "offline-peer" && r.Total != nil {
				t.Fatal("offline fabricated a live total")
			}
		})
	}
}
func TestUnknownServingNodeNeverFabricatesZeroTotals(t *testing.T) {
	var x map[string]any
	_ = json.Unmarshal(fixtureBodies["idle"], &x)
	x["source"].(map[string]any)["host"] = "unknown-monitor"
	b, _ := json.Marshal(x)
	read := fixtureClient(b).Read(context.Background())
	if read.Total != nil || read.Incoming != nil || read.Outgoing != nil {
		t.Fatal("unknown monitor fabricated zero totals")
	}
}

func TestConsumerCompatibilityAndInvalidData(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(map[string]any)
		wantErr bool
	}{
		{"future optional", func(x map[string]any) { x["future"] = true }, false},
		{"future enum", func(x map[string]any) { x["run"].(map[string]any)["phase"] = "future-phase" }, false},
		{"missing", func(x map[string]any) { delete(x, "nodes") }, true},
		{"null required", func(x map[string]any) { x["status"] = nil }, true},
		{"wrong version", func(x map[string]any) { x["schema_version"] = "2.0.0" }, true},
		{"uncalibrated number", func(x map[string]any) {
			x["nodes"].([]any)[0].(map[string]any)["gpu"].(map[string]any)["calibration"].(map[string]any)["status"] = "uncalibrated"
		}, true},
		{"range", func(x map[string]any) {
			x["nodes"].([]any)[0].(map[string]any)["gpu"].(map[string]any)["work_pct"] = 101
		}, true},
		{"null origin", func(x map[string]any) { x["links"].([]any)[0].(map[string]any)["ends"].([]any)[0] = nil }, true},
		{"wrong link", func(x map[string]any) { x["links"].([]any)[0].(map[string]any)["id"] = "wrong" }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var x map[string]any
			_ = json.Unmarshal(fixtureBodies["live"], &x)
			tc.mutate(x)
			b, _ := json.Marshal(x)
			_, err := Decode(b)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
		})
	}
}
func TestReadBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*Client)
		state  string
	}{
		{"unconfigured", func(c *Client) { c.Endpoint = "" }, "unconfigured"},
		{"public", func(c *Client) { c.Endpoint = "http://8.8.8.8/api/v1/dashboard" }, "invalid"},
		{"secret url", func(c *Client) { c.Endpoint = "http://user:secret@localhost/api/v1/dashboard" }, "invalid"},
		{"query", func(c *Client) { c.Endpoint += "?token=fixture" }, "invalid"},
		{"stale", func(c *Client) { c.Now = func() time.Time { return time.Unix(1792339210, 0) } }, "stale"},
		{"too big", func(c *Client) { c.HTTP = fixtureClient([]byte(strings.Repeat(" ", MaxBytes+1))).HTTP }, "invalid"},
		{"offline", func(c *Client) {
			c.HTTP = doerFunc(func(*http.Request) (*http.Response, error) { return nil, os.ErrNotExist })
		}, "offline"},
		{"permission", func(c *Client) {
			c.HTTP = doerFunc(func(*http.Request) (*http.Response, error) { return nil, os.ErrPermission })
		}, "permission_required"},
		{"bad json", func(c *Client) { c.HTTP = fixtureClient([]byte("{")).HTTP }, "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixtureClient(fixtureBodies["live"])
			tc.modify(c)
			r := c.Read(context.Background())
			if r.State != tc.state {
				t.Fatalf("%s != %s", r.State, tc.state)
			}
			if strings.Contains(r.Message, "secret") {
				t.Fatal("leaked URL")
			}
			if r.State == "stale" && r.Total != nil {
				t.Fatal("stale live total")
			}
		})
	}
}
func TestHTTPGuardsAndCancellation(t *testing.T) {
	for _, kind := range []string{"status", "type", "schema", "cancel", "refused"} {
		t.Run(kind, func(t *testing.T) {
			c := fixtureClient(fixtureBodies["live"])
			normal := c.HTTP
			c.HTTP = doerFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.Header.Get("Origin") != "" {
					t.Fatal("wrong method/origin")
				}
				if kind == "cancel" {
					<-r.Context().Done()
					return nil, r.Context().Err()
				}
				response, _ := normal.Do(r)
				switch kind {
				case "status":
					response.StatusCode = 302
				case "refused":
					response.StatusCode = http.StatusForbidden
				case "type":
					response.Header.Set("Content-Type", "text/html")
				case "schema":
					response.Header.Set("X-Mercury-Dashboard-Schema", "2")
				}
				return response, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			if kind == "cancel" {
				cancel()
			}
			defer cancel()
			got := c.Read(ctx)
			if kind == "refused" {
				if got.State != "permission_required" {
					t.Fatal(got.State)
				}
			} else if got.State != "invalid" && got.State != "offline" {
				t.Fatal(got.State)
			}
		})
	}
	c := New("http://localhost:8099/api/v1/dashboard")
	hc := c.HTTP.(*http.Client)
	if hc.Timeout != 2*time.Second || hc.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("unbounded/redirect transport")
	}
	transport := hc.Transport.(*http.Transport)
	if transport.Proxy != nil {
		t.Fatal("environment proxy enabled")
	}
	if !transport.DisableKeepAlives {
		t.Fatal("Mercury reads must not retain idle connections")
	}
}
func TestWrappedKernelPermissionIsActionableAndDoesNotLeakEndpoint(t *testing.T) {
	c := fixtureClient(fixtureBodies["live"])
	c.HTTP = doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: "http://private.invalid/blocked-monitor", Err: &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: syscall.EPERM}}}
	})
	read := c.Read(context.Background())
	if read.State != "permission_required" || !strings.Contains(read.Message, "not permitted") || strings.Contains(read.Message, "private.invalid") || read.Snapshot != nil {
		t.Fatalf("%+v", read)
	}
}

func TestPrivateAddressBoundary(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "::1", "10.1.2.3", "192.168.1.2", "169.254.1.2", "100.64.0.1", "fd00::1"} {
		if !privateIP(net.ParseIP(address)) {
			t.Fatal(address)
		}
	}
	for _, address := range []string{"8.8.8.8", "100.63.0.1", "100.128.0.1", "2001:4860::1"} {
		if privateIP(net.ParseIP(address)) {
			t.Fatal(address)
		}
	}
}

//go:embed testdata/samples.json
var fixtureJSON string
