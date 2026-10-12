package routerstore

import (
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
