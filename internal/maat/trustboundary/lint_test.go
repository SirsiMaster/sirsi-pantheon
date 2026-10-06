package trustboundary

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var wantMarker = regexp.MustCompile(`(?://|#) want ([A-H](?: [A-H])*)\b`)

// TestFixtures: every `// want X [Y]` marker in testdata must produce exactly
// those rule letters on that line, and nothing else may fire. Each fixture is
// a minimal reproduction of one class from the 2026-10-05/06 sweep (ADR-076).
func TestFixtures(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			want := map[int][]string{}
			for i, l := range strings.Split(string(src), "\n") {
				if m := wantMarker.FindStringSubmatch(l); m != nil {
					want[i+1] = strings.Fields(m[1])
				}
			}
			got := map[int][]string{}
			for _, f := range lintSource(name, src) {
				got[f.Line] = append(got[f.Line], f.Rule)
				sort.Strings(got[f.Line])
			}
			for line, rules := range want {
				if strings.Join(got[line], " ") != strings.Join(rules, " ") {
					t.Errorf("line %d: want %v, got %v", line, rules, got[line])
				}
			}
			for line, rules := range got {
				if _, ok := want[line]; !ok {
					t.Errorf("line %d: unexpected finding %v", line, rules)
				}
			}
		})
	}
}

func TestLintableSkipsTestsAndGenerated(t *testing.T) {
	for p, want := range map[string]bool{
		"api/internal/forms/pack.go":      true,
		"api/internal/forms/pack_test.go": false,
		"web/src/lib/money.ts":            true,
		"web/src/lib/money.test.ts":       false,
		"web/src/routeTree.gen.ts":        false,
		"api/internal/gen/v1/forms.pb.go": false,
		"web/node_modules/x/index.js":     false,
		".githooks/pre-push":              true,
		".github/workflows/ci.yml":        true,
		"docs/ARCHITECTURE.md":            false,
	} {
		if got := Lintable(p); got != want {
			t.Errorf("Lintable(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestReportListsEveryRule(t *testing.T) {
	r := Report()
	for _, letter := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		if !strings.Contains(r, "**"+letter+".**") {
			t.Errorf("report lacks rule %s", letter)
		}
	}
	if !strings.Contains(r, "satisfied") || !strings.Contains(r, "n/a") {
		t.Error("report must tell agents the two allowed verdicts")
	}
}
