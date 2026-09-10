package router

// relayagent.go — the per-host relay LaunchAgent (ADR-062 step 20a.4). One
// resident `sirsi router relay serve` per host holds the router service token;
// lanes talk to it through the spool and carry no token. The plist is the only
// file on the host that contains the token, so it is written privately (0600,
// temp + rename) and tightened on the idempotent path, like the wake plists.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RelayLaunchAgentLabel is the launchd label of the host relay.
const RelayLaunchAgentLabel = "ai.sirsi.router.relay"

// RelayLaunchAgentPlist renders the relay plist. url/token are the SERVICE
// address and this host's token — never a spool:// URL, which is for lanes.
func RelayLaunchAgentPlist(sirsiBin, spool, url, token string) string {
	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".sirsi", "logs", "router-relay.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PATH</key>
    <string>%s</string>
    <key>SIRSI_ROUTER_URL</key>
    <string>%s</string>
    <key>SIRSI_ROUTER_TOKEN</key>
    <string>%s</string>
  </dict>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>router</string>
    <string>relay</string>
    <string>serve</string>
    <string>--spool</string>
    <string>%s</string>
  </array>
  <key>KeepAlive</key>
  <true/>
  <key>RunAtLoad</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, RelayLaunchAgentLabel, escapeXML(LaunchAgentPATH(sirsiBin)), escapeXML(url), escapeXML(token), escapeXML(sirsiBin), escapeXML(spool), escapeXML(logPath), escapeXML(logPath))
}

// InstallRelayLaunchAgent writes the relay plist privately and returns whether
// it changed and its path. sirsiBin "" resolves to the running executable.
func InstallRelayLaunchAgent(sirsiBin, spool, url, token string) (changed bool, path string, err error) {
	switch {
	case strings.TrimSpace(url) == "" || strings.HasPrefix(strings.TrimSpace(url), "spool://"):
		return false, "", fmt.Errorf("relay install: SIRSI_ROUTER_URL must be the service's https URL (got %q)", url)
	case strings.TrimSpace(token) == "":
		return false, "", fmt.Errorf("relay install: SIRSI_ROUTER_TOKEN is empty; the relay is the host's token holder")
	}
	if strings.TrimSpace(sirsiBin) == "" {
		self, serr := os.Executable()
		if serr != nil {
			return false, "", fmt.Errorf("relay install: resolve sirsi binary: %w", serr)
		}
		sirsiBin = self
	}
	if !filepath.IsAbs(spool) {
		return false, "", fmt.Errorf("relay install: spool must be an absolute path (got %q)", spool)
	}
	dir := launchAgentsDir()
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return false, "", err
	}
	if home, herr := os.UserHomeDir(); herr == nil {
		if err = os.MkdirAll(filepath.Join(home, ".sirsi", "logs"), 0o755); err != nil {
			return false, "", err
		}
	}
	if err = os.MkdirAll(spool, 0o700); err != nil {
		return false, "", fmt.Errorf("relay install: spool dir: %w", err)
	}
	path = filepath.Join(dir, RelayLaunchAgentLabel+".plist")
	content := RelayLaunchAgentPlist(sirsiBin, spool, url, token)
	return writePrivatePlist(path, content)
}

// writePrivatePlist writes content to path at 0600 via a same-directory temp
// file and rename; an existing identical file is chmod'ed to 0600 and reported
// unchanged.
func writePrivatePlist(path, content string) (bool, string, error) {
	if existing, rerr := os.ReadFile(path); rerr == nil && string(existing) == content {
		if cerr := os.Chmod(path, 0o600); cerr != nil {
			return false, path, fmt.Errorf("secure plist: %w", cerr)
		}
		return false, path, nil
	}
	tmp, terr := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if terr != nil {
		return false, path, fmt.Errorf("write plist: %w", terr)
	}
	tmpName := tmp.Name()
	_, err := tmp.WriteString(content)
	if err == nil {
		err = tmp.Chmod(0o600)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return false, path, fmt.Errorf("write plist: %w", err)
	}
	return true, path, nil
}

// LoadRelayAgent (re)bootstraps the relay job: bootout is best-effort (absent
// is fine), bootstrap must succeed.
func LoadRelayAgent(plistPath string) error {
	_ = runLaunchctl("bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), RelayLaunchAgentLabel))
	if err := runLaunchctl("bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), plistPath); err != nil {
		return fmt.Errorf("launchctl bootstrap %s: %w", RelayLaunchAgentLabel, err)
	}
	return nil
}
