package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

func TestRelieveMemoryTopUserUsesFootprintThenRSSFallback(t *testing.T) {
	tests := []struct {
		name string
		proc guard.MemProc
		want string
	}{
		{
			name: "physical footprint",
			proc: guard.MemProc{
				Name:      "/Applications/Apollo.app/Contents/MacOS/Apollo",
				RSS:       100 << 20,
				Footprint: 133 * (1 << 30) / 10,
			},
			want: "Apollo (13.3 GB)",
		},
		{
			name: "RSS fallback",
			proc: guard.MemProc{Name: "Apollo", RSS: 100 << 20},
			want: "Apollo (0.1 GB)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relieveMemoryTopUser(tt.proc); got != tt.want {
				t.Fatalf("relieveMemoryTopUser() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRelieveMemoryPreviewSeparatesPressureFromProcessSample(t *testing.T) {
	tests := []struct {
		name             string
		pressure         guard.PressureLevel
		wantContext      string
		wantNoCauseClaim bool
	}{
		{name: "normal", pressure: guard.PressureNormal, wantContext: "Memory pressure is normal; this sample identifies no active pressure cause.", wantNoCauseClaim: true},
		{name: "warn", pressure: guard.PressureWarn, wantContext: "Memory pressure is elevated (warn)."},
		{name: "unknown", pressure: guard.PressureUnknown, wantContext: "Memory pressure is unknown; this sample cannot identify an active pressure cause.", wantNoCauseClaim: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildRelieveMemoryPreview(guard.NodeCapacity{
				FreeRAM:  21 << 30,
				Pressure: tt.pressure,
			}, "apollo-plain-native-serve (13.3 GB)")

			if result.Command != "sirsi relieve --memory" || result.Status != "preview" {
				t.Fatalf("command/status = %q/%q, want memory-relief preview", result.Command, result.Status)
			}
			if !strings.Contains(result.Summary, tt.wantContext) {
				t.Fatalf("summary %q does not retain pressure context %q", result.Summary, tt.wantContext)
			}
			if !strings.Contains(result.Summary, "does not release memory actively held by apps") ||
				!strings.Contains(result.Summary, "not as a recommendation to stop it") {
				t.Fatalf("summary lacks cache/active-working-set distinction: %q", result.Summary)
			}
			if tt.wantNoCauseClaim && strings.Contains(result.Summary, "To free more, quit") {
				t.Fatalf("non-elevated or unknown pressure recommended stopping a process: %q", result.Summary)
			}
			if len(result.Evidence) != 3 || result.Evidence[0].Label != "Largest sampled process" {
				t.Fatalf("evidence = %+v, want process context followed by free memory and pressure", result.Evidence)
			}
			if result.Evidence[2].Value != tt.pressure.String() {
				t.Fatalf("pressure evidence = %q, want %q", result.Evidence[2].Value, tt.pressure.String())
			}
			if len(result.NextActions) != 1 || result.NextActions[0].Command != "relieve --memory --confirm" {
				t.Fatalf("next actions = %+v, want only the existing cache-flush confirmation", result.NextActions)
			}
			if !strings.Contains(result.NextActions[0].Description, "no app is stopped or active memory released") {
				t.Fatalf("action description overstates the cache flush: %+v", result.NextActions[0])
			}
			if strings.Contains(result.NextActions[0].Command, "apollo-plain-native-serve") ||
				strings.Contains(result.NextActions[0].Command, "kill") ||
				strings.Contains(result.NextActions[0].Command, "stop") {
				t.Fatalf("preview proposed terminating an active workload: %+v", result.NextActions)
			}
		})
	}
}

func TestPurgeCachesNeverPromptsUnattended(t *testing.T) {
	cases := []struct {
		name        string
		sudoFails   bool
		unattended  bool
		wantSkipped bool
		wantOsa     bool
	}{
		{"sudo ok, unattended", false, true, false, false},
		{"sudo ok, interactive", false, false, false, false},
		{"sudo fails, unattended: skip, no dialog", true, true, true, false},
		{"sudo fails, interactive: dialog fallback", true, false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			osa := false
			run := func(name string, _ ...string) ([]byte, error) {
				if name == "osascript" {
					osa = true
					return nil, nil
				}
				if c.sudoFails {
					return []byte("sudo: a password is required"), errors.New("exit 1")
				}
				return nil, nil
			}
			skipped, _, err := purgeCaches(run, c.unattended)
			if skipped != c.wantSkipped || osa != c.wantOsa {
				t.Fatalf("skipped=%v osascript=%v, want skipped=%v osascript=%v", skipped, osa, c.wantSkipped, c.wantOsa)
			}
			if err != nil && !c.wantSkipped {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}
