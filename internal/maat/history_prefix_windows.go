//go:build windows

package maat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func (j *FileDecisionJournal) queryHistoryPrefixBound(query HistoryQuery) (HistoryPage, error) {
	return j.queryHistoryWindowsLegacy(query)
}

func (j *FileDecisionJournal) queryHistoryWindowsLegacy(query HistoryQuery) (HistoryPage, error) {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return HistoryPage{}, fmt.Errorf("maat decision history: empty path")
	}
	if query.Scope == "" {
		query.Scope = HistoryScopeAll
	}
	if query.Scope != HistoryScopeAll && query.Scope != HistoryScopeSystemOne {
		return HistoryPage{}, fmt.Errorf("maat decision history: invalid scope")
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > maxHistoryPageSize {
		return HistoryPage{}, fmt.Errorf("maat decision history: page size exceeds %d", maxHistoryPageSize)
	}
	if strings.TrimSpace(query.QueryKey) == "" {
		return HistoryPage{}, fmt.Errorf("maat decision history: query identity is required")
	}
	if len(query.QueryKey) > 1024 {
		return HistoryPage{}, fmt.Errorf("maat decision history: query identity exceeds its bound")
	}
	if query.ByteBudget <= 0 {
		query.ByteBudget = defaultHistoryByteBudget
	} else if query.ByteBudget > maxHistoryByteBudget {
		query.ByteBudget = maxHistoryByteBudget
	}
	if query.RowBudget <= 0 {
		query.RowBudget = defaultHistoryRowBudget
	} else if query.RowBudget > maxHistoryRowBudget {
		query.RowBudget = maxHistoryRowBudget
	}
	if query.TimeBudget <= 0 {
		query.TimeBudget = defaultHistoryTimeBudget
	} else if query.TimeBudget > maxHistoryTimeBudget {
		query.TimeBudget = maxHistoryTimeBudget
	}
	ctx := query.Context
	if ctx == nil {
		ctx = context.Background()
	}

	file, err := os.Open(j.Path)
	if err != nil {
		if os.IsNotExist(err) && query.Cursor == "" {
			return HistoryPage{Candidates: []HistoryCandidate{}, Coverage: HistoryCoverageComplete, Snapshot: "missing"}, nil
		}
		if query.Cursor != "" {
			return HistoryPage{}, ErrHistorySnapshotChanged
		}
		return HistoryPage{}, fmt.Errorf("maat decision history: open: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return HistoryPage{}, fmt.Errorf("maat decision history: journal is not a readable regular file")
	}
	snapshot := historySnapshotID(j.Path, info)
	state, err := decodeHistoryCursor(query.Cursor)
	if err != nil {
		return HistoryPage{}, err
	}
	if state == nil {
		state = &historyCursor{Version: historyCursorVersion, Snapshot: snapshot, QueryKey: query.QueryKey, Phase: "scan", Limit: query.Limit}
	} else if state.Snapshot != snapshot || state.QueryKey != query.QueryKey || state.Version != historyCursorVersion {
		return HistoryPage{}, ErrHistorySnapshotChanged
	}
	if err := validateHistoryCursor(state, query.Limit, info.Size()); err != nil {
		return HistoryPage{}, err
	}

	started := time.Now()
	deadline := started.Add(query.TimeBudget)
	page := HistoryPage{Coverage: HistoryCoveragePartial, Snapshot: snapshot, Candidates: []HistoryCandidate{}, Links: HistoryLinks{Acceptances: []Decision{}, ReviewedResolutionFor: []string{}}, AfterCursor: state.After != nil}
	for page.ScannedBytes < query.ByteBudget && page.ScannedRecords < query.RowBudget && time.Since(started) < query.TimeBudget {
		if err := ctx.Err(); err != nil {
			break
		}
		remainingBytes := query.ByteBudget - page.ScannedBytes
		remainingRows := query.RowBudget - page.ScannedRecords
		if state.Phase == "scan" {
			done, bytesRead, rowsRead, err := scanHistoryChunk(ctx, deadline, file, state, remainingBytes, remainingRows, query.Scope, query.Evidence, query.Match, query.NeedsState, query.MatchState)
			if err != nil {
				return HistoryPage{}, err
			}
			page.ScannedBytes += bytesRead
			page.ScannedRecords += rowsRead
			if !done {
				if state.Phase == "candidate-links" || state.Phase == "batch-links" {
					continue
				}
				break
			}
			state.Phase = "links"
			state.Offset = 0
			state.Links = makeHistoryLinkRefs(state.Candidates)
			if len(state.Candidates) == 0 {
				if err := verifyHistorySnapshot(file, j.Path, snapshot); err != nil {
					return HistoryPage{}, err
				}
				page.Coverage = HistoryCoverageComplete
				page.Integrity = cursorIntegrity(state)
				page.MatchCount = state.Seen
				page.EvidenceMatches = state.EvidenceSeen
				return page, nil
			}
			continue
		}
		if state.Phase == "batch-links" {
			// Reuse the exact canonical link resolver for a bounded reference batch.
			// Only the reference slice is swapped; offsets/discard state and work
			// counters advance in this same snapshot, never through another reader.
			state.Links, state.BatchLinks = state.BatchLinks, state.Links
			done, bytesRead, rowsRead, err := scanHistoryLinks(ctx, deadline, file, state, remainingBytes, remainingRows)
			state.Links, state.BatchLinks = state.BatchLinks, state.Links
			page.ScannedBytes += bytesRead
			page.ScannedRecords += rowsRead
			if err != nil {
				return HistoryPage{}, err
			}
			if !done {
				break
			}
			state.Phase = "batch-finalize"
			continue
		}
		if state.Phase == "batch-finalize" {
			pending := *state
			ref, link := state.Batch[state.BatchIndex], state.BatchLinks[state.BatchIndex]
			pending.Pending = &ref
			pending.PendingAccepted = link.Acceptance != nil
			pending.PendingReviewed = link.Review != nil
			pending.PendingAcceptance = link.Acceptance
			pending.PendingRepairOutcome = link.RepairOutcome
			pending.PendingRepairAttempt = link.RepairAttempt
			bundleBytes, bundleRows := ref.Order.Size, 1
			if link.Acceptance != nil {
				bundleBytes += link.Acceptance.Size
				bundleRows++
			}
			if repair, _ := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); repair != nil {
				bundleBytes += repair.Size
				bundleRows++
			}
			if (page.ScannedBytes > 0 || page.ScannedRecords > 0) && (bundleBytes > remainingBytes || bundleRows > remainingRows) {
				break
			}
			matched, bytesRead, rowsRead, err := finishPendingHistoryCandidate(ctx, deadline, file, &pending, query)
			page.ScannedBytes += bytesRead
			page.ScannedRecords += rowsRead
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					break
				}
				return HistoryPage{}, err
			}
			state.Candidates = pending.Candidates
			if matched {
				state.Seen++
			}
			state.BatchIndex++
			if state.BatchIndex == len(state.Batch) {
				state.Batch = nil
				state.BatchLinks = nil
				state.BatchIndex = 0
				state.Offset = state.ResumeOffset
				state.ResumeOffset = 0
				state.Phase = "scan"
			}
			continue
		}
		if state.Phase == "candidate-links" {
			done, bytesRead, rowsRead, err := scanPendingHistoryLinks(ctx, deadline, file, state, remainingBytes, remainingRows)
			if err != nil {
				return HistoryPage{}, err
			}
			page.ScannedBytes += bytesRead
			page.ScannedRecords += rowsRead
			if !done {
				break
			}
			state.Phase = "candidate-finalize"
			continue
		}
		if state.Phase == "candidate-finalize" {
			bundleBytes, bundleRows := state.Pending.Order.Size, 1
			if state.PendingAcceptance != nil {
				bundleBytes += state.PendingAcceptance.Size
				bundleRows++
			}
			if repair, _ := selectedHistoryRepair(state.PendingAcceptance, state.PendingRepairOutcome, state.PendingRepairAttempt); repair != nil {
				bundleBytes += repair.Size
				bundleRows++
			}
			if (page.ScannedBytes > 0 || page.ScannedRecords > 0) && (bundleBytes > remainingBytes || bundleRows > remainingRows) {
				break
			}
			candidate, bytesRead, rowsRead, err := finishPendingHistoryCandidate(ctx, deadline, file, state, query)
			page.ScannedBytes += bytesRead
			page.ScannedRecords += rowsRead
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					break
				}
				return HistoryPage{}, err
			}
			if candidate {
				state.Seen++
			}
			state.Offset = state.ResumeOffset
			state.ResumeOffset = 0
			state.Pending = nil
			state.PendingAccepted = false
			state.PendingReviewed = false
			state.PendingAcceptance = nil
			state.PendingRepairOutcome = nil
			state.PendingRepairAttempt = nil
			state.Phase = "scan"
			continue
		}
		if state.Phase == "links" {
			done, bytesRead, rowsRead, err := scanHistoryLinks(ctx, deadline, file, state, remainingBytes, remainingRows)
			if err != nil {
				return HistoryPage{}, err
			}
			page.ScannedBytes += bytesRead
			page.ScannedRecords += rowsRead
			if !done {
				break
			}
			state.Phase = "materialize"
			continue
		}
		if state.Phase == "materialize" {
			limited, materializedBytes, materializedRows, err := materializeHistoryPage(ctx, deadline, file, j.Path, snapshot, state, query, &page, query.ByteBudget-page.ScannedBytes, query.RowBudget-page.ScannedRecords)
			page.ScannedBytes += materializedBytes
			page.ScannedRecords += materializedRows
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errHistoryWorkBudget) {
					break
				}
				return HistoryPage{}, err
			}
			if limited && (remainingBytes < query.ByteBudget || remainingRows < query.RowBudget) {
				// Earlier phases consumed this request's budget. Keep every
				// selected reference and retry materialization with a fresh
				// request instead of publishing this residual-budget prefix.
				break
			}
			page.Coverage = HistoryCoverageComplete
			page.Integrity = cursorIntegrity(state)
			page.MatchCount = state.Seen
			page.EvidenceMatches = state.EvidenceSeen
			if limited || len(state.Candidates) > len(page.Candidates) {
				page.HasMore = true
				last := page.Candidates[len(page.Candidates)-1]
				page.NextPageCursor, err = encodeHistoryCursor(nextHistoryPageCursor(state, historyOrder{Time: last.Decision.Time, Offset: last.Offset}))
				if err != nil {
					return HistoryPage{}, err
				}
			}
			return page, nil
		}
		return HistoryPage{}, fmt.Errorf("maat decision history: invalid cursor phase")
	}
	if err := verifyHistorySnapshot(file, j.Path, snapshot); err != nil {
		return HistoryPage{}, err
	}
	page.Integrity = cursorIntegrity(state)
	page.Cursor, err = encodeHistoryCursor(state)
	if err != nil {
		return HistoryPage{}, err
	}
	page.MatchCount = state.Seen
	page.EvidenceMatches = state.EvidenceSeen
	page.Candidates = []HistoryCandidate{}
	page.Links = HistoryLinks{Acceptances: []Decision{}, ReviewedResolutionFor: []string{}}
	return page, nil
}
