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
		"sirsi-governance-m5", "hermes-hermes-m5", "codex-apollo-m5",
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

// TestAssignWith covers the router name-authority core: components -> the one
// canonical name, refusing out-of-schema components (A35, both directions).
func TestAssignWith(t *testing.T) {
	raw, _ := os.ReadFile(filepath.Join("..", "..", "contracts", "naming", "registry-schema-v1.json"))
	s, err := LoadSchema(raw)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	// valid: components construct the canonical name
	n, err := AssignWith("claude", "finalwishes", "fw-r02", "m1", s)
	if err != nil {
		t.Fatalf("AssignWith valid: %v", err)
	}
	if n.String() != "claude-finalwishes-m1-fw-r02" {
		t.Errorf("AssignWith name = %q", n.String())
	}
	// no task
	if n2, _ := AssignWith("ra", "router", "", "m1", s); n2.String() != "ra-router-m1" {
		t.Errorf("AssignWith no-task = %q", n2.String())
	}
	// refusals: out-of-schema agent/project/machine, malformed component
	bad := [][4]string{
		{"nobody", "router", "", "m1"},           // agent not in schema
		{"claude", "notaproject", "", "m1"},      // project not in schema
		{"ra", "router", "", "m9"},               // machine not in schema
		{"claude", "final_wishes", "", "m1"},     // malformed project
		{"claude", "finalwishes", "bad_t", "m1"}, // malformed task
	}
	for _, b := range bad {
		if _, err := AssignWith(b[0], b[1], b[2], b[3], s); err == nil {
			t.Errorf("AssignWith(%v) should be refused, got nil", b)
		}
	}
}

// fakeReader is a SchemaReader stub for the C2 origin-read.
type fakeReader struct {
	raw    []byte
	exists bool
	err    error
}

func (f fakeReader) ReadFile(repo, path, ref string) ([]byte, bool, error) {
	return f.raw, f.exists, f.err
}

// TestLoadSchemaFromOrigin: the router loads the schema from origin (C2); a
// missing-on-origin or read error is refused, never a permissive default (A35).
func TestLoadSchemaFromOrigin(t *testing.T) {
	real, _ := os.ReadFile(filepath.Join("..", "..", "contracts", "naming", "registry-schema-v1.json"))
	if _, err := LoadSchemaFromOrigin(fakeReader{raw: real, exists: true}, "SirsiMaster/sirsi-pantheon"); err != nil {
		t.Errorf("origin read of the real schema should load: %v", err)
	}
	if _, err := LoadSchemaFromOrigin(fakeReader{exists: false}, "r"); err == nil {
		t.Errorf("missing-on-origin must error (C2/A37), got nil")
	}
	if _, err := LoadSchemaFromOrigin(fakeReader{err: errFake}, "r"); err == nil {
		t.Errorf("read error must propagate, got nil")
	}
}

var errFake = fmtErr("boom")

func fmtErr(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
