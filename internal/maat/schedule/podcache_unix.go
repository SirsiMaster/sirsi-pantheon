//go:build darwin || linux

package schedule

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type signedPodCache struct {
	Grant     string `json:"grant"`
	Signer    string `json:"signer"`
	Signature []byte `json:"signature"`
}

func cacheCertificate(identity tls.Certificate) (*x509.Certificate, error) {
	if len(identity.Certificate) == 0 {
		return nil, fmt.Errorf("maat: existing native identity required")
	}
	return x509.ParseCertificate(identity.Certificate[0])
}
func cacheSignatureInput(cert *x509.Certificate, body []byte) (x509.SignatureAlgorithm, []byte, crypto.SignerOpts, error) {
	h := sha256.Sum256(body)
	switch cert.PublicKey.(type) {
	case ed25519.PublicKey:
		return x509.PureEd25519, body, crypto.Hash(0), nil
	case *rsa.PublicKey:
		return x509.SHA256WithRSA, h[:], crypto.SHA256, nil
	case *ecdsa.PublicKey:
		return x509.ECDSAWithSHA256, h[:], crypto.SHA256, nil
	}
	return 0, nil, nil, fmt.Errorf("maat: unsupported approved native signing identity")
}

// openPodCache resolves every ancestor relative to retained no-follow handles.
// The cache contains only signed delegation metadata, never a private key/token.
func openPodCache(path string, flags int) (*os.File, int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, -1, fmt.Errorf("maat: canonical cache path required")
	}
	parent, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, -1, e
	}
	for _, part := range strings.Split(strings.TrimPrefix(filepath.Dir(path), "/"), "/") {
		if part == "" {
			continue
		}
		next, err := unix.Openat(parent, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(parent)
		if err != nil {
			return nil, -1, err
		}
		parent = next
	}
	fd, e := unix.Openat(parent, filepath.Base(path), flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		unix.Close(parent)
		return nil, -1, e
	}
	var st unix.Stat_t
	e = unix.Fstat(fd, &st)
	if e != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Size > 1<<20 {
		unix.Close(fd)
		unix.Close(parent)
		return nil, -1, fmt.Errorf("maat: bounded private native cache required")
	}
	return os.NewFile(uintptr(fd), path), parent, nil
}

// PersistFiniteGrantCache signs an authenticated snapshot using the ALREADY
// approved native client identity. Exclusive creation never replaces history or
// locally extends expiry. A new epoch uses a new explicitly selected cache file.
func PersistFiniteGrantCache(path string, cache *FiniteGrantCache, identity tls.Certificate) error {
	if _, e := cache.Snapshot(); e != nil {
		return e
	}
	cert, e := cacheCertificate(identity)
	if e != nil {
		return e
	}
	signer, ok := identity.PrivateKey.(crypto.Signer)
	if !ok {
		return fmt.Errorf("maat: approved identity cannot sign cache")
	}
	_, input, opts, e := cacheSignatureInput(cert, []byte(cache.encoded))
	if e != nil {
		return e
	}
	sig, e := signer.Sign(rand.Reader, input, opts)
	if e != nil {
		return fmt.Errorf("maat: native cache signing failed")
	}
	hash := sha256.Sum256(cert.Raw)
	doc := signedPodCache{Grant: cache.encoded, Signer: hex.EncodeToString(hash[:]), Signature: sig}
	b, e := json.Marshal(doc)
	if e != nil {
		return e
	}
	f, parent, e := openPodCache(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL)
	if os.IsExist(e) {
		old, err := LoadFiniteGrantCache(path, identity)
		if err == nil && old.encoded == cache.encoded {
			return nil
		}
		return fmt.Errorf("maat: immutable cache exists with different/invalid grant")
	}
	if e != nil {
		return e
	}
	defer f.Close()
	defer unix.Close(parent)
	if _, e = f.Write(b); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	return unix.Fsync(parent)
}

// LoadFiniteGrantCache permits offline restart only for the exact same existing
// native identity and unexpired signed grant. Local bytes cannot extend it.
func LoadFiniteGrantCache(path string, identity tls.Certificate) (*FiniteGrantCache, error) {
	f, parent, e := openPodCache(path, unix.O_RDONLY)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	defer unix.Close(parent)
	b, e := io.ReadAll(io.LimitReader(f, 1<<20))
	if e != nil {
		return nil, e
	}
	var doc signedPodCache
	if e = json.Unmarshal(b, &doc); e != nil {
		return nil, e
	}
	cert, e := cacheCertificate(identity)
	if e != nil {
		return nil, e
	}
	hash := sha256.Sum256(cert.Raw)
	if doc.Signer != hex.EncodeToString(hash[:]) {
		return nil, fmt.Errorf("maat: cached native identity changed")
	}
	algorithm, _, _, e := cacheSignatureInput(cert, []byte(doc.Grant))
	if e != nil {
		return nil, e
	}
	if e = cert.CheckSignature(algorithm, []byte(doc.Grant), doc.Signature); e != nil {
		return nil, fmt.Errorf("maat: cache signature invalid")
	}
	cache := &FiniteGrantCache{encoded: doc.Grant}
	d, e := cache.Snapshot()
	if e != nil {
		return nil, e
	}
	if e = validateDelegation(*d); e != nil {
		return nil, e
	}
	if d.Epoch < 1 {
		return nil, fmt.Errorf("maat: cached grant has no epoch")
	}
	return cache, nil
}
