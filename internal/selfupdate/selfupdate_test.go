package selfupdate

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func TestDetectMethod(t *testing.T) {
	tests := []struct {
		name string
		path string
		want Method
	}{
		{"empty", "", MethodUnknown},
		{"homebrew arm", "/opt/homebrew/bin/sirsi", MethodHomebrew},
		{"homebrew intel", "/usr/local/bin/sirsi", MethodHomebrew},
		{"raw local", "/Users/x/.local/bin/sirsi", MethodRaw},
		{"go-build cache", "/tmp/go-build123/b001/exe/sirsi", MethodGoRun},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectMethod(tt.path); got != tt.want {
				t.Errorf("DetectMethod(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func sib(binary, ver, path string) Sibling {
	return Sibling{Info: version.Info{Binary: binary, Version: ver, Path: path}, Method: MethodRaw}
}

func TestBuildReport_Healthy(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.22.0", Path: "/Users/x/.local/bin/sirsi"}
	r := BuildReport(self, []Sibling{sib("sirsi-menubar", "v0.22.0", "/opt/homebrew/bin/sirsi-menubar")}, "")
	if !r.Healthy {
		t.Fatalf("expected healthy, got drift: %s", r.Summary())
	}
	if len(r.D2Mismatch) != 0 {
		t.Errorf("D2Mismatch = %d, want 0", len(r.D2Mismatch))
	}
}

func TestBuildReport_D2SiblingDrift(t *testing.T) {
	// The tonight bug: sirsi fresh, menubar stale.
	self := version.Info{Binary: "sirsi", Version: "v0.22.0", Path: "/Users/x/.local/bin/sirsi"}
	r := BuildReport(self, []Sibling{sib("sirsi-menubar", "v0.20.0", "/opt/homebrew/bin/sirsi-menubar")}, "")
	if r.Healthy {
		t.Fatal("expected drift, got healthy")
	}
	if len(r.D2Mismatch) != 1 || r.D2Mismatch[0].Binary != "sirsi-menubar" {
		t.Fatalf("D2Mismatch = %+v, want one sirsi-menubar entry", r.D2Mismatch)
	}
}

func TestBuildReport_IgnoresUnknownAndDevSiblings(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.22.0"}
	siblings := []Sibling{
		sib("sirsi-menubar", "dev", "/x/sirsi-menubar"),                   // unstamped — not a real mismatch
		{Info: version.Info{Binary: "sirsi-menubar"}, Err: "exec failed"}, // unprobeable
	}
	r := BuildReport(self, siblings, "")
	if !r.Healthy {
		t.Errorf("dev/errored siblings must not count as drift: %s", r.Summary())
	}
}

func TestBuildReport_D3PathDrift(t *testing.T) {
	self := version.Info{Binary: "sirsi", Version: "v0.22.0", Path: "/Users/x/.local/bin/sirsi"}
	r := BuildReport(self, nil, "/opt/homebrew/bin/sirsi")
	if r.Healthy {
		t.Fatal("expected D3 path drift")
	}
	if r.D3PathBin != "/opt/homebrew/bin/sirsi" {
		t.Errorf("D3PathBin = %q", r.D3PathBin)
	}
}

func TestSummary_InSync(t *testing.T) {
	r := BuildReport(version.Info{Version: "v0.22.0"}, nil, "")
	if got := r.Summary(); got != "v0.22.0 in sync" {
		t.Errorf("Summary = %q, want 'v0.22.0 in sync'", got)
	}
}
