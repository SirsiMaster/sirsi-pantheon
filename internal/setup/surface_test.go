package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectableSurfaces(t *testing.T) {
	sel := Selectable()
	if len(sel) != 3 {
		t.Fatalf("expected 3 selectable surfaces, got %d", len(sel))
	}
	want := map[Surface]bool{SurfaceCLI: true, SurfaceTUI: true, SurfaceIDE: true}
	for _, s := range sel {
		if !want[s] {
			t.Errorf("unexpected selectable surface %q", s)
		}
		if s.Title() == "" || s.Detail() == "" {
			t.Errorf("surface %q missing Title/Detail", s)
		}
	}
	// Menubar is installed by default, never user-selectable.
	for _, s := range sel {
		if s == SurfaceMenubar {
			t.Error("menubar must not appear in the selectable set")
		}
	}
}

func TestMCPConfigSnippet(t *testing.T) {
	snip := MCPConfigSnippet()
	for _, sub := range []string{"mcpServers", "sirsi", `"mcp"`} {
		if !strings.Contains(snip, sub) {
			t.Errorf("MCP snippet missing %q: %s", sub, snip)
		}
	}
}

func TestActiveSurfaceDefault(t *testing.T) {
	// Point HOME at a temp dir with no preference file → must default to CLI.
	t.Setenv("HOME", t.TempDir())
	if got := ActiveSurface(); got != SurfaceCLI {
		t.Errorf("ActiveSurface() with no pref = %q, want cli", got)
	}
}

func TestSaveAndLoadActiveSurface(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := SaveActiveSurface(SurfaceIDE); err != nil {
		t.Fatalf("SaveActiveSurface: %v", err)
	}
	if got := ActiveSurface(); got != SurfaceIDE {
		t.Errorf("ActiveSurface() = %q, want ide", got)
	}
	// File should live under ~/.config/sirsi/surface.
	if _, err := os.Stat(filepath.Join(home, ".config", "sirsi", "surface")); err != nil {
		t.Errorf("preference file not written: %v", err)
	}
}

func TestActiveSurfaceRejectsGarbage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".config", "sirsi", "surface")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bogus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ActiveSurface(); got != SurfaceCLI {
		t.Errorf("garbage preference should fall back to cli, got %q", got)
	}
}
