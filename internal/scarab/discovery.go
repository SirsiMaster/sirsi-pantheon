// Package scarab provides network discovery and fleet scanning.
// Named after the Egyptian dung beetle — the sacred transformer
// that rolls across the landscape discovering and transforming.
package scarab

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Injectable dependencies for testability.
var (
	getLocalSubnetFn = defaultGetLocalSubnet
	parseARPTableFn  = defaultParseARPTable
	pingSweepFn      = defaultPingSweep
	pingHostFn       = defaultPingHost
	runARPCommand    = func() ([]byte, error) {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("arp", "-a")
		case "linux":
			cmd = exec.Command("arp", "-n")
		default:
			return nil, fmt.Errorf("unsupported OS")
		}
		return cmd.Output()
	}
)

// Host represents a discovered host on the network.
type Host struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	Vendor    string `json:"vendor,omitempty"`
	OpenPorts []int  `json:"open_ports,omitempty"`
	Alive     bool   `json:"alive"`
}

// DiscoveryResult contains all discovered hosts.
type DiscoveryResult struct {
	Hosts      []Host        `json:"hosts"`
	Subnet     string        `json:"subnet"`
	Duration   time.Duration `json:"duration"`
	TotalAlive int           `json:"total_alive"`
}

// Discover scans the local subnet for active hosts.
// Uses ARP table (instant) + parallel ping sweep.
func Discover() (*DiscoveryResult, error) {
	start := time.Now()
	result := &DiscoveryResult{}

	// Get local subnet
	subnet, err := getLocalSubnetFn()
	if err != nil {
		return nil, fmt.Errorf("detect subnet: %w", err)
	}
	result.Subnet = subnet

	// Phase 1: Parse ARP table (instant — already known hosts)
	arpHosts := parseARPTableFn()

	// Phase 2: Ping sweep for additional discovery
	pingHosts := pingSweepFn(subnet)

	// Merge results
	hostMap := make(map[string]*Host)
	for _, h := range arpHosts {
		hostMap[h.IP] = &h
	}
	for _, h := range pingHosts {
		if existing, ok := hostMap[h.IP]; ok {
			existing.Alive = true
		} else {
			hostMap[h.IP] = &h
		}
	}

	for _, h := range hostMap {
		if h.Alive {
			result.TotalAlive++
		}
		result.Hosts = append(result.Hosts, *h)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// getLocalSubnet returns the local subnet in CIDR notation.
func getLocalSubnet() (string, error) {
	return getLocalSubnetFn()
}

func defaultGetLocalSubnet() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
					network := ipnet.IP.Mask(ipnet.Mask)
					ones, _ := ipnet.Mask.Size()
					return fmt.Sprintf("%s/%d", network.String(), ones), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no active network interface found")
}

// parseARPTable reads the system ARP cache.
func parseARPTable() []Host {
	return parseARPTableFn()
}

func defaultParseARPTable() []Host {
	var hosts []Host

	out, err := runARPCommand()
	if err != nil {
		return hosts
	}

	for _, line := range strings.Split(string(out), "\n") {
		h := parseARPLine(line)
		if h != nil {
			hosts = append(hosts, *h)
		}
	}
	return hosts
}

// parseARPLine extracts IP and MAC from an ARP table entry.
func parseARPLine(line string) *Host {
	return parseARPLineFor(line, runtime.GOOS)
}

func parseARPLineFor(line, goos string) *Host {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// macOS: ? (192.168.1.1) at aa:bb:cc:dd:ee:ff on en0 ...
	// Linux: 192.168.1.1  ether  aa:bb:cc:dd:ee:ff  C  en0
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return nil
	}

	var ip, mac string

	if goos == "darwin" {
		// Extract IP from parentheses
		for _, f := range fields {
			if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
				ip = strings.Trim(f, "()")
				break
			}
		}
		// MAC is usually field[3] if field[2] is 'at'
		if len(fields) > 3 && fields[2] == "at" {
			mac = fields[3]
		}
	} else {
		ip = fields[0]
		// Linux 'arp -n' format: IP address HW type HW address Flags Mask Iface
		// 192.168.1.1 ether aa:bb:cc:dd:ee:ff C en0
		if len(fields) > 2 {
			mac = fields[2]
		}
	}

	if ip == "" || !isValidIP(ip) {
		return nil
	}
	if mac == "(incomplete)" || mac == "<incomplete>" {
		mac = ""
	}

	return &Host{
		IP:    ip,
		MAC:   mac,
		Alive: mac != "", // If in ARP table with MAC, it was recently alive
	}
}

// pingSweep sends ICMP pings across the subnet.
func pingSweep(subnet string) []Host {
	return pingSweepFn(subnet)
}

func defaultPingSweep(subnet string) []Host {
	ip, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil
	}

	var hosts []Host
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrency to avoid flood
	sem := make(chan struct{}, 50)

	for scanIP := ip.Mask(ipnet.Mask); ipnet.Contains(scanIP); incrementIP(scanIP) {
		target := scanIP.String()
		if target == ipnet.IP.String() {
			continue // Skip network address
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(t string) {
			defer wg.Done()
			defer func() { <-sem }()

			if pingHostFn(t) {
				mu.Lock()
				hosts = append(hosts, Host{
					IP:    t,
					Alive: true,
				})
				mu.Unlock()
			}
		}(target)
	}

	wg.Wait()
	return hosts
}

// pingHost sends a single ping and returns success.
func pingHost(ip string) bool {
	return pingHostFn(ip)
}

func defaultPingHost(ip string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("ping", "-c", "1", "-W", "500", ip)
	case "linux":
		cmd = exec.Command("ping", "-c", "1", "-W", "1", ip)
	default:
		return false
	}
	return cmd.Run() == nil
}

// incrementIP increments an IP address by one.
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isValidIP(s string) bool {
	return net.ParseIP(s) != nil
}
