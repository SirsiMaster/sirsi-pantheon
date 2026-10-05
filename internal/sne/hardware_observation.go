package sne

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

// Hardware observations constrain compute launch, never desktop access. The
// operator enrolls a node by pinning SHA's receipt path, node and collector.
// Live memory admission still runs after this independent qualification gate.
func checkHardwareObservation(profile SupervisorProfile, now time.Time, read func(string) ([]byte, error)) error {
	p := profile.SNE
	if p.HardwareObservation == "" && p.HardwareNodeID == "" && p.HardwareCollectorSHA256 == "" {
		return nil // Existing unenrolled profiles retain their live admission.
	}
	deny := func() error {
		return resourceAdmissionError("hardware_observation_hold", "Apollo launch held: fresh node-specific hardware qualification is unavailable or held", "Refresh the enrolled SHA observation without changing protection settings or rebooting; desktop recovery remains separate.")
	}
	collector, err := hex.DecodeString(p.HardwareCollectorSHA256)
	if p.HardwareObservation == "" || p.HardwareNodeID == "" || err != nil || len(collector) != 32 {
		return deny()
	}
	raw, err := read(p.HardwareObservation)
	if err != nil || len(raw) > 2<<20 {
		return deny()
	}
	var receipt struct {
		Schema     string    `json:"schema"`
		Owner      string    `json:"owner"`
		ObservedAt time.Time `json:"observed_at"`
		Collector  string    `json:"collector_sha256"`
		Hosts      map[string]struct {
			CaptureResult string `json:"capture_result"`
			SSHResult     string `json:"ssh_result"`
			Admission     struct {
				Overall    string   `json:"overall"`
				Thermal    string   `json:"thermal"`
				Swap       string   `json:"swap"`
				Protected  string   `json:"protected_state"`
				FileVault  string   `json:"filevault"`
				Assessment string   `json:"assessment_policy"`
				Missing    []string `json:"missing_sections"`
			} `json:"admission"`
		} `json:"hosts"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.Schema != "sirsi.stacklab.hardware-observation.v2" || receipt.Owner != "sirsi-hardware-admin" || receipt.Collector != p.HardwareCollectorSHA256 || receipt.ObservedAt.IsZero() || receipt.ObservedAt.After(now) || now.Sub(receipt.ObservedAt) >= 5*time.Minute {
		return deny()
	}
	host, exists := receipt.Hosts[p.HardwareNodeID]
	a := host.Admission
	if !exists || (host.CaptureResult != "PASS" && host.SSHResult != "PASS") || a.Overall != "PASS_OBSERVATION_ONLY" || a.Thermal != "PASS" || a.Swap != "PASS" || a.Protected != "PASS" || a.FileVault != "PASS" || a.Assessment != "PASS" || len(a.Missing) != 0 {
		return deny()
	}
	return nil
}
