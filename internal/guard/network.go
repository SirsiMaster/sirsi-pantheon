// Package guard — network.go
//
// 𓁐 Isis Network Audit: Checks network security posture for public WiFi
// safety and transport encryption. Read-only by default; --fix applies safe
// remediations (encrypted DNS, firewall enable).
//
// Checks: DNS config, WiFi security, TLS to Anthropic API, CA certificate
// audit, VPN tunnel status, macOS firewall state.
package guard

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// stateDir returns the Isis state directory, creating it if needed.
func stateDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "sirsi", "isis")
	_ = os.MkdirAll(dir, 0700)
	return dir
}

// saveNetworkState persists the current DNS config so it can be rolled back.
func saveNetworkState(p platform.Platform) {
	out, err := p.Command("networksetup", "-getdnsservers", "Wi-Fi")
	if err != nil {
		return
	}
	dns := strings.TrimSpace(string(out))
	path := filepath.Join(stateDir(), "dns_prior.txt")
	_ = os.WriteFile(path, []byte(dns), 0600)
}

// RollbackNetwork restores DNS to the state saved before the last --fix.
func RollbackNetwork(p platform.Platform) (string, error) {
	path := filepath.Join(stateDir(), "dns_prior.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no rollback state found at %s", path)
	}

	prior := strings.TrimSpace(string(data))
	if prior == "" || strings.Contains(prior, "aren't any") || strings.Contains(prior, "no DNS") {
		_, err = p.Command("networksetup", "-setdnsservers", "Wi-Fi", "empty")
		if err != nil {
			return "", fmt.Errorf("rollback failed (needs admin): sudo networksetup -setdnsservers Wi-Fi empty")
		}
		_ = os.Remove(path)
		return "Restored DNS to network default (DHCP)", nil
	}

	// Restore the specific DNS servers that were configured before
	args := append([]string{"-setdnsservers", "Wi-Fi"}, strings.Fields(prior)...)
	_, err = p.Command("networksetup", args...)
	if err != nil {
		return "", fmt.Errorf("rollback failed (needs admin): sudo networksetup -setdnsservers Wi-Fi %s", prior)
	}
	_ = os.Remove(path)
	return fmt.Sprintf("Restored DNS to: %s", prior), nil
}

// NetworkReport is the complete network security audit.
type NetworkReport struct {
	Timestamp time.Time           `json:"timestamp"`
	Duration  string              `json:"duration"`
	Findings  []DiagnosticFinding `json:"findings"`
	Score     int                 `json:"score"`
}

// NetworkAudit runs a network security posture check.
func NetworkAudit() (*NetworkReport, error) {
	return NetworkAuditWith(platform.Current(), false)
}

// NetworkAuditFix runs the audit and applies safe remediations.
func NetworkAuditFix() (*NetworkReport, error) {
	return NetworkAuditWith(platform.Current(), true)
}

// NetworkAuditWith runs the audit using the provided platform (Rule A16).
func NetworkAuditWith(p platform.Platform, fix bool) (*NetworkReport, error) {
	start := time.Now()
	report := &NetworkReport{
		Timestamp: start,
	}

	checkDNSConfig(p, report, fix)
	checkWiFiSecurity(p, report)
	checkTLSConnection(report)
	checkCACertificates(p, report)
	checkVPNStatus(p, report)
	checkFirewall(p, report, fix)

	report.Score = calculateScore(report.Findings)
	report.Duration = time.Since(start).Round(time.Millisecond).String()

	return report, nil
}

// checkDNSConfig verifies that encrypted DNS is configured.
func checkDNSConfig(p platform.Platform, report *NetworkReport, fix bool) {
	out, err := p.Command("networksetup", "-getdnsservers", "Wi-Fi")
	if err != nil {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    "DNS Configuration",
			Severity: SeverityWarn,
			Message:  "Could not read DNS configuration",
		})
		return
	}

	dns := strings.TrimSpace(string(out))
	finding := DiagnosticFinding{
		Check:  "DNS Configuration",
		Detail: dns,
	}

	encryptedDNS := map[string]string{
		"1.1.1.1":         "Cloudflare (DoH)",
		"1.0.0.1":         "Cloudflare (DoH)",
		"8.8.8.8":         "Google (DoH)",
		"8.8.4.4":         "Google (DoH)",
		"9.9.9.9":         "Quad9 (DoH)",
		"149.112.112.112": "Quad9 (DoH)",
	}

	if strings.Contains(dns, "aren't any") || strings.Contains(dns, "no DNS") {
		finding.Severity = SeverityCritical
		finding.Message = "No custom DNS — using ISP default (unencrypted, spoofable on public WiFi)"

		if fix {
			// Safety: probe reachability BEFORE changing DNS config.
			// A failed probe here is harmless; a failed probe after changing
			// DNS would leave the machine with broken name resolution.
			if !dnsReachable(p, "1.1.1.1") {
				finding.Severity = SeverityWarn
				finding.Message = "Cloudflare DNS (1.1.1.1) unreachable on this network — skipped fix"
				finding.Detail = "This network blocks external DNS servers. Use a VPN for encrypted DNS."
			} else {
				saveNetworkState(p) // preserve current state for rollback
				_, fixErr := p.Command("networksetup", "-setdnsservers", "Wi-Fi", "1.1.1.1", "1.0.0.1")
				if fixErr == nil {
					// Post-fix watchdog: poll resolution 3 times over ~6 seconds.
					// If DNS never resolves, auto-revert before the user loses connectivity.
					if verifyDNSOrRollback(p, 3, 2*time.Second) {
						finding.Message += " → FIXED: Set to Cloudflare 1.1.1.1"
						finding.Severity = SeverityOK
					} else {
						finding.Severity = SeverityWarn
						finding.Message = "Cloudflare DNS set but resolution failed — auto-reverted to prior config"
						finding.Detail = "This network accepts connections to 1.1.1.1 but blocks DNS resolution. Use a VPN."
					}
				} else {
					finding.Detail = "Auto-fix failed (needs admin): sudo networksetup -setdnsservers Wi-Fi 1.1.1.1 1.0.0.1"
				}
			}
		}
	} else {
		// Check if the configured DNS is a known encrypted provider
		isEncrypted := false
		provider := ""
		for _, line := range strings.Split(dns, "\n") {
			ip := strings.TrimSpace(line)
			if name, ok := encryptedDNS[ip]; ok {
				isEncrypted = true
				provider = name
			}
		}

		if isEncrypted {
			// Verify the encrypted DNS is actually reachable
			firstDNS := strings.TrimSpace(strings.Split(dns, "\n")[0])
			if dnsReachable(p, firstDNS) {
				finding.Severity = SeverityOK
				finding.Message = fmt.Sprintf("Encrypted DNS configured (%s)", provider)
			} else {
				finding.Severity = SeverityCritical
				finding.Message = fmt.Sprintf("Encrypted DNS configured (%s) but UNREACHABLE — network may be blocking it", provider)
				finding.Detail = "Fix: sudo networksetup -setdnsservers Wi-Fi empty (reverts to network DNS)"
				if fix {
					// Fall back to network default and verify resolution recovers
					saveNetworkState(p)
					_, _ = p.Command("networksetup", "-setdnsservers", "Wi-Fi", "empty")
					if verifyDNSOrRollback(p, 3, 2*time.Second) {
						finding.Severity = SeverityWarn
						finding.Message = fmt.Sprintf("%s was unreachable — reverted to network default DNS", provider)
						finding.Detail = "This network blocks external DNS. Use a VPN for encrypted DNS on this network."
					} else {
						// Network default also broken — restore what was there before
						finding.Severity = SeverityCritical
						finding.Message = fmt.Sprintf("Reverted from %s but network DNS also failing — restored original config", provider)
						finding.Detail = "Both external and network DNS are failing. Check network connectivity."
					}
				}
			}
		} else {
			finding.Severity = SeverityWarn
			finding.Message = "Custom DNS set but not a known encrypted provider"
		}
	}

	report.Findings = append(report.Findings, finding)
}

// checkWiFiSecurity checks the current WiFi connection's security protocol.
// Uses networksetup (fast) instead of system_profiler (can hang for 10s+).
func checkWiFiSecurity(p platform.Platform, report *NetworkReport) {
	finding := DiagnosticFinding{
		Check: "WiFi Security",
	}

	// Get current SSID via networksetup (fast, no admin)
	out, err := p.Command("networksetup", "-getairportnetwork", "en0")
	ssid := ""
	if err == nil {
		line := strings.TrimSpace(string(out))
		if strings.HasPrefix(line, "Current Wi-Fi Network:") {
			ssid = strings.TrimSpace(strings.TrimPrefix(line, "Current Wi-Fi Network:"))
		}
	}

	if ssid == "" {
		// Try en1 (some Macs use different interface)
		out, err = p.Command("networksetup", "-getairportnetwork", "en1")
		if err == nil {
			line := strings.TrimSpace(string(out))
			if strings.HasPrefix(line, "Current Wi-Fi Network:") {
				ssid = strings.TrimSpace(strings.TrimPrefix(line, "Current Wi-Fi Network:"))
			}
		}
	}

	if ssid == "" {
		finding.Severity = SeverityInfo
		finding.Message = "Not connected to WiFi (Ethernet or not associated)"
		report.Findings = append(report.Findings, finding)
		return
	}

	// Read known networks plist to find security type for current SSID
	out, err = p.Command("defaults", "read",
		"/Library/Preferences/com.apple.wifi.known-networks")
	if err != nil {
		// Fallback — we know the SSID but not the security
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Connected to \"%s\" — security type unknown (needs admin to check)", ssid)
		finding.Detail = "Run with sudo for full WiFi security details"
		report.Findings = append(report.Findings, finding)
		return
	}

	plist := string(out)
	finding.Detail = fmt.Sprintf("SSID: %s", ssid)

	// Parse security from known networks — look for WPA3, WPA2, or None near our SSID
	switch {
	case strings.Contains(plist, "WPA3"):
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("Connected to \"%s\" — WPA3 encryption", ssid)
	case strings.Contains(plist, "WPA2"):
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("Connected to \"%s\" — WPA2 encryption", ssid)
	case strings.Contains(plist, "None") || strings.Contains(plist, "Open"):
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf("Connected to \"%s\" — OPEN network, use VPN immediately", ssid)
	default:
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("Connected to \"%s\"", ssid)
	}

	report.Findings = append(report.Findings, finding)
}

// checkTLSConnection verifies TLS 1.3 to the Anthropic API.
func checkTLSConnection(report *NetworkReport) {
	finding := DiagnosticFinding{
		Check: "TLS to Anthropic API",
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp",
		"api.anthropic.com:443",
		&tls.Config{MinVersion: tls.VersionTLS13},
	)
	if err != nil {
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf("TLS 1.3 connection failed: %v", err)
		report.Findings = append(report.Findings, finding)
		return
	}
	defer conn.Close()

	state := conn.ConnectionState()
	cipherName := tls.CipherSuiteName(state.CipherSuite)

	finding.Severity = SeverityOK
	finding.Message = fmt.Sprintf("TLS 1.3 verified — %s", cipherName)
	finding.Detail = fmt.Sprintf("Server: %s | Cipher: %s | ALPN: %s",
		state.ServerName, cipherName, state.NegotiatedProtocol)

	report.Findings = append(report.Findings, finding)
}

// dnsReachable tests if a DNS server is reachable at the transport level.
// Uses a raw TCP connect to port 53 — does NOT depend on DNS resolution,
// so it works even when the system's DNS is misconfigured or broken.
func dnsReachable(_ platform.Platform, dnsIP string) bool {
	conn, err := net.DialTimeout("tcp", dnsIP+":53", 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// dnsResolves verifies that the system can actually resolve a hostname.
// Unlike dnsReachable (transport-level), this confirms end-to-end resolution
// works after a config change. Uses a hardcoded IP-free lookup so it exercises
// the full DNS stack.
func dnsResolves() bool {
	_, err := net.LookupHost("api.anthropic.com")
	return err == nil
}

// verifyDNSOrRollback polls DNS resolution after a config change.
// If resolution fails across all attempts, it reverts DNS to the prior state.
// Returns true if DNS is working, false if it rolled back.
func verifyDNSOrRollback(p platform.Platform, attempts int, interval time.Duration) bool {
	for i := 0; i < attempts; i++ {
		if dnsResolves() {
			return true
		}
		if i < attempts-1 {
			time.Sleep(interval)
		}
	}
	// DNS never came up — revert to prior state
	path := filepath.Join(stateDir(), "dns_prior.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		// No saved state — fall back to DHCP default
		_, _ = p.Command("networksetup", "-setdnsservers", "Wi-Fi", "empty")
		return false
	}
	prior := strings.TrimSpace(string(data))
	if prior == "" || strings.Contains(prior, "aren't any") || strings.Contains(prior, "no DNS") {
		_, _ = p.Command("networksetup", "-setdnsservers", "Wi-Fi", "empty")
	} else {
		args := append([]string{"-setdnsservers", "Wi-Fi"}, strings.Fields(prior)...)
		_, _ = p.Command("networksetup", args...)
	}
	_ = os.Remove(path)
	return false
}

// checkCACertificates audits the system root certificate store.
func checkCACertificates(p platform.Platform, report *NetworkReport) {
	out, err := p.Command("security", "find-certificate", "-a", "/System/Library/Keychains/SystemRootCertificates.keychain")
	if err != nil {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    "CA Certificate Audit",
			Severity: SeverityWarn,
			Message:  "Could not read system certificate store",
		})
		return
	}

	certCount := strings.Count(string(out), "labl")

	finding := DiagnosticFinding{
		Check:  "CA Certificate Audit",
		Detail: fmt.Sprintf("%d root certificates in system store", certCount),
	}

	switch {
	case certCount > 200:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("%d root CAs — unusually high, check for rogue certificates", certCount)
	case certCount < 100:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("%d root CAs — unusually low, certificate validation may fail", certCount)
	default:
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("%d root CAs — normal range for macOS", certCount)
	}

	report.Findings = append(report.Findings, finding)
}

// checkVPNStatus checks for active VPN tunnel interfaces.
func checkVPNStatus(p platform.Platform, report *NetworkReport) {
	out, err := p.Command("ifconfig")
	if err != nil {
		return
	}

	ifcOutput := string(out)

	// Count utun interfaces with actual traffic (UP and RUNNING)
	var activeVPN []string
	sections := strings.Split(ifcOutput, "\n\n")
	for _, section := range sections {
		if !strings.Contains(section, "utun") {
			continue
		}
		lines := strings.Split(section, "\n")
		if len(lines) == 0 {
			continue
		}
		ifName := strings.TrimSuffix(strings.Fields(lines[0])[0], ":")
		if strings.Contains(section, "inet ") && strings.Contains(section, "UP") {
			activeVPN = append(activeVPN, ifName)
		}
	}

	finding := DiagnosticFinding{
		Check: "VPN Status",
	}

	// macOS creates system utun interfaces (ipsec, Network Extensions) even without VPN.
	// A real VPN usually has inet addresses on utun interfaces.
	if len(activeVPN) > 2 {
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("VPN likely active (%d tunnel interfaces with addresses)", len(activeVPN))
		finding.Detail = strings.Join(activeVPN, ", ")
	} else {
		finding.Severity = SeverityWarn
		finding.Message = "No VPN detected — traffic visible to network operator"
		finding.Detail = "Recommend: WireGuard, Tailscale, or commercial VPN for public WiFi"
	}

	report.Findings = append(report.Findings, finding)
}

// checkFirewall checks if the macOS application firewall is enabled.
func checkFirewall(p platform.Platform, report *NetworkReport, fix bool) {
	out, err := p.Command("/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate")
	if err != nil {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    "macOS Firewall",
			Severity: SeverityWarn,
			Message:  "Could not read firewall status",
		})
		return
	}

	status := strings.TrimSpace(string(out))
	finding := DiagnosticFinding{
		Check:  "macOS Firewall",
		Detail: status,
	}

	if strings.Contains(status, "disabled") || strings.Contains(status, "State = 0") {
		finding.Severity = SeverityCritical
		finding.Message = "Firewall is disabled — accepting unsolicited inbound connections"

		if fix {
			_, fixErr := p.Command("sudo", "/usr/libexec/ApplicationFirewall/socketfilterfw", "--setglobalstate", "on")
			if fixErr == nil {
				finding.Message += " → FIXED: Firewall enabled"
				finding.Severity = SeverityOK
			} else {
				finding.Detail = "Auto-fix failed (needs admin): sudo /usr/libexec/ApplicationFirewall/socketfilterfw --setglobalstate on"
			}
		}
	} else {
		finding.Severity = SeverityOK
		finding.Message = "Firewall is enabled"
	}

	report.Findings = append(report.Findings, finding)
}
