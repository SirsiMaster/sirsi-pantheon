package rolereceipt

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type testAuthenticator struct {
	wantRaw []byte
	err     error
}

func (a testAuthenticator) VerifyRoleReceipt(raw []byte, _ Receipt) error {
	if string(raw) != string(a.wantRaw) {
		return errors.New("raw receipt bytes changed")
	}
	return a.err
}

func TestAuthenticateAndValidateRequiresExternalTrustRoot(t *testing.T) {
	receipt := validReceipt(t)
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	constraints := Constraints{Now: receipt.IssuedAt.Add(time.Minute), Role: ConstrainedClient, HostID: receipt.HostProfile.ID, RequiredScope: []string{"inspect"}}
	if _, err := AuthenticateAndValidate(raw, constraints, nil); err == nil {
		t.Fatal("structural receipt became authority without a trust root")
	}
	verified, err := AuthenticateAndValidate(raw, constraints, testAuthenticator{wantRaw: raw})
	if err != nil {
		t.Fatalf("AuthenticateAndValidate: %v", err)
	}
	if verified.RawSHA256() == "" || verified.Receipt().ReceiptID != receipt.ReceiptID {
		t.Fatalf("authenticated receipt lost identity: %#v", verified)
	}
}

func TestAuthenticateAndValidateRejectsTrustRootFailure(t *testing.T) {
	receipt := validReceipt(t)
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	constraints := Constraints{Now: receipt.IssuedAt.Add(time.Minute), Role: ConstrainedClient, HostID: receipt.HostProfile.ID, RequiredScope: []string{"inspect"}}
	if _, err := AuthenticateAndValidate(raw, constraints, testAuthenticator{wantRaw: raw, err: errors.New("revoked")}); err == nil {
		t.Fatal("revoked receipt accepted")
	}
}

func TestAuthenticateAndValidateBindsExactRawBytes(t *testing.T) {
	receipt := validReceipt(t)
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	constraints := Constraints{Now: receipt.IssuedAt.Add(time.Minute), Role: ConstrainedClient, HostID: receipt.HostProfile.ID, RequiredScope: []string{"inspect"}}
	auth := testAuthenticator{wantRaw: append([]byte(nil), raw...)}
	raw = append(raw, '\n')
	if _, err := AuthenticateAndValidate(raw, constraints, auth); err == nil {
		t.Fatal("trust root accepted bytes different from the signed input")
	}
}
