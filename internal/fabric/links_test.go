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

// Observed header/row shape from main's exit-0 native capture, with live
// load addresses replaced by representative same-format constants.
// The full native capture and separate stderr remain private, outside Git.
const kmutilHeader = "Index Refs Address            Size       Wired      Name (Version) UUID <Linked Against>\n"
const kmutilZeroRow = "    3  215 0                  0          0          com.apple.kpi.bsd (27.0.0) 25FECA3D-AD9D-3958-B633-A14EC36D3BE7 <>\n"
const kmutilHexRow = "   10    6 0xfffffe0001000000 0x7098     0x7098     com.apple.kec.Libm (1) 4E3603E4-7BF1-3ED1-B3B6-F6FA24130C74 <6>\n"
const kmutilFamilyRow = "  235    1 0xfffffe0002000000 0x9316     0x9316     com.apple.iokit.IORDMAFamily (1.0) A266614B-439F-3B38-9862-F6D245F8327D <9 8 7 6 5 3>\n"
const kmutilDriverRow = "  236    0 0xfffffe0003000000 0x29ba5    0x29ba5    com.apple.driver.AppleThunderboltRDMA (0.0.1) 97C262AC-8D90-3704-A6F8-71CB7E734457 <235 108 77 58 7 6 5 3>\n"

func mock(sp string) *platform.Mock {
	return &platform.Mock{NameStr: "darwin", CommandResults: map[string]string{
		"hostname":                "studio-a\n",
		"sw_vers -productVersion": "26.6.1\n",
		"system_profiler SPThunderboltDataType -json": sp,
		"networksetup -listallhardwareports":          netsetup,
		"sysctl -n debug.rdma.version":                "Jul 11 2026 14:22:31 RDMA_family\n",
		"kmutil showloaded":                           kmutilHeader + kmutilZeroRow + kmutilFamilyRow,
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

func TestCollect_RDMASysctlAbsentUsesLoadedKextInventory(t *testing.T) {
	m := mock(spNoDevices)
	m.CommandResults["sysctl -n debug.rdma.version"] = "sysctl: unknown oid 'debug.rdma.version'\n"
	m.CommandResults["kmutil showloaded"] = kmutilHeader + kmutilZeroRow + kmutilFamilyRow + kmutilDriverRow
	l := Collect(m)
	if l.RDMA.FamilyLoaded != Yes {
		t.Fatalf("family_loaded=%s, want yes from kmutil fallback; rdma=%+v", l.RDMA.FamilyLoaded, l.RDMA)
	}
}

func TestCollect_RDMASysctlAbsentReportsNoOnlyAfterReadableKextInventory(t *testing.T) {
	m := mock(spNoDevices)
	m.CommandResults["sysctl -n debug.rdma.version"] = "sysctl: unknown oid 'debug.rdma.version'\n"
	m.CommandResults["kmutil showloaded"] = kmutilHeader + kmutilZeroRow + kmutilHexRow
	l := Collect(m)
	if l.RDMA.FamilyLoaded != No {
		t.Fatalf("family_loaded=%s, want no from readable kmutil inventory; rdma=%+v", l.RDMA.FamilyLoaded, l.RDMA)
	}
}

type kmutilDiagnosticErrorPlatform struct {
	*platform.Mock
	output string
}

type rdmaVersionProbePlatform struct {
	*platform.Mock
	version        string
	versionErr     error
	inventory      string
	inventoryErr   error
	inventoryCalls int
}

func (p *rdmaVersionProbePlatform) Command(name string, args ...string) ([]byte, error) {
	if name == "sysctl" {
		return []byte(p.version), p.versionErr
	}
	if name == "kmutil" {
		p.inventoryCalls++
		return []byte(p.inventory), p.inventoryErr
	}
	return p.Mock.Command(name, args...)
}

func TestCollect_RDMAVersionRequiresSuccessfulRecognizedShape(t *testing.T) {
	const historicalVersion = "Jul 11 2026 14:22:31 RDMA_family\n"
	denied := errors.New("exit status 1")
	for _, version := range []struct {
		name    string
		output  string
		err     error
		trusted bool
	}{
		{"historical-success", historicalVersion, nil, true},
		{"denied-nonempty", "sysctl: Operation not permitted\n", denied, false},
		{"malformed-success", "not a version\n", nil, false},
		{"diagnostic-marker-error", "sysctl: RDMA_family: Operation not permitted\n", denied, false},
		{"historical-shape-error", historicalVersion, denied, false},
		{"diagnostic-marker-success", "sysctl: RDMA_family unavailable\n", nil, false},
		{"invalid-date-success", "Jul 99 2026 14:22:31 RDMA_family\n", nil, false},
		{"trailing-diagnostic-success", historicalVersion + "unexpected diagnostic\n", nil, false},
		{"bare-marker-success", "RDMA_family\n", nil, false},
	} {
		for _, inventory := range []struct {
			name   string
			output string
			err    error
			want   Tri
		}{
			{"loaded", kmutilHeader + kmutilFamilyRow, nil, Yes},
			{"absent", kmutilHeader + kmutilZeroRow, nil, No},
			{"failed-even-with-family", kmutilHeader + kmutilFamilyRow, denied, Unknown},
			{"malformed-success", "diagnostic IORDMAFamily\n", nil, Unknown},
		} {
			t.Run(version.name+"/"+inventory.name, func(t *testing.T) {
				p := &rdmaVersionProbePlatform{
					Mock: mock(spNoDevices), version: version.output, versionErr: version.err,
					inventory: inventory.output, inventoryErr: inventory.err,
				}
				l := Collect(p)
				want, calls := inventory.want, 1
				if version.trusted {
					want, calls = Yes, 0
				}
				if l.RDMA.FamilyLoaded != want || p.inventoryCalls != calls {
					t.Fatalf("rdma=%+v inventory calls=%d, want family=%s calls=%d", l.RDMA, p.inventoryCalls, want, calls)
				}
				if want == Unknown && (l.RDMA.Reason == "" || len(l.Unknown) == 0) {
					t.Fatal("unknown fallback must retain an observable reason")
				}
				if l.RDMA.NVRAMEnabled != No {
					t.Fatal("legacy version handling changed NVRAM reporting")
				}
			})
		}
	}
}

func (p kmutilDiagnosticErrorPlatform) Command(name string, args ...string) ([]byte, error) {
	if name == "kmutil" {
		return []byte(p.output), errors.New("exit status 1")
	}
	return p.Mock.Command(name, args...)
}

func TestCollect_RDMASysctlAbsentAndKMUtilFailureStaysUnknown(t *testing.T) {
	m := mock(spNoDevices)
	m.CommandResults["sysctl -n debug.rdma.version"] = "sysctl: unknown oid 'debug.rdma.version'\n"
	for _, output := range []string{"kmutil: Operation not permitted\n", kmutilHeader + kmutilFamilyRow} {
		l := Collect(kmutilDiagnosticErrorPlatform{m, output})
		if l.RDMA.FamilyLoaded != Unknown || l.RDMA.Reason == "" {
			t.Fatalf("family_loaded=%s, want unknown with reason when kmutil fails; rdma=%+v", l.RDMA.FamilyLoaded, l.RDMA)
		}
	}
}

func TestCollect_RDMAInventoryValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		want   Tri
	}{
		{"observed-zero-and-hex-rows", kmutilHeader + kmutilZeroRow + kmutilHexRow, No},
		{"observed-loaded", kmutilHeader + kmutilZeroRow + kmutilFamilyRow + kmutilDriverRow, Yes},
		{"observed-stderr-prefix", "No variant specified, falling back to release\n" + kmutilHeader + kmutilFamilyRow, Yes},
		{"bare-family-word", "IORDMAFamily", Unknown},
		{"arbitrary-success", "not an inventory", Unknown},
		{"denied-success", "kmutil: Operation not permitted\n", Unknown},
		{"empty-success", "", Unknown},
		{"header-only", kmutilHeader, Unknown},
		{"row-without-header", kmutilFamilyRow, Unknown},
		{"truncated-family-row", kmutilHeader + "235 1 0 0 0 com.apple.iokit.IORDMAFamily (1.0)\n", Unknown},
		{"wrong-header", "Index Name\n" + kmutilFamilyRow, Unknown},
		{"invalid-index", kmutilHeader + strings.Replace(kmutilFamilyRow, "235", "oops", 1), Unknown},
		{"invalid-address", kmutilHeader + strings.Replace(kmutilFamilyRow, "0xfffffe0002000000", "not-address", 1), Unknown},
		{"invalid-uuid", kmutilHeader + strings.Replace(kmutilFamilyRow, "A266614B-439F-3B38-9862-F6D245F8327D", "not-uuid", 1), Unknown},
		{"invalid-dependencies", kmutilHeader + strings.Replace(kmutilFamilyRow, "<9 8 7 6 5 3>", "<oops>", 1), Unknown},
		{"family-substring-not-exact", kmutilHeader + strings.Replace(kmutilFamilyRow, "com.apple.iokit.IORDMAFamily", "com.apple.iokit.IORDMAFamilyOther", 1), No},
		{"driver-only-is-not-family", kmutilHeader + kmutilDriverRow, No},
		{"malformed-after-family", kmutilHeader + kmutilFamilyRow + "garbage IORDMAFamily\n", Unknown},
		{"unrecognized-prefix", "unexpected notice\n" + kmutilHeader + kmutilFamilyRow, Unknown},
		{"duplicate-prefix", strings.Repeat("No variant specified, falling back to release\n", 2) + kmutilHeader + kmutilFamilyRow, Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := mock(spNoDevices)
			m.CommandResults["sysctl -n debug.rdma.version"] = "sysctl: unknown oid 'debug.rdma.version'\n"
			m.CommandResults["kmutil showloaded"] = tc.output
			l := Collect(m)
			if l.RDMA.FamilyLoaded != tc.want {
				t.Fatalf("family_loaded=%s, want %s; rdma=%+v", l.RDMA.FamilyLoaded, tc.want, l.RDMA)
			}
			if tc.want == Unknown && (l.RDMA.Reason == "" || len(l.Unknown) == 0) {
				t.Fatal("unreadable inventory must expose an unknown reason")
			}
			if l.RDMA.NVRAMEnabled != No {
				t.Fatal("inventory validation changed separate NVRAM reporting")
			}
		})
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
