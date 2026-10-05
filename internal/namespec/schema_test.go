package namespec

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadRealSchema loads the committed C2 schema and asserts the router can
// authorize the canonical names, and refuses ones outside the sets — both
// directions (A35). This also guards the schema file itself: if someone drops a
// required project (e.g. `home`), a canonical name stops being allowed and this
// test goes red.
func TestLoadRealSchema(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "naming", "registry-schema-v1.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	s, err := LoadSchema(raw)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	allow := []string{
		"ra-router-m1", "claude-finalwishes-m1", "claude-finalwishes-m5",
		"claude-home-m1", "claude-home-m5", "codex-finalwishes-m5",
		"sirsi-governance-m5", "mercury-mercury-m5", "codex-apollo-m5",
		"claude-finalwishes-m1-fw-r02",
	}
	for _, in := range allow {
		n, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		if err := s.Allows(n); err != nil {
			t.Errorf("schema should allow %q: %v", in, err)
		}
	}
	refuse := []string{
		"claude-notaproject-m1", // unknown project
		"nobody-router-m1",      // unknown agent
		"ra-router-m9",          // unknown machine alias
	}
	for _, in := range refuse {
		n, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) should succeed (grammar ok): %v", in, err)
			continue
		}
		if err := s.Allows(n); err == nil {
			t.Errorf("schema should refuse %q, got nil", in)
		}
	}
}

func TestLoadSchemaRejections(t *testing.T) {
	bad := map[string]string{
		"empty":          `{}`,
		"no version":     `{"agents":["ra"],"projects":["router"],"machines":{"m1":{}}}`,
		"no agents":      `{"schema_version":1,"projects":["router"],"machines":{"m1":{}}}`,
		"no projects":    `{"schema_version":1,"agents":["ra"],"machines":{"m1":{}}}`,
		"no machines":    `{"schema_version":1,"agents":["ra"],"projects":["router"]}`,
		"bad agent slot": `{"schema_version":1,"agents":["Ra"],"projects":["router"],"machines":{"m1":{}}}`,
		"bad machine":    `{"schema_version":1,"agents":["ra"],"projects":["router"],"machines":{"M1":{}}}`,
		"unknown field":  `{"schema_version":1,"agents":["ra"],"projects":["router"],"machines":{"m1":{}},"rogue":true}`,
		"malformed json": `{`,
	}
	for name, doc := range bad {
		if _, err := LoadSchema([]byte(doc)); err == nil {
			t.Errorf("LoadSchema(%s) = nil error, want rejection", name)
		}
	}
}
