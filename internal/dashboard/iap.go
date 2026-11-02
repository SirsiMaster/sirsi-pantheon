package dashboard

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// IAP identity for the hosted board's ledger edits. Cloud Run IAP signs every request it lets through with
// an ES256 JWT in x-goog-iap-jwt-assertion; the board trusts only that signature (never a plain email
// header, which anything inside the project could forge) and only for the configured audience.
const (
	iapIssuer  = "https://cloud.google.com/iap"
	iapKeysURL = "https://www.gstatic.com/iap/verify/public_key-jwk"
	iapHeader  = "x-goog-iap-jwt-assertion"
	iapKeysTTL = time.Hour
	iapSkew    = 30 * time.Second
)

var errIAP = errors.New("board: IAP identity refused")

type iapVerifier struct {
	audience, domain string
	now              func() time.Time
	fetch            func() (map[string]*ecdsa.PublicKey, error)

	mu      sync.Mutex
	keys    map[string]*ecdsa.PublicKey
	fetched time.Time
}

func newIAPVerifier(audience, domain string) *iapVerifier {
	return &iapVerifier{audience: audience, domain: domain, now: time.Now, fetch: fetchIAPKeys}
}

// verify returns the signed-in email, or errIAP. Every failure is the same refusal: an unset audience, a
// missing or malformed header, an unknown key, a bad signature, a wrong issuer or audience, an expired or
// future token, or an account outside the domain.
func (v *iapVerifier) verify(r *http.Request) (string, error) {
	if v == nil || v.audience == "" || v.domain == "" {
		return "", fmt.Errorf("%w: audience not configured", errIAP)
	}
	parts := strings.Split(r.Header.Get(iapHeader), ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: no signed assertion", errIAP)
	}
	var head struct{ Alg, Kid string }
	if err := decodeJWTPart(parts[0], &head); err != nil || head.Alg != "ES256" || head.Kid == "" {
		return "", fmt.Errorf("%w: header", errIAP)
	}
	key, err := v.key(head.Kid)
	if err != nil {
		return "", err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return "", fmt.Errorf("%w: signature encoding", errIAP)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return "", fmt.Errorf("%w: signature", errIAP)
	}
	var c struct {
		Iss, Aud, Email, Hd string
		Exp, Iat            int64
	}
	if err := decodeJWTPart(parts[1], &c); err != nil {
		return "", fmt.Errorf("%w: claims", errIAP)
	}
	now := v.now()
	switch {
	case c.Iss != iapIssuer, c.Aud != v.audience:
		return "", fmt.Errorf("%w: issuer or audience", errIAP)
	case now.After(time.Unix(c.Exp, 0).Add(iapSkew)), time.Unix(c.Iat, 0).After(now.Add(iapSkew)):
		return "", fmt.Errorf("%w: expired or not yet valid", errIAP)
	}
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if at := strings.LastIndex(email, "@"); at <= 0 || email[at+1:] != v.domain || (c.Hd != "" && c.Hd != v.domain) {
		return "", fmt.Errorf("%w: account outside %s", errIAP, v.domain)
	}
	return email, nil
}

func (v *iapVerifier) key(kid string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k := v.keys[kid]; k != nil && v.now().Sub(v.fetched) < iapKeysTTL {
		return k, nil
	}
	keys, err := v.fetch()
	if err != nil {
		return nil, fmt.Errorf("%w: key set unavailable", errIAP)
	}
	v.keys, v.fetched = keys, v.now()
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, fmt.Errorf("%w: unknown key", errIAP)
}

func decodeJWTPart(s string, into any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

func fetchIAPKeys() (map[string]*ecdsa.PublicKey, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(iapKeysURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iap keys: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseIAPKeys(b)
}

func parseIAPKeys(b []byte) (map[string]*ecdsa.PublicKey, error) {
	var set struct {
		Keys []struct{ Kid, Kty, Crv, X, Y string } `json:"keys"`
	}
	if err := json.Unmarshal(b, &set); err != nil {
		return nil, err
	}
	out := map[string]*ecdsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "EC" || k.Crv != "P-256" {
			continue
		}
		x, xerr := base64.RawURLEncoding.DecodeString(k.X)
		y, yerr := base64.RawURLEncoding.DecodeString(k.Y)
		if xerr != nil || yerr != nil || len(x) != 32 || len(y) != 32 {
			continue
		}
		// Parsing validates the point is on the curve.
		pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append(append([]byte{4}, x...), y...))
		if err != nil {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("iap keys: no usable P-256 keys")
	}
	return out, nil
}
