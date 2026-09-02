package fabric

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// Captured from a MacBook Pro on macOS 26.6.1 with nothing plugged in.
const spNoDevices = `{"SPThunderboltDataType":[
 {"_name":"thunderboltusb4_bus_2","device_name_key":"MacBook Pro","receptacle_1_tag":{"current_speed_key":"Up to 120 Gb/s","link_status_key":"0x100","receptacle_id_key":"3","receptacle_status_key":"receptacle_no_devices_connected"}},
 {"_name":"thunderboltusb4_bus_1","device_name_key":"MacBook Pro","receptacle_1_tag":{"current_speed_key":"Up to 120 Gb/s","link_status_key":"0x100","receptacle_id_key":"2","receptacle_status_key":"receptacle_no_devices_connected"}},
 {"_name":"thunderboltusb4_bus_0","device_name_key":"MacBook Pro","receptacle_1_tag":{"current_speed_key":"Up to 120 Gb/s","link_status_key":"0x100","receptacle_id_key":"1","receptacle_status_key":"receptacle_no_devices_connected"}}]}`

// Same shape with one attached item, as system_profiler renders a connected device.
const spOnePeer = `{"SPThunderboltDataType":[
 {"_name":"thunderboltusb4_bus_0","receptacle_1_tag":{"current_speed_key":"Up to 120 Gb/s","link_status_key":"0x2","receptacle_id_key":"1","receptacle_status_key":"receptacle_connected"},
  "_items":[{"_name":"Mac Studio","device_name_key":"Mac Studio","vendor_name_key":"Apple Inc."}]}]}`

const netsetup = "Hardware Port: Thunderbolt Bridge\nDevice: bridge0\nEthernet Address: 36:17:01:8c:7d:c0\n\nHardware Port: Thunderbolt 1\nDevice: en1\nEthernet Address: 36:17:01:8c:7d:c0\n\nHardware Port: Wi-Fi\nDevice: en0\n"

func mock(sp string) *platform.Mock {
	return &platform.Mock{NameStr: "darwin", CommandResults: map[string]string{
		"hostname":                "studio-a\n",
		"sw_vers -productVersion": "26.6.1\n",
		"system_profiler SPThunderboltDataType -json": sp,
		"networksetup -listallhardwareports":          netsetup,
		"sysctl -n debug.rdma.version":                "Jul 11 2026 14:22:31 RDMA_family\n",
		"nvram rdma-enable":                           "nvram: Error getting variable - 'rdma-enable': (iokit/common) data was not found\n",
	}}
}

func TestCollect_NoDevices(t *testing.T) {
	l := Collect(mock(spNoDevices))
	if !l.PortCountKnown || len(l.Ports) != 3 {
		t.Fatalf("ports: known=%v n=%d", l.PortCountKnown, len(l.Ports))
	}
	for _, p := range l.Ports {
		if p.Connected != No {
			t.Errorf("%s connected=%s, want no", p.Bus, p.Connected)
		}
		if p.AdvertisedMax != "Up to 120 Gb/s" {
			t.Errorf("%s advertised=%q", p.Bus, p.AdvertisedMax)
		}
		// The ceiling must never leak into the negotiated field.
		if p.NegotiatedRate != string(Unknown) || p.NegotiatedWhy == "" {
			t.Errorf("%s negotiated=%q why=%q — must be unknown with a reason", p.Bus, p.NegotiatedRate, p.NegotiatedWhy)
		}
	}
	if l.RDMA.FamilyLoaded != Yes || l.RDMA.NVRAMEnabled != No {
		t.Errorf("rdma = %+v, want family=yes nvram=no", l.RDMA)
	}
	if strings.Join(l.Bridges, ",") != "bridge0,en1" {
		t.Errorf("bridges = %v", l.Bridges)
	}
	if len(l.Unknown) != 0 {
		t.Errorf("unexpected unknowns: %v", l.Unknown)
	}
	if l.Host != "studio-a" || l.MacOS != "26.6.1" || l.CollectedAt.IsZero() {
		t.Errorf("identity: host=%q macos=%q at=%v", l.Host, l.MacOS, l.CollectedAt)
	}
}

func TestCollect_OnePeer(t *testing.T) {
	m := mock(spOnePeer)
	m.CommandResults["nvram rdma-enable"] = "rdma-enable\t1\n"
	l := Collect(m)
	if len(l.Ports) != 1 || l.Ports[0].Connected != Yes || l.Ports[0].Peer != "Mac Studio" {
		t.Fatalf("ports = %+v", l.Ports)
	}
	if l.RDMA.NVRAMEnabled != Yes {
		t.Errorf("nvram enabled = %s, want yes", l.RDMA.NVRAMEnabled)
	}
}

// Negative control: when nothing can be read, every field says so. A zero port
// count rendered as fact would be the 2026-07-29 board defect again.
func TestCollect_AllSourcesFail_IsUnknownNotZero(t *testing.T) {
	m := &platform.Mock{NameStr: "darwin", CommandError: errors.New("denied")}
	l := Collect(m)
	if l.PortCountKnown {
		t.Fatal("PortCountKnown must be false when system_profiler fails")
	}
	if l.Host != "unknown" || l.MacOS != "unknown" {
		t.Errorf("identity should be unknown, got host=%q macos=%q", l.Host, l.MacOS)
	}
	if l.RDMA.FamilyLoaded != Unknown || l.RDMA.NVRAMEnabled != Unknown {
		t.Errorf("rdma should be unknown, got %+v", l.RDMA)
	}
	if l.Bridges != nil {
		t.Errorf("bridges should be nil (unreadable), got %v", l.Bridges)
	}
	want := []string{"host:", "macos:", "ports:", "thunderbolt_interfaces:", "rdma:"}
	for _, w := range want {
		found := false
		for _, u := range l.Unknown {
			if strings.HasPrefix(u, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("Unknown lacks %q entry; got %v", w, l.Unknown)
		}
	}
}

func TestParsePorts_Garbage(t *testing.T) {
	if _, err := parsePorts([]byte("not json")); err == nil {
		t.Error("garbage must error")
	}
	if _, err := parsePorts([]byte(`{"other":[]}`)); err == nil {
		t.Error("missing key must error, not yield zero ports")
	}
}
