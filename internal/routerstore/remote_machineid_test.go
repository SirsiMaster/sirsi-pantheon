package routerstore

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/machineid"
)

// Default off: NewRemoteStore claims os.Hostname(), exactly the fleet's
// current, live behavior — a redeploy of this package alone changes nothing
// for anyone until EnvUseMachineIDHost is explicitly set.
func TestNewRemoteStoreDefaultsToHostname(t *testing.T) {
	t.Setenv(EnvUseMachineIDHost, "")
	rs := NewRemoteStore("https://x", "t")
	if rs.host == "" {
		t.Fatal("host must not be empty by default")
	}
	if machineid.LooksLikeMachineID(rs.host) {
		t.Fatalf("default host claim looks like a machine id (%q) — must be the hostname unless opted in", rs.host)
	}
}

// Opted in: claims MachineID() when the platform provides one.
func TestNewRemoteStoreOptInUsesMachineID(t *testing.T) {
	old := machineid.GetProbe()
	machineid.SetProbe(func() string { return "6BA7B810-9DAD-11D1-80B4-00C04FD430C8" })
	defer machineid.SetProbe(old)
	t.Setenv(EnvUseMachineIDHost, "1")
	rs := NewRemoteStore("https://x", "t")
	if rs.host != "6BA7B810-9DAD-11D1-80B4-00C04FD430C8" {
		t.Fatalf("host = %q, want the injected machine id", rs.host)
	}
}

// Opted in but the platform has no machine id: falls back to the hostname
// claim rather than authenticating as "".
func TestNewRemoteStoreOptInFallsBackWhenProbeEmpty(t *testing.T) {
	old := machineid.GetProbe()
	machineid.SetProbe(func() string { return "" })
	defer machineid.SetProbe(old)
	t.Setenv(EnvUseMachineIDHost, "1")
	rs := NewRemoteStore("https://x", "t")
	if rs.host == "" {
		t.Fatal("must fall back to the hostname claim, never authenticate as empty")
	}
}
