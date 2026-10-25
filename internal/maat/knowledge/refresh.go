package knowledge

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

// RefreshResult is a content-free summary of source coverage and migration
// outcomes. It never includes raw adapter errors or source bodies.
type RefreshResult struct {
	State      ImportState `json:"state"`
	Runs       []ImportRun `json:"runs"`
	ErrorCodes []string    `json:"error_codes,omitempty"`
}

// RefreshLocal refreshes the one explicitly configured, local reader in this
// vertical slice. Online adapters are deliberately not part of the default
// registry and are never called by this method.
func RefreshLocal(ctx context.Context, home, source string, since time.Time, claudeDir string) (RefreshResult, error) {
	if source == "" {
		source = "claude"
	}
	if source != "claude" {
		return RefreshResult{State: ImportUnavailable, Runs: []ImportRun{}, ErrorCodes: []string{"source_not_enabled_locally"}}, errors.New("requested source is not enabled for local Ma'at refresh")
	}
	archive, err := Open(home)
	if err != nil {
		return RefreshResult{State: ImportFailed, Runs: []ImportRun{}, ErrorCodes: []string{"archive_unavailable"}}, err
	}
	defer archive.Close()
	result := RefreshResult{Runs: []ImportRun{}, State: ImportComplete}
	legacyDir := filepath.Join(home, ".config", "seshat", "store")
	legacyRuns, legacyErr := archive.MigrateLegacy(ctx, legacyDir)
	result.Runs = append(result.Runs, legacyRuns...)
	if legacyErr != nil {
		result.State = ImportPartial
		result.ErrorCodes = append(result.ErrorCodes, "legacy_migration_review_required")
	}
	adapter := &seshat.ClaudeAdapter{ClaudeDir: claudeDir}
	items, sourceErr := adapter.IngestArchiveContext(ctx, since)
	run, importErr := archive.ImportSource(ctx, "claude", items, sourceErr)
	if importErr != nil {
		result.State = ImportFailed
		result.ErrorCodes = append(result.ErrorCodes, "archive_import_failed")
		return result, importErr
	}
	result.Runs = append(result.Runs, run)
	if run.State != ImportComplete && run.State != ImportNoNewItems {
		if result.State != ImportPartial {
			result.State = run.State
		}
		if run.ErrorCode != "" {
			result.ErrorCodes = append(result.ErrorCodes, run.ErrorCode)
		}
	}
	return result, nil
}
