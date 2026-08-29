package sne

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func supportMatrixFixture() SupportMatrix {
	matrix := SupportMatrix{
		Schema: "sne.support-matrix.v2", CatalogRoot: "model-catalog/gemma4-v1",
		AsOf: "2026-08-20T19:50:00-04:00", SelectionPolicy: "exact-tuple-evidence-only", Fallback: "none",
		Entries: []SupportMatrixEntry{{
			TupleID: "12b@revision", CatalogEntryID: "12b", ModelID: "gemma-test", ModelRevision: "revision",
			ArtifactSetSHA256: strings.Repeat("a", 64), EvidenceSHA256: strings.Repeat("b", 64), Family: "gemma-4",
			Architecture: "gemma4-dense", ParameterClass: "12B", ArchitectureAdapter: "gemma4-dense-v0",
			ExecutionMode: "plain", Weight: Precision{Format: "affine", Bits: 8, GroupSize: 64},
			DeviceFamilies: []string{"Apple M5 Max"}, RequiredMemoryBytes: 1024, ServingCacheCapacity: 4096,
			ServingPolicy:         SupportServingPolicy{Profile: "interactive", MaxConcurrentRequests: 1, MaxQueuedRequests: 8, QueueDiscipline: "fifo", RequestTimeoutMS: 120000},
			ManifestQualification: "candidate", SupportStatus: "pilot-candidate", NextGate: "clean-host", Fallback: "none",
		}},
	}
	matrix.ClaimBoundaries.DeviceNonprojection = true
	matrix.ClaimBoundaries.MTPPlainSeparation = true
	matrix.ClaimBoundaries.PrecisionSeparation = true
	matrix.ClaimBoundaries.CandidateNotSupport = true
	matrix.ClaimBoundaries.ServingPolicyBound = true
	return matrix
}

func TestLoadSupportMatrixRejectsUnboundServingPolicy(t *testing.T) {
	matrix := supportMatrixFixture()
	matrix.Entries[0].ServingPolicy.MaxConcurrentRequests = 4
	matrixPath, signaturePath, publicPath := writeSignedSupportMatrix(t, t.TempDir(), matrix)
	if _, err := LoadSignedSupportMatrix(matrixPath, signaturePath, publicPath); err == nil || !strings.Contains(err.Error(), "invalid serving policy") {
		t.Fatalf("false native parallelism accepted: %v", err)
	}

	matrix = supportMatrixFixture()
	matrix.ClaimBoundaries.ServingPolicyBound = false
	matrixPath, signaturePath, publicPath = writeSignedSupportMatrix(t, t.TempDir(), matrix)
	if _, err := LoadSignedSupportMatrix(matrixPath, signaturePath, publicPath); err == nil || !strings.Contains(err.Error(), "claim boundaries") {
		t.Fatalf("unbound serving policy accepted: %v", err)
	}
}

func writeSignedSupportMatrix(t *testing.T, root string, matrix SupportMatrix) (string, string, string) {
	t.Helper()
	data, err := json.Marshal(matrix)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	matrixPath := filepath.Join(root, "support-matrix.json")
	signaturePath := matrixPath + ".sig"
	publicPath := filepath.Join(root, "public.pem")
	if err := os.WriteFile(matrixPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signaturePath, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, data))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return matrixPath, signaturePath, publicPath
}

func TestLoadSignedSupportMatrixRejectsTamperAndFalseRelease(t *testing.T) {
	root := t.TempDir()
	matrixPath, signaturePath, publicPath := writeSignedSupportMatrix(t, root, supportMatrixFixture())
	if _, err := LoadSignedSupportMatrix(matrixPath, signaturePath, publicPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(matrixPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSignedSupportMatrix(matrixPath, signaturePath, publicPath); err == nil || !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("tampered matrix accepted: %v", err)
	}

	invalid := supportMatrixFixture()
	invalid.Entries[0].SupportStatus = "release-supported"
	invalid.Entries[0].NextGate = "clean-host"
	matrixPath, signaturePath, publicPath = writeSignedSupportMatrix(t, t.TempDir(), invalid)
	if _, err := LoadSignedSupportMatrix(matrixPath, signaturePath, publicPath); err == nil || !strings.Contains(err.Error(), "before completion") {
		t.Fatalf("false release accepted: %v", err)
	}
}

func TestInstallSignedSupportMatrixIsVersionedAndPointerBounded(t *testing.T) {
	source := t.TempDir()
	matrixPath, signaturePath, publicPath := writeSignedSupportMatrix(t, source, supportMatrixFixture())
	store := t.TempDir()
	result, err := InstallSignedSupportMatrix(store, matrixPath, signaturePath, publicPath)
	if err != nil {
		t.Fatal(err)
	}
	current, err := CurrentSupportMatrixVersion(store)
	if err != nil {
		t.Fatal(err)
	}
	if current != result.VersionSHA256 || current == "" {
		t.Fatalf("current=%q result=%q", current, result.VersionSHA256)
	}
	if _, err := LoadSignedSupportMatrix(filepath.Join(store, supportMatrixCurrentLink, supportMatrixFile), filepath.Join(store, supportMatrixCurrentLink, supportMatrixSignature), publicPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store, supportMatrixCurrentLink)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp", filepath.Join(store, supportMatrixCurrentLink)); err != nil {
		t.Fatal(err)
	}
	if _, err := CurrentSupportMatrixVersion(store); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaping pointer accepted: %v", err)
	}
}
