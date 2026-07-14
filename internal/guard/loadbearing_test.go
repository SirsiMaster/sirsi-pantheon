package guard

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ── isLoadBearingWith — the core classifier ──────────────────────────────────

func TestIsLoadBearingWith(t *testing.T) {
	tests := []struct {
		name        string
		proc        ProcessInfo
		wantLB      bool
		reasonMatch string // substring the reason must contain when load-bearing
	}{
		{
			name: "gemma-capped-server is load-bearing (the forensic case)",
			proc: ProcessInfo{
				PID:        1210,
				Name:       "Python",
				Command:    "/Library/Frameworks/Python.framework/Versions/3.12/Resources/Python.app/Contents/MacOS/Python /Users/x/.sirsi/gemma-capped-server.py 27204354048 --model mlx-community/gemma-4-12B-it-8bit --host 127.0.0.1 --port 11434",
				RSS:        27 << 30,
				CPUPercent: 19.1,
			},
			wantLB:      true,
			reasonMatch: "sizing lever",
		},
		{
			name:        "ollama serve is load-bearing",
			proc:        ProcessInfo{PID: 2000, Name: "ollama", Command: "ollama serve"},
			wantLB:      true,
			reasonMatch: "smaller/more-quantized",
		},
		{
			name:        "generic mlx model server is load-bearing (heuristic)",
			proc:        ProcessInfo{PID: 2001, Name: "python3", Command: "python3 -m some.server --model /models/x.safetensors --port 8080"},
			wantLB:      true,
			reasonMatch: "model server",
		},
		{
			name:        "llama-server is load-bearing",
			proc:        ProcessInfo{PID: 2002, Name: "llama-server", Command: "/opt/llama.cpp/llama-server -m /models/q4.gguf --port 8081"},
			wantLB:      true,
			reasonMatch: "evict-when-idle",
		},
		{
			name:   "plain node build is NOT load-bearing (must stay killable)",
			proc:   ProcessInfo{PID: 3000, Name: "node", Command: "node /Users/x/proj/build.js --watch"},
			wantLB: false,
		},
		{
			name:   "loopback dev web server WITHOUT a model is NOT load-bearing",
			proc:   ProcessInfo{PID: 3001, Name: "python3", Command: "python3 -m http.server --port 8000 --host 127.0.0.1"},
			wantLB: false,
		},
		{
			name:   "empty command is NOT load-bearing (cannot misidentify what we can't read)",
			proc:   ProcessInfo{PID: 3002, Name: "", Command: ""},
			wantLB: false,
		},
	}

	// A mock whose ps probes return nothing forces the deterministic argv path
	// (proc.Command) — no reliance on live enrichment.
	m := &platform.Mock{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLB, reason := isLoadBearingWith(m, tt.proc)
			if gotLB != tt.wantLB {
				t.Fatalf("isLoadBearingWith() = %v, want %v (reason=%q)", gotLB, tt.wantLB, reason)
			}
			if tt.wantLB {
				if reason == "" {
					t.Fatal("load-bearing process must carry a reason")
				}
				if tt.reasonMatch != "" && !strings.Contains(reason, tt.reasonMatch) {
					t.Errorf("reason %q missing %q", reason, tt.reasonMatch)
				}
				// The reason must steer AWAY from killing, toward the sizing lever.
				if strings.Contains(strings.ToLower(reason), "kill it to reclaim") {
					t.Errorf("reason must not recommend killing: %q", reason)
				}
			}
		})
	}
}

// isProtectedProcessWith must treat a load-bearing server as protected — this is
// the single chokepoint that guards BOTH SlayWith and KillTrueOrphans.
func TestIsProtectedProcessWith_LoadBearing(t *testing.T) {
	m := &platform.Mock{}
	gemma := ProcessInfo{
		PID:     1210,
		Name:    "Python",
		User:    "user",
		Command: "/…/Python /Users/x/.sirsi/gemma-capped-server.py 27204354048 --model gemma-4-12B-it-8bit --port 11434",
	}
	if !isProtectedProcessWith(m, gemma) {
		t.Fatal("gemma-capped-server must be protected from the slayer")
	}

	plainNode := ProcessInfo{PID: 3000, Name: "node", User: "user", Command: "node build.js"}
	if isProtectedProcessWith(m, plainNode) {
		t.Error("a plain node build must remain killable")
	}
}

// SlayWith must spare a load-bearing server AND record the reason in Protected.
func TestSlayWith_SparesLoadBearingWithReason(t *testing.T) {
	m := &platform.Mock{
		CommandResults: map[string]string{
			// A gemma model server classified into the "ai"/other groups by argv.
			"ps -axo pid,rss,vsz,%cpu,user,command": "  PID   RSS   VSZ  %CPU USER  COMMAND\n" +
				" 1210 27000000 40000000 19.1 user /usr/bin/ollama serve --port 11434\n",
		},
	}
	// SlayAll targets known orphan groups; ollama classifies via orphanPatterns/heuristics.
	res, err := SlayWith(m, SlayAll, false)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	// Whether or not it landed in a slay group, if it was a target it must be
	// spared — never killed. Assert it was not killed.
	if res.Killed > 0 {
		t.Errorf("a load-bearing server must never be killed; Killed=%d", res.Killed)
	}
	// When it IS caught as a target, it is recorded in Protected with a lever hint.
	for _, prot := range res.Protected {
		if prot.PID == 1210 && !strings.Contains(prot.Reason, "sizing lever") {
			t.Errorf("protected reason must name the sizing lever: %q", prot.Reason)
		}
	}
}

// fullCommand must fetch argv on demand when Command is a bare executable, and
// trust Command when it already carries args.
func TestFullCommand(t *testing.T) {
	// Bare interpreter → fetch via ps.
	m := &platform.Mock{
		CommandResults: map[string]string{
			"ps -o command= -p 1210": "/…/Python /Users/x/.sirsi/gemma-capped-server.py --port 11434\n",
		},
	}
	got := fullCommand(m, ProcessInfo{PID: 1210, Command: "Python"})
	if !strings.Contains(got, "gemma-capped-server.py") {
		t.Errorf("fullCommand should fetch argv on demand, got %q", got)
	}

	// Already has args → trusted as-is, no probe.
	m2 := &platform.Mock{}
	got2 := fullCommand(m2, ProcessInfo{PID: 42, Command: "node build.js --watch"})
	if got2 != "node build.js --watch" {
		t.Errorf("fullCommand should trust argv-carrying Command, got %q", got2)
	}
}
