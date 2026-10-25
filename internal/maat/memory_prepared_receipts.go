package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const PreparedPreflightReceiptSchema = "sirsi.maat.prepared-preflight-receipt.v1"

// An inspection target binds existing private custody, not action authority.
// Native callers forward TargetJSON unchanged; no raw bytes may be imported.
type PreparedPreflightTarget struct {
	Schema      string                    `json:"schema"`
	Reference   PreflightReceiptReference `json:"reference"`
	StoreDevice uint64                    `json:"store_device"`
	StoreInode  uint64                    `json:"store_inode"`
	IntentKey   string                    `json:"intent_key"`
	ByteLength  int                       `json:"byte_length"`
}

type PreparedPreflightReceipt struct {
	Schema     string                  `json:"schema"`
	Target     PreparedPreflightTarget `json:"target"`
	TargetJSON string                  `json:"target_json"`
	Read       PreflightReceiptRead    `json:"-"`
	ReadJSON   string                  `json:"read_json"`
}

func DecodePreparedPreflightTarget(raw []byte) (PreparedPreflightTarget, error) {
	var target PreparedPreflightTarget
	if len(raw) == 0 || len(raw) > 8192 || json.Unmarshal(raw, &target) != nil {
		return target, errors.New("maat memory: bounded prepared target required")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, raw) || target.Schema != PreparedPreflightReceiptSchema || target.Reference.validate() != nil || target.StoreInode == 0 || !validDigest(target.IntentKey) || target.ByteLength <= 0 || target.ByteLength > maxMemoryObjectBytes {
		return target, errors.New("maat memory: exact closed prepared target required")
	}
	return target, nil
}

// InspectPreparedPreflightReceipt is read-only and needs no prior Casebook row.
// A candidate is returned only for complete canonical v2 receipt bytes and one
// exact uncommitted intent, with the entire remaining WAL/targets verified.
func (s *Store) InspectPreparedPreflightReceipt() (*PreparedPreflightReceipt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	names, err := readDirectoryNames(s.writesFD)
	if err != nil {
		return nil, err
	}
	var pending *writeIntent
	for _, name := range names {
		if !strings.HasSuffix(name, ".prepared.json") {
			continue
		}
		raw, err := readExact(s.writesFD, name)
		if err != nil {
			return nil, err
		}
		var intent writeIntent
		if json.Unmarshal(raw, &intent) != nil || intent.validate() != nil || name != intent.Key+".prepared.json" {
			return nil, errors.New("maat memory: unverifiable prepared receipt custody")
		}
		if _, err := readExact(s.writesFD, intent.Key+".committed.json"); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if pending != nil {
			return nil, errors.New("maat memory: unrelated prepared debt remains")
		}
		pending = &intent
	}
	if pending == nil {
		_, err := s.verifyWriteJournal()
		return nil, err
	}
	if pending.Namespace != "evidence" || pending.Leaf != pending.SHA256 {
		return nil, errors.New("maat memory: prepared debt is not an exact preflight receipt")
	}
	return s.inspectPreparedReceiptLocked(pending.Leaf)
}

func (s *Store) inspectPreparedReceiptLocked(digest string) (*PreparedPreflightReceipt, error) {
	data, err := readExact(s.evidenceFD, digest)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("maat memory: prepared receipt digest differs")
	}
	r, err := decodePreflightReceipt(data)
	if err != nil || r.RegistrySnapshotVersion != 2 {
		return nil, errors.New("maat memory: complete canonical v2 prepared receipt required")
	}
	intent, err := newWriteIntent("evidence", digest, data)
	if err != nil {
		return nil, err
	}
	expected, err := json.Marshal(intent)
	if err != nil {
		return nil, err
	}
	prepared, err := readExact(s.writesFD, intent.Key+".prepared.json")
	if err != nil || !bytes.Equal(prepared, expected) {
		return nil, errors.New("maat memory: exact prepared receipt intent unavailable")
	}
	if _, err := s.verifyWriteJournalAllow(intent.Key); err != nil {
		return nil, err
	}
	var identity unix.Stat_t
	if err := unix.Fstat(s.rootFD, &identity); err != nil {
		return nil, err
	}
	if s.storeUUID != "" {
		if err := validateGenesis(s.rootFD, s.storeUUID, uint32(os.Geteuid())); err != nil {
			return nil, err
		}
	}
	ref := PreflightReceiptReference{ReceiptSHA256: digest, ActionSHA256: r.ActionManifestSHA256,
		RegistrySHA256: r.RegistrySnapshotSHA256, RegistrySnapshotVersion: 2, Action: r.Action, StoreBinding: s.receiptBinding()}
	target := PreparedPreflightTarget{PreparedPreflightReceiptSchema, ref, uint64(identity.Dev), identity.Ino, intent.Key, len(data)}
	raw, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}
	if _, err := DecodePreparedPreflightTarget(raw); err != nil {
		return nil, err
	}
	read := PreflightReceiptRead{Schema: PreflightReceiptReadSchema, ReceiptSHA256: digest, Receipt: r, StoreBinding: s.receiptBinding(),
		ReadAtUTC: time.Now().UTC(), Freshness: "historical", RegistryRelation: "unverifiable", ActionAuthorized: false}
	readJSON, err := json.Marshal(read)
	if err != nil {
		return nil, err
	}
	return &PreparedPreflightReceipt{Schema: PreparedPreflightReceiptSchema, Target: target, TargetJSON: string(raw), Read: read, ReadJSON: string(readJSON)}, nil
}

// RecoverPreparedPreflightReceipt revalidates the captured tuple under the same
// exclusive store lock before delegating to existing historical recovery. It
// never initializes custody, projects a decision or supplies missing bytes.
func (s *Store) RecoverPreparedPreflightReceipt(expected PreparedPreflightTarget) (PreflightReceiptRead, error) {
	raw, err := json.Marshal(expected)
	if err != nil {
		return PreflightReceiptRead{}, err
	}
	if _, err := DecodePreparedPreflightTarget(raw); err != nil {
		return PreflightReceiptRead{}, err
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return PreflightReceiptRead{}, err
	}
	defer s.unlock()
	actual, err := s.inspectPreparedReceiptLocked(expected.Reference.ReceiptSHA256)
	if err != nil || actual.Target != expected {
		return PreflightReceiptRead{}, errors.New("maat memory: prepared receipt/store/intent binding changed; inspect again")
	}
	return s.recoverPreflightReceiptLocked(expected.Reference.ReceiptSHA256)
}
