// Package fabric — read-only Thunderbolt link telemetry for the Sirsi Sleeve
// program (SirsiNexusApp Notebook 4 §4.9 P0; docs/HARDWARE_BASELINE_ALIGNMENT.md
// "Shared transport capability receipt").
//
// Rules this package obeys, in order:
//   - A field the host cannot observe is "unknown" with a reason — never a zero,
//     never a guess (IO7a; baseline "Non-negotiable claim rules").
//   - Advertised is not negotiated is not observed. system_profiler exposes the
//     receptacle's advertised ceiling only; the negotiated rate of a host-to-host
//     link is not exposed there, so it is reported unknown rather than copied
//     from the ceiling.
//   - Nothing here mutates anything.
package fabric

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Tri is a three-valued observation. "unknown" is a first-class answer.
type Tri string

const (
	Yes     Tri = "yes"
	No      Tri = "no"
	Unknown Tri = "unknown"
)

// Port is one Thunderbolt receptacle as the host reports it.
type Port struct {
	Bus            string `json:"bus"`
	ReceptacleID   string `json:"receptacle_id"`
	AdvertisedMax  string `json:"advertised_max"`         // e.g. "Up to 120 Gb/s" — a ceiling, not a rate
	LinkStatusRaw  string `json:"link_status_raw"`        // vendor hex, passed through untouched
	Connected      Tri    `json:"connected"`              // a device or peer is attached
	Peer           string `json:"peer,omitempty"`         // attached device name, when connected
	NegotiatedRate string `json:"negotiated_rate"`        // always "unknown" from this source; see reason
	NegotiatedWhy  string `json:"negotiated_rate_reason"` // why it is unknown
}

// RDMA is the host's RDMA-over-Thunderbolt enablement state.
type RDMA struct {
	FamilyLoaded Tri    `json:"family_loaded"` // IORDMAFamily present (sysctl debug.rdma.version)
	NVRAMEnabled Tri    `json:"nvram_enabled"` // nvram rdma-enable set (Recovery-time enablement, TN3205)
	Reason       string `json:"reason,omitempty"`
}

// Links is the receipt. CollectedAt is the collection time (D-COMMON: a
// projection carries its own clock).
type Links struct {
	CollectedAt    time.Time `json:"collected_at"`
	Host           string    `json:"host"`
	MacOS          string    `json:"macos"`
	Ports          []Port    `json:"ports"`
	PortCountKnown bool      `json:"port_count_known"` // false ⇒ Ports is not evidence of anything
	Bridges        []string  `json:"thunderbolt_interfaces"`
	RDMA           RDMA      `json:"rdma"`
	Unknown        []string  `json:"unknown"` // every field that could not be observed, with why
	Source         string    `json:"source"`
}

const negotiatedWhy = "system_profiler reports the receptacle ceiling only; negotiated host-to-host rate needs a link-level probe (Notebook 4 Phase 0)"

// Collect reads the host once. It never returns an error: an unreadable source
// becomes an "unknown" entry, because a missing receipt field must be visible
// as missing, not as a failed command that renders nothing.
func Collect(p platform.Platform) Links {
	l := Links{CollectedAt: time.Now().UTC(), Source: "system_profiler SPThunderboltDataType; nvram rdma-enable; sysctl debug.rdma.version; networksetup"}
	l.Host = firstLine(run(p, "hostname"))
	if l.Host == "" {
		l.Host = "unknown"
		l.Unknown = append(l.Unknown, "host: hostname unreadable")
	}
	l.MacOS = firstLine(run(p, "sw_vers", "-productVersion"))
	if l.MacOS == "" {
		l.MacOS = "unknown"
		l.Unknown = append(l.Unknown, "macos: sw_vers unreadable")
	}

	raw, err := p.Command("system_profiler", "SPThunderboltDataType", "-json")
	ports, perr := parsePorts(raw)
	switch {
	case err != nil:
		l.Unknown = append(l.Unknown, "ports: system_profiler failed: "+err.Error())
	case perr != nil:
		l.Unknown = append(l.Unknown, "ports: "+perr.Error())
	default:
		l.Ports = ports
		l.PortCountKnown = true
	}

	l.Bridges = tbInterfaces(run(p, "networksetup", "-listallhardwareports"))
	if l.Bridges == nil {
		l.Unknown = append(l.Unknown, "thunderbolt_interfaces: networksetup unreadable")
	}

	l.RDMA = rdmaState(p)
	if l.RDMA.FamilyLoaded == Unknown || l.RDMA.NVRAMEnabled == Unknown {
		l.Unknown = append(l.Unknown, "rdma: "+l.RDMA.Reason)
	}
	return l
}

func run(p platform.Platform, name string, args ...string) string {
	out, err := p.Command(name, args...)
	if err != nil && len(out) == 0 {
		return ""
	}
	return string(out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// parsePorts reads system_profiler's JSON. Shape (macOS 26): one entry per
// bus; "receptacle_1_tag" describes the port; "_items", when present, lists
// attached devices. Unknown keys are ignored, missing keys become "unknown".
func parsePorts(raw []byte) ([]Port, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("system_profiler returned nothing")
	}
	var doc struct {
		Buses []map[string]json.RawMessage `json:"SPThunderboltDataType"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("system_profiler JSON unparseable: %v", err)
	}
	if doc.Buses == nil {
		return nil, fmt.Errorf("system_profiler JSON has no SPThunderboltDataType key")
	}
	ports := make([]Port, 0, len(doc.Buses))
	for _, b := range doc.Buses {
		pt := Port{Bus: str(b, "_name"), NegotiatedRate: string(Unknown), NegotiatedWhy: negotiatedWhy, Connected: Unknown}
		if pt.Bus == "" {
			pt.Bus = "unknown"
		}
		var rec map[string]json.RawMessage
		if r, ok := b["receptacle_1_tag"]; ok && json.Unmarshal(r, &rec) == nil {
			pt.ReceptacleID = str(rec, "receptacle_id_key")
			pt.AdvertisedMax = str(rec, "current_speed_key")
			pt.LinkStatusRaw = str(rec, "link_status_key")
			switch s := str(rec, "receptacle_status_key"); {
			case s == "receptacle_no_devices_connected":
				pt.Connected = No
			case strings.Contains(s, "connected"):
				pt.Connected = Yes
			}
		}
		if pt.ReceptacleID == "" {
			pt.ReceptacleID = "unknown"
		}
		if pt.AdvertisedMax == "" {
			pt.AdvertisedMax = "unknown"
		}
		var items []map[string]json.RawMessage
		if r, ok := b["_items"]; ok && json.Unmarshal(r, &items) == nil && len(items) > 0 {
			pt.Connected = Yes
			names := make([]string, 0, len(items))
			for _, it := range items {
				if n := str(it, "device_name_key"); n != "" {
					names = append(names, n)
				} else if n := str(it, "_name"); n != "" {
					names = append(names, n)
				}
			}
			pt.Peer = strings.Join(names, ", ")
		}
		ports = append(ports, pt)
	}
	return ports, nil
}

func str(m map[string]json.RawMessage, k string) string {
	var s string
	if r, ok := m[k]; ok && json.Unmarshal(r, &s) == nil {
		return s
	}
	return ""
}

// tbInterfaces returns the BSD names of "Thunderbolt N" hardware ports and the
// Thunderbolt Bridge, or nil when the listing is unreadable.
func tbInterfaces(listing string) []string {
	if strings.TrimSpace(listing) == "" {
		return nil
	}
	var out []string
	lines := strings.Split(listing, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if strings.HasPrefix(lines[i], "Hardware Port: Thunderbolt") && strings.HasPrefix(lines[i+1], "Device: ") {
			out = append(out, strings.TrimPrefix(lines[i+1], "Device: "))
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func rdmaState(p platform.Platform) RDMA {
	r := RDMA{FamilyLoaded: Unknown, NVRAMEnabled: Unknown}
	var why []string

	if v := firstLine(run(p, "sysctl", "-n", "debug.rdma.version")); v != "" && !strings.Contains(v, "unknown oid") {
		r.FamilyLoaded = Yes
	} else if strings.Contains(v, "unknown oid") {
		r.FamilyLoaded = No
	} else {
		why = append(why, "sysctl debug.rdma.version returned nothing")
	}

	out := run(p, "nvram", "rdma-enable")
	switch {
	case strings.Contains(out, "data was not found"):
		r.NVRAMEnabled = No
	case strings.HasPrefix(strings.TrimSpace(out), "rdma-enable"):
		r.NVRAMEnabled = Yes
	default:
		why = append(why, "nvram rdma-enable unreadable")
	}
	r.Reason = strings.Join(why, "; ")
	return r
}
