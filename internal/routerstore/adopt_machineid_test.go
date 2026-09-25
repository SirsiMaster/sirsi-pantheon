package routerstore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/machineid"
)

// ADR-067 (rs-42) verification contract §6. The load-bearing negative control is
// the pre-existing TestThreadAuthorityIsHostScoped, which must keep passing
// UNCHANGED — an attacker with its own valid identity, but no recorded alias to
// the victim, is still refused. These tests cover the adoption verb itself, the
// resolver, and the ONE new thing threadAuthority now allows: a reclaim across
// two host strings a live token has tied to the same machine id.

func machineToken(t *testing.T, backend Store, host string) {
	t.Helper()
	if _, _, err := backend.MintHostToken(host, "test"); err != nil {
		t.Fatalf("mint token for %q: %v", host, err)
	}
}

func machineIDOf(t *testing.T, backend Store, host string) string {
	t.Helper()
	toks, err := backend.ListHostTokens()
	if err != nil {
		t.Fatal(err)
	}
	for _, tk := range toks {
		if tk.Host == host && tk.Revoked == "" {
			return tk.MachineID
		}
	}
	t.Fatalf("no live token for %q", host)
	return ""
}

func TestAdoptTokenMachineID_Lifecycle(t *testing.T) {
	backend := newDst(t)
	machineToken(t, backend, "Mac")

	if err := backend.AdoptTokenMachineID("Mac", "MID-1"); err != nil {
		t.Fatalf("first adoption must succeed: %v", err)
	}
	if got := machineIDOf(t, backend, "Mac"); got != "MID-1" {
		t.Fatalf("token machine id = %q, want MID-1", got)
	}
	// Idempotent: re-adopting the SAME id is a no-op success.
	if err := backend.AdoptTokenMachineID("Mac", "MID-1"); err != nil {
		t.Fatalf("re-adopting the same id must be idempotent: %v", err)
	}
	// One-way: a DIFFERENT id on an already-adopted token is refused.
	if err := backend.AdoptTokenMachineID("Mac", "MID-2"); !errors.Is(err, ErrMachineIDAdopted) {
		t.Fatalf("re-point to a new id must be ErrMachineIDAdopted, got %v", err)
	}
	if got := machineIDOf(t, backend, "Mac"); got != "MID-1" {
		t.Fatalf("a refused re-point must not mutate: id = %q", got)
	}
}

func TestAdoptTokenMachineID_Exclusivity(t *testing.T) {
	backend := newDst(t)
	machineToken(t, backend, "Mac")
	machineToken(t, backend, "M5")

	if err := backend.AdoptTokenMachineID("Mac", "MID-1"); err != nil {
		t.Fatal(err)
	}
	// A DIFFERENT live token cannot claim the same machine id (the §3.2 cap:
	// this is what makes the worst case a reversible DoS, not impersonation).
	if err := backend.AdoptTokenMachineID("M5", "MID-1"); !errors.Is(err, ErrMachineIDClaimed) {
		t.Fatalf("second live token claiming MID-1 must be ErrMachineIDClaimed, got %v", err)
	}
	// Revoking the holder FREES the id (the revoked!='' predicate on the index).
	toks, _ := backend.ListHostTokens()
	var macID string
	for _, tk := range toks {
		if tk.Host == "Mac" {
			macID = tk.ID
		}
	}
	if err := backend.RevokeHostToken(macID); err != nil {
		t.Fatal(err)
	}
	if err := backend.AdoptTokenMachineID("M5", "MID-1"); err != nil {
		t.Fatalf("after revoking the holder, MID-1 must be free: %v", err)
	}
}

func TestAdoptTokenMachineID_NoToken(t *testing.T) {
	backend := newDst(t)
	if err := backend.AdoptTokenMachineID("ghost", "MID-1"); !errors.Is(err, ErrNoTokenForHost) {
		t.Fatalf("adopting with no live token must be ErrNoTokenForHost, got %v", err)
	}
}

func TestHostIdentityResolution(t *testing.T) {
	backend := newDst(t)
	machineToken(t, backend, "Mac")
	if err := backend.AdoptTokenMachineID("Mac", "MID-1"); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ in, want string }{
		{"Mac", "MID-1"},   // a host resolves to its adopted id
		{"MID-1", "MID-1"}, // the id resolves to itself (a live token holds it)
		{"nope", "nope"},   // an unknown identity is its own canon
		{"", ""},           // empty stays empty
	}
	for _, c := range cases {
		got, err := backend.HostIdentity(c.in)
		if err != nil {
			t.Fatalf("HostIdentity(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("HostIdentity(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The positive migration case at the server layer: a thread bound to an OLD host
// string is reclaimable by a session whose identity a live token has tied to the
// same machine id — and a session with no such alias is still refused.
func TestThreadAuthorityBridgesAdoptedIdentity(t *testing.T) {
	restore := machineid.GetProbe()
	t.Cleanup(func() { machineid.SetProbe(restore) })
	t.Setenv(EnvUseMachineIDHost, "1") // clients claim machineid.MachineID() as their host

	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	later := now.UTC().Format(time.RFC3339Nano)

	// The alias, created as DATA by an authenticated act: the M1's live token
	// (host "MacBookPro", the drifted old name) adopts the stable id.
	machineToken(t, backend, "MacBookPro")
	if err := backend.AdoptTokenMachineID("MacBookPro", "MID-M1"); err != nil {
		t.Fatal(err)
	}

	// A thread orphaned under the old host string.
	register(t, backend, "thr-ra", "ra", "MacBookPro", "active", now.Add(-time.Minute))

	// A session that resolves to the same machine id reclaims it — the write is
	// applied (fence accepted), and the stored host string is PRESERVED (it still
	// resolves to the machine; migrating the string is ADR-067 §5, not this verb).
	machineid.SetProbe(func() string { return "MID-M1" })
	applied, err := client("ra", "thr-ra").UpsertThreadCAS(ThreadRecord{
		ThreadID: "thr-ra", Agent: "ra", Status: "active", LastSeenAt: later, Payload: []byte(`{"v":1}`),
	})
	if err != nil {
		t.Fatalf("a session adopted to the record's machine id must reclaim it: %v", err)
	}
	if !applied {
		t.Fatal("the store fence must ACCEPT an aliased reclaim (applied=false means the host fence still blocked it)")
	}
	if b, _ := backend.ThreadBinding("thr-ra"); b.Host != "MacBookPro" {
		t.Fatalf("reclaim preserves the existing (aliased) host string, got %q", b.Host)
	}

	// A session with NO alias to that machine is still refused — the negative
	// control for the bridge itself (not just the pre-existing host-scoped test).
	register(t, backend, "thr-victim", "ra", "MacBookPro", "active", now.Add(-time.Minute))
	machineid.SetProbe(func() string { return "MID-EVIL" })
	if _, err := client("ra", "thr-victim").UpsertThreadCAS(ThreadRecord{
		ThreadID: "thr-victim", Agent: "ra", Status: "active", LastSeenAt: later, Payload: []byte(`{}`),
	}); !errors.Is(err, ErrThreadAuthority) {
		t.Fatalf("a session with no adopted alias must be refused, got %v", err)
	}
}

// The verb injects the authenticated host: a client cannot adopt onto a token
// it did not authenticate as (ADR-067 §3.1). Driven over the wire.
func TestAdoptTokenMachineID_ServerInjectsAuthenticatedHost(t *testing.T) {
	restore := machineid.GetProbe()
	t.Cleanup(func() { machineid.SetProbe(restore) })
	machineid.SetProbe(func() string { return "MID-CALLER" })
	t.Setenv(EnvUseMachineIDHost, "1")

	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	machineToken(t, backend, "MID-CALLER")

	// The client passes a BOGUS host; the server must ignore it and adopt onto
	// sess.Host ("MID-CALLER"). If the bogus arg were honored the store would
	// return ErrNoTokenForHost (no token for "attacker-host").
	if err := client("ra", "").AdoptTokenMachineID("attacker-host", "MID-ADOPTED"); err != nil {
		t.Fatalf("adopt must inject sess.Host and succeed: %v", err)
	}
	if got := machineIDOf(t, backend, "MID-CALLER"); got != "MID-ADOPTED" {
		t.Fatalf("adopted id on caller token = %q, want MID-ADOPTED", got)
	}
}
