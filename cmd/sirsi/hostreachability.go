package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type hostProbeResult struct {
	Host               string `json:"host"`
	TransportReachable bool   `json:"transport_reachable"`
	TailscalePing      bool   `json:"tailscale_ping"`
	SSHReachable       bool   `json:"ssh_reachable"`
	ScreenSharing      bool   `json:"screen_sharing_reachable"`
	GUIState           string `json:"gui_state"`
	SNEState           string `json:"sne_state"`
	Classification     string `json:"classification"`
	Detail             string `json:"detail,omitempty"`
}

var (
	hostReachabilityTCP = probeHostTCP
	hostReachabilityRun = runHostProbeCommand
)

func probeHostTCP(ctx context.Context, address string) bool {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

func runHostProbeCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func classifyHostReachability(result hostProbeResult) hostProbeResult {
	result.TransportReachable = result.TailscalePing || result.SSHReachable || result.ScreenSharing
	switch {
	case result.TransportReachable && result.SNEState == "ready":
		result.Classification = "reachable-sne-ready"
	case result.TransportReachable:
		result.Classification = "reachable"
	default:
		result.Classification = "unreachable"
	}
	return result
}

func newHostReachabilityCommand() *cobra.Command {
	var host string
	var sshUser string
	var tailscaleBinary string
	var timeout time.Duration

	command := &cobra.Command{
		Use:   "reachability",
		Short: "Probe a Pantheon host without conflating transport and GUI lock state",
		RunE: func(cmd *cobra.Command, args []string) error {
			host = strings.TrimSpace(host)
			if host == "" {
				return fmt.Errorf("host is required")
			}
			if timeout <= 0 || timeout > 30*time.Second {
				return fmt.Errorf("timeout must be greater than zero and no more than 30s")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			result := hostProbeResult{Host: host, GUIState: "unknown", SNEState: "unknown"}
			tailscaleResult := make(chan bool, 1)
			sshResult := make(chan bool, 1)
			screenResult := make(chan bool, 1)
			go func() {
				probeCtx, probeCancel := context.WithTimeout(ctx, 3*time.Second)
				defer probeCancel()
				output, err := hostReachabilityRun(probeCtx, tailscaleBinary, "ping", "--c", "1", host)
				tailscaleResult <- err == nil && strings.Contains(string(output), "pong from")
			}()
			go func() { sshResult <- hostReachabilityTCP(ctx, net.JoinHostPort(host, "22")) }()
			go func() { screenResult <- hostReachabilityTCP(ctx, net.JoinHostPort(host, "5900")) }()
			result.TailscalePing = <-tailscaleResult
			result.SSHReachable = <-sshResult
			result.ScreenSharing = <-screenResult

			if result.SSHReachable && strings.TrimSpace(sshUser) != "" {
				remote := "locked=$(/usr/sbin/ioreg -l -w 0 | /usr/bin/grep -m1 '\"IOConsoleLocked\"' || true); " +
					"ready=$(/usr/bin/curl -fsS --max-time 3 http://127.0.0.1:8477/health/ready 2>/dev/null || true); " +
					"printf 'LOCK=%s\\nREADY=%s\\n' \"$locked\" \"$ready\""
				output, err := hostReachabilityRun(ctx, "/usr/bin/ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", sshUser+"@"+host, remote)
				if err == nil {
					text := string(output)
					switch {
					case strings.Contains(text, "IOConsoleLocked\" = Yes"):
						result.GUIState = "locked"
					case strings.Contains(text, "IOConsoleLocked\" = No"):
						result.GUIState = "unlocked"
					}
					if strings.Contains(text, `"status":"ready"`) {
						result.SNEState = "ready"
					} else if strings.Contains(text, "READY=") {
						result.SNEState = "not-ready"
					}
				}
			}

			result = classifyHostReachability(result)
			result.Detail = "Transport is derived from bounded probes; idle Tailscale Active=false is never an unreachability veto. GUI and SNE states are independent."
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(result)
		},
	}
	command.Flags().StringVar(&host, "host", "", "Tailscale IP or MagicDNS host")
	command.Flags().StringVar(&sshUser, "ssh-user", "", "optional SSH user for GUI and SNE readiness classification")
	command.Flags().StringVar(&tailscaleBinary, "tailscale-binary", "/Applications/Tailscale.app/Contents/MacOS/Tailscale", "Tailscale CLI path")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Second, "whole-probe deadline")
	return command
}
