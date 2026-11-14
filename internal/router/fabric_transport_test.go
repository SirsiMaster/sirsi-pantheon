package router

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCodexConsumersKeepClosedNativeProxy(t *testing.T) {
	b, err := os.ReadFile("../../.agents/idea-router/agents.json")
	if err != nil {
		t.Fatal(err)
	}
	var reg Registry
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}
	count := 0
	for id, cfg := range reg.Agents {
		if cfg.Type != "codex" || len(cfg.Consumer.Command) == 0 {
			continue
		}
		count++
		t.Run(id, func(t *testing.T) {
			joined := strings.Join(cfg.Consumer.Command, "\n")
			for _, required := range []string{
				`default_permissions="sirsi-router"`,
				`features.network_proxy.enabled=true`,
				`features.network_proxy.allow_upstream_proxy=false`,
				`features.network_proxy.enable_socks5_udp=false`,
				`permissions.sirsi-router.network.enabled=true`,
				`permissions.sirsi-router.network.allow_local_binding=false`,
				`permissions.sirsi-router.network.allow_upstream_proxy=false`,
				`permissions.sirsi-router.network.enable_socks5_udp=false`,
				`permissions.sirsi-router.network.domains={` + consumerTailnetPlaceholder + `,"192.168.1.185"="allow","10.96.127.2"="allow"}`,
			} {
				if !strings.Contains(joined, "-c\n"+required) {
					t.Errorf("missing bounded setting %s", required)
				}
			}
			for _, forbidden := range []string{"--sandbox", "--dangerously", "SIRSI_ROUTER_TOKEN=", `"*"="allow"`, "network.enabled=false", "--add-dir\n/var/"} {
				if strings.Contains(joined, forbidden) {
					t.Errorf("unsafe/stale override %s", forbidden)
				}
			}
			if !strings.Contains(joined, "SIRSI_AGENT_ID="+id) || !strings.Contains(joined, "SIRSI_ROUTER_URL=spool:///private/var/sirsipantheon/relay") {
				t.Fatal("own identity and physical canonical relay must remain declared")
			}
			if !strings.Contains(cfg.Consumer.Prompt, "sirsi-fabric-run") {
				t.Fatal("missing scoped transport instructions")
			}
		})
	}
	if count == 0 {
		t.Fatal("vacuous registry check")
	}
}

func fabricPlanCommand(t *testing.T, env []string, args ...string) *exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("native macOS shell helper")
	}
	cmd := exec.Command("/bin/bash", append([]string{"../../scripts/maat/fabric-run.sh", "--plan"}, args...)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "SIRSI_AGENT_ID=codex-pantheon", "ALL_PROXY=socks5://127.0.0.1:19399"}
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

func TestFabricTransportPlanQuotesEveryRemoteArg(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	payload := "a 'quoted' $(touch " + marker + ") ; literal"
	out, err := fabricPlanCommand(t, nil, "m1", "--work", "quote-proof", "--", "/usr/bin/printf", payload).CombinedOutput()
	if err != nil {
		t.Fatalf("plan: %s: %v", out, err)
	}
	var remote string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "REMOTE_COMMAND=") {
			remote = strings.TrimPrefix(line, "REMOTE_COMMAND=")
		}
	}
	if remote == "" {
		t.Fatal("missing literal remote plan")
	}
	// Parse under the same POSIX shell grammar WITHOUT executing the runner.
	// A broken quote could only create the controlled temp marker, never a job.
	parse := exec.Command("/bin/bash", "-c", "set -- "+remote+"\nprintf '%s\\0' \"$@\"")
	got, err := parse.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/Users/sirsimasterdev/.local/bin/sirsi-maat-run", "--agent", "codex-pantheon", "--register-executor", "--work", "quote-proof", "--cpu-cores", "1", "--timeout", "90", "--", "/usr/bin/printf", payload}
	if !bytes.Equal(got, []byte(strings.Join(want, "\x00")+"\x00")) {
		t.Fatalf("argv not retained: %q", got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("payload escaped literal quoting")
	}
	for _, required := range []string{"StrictHostKeyChecking=yes", "PasswordAuthentication=no", "KbdInteractiveAuthentication=no", "PreferredAuthentications=publickey", "UserKnownHostsFile=", "HostKeyAlias=sirsi-m1", "ProxyCommand", "/usr/bin/python3", "fabric-endpoints.py", "connect", "socks", "ENDPOINT_POLICY=thunderbolt,thunderbolt-relay,classified-lan,tailscale", "fabric-ssh-quote-proof"} {
		if !strings.Contains(string(out), required) {
			t.Errorf("missing transport boundary %s", required)
		}
	}
}

func TestFabricTransportPlanRejectsMissingOrBroadAuthority(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env, args []string
	}{
		{"unknown host", nil, []string{"example.com", "--work", "x", "--", "/usr/bin/true"}},
		{"explicit socks without proxy", []string{"ALL_PROXY="}, []string{"--transport", "socks", "m1", "--work", "x", "--", "/usr/bin/true"}},
		{"remote proxy", []string{"ALL_PROXY=socks5://example.com:1080"}, []string{"m1", "--work", "x", "--", "/usr/bin/true"}},
		{"conflicting identity", []string{"SIRSI_ROUTER_AGENT=ra"}, []string{"m1", "--work", "x", "--", "/usr/bin/true"}},
		{"injected work", nil, []string{"m1", "--work", "x;touch bad", "--", "/usr/bin/true"}},
		{"zero timeout", nil, []string{"m1", "--work", "x", "--timeout", "0", "--", "/usr/bin/true"}},
		{"missing command", nil, []string{"m1", "--work", "x", "--"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, err := fabricPlanCommand(t, tc.env, tc.args...).CombinedOutput(); err == nil {
				t.Fatalf("refusal missing: %s", out)
			}
		})
	}
}

func TestFabricTransportPlanDefaultWithoutProxyMatchesExplicitDirect(t *testing.T) {
	proxyKeys := []string{"ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"}
	for _, host := range []string{"m1", "m5", "studio"} {
		for _, allEmpty := range []bool{false, true} {
			name := host + "/absent"
			if allEmpty {
				name = host + "/empty"
			}
			t.Run(name, func(t *testing.T) {
				cmd := fabricPlanCommand(t, nil, host, "--work", "default-proof", "--", "/usr/bin/true")
				env := make([]string, 0, len(cmd.Env)+len(proxyKeys))
				for _, entry := range cmd.Env {
					key, _, _ := strings.Cut(entry, "=")
					switch key {
					case "ALL_PROXY", "all_proxy", "HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy":
						continue
					}
					env = append(env, entry)
				}
				if allEmpty {
					for _, key := range proxyKeys {
						env = append(env, key+"=")
					}
				}
				cmd.Env = env
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("default plan: %s: %v", out, err)
				}
				explicit := exec.Command("/bin/bash", "../../scripts/maat/fabric-run.sh", "--plan", "--transport", "direct",
					host, "--work", "default-proof", "--", "/usr/bin/true")
				explicit.Env = append([]string(nil), env...)
				direct, err := explicit.CombinedOutput()
				if err != nil {
					t.Fatalf("explicit direct plan: %s: %v", direct, err)
				}
				if !bytes.Equal(out, direct) {
					t.Fatalf("default differs from explicit direct:\ndefault: %s\nexplicit: %s", out, direct)
				}
				for _, required := range []string{"PLAN_ONLY_NO_EXECUTION", "StrictHostKeyChecking=yes",
					"HostKeyAlias=sirsi-" + host, "ENDPOINT_POLICY=thunderbolt,thunderbolt-relay,classified-lan,tailscale"} {
					if !strings.Contains(string(out), required) {
						t.Errorf("missing direct-plan boundary %s", required)
					}
				}
			})
		}
	}
}
