package guard

// coverage_lift_test.go — deterministic coverage for the guard package's
// previously untested paths (Ma'at Tier A ≥80%). Every test here is hermetic:
// platforms are mocked (platform.Mock or the failing wrapper below), network
// probes are swapped via the Rule-A16 seams (dnsReachableFn / dnsResolvesFn /
// tlsAuditAddr / hapiVMStatFn / hapiPsFn), $HOME is redirected to t.TempDir()
// wherever a function writes state, and signal paths use only the A1-gate
// refusals or a PID above the macOS PID ceiling (99999999 can never exist).
// No t.Parallel anywhere: these tests swap package-level seams (PRs #129/#131).

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// bogusPID is above the macOS PID ceiling (PID_MAX 99998), so it can NEVER name
// a real process — signaling it deterministically fails with ESRCH.
const bogusPID = 99999999

// failingCmdPlatform wraps platform.Mock so a test can fail SPECIFIC commands
// while others succeed (plain Mock.CommandError fails every command).
type failingCmdPlatform struct {
	*platform.Mock
	failPrefixes []string
}

func (f *failingCmdPlatform) Command(name string, args ...string) ([]byte, error) {
	full := name + " " + strings.Join(args, " ")
	for _, p := range f.failPrefixes {
		if strings.HasPrefix(full, p) {
			return nil, errors.New("mock failure: " + p)
		}
	}
	return f.Mock.Command(name, args...)
}

// swapDNSProbes swaps the network probe seams and restores them on cleanup.
func swapDNSProbes(t *testing.T, reachable, resolves bool) {
	t.Helper()
	origReach, origResolve := dnsReachableFn, dnsResolvesFn
	t.Cleanup(func() { dnsReachableFn, dnsResolvesFn = origReach, origResolve })
	dnsReachableFn = func(platform.Platform, string) bool { return reachable }
	dnsResolvesFn = func() bool { return resolves }
}

func findingByCheck(t *testing.T, findings []DiagnosticFinding, check string) DiagnosticFinding {
	t.Helper()
	for _, f := range findings {
		if f.Check == check {
			return f
		}
	}
	t.Fatalf("no %q finding in %+v", check, findings)
	return DiagnosticFinding{}
}

// ─── network.go: saveNetworkState ───────────────────────────────────────────

func TestSaveNetworkState_WritesPriorDNS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "8.8.8.8\n",
	}}
	saveNetworkState(mock)
	b, err := os.ReadFile(filepath.Join(stateDir(), "dns_prior.txt"))
	if err != nil {
		t.Fatalf("state file should exist: %v", err)
	}
	if got := string(b); got != "8.8.8.8" {
		t.Fatalf("want trimmed DNS state, got %q", got)
	}
}

func TestSaveNetworkState_CommandErrorWritesNothing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveNetworkState(&platform.Mock{CommandError: errors.New("boom")})
	if _, err := os.Stat(filepath.Join(stateDir(), "dns_prior.txt")); !os.IsNotExist(err) {
		t.Fatal("no state file should be written when reading DNS fails")
	}
}

// ─── network.go: checkDNSConfig branches ─────────────────────────────────────

func TestCheckDNSConfig_ReadFailureIsWarn(t *testing.T) {
	report := &NetworkReport{}
	checkDNSConfig(&platform.Mock{CommandError: errors.New("boom")}, report, false)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityWarn || !strings.Contains(f.Message, "Could not read") {
		t.Fatalf("want warn/could-not-read, got %+v", f)
	}
}

func TestCheckDNSConfig_NoCustomDNSNoFixIsCritical(t *testing.T) {
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "There aren't any DNS Servers set on Wi-Fi.",
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, false)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityCritical {
		t.Fatalf("ISP-default DNS must be critical, got %+v", f)
	}
}

func TestCheckDNSConfig_FixSkippedWhenResolverUnreachable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	swapDNSProbes(t, false, true) // 1.1.1.1 unreachable → fix must be skipped
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "There aren't any DNS Servers set on Wi-Fi.",
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, true)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityWarn || !strings.Contains(f.Message, "skipped fix") {
		t.Fatalf("unreachable resolver must skip the fix, got %+v", f)
	}
}

func TestCheckDNSConfig_FixAppliedAndVerified(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	swapDNSProbes(t, true, true) // reachable + resolution verifies on first try
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "There aren't any DNS Servers set on Wi-Fi.",
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, true)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityOK || !strings.Contains(f.Message, "FIXED") {
		t.Fatalf("verified fix must be OK/FIXED, got %+v", f)
	}
}

func TestCheckDNSConfig_FixCommandFailureExplainsAdmin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	swapDNSProbes(t, true, true)
	p := &failingCmdPlatform{
		Mock: &platform.Mock{CommandResults: map[string]string{
			"networksetup -getdnsservers Wi-Fi": "There aren't any DNS Servers set on Wi-Fi.",
		}},
		failPrefixes: []string{"networksetup -setdnsservers"},
	}
	report := &NetworkReport{}
	checkDNSConfig(p, report, true)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityCritical || !strings.Contains(f.Detail, "needs admin") {
		t.Fatalf("failed fix must stay critical + explain admin, got %+v", f)
	}
}

func TestCheckDNSConfig_EncryptedReachableIsOK(t *testing.T) {
	swapDNSProbes(t, true, true)
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "1.1.1.1\n1.0.0.1",
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, false)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityOK || !strings.Contains(f.Message, "Cloudflare") {
		t.Fatalf("reachable encrypted DNS must be OK, got %+v", f)
	}
}

func TestCheckDNSConfig_EncryptedUnreachableNoFixIsCritical(t *testing.T) {
	swapDNSProbes(t, false, true)
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "9.9.9.9",
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, false)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityCritical || !strings.Contains(f.Message, "UNREACHABLE") {
		t.Fatalf("blocked encrypted DNS must be critical, got %+v", f)
	}
}

func TestCheckDNSConfig_EncryptedUnreachableFixRevertsToDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	swapDNSProbes(t, false, true) // blocked provider, but network DNS resolves after revert
	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "8.8.8.8",
	}}
	report := &NetworkReport{}
	checkDNSConfig(mock, report, true)
	f := findingByCheck(t, report.Findings, "DNS Configuration")
	if f.Severity != SeverityWarn || !strings.Contains(f.Message, "reverted to network default") {
		t.Fatalf("fix on blocked provider must revert to default, got %+v", f)
	}
}

// ─── network.go: verifyDNSOrRollback ─────────────────────────────────────────

func TestVerifyDNSOrRollback_ResolvesImmediately(t *testing.T) {
	swapDNSProbes(t, true, true)
	if !verifyDNSOrRollback(&platform.Mock{}, 1, 0) {
		t.Fatal("resolving DNS must verify true")
	}
}

func TestVerifyDNSOrRollback_NoSavedStateFallsBackToDHCP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	swapDNSProbes(t, true, false)
	if verifyDNSOrRollback(&platform.Mock{}, 2, 0) {
		t.Fatal("non-resolving DNS must report rollback (false)")
	}
}

func TestVerifyDNSOrRollback_RestoresSavedServers(t *testing.T) {
	statePath := writePriorState(t, "1.1.1.1 8.8.8.8")
	swapDNSProbes(t, true, false)
	if verifyDNSOrRollback(&platform.Mock{}, 1, 0) {
		t.Fatal("want rollback (false)")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("state file must be consumed by the rollback")
	}
}

func TestVerifyDNSOrRollback_EmptyPriorRestoresDefault(t *testing.T) {
	writePriorState(t, "There aren't any DNS Servers set on Wi-Fi.")
	swapDNSProbes(t, true, false)
	if verifyDNSOrRollback(&platform.Mock{}, 1, 0) {
		t.Fatal("want rollback (false)")
	}
}

// ─── network.go: checkWiFiSecurity ───────────────────────────────────────────

func TestCheckWiFiSecurity(t *testing.T) {
	wifi := func(ssid string) string { return "Current Wi-Fi Network: " + ssid }
	cases := []struct {
		name     string
		p        platform.Platform
		severity DiagnosticSeverity
		contains string
	}{
		{
			name:     "not connected",
			p:        &platform.Mock{}, // en0+en1 both return empty output
			severity: SeverityInfo,
			contains: "Not connected",
		},
		{
			name: "en1 fallback with WPA3",
			p: &failingCmdPlatform{
				Mock: &platform.Mock{CommandResults: map[string]string{
					"networksetup -getairportnetwork en1":                              wifi("HomeNet"),
					"defaults read /Library/Preferences/com.apple.wifi.known-networks": "SecurityType = WPA3;",
				}},
				failPrefixes: []string{"networksetup -getairportnetwork en0"},
			},
			severity: SeverityOK,
			contains: "WPA3",
		},
		{
			name: "security unknown when plist unreadable",
			p: &failingCmdPlatform{
				Mock: &platform.Mock{CommandResults: map[string]string{
					"networksetup -getairportnetwork en0": wifi("CoffeeShop"),
				}},
				failPrefixes: []string{"defaults read"},
			},
			severity: SeverityWarn,
			contains: "security type unknown",
		},
		{
			name: "WPA2",
			p: &platform.Mock{CommandResults: map[string]string{
				"networksetup -getairportnetwork en0":                              wifi("HomeNet"),
				"defaults read /Library/Preferences/com.apple.wifi.known-networks": "SecurityType = WPA2;",
			}},
			severity: SeverityOK,
			contains: "WPA2",
		},
		{
			name: "open network is critical",
			p: &platform.Mock{CommandResults: map[string]string{
				"networksetup -getairportnetwork en0":                              wifi("FreeAirport"),
				"defaults read /Library/Preferences/com.apple.wifi.known-networks": "SecurityType = None;",
			}},
			severity: SeverityCritical,
			contains: "OPEN network",
		},
		{
			name: "unknown plist content defaults to connected",
			p: &platform.Mock{CommandResults: map[string]string{
				"networksetup -getairportnetwork en0":                              wifi("HomeNet"),
				"defaults read /Library/Preferences/com.apple.wifi.known-networks": "nothing recognizable",
			}},
			severity: SeverityOK,
			contains: "Connected to",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := &NetworkReport{}
			checkWiFiSecurity(c.p, report)
			f := findingByCheck(t, report.Findings, "WiFi Security")
			if f.Severity != c.severity || !strings.Contains(f.Message, c.contains) {
				t.Fatalf("want %v/%q, got %+v", c.severity, c.contains, f)
			}
		})
	}
}

// ─── network.go: checkCACertificates ─────────────────────────────────────────

func TestCheckCACertificates(t *testing.T) {
	cases := []struct {
		name     string
		p        platform.Platform
		severity DiagnosticSeverity
		contains string
	}{
		{"unreadable store", &platform.Mock{CommandError: errors.New("boom")}, SeverityWarn, "Could not read"},
		{"unusually high", &platform.Mock{CommandResults: map[string]string{
			"security": strings.Repeat("labl\n", 250)}}, SeverityWarn, "unusually high"},
		{"unusually low", &platform.Mock{CommandResults: map[string]string{
			"security": strings.Repeat("labl\n", 5)}}, SeverityWarn, "unusually low"},
		{"normal range", &platform.Mock{CommandResults: map[string]string{
			"security": strings.Repeat("labl\n", 150)}}, SeverityOK, "normal range"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := &NetworkReport{}
			checkCACertificates(c.p, report)
			f := findingByCheck(t, report.Findings, "CA Certificate Audit")
			if f.Severity != c.severity || !strings.Contains(f.Message, c.contains) {
				t.Fatalf("want %v/%q, got %+v", c.severity, c.contains, f)
			}
		})
	}
}

// ─── network.go: checkVPNStatus ──────────────────────────────────────────────

func TestCheckVPNStatus_CommandErrorAddsNoFinding(t *testing.T) {
	report := &NetworkReport{}
	checkVPNStatus(&platform.Mock{CommandError: errors.New("boom")}, report)
	if len(report.Findings) != 0 {
		t.Fatalf("ifconfig failure must add no finding, got %+v", report.Findings)
	}
}

func TestCheckVPNStatus_ActiveTunnels(t *testing.T) {
	utun := func(n string, ip string) string {
		return n + ": flags=8051<UP,POINTOPOINT,RUNNING,MULTICAST> mtu 1380\n\tinet " + ip + " --> " + ip + " netmask 0xffffffff"
	}
	ifconfig := strings.Join([]string{
		"lo0: flags=8049<UP,LOOPBACK,RUNNING,MULTICAST> mtu 16384\n\tinet 127.0.0.1 netmask 0xff000000",
		utun("utun0", "10.0.0.1"),
		utun("utun1", "10.0.0.2"),
		utun("utun2", "10.0.0.3"),
	}, "\n\n")
	report := &NetworkReport{}
	checkVPNStatus(&platform.Mock{CommandResults: map[string]string{"ifconfig": ifconfig}}, report)
	f := findingByCheck(t, report.Findings, "VPN Status")
	if f.Severity != SeverityOK || !strings.Contains(f.Message, "VPN likely active") {
		t.Fatalf("3 addressed tunnels must read VPN-active, got %+v", f)
	}
	if !strings.Contains(f.Detail, "utun2") {
		t.Fatalf("detail must list the tunnel interfaces, got %q", f.Detail)
	}
}

func TestCheckVPNStatus_NoVPNIsWarn(t *testing.T) {
	report := &NetworkReport{}
	checkVPNStatus(&platform.Mock{CommandResults: map[string]string{"ifconfig": "en0: flags=UP\n\tinet 192.168.1.5"}}, report)
	f := findingByCheck(t, report.Findings, "VPN Status")
	if f.Severity != SeverityWarn || !strings.Contains(f.Message, "No VPN detected") {
		t.Fatalf("no tunnels must warn, got %+v", f)
	}
}

// ─── network.go: checkFirewall ───────────────────────────────────────────────

func TestCheckFirewall(t *testing.T) {
	const fwCmd = "/usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate"
	cases := []struct {
		name     string
		p        platform.Platform
		fix      bool
		severity DiagnosticSeverity
		contains string
	}{
		{"unreadable", &platform.Mock{CommandError: errors.New("boom")}, false, SeverityWarn, "Could not read"},
		{"disabled no fix", &platform.Mock{CommandResults: map[string]string{
			fwCmd: "Firewall is disabled. (State = 0)"}}, false, SeverityCritical, "disabled"},
		{"disabled fix succeeds", &platform.Mock{CommandResults: map[string]string{
			fwCmd: "Firewall is disabled. (State = 0)"}}, true, SeverityOK, "FIXED"},
		{"enabled", &platform.Mock{CommandResults: map[string]string{
			fwCmd: "Firewall is enabled. (State = 1)"}}, false, SeverityOK, "enabled"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := &NetworkReport{}
			checkFirewall(c.p, report, c.fix)
			f := findingByCheck(t, report.Findings, "macOS Firewall")
			if f.Severity != c.severity || !strings.Contains(f.Message, c.contains) {
				t.Fatalf("want %v/%q, got %+v", c.severity, c.contains, f)
			}
		})
	}
}

func TestCheckFirewall_FixFailureExplainsAdmin(t *testing.T) {
	p := &failingCmdPlatform{
		Mock: &platform.Mock{CommandResults: map[string]string{
			"/usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate": "Firewall is disabled. (State = 0)",
		}},
		failPrefixes: []string{"sudo"},
	}
	report := &NetworkReport{}
	checkFirewall(p, report, true)
	f := findingByCheck(t, report.Findings, "macOS Firewall")
	if f.Severity != SeverityCritical || !strings.Contains(f.Detail, "needs admin") {
		t.Fatalf("failed firewall fix must stay critical + explain admin, got %+v", f)
	}
}

// ─── network.go: checkTLSConnection + NetworkAuditWith (local addr, no live net) ──

func TestCheckTLSConnection_DialFailureIsCritical(t *testing.T) {
	orig := tlsAuditAddr
	t.Cleanup(func() { tlsAuditAddr = orig })
	tlsAuditAddr = "127.0.0.1:0" // port 0 can never be connected to — fails instantly
	report := &NetworkReport{}
	checkTLSConnection(report)
	f := findingByCheck(t, report.Findings, "TLS to Anthropic API")
	if f.Severity != SeverityCritical || !strings.Contains(f.Message, "TLS 1.3 connection failed") {
		t.Fatalf("dial failure must be critical, got %+v", f)
	}
}

func TestNetworkAuditWith_RunsAllChecksHermetically(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	orig := tlsAuditAddr
	t.Cleanup(func() { tlsAuditAddr = orig })
	tlsAuditAddr = "127.0.0.1:0"
	swapDNSProbes(t, true, true)

	mock := &platform.Mock{CommandResults: map[string]string{
		"networksetup -getdnsservers Wi-Fi": "1.1.1.1",
		"security":                          strings.Repeat("labl\n", 150),
		"ifconfig":                          "en0: flags=UP\n\tinet 192.168.1.5",
		"/usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate": "Firewall is enabled. (State = 1)",
	}}
	report, err := NetworkAuditWith(mock, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 6 {
		t.Fatalf("want 6 findings (DNS, WiFi, TLS, CA, VPN, firewall), got %d: %+v", len(report.Findings), report.Findings)
	}
	if report.Score < 0 || report.Score > 100 {
		t.Fatalf("score out of range: %d", report.Score)
	}
	if report.Duration == "" {
		t.Fatal("duration must be recorded")
	}
}

// ─── hapi.go: MemTier.String, SampleMemory, hapiCanAct ───────────────────────

func TestMemTierString(t *testing.T) {
	cases := map[MemTier]string{
		TierOK:        "ok",
		TierWarn:      "warn",
		TierCritical:  "critical",
		TierEmergency: "emergency",
		MemTier(99):   "ok",
	}
	for tier, want := range cases {
		if got := tier.String(); got != want {
			t.Errorf("MemTier(%d).String() = %q, want %q", tier, got, want)
		}
	}
}

func TestSampleMemoryUsesInjectedSampler(t *testing.T) {
	orig := hapiSampleFn
	t.Cleanup(func() { setHapiSampleFn(orig) })
	setHapiSampleFn(func() (MemSample, error) { return sampleAt(42), nil })
	s, err := SampleMemory()
	if err != nil {
		t.Fatal(err)
	}
	if s.FreePercent != 42 {
		t.Fatalf("want the injected sample, got %+v", s)
	}
}

func TestHapiCanAct(t *testing.T) {
	cases := []struct {
		name    string
		pid     int
		proc    string
		refusal string // "" = allowed
	}{
		{"system pid", 1, "launchd", "system"},
		{"self", os.Getpid(), "python", "self"},
		{"protected", bogusPID, "WindowServer", "protected"},
		{"live agent", bogusPID, "claude-code", "agent"},
		{"governable", bogusPID, "python3", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := hapiCanAct(c.pid, c.proc)
			if c.refusal == "" {
				if err != nil {
					t.Fatalf("want allowed, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.refusal) {
				t.Fatalf("want refusal containing %q, got %v", c.refusal, err)
			}
		})
	}
}

// ─── hapi_signals: liveness probe + gated interventions (never a real target) ──

func TestHapiProcessAlive(t *testing.T) {
	if hapiProcessAlive(0) || hapiProcessAlive(-5) {
		t.Fatal("non-positive PIDs are never alive")
	}
	if !hapiProcessAlive(os.Getpid()) {
		t.Fatal("our own PID must be alive")
	}
	if hapiProcessAlive(bogusPID) {
		t.Fatal("a PID above the macOS ceiling can never be alive")
	}
}

func TestHapiSuspendRefusesGateAndMissingProcess(t *testing.T) {
	if err := hapiSuspend(1, "launchd"); err == nil {
		t.Fatal("suspend must refuse the A1 gate")
	}
	if err := hapiSuspend(bogusPID, "definitely-not-real"); err == nil {
		t.Fatal("suspending a nonexistent PID must error (ESRCH)")
	}
}

func TestHapiKillRefusesGateAndMissingProcess(t *testing.T) {
	if err := hapiKill(os.Getpid(), "sirsi-test"); err == nil {
		t.Fatal("kill must refuse self")
	}
	if err := hapiKill(bogusPID, "definitely-not-real"); err == nil {
		t.Fatal("killing a nonexistent PID must error (ESRCH)")
	}
}

func TestHapiResumeMissingProcessErrors(t *testing.T) {
	if err := hapiResume(bogusPID); err == nil {
		t.Fatal("resuming a nonexistent PID must error (ESRCH)")
	}
}

// ─── hapi.go: governed registry (hermetic under $HOME=t.TempDir()) ────────────

func TestGovernedRegistryLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	self := os.Getpid()

	if got := hapiGovernedPIDs(); len(got) != 0 {
		t.Fatalf("fresh registry must be empty, got %v", got)
	}
	if err := HapiRegisterGoverned(self, "sirsi gemma broker"); err != nil {
		t.Fatal(err)
	}
	got := hapiGovernedPIDs()
	if got[self] != "sirsi gemma broker" {
		t.Fatalf("want self registered with name, got %v", got)
	}
	// A dead PID is pruned on read (the PID-alive lesson).
	if err := HapiRegisterGoverned(bogusPID, "ghost"); err != nil {
		t.Fatal(err)
	}
	got = hapiGovernedPIDs()
	if _, ok := got[bogusPID]; ok {
		t.Fatal("dead PID must be pruned on read")
	}
	if err := HapiUnregisterGoverned(self); err != nil {
		t.Fatal(err)
	}
	if got := hapiGovernedPIDs(); len(got) != 0 {
		t.Fatalf("registry must be empty after unregister, got %v", got)
	}
}

func TestGovernedRegistryToleratesMalformedLines(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	self := os.Getpid()
	if err := os.MkdirAll(filepath.Dir(hapiGovernedPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "\nnot-a-pid gopls\n" + // Atoi failure → skipped
		"   \n" + // blank → skipped
		"999999999 dead\n" + // dead → pruned
		strconv.Itoa(self) + "\n" // pid-only line → empty name
	if err := os.WriteFile(hapiGovernedPath(), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got := hapiGovernedPIDs()
	if len(got) != 1 {
		t.Fatalf("only the live pid-only line survives, got %v", got)
	}
	if name, ok := got[self]; !ok || name != "" {
		t.Fatalf("pid-only line must register with empty name, got %v", got)
	}
}

// ─── hapi.go: vm_stat / ps parsers via the command seams ─────────────────────

func TestHapiFreeRAMBytesParsesVMStat(t *testing.T) {
	orig := hapiVMStatFn
	t.Cleanup(func() { hapiVMStatFn = orig })
	hapiVMStatFn = func() ([]byte, error) {
		return []byte(`Mach Virtual Memory Statistics: (page size of 4096 bytes)
Pages free:                              100000.
Pages active:                            200000.
Pages inactive:                           50000.
Pages speculative:                        25000.
Pages wired down:                         90000.
`), nil
	}
	want := int64(100000+50000+25000) * 4096
	if got := hapiFreeRAMBytes(); got != want {
		t.Fatalf("want %d reclaimable bytes, got %d", want, got)
	}
}

func TestHapiFreeRAMBytesDefaultPageSize(t *testing.T) {
	orig := hapiVMStatFn
	t.Cleanup(func() { hapiVMStatFn = orig })
	hapiVMStatFn = func() ([]byte, error) {
		return []byte("Pages free: 10.\nPages inactive: 5.\nPages speculative: 1.\n"), nil
	}
	if got := hapiFreeRAMBytes(); got != 16*16384 {
		t.Fatalf("want M-series default page size applied, got %d", got)
	}
}

func TestHapiFreeRAMBytesIncludesActiveFileBackedSnapshotCache(t *testing.T) {
	orig := hapiVMStatFn
	t.Cleanup(func() { hapiVMStatFn = orig })
	hapiVMStatFn = func() ([]byte, error) {
		return []byte(`Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                              500000.
Pages inactive:                          600000.
Pages speculative:                        10000.
File-backed pages:                      1800000.
`), nil
	}
	// Queue estimate is 1,110,000 pages. The full-snapshot read made 1,800,000
	// pages file-backed and reclaimable; Apple's memorystatus approximation is
	// free + file-backed = 2,300,000 pages.
	want := int64(500000+1800000) * 16384
	if got := hapiFreeRAMBytes(); got != want {
		t.Fatalf("want %d reclaimable bytes including active file cache, got %d", want, got)
	}
}

func TestHapiFreeRAMBytesCommandFailureIsZero(t *testing.T) {
	orig := hapiVMStatFn
	t.Cleanup(func() { hapiVMStatFn = orig })
	hapiVMStatFn = func() ([]byte, error) { return nil, errors.New("boom") }
	if got := hapiFreeRAMBytes(); got != 0 {
		t.Fatalf("vm_stat failure must read as 0 free, got %d", got)
	}
}

func TestHapiTopByRSSParsesSortsAndTruncates(t *testing.T) {
	orig := hapiPsFn
	t.Cleanup(func() { hapiPsFn = orig })
	oldFootprint := getHapiFootprintFn()
	t.Cleanup(func() { setHapiFootprintFn(oldFootprint) })
	setHapiFootprintFn(func(int) (uint64, error) { return 0, errors.New("unavailable") })
	hapiPsFn = func() ([]byte, error) {
		return []byte(`  PID    RSS COMM
    1  10000 /sbin/launchd
  200 500000 /usr/bin/python three words

  bad
  300 100000 short
`), nil
	}
	procs, err := hapiTopByRSS(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(procs) != 2 {
		t.Fatalf("want top-2, got %d: %+v", len(procs), procs)
	}
	if procs[0].PID != 200 || procs[0].RSS != 500000*1024 {
		t.Fatalf("largest RSS first (KB→bytes), got %+v", procs[0])
	}
	if procs[0].Name != "/usr/bin/python three words" {
		t.Fatalf("multi-word command must be rejoined, got %q", procs[0].Name)
	}
	if procs[1].PID != 300 {
		t.Fatalf("second largest is pid 300, got %+v", procs[1])
	}
}

func TestHapiTopByRSSPrefersPhysicalFootprint(t *testing.T) {
	orig := hapiPsFn
	t.Cleanup(func() { hapiPsFn = orig })
	oldFootprint := getHapiFootprintFn()
	t.Cleanup(func() { setHapiFootprintFn(oldFootprint) })
	hapiPsFn = func() ([]byte, error) {
		return []byte("PID RSS COMM\n100 900000 resident-heavy\n200 100000 compressed-heavy\n"), nil
	}
	setHapiFootprintFn(func(pid int) (uint64, error) {
		if pid == 200 {
			return uint64(30 * gb), nil
		}
		return uint64(2 * gb), nil
	})
	procs, err := hapiTopByRSS(2)
	if err != nil {
		t.Fatal(err)
	}
	if procs[0].PID != 200 || procs[0].Footprint != 30*gb {
		t.Fatalf("want compressed-heavy first by footprint, got %+v", procs)
	}
}

func TestHapiTopByRSSCommandFailure(t *testing.T) {
	orig := hapiPsFn
	t.Cleanup(func() { hapiPsFn = orig })
	hapiPsFn = func() ([]byte, error) { return nil, errors.New("boom") }
	if _, err := hapiTopByRSS(5); err == nil {
		t.Fatal("ps failure must surface as error")
	}
}

// ─── hapi.go: MemGovernor.Start lifecycle ────────────────────────────────────

func TestMemGovernorStartTicksAndStops(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	orig := hapiSampleFn
	setHapiSampleFn(func() (MemSample, error) { return sampleAt(40), nil })

	g := DefaultMemGovernor()
	g.Interval = 20 * time.Millisecond
	stop := make(chan struct{})
	passes := make(chan GovernResult, 16)
	done := make(chan struct{})
	go func() {
		g.Start(stop, func(r GovernResult) {
			select {
			case passes <- r:
			default:
			}
		})
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case r := <-passes:
			if r.Tier != TierOK {
				t.Errorf("40%% free must classify OK, got %v", r.Tier)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("governor never ticked")
		}
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("governor did not stop")
	}
	setHapiSampleFn(orig) // restore only after the goroutine exited (A21)
}

func TestMemGovernorStartDefaultsIntervalAndResumesOnShutdown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	origSample := hapiSampleFn
	setHapiSampleFn(func() (MemSample, error) { return sampleAt(40), nil })

	var resumed []int
	origResume := getResumeFn()
	setResumeFn(func(pid int) error { resumed = append(resumed, pid); return nil })

	g := &MemGovernor{suspended: map[int]string{bogusPID: "ghost"}} // Interval unset → default
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { g.Start(stop, nil); close(done) }()
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("governor did not stop")
	}
	setResumeFn(origResume)
	setHapiSampleFn(origSample)

	if g.Interval != 3*time.Second {
		t.Fatalf("zero interval must default to 3s, got %v", g.Interval)
	}
	if len(resumed) != 1 || resumed[0] != bogusPID {
		t.Fatalf("shutdown must resume everything Hapi paused, got %v", resumed)
	}
}

// ─── pressure.go: observedPressure recorded branch ───────────────────────────

func TestObservedPressureReportsRecordedLevel(t *testing.T) {
	prevAuth, prevLvl := lastPressureAuth.Load(), lastPressureLevel.Load()
	t.Cleanup(func() { lastPressureLevel.Store(prevLvl); lastPressureAuth.Store(prevAuth) })

	lastPressureAuth.Store(false)
	if _, ok := observedPressure(); ok {
		t.Fatal("no recorded level must report not-observed")
	}
	lastPressureLevel.Store(int32(PressureWarn))
	lastPressureAuth.Store(true)
	l, ok := observedPressure()
	if !ok || l != PressureWarn {
		t.Fatalf("want recorded warn level, got %v/%v", l, ok)
	}
}

// ─── doctor.go: HealthStatus Icon/Label ──────────────────────────────────────

func TestHealthStatusIconAndLabel(t *testing.T) {
	cases := []struct {
		h     HealthStatus
		icon  string
		label string
	}{
		{HealthRed, "🔴", "Critical — act now"},
		{HealthAmber, "🟡", "Attention"},
		{HealthGreen, "🟢", "Healthy"},
		{HealthStatus("bogus"), "🟢", "Healthy"},
	}
	for _, c := range cases {
		if got := c.h.Icon(); got != c.icon {
			t.Errorf("%q.Icon() = %q, want %q", c.h, got, c.icon)
		}
		if got := c.h.Label(); got != c.label {
			t.Errorf("%q.Label() = %q, want %q", c.h, got, c.label)
		}
	}
}

// ─── small exported wrappers over injectable seams ───────────────────────────

func TestScanOrphansUsesInjectedPs(t *testing.T) {
	orig := orphanPsFn
	t.Cleanup(func() { orphanPsFn = orig })
	orphanPsFn = func() ([]orphanPsEntry, error) {
		return []orphanPsEntry{
			{PID: 500, PPID: 1, RSS: 2048, Name: "playwright run-driver", ElapsedTime: "01:00"},
		}, nil
	}
	report, err := ScanOrphans()
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalOrphans != 1 {
		t.Fatalf("want the injected orphan found, got %+v", report)
	}
}

func TestLocalSnapshotsUsesInjectedLister(t *testing.T) {
	orig := localSnapshotsFn
	t.Cleanup(func() { localSnapshotsFn = orig })
	localSnapshotsFn = func(vol string) []string { return []string{"com.apple.TimeMachine.x." + vol} }
	got := LocalSnapshots("/")
	if len(got) != 1 || got[0] != "com.apple.TimeMachine.x./" {
		t.Fatalf("LocalSnapshots must delegate to the seam, got %v", got)
	}
}

func TestShouldRenice(t *testing.T) {
	cases := []struct {
		target ReniceTarget
		name   string
		want   bool
	}{
		{ReniceTargetLSP, "gopls", true},
		{ReniceTargetLSP, "Safari", false},
		{ReniceTargetAll, "clangd", true},
		{ReniceTarget("bogus"), "gopls", false},
	}
	for _, c := range cases {
		if got := shouldRenice(c.target, c.name); got != c.want {
			t.Errorf("shouldRenice(%q, %q) = %v, want %v", c.target, c.name, got, c.want)
		}
	}
}

func TestFitsDefaultsUnknownModelSize(t *testing.T) {
	roomy := NodeCapacity{TotalRAM: 128 * gb, FreeRAM: 100 * gb}
	if !roomy.Fits(0) {
		t.Fatal("unknown model size on a roomy node must fit (conservative default)")
	}
	tight := NodeCapacity{TotalRAM: 16 * gb, FreeRAM: 4 * gb}
	if tight.Fits(0) {
		t.Fatal("unknown model size on a tight node must refuse")
	}
}
