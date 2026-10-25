package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

const maxLegacyFileBytes = 64 << 20

// MigrateLegacy imports every retained Seshat ingestion snapshot plus latest.json
// through the same sensitivity and version admission as current source reads.
// It never modifies or deletes a legacy file. Repeated calls are digest-idempotent.
func (a *Archive) MigrateLegacy(ctx context.Context, legacyDir string) ([]ImportRun, error) {
	if err := a.verifyLocation(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(legacyDir) == "" {
		return nil, errors.New("legacy knowledge path is empty")
	}
	dirInfo, err := os.Lstat(legacyDir)
	if errors.Is(err, os.ErrNotExist) {
		return []ImportRun{}, nil
	}
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("legacy knowledge directory is not safe")
	}
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return nil, errors.New("legacy knowledge directory could not be read")
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if name == "latest.json" || (strings.HasPrefix(name, "ingestion_") && strings.HasSuffix(name, ".json")) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	runs := make([]ImportRun, 0, len(names))
	var partial bool
	recordFailure := func(code string, state ImportState) error {
		run, err := a.recordLegacyFailure(ctx, code, state)
		if err != nil {
			return err
		}
		runs = append(runs, run)
		partial = true
		return nil
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return runs, err
		}
		path := filepath.Join(legacyDir, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxLegacyFileBytes {
			if err := recordFailure("legacy_file_unverifiable", ImportUnverifiable); err != nil {
				return runs, err
			}
			continue
		}
		data, readErr := readLegacyArchiveFile(path, maxLegacyFileBytes)
		if readErr != nil {
			if err := recordFailure("legacy_file_unverifiable", ImportUnverifiable); err != nil {
				return runs, err
			}
			continue
		}
		fileDigest := sha256.Sum256(data)
		pathHash := sha256.Sum256([]byte(name))
		pathKey := hex.EncodeToString(pathHash[:])
		digest := hex.EncodeToString(fileDigest[:])
		var importedDigest string
		markerErr := a.db.QueryRowContext(ctx, `SELECT file_digest FROM legacy_imports WHERE path_key=?`, pathKey).Scan(&importedDigest)
		if markerErr == nil && importedDigest == digest {
			continue
		}
		if markerErr != nil && !errors.Is(markerErr, sql.ErrNoRows) {
			return runs, errors.New("legacy migration state is unavailable")
		}
		var items []seshat.KnowledgeItem
		if err := json.Unmarshal(data, &items); err != nil {
			if err := recordFailure("legacy_json_unverifiable", ImportUnverifiable); err != nil {
				return runs, err
			}
			continue
		}
		records := make([]seshat.SourceRecord, 0, len(items))
		for index, item := range items {
			source := legacySource(item.References)
			if source == "" {
				source = "legacy-seshat"
			}
			nativeID, stableNativeID := legacyNativeID(item, index)
			instanceID := "legacy-file:" + pathKey[:24]
			if stableNativeID {
				instanceID = "legacy-seshat:" + source
			}
			content := item.Content
			if content == "" {
				content = item.Summary
			}
			references := append([]seshat.KIReference(nil), item.References...)
			references = append(references, seshat.KIReference{Type: "legacy_snapshot_id", Value: pathKey})
			records = append(records, seshat.SourceRecord{
				Source: source, InstanceID: instanceID, NativeItemID: nativeID,
				Schema: "legacy-seshat-v1", ExtractedAt: info.ModTime().UTC(),
				Title: item.Title, Summary: item.Summary, Content: content,
				References: references,
			})
		}
		groups := groupLegacyRecords(records)
		fileRuns := []ImportRun{}
		fileComplete := true
		for _, source := range sortedLegacySources(groups) {
			group := groups[source]
			for offset := 0; offset < len(group); offset += 10_000 {
				end := offset + 10_000
				if end > len(group) {
					end = len(group)
				}
				run, importErr := a.ImportSource(ctx, source, group[offset:end], nil)
				if importErr != nil {
					partial = true
					fileComplete = false
					continue
				}
				fileRuns = append(fileRuns, run)
				if run.State != ImportComplete && run.State != ImportNoNewItems {
					partial = true
					fileComplete = false
				}
			}
		}
		if !fileComplete || (len(fileRuns) == 0 && len(records) > 0) {
			partial = true
			continue
		}
		state := ImportComplete
		if len(fileRuns) == 0 && len(records) == 0 {
			state = ImportNoNewItems
		}
		if err := a.verifyLocation(); err != nil {
			return runs, err
		}
		if _, err := a.db.ExecContext(ctx, `INSERT INTO legacy_imports(path_key,file_digest,imported_at,state) VALUES(?,?,?,?) ON CONFLICT(path_key) DO UPDATE SET file_digest=excluded.file_digest,imported_at=excluded.imported_at,state=excluded.state`, pathKey, digest, time.Now().UTC().Format(time.RFC3339Nano), state); err != nil {
			return runs, errors.New("legacy migration receipt could not be committed")
		}
		runs = append(runs, fileRuns...)
	}
	if partial {
		return runs, errors.New("one or more legacy knowledge snapshots require review")
	}
	return runs, nil
}

func (a *Archive) recordLegacyFailure(ctx context.Context, code string, state ImportState) (ImportRun, error) {
	started := time.Now().UTC()
	finished := time.Now().UTC()
	res, err := a.db.ExecContext(ctx, `INSERT INTO import_runs(source,state,started_at,finished_at,seen,added,versioned,unchanged,withheld,rejected,error_code) VALUES('legacy-seshat',?,?,?,?,0,0,0,0,1,?)`, state, started.Format(time.RFC3339Nano), finished.Format(time.RFC3339Nano), 0, code)
	if err != nil {
		return ImportRun{}, errors.New("legacy migration coverage could not be recorded")
	}
	id, _ := res.LastInsertId()
	return ImportRun{ID: id, Source: "legacy-seshat", State: state, StartedAt: started, FinishedAt: finished, Rejected: 1, ErrorCode: code}, nil
}

func legacySource(references []seshat.KIReference) string {
	for _, reference := range references {
		if reference.Type == "source" && reference.Value != "" {
			return reference.Value
		}
	}
	return ""
}

func legacyNativeID(item seshat.KnowledgeItem, index int) (string, bool) {
	if item.ID != "" {
		return item.ID, true
	}
	for _, reference := range item.References {
		if reference.Type == "conversation_id" && reference.Value != "" {
			return reference.Value, true
		}
	}
	return fmt.Sprintf("legacy-item-%06d", index), false
}

func groupLegacyRecords(records []seshat.SourceRecord) map[string][]seshat.SourceRecord {
	groups := make(map[string][]seshat.SourceRecord)
	for _, record := range records {
		groups[record.Source] = append(groups[record.Source], record)
	}
	return groups
}

func sortedLegacySources(groups map[string][]seshat.SourceRecord) []string {
	sources := make([]string, 0, len(groups))
	for source := range groups {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}
