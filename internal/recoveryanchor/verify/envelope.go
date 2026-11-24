package verify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
)

// Verb is one of the ADR-078 §3 fixed recovery verbs. The set is closed.
type Verb string

const (
	VerbRestartBroker        Verb = "restart-broker"
	VerbRemountVolume        Verb = "remount-volume"
	VerbReArmSupervisor      Verb = "re-arm-supervisor"
	VerbFetchKnownGoodConfig Verb = "fetch-known-good-config"
)

// AllVerbs is the complete, fixed verb set.
func AllVerbs() []Verb {
	return []Verb{VerbRestartBroker, VerbRemountVolume, VerbReArmSupervisor, VerbFetchKnownGoodConfig}
}

// IsVerb reports whether v is one of the fixed four.
func IsVerb(v Verb) bool {
	switch v {
	case VerbRestartBroker, VerbRemountVolume, VerbReArmSupervisor, VerbFetchKnownGoodConfig:
		return true
	}
	return false
}

// MaxEnvelopeBytes bounds the envelope input. A maximal canonical envelope is
// under 600 bytes.
const MaxEnvelopeBytes = 1024

// Envelope is ADR-078 §1's eight-field signed request. Field types are
// pinned: strings, and integers in Unix seconds UTC.
type Envelope struct {
	NodeID       string
	Verb         Verb
	Target       string
	ConfigDigest string // lowercase hex sha256, 64 chars
	RequestID    string // UUIDv4, canonical lowercase
	IssuedAt     int64
	ExpiresAt    int64
	Epoch        uint64
}

// Charsets are restricted so that canonical JSON string encoding never needs
// an escape: that is what makes the byte-level canonical form unambiguous
// across encoders (ADR-078 §1, "key-sorting and a trailing newline alone do
// not pin every JSON encoder's whitespace and escaping choices").
var (
	reNodeID  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
	reTarget  = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	reVerb    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	reDigest  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reUUIDv4  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	envFields = []string{"config_digest", "epoch", "expires_at", "issued_at", "node_id", "request_id", "target", "verb"}
)

// Canonical returns the exact bytes a signer signs: key-sorted, UTF-8,
// no insignificant whitespace, newline-terminated.
func (e Envelope) Canonical() []byte {
	return fmt.Appendf(nil,
		`{"config_digest":%q,"epoch":%d,"expires_at":%d,"issued_at":%d,"node_id":%q,"request_id":%q,"target":%q,"verb":%q}`+"\n",
		e.ConfigDigest, e.Epoch, e.ExpiresAt, e.IssuedAt, e.NodeID, e.RequestID, e.Target, string(e.Verb))
}

// ParseEnvelope parses b as exactly one canonical envelope. It rejects
// oversize input, duplicate keys, unknown keys, missing keys, wrong types,
// out-of-charset values, trailing data, and any byte sequence that is not
// identical to the canonical re-encoding. Errors are *DenyError.
//
// Membership of Verb in the fixed four is NOT checked here (the parser's job
// is well-formedness); Verifier.Admit denies unknown verbs.
func ParseEnvelope(b []byte) (Envelope, error) {
	var zero Envelope
	if len(b) > MaxEnvelopeBytes {
		return zero, deny(ReasonEnvelopeOversize, "%d bytes exceeds %d", len(b), MaxEnvelopeBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return zero, deny(ReasonEnvelopeMalformed, "not a JSON object")
	}
	allowed := make(map[string]bool, len(envFields))
	for _, f := range envFields {
		allowed[f] = true
	}
	vals := make(map[string]json.Token, len(envFields))
	for dec.More() {
		kt, err := dec.Token()
		key, ok := kt.(string)
		if err != nil || !ok {
			return zero, deny(ReasonEnvelopeMalformed, "bad object key")
		}
		if !allowed[key] {
			return zero, deny(ReasonEnvelopeUnknownField, "unrecognized field %q", key)
		}
		if _, dup := vals[key]; dup {
			return zero, deny(ReasonEnvelopeDuplicateField, "duplicate field %q", key)
		}
		vt, err := dec.Token()
		if err != nil {
			return zero, deny(ReasonEnvelopeMalformed, "bad value for %q", key)
		}
		switch vt.(type) {
		case string, json.Number:
		default:
			return zero, deny(ReasonEnvelopeMalformed, "field %q must be a string or integer", key)
		}
		vals[key] = vt
	}
	if t, err := dec.Token(); err != nil || t != json.Delim('}') {
		return zero, deny(ReasonEnvelopeMalformed, "unterminated object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return zero, deny(ReasonEnvelopeMalformed, "trailing data after object")
	}
	for _, f := range envFields {
		if _, ok := vals[f]; !ok {
			return zero, deny(ReasonEnvelopeMissingField, "missing field %q", f)
		}
	}

	var e Envelope
	str := func(k string, re *regexp.Regexp) (string, error) {
		s, ok := vals[k].(string)
		if !ok || !re.MatchString(s) {
			return "", deny(ReasonEnvelopeMalformed, "field %q has wrong type or form", k)
		}
		return s, nil
	}
	num := func(k string) (int64, error) {
		n, ok := vals[k].(json.Number)
		if !ok {
			return 0, deny(ReasonEnvelopeMalformed, "field %q must be an integer", k)
		}
		v, err := strconv.ParseInt(n.String(), 10, 64)
		if err != nil || v <= 0 {
			return 0, deny(ReasonEnvelopeMalformed, "field %q must be a positive integer", k)
		}
		return v, nil
	}
	var err error
	if e.NodeID, err = str("node_id", reNodeID); err != nil {
		return zero, err
	}
	var verb string
	if verb, err = str("verb", reVerb); err != nil {
		return zero, err
	}
	e.Verb = Verb(verb)
	if e.Target, err = str("target", reTarget); err != nil {
		return zero, err
	}
	if e.ConfigDigest, err = str("config_digest", reDigest); err != nil {
		return zero, err
	}
	if e.RequestID, err = str("request_id", reUUIDv4); err != nil {
		return zero, err
	}
	if e.IssuedAt, err = num("issued_at"); err != nil {
		return zero, err
	}
	if e.ExpiresAt, err = num("expires_at"); err != nil {
		return zero, err
	}
	ep, err := num("epoch")
	if err != nil {
		return zero, err
	}
	e.Epoch = uint64(ep)

	if !bytes.Equal(e.Canonical(), b) {
		return zero, deny(ReasonEnvelopeNonCanonical, "bytes differ from canonical encoding")
	}
	return e, nil
}
