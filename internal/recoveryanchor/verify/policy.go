package verify

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Bounds on policy inputs.
const (
	MaxGrantsBytes       = 64 << 10
	MaxGrantEntries      = 1024
	MaxRestrictionsBytes = 256 << 10
)

// ErrPolicyInvalid wraps every grant/restriction parse failure. A caller that
// sees it must treat the proposed policy as invalid (ADR-078 §1: refuse to
// publish; dirty if the source changed).
var ErrPolicyInvalid = errors.New("anchor policy invalid")

// Grant is one anchor-grants.allow line: the ADR's
// (ssh_pubkey_fingerprint, local UID, permitted verbs) tuple plus the
// explicit override flag.
//
// Line syntax (this package's concrete spelling of the ADR's tuple; the ADR
// fixes the semantics, not the bytes):
//
//	SHA256:<43 base64 chars> uid=<decimal> verbs=<v1,v2,...> [override=true]
//
// Blank lines and lines whose first character is '#' are ignored. Any other
// field, a duplicate field, a duplicate fingerprint, an unknown verb, or a
// CR/NUL byte rejects the whole source.
type Grant struct {
	Fingerprint string
	UID         uint32
	Verbs       []Verb
	Override    bool
}

// Allows reports whether the grant permits v.
func (g Grant) Allows(v Verb) bool { return slices.Contains(g.Verbs, v) }

func policyErr(line int, format string, a ...any) error {
	return fmt.Errorf("%w: line %d: %s", ErrPolicyInvalid, line, fmt.Sprintf(format, a...))
}

func validFingerprint(fp string) bool {
	rest, ok := strings.CutPrefix(fp, "SHA256:")
	if !ok || len(rest) != 43 {
		return false
	}
	raw, err := base64.RawStdEncoding.Strict().DecodeString(rest)
	return err == nil && len(raw) == sha256.Size
}

// ParseGrants parses grant-source bytes. It never touches the filesystem;
// file mode, ownership and parent-directory checks (ADR-078 §1) are the
// caller's, before it reads the bytes.
func ParseGrants(src []byte) ([]Grant, error) {
	if len(src) > MaxGrantsBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrPolicyInvalid, len(src), MaxGrantsBytes)
	}
	if bytes.ContainsAny(src, "\r\x00") {
		return nil, fmt.Errorf("%w: CR or NUL byte", ErrPolicyInvalid)
	}
	var out []Grant
	seen := map[string]bool{}
	for i, line := range strings.Split(string(src), "\n") {
		n := i + 1
		if line == "" || line[0] == '#' || strings.TrimSpace(line) == "" {
			continue
		}
		if len(out) >= MaxGrantEntries {
			return nil, policyErr(n, "more than %d entries", MaxGrantEntries)
		}
		f := strings.Fields(line)
		g := Grant{Fingerprint: f[0]}
		if !validFingerprint(g.Fingerprint) {
			return nil, policyErr(n, "bad fingerprint")
		}
		if seen[g.Fingerprint] {
			return nil, policyErr(n, "duplicate fingerprint")
		}
		seen[g.Fingerprint] = true
		var haveUID, haveVerbs, haveOverride bool
		for _, tok := range f[1:] {
			k, v, ok := strings.Cut(tok, "=")
			if !ok {
				return nil, policyErr(n, "field %q is not key=value", tok)
			}
			switch k {
			case "uid":
				u, err := strconv.ParseUint(v, 10, 32)
				if haveUID || err != nil || strconv.FormatUint(u, 10) != v {
					return nil, policyErr(n, "bad or duplicate uid")
				}
				g.UID, haveUID = uint32(u), true
			case "verbs":
				if haveVerbs || v == "" {
					return nil, policyErr(n, "bad or duplicate verbs")
				}
				for _, name := range strings.Split(v, ",") {
					vb := Verb(name)
					if !IsVerb(vb) || slices.Contains(g.Verbs, vb) {
						return nil, policyErr(n, "unknown or repeated verb %q", name)
					}
					g.Verbs = append(g.Verbs, vb)
				}
				haveVerbs = true
			case "override":
				if haveOverride || v != "true" {
					return nil, policyErr(n, "override must be exactly override=true, once")
				}
				g.Override, haveOverride = true, true
			default:
				return nil, policyErr(n, "unknown field %q", k)
			}
		}
		if !haveUID || !haveVerbs {
			return nil, policyErr(n, "uid and verbs are both required")
		}
		out = append(out, g)
	}
	return out, nil
}

// ParseRestrictions reads authorized_keys bytes (concatenated by the caller
// if several accounts matter) and returns the fingerprints whose entry
// carries any option (from=, command=, ...). authorized_keys is
// account-writable: it can only NARROW (ADR-078 §1). A key that appears
// twice counts as restricted if either entry has options. An unparseable
// line rejects the whole source (fail closed).
func ParseRestrictions(src []byte) (map[string]bool, error) {
	if len(src) > MaxRestrictionsBytes {
		return nil, fmt.Errorf("%w: restrictions %d bytes exceeds %d", ErrPolicyInvalid, len(src), MaxRestrictionsBytes)
	}
	restricted := map[string]bool{}
	for i, line := range bytes.Split(src, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		key, _, opts, _, err := ssh.ParseAuthorizedKey(line)
		if err != nil {
			return nil, policyErr(i+1, "unparseable authorized_keys line")
		}
		if len(opts) > 0 {
			restricted[ssh.FingerprintSHA256(key)] = true
		}
	}
	return restricted, nil
}

// HashPair identifies the exact policy bytes a generation was derived from
// (ADR-078 §1a: content hash, not mtime+size).
type HashPair struct {
	Grants       [32]byte
	Restrictions [32]byte
}

// Policy is the derived mapping fingerprint -> grant, after authorized_keys
// narrowing, bound to the generation it was published under.
type Policy struct {
	Generation uint64
	Hashes     HashPair
	byFP       map[string]Grant
}

// BuildPolicy is the single validate function: the async sync step and the
// synchronous pre-dispatch check must both call it (ADR-078 §1a). A key in
// grantsSrc whose authorized_keys entry carries options is excluded unless
// its grant line says override=true.
func BuildPolicy(generation uint64, grantsSrc, restrictionsSrc []byte) (*Policy, error) {
	grants, err := ParseGrants(grantsSrc)
	if err != nil {
		return nil, err
	}
	restricted, err := ParseRestrictions(restrictionsSrc)
	if err != nil {
		return nil, err
	}
	p := &Policy{
		Generation: generation,
		Hashes:     HashPair{Grants: sha256.Sum256(grantsSrc), Restrictions: sha256.Sum256(restrictionsSrc)},
		byFP:       make(map[string]Grant, len(grants)),
	}
	for _, g := range grants {
		if restricted[g.Fingerprint] && !g.Override {
			continue
		}
		p.byFP[g.Fingerprint] = g
	}
	return p, nil
}

// Lookup returns the grant for a signing fingerprint, if any. Safe on nil.
func (p *Policy) Lookup(fingerprint string) (Grant, bool) {
	if p == nil {
		return Grant{}, false
	}
	g, ok := p.byFP[fingerprint]
	return g, ok
}
