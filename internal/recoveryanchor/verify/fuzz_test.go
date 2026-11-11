package verify

import (
	"bytes"
	"os"
	"testing"
)

func FuzzEnvelope(f *testing.F) {
	if b, err := os.ReadFile("testdata/canonical-envelope-valid.json"); err == nil {
		f.Add(b)
	}
	f.Add(goldenEnvelope().Canonical())
	for _, s := range []string{
		"", "{}", "{}\n", "[]", "null", `{"epoch":1,"epoch":2}`, `{"a":{"b":[1,2,{"c":null}]}}`,
		`{"epoch":99999999999999999999999}`, "{\"node_id\":\"\\u0000\"}\n", "\xff\xfe{", `{"epoch":1e400}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		e, err := ParseEnvelope(in)
		if err != nil {
			if _, ok := err.(*DenyError); !ok {
				t.Fatalf("non-DenyError %T", err)
			}
			return
		}
		// Anything accepted must be byte-identical to its canonical form and
		// must round-trip.
		if !bytes.Equal(e.Canonical(), in) {
			t.Fatalf("accepted non-canonical input %q", in)
		}
		if e2, err := ParseEnvelope(e.Canonical()); err != nil || e2 != e {
			t.Fatalf("round trip failed: %v", err)
		}
	})
}

func FuzzSignatureArmor(f *testing.F) {
	f.Add([]byte("-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n"))
	f.Add([]byte(""))
	f.Add([]byte("-----BEGIN SSH SIGNATURE-----\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		if _, err := ParseSignature(in); err != nil {
			if _, ok := err.(*DenyError); !ok {
				t.Fatalf("non-DenyError %T", err)
			}
		}
	})
}

func FuzzGrants(f *testing.F) {
	if b, err := os.ReadFile("testdata/grants-format-example.txt"); err == nil {
		f.Add(b)
	}
	f.Add([]byte("SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA uid=1 verbs=restart-broker\n"))
	f.Add([]byte("\x00\r\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		gs, err := ParseGrants(in)
		if err != nil {
			return
		}
		for _, g := range gs {
			if !validFingerprint(g.Fingerprint) || len(g.Verbs) == 0 {
				t.Fatalf("accepted bad grant %+v", g)
			}
			for _, v := range g.Verbs {
				if !IsVerb(v) {
					t.Fatalf("accepted verb %q", v)
				}
			}
		}
	})
}
