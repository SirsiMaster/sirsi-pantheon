package help

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// captureStdout runs fn while capturing everything written to os.Stdout.
// ANSI escape sequences are stripped so assertions are independent of the
// terminal color profile lipgloss detects (styled underline splits words
// with escape codes).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return ansiRE.ReplaceAllString(buf.String(), "")
}

func TestAllDeities(t *testing.T) {
	all := AllDeities()
	if len(all) == 0 {
		t.Fatal("AllDeities returned no deities")
	}

	// Every listed deity must have a guide, and every guide must be listed.
	g := guides()
	if len(all) != len(g) {
		t.Errorf("AllDeities has %d entries, guides() has %d", len(all), len(g))
	}
	for _, name := range all {
		if _, ok := g[name]; !ok {
			t.Errorf("deity %q listed in AllDeities but has no guide", name)
		}
	}

	// Sorted ascending.
	for i := 1; i < len(all); i++ {
		if all[i-1] >= all[i] {
			t.Errorf("AllDeities not sorted: %q before %q", all[i-1], all[i])
		}
	}
}

func TestGuidesContent(t *testing.T) {
	for name, g := range guides() {
		if g.Name == "" {
			t.Errorf("guide %q: empty Name", name)
		}
		if g.Glyph == "" {
			t.Errorf("guide %q: empty Glyph", name)
		}
		if g.Tagline == "" {
			t.Errorf("guide %q: empty Tagline", name)
		}
		if len(g.Steps) == 0 {
			t.Errorf("guide %q: no steps", name)
		}
		for i, s := range g.Steps {
			if s.Title == "" || s.Body == "" {
				t.Errorf("guide %q step %d: empty title or body", name, i)
			}
		}
		if len(g.Examples) == 0 {
			t.Errorf("guide %q: no examples", name)
		}
	}
}

func TestDocsURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"thoth", "https://sirsi.ai/pantheon/thoth.html"},
		{"MAAT", "https://sirsi.ai/pantheon/maat.html"},
		{"Osiris", "https://sirsi.ai/pantheon/osiris.html"},
	}
	for _, tt := range tests {
		if got := docsURL(tt.in); got != tt.want {
			t.Errorf("docsURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShowGuideUnknownDeity(t *testing.T) {
	err := ShowGuide("zeus")
	if err == nil {
		t.Fatal("ShowGuide(zeus) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "zeus") {
		t.Errorf("error should mention the unknown deity, got: %v", err)
	}
}

func TestShowGuideKnownDeity(t *testing.T) {
	out := captureStdout(t, func() {
		if err := ShowGuide("thoth"); err != nil {
			t.Errorf("ShowGuide(thoth) unexpected error: %v", err)
		}
	})

	for _, want := range []string{
		"Thoth",
		"Persistent Knowledge",
		"Initialize a project",
		"sirsi thoth init --yes --name myproject",
		"https://sirsi.ai/pantheon/thoth.html",
		"Platform:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ShowGuide(thoth) output missing %q", want)
		}
	}
}

func TestShowGuideNormalizesInput(t *testing.T) {
	// Mixed case with surrounding whitespace should resolve.
	out := captureStdout(t, func() {
		if err := ShowGuide("  MaAt  "); err != nil {
			t.Errorf("ShowGuide('  MaAt  ') unexpected error: %v", err)
		}
	})
	if !strings.Contains(out, "Ma'at") {
		t.Error("ShowGuide('  MaAt  ') output missing guide title")
	}
}

func TestShowGuideAllDeitiesRender(t *testing.T) {
	for _, name := range AllDeities() {
		out := captureStdout(t, func() {
			if err := ShowGuide(name); err != nil {
				t.Errorf("ShowGuide(%q) unexpected error: %v", name, err)
			}
		})
		if !strings.Contains(out, "Web docs:") {
			t.Errorf("ShowGuide(%q) output missing footer", name)
		}
	}
}

func TestListGuides(t *testing.T) {
	out := captureStdout(t, ListGuides)

	if !strings.Contains(out, "Available Pantheon Guides") {
		t.Error("ListGuides output missing title")
	}
	for _, name := range AllDeities() {
		if !strings.Contains(out, name) {
			t.Errorf("ListGuides output missing deity %q", name)
		}
	}
	if !strings.Contains(out, "sirsi help <deity>") {
		t.Error("ListGuides output missing usage hint")
	}
}

func TestOpenDocsUnknownDeity(t *testing.T) {
	// Guard: even if validation regressed, never launch a real browser.
	orig := startCommand
	t.Cleanup(func() { startCommand = orig })
	startCommand = func(cmd *exec.Cmd) error { return nil }

	if err := OpenDocs("hades"); err == nil {
		t.Fatal("OpenDocs(hades) expected error, got nil")
	}
}

func TestOpenDocsLaunchesBrowser(t *testing.T) {
	orig := startCommand
	t.Cleanup(func() { startCommand = orig })

	var gotArgs []string
	startCommand = func(cmd *exec.Cmd) error {
		gotArgs = cmd.Args
		return nil
	}

	if err := OpenDocs("  Anubis "); err != nil {
		t.Fatalf("OpenDocs(anubis) unexpected error: %v", err)
	}
	if len(gotArgs) == 0 {
		t.Fatal("startCommand was never invoked")
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "https://sirsi.ai/pantheon/anubis.html") {
		t.Errorf("browser command %q missing docs URL", joined)
	}
}

func TestOpenDocsStartFailure(t *testing.T) {
	orig := startCommand
	t.Cleanup(func() { startCommand = orig })
	startCommand = func(cmd *exec.Cmd) error { return errors.New("boom") }

	err := OpenDocs("isis")
	if err == nil {
		t.Fatal("OpenDocs expected error when browser launch fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open browser") {
		t.Errorf("unexpected error: %v", err)
	}
}
