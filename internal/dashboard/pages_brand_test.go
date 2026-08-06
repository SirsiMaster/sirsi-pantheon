package dashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestPageShellDerivesFromBrand pins the ADR-038 single-palette contract for
// the dashboard: every color comes from internal/brand via :root CSS vars —
// no hardcoded hex or stale-gold rgba literal may reappear in the shell.
func TestPageShellDerivesFromBrand(t *testing.T) {
	html := pageShell("Test", "home", "<p>body</p>", DashboardPort)

	if !strings.Contains(html, ":root{") || !strings.Contains(html, "--gold:") || !strings.Contains(html, "--ok:") {
		t.Fatal("pageShell missing the brand :root CSS-vars block")
	}
	// The %s-injected Color* constants are brand-derived hex — only literals
	// OUTSIDE the :root block are drift. Strip the vars block, then scan.
	stripped := regexp.MustCompile(`:root\{[^}]*\}`).ReplaceAllString(html, "")
	for _, c := range []string{"#C8A951", "#44FF88", "#FF4444", "#FF8844", "#555", "#666", "#444", "#333", "rgba(200,169,81"} {
		if strings.Contains(stripped, c) {
			t.Errorf("hardcoded color literal %q survives in the dashboard shell", c)
		}
	}
	if !strings.Contains(html, "var(--gold)") {
		t.Error("expected classes to reference var(--gold)")
	}
}
