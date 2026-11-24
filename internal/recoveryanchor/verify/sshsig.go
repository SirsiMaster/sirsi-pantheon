package verify

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Namespace is the dedicated SSHSIG namespace (ADR-078 §1). A signature made
// under any other namespace is rejected before any further check.
const Namespace = "sirsi-recovery-anchor"

// Bounds on the armored signature and its decoded blob.
const (
	MaxSignatureArmorBytes = 4096
	maxSignatureBlobBytes  = 3072
)

const (
	sshsigMagic   = "SSHSIG"
	sshsigVersion = 1
	armorHeader   = "-----BEGIN SSH SIGNATURE-----\n"
	armorFooter   = "-----END SSH SIGNATURE-----"
)

// Allowed key types and digests. ADR-078 §1 names the key types
// ("ed25519 and ecdsa-sha2-nistp256 at launch; nothing else"). The ADR is
// silent on SSHSIG digests; sha256 and sha512 are the two that
// `ssh-keygen -Y sign` can emit, and nothing weaker is accepted.
var (
	allowedKeyTypes = map[string]bool{
		ssh.KeyAlgoED25519:  true,
		ssh.KeyAlgoECDSA256: true,
	}
	allowedHashes = map[string]bool{"sha256": true, "sha512": true}
)

// Signature is a parsed, namespace- and key-type-checked SSHSIG. Parsing does
// not verify the signature; call VerifyMessage.
type Signature struct {
	Key         ssh.PublicKey
	Fingerprint string // "SHA256:<base64>" of Key
	HashAlg     string

	namespace string
	inner     ssh.Signature
}

type wireReader struct {
	b   []byte
	bad bool
}

func (r *wireReader) u32() uint32 {
	if r.bad || len(r.b) < 4 {
		r.bad = true
		return 0
	}
	v := binary.BigEndian.Uint32(r.b)
	r.b = r.b[4:]
	return v
}

func (r *wireReader) fixed(n int) []byte {
	if r.bad || len(r.b) < n {
		r.bad = true
		return nil
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}

func (r *wireReader) str() []byte {
	n := r.u32()
	if r.bad || uint64(n) > uint64(len(r.b)) {
		r.bad = true
		return nil
	}
	return r.fixed(int(n))
}

func (r *wireReader) done() bool { return !r.bad && len(r.b) == 0 }

func appendString(b, s []byte) []byte {
	b = binary.BigEndian.AppendUint32(b, uint32(len(s)))
	return append(b, s...)
}

// ParseSignature parses an armored SSHSIG (the output of `ssh-keygen -Y
// sign`). It rejects wrong namespace, disallowed key type or digest, a
// non-empty reserved field, malformed or trailing data. Errors are *DenyError.
func ParseSignature(armored []byte) (*Signature, error) {
	if len(armored) > MaxSignatureArmorBytes {
		return nil, deny(ReasonSignatureOversize, "%d bytes exceeds %d", len(armored), MaxSignatureArmorBytes)
	}
	s := string(armored)
	if !strings.HasPrefix(s, armorHeader) {
		return nil, deny(ReasonSignatureMalformed, "missing armor header")
	}
	s = strings.TrimPrefix(s, armorHeader)
	s = strings.TrimSuffix(s, "\n")
	if !strings.HasSuffix(s, armorFooter) {
		return nil, deny(ReasonSignatureMalformed, "missing armor footer")
	}
	body := strings.ReplaceAll(strings.TrimSuffix(s, armorFooter), "\n", "")
	if body == "" || strings.ContainsAny(body, " \t\r\x00") {
		return nil, deny(ReasonSignatureMalformed, "bad armor body")
	}
	blob, err := base64.StdEncoding.Strict().DecodeString(body)
	if err != nil || len(blob) > maxSignatureBlobBytes {
		return nil, deny(ReasonSignatureMalformed, "bad base64 body")
	}

	r := &wireReader{b: blob}
	if !bytes.Equal(r.fixed(len(sshsigMagic)), []byte(sshsigMagic)) || r.u32() != sshsigVersion {
		return nil, deny(ReasonSignatureMalformed, "bad SSHSIG magic or version")
	}
	pubBlob := r.str()
	ns := r.str()
	reserved := r.str()
	hashAlg := r.str()
	sigBlob := r.str()
	if !r.done() {
		return nil, deny(ReasonSignatureMalformed, "truncated or trailing SSHSIG data")
	}

	// Order matters (ADR-078 §1): namespace first, then key format, before
	// anything reaches the allowlist or signature check.
	if string(ns) != Namespace {
		return nil, deny(ReasonWrongNamespace, "signature namespace is not %q", Namespace)
	}
	if len(reserved) != 0 {
		return nil, deny(ReasonSignatureMalformed, "reserved field not empty")
	}
	kr := &wireReader{b: pubBlob}
	keyType := string(kr.str())
	if kr.bad || !allowedKeyTypes[keyType] {
		return nil, deny(ReasonKeyTypeNotAllowed, "key type not allowed")
	}
	key, err := ssh.ParsePublicKey(pubBlob)
	if err != nil || key.Type() != keyType || !bytes.Equal(key.Marshal(), pubBlob) {
		return nil, deny(ReasonSignatureMalformed, "bad public key blob")
	}
	if !allowedHashes[string(hashAlg)] {
		return nil, deny(ReasonHashNotAllowed, "hash algorithm not allowed")
	}
	sr := &wireReader{b: sigBlob}
	format := string(sr.str())
	raw := sr.str()
	if !sr.done() || format != keyType {
		return nil, deny(ReasonSignatureMalformed, "bad inner signature")
	}
	return &Signature{
		Key:         key,
		Fingerprint: ssh.FingerprintSHA256(key),
		HashAlg:     string(hashAlg),
		namespace:   string(ns),
		inner:       ssh.Signature{Format: format, Blob: append([]byte(nil), raw...)},
	}, nil
}

// VerifyMessage checks the signature over msg per PROTOCOL.sshsig: the
// signer signs magic || namespace || reserved || hash_algorithm || H(msg).
func (s *Signature) VerifyMessage(msg []byte) error {
	var h []byte
	switch s.HashAlg {
	case "sha256":
		d := sha256.Sum256(msg)
		h = d[:]
	case "sha512":
		d := sha512.Sum512(msg)
		h = d[:]
	default:
		return deny(ReasonHashNotAllowed, "hash algorithm not allowed")
	}
	data := []byte(sshsigMagic)
	data = appendString(data, []byte(s.namespace))
	data = appendString(data, nil)
	data = appendString(data, []byte(s.HashAlg))
	data = appendString(data, h)
	sig := s.inner
	if err := s.Key.Verify(data, &sig); err != nil {
		return deny(ReasonSignatureInvalid, "signature does not verify over these bytes")
	}
	return nil
}
