package verify

import (
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func fakeFP(c byte) string { return "SHA256:" + strings.Repeat(string(c), 42) + "A" }

func TestParseGrantsFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/grants-format-example.txt")
	if err != nil {
		t.Fatal(err)
	}
	gs, err := ParseGrants(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 2 || gs[0].UID != 501 || !gs[0].Allows(VerbRestartBroker) || gs[0].Allows(VerbRemountVolume) ||
		gs[1].UID != 502 || !gs[1].Override || !gs[1].Allows(VerbFetchKnownGoodConfig) {
		t.Fatalf("unexpected parse: %+v", gs)
	}
}

func TestParseGrantsRejects(t *testing.T) {
	a := fakeFP('A')
	b := fakeFP('B')
	cases := map[string]string{
		"bad fingerprint prefix": "MD5:abc uid=1 verbs=restart-broker\n",
		"short fingerprint":      "SHA256:AAAA uid=1 verbs=restart-broker\n",
		"non-canonical base64":   "SHA256:" + strings.Repeat("A", 42) + "B uid=1 verbs=restart-broker\n",
		"duplicate fingerprint":  a + " uid=1 verbs=restart-broker\n" + a + " uid=1 verbs=restart-broker\n",
		"conflicting duplicate":  a + " uid=1 verbs=restart-broker\n" + a + " uid=1 verbs=remount-volume\n",
		"unknown field":          a + " uid=1 verbs=restart-broker shell=yes\n",
		"not key=value":          a + " uid=1 verbs=restart-broker bare\n",
		"unknown verb":           a + " uid=1 verbs=restart-broker,format-disk\n",
		"repeated verb":          a + " uid=1 verbs=restart-broker,restart-broker\n",
		"empty verbs":            a + " uid=1 verbs=\n",
		"missing uid":            a + " verbs=restart-broker\n",
		"missing verbs":          a + " uid=1\n",
		"duplicate uid":          a + " uid=1 uid=2 verbs=restart-broker\n",
		"uid leading zero":       a + " uid=01 verbs=restart-broker\n",
		"uid plus":               a + " uid=+1 verbs=restart-broker\n",
		"uid negative":           a + " uid=-1 verbs=restart-broker\n",
		"uid overflow":           a + " uid=4294967296 verbs=restart-broker\n",
		"override false":         a + " uid=1 verbs=restart-broker override=false\n",
		"override twice":         a + " uid=1 verbs=restart-broker override=true override=true\n",
		"CR byte":                a + " uid=1 verbs=restart-broker\r\n",
		"NUL byte":               a + " uid=1 verbs=restart-broker\x00\n",
		"indented comment":       " # no\n" + b + " uid=1 verbs=restart-broker\n",
		"oversize":               "#" + strings.Repeat("x", MaxGrantsBytes) + "\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseGrants([]byte(in))
			if !errors.Is(err, ErrPolicyInvalid) {
				t.Fatalf("want ErrPolicyInvalid, got %v", err)
			}
		})
	}
	t.Run("too many entries", func(t *testing.T) {
		var sb strings.Builder
		for i := 0; i <= MaxGrantEntries; i++ {
			// distinct valid fingerprints: vary two leading chars over base64 alphabet
			const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
			fp := "SHA256:" + string(alpha[i%62]) + string(alpha[i/62]) + strings.Repeat("A", 41)
			sb.WriteString(fp + " uid=1 verbs=restart-broker\n")
		}
		if _, err := ParseGrants([]byte(sb.String())); !errors.Is(err, ErrPolicyInvalid) {
			t.Fatalf("want ErrPolicyInvalid, got %v", err)
		}
	})
}

func TestPolicyNarrowingFromAuthorizedKeys(t *testing.T) {
	restricted := newEd25519(t)
	free := newEd25519(t)
	selfEnrolled := newEd25519(t)
	lineFor := func(k testKey, opts string) string {
		return opts + string(ssh.MarshalAuthorizedKey(k.signer.PublicKey()))
	}
	authKeys := "# account file\n" +
		lineFor(restricted, `from="10.0.0.5",command="/bin/true" `) +
		lineFor(free, "") +
		lineFor(selfEnrolled, "") + "\n"
	grants := grantLine(restricted, 501, "restart-broker", false) + grantLine(free, 501, "restart-broker", false)

	p, err := BuildPolicy(1, []byte(grants), []byte(authKeys))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Lookup(restricted.fp); ok {
		t.Fatal("restricted key without override must be absent from the derived mapping")
	}
	if _, ok := p.Lookup(free.fp); !ok {
		t.Fatal("unrestricted granted key must be present")
	}
	if _, ok := p.Lookup(selfEnrolled.fp); ok {
		t.Fatal("authorized_keys alone must never create a grant (self-enrollment)")
	}

	over := grantLine(restricted, 501, "restart-broker", true)
	p2, err := BuildPolicy(1, []byte(over), []byte(authKeys))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := p2.Lookup(restricted.fp); !ok {
		t.Fatal("override=true must keep a restricted key")
	}

	// Removing the restriction restores the pre-existing grant (ADR §1
	// self-restoration), and never more than the grant says.
	p3, err := BuildPolicy(1, []byte(grants), []byte(lineFor(restricted, "")))
	if err != nil {
		t.Fatal(err)
	}
	g, ok := p3.Lookup(restricted.fp)
	if !ok || g.Allows(VerbRemountVolume) {
		t.Fatalf("restored grant wrong: %+v %v", g, ok)
	}

	if _, err := BuildPolicy(1, []byte(grants), []byte("not a key line\n")); !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("unparseable authorized_keys line must fail closed, got %v", err)
	}
	if _, err := BuildPolicy(1, []byte(grants), make([]byte, MaxRestrictionsBytes+1)); !errors.Is(err, ErrPolicyInvalid) {
		t.Fatalf("oversize restrictions must fail closed, got %v", err)
	}
}

func TestPolicyHashIsContentNotMetadata(t *testing.T) {
	a := fakeFP('A')
	p1, _ := BuildPolicy(1, []byte(a+" uid=501 verbs=restart-broker\n"), nil)
	p2, _ := BuildPolicy(1, []byte(a+" uid=502 verbs=restart-broker\n"), nil) // same length
	if p1.Hashes == p2.Hashes {
		t.Fatal("same-length content change must change the hash")
	}
	var nilPolicy *Policy
	if _, ok := nilPolicy.Lookup(a); ok {
		t.Fatal("nil policy must grant nothing")
	}
}
