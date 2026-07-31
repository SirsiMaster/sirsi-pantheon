package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// The Go resolver must read the SAME file sirsi-brain.sh writes. Two configs
// disagreeing about which brain is active is the dual-source class that
// produced the router's phantom-item P0 — this pins compatibility.
func TestLoadConfReadsTheBashFormat(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Exactly the shape sirsi-brain.sh parses: bare key=value, comments, quotes.
	body := "# orchestrator brain\nprovider=openai\nmodel=\"gpt-4o-mini\"\nendpoint = https://example.test/v1\n\n"
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "orchestrator.conf"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c := LoadConf(home)
	if c.Provider != "openai" || c.Model != "gpt-4o-mini" || c.Endpoint != "https://example.test/v1" {
		t.Fatalf("parsed %+v", c)
	}
}

// A missing config is "unconfigured", never an error. Sirsi must be useful on a
// machine where nothing has been set up, or the local premise is fiction.
func TestMissingConfIsNotFatal(t *testing.T) {
	if c := LoadConf(t.TempDir()); c.Provider != "" || c.Endpoint != "" {
		t.Fatalf("expected empty conf, got %+v", c)
	}
}

// The broker's port is READ, never assumed. Assuming 8080 is how a probe
// reports a healthy broker as dead.
func TestLocalEndpointReadsThePortFile(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte("8765\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := localEndpoint(home); got != "http://127.0.0.1:8765/v1" {
		t.Errorf("localEndpoint = %q", got)
	}
	if got := localEndpoint(t.TempDir()); got != "" {
		t.Errorf("no port file should yield empty endpoint, got %q", got)
	}
}

// The ladder must put the zero-token, offline rung first. Remote is an
// enhancement, never a dependency — if local ever sorts second, every query
// silently starts costing tokens.
func TestLadderPrefersLocalOverRemote(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte("8765"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_REMOTE_API_KEY", "k")
	t.Setenv("SIRSI_REMOTE_ENDPOINT", "https://example.test/v1")

	l := Ladder(t.Context(), home)
	if len(l) != 2 {
		t.Fatalf("ladder has %d rungs, want 2", len(l))
	}
	if l[0].Tier() != TierLocal {
		t.Errorf("first rung is %s, must be local", l[0].Tier())
	}
	if l[1].Tier() != TierRemote {
		t.Errorf("second rung is %s, must be remote", l[1].Tier())
	}
	if !l[0].Caps().Offline {
		t.Error("local rung must report Offline — it is the whole sovereignty claim")
	}
}

// No credentials means no remote rung, never an error. A machine with no cloud
// access must still have a working Sirsi.
func TestNoCredentialsMeansNoRemoteRung(t *testing.T) {
	t.Setenv("SIRSI_REMOTE_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	if r := remoteFromEnv(Conf{Endpoint: "https://example.test/v1"}); r != nil {
		t.Error("built a remote provider with no API key")
	}
}

// Capability honesty: the local broker cannot call tools, and saying otherwise
// would make the loop read a silent no-op as the model declining to act.
func TestLocalDeclaresNoToolCalling(t *testing.T) {
	home := t.TempDir()
	_ = os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755)
	_ = os.WriteFile(filepath.Join(home, ".sirsi", "gemma-server.port"), []byte("8765"), 0o600)
	l := Local(home, Conf{})
	if l == nil {
		t.Fatal("no local provider built")
	}
	if l.Caps().Tools {
		t.Error("local provider claims tool-calling; mlx_lm.server has none")
	}
}

func TestRemoteConfigIsNeverClassifiedAsLocal(t *testing.T) {
	remote := Conf{Provider: "openai", Endpoint: "https://api.example.test/v1", Model: "remote-model"}
	if got := Local(t.TempDir(), remote); got != nil {
		t.Fatalf("remote endpoint classified as local/offline: %+v", got)
	}
	if got := Local(t.TempDir(), Conf{Provider: "gemma", Endpoint: "https://api.example.test/v1"}); got != nil {
		t.Fatalf("non-loopback gemma endpoint classified as local/offline: %+v", got)
	}
}

func TestLoopbackEndpointRecognition(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{"http://127.0.0.1:8765/v1", true},
		{"http://[::1]:8765/v1", true},
		{"http://localhost:8765/v1", true},
		{"https://api.example.test/v1", false},
		{"not a URL", false},
	}
	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			if got := isLoopbackEndpoint(tt.endpoint); got != tt.want {
				t.Fatalf("isLoopbackEndpoint(%q) = %v, want %v", tt.endpoint, got, tt.want)
			}
		})
	}
}
