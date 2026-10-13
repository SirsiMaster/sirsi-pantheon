package maat

import (
	"errors"
	"testing"
)

func TestCloudResourceStringAndParseRoundTrip(t *testing.T) {
	r := CloudResource{Provider: "gcp", Account: "01ABF6-FC7DBF-2BB83A"}
	s := r.String()
	if s != "cloud:gcp:01ABF6-FC7DBF-2BB83A" {
		t.Fatalf("String() = %q", s)
	}
	parsed, err := ParseCloudResource(s)
	if err != nil {
		t.Fatalf("ParseCloudResource: %v", err)
	}
	if parsed != r {
		t.Fatalf("round trip mismatch: got %+v, want %+v", parsed, r)
	}
}

func TestParseCloudResourceRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "gcp:acct", "cloud:gcp", "cloud::acct", "cloud:gcp:", "aws:acct:x"} {
		if _, err := ParseCloudResource(bad); err == nil {
			t.Fatalf("ParseCloudResource(%q) = nil error, want error", bad)
		}
	}
}

func TestPreflightCloudAdmissionNilSourceIsUnknown(t *testing.T) {
	res := CloudResource{Provider: "gcp", Account: "acct"}
	admission, err := PreflightCloudAdmission(res, nil)
	if err != nil {
		t.Fatalf("PreflightCloudAdmission: %v", err)
	}
	if admission.Status != FundingUnknown {
		t.Fatalf("Status = %q, want FundingUnknown", admission.Status)
	}
}

func TestPreflightCloudAdmissionSourceErrorIsUnknown(t *testing.T) {
	res := CloudResource{Provider: "gcp", Account: "acct"}
	source := func(CloudResource) (FundingStatus, string, error) {
		return FundingFunded, "", errors.New("billing api unreachable")
	}
	admission, err := PreflightCloudAdmission(res, source)
	if err != nil {
		t.Fatalf("PreflightCloudAdmission: %v", err)
	}
	if admission.Status != FundingUnknown {
		t.Fatalf("Status = %q, want FundingUnknown on source error", admission.Status)
	}
}

func TestPreflightCloudAdmissionHonorsFundedStatus(t *testing.T) {
	res := CloudResource{Provider: "gcp", Account: "acct"}
	source := func(CloudResource) (FundingStatus, string, error) {
		return FundingFunded, "budget API confirms active credit", nil
	}
	admission, err := PreflightCloudAdmission(res, source)
	if err != nil {
		t.Fatalf("PreflightCloudAdmission: %v", err)
	}
	if admission.Status != FundingFunded {
		t.Fatalf("Status = %q, want FundingFunded", admission.Status)
	}
}

func TestPreflightCloudAdmissionRejectsInvalidResource(t *testing.T) {
	if _, err := PreflightCloudAdmission(CloudResource{}, nil); err == nil {
		t.Fatal("PreflightCloudAdmission(empty resource) = nil error, want error")
	}
}
