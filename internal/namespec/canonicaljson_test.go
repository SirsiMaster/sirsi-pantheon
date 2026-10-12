package namespec

import (
	"os"
	"path/filepath"
	"strings"
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

// TestSchemaHashBypassesCanonicalizeChecks_NowRejects: codex-pantheon
// CHANGES_REQUESTED (PR #1056, head 443f42f8) — SchemaHash ran its own
// independent decode that skipped canonicalize's UTF-8 validity and
// trailing-content checks. Both entry points now share parseCanonicalValue,
// so SchemaHash must refuse exactly what canonicalize refuses.
func TestSchemaHashBypassesCanonicalizeChecks_NowRejects(t *testing.T) {
	invalidUTF8 := append([]byte(`{"a":"`), 0xff, 0xfe)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	if _, err := SchemaHash(invalidUTF8); err == nil {
		t.Error("SchemaHash should reject invalid UTF-8 like canonicalize does, got nil error")
	}

	trailing := []byte(`{"a":1}garbage`)
	if _, err := SchemaHash(trailing); err == nil {
		t.Error("SchemaHash should reject trailing content after the top-level value, got nil error")
	}
}

// TestEncodeCanonicalString_DoesNotHTMLEscapeOrEscapeLineSeparators:
// codex-pantheon CHANGES_REQUESTED (PR #1056) — encodeCanonicalString used
// json.Marshal, which HTML-escapes '<','>','&' and unconditionally escapes
// U+2028/U+2029. RFC 8785 JCS forbids escaping any of those; a schema's hash
// must be reproducible by any compliant JCS implementation, not just Go's.
func TestEncodeCanonicalString_DoesNotHTMLEscapeOrEscapeLineSeparators(t *testing.T) {
	raw := []byte("{\"a\":\"<tag>&amp;\u2028\u2029\"}")
	out, err := canonicalize(raw)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	got := string(out)
	for _, escaped := range []string{`\u003c`, `\u003e`, `\u0026`, `\u2028`, `\u2029`} {
		if strings.Contains(got, escaped) {
			t.Errorf("canonical output must not contain the escape %s (JCS forbids it), got %q", escaped, got)
		}
	}
	want := "{\"a\":\"<tag>&amp;\u2028\u2029\"}"
	if got != want {
		t.Errorf("expected the literal characters unescaped in output: got %q, want %q", got, want)
	}
}

// TestCanonicalize_RejectsLoneSurrogateEscapes: codex-pantheon
// CHANGES_REQUESTED (PR #1056) — a lone (unpaired) UTF-16 surrogate escape is
// invalid Unicode text; encoding/json's decoder silently substitutes U+FFFD
// instead of erroring, so the hash would be computed over corrupted input
// without anyone knowing. Both a lone high surrogate and a lone low
// surrogate must be refused; a correctly paired surrogate (a real character
// outside the BMP) must still be accepted.
func TestCanonicalize_RejectsLoneSurrogateEscapes(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"lone high surrogate", `{"a":"\uD800"}`, true},
		{"lone low surrogate", `{"a":"\uDC00"}`, true},
		{"high surrogate followed by non-escape", `{"a":"\uD800x"}`, true},
		{"valid surrogate pair", `{"a":"😀"}`, false}, // U+1F600 GRINNING FACE
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := canonicalize([]byte(c.raw))
			if c.wantErr && err == nil {
				t.Errorf("canonicalize(%q): expected an error, got nil", c.raw)
			}
			if !c.wantErr && err != nil {
				t.Errorf("canonicalize(%q): unexpected error: %v", c.raw, err)
			}
		})
	}
}
