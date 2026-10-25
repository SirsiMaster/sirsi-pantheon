package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRetainedPreflightProjectsAndAdmitsWithoutSelfStaleness(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	m := fixtureLivenessManifest(t, s.storeUUID)
	r, err := s.PreflightLivenessWatch(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginLivenessWatchAttempt(m, r); err == nil {
		t.Fatal("unretained receipt admitted")
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	data, err := readExact(s.evidenceFD, digest)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.Marshal(r)
	if !bytes.Equal(data, expected) {
		t.Fatal("stored bytes differ from Digest bytes")
	}
	read, err := s.ReadPreflightReceipt(digest)
	if err != nil {
		t.Fatal(err)
	}
	if read.RegistryRelation != "same" || read.Freshness != "historical" || read.ActionAuthorized || read.StoreBinding.StoreUUID != s.storeUUID {
		t.Fatalf("false read authority/custody: %+v", read)
	}
	j := &recordingDecisionJournal{}
	if err := ProjectFailureMemoryPreflight(j, h); err != nil {
		t.Fatal(err)
	}
	if len(j.decisions) != 1 || j.decisions[0].FailureMemoryReceipt == nil || j.decisions[0].FailureMemoryReceipt.ReceiptSHA256 != digest {
		t.Fatalf("missing bound projection: %+v", j.decisions)
	}
	if _, err := s.PutEvidence([]byte("unreferenced safe evidence")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginLivenessWatchAttempt(m, r); err != nil {
		t.Fatalf("receipt bookkeeping self-staled admission: %v", err)
	}
	after, err := s.ReadPreflightReceipt(digest)
	if err != nil || after.RegistryRelation != "same" || after.ActionAuthorized {
		t.Fatalf("attempt bookkeeping changed semantic snapshot: %+v %v", after, err)
	}
}

func TestRetainedPreflightPreservesHistoricalSnapshotAfterIncidentDrift(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := s.PutEvidence([]byte("new verified local evidence"))
	if err != nil {
		t.Fatal(err)
	}
	i, err := NewIncident(testSignature(), evidence, testScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, testActions())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(i); err != nil {
		t.Fatal(err)
	}
	read, err := s.ReadPreflightReceipt(h.Reference().ReceiptSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if read.Receipt.Decision != PreflightPass || read.RegistryRelation != "changed" || read.ActionAuthorized {
		t.Fatalf("history promoted or overwritten: %+v", read)
	}
	if _, err := s.RetainPreflightReceipt(r); err == nil {
		t.Fatal("stale receipt re-attested")
	}
}

func TestPreflightReceiptCanonicalHistoricalGoldensAndNullDistinction(t *testing.T) {
	r := PreflightReceipt{Schema: MemorySchema, Action: testScope(), ActionManifestSHA256: strings.Repeat("a", 64), RegistrySnapshotSHA256: strings.Repeat("b", 64), Decision: PreflightPass, EvaluatedAtUTC: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}
	canonical, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	golden := `{"schema":"sirsi.maat.failure-memory.v1","action":{"component":"router","profile":"macos-local","operation":"service-repair"},"action_manifest_sha256":"` + strings.Repeat("a", 64) + `","registry_snapshot_sha256":"` + strings.Repeat("b", 64) + `","evaluated_guards":null,"measured_checks":null,"decision":"pass","incident_keys":null,"recovery_actions":null,"recovery_reference":"","evaluated_at_utc":"2026-10-24T00:00:00Z"}`
	if string(canonical) != golden {
		t.Fatalf("historical canonical golden changed: %s", canonical)
	}

	// These historical required fields must stay null, not omitted or [];
	// adding v2 must not relabel old bytes.
	for _, literal := range []string{`"evaluated_guards":null`, `"measured_checks":null`, `"incident_keys":null`, `"recovery_actions":null`} {
		if !bytes.Contains(canonical, []byte(literal)) {
			t.Fatalf("lost canonical field %s", literal)
		}
	}
	if bytes.Contains(canonical, []byte("registry_snapshot_version")) {
		t.Fatal("legacy receipt rewritten as v2")
	}
	decoded, err := decodePreflightReceipt(canonical)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := decoded.Digest()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	if digest != hex.EncodeToString(sum[:]) {
		t.Fatal("Digest contract differs")
	}
	r.IncidentKeys = []string{}
	empty, _ := json.Marshal(r)
	if bytes.Equal(canonical, empty) {
		t.Fatal("null and empty historical bytes collapsed")
	}
	g, _ := json.Marshal(GuardEvaluation{ID: "guard", Version: "v1", SHA256: strings.Repeat("a", 64)})
	c, _ := json.Marshal(CheckOutcome{IncidentKey: "incident", EvidenceSHA256: "proof", Status: "active", Outcome: "outcome"})
	if string(g) != `{"ID":"guard","Version":"v1","SHA256":"`+strings.Repeat("a", 64)+`"}` || string(c) != `{"IncidentKey":"incident","EvidenceSHA256":"proof","Status":"active","Outcome":"outcome"}` {
		t.Fatal("historical inner key casing changed")
	}
	for _, bad := range [][]byte{append(append([]byte{}, canonical...), []byte("{}")...), append(append([]byte{}, canonical...), '\n'), bytes.Replace(canonical, []byte(`"schema":`), []byte(`"Schema":`), 1), bytes.Replace(canonical, []byte(`"schema":`), []byte(`"unknown":0,"schema":`), 1), bytes.Replace(canonical, []byte(`"decision":"pass"`), []byte(`"decision":"pass","decision":"pass"`), 1), bytes.Replace(canonical, []byte(`"decision":"pass"`), []byte(`"decision":"unknown"`), 1), bytes.Replace(canonical, []byte(`"decision":"pass"`), []byte(`"decision":"pass","registry_snapshot_version":3`), 1)} {
		if _, err := decodePreflightReceipt(bad); err == nil {
			t.Fatalf("noncanonical/unknown receipt accepted: %s", bad)
		}
	}
}

func TestPreflightReceiptReadRefusesUncommittedAndUnsafeObjects(t *testing.T) {
	for _, mode := range []string{"missing", "uncommitted", "tampered", "symlink", "fifo", "multiple-links", "oversize", "truncated", "missing-commit", "prepared-unrelated"} {
		t.Run(mode, func(t *testing.T) {
			root := privateMemoryStoreRoot(t)
			s, err := OpenStore(root)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r, err := s.Preflight(testScope())
			if err != nil {
				t.Fatal(err)
			}
			h, err := s.RetainPreflightReceipt(r)
			if err != nil {
				t.Fatal(err)
			}
			digest := h.Reference().ReceiptSHA256
			leaf := filepath.Join(root, "evidence", digest)
			data, err := os.ReadFile(leaf)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing":
				if err := os.Remove(leaf); err != nil {
					t.Fatal(err)
				}
			case "uncommitted":
				data = bytes.Replace(data, []byte(`"registry_snapshot_version":2`), []byte(`"registry_snapshot_version":0`), 1)
				sum := sha256.Sum256(data)
				digest = hex.EncodeToString(sum[:])
				if err := os.WriteFile(filepath.Join(root, "evidence", digest), data, 0o600); err != nil {
					t.Fatal(err)
				}
			case "tampered":
				if err := os.WriteFile(leaf, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "truncated":
				if err := os.WriteFile(leaf, data[:len(data)-1], 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink", "fifo":
				if err := os.Remove(leaf); err != nil {
					t.Fatal(err)
				}
				if mode == "symlink" {
					if err := os.Symlink("missing", leaf); err != nil {
						t.Fatal(err)
					}
				} else if err := unix.Mkfifo(leaf, 0o600); err != nil {
					t.Fatal(err)
				}
			case "multiple-links":
				if err := os.Link(leaf, filepath.Join(root, "second-link")); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				if err := os.WriteFile(leaf, bytes.Repeat([]byte("x"), maxMemoryObjectBytes+1), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing-commit":
				intent, err := newWriteIntent("evidence", digest, data)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(root, "writes", intent.Key+".committed.json")); err != nil {
					t.Fatal(err)
				}
			case "prepared-unrelated":
				other := []byte("interrupted evidence")
				sum := sha256.Sum256(other)
				intent, err := newWriteIntent("evidence", hex.EncodeToString(sum[:]), other)
				if err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(intent)
				if err := createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", b); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.ReadPreflightReceipt(digest); err == nil {
				t.Fatal("unsafe/uncommitted read accepted")
			}
			if mode == "prepared-unrelated" {
				if current, err := s.Preflight(testScope()); err == nil || current.Decision != PreflightUnverifiable {
					t.Fatal("semantic filtering masked unresolved WAL")
				}
			}
		})
	}
}

func TestExistingStoreReadNeverCreatesMissingLayout(t *testing.T) {
	root := privateMemoryStoreRoot(t)
	if _, err := OpenExistingStore(filepath.Join(root, "absent")); err == nil {
		t.Fatal("absent store opened")
	}
	if _, err := OpenExistingStore(root); err == nil {
		t.Fatal("empty root adopted")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inspection created layout: %v %v", entries, err)
	}
}

func TestProjectionFailureLeavesExactRetainedReceiptReadable(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := ProjectFailureMemoryPreflight(&failingMemoryJournal{}, h); err == nil {
		t.Fatal("journal failure hidden")
	}
	if read, err := s.ReadPreflightReceipt(h.Reference().ReceiptSHA256); err != nil || read.Receipt.Decision != r.Decision {
		t.Fatalf("retained bytes lost after journal refusal: %+v %v", read, err)
	}
	if err := ProjectFailureMemoryPreflight(&recordingDecisionJournal{}, StoredPreflightReceipt{}); err == nil {
		t.Fatal("zero opaque handle accepted")
	}
}

func TestStoredPreflightHandleDoesNotExposeMutableReceipt(t *testing.T) {
	s, _ := memoryGuardFixture(t, ActiveIncidentGuardID, ActiveIncidentGuardVersion)
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	copy, err := h.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	copy.IncidentKeys[0] = strings.Repeat("f", 64)
	copy.RecoveryActions[0].Instruction = "changed caller text"
	original, err := h.Receipt()
	if err != nil {
		t.Fatal(err)
	}
	if original.IncidentKeys[0] != r.IncidentKeys[0] || original.RecoveryActions[0].Instruction != r.RecoveryActions[0].Instruction {
		t.Fatal("opaque handle exposes mutable custody")
	}
}

func TestReceiptRetentionRefusesDecodedChangedAndForeignAssessments(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var imported PreflightReceipt
	if err := json.Unmarshal(data, &imported); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RetainPreflightReceipt(imported); err == nil {
		t.Fatal("decoded JSON impersonated an issued local assessment")
	}
	changed := r
	changed.ActionManifestSHA256 = strings.Repeat("f", 64)
	if _, err := s.RetainPreflightReceipt(changed); err == nil {
		t.Fatal("caller rewrote the issued action binding")
	}
	foreign, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	if _, err := foreign.RetainPreflightReceipt(r); err == nil {
		t.Fatal("another store's assessment acquired custody")
	}
}

func TestConcurrentRetentionUsesOneCommittedExactReceipt(t *testing.T) {
	root := privateMemoryStoreRoot(t)
	first, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenExistingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	r, err := first.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, s := range []*Store{first, second} {
		go func(s *Store) { _, err := s.RetainPreflightReceipt(r); results <- err }(s)
	}
	var failures []error
	for range 2 {
		if err := <-results; err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) != 0 {
		t.Fatal(failures)
	}
	digest, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	read, err := second.ReadPreflightReceipt(digest)
	if err != nil || read.RegistryRelation != "same" {
		t.Fatalf("concurrent custody: %+v %v", read, err)
	}
	names, err := readDirectoryNames(first.evidenceFD)
	if err != nil || len(names) != 1 || names[0] != digest {
		t.Fatalf("retention duplicated evidence: %v %v", names, err)
	}
}

func TestLegacyCommittedReceiptReadDoesNotUpgradeOrReconstruct(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := PreflightReceipt{Schema: MemorySchema, Action: testScope(), ActionManifestSHA256: strings.Repeat("a", 64), RegistrySnapshotSHA256: strings.Repeat("b", 64), Decision: PreflightPass, EvaluatedAtUTC: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := s.PutEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.ReadPreflightReceipt(digest)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(read.Receipt)
	if err != nil || !bytes.Equal(actual, data) || read.RegistryRelation != "unverifiable" || read.ActionAuthorized || read.Receipt.RegistrySnapshotVersion != 0 {
		t.Fatalf("legacy receipt relabeled: %+v %v", read, err)
	}
	if _, err := s.RetainPreflightReceipt(read.Receipt); err == nil {
		t.Fatal("legacy receipt acquired new-retention authority")
	}
	if _, err := s.ReadPreflightReceipt(strings.Repeat("f", 64)); err == nil {
		t.Fatal("missing legacy receipt reconstructed")
	}
}
