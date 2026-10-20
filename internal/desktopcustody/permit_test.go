package desktopcustody

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// Test-only issuer. Production has no issuer minting or automatic enrollment.
type fixtureIssuer struct {
	public  ed25519.PublicKey
	revoked bool
}

func (i *fixtureIssuer) SleepIssuer(_ context.Context, id string) (ed25519.PublicKey, error) {
	if i.revoked || id != "sleep-authority" {
		return nil, errors.New("not enrolled")
	}
	return i.public, nil
}

func signedFixture(t *testing.T) (*fixture, SignedSleepVerifier, SleepPermitClaims, ed25519.PrivateKey) {
	t.Helper()
	f, _ := newFixture(t)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f.a.Issuer = "sleep-authority"
	review := []byte("authenticated hardware review, test only")
	f.a.ReviewDigest = digestBytes(review)
	claims := SleepPermitClaims{SchemaVersion: 1, Purpose: sleepPermitPurpose, Request: f.req, Admission: f.a, ExecutableSHA256: strings.Repeat("a", 64), IssuedAt: f.now, Review: review}
	v := SignedSleepVerifier{Authority: &fixtureIssuer{public: public}, Identity: func(context.Context) (HostIdentity, error) {
		return HostIdentity{Host: f.a.Host, PlatformUUID: f.a.PlatformUUID, BootID: f.a.BootID, ExecutableSHA256: claims.ExecutableSHA256}, nil
	}, Now: func() time.Time { return f.now }, MaxReviewAge: time.Minute, MaxClockSkew: time.Second}
	v.Permit = signFixturePermit(t, claims, private)
	return f, v, claims, private
}

func signFixturePermit(t *testing.T, claims SleepPermitClaims, private ed25519.PrivateKey) []byte {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(private, append([]byte(sleepPermitPurpose+"\x00"), payload...))
	data, err := json.Marshal(SignedSleepPermit{KeyID: "sleep-authority", Payload: payload, Signature: base64.RawURLEncoding.EncodeToString(signature)})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSignedPermitBindsActualHostBootSourceRequestAndReview(t *testing.T) {
	f, v, _, _ := signedFixture(t)
	a, err := v.VerifySleep(context.Background(), f.req)
	if err != nil || a != f.a {
		t.Fatalf("admission=%v err=%v", a, err)
	}
}

func TestSignedPermitRejectsValidlySignedWrongBindings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SleepPermitClaims)
	}{
		{"recovery-purpose", func(c *SleepPermitClaims) { c.Purpose = "pantheon.desktop-recovery" }},
		{"foreign-host", func(c *SleepPermitClaims) { c.Admission.Host = "studio" }},
		{"foreign-boot", func(c *SleepPermitClaims) { c.Admission.BootID = "another-boot" }},
		{"different-platform", func(c *SleepPermitClaims) { c.Admission.PlatformUUID = "another-platform" }},
		{"different-executable", func(c *SleepPermitClaims) { c.ExecutableSHA256 = strings.Repeat("b", 64) }},
		{"changed-wake", func(c *SleepPermitClaims) { c.Request.WakeAt = c.Request.WakeAt.Add(time.Second) }},
		{"changed-review", func(c *SleepPermitClaims) { c.Review = []byte("substituted") }},
		{"stale-review", func(c *SleepPermitClaims) { c.IssuedAt = c.IssuedAt.Add(-2 * time.Minute) }},
		{"future-review", func(c *SleepPermitClaims) { c.IssuedAt = c.IssuedAt.Add(2 * time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, v, c, key := signedFixture(t)
			tc.change(&c)
			v.Permit = signFixturePermit(t, c, key)
			if _, err := v.VerifySleep(context.Background(), f.req); err == nil {
				t.Fatal("wrong binding accepted")
			}
		})
	}
}

func TestSignedPermitDeniesMissingEnrollmentRevocationAndPolicy(t *testing.T) {
	for _, change := range []func(*SignedSleepVerifier){
		func(v *SignedSleepVerifier) { v.Authority = nil },
		func(v *SignedSleepVerifier) { v.Authority.(*fixtureIssuer).revoked = true },
		func(v *SignedSleepVerifier) { v.Identity = nil },
		func(v *SignedSleepVerifier) { v.MaxReviewAge = 0 },
		func(v *SignedSleepVerifier) { v.MaxClockSkew = 0 },
		func(v *SignedSleepVerifier) { v.Permit = []byte(`{}`) },
	} {
		f, v, _, _ := signedFixture(t)
		change(&v)
		if _, err := v.VerifySleep(context.Background(), f.req); err == nil {
			t.Fatal("missing authority/policy accepted")
		}
	}
}

func TestSignedPermitRejectsUnsignedPayloadChange(t *testing.T) {
	f, v, _, _ := signedFixture(t)
	var envelope SignedSleepPermit
	if err := json.Unmarshal(v.Permit, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Payload = []byte(strings.Replace(string(envelope.Payload), "cylton", "intruder", 1))
	v.Permit, _ = json.Marshal(envelope)
	if _, err := v.VerifySleep(context.Background(), f.req); err == nil {
		t.Fatal("modified payload accepted without a matching signature")
	}
}
