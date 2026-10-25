package maat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	internalversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"golang.org/x/sys/unix"
)

func rawSelfUpdateFixture(t *testing.T) (*Store, RawSelfUpdateManifest, StoredPreflightReceipt) {
	t.Helper()
	root := privateMemoryStoreRoot(t)
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	t.Cleanup(func() { _ = store.Close() })
	selfPath, targetPath := filepath.Join(root, "sirsi-self"), filepath.Join(root, "sirsi-target")
	for _, item := range []struct{ path, content string }{{selfPath, "running-release"}, {targetPath, "stale-release"}} {
		if err := os.WriteFile(item.path, []byte(item.content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	identity := func(path string) RawSelfUpdateTarget {
		t.Helper()
		var stat unix.Stat_t
		if err := unix.Stat(path, &stat); err != nil {
			t.Fatal(err)
		}
		digest, err := testFileDigest(path)
		if err != nil {
			t.Fatal(err)
		}
		return RawSelfUpdateTarget{Path: path, Method: "raw", SHA256: digest, OwnerUID: stat.Uid,
			Device: uint64(stat.Dev), Inode: stat.Ino, Mode: uint32(stat.Mode)}
	}
	self, target := identity(selfPath), identity(targetPath)
	info := internalversion.Current("sirsi")
	info.Path, info.Version, info.Commit, info.Dirty = selfPath, "0.24.137", "fixture", false
	info.RouterSchemaMax = routerstore.MaxSupportedSchemaVersion()
	manifest := RawSelfUpdateManifest{Schema: RawSelfUpdateManifestSchema, PreviewBinding: strings.Repeat("b", 64),
		SelfPath: selfPath, SelfSHA256: self.SHA256, SelfOwnerUID: self.OwnerUID, SelfDevice: self.Device,
		SelfInode: self.Inode, SelfMode: self.Mode, RunningInfo: info, Targets: []RawSelfUpdateTarget{target},
		AuthorizedUID: uint32(os.Geteuid()), LiveRouterSchema: 0, ObservedSchemaCompatible: true,
		StoreUUID: store.storeUUID, ObservedAtUTC: time.Now().UTC()}
	receipt, err := store.PreflightRawSelfUpdate(manifest)
	if err != nil && receipt.Decision != PreflightUnverifiable {
		t.Fatalf("raw self-update preflight: %v", err)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		t.Fatalf("retain raw self-update receipt: %v", err)
	}
	return store, manifest, stored
}

func TestRawSelfUpdateAttemptRetainsExactReceiptAndProjectionDebtUntilReadback(t *testing.T) {
	store, manifest, stored := rawSelfUpdateFixture(t)
	attempt, err := store.BeginRawSelfUpdateAttempt(manifest, stored)
	if err != nil {
		t.Fatalf("begin raw self-update: %v", err)
	}
	if got, err := store.PendingRawSelfUpdateAttempt(); err != nil || got.ID != attempt.ID {
		t.Fatalf("intent without invocation must remain pending: got=%s err=%v", got.ID, err)
	}
	if err := store.RecordRawSelfUpdateInvocation(attempt); err != nil {
		t.Fatalf("record invocation barrier: %v", err)
	}
	if got, err := store.PendingRawSelfUpdateAttempt(); err != nil || got.ID != attempt.ID {
		t.Fatalf("invocation without terminal outcome must remain pending: got=%s err=%v", got.ID, err)
	}
	if _, err := store.BeginRawSelfUpdateAttempt(manifest, stored); err == nil {
		t.Fatal("a second action was admitted while the invocation outcome was unknown")
	}
	observation := strings.Repeat("c", 64)
	outcome, err := store.RecordRawSelfUpdateOutcome(attempt, true, observation, "exact target postcheck passed")
	if err != nil {
		t.Fatalf("record terminal postcheck: %v", err)
	}
	if !outcome.PostcheckPassed || outcome.DecisionSHA256 == "" {
		t.Fatal("terminal record did not retain the exact postcheck and projection intent")
	}
	if err := store.lock(unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	projectionDebtErr := store.ensureNoConflictingRawSelfUpdateLocked()
	store.unlock()
	if projectionDebtErr == nil {
		t.Fatal("unread terminal projection debt admitted a conflicting operation")
	}
	if err := store.MarkRawSelfUpdateProjectionReadBack(attempt, outcome, outcome.Decision); err != nil {
		t.Fatalf("record exact projection readback: %v", err)
	}
	if _, err := store.PendingRawSelfUpdateAttempt(); !errors.Is(err, ErrNoIncompleteRawSelfUpdateAttempt) {
		t.Fatalf("completed projection remained pending after exact readback: err=%v", err)
	}
	history, err := store.RawSelfUpdateReconciliations()
	if err != nil || len(history) != 1 || !history[0].ProjectionReadBack {
		t.Fatalf("raw self-update history did not bind exact readback: entries=%d err=%v", len(history), err)
	}
	if err := store.lock(unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	conflictErr := store.ensureNoConflictingRawSelfUpdateLocked()
	store.unlock()
	if conflictErr != nil {
		t.Fatalf("verified terminal readback left false conflict: %v", conflictErr)
	}
}

func TestRawSelfUpdateHistoricalManifestReadableButNotFreshForAdmission(t *testing.T) {
	_, manifest, _ := rawSelfUpdateFixture(t)
	oldNow := binaryUpdateNow
	now := time.Now().UTC()
	binaryUpdateNow = func() time.Time { return now }
	t.Cleanup(func() { binaryUpdateNow = oldNow })
	manifest.ObservedAtUTC = now.Add(-3 * time.Minute)
	if err := manifest.Validate(); err != nil {
		t.Fatalf("historical manifest failed structural validation: %v", err)
	}
	if _, err := manifest.Digest(); err != nil {
		t.Fatalf("historical manifest digest failed: %v", err)
	}
	if err := manifest.ValidateFresh(); err == nil {
		t.Fatal("stale preview was admitted for a new raw self-update")
	}
}
