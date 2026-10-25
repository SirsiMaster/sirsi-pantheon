package maat

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const diagnosticDiscoverySchema = "sirsi.maat.diagnostic-discovery.v1"
const diagnosticDiscoveryMaxRows = 128
const diagnosticDiscoveryPageBytes = 512 << 10

type DiagnosticDiscoveryQuery struct {
	Limit   int
	Cursor  string
	Context context.Context
}
type DiagnosticDiscoveryHistory struct {
	Schema      string `json:"schema"`
	QueryID     string `json:"query_id"`
	Snapshot    string `json:"snapshot"`
	Coverage    string `json:"coverage"`
	NextCursor  string `json:"next_cursor,omitempty"`
	AfterCursor bool   `json:"after_cursor"`
	Returned    int    `json:"returned"`
	CountScope  string `json:"count_scope"`
}
type DiagnosticDiscoveryPage struct {
	Schema   string                     `json:"schema"`
	Requests []DiagnosticRequestRead    `json:"requests"`
	Records  []DiagnosticRecordRead     `json:"records"`
	History  DiagnosticDiscoveryHistory `json:"history"`
}
type diagnosticDiscoveryCursor struct {
	Schema   string `json:"schema"`
	QueryID  string `json:"query_id"`
	Snapshot string `json:"snapshot"`
	Limit    int    `json:"limit"`
	After    string `json:"after"`
}

// QueryDiagnosticDiscovery bounds returned rows/bytes, not total history.
// It preserves the existing strict whole-store/WAL validation pass. Snapshot
// fingerprints reuse history's inode/ctime (not read-atime) identity and also
// cover earlier pages, so same-size in-place rewrites invalidate continuation.
func (s *Store) QueryDiagnosticDiscovery(query DiagnosticDiscoveryQuery) (DiagnosticDiscoveryPage, error) {
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > diagnosticDiscoveryMaxRows {
		return DiagnosticDiscoveryPage{}, errors.New("maat diagnostic: discovery limit must be 1..128")
	}
	ctx := query.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.lock(unix.LOCK_SH); err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	defer s.unlock()
	if !validUUID(s.storeUUID) {
		return DiagnosticDiscoveryPage{}, errors.New("maat diagnostic: discovery requires initialized custody")
	}
	var root unix.Stat_t
	if err := unix.Fstat(s.rootFD, &root); err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	identity, err := diagnosticSHA(struct {
		Schema, Store, Root string
		UID                 uint32
		Limit               int
	}{diagnosticDiscoverySchema, s.storeUUID, fmt.Sprintf("%d:%d", uint64(root.Dev), uint64(root.Ino)), uint32(os.Geteuid()), query.Limit})
	if err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	snapshot, err := s.diagnosticDiscoverySnapshot(ctx)
	if err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	after := ""
	if query.Cursor != "" {
		if len(query.Cursor) > 2048 {
			return DiagnosticDiscoveryPage{}, errors.New("maat diagnostic: oversized discovery continuation")
		}
		raw, err := base64.RawURLEncoding.DecodeString(query.Cursor)
		var cursor diagnosticDiscoveryCursor
		if err != nil || base64.RawURLEncoding.EncodeToString(raw) != query.Cursor || decodeDiagnosticCanonical(raw, &cursor) != nil || cursor.Schema != diagnosticDiscoverySchema || cursor.Limit != query.Limit || cursor.After == "" || !validDigest(cursor.QueryID) || !validDigest(cursor.Snapshot) {
			return DiagnosticDiscoveryPage{}, errors.New("maat diagnostic: invalid discovery continuation")
		}
		if cursor.QueryID != identity || cursor.Snapshot != snapshot {
			return DiagnosticDiscoveryPage{}, ErrHistorySnapshotChanged
		}
		after = cursor.After
	}
	pendingResult, _, err := s.diagnosticPendingResultLocked()
	if err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	keys := []string{}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return DiagnosticDiscoveryPage{}, err
		}
		// Preserve the prior discovery's complete diagnostic namespace checks;
		// paging is not a way to hide corrupt/unrelated custody on later pages.
		if isDiagnosticRecordLeaf(name) {
			if err := s.validateDiagnosticRecordLeafLocked(name); err != nil {
				return DiagnosticDiscoveryPage{}, err
			}
		}
		if strings.HasSuffix(name, diagnosticRequestSuffix) {
			keys = append(keys, "request:"+strings.TrimSuffix(name, diagnosticRequestSuffix))
		}
		if strings.HasSuffix(name, diagnosticIntentSuffix) {
			keys = append(keys, "record:"+strings.TrimSuffix(name, diagnosticIntentSuffix))
		}
	}
	sort.Strings(keys)
	index := 0
	if after != "" {
		index = sort.SearchStrings(keys, after)
		if index == len(keys) || keys[index] != after {
			return DiagnosticDiscoveryPage{}, errors.New("maat diagnostic: invalid discovery boundary")
		}
		index++
	}
	page := DiagnosticDiscoveryPage{Schema: DiagnosticRequestSchema, Requests: []DiagnosticRequestRead{}, Records: []DiagnosticRecordRead{}, History: DiagnosticDiscoveryHistory{Schema: diagnosticDiscoverySchema, QueryID: identity, Snapshot: snapshot, Coverage: HistoryCoverageComplete, AfterCursor: query.Cursor != "", CountScope: "page"}}
	bytesUsed := 0
	last := after
	for index < len(keys) && page.History.Returned < query.Limit {
		if err := ctx.Err(); err != nil {
			return DiagnosticDiscoveryPage{}, err
		}
		key := keys[index]
		var request *DiagnosticRequestRead
		var record *DiagnosticRecordRead
		if strings.HasPrefix(key, "request:") {
			bound, err := s.readDiagnosticRequestLocked(strings.TrimPrefix(key, "request:"))
			if err != nil {
				return DiagnosticDiscoveryPage{}, err
			}
			value, err := s.readDiagnosticRequestOutcomeAtLocked(bound, pendingResult)
			if err != nil {
				return DiagnosticDiscoveryPage{}, err
			}
			request = &value
		} else {
			id := strings.TrimPrefix(key, "record:")
			intent, err := s.readDiagnosticIntentLocked(id)
			if err != nil {
				return DiagnosticDiscoveryPage{}, err
			}
			value := DiagnosticRecordRead{Intent: intent, State: "pending"}
			if pendingResult != id {
				result, err := s.readDiagnosticResultLocked(id)
				if err == nil {
					value.Result = &result
					value.State = result.State
				} else if !errors.Is(err, os.ErrNotExist) {
					return DiagnosticDiscoveryPage{}, err
				}
			}
			record = &value
		}
		var raw []byte
		if request != nil {
			raw, err = json.Marshal(request)
		} else {
			raw, err = json.Marshal(record)
		}
		if err != nil {
			return DiagnosticDiscoveryPage{}, err
		}
		if len(raw) > diagnosticDiscoveryPageBytes {
			return DiagnosticDiscoveryPage{}, errors.New("maat diagnostic: individual discovery row exceeds its bound")
		}
		if bytesUsed+len(raw) > diagnosticDiscoveryPageBytes {
			break
		}
		if request != nil {
			page.Requests = append(page.Requests, *request)
		} else {
			page.Records = append(page.Records, *record)
		}
		bytesUsed += len(raw)
		page.History.Returned++
		last = key
		index++
	}
	if index < len(keys) {
		page.History.Coverage = HistoryCoveragePartial
		raw, err := json.Marshal(diagnosticDiscoveryCursor{diagnosticDiscoverySchema, identity, snapshot, query.Limit, last})
		if err != nil {
			return DiagnosticDiscoveryPage{}, err
		}
		page.History.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	end, err := s.diagnosticDiscoverySnapshot(ctx)
	if err != nil {
		return DiagnosticDiscoveryPage{}, err
	}
	if end != snapshot {
		return DiagnosticDiscoveryPage{}, ErrHistorySnapshotChanged
	}
	return page, nil
}

func (s *Store) diagnosticDiscoverySnapshot(ctx context.Context) (string, error) {
	h := sha256.New()
	encoder := json.NewEncoder(h)
	for _, space := range []struct {
		name string
		fd   int
	}{{"root", s.rootFD}, {"attempts", s.attemptsFD}, {"evidence", s.evidenceFD}, {"writes", s.writesFD}} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := requireOwnedDirectory(space.fd, true); err != nil {
			return "", err
		}
		fd, err := unix.Openat(space.fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return "", err
		}
		file := os.NewFile(uintptr(fd), "diagnostic-discovery-directory")
		info, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil || closeErr != nil {
			return "", errors.Join(statErr, closeErr)
		}
		label := "/" + diagnosticDiscoverySchema + "/" + s.storeUUID + "/" + space.name
		if err := encoder.Encode(historySnapshotID(label, info)); err != nil {
			return "", err
		}
		if space.name == "root" {
			continue
		}
		names, err := readDirectoryNames(space.fd)
		if err != nil {
			return "", err
		}
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			fd, err := unix.Openat(space.fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if err != nil {
				return "", err
			}
			if err := requireRegular(fd); err != nil {
				_ = unix.Close(fd)
				return "", err
			}
			file := os.NewFile(uintptr(fd), "diagnostic-discovery-object")
			info, statErr := file.Stat()
			closeErr := file.Close()
			if statErr != nil || closeErr != nil {
				return "", errors.Join(statErr, closeErr)
			}
			if err := encoder.Encode(historySnapshotID(label+"/"+name, info)); err != nil {
				return "", err
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
