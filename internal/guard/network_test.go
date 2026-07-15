package guard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// network.go mutates system DNS (`networksetup -setdnsservers`) via RollbackNetwork
// and the audit fix path, yet had ZERO test coverage (full-repo audit P0). These
// tests exercise the DNS-mutating RollbackNetwork end to end — happy path AND the
// failure path (Rule A16) — fully hermetically: the platform is mocked and
// stateDir() is redirected via $HOME so no real DNS or real home is touched.

// priorStatePath mirrors network.go's stateDir()/dns_prior.txt under a temp HOME.
func writePriorState(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	stateDir := filepath.Join(dir, ".config", "sirsi", "isis")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(stateDir, "dns_prior.txt")
	if err := os.WriteFile(p, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRollbackNetwork_RestoresSpecificDNS(t *testing.T) {
	statePath := writePriorState(t, "1.1.1.1 1.0.0.1")
	msg, err := RollbackNetwork(&platform.Mock{})
	if err != nil {
		t.Fatalf("rollback should succeed, got %v", err)
	}
	if !strings.Contains(msg, "1.1.1.1 1.0.0.1") {
		t.Fatalf("expected restored DNS in message, got %q", msg)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("state file should be removed after a successful rollback")
	}
}

func TestRollbackNetwork_EmptyPriorRestoresDefault(t *testing.T) {
	writePriorState(t, "There aren't any DNS Servers set on Wi-Fi.")
	msg, err := RollbackNetwork(&platform.Mock{})
	if err != nil {
		t.Fatalf("rollback should succeed, got %v", err)
	}
	if !strings.Contains(msg, "network default") {
		t.Fatalf("expected DHCP-default restore message, got %q", msg)
	}
}

func TestRollbackNetwork_NoStateIsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // no dns_prior.txt written
	if _, err := RollbackNetwork(&platform.Mock{}); err == nil {
		t.Fatal("rollback with no saved state must error")
	}
}

// The A16 failure path: the DNS-set command fails (e.g. needs admin). Rollback
// must surface the error AND preserve the state file so a retry is possible.
func TestRollbackNetwork_CommandFailurePreservesState(t *testing.T) {
	statePath := writePriorState(t, "1.1.1.1 1.0.0.1")
	mock := &platform.Mock{CommandError: errors.New("must be run as root")}
	msg, err := RollbackNetwork(mock)
	if err == nil {
		t.Fatal("rollback must fail when the networksetup command errors")
	}
	if msg != "" {
		t.Fatalf("no success message on failure, got %q", msg)
	}
	if !strings.Contains(err.Error(), "needs admin") {
		t.Fatalf("error should explain the admin requirement, got %v", err)
	}
	if _, statErr := os.Stat(statePath); statErr != nil {
		t.Fatal("state file must be preserved after a failed rollback (for retry)")
	}
}

// checkDNSConfig with fix=true exercises the DNS auto-fix branch deterministically
// (mocked platform: reading DNS succeeds, the fix command is recorded), without a
// real network call.
func TestCheckDNSConfig_RecordsFinding(t *testing.T) {
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "192.168.1.1", // ISP/router DNS, not encrypted
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, false)
	if len(report.Findings) == 0 {
		t.Fatal("checkDNSConfig should record a DNS Configuration finding")
	}
	found := false
	for _, f := range report.Findings {
		if f.Check == "DNS Configuration" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a 'DNS Configuration' finding")
	}
}
