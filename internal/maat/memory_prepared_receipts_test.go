package maat

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func interruptedPreparedReceipt(t *testing.T) (string, *Store, PreflightReceipt) {
	t.Helper()
	home := privateMemoryStoreRoot(t)
	t.Setenv("HOME", home)
	if _, err := InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	s, err := OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	r, err := s.Preflight(testScope())
	if err != nil {
		t.Fatal(err)
	}
	handle, err := s.retainPreflightReceiptWithWrite(r, receiptInterruptionWriter(t, s, "before-commit"))
	if !errors.Is(err, errReceiptWriteInterrupted) {
		t.Fatal(err)
	}
	journal := &recordingDecisionJournal{}
	if ProjectFailureMemoryPreflight(journal, handle) == nil || len(journal.decisions) != 0 {
		t.Fatal("failed first retention projected a Casebook decision")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".sirsi", "maat", "failure-memory"), s, r
}

func TestPreparedReceiptInspectorRecoversFirstRetentionWithoutCasebookRow(t *testing.T) {
	root, s, original := interruptedPreparedReceipt(t)
	before := receiptRecoveryFiles(t, root)
	prepared, err := s.InspectPreparedPreflightReceipt()
	if err != nil || prepared == nil {
		t.Fatalf("no discoverable exact recovery: %v", err)
	}
	if !bytes.Equal(before, receiptRecoveryFiles(t, root)) {
		t.Fatal("inspection wrote custody")
	}
	if prepared.Read.ActionAuthorized || prepared.Read.Receipt.localOrigin != nil || prepared.Read.Freshness != "historical" || !prepared.Read.Receipt.EvaluatedAtUTC.Equal(original.EvaluatedAtUTC) {
		t.Fatal("inspection fabricated fresh issuance")
	}
	decoded, err := DecodePreparedPreflightTarget([]byte(prepared.TargetJSON))
	if err != nil || decoded != prepared.Target || decoded.Reference.StoreBinding.StoreUUID != s.storeUUID {
		t.Fatalf("invalid captured binding: %v", err)
	}
	var wire PreflightReceiptRead
	if err := json.Unmarshal([]byte(prepared.ReadJSON), &wire); err != nil || wire.ReceiptSHA256 != decoded.Reference.ReceiptSHA256 {
		t.Fatalf("full historical reader missing: %v", err)
	}
	read, err := s.RecoverPreparedPreflightReceipt(decoded)
	if err != nil || read.ActionAuthorized || read.Receipt.localOrigin != nil || !read.Receipt.EvaluatedAtUTC.Equal(original.EvaluatedAtUTC) {
		t.Fatalf("bound recovery: %+v %v", read, err)
	}
	after := receiptRecoveryFiles(t, root)
	if _, err := s.RecoverPreparedPreflightReceipt(decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, receiptRecoveryFiles(t, root)) {
		t.Fatal("exact repeat rewrote history")
	}
	if pending, err := s.InspectPreparedPreflightReceipt(); err != nil || pending != nil {
		t.Fatalf("completed intent still offered: %+v %v", pending, err)
	}
	if names, err := readDirectoryNames(s.attemptsFD); err != nil || len(names) != 0 {
		t.Fatalf("recovery created attempt: %v %v", names, err)
	}
	if _, err := s.Preflight(testScope()); err != nil {
		t.Fatalf("fresh ordinary assessment unavailable: %v", err)
	}
}

func TestPreparedReceiptInspectorRefusesIncompleteOrUnrelatedDebt(t *testing.T) {
	for _, mode := range []string{"missing", "truncated", "changed-prepared", "unrelated-prepared"} {
		t.Run(mode, func(t *testing.T) {
			root, s, _ := interruptedPreparedReceipt(t)
			prepared, err := s.InspectPreparedPreflightReceipt()
			if err != nil || prepared == nil {
				t.Fatal(err)
			}
			leaf := filepath.Join(root, "evidence", prepared.Target.Reference.ReceiptSHA256)
			switch mode {
			case "missing":
				err = os.Remove(leaf)
			case "truncated":
				err = os.WriteFile(leaf, []byte("{"), 0o600)
			case "changed-prepared":
				err = os.WriteFile(filepath.Join(root, "writes", prepared.Target.IntentKey+".prepared.json"), []byte("{}"), 0o600)
			case "unrelated-prepared":
				intent, e := newWriteIntent("evidence", strings.Repeat("a", 64), []byte("other debt"))
				if e != nil {
					t.Fatal(e)
				}
				raw, e := json.Marshal(intent)
				if e != nil {
					t.Fatal(e)
				}
				err = createOrVerifyRaw(s.writesFD, intent.Key+".prepared.json", raw)
			}
			if err != nil {
				t.Fatal(err)
			}
			before := receiptRecoveryFiles(t, root)
			if got, err := s.InspectPreparedPreflightReceipt(); err == nil || got != nil {
				t.Fatal("unsafe debt offered as recoverable")
			}
			if _, err := s.RecoverPreparedPreflightReceipt(prepared.Target); err == nil {
				t.Fatal("captured approval waived changed custody")
			}
			if !bytes.Equal(before, receiptRecoveryFiles(t, root)) {
				t.Fatal("refusal modified custody")
			}
		})
	}
}

func TestPreparedReceiptRecoveryBindsEveryCapturedDimension(t *testing.T) {
	root, s, _ := interruptedPreparedReceipt(t)
	prepared, err := s.InspectPreparedPreflightReceipt()
	if err != nil || prepared == nil {
		t.Fatal(err)
	}
	for _, dimension := range []string{"device", "inode", "intent", "length", "uuid", "action", "registry", "receipt"} {
		t.Run(dimension, func(t *testing.T) {
			target := prepared.Target
			switch dimension {
			case "device":
				target.StoreDevice++
			case "inode":
				target.StoreInode++
			case "intent":
				target.IntentKey = strings.Repeat("f", 64)
			case "length":
				target.ByteLength++
			case "uuid":
				target.Reference.StoreBinding.StoreUUID = "c4d626b3-5884-4764-97e2-9b684d3dff56"
			case "action":
				target.Reference.ActionSHA256 = strings.Repeat("f", 64)
			case "registry":
				target.Reference.RegistrySHA256 = strings.Repeat("f", 64)
			case "receipt":
				target.Reference.ReceiptSHA256 = strings.Repeat("f", 64)
			}
			before := receiptRecoveryFiles(t, root)
			if _, err := s.RecoverPreparedPreflightReceipt(target); err == nil {
				t.Fatal("changed approval accepted")
			}
			if !bytes.Equal(before, receiptRecoveryFiles(t, root)) {
				t.Fatal("refused approval wrote custody")
			}
		})
	}
	for _, raw := range []string{prepared.TargetJSON + " ", strings.Replace(prepared.TargetJSON, `"schema":`, `"Schema":`, 1), strings.Replace(prepared.TargetJSON, `"schema":`, `"unknown":0,"schema":`, 1), strings.Replace(prepared.TargetJSON, `"store_inode":`, `"store_inode":1,"store_inode":`, 1)} {
		if _, err := DecodePreparedPreflightTarget([]byte(raw)); err == nil {
			t.Fatal("noncanonical target accepted")
		}
	}
}
