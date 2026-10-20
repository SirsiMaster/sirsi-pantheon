package desktopcustody

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const sleepPermitPurpose = "pantheon.sleep-custody.v1"

// SleepIssuerLookup must be backed by the already enrolled authority's live
// sleep-purpose policy, including revocation. Recovery/SSH keys are not sleep
// issuers. There is deliberately no issuer generator or file-minting fallback.
type SleepIssuerLookup interface {
	SleepIssuer(context.Context, string) (ed25519.PublicKey, error)
}

type HostIdentity struct {
	Host, PlatformUUID, BootID, ExecutableSHA256 string
}

type SignedSleepPermit struct {
	KeyID     string          `json:"key_id"`
	Payload   json.RawMessage `json:"payload"`
	Signature string          `json:"signature"`
}

// SleepPermitClaims is authenticated intent, not proof of current quietness.
// CurrentQuiet/AuthorizeSleep must independently revalidate real workload and
// observer custody immediately before invocation.
type SleepPermitClaims struct {
	SchemaVersion    int            `json:"schema_version"`
	Purpose          string         `json:"purpose"`
	Request          SleepRequest   `json:"request"`
	Admission        SleepAdmission `json:"admission"`
	ExecutableSHA256 string         `json:"executable_sha256"`
	IssuedAt         time.Time      `json:"issued_at"`
	Review           []byte         `json:"review"`
}

type SignedSleepVerifier struct {
	Authority    SleepIssuerLookup
	Permit       []byte
	Identity     func(context.Context) (HostIdentity, error)
	Now          func() time.Time
	MaxReviewAge time.Duration
	MaxClockSkew time.Duration
}

func (v SignedSleepVerifier) VerifySleep(ctx context.Context, req SleepRequest) (SleepAdmission, error) {
	if v.Authority == nil || v.Identity == nil || v.Now == nil || v.MaxReviewAge <= 0 || v.MaxReviewAge > maxSleepWindow || v.MaxClockSkew <= 0 || v.MaxClockSkew > v.MaxReviewAge {
		return SleepAdmission{}, errors.New("sleep custody: enrolled authority, native identity and reviewed freshness policy are required")
	}
	if len(v.Permit) == 0 || len(v.Permit) > 32<<10 {
		return SleepAdmission{}, errors.New("sleep custody: signed permit is absent or oversized")
	}
	var envelope SignedSleepPermit
	if err := decodeSleepExact(v.Permit, &envelope); err != nil || !identifierRE.MatchString(envelope.KeyID) {
		return SleepAdmission{}, errors.New("sleep custody: signed permit envelope is malformed")
	}
	key, err := v.Authority.SleepIssuer(ctx, envelope.KeyID)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return SleepAdmission{}, errors.New("sleep custody: issuer is unknown, revoked or not enrolled for sleep")
	}
	sig, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize || base64.RawURLEncoding.EncodeToString(sig) != envelope.Signature {
		return SleepAdmission{}, errors.New("sleep custody: permit signature encoding is malformed")
	}
	message := append([]byte(sleepPermitPurpose+"\x00"), envelope.Payload...)
	if !ed25519.Verify(key, message, sig) {
		return SleepAdmission{}, errors.New("sleep custody: permit signature is not accepted")
	}
	var claims SleepPermitClaims
	if err := decodeSleepExact(envelope.Payload, &claims); err != nil {
		return SleepAdmission{}, fmt.Errorf("sleep custody: malformed signed claims: %w", err)
	}
	// One exact encoding rejects duplicate keys and parser-dependent signed
	// representations. Issuers sign json.Marshal(SleepPermitClaims) bytes.
	canonical, err := json.Marshal(claims)
	if err != nil || !bytes.Equal(canonical, envelope.Payload) {
		return SleepAdmission{}, errors.New("sleep custody: signed claims are not canonical")
	}
	identity, err := v.Identity(ctx)
	if err != nil || identity.Host == "" || identity.PlatformUUID == "" || identity.BootID == "" || len(identity.ExecutableSHA256) != 64 {
		return SleepAdmission{}, errors.New("sleep custody: actual native host/boot/executable identity unavailable")
	}
	a, now := claims.Admission, v.Now().UTC()
	if claims.SchemaVersion != 1 || claims.Purpose != sleepPermitPurpose || claims.Request.ID != req.ID || claims.Request.Host != req.Host || !claims.Request.WakeAt.Equal(req.WakeAt) || a.Issuer != envelope.KeyID || a.Host != identity.Host || a.PlatformUUID != identity.PlatformUUID || a.BootID != identity.BootID || claims.ExecutableSHA256 != identity.ExecutableSHA256 || a.ReviewDigest != digestBytes(claims.Review) || len(claims.Review) == 0 {
		return SleepAdmission{}, errors.New("sleep custody: signed purpose, request, review or native identity does not match")
	}
	if claims.IssuedAt.IsZero() || claims.IssuedAt.After(now.Add(v.MaxClockSkew)) || now.Sub(claims.IssuedAt) > v.MaxReviewAge || claims.IssuedAt.Before(a.NotBefore.Add(-v.MaxClockSkew)) {
		return SleepAdmission{}, errors.New("sleep custody: signed review is stale or outside the reviewed clock bound")
	}
	if err := (SleepService{Now: v.Now}).validateAdmission(req, a); err != nil {
		return SleepAdmission{}, err
	}
	return a, nil
}
