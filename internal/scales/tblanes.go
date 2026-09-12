package scales

// Thunderbolt lane hygiene: the invariant sirsi-io-connect's raw Thunderbolt
// transport (tbraw) needs on every Mac in a pod, weighed and healed by Scales.
//
// Verified 2026-09-12 on the M1 and the M5: on every Thunderbolt link
// renegotiation (replug, sleep/wake, peer reboot) macOS re-claims the TB
// interfaces into bridge0 and resets their MTU to 1500 — even with the
// "Thunderbolt Bridge" network service disabled on both ends, so that toggle
// is not a fix. A 65518-byte raw frame on a 1500-MTU lane fails with EMSGSIZE,
// which killed a whole transport's send path (io-connect #173, #174). The
// universal invariant lives here: every ACTIVE Thunderbolt port is out of
// bridge0 at MTU 65518. The host-specific control-lane IP aliases stay with
// io-connect's ai.sirsi.tb-lanes daemon; they are pod cabling, not hygiene.

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// TBLaneMTU is the jumbo MTU tbraw's 65518-byte frames need on every lane.
const TBLaneMTU = 65518

// TBLane is one Thunderbolt hardware port as the OS currently has it.
type TBLane struct {
	Iface    string `json:"iface"`     // en1
	Port     string `json:"port"`      // "Thunderbolt 1"
	MTU      int    `json:"mtu"`       // as configured
	Active   bool   `json:"active"`    // link up (ifconfig status: active)
	InBridge bool   `json:"in_bridge"` // a member of bridge0
}

// Drifted reports whether this lane violates the invariant. Only an active
// link counts: an unplugged port is not drift, it is just idle.
func (l TBLane) Drifted() bool { return l.Active && (l.InBridge || l.MTU != TBLaneMTU) }

// Injectable providers (Rule A21: concurrency-safe injectable mocks).
var (
	tbLaneLister func() ([]TBLane, error)   = listTBLanes
	tbLaneRunner func(args ...string) error = runIfconfig
	tbLaneMu     sync.RWMutex
)

// SetTBLaneProviders overrides how lanes are listed and how ifconfig is run
// (for testing). Pass nil to keep the current provider.
func SetTBLaneProviders(lister func() ([]TBLane, error), runner func(args ...string) error) {
	tbLaneMu.Lock()
	defer tbLaneMu.Unlock()
	if lister != nil {
		tbLaneLister = lister
	}
	if runner != nil {
		tbLaneRunner = runner
	}
}

func getTBLaneProviders() (func() ([]TBLane, error), func(args ...string) error) {
	tbLaneMu.RLock()
	defer tbLaneMu.RUnlock()
	return tbLaneLister, tbLaneRunner
}

// CollectTBLaneDrift lists the Thunderbolt lanes and counts the drifted ones.
func CollectTBLaneDrift() (drifted int, lanes []TBLane, err error) {
	lister, _ := getTBLaneProviders()
	lanes, err = lister()
	if err != nil {
		return 0, nil, err
	}
	for _, l := range lanes {
		if l.Drifted() {
			drifted++
		}
	}
	return drifted, lanes, nil
}

// FixTBLanes heals every drifted lane: removes it from bridge0 and sets its
// MTU to TBLaneMTU. dryRun reports what would be fixed and touches nothing.
// Returns how many lanes were (or would be) fixed and, after a real fix, the
// lanes still drifted on re-list (should be none).
func FixTBLanes(dryRun bool) (fixed int, remaining []TBLane, err error) {
	lister, runner := getTBLaneProviders()
	lanes, err := lister()
	if err != nil {
		return 0, nil, err
	}
	for _, l := range lanes {
		if !l.Drifted() {
			continue
		}
		fixed++
		if dryRun {
			continue
		}
		if l.InBridge {
			if e := runner("bridge0", "deletem", l.Iface); e != nil {
				return fixed, nil, fmt.Errorf("%s: remove from bridge0: %w", l.Iface, e)
			}
		}
		if l.MTU != TBLaneMTU {
			if e := runner(l.Iface, "mtu", strconv.Itoa(TBLaneMTU)); e != nil {
				return fixed, nil, fmt.Errorf("%s: mtu %d: %w", l.Iface, TBLaneMTU, e)
			}
		}
	}
	if dryRun {
		return fixed, nil, nil
	}
	after, err := lister()
	if err != nil {
		return fixed, nil, err
	}
	for _, l := range after {
		if l.Drifted() {
			remaining = append(remaining, l)
		}
	}
	return fixed, remaining, nil
}

// listTBLanes is the real lister: networksetup for the Thunderbolt port ->
// interface map, ifconfig for MTU/link state and bridge0 membership.
func listTBLanes() ([]TBLane, error) {
	hw, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return nil, fmt.Errorf("networksetup: %w", err)
	}
	ports := parseHardwarePorts(string(hw))
	if len(ports) == 0 {
		return nil, nil // no Thunderbolt ports on this Mac: nothing to weigh
	}
	br, _ := exec.Command("ifconfig", "bridge0").Output() // absent bridge0 = no members
	members := parseBridgeMembers(string(br))
	var lanes []TBLane
	for port, iface := range ports {
		out, err := exec.Command("ifconfig", iface).Output()
		if err != nil {
			continue // interface vanished between the two calls
		}
		mtu, active := parseIfconfig(string(out))
		lanes = append(lanes, TBLane{Iface: iface, Port: port, MTU: mtu, Active: active, InBridge: members[iface]})
	}
	return lanes, nil
}

// runIfconfig runs ifconfig with the given args, via sudo -n when not root.
func runIfconfig(args ...string) error {
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command("ifconfig", args...)
	} else {
		cmd = exec.Command("sudo", append([]string{"-n", "ifconfig"}, args...)...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ifconfig %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseHardwarePorts maps "Thunderbolt N" hardware ports to their device
// name from `networksetup -listallhardwareports`. "Thunderbolt Bridge" (the
// bridge0 service itself) is not a lane and is skipped.
func parseHardwarePorts(out string) map[string]string {
	ports := map[string]string{}
	var port string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "Hardware Port:"):
			port = strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
		case strings.HasPrefix(line, "Device:"):
			dev := strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
			if strings.HasPrefix(port, "Thunderbolt ") && !strings.HasPrefix(port, "Thunderbolt Bridge") && dev != "" {
				ports[port] = dev
			}
			port = ""
		}
	}
	return ports
}

// parseIfconfig pulls the MTU and link state out of one `ifconfig enX`.
func parseIfconfig(out string) (mtu int, active bool) {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, " mtu "); i >= 0 && mtu == 0 {
			f := strings.Fields(line[i+5:])
			if len(f) > 0 {
				mtu, _ = strconv.Atoi(f[0])
			}
		}
		if strings.HasPrefix(line, "status:") {
			active = strings.TrimSpace(strings.TrimPrefix(line, "status:")) == "active"
		}
	}
	return mtu, active
}

// parseBridgeMembers returns the member set from `ifconfig bridge0`.
func parseBridgeMembers(out string) map[string]bool {
	members := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "member:") {
			f := strings.Fields(strings.TrimPrefix(line, "member:"))
			if len(f) > 0 {
				members[f[0]] = true
			}
		}
	}
	return members
}
