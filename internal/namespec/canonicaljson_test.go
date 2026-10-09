package namespec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalizeSortsKeysAndStripsWhitespace(t *testing.T) {
	got, err := canonicalize([]byte(`{ "b": 1, "a": [1, 2, 3], "c": { "z": true, "y": false } }`))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	want := `{"a":[1,2,3],"b":1,"c":{"y":false,"z":true}}`
	if string(got) != want {
		t.Errorf("canonicalize = %s, want %s", got, want)
	}
}

func TestCanonicalizeRejectsDuplicateKey(t *testing.T) {
	if _, err := canonicalize([]byte(`{"a":1,"a":2}`)); err == nil {
		t.Error("canonicalize should reject a duplicate key, got nil error")
	}
}

func TestCanonicalizeRejectsNonUTF8(t *testing.T) {
	bad := append([]byte(`{"a":"`), 0xff, 0xfe)
	bad = append(bad, []byte(`"}`)...)
	if _, err := canonicalize(bad); err == nil {
		t.Error("canonicalize should reject non-UTF-8 input, got nil error")
	}
}

func TestCanonicalizeRejectsTrailingContent(t *testing.T) {
	if _, err := canonicalize([]byte(`{"a":1} {"b":2}`)); err == nil {
		t.Error("canonicalize should reject trailing content after the top-level value, got nil error")
	}
}

func TestCanonicalizeRejectsNonIntegerNumber(t *testing.T) {
	if _, err := canonicalize([]byte(`{"a":1.5}`)); err == nil {
		t.Error("canonicalize should reject a non-integer number (scoped to integers), got nil error")
	}
}

// TestSchemaHashExcludesItself asserts the hash is computed over the document
// with "schema_hash" removed, so a document that merely records its own
// previously-computed hash still verifies: embedding the hash can never
// change the hash it claims to be (ADR-072 C2 — "a hash that includes itself
// proves nothing").
func TestSchemaHashExcludesItself(t *testing.T) {
	without := []byte(`{"schema_version":1,"agents":["ra"]}`)
	h, err := SchemaHash(without)
	if err != nil {
		t.Fatalf("SchemaHash(without): %v", err)
	}
	with := []byte(`{"schema_version":1,"agents":["ra"],"schema_hash":"` + h + `"}`)
	h2, err := SchemaHash(with)
	if err != nil {
		t.Fatalf("SchemaHash(with): %v", err)
	}
	if h != h2 {
		t.Errorf("SchemaHash changed when schema_hash was embedded: %s vs %s", h, h2)
	}
}

// TestSchemaHashIsKeyOrderIndependent: two documents differing only in key
// order and whitespace must hash identically — that's the entire point of
// canonicalizing before hashing.
func TestSchemaHashIsKeyOrderIndependent(t *testing.T) {
	a := []byte(`{"schema_version":1,"agents":["ra","codex"]}`)
	b := []byte(`{  "agents"  :  [ "ra" , "codex" ] ,  "schema_version" : 1  }`)
	ha, err := SchemaHash(a)
	if err != nil {
		t.Fatalf("SchemaHash(a): %v", err)
	}
	hb, err := SchemaHash(b)
	if err != nil {
		t.Fatalf("SchemaHash(b): %v", err)
	}
	if ha != hb {
		t.Errorf("SchemaHash should be key-order independent: %s vs %s", ha, hb)
	}
}

// TestSchemaHashDetectsRealChange: the hash is not a trivial constant — an
// actual payload change (not just the excluded field) must change it.
func TestSchemaHashDetectsRealChange(t *testing.T) {
	a := []byte(`{"schema_version":1,"agents":["ra"]}`)
	b := []byte(`{"schema_version":2,"agents":["ra"]}`)
	ha, _ := SchemaHash(a)
	hb, _ := SchemaHash(b)
	if ha == hb {
		t.Error("SchemaHash should differ when the payload differs")
	}
}

func TestSchemaHashRejectsNonObject(t *testing.T) {
	if _, err := SchemaHash([]byte(`[1,2,3]`)); err == nil {
		t.Error("SchemaHash should reject a non-object top level, got nil error")
	}
}

// TestSchemaHashOfRealSchemaIsStable pins the actual committed schema's hash
// against a second independent computation (not a literal, since the exact
// digest is not itself meaningful outside a promotion receipt) — this guards
// that the function is deterministic across repeated calls on the real file.
func TestSchemaHashOfRealSchemaIsStable(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "naming", "registry-schema-v1.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	h1, err := SchemaHash(raw)
	if err != nil {
		t.Fatalf("SchemaHash: %v", err)
	}
	h2, err := SchemaHash(raw)
	if err != nil {
		t.Fatalf("SchemaHash: %v", err)
	}
	if h1 != h2 {
		t.Errorf("SchemaHash is not deterministic: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("SchemaHash should be a 64-char hex sha256, got %d chars", len(h1))
	}
}
