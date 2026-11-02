package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

// Machine registry: Macs register with Ra the way threads do (owner 2026-10-16). One record per
// Mac in .agents/idea-router/machines.json; admission is a merge to main, and a host reads the
// origin-pinned snapshot that `registry sync` stores next to agents.json. Consumers (Mercury's
// run selector) offer only registered machines; discovery may find others, Ra admits.

// Machine is one admitted Mac.
type Machine struct {
	ID           string `json:"id"` // stable name: M1, M5, M5-Studio
	Model        string `json:"model"`
	Chip         string `json:"chip,omitempty"`
	MemoryGiB    int    `json:"memory_gib,omitempty"`
	Tailscale    string `json:"tailscale,omitempty"`
	LAN          string `json:"lan,omitempty"` // DHCP: observed at registration, may drift
	MonitorPort  int    `json:"monitor_port"`
	RegisteredAt string `json:"registered_at"`
	AdmittedBy   string `json:"admitted_by"`
}

type machineFile struct {
	Machines []Machine `json:"machines"`
}

func machinesWorkingPath(routerRoot string) string { return filepath.Join(routerRoot, "machines.json") }

func machinesSnapshotPath() string {
	if d := snapshotDir(); d != "" {
		return filepath.Join(d, "machines.json")
	}
	return ""
}

// LoadMachines reads the pinned snapshot when this host is pinned to origin, else the working tree.
func LoadMachines(routerRoot string) ([]Machine, error) {
	path := machinesWorkingPath(routerRoot)
	if _, pinned := registrySource(routerRoot); pinned {
		if sp := machinesSnapshotPath(); sp != "" {
			path = sp
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("machine registry: %s not found: run `sirsi router registry sync` once machines.json is on origin/main", path)
		}
		return nil, fmt.Errorf("machine registry: %w", err)
	}
	return parseMachines(b)
}

func parseMachines(b []byte) ([]Machine, error) {
	var f machineFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("machine registry: %w", err)
	}
	seen := map[string]bool{}
	for _, m := range f.Machines {
		if strings.TrimSpace(m.ID) == "" || m.Model == "" || m.AdmittedBy == "" || m.RegisteredAt == "" {
			return nil, fmt.Errorf("machine registry: record %q is missing id, model, registered_at or admitted_by", m.ID)
		}
		if seen[m.ID] {
			return nil, fmt.Errorf("machine registry: duplicate machine %q", m.ID)
		}
		seen[m.ID] = true
	}
	if len(f.Machines) == 0 {
		return nil, errors.New("machine registry: no machines")
	}
	return f.Machines, nil
}

// tailnetRange is Tailscale's CGNAT address block; every node address falls inside it.
var tailnetRange = netip.MustParsePrefix("100.64.0.0/10")

// machineTailnetDomains renders each admitted Mac's Tailscale address as codex
// permission-table entries ("ip"="allow",...). It fails closed on an address that
// is not a tailnet IPv4 or is claimed twice: a bad registry record must never
// widen a lane's sandbox or break the table's syntax.
func machineTailnetDomains(ms []Machine) (string, error) {
	seen := map[netip.Addr]string{}
	var entries []string
	for _, m := range ms {
		if m.Tailscale == "" {
			continue
		}
		a, err := netip.ParseAddr(m.Tailscale)
		if err != nil || !a.Is4() || !tailnetRange.Contains(a) {
			return "", fmt.Errorf("machine %q tailscale %q is not a tailnet IPv4 address", m.ID, m.Tailscale)
		}
		if other, dup := seen[a]; dup {
			return "", fmt.Errorf("machines %q and %q both claim tailscale %s", other, m.ID, a)
		}
		seen[a] = m.ID
		entries = append(entries, fmt.Sprintf("%q=\"allow\"", a.String()))
	}
	if len(entries) == 0 {
		return "", errors.New("machine registry lists no tailscale addresses")
	}
	return strings.Join(entries, ","), nil
}
