package routerstore

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestSSACloudExecutorHostUsesExactScopedLogicalIdentity(t *testing.T) {
	t.Setenv(EnvUseMachineIDHost, "")
	t.Setenv(EnvCloudHost, "cloud-host")
	t.Setenv("SIRSI_AGENT_ID", "sirsi-software-admin")
	t.Setenv("SIRSI_THREAD_ID", "")
	f := newScopedFixture(t, []string{"sirsi-software-admin"})
	rs := NewRemoteStore(f.url, f.token)
	rs.sessionDir = ""
	if rs.host != f.host {
		t.Fatal("cloud host was not retained")
	}
	if _, err := rs.Inbox("sirsi-software-admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := rs.Inbox("ra"); err == nil {
		t.Fatal("foreign inbox accepted")
	}
	t.Setenv(EnvCloudHost, "cloud-foreign")
	foreign := NewRemoteStore(f.url, f.token)
	foreign.sessionDir = ""
	if _, err := foreign.Inbox("sirsi-software-admin"); err == nil || !strings.Contains(err.Error(), ErrHostMismatch.Error()) {
		t.Fatalf("foreign host not refused: %v", err)
	}
}

func TestSSACloudExecutorHostInvalidOrConflictingFailsBeforeNetwork(t *testing.T) {
	for _, host := range []string{"Mac", "cloud-../owner", "cloud-ra/foreign", "cloud-"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv(EnvUseMachineIDHost, "")
			t.Setenv(EnvCloudHost, host)
			rs := NewRemoteStore("https://unreachable.invalid", "secret")
			if _, err := rs.Inbox("sirsi-software-admin"); err == nil || !strings.Contains(err.Error(), "cloud executor host") {
				t.Fatalf("not refused before transport: %v", err)
			}
		})
	}
	t.Setenv(EnvUseMachineIDHost, "1")
	t.Setenv(EnvCloudHost, "cloud-ssa")
	rs := NewRemoteStore("https://unreachable.invalid", "secret")
	if rs.identityErr == nil {
		t.Fatal("cloud logical host conflated with physical machine identity")
	}
}

type cloudHostRejectTransport struct{ t *testing.T }

func (r cloudHostRejectTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Fatal("invalid identity reached HTTP")
	return nil, nil
}
func TestCloudHostInvalidNeverReachesHTTP(t *testing.T) {
	for _, host := range []string{"Mac", "cloud-../owner", "cloud-ra/foreign", "cloud-", "cloud-UPPER", "cloud-a\nheader", "cloud-" + strings.Repeat("a", 122)} {
		t.Run(host, func(t *testing.T) {
			t.Setenv(EnvUseMachineIDHost, "")
			t.Setenv(EnvCloudHost, host)
			r := NewRemoteStore("https://example.invalid", "test-only")
			r.sessionDir = ""
			r.client.Transport = cloudHostRejectTransport{t}
			if e := r.do(context.Background(), "MintSession", nil, nil); e == nil {
				t.Fatal("invalid identity accepted")
			}
		})
	}
}
func TestCloudHostPhysicalDefaultAndGrammar(t *testing.T) {
	t.Setenv(EnvUseMachineIDHost, "")
	t.Setenv(EnvCloudHost, "")
	r := NewRemoteStore("https://example.invalid", "")
	h, _ := os.Hostname()
	if r.host != h || r.identityErr != nil {
		t.Fatal("physical default changed")
	}
	for _, h := range []string{"cloud-a", "cloud-ra-20261012", "cloud-" + strings.Repeat("a", 121)} {
		if !validCloudExecutorHost(h) {
			t.Fatal("valid host refused")
		}
	}
	t.Setenv(EnvCloudHost, " cloud-ra-20261012 ")
	r = NewRemoteStore("https://example.invalid", "")
	if r.host != "cloud-ra-20261012" || r.identityErr != nil {
		t.Fatal("trimmed logical identity lost")
	}
}
