package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

var errReceiptWriteInterrupted = errors.New("owned fixture: receipt persistence interrupted")

// Fail the actual retention writer at either durable prefix, without global
// hooks, privileged permissions, clock sleeps, or a raw production importer.
func receiptInterruptionWriter(t *testing.T, s *Store, stage string) func(string, int, string, []byte) error {
	t.Helper()
	return func(namespace string, fd int, leaf string, data []byte) error {
		if namespace != "evidence" {
			t.Errorf("unexpected namespace %s", namespace)
			return errReceiptWriteInterrupted
		}
		intent, err := newWriteIntent(namespace, leaf, data)
		if err != nil {
			return err
		}
		prepared, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		if err := createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", prepared); err != nil {
			return err
		}
		if stage == "before-commit" {
			if err := createOrVerifyRaw(fd, leaf, data); err != nil {
				return err
			}
		}
		return errReceiptWriteInterrupted
	}
}

func TestRetainPreflightExactRetryResumesBothInterruptedPrefixesBeforeAdmission(t *testing.T) {
	for _, stage := range []string{"before-leaf", "before-commit"} {
		t.Run(stage, func(t *testing.T) {
			s, err := OpenStore(privateMemoryStoreRoot(t))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
			manifest := fixtureLivenessManifest(t, s.storeUUID)
			r, err := s.PreflightLivenessWatch(manifest)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := r.Digest()
			if err != nil {
				t.Fatal(err)
			}
			h, err := s.retainPreflightReceiptWithWrite(r, receiptInterruptionWriter(t, s, stage))
			if !errors.Is(err, errReceiptWriteInterrupted) {
				t.Fatalf("write did not reach requested prefix: %v", err)
			}
			journal := &recordingDecisionJournal{}
			if err := ProjectFailureMemoryPreflight(journal, h); err == nil || len(journal.decisions) != 0 {
				t.Fatal("uncommitted retention produced a usable handle/projection")
			}
			if _, err := s.BeginLivenessWatchAttempt(manifest, r); err == nil {
				t.Fatal("attempt began before exact receipt commit")
			}
			if names, err := readDirectoryNames(s.attemptsFD); err != nil || len(names) != 0 {
				t.Fatalf("interruption produced effects: %v %v", names, err)
			}
			if _, err := s.ReadPreflightReceipt(digest); err == nil {
				t.Fatal("pure read waived the pending receipt")
			}
			stored, err := s.RetainPreflightReceipt(r)
			if err != nil {
				t.Fatalf("exact issued tuple retry stranded: %v", err)
			}
			if stored.Reference().ReceiptSHA256 != digest {
				t.Fatal("retry changed receipt digest")
			}
			actual, err := readExact(s.evidenceFD, digest)
			if err != nil || !bytes.Equal(actual, expected) {
				t.Fatalf("retry changed original bytes: %v", err)
			}
			retained, err := stored.Receipt()
			if err != nil || !retained.EvaluatedAtUTC.Equal(r.EvaluatedAtUTC) {
				t.Fatal("retry rewrote observation time")
			}
			if _, err := s.verifyWriteJournal(); err != nil {
				t.Fatal(err)
			}
			if err := ProjectFailureMemoryPreflight(journal, stored); err != nil {
				t.Fatal(err)
			}
			if _, err := s.BeginLivenessWatchAttempt(manifest, r); err != nil {
				t.Fatalf("ordinary admission failed after exact commit/readback: %v", err)
			}
		})
	}
}

func TestHistoricalReceiptRecoveryReopensOnlyCompletePreparedTarget(t *testing.T) {
	root := privateMemoryStoreRoot(t)
	s, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.retainPreflightReceiptWithWrite(r, receiptInterruptionWriter(t, s, "before-commit")); !errors.Is(err, errReceiptWriteInterrupted) {
		t.Fatal(err)
	}
	digest, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenExistingStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	read, err := s.RecoverPreflightReceipt(digest)
	if err != nil {
		t.Fatalf("exact retained target could not recover: %v", err)
	}
	actual, err := json.Marshal(read.Receipt)
	if err != nil || !bytes.Equal(actual, expected) || read.ActionAuthorized || read.Freshness != "historical" {
		t.Fatalf("recovery manufactured current authority/bytes: %+v %v", read, err)
	}
	if read.Receipt.localOrigin != nil {
		t.Fatal("historical recovery minted local issuance")
	}
	if _, err := s.RetainPreflightReceipt(read.Receipt); err == nil {
		t.Fatal("historical recovery was accepted as a new issued assessment")
	}
	if again, err := s.RecoverPreflightReceipt(digest); err != nil || !again.Receipt.EvaluatedAtUTC.Equal(r.EvaluatedAtUTC) {
		t.Fatalf("recovery is not idempotent: %+v %v", again, err)
	}
	if names, err := readDirectoryNames(s.attemptsFD); err != nil || len(names) != 0 {
		t.Fatalf("historical recovery created an action: %v %v", names, err)
	}
}

func TestReceiptRetryClosesOldPersistenceWithoutIssuingFreshPermission(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	// Simulate elapsed wall time on the owned issuance fixture, not a public
	// receipt constructor or a production clock override.
	r.EvaluatedAtUTC = r.EvaluatedAtUTC.Add(-3 * time.Minute)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	r.localOrigin.canonical = string(data)
	digest, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := receiptInterruptionWriter(t, s, "before-leaf")("evidence", s.evidenceFD, digest, data); !errors.Is(err, errReceiptWriteInterrupted) {
		t.Fatal(err)
	}
	stored, err := s.RetainPreflightReceipt(r)
	if err == nil {
		t.Fatal("old issued observation became fresh eligibility")
	}
	if _, err := stored.Receipt(); err == nil {
		t.Fatal("stale retry returned a projection handle")
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		t.Fatalf("exact old persistence remained stranded: %v", err)
	}
	read, err := s.ReadPreflightReceipt(digest)
	if err != nil || !read.Receipt.EvaluatedAtUTC.Equal(r.EvaluatedAtUTC) || read.ActionAuthorized {
		t.Fatalf("old bytes rewritten or promoted: %+v %v", read, err)
	}
}

func TestReceiptRetryCommitsExactHistoryButRefusesChangedRegistryEligibility(t *testing.T) {
	s, err := OpenStore(privateMemoryStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.retainPreflightReceiptWithWrite(r, receiptInterruptionWriter(t, s, "before-leaf")); !errors.Is(err, errReceiptWriteInterrupted) {
		t.Fatal(err)
	}
	// Independently materialize a fully committed inert registry mutation.
	// This bypass exists only in the fixture, never in receipt recovery.
	commitFixture := func(namespace string, fd int, leaf string, data []byte) {
		t.Helper()
		intent, err := newWriteIntent(namespace, leaf, data)
		if err != nil {
			t.Fatal(err)
		}
		prepared, err := json.Marshal(intent)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := json.Marshal(writeCommit{Schema: MemorySchema, IntentKey: intent.Key})
		if err != nil {
			t.Fatal(err)
		}
		if err := createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", prepared); err != nil {
			t.Fatal(err)
		}
		if err := createOrVerifyRaw(fd, leaf, data); err != nil {
			t.Fatal(err)
		}
		if err := createOrVerifyRaw(s.writesFD, intent.Key+".committed.json", committed); err != nil {
			t.Fatal(err)
		}
	}
	proof := []byte("inert independently committed registry proof")
	sum := sha256.Sum256(proof)
	proofSHA := hex.EncodeToString(sum[:])
	commitFixture("evidence", s.evidenceFD, proofSHA, proof)
	incident, err := NewIncident(testSignature(), proofSHA, testScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, testActions())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(incident)
	if err != nil {
		t.Fatal(err)
	}
	commitFixture("incidents", s.incidentFD, incident.Key+".json", data)
	stored, err := s.RetainPreflightReceipt(r)
	if err == nil {
		t.Fatal("exact persistence recovery waived a changed registry")
	}
	if _, err := stored.Receipt(); err == nil {
		t.Fatal("stale recovery returned a projection handle")
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		t.Fatalf("exact old tuple remained stranded: %v", err)
	}
	digest, err := r.Digest()
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.ReadPreflightReceipt(digest)
	if err != nil || read.Receipt.Decision != PreflightPass || read.RegistryRelation != "changed" || read.ActionAuthorized {
		t.Fatalf("historical result rewritten or promoted: %+v %v", read, err)
	}
}

func TestHistoricalReceiptRecoveryRefusesMissingChangedAndUnrelatedDebt(t *testing.T) {
	for _, mode := range []string{"missing", "truncated", "substituted", "unrelated-prepared", "missing-prepared", "changed-prepared"} {
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
			if _, err := s.retainPreflightReceiptWithWrite(r, receiptInterruptionWriter(t, s, "before-commit")); !errors.Is(err, errReceiptWriteInterrupted) {
				t.Fatal(err)
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := r.Digest()
			if err != nil {
				t.Fatal(err)
			}
			intent, err := newWriteIntent("evidence", digest, data)
			if err != nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(root, "evidence", digest)
			prepared := filepath.Join(root, "writes", intent.Key+".prepared.json")
			switch mode {
			case "missing":
				if err := os.Remove(leaf); err != nil {
					t.Fatal(err)
				}
			case "truncated":
				if err := os.WriteFile(leaf, data[:len(data)-1], 0o600); err != nil {
					t.Fatal(err)
				}
			case "substituted":
				changed := r
				changed.EvaluatedAtUTC = changed.EvaluatedAtUTC.Add(time.Second)
				other, err := json.Marshal(changed)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(leaf, other, 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing-prepared":
				if err := os.Remove(prepared); err != nil {
					t.Fatal(err)
				}
			case "changed-prepared":
				if err := os.WriteFile(prepared, []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unrelated-prepared":
				other := []byte("unrelated incomplete proof")
				sum := sha256.Sum256(other)
				pending, err := newWriteIntent("evidence", hex.EncodeToString(sum[:]), other)
				if err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(pending)
				if err != nil {
					t.Fatal(err)
				}
				if err := createOrVerifyRaw(s.writesFD, pending.Key+".prepared.json", b); err != nil {
					t.Fatal(err)
				}
			}
			before := receiptRecoveryFiles(t, root)
			if _, err := s.RecoverPreflightReceipt(digest); err == nil {
				t.Fatal("unsafe or unrelated debt recovered")
			}
			if mode == "unrelated-prepared" {
				if _, err := s.RetainPreflightReceipt(r); err == nil {
					t.Fatal("normal retry waived unrelated prepared debt")
				}
			}
			after := receiptRecoveryFiles(t, root)
			if !bytes.Equal(before, after) {
				t.Fatal("refused recovery modified retained bytes or modes")
			}
		})
	}
}

func receiptRecoveryFiles(t *testing.T, root string) []byte {
	t.Helper()
	records := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		records[rel] = info.Mode().String() + ":" + hex.EncodeToString(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReceiptAndWALReadersRefuseUnchangedNonprivateLeaves(t *testing.T) {
	for _, kind := range []string{"receipt", "prepared", "committed"} {
		for _, mode := range []os.FileMode{0o644, 0o660} {
			t.Run(kind+"-"+mode.String(), func(t *testing.T) {
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
				stored, err := s.RetainPreflightReceipt(r)
				if err != nil {
					t.Fatal(err)
				}
				digest := stored.Reference().ReceiptSHA256
				data, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				intent, err := newWriteIntent("evidence", digest, data)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, "evidence", digest)
				if kind != "receipt" {
					path = filepath.Join(root, "writes", intent.Key+"."+kind+".json")
				}
				original, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, mode); err != nil {
					t.Fatal(err)
				}
				if _, err := s.ReadPreflightReceipt(digest); err == nil {
					t.Fatal("nonprivate unchanged bytes accepted by verified read")
				}
				if _, err := s.RetainPreflightReceipt(r); err == nil {
					t.Fatal("normal reuse adopted nonprivate custody")
				}
				if _, err := s.RecoverPreflightReceipt(digest); err == nil {
					t.Fatal("recovery adopted nonprivate custody")
				}
				actual, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(actual, original) {
					t.Fatalf("refusal changed bytes: %v", err)
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != mode {
					t.Fatalf("refusal silently chmodded leaf: %v", err)
				}
			})
		}
	}
}

func TestReceiptOpenedStatRejectsForeignOwnerAndReadbackOwnershipDrift(t *testing.T) {
	const owner = uint32(501)
	before := unix.Stat_t{Mode: unix.S_IFREG | 0o600, Nlink: 1, Uid: owner, Size: 2, Dev: 1, Ino: 2}
	if err := validateMemoryObjectStat(before, owner); err != nil {
		t.Fatal(err)
	}
	if err := validateMemoryObjectReadback(before, before, owner); err != nil {
		t.Fatal(err)
	}
	foreign := before
	foreign.Uid = owner + 1
	if err := validateMemoryObjectStat(foreign, owner); err == nil {
		t.Fatal("foreign owner passed the opened-FD policy")
	}
	if err := validateMemoryObjectReadback(before, foreign, owner); err == nil {
		t.Fatal("ownership drift passed retained-FD readback")
	}
	if err := validateMemoryObjectReadback(before, foreign, owner+1); err == nil {
		t.Fatal("readback adopted changed owner after an actor credential change")
	}
	for index := range 2 {
		changed := before
		if index == 0 {
			changed.Mode = unix.S_IFREG | 0o644
		} else {
			changed.Mode = unix.S_IFREG | 0o660
		}
		if err := validateMemoryObjectStat(changed, owner); err == nil {
			t.Fatal("nonprivate stat accepted")
		}
	}
}

// Main runs this unchanged fixture under both child umask022 and077. Creation
// modes are explicit; the fixture never changes the process-global umask.
func TestReceiptAndWALPositiveCustodyUsesExplicitPrivateModes(t *testing.T) {
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
	stored, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadPreflightReceipt(stored.Reference().ReceiptSHA256); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		want := os.FileMode(0o600)
		if entry.IsDir() {
			want = 0o700
		}
		if info.Mode().Perm() != want {
			return errors.New("owned fixture mode is not explicitly private: " + strings.TrimPrefix(path, root))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
