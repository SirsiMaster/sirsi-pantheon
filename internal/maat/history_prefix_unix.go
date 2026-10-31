//go:build !windows

package maat

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func (j *FileDecisionJournal) queryHistoryPrefixBound(query HistoryQuery) (HistoryPage, error) {
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
	started := time.Now()
	overallDeadline := started.Add(maxHistoryTimeBudget)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(overallDeadline) {
		overallDeadline = callerDeadline
	}
	file, size, err := openHistoryPrefix(j.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && query.Cursor == "" {
			return HistoryPage{Candidates: []HistoryCandidate{}, Coverage: HistoryCoverageComplete, Snapshot: "missing"}, nil
		}
		if query.Cursor != "" {
			return HistoryPage{}, ErrHistorySnapshotChanged
		}
		return HistoryPage{}, err
	}
	defer file.close()
	state, err := decodeHistoryCursor(query.Cursor)
	if err != nil {
		return HistoryPage{}, err
	}
	snapshot := file.snapshotID()
	if state == nil {
		state = &historyCursor{Version: historyCursorVersion, Snapshot: snapshot, PrefixBytes: size, QueryKey: query.QueryKey, Phase: "anchor", Limit: query.Limit}
	} else if state.QueryKey != query.QueryKey || state.Version != historyCursorVersion || state.PrefixBytes < 0 || state.PrefixBytes > size || !validHistoryDigest(state.PrefixSHA) || (state.Phase == "anchor" && state.Snapshot != snapshot) || (state.Phase != "anchor" && state.Snapshot != file.prefixSnapshotID(state.PrefixBytes, state.PrefixSHA)) {
		return HistoryPage{}, ErrHistorySnapshotChanged
	}
	if state.Phase != "anchor" {
		snapshot = state.Snapshot
	}
	if err := validateHistoryCursor(state, query.Limit, state.PrefixBytes); err != nil {
		return HistoryPage{}, err
	}
	working, err := cloneHistoryCursor(state)
	if err != nil {
		return HistoryPage{}, err
	}
	initialCursor, err := encodeHistoryCursor(working)
	if err != nil {
		return HistoryPage{}, err
	}
	page := HistoryPage{Coverage: HistoryCoveragePartial, Snapshot: snapshot, Candidates: []HistoryCandidate{}, Links: HistoryLinks{Acceptances: []Decision{}, ReviewedResolutionFor: []string{}}, AfterCursor: working.After != nil}
	workClock := &historyWorkClock{started: time.Now(), budget: query.TimeBudget}
	workCtx := context.WithValue(ctx, historyWorkClockKey{}, workClock)
	workDeadline := overallDeadline
	remainingBytes, remainingRows := query.ByteBudget, query.RowBudget
	deadlineExpired := false
	for {
		if err := ctx.Err(); err != nil {
			return HistoryPage{}, err
		}
		if !time.Now().Before(overallDeadline) {
			if working.Phase == "anchor" {
				return HistoryPage{}, context.DeadlineExceeded
			}
			deadlineExpired = true
			break
		}
		if working.Phase != "anchor" && (workClock.expired() || remainingBytes <= 0 || remainingRows <= 0) {
			page.Cursor, err = encodeHistoryCursor(working)
			if err != nil {
				return HistoryPage{}, err
			}
			page.Integrity, page.MatchCount, page.EvidenceMatches = cursorIntegrity(working), working.Seen, working.EvidenceSeen
			return page, nil
		}
		tentative, cloneErr := cloneHistoryCursor(working)
		if cloneErr != nil {
			return HistoryPage{}, cloneErr
		}
		phase := tentative.Phase
		phaseQuery := query
		phaseQuery.ByteBudget, phaseQuery.RowBudget = remainingBytes, remainingRows
		var phasePage HistoryPage
		var passBytes int64
		var passRows int
		var passDone bool
		stopAfterPass := false
		if phase == "anchor" {
			workClock.pause()
			digest, passErr := historyPrefixPass(file, tentative.PrefixBytes, "", ctx, overallDeadline, nil)
			workClock.resume()
			if passErr != nil {
				return HistoryPage{}, passErr
			}
			tentative.PrefixSHA = digest
			tentative.Snapshot = file.prefixSnapshotID(tentative.PrefixBytes, digest)
			tentative.Phase = "scan"
			working, snapshot = tentative, tentative.Snapshot
			page.Snapshot = snapshot
			page.VerificationBytes += tentative.PrefixBytes
			continue
		}
		if phase == "scan" || phase == "links" || phase == "candidate-links" || phase == "batch-links" {
			expected := tentative.PrefixSHA
			digest, passErr := historyPrefixPass(file, tentative.PrefixBytes, expected, workCtx, overallDeadline, func(reader *bufio.Reader) error {
				if tentative.Offset > 0 {
					workClock.pause()
					_, skipErr := io.CopyN(io.Discard, reader, tentative.Offset)
					workClock.resume()
					if skipErr != nil {
						return ErrHistorySnapshotChanged
					}
				}
				switch phase {
				case "scan":
					passDone, passBytes, passRows, err = scanHistoryChunkReader(workCtx, workDeadline, reader, tentative, remainingBytes, remainingRows, query.Scope, query.Evidence, query.Match, query.NeedsState, query.MatchState)
				case "links":
					passDone, passBytes, passRows, err = scanHistoryLinksReader(workCtx, workDeadline, reader, tentative, remainingBytes, remainingRows)
				case "candidate-links":
					passDone, passBytes, passRows, err = scanPendingHistoryLinksReader(workCtx, workDeadline, reader, tentative, remainingBytes, remainingRows)
				case "batch-links":
					tentative.Links, tentative.BatchLinks = tentative.BatchLinks, tentative.Links
					passDone, passBytes, passRows, err = scanHistoryLinksReader(workCtx, workDeadline, reader, tentative, remainingBytes, remainingRows)
					tentative.Links, tentative.BatchLinks = tentative.BatchLinks, tentative.Links
				}
				return err
			})
			if passErr != nil {
				if errors.Is(passErr, context.DeadlineExceeded) && ctx.Err() == nil {
					deadlineExpired = true
					break
				}
				return HistoryPage{}, passErr
			}
			if digest != tentative.PrefixSHA {
				return HistoryPage{}, ErrHistorySnapshotChanged
			}
			if passDone {
				switch phase {
				case "scan":
					if len(tentative.Candidates) == 0 {
						page.VerificationBytes += tentative.PrefixBytes
						page.ScannedBytes += passBytes
						page.ScannedRecords += passRows
						page.Coverage, page.Integrity = HistoryCoverageComplete, cursorIntegrity(tentative)
						page.MatchCount, page.EvidenceMatches = tentative.Seen, tentative.EvidenceSeen
						return page, nil
					}
					tentative.Links, tentative.Offset, tentative.Phase = makeHistoryLinkRefs(tentative.Candidates), 0, "links"
				case "links":
					tentative.Phase = "materialize"
				case "candidate-links":
					tentative.Phase = "candidate-finalize"
				case "batch-links":
					tentative.Phase = "batch-finalize"
				}
			}
			page.VerificationBytes += tentative.PrefixBytes
		} else {
			if (phase == "candidate-finalize" || phase == "batch-finalize" || phase == "materialize") && (page.ScannedBytes > 0 || page.ScannedRecords > 0) {
				var first historyCandidateRef
				var link historyLinkRef
				available := true
				switch phase {
				case "candidate-finalize":
					if tentative.Pending == nil {
						return HistoryPage{}, fmt.Errorf("maat decision history: invalid pending candidate")
					}
					first, link = *tentative.Pending, historyLinkRef{Acceptance: tentative.PendingAcceptance, RepairOutcome: tentative.PendingRepairOutcome, RepairAttempt: tentative.PendingRepairAttempt, SystemOneSourceCount: tentative.PendingSystemOneSourceCount}
				case "batch-finalize":
					if tentative.BatchIndex >= len(tentative.Batch) {
						return HistoryPage{}, fmt.Errorf("maat decision history: invalid batch cursor")
					}
					first, link = tentative.Batch[tentative.BatchIndex], tentative.BatchLinks[tentative.BatchIndex]
				case "materialize":
					if len(tentative.Candidates) == 0 {
						available = false
					} else {
						first, link = tentative.Candidates[0], tentative.Links[0]
					}
				}
				if available {
					bundleBytes, bundleRows := first.Order.Size, 1
					if link.Acceptance != nil {
						bundleBytes += link.Acceptance.Size
						bundleRows++
					}
					if phase == "materialize" && link.Review != nil {
						bundleBytes += link.Review.Size
						bundleRows++
					}
					if repair, _ := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); repair != nil {
						bundleBytes += repair.Size
						bundleRows++
					}
					if bundleBytes > remainingBytes || bundleRows > remainingRows {
						break
					}
				}
			}
			phasePage = HistoryPage{Coverage: HistoryCoveragePartial, Snapshot: snapshot, Candidates: []HistoryCandidate{}, Links: HistoryLinks{Acceptances: []Decision{}, ReviewedResolutionFor: []string{}}, AfterCursor: tentative.After != nil}
			capturedPage, captureErr := queryHistoryCapturedPhase(file, tentative, phaseQuery, phasePage, workCtx, workDeadline, overallDeadline, page.ScannedBytes > 0 || page.ScannedRecords > 0)
			if captureErr != nil {
				if errors.Is(captureErr, errHistoryWorkBudget) {
					break
				}
				if errors.Is(captureErr, context.DeadlineExceeded) && ctx.Err() == nil {
					deadlineExpired = true
					break
				}
				return HistoryPage{}, captureErr
			}
			phasePage = capturedPage
			passBytes, passRows = phasePage.ScannedBytes, phasePage.ScannedRecords
			page.VerificationBytes += phasePage.VerificationBytes
			if phase == "materialize" && phasePage.Coverage == HistoryCoverageComplete {
				phasePage.VerificationBytes = page.VerificationBytes
				phasePage.ScannedBytes += page.ScannedBytes
				phasePage.ScannedRecords += page.ScannedRecords
				return phasePage, nil
			}
			stopAfterPass = phase == "materialize" && phasePage.Coverage == HistoryCoveragePartial
			if phasePage.Cursor == "" {
				return HistoryPage{}, fmt.Errorf("maat decision history: phase produced no verified continuation")
			}
		}
		page.ScannedBytes += passBytes
		page.ScannedRecords += passRows
		remainingBytes -= passBytes
		remainingRows -= passRows
		working = tentative
		if stopAfterPass {
			break
		}
		// A verified scan can end by switching into a qualification batch before
		// reaching EOF. Continue that new phase in this request while its existing
		// time and work budgets remain; only an unfinished phase should yield.
		if !passDone && tentative.Phase == phase && (phase == "anchor" || phase == "scan" || phase == "links" || phase == "candidate-links" || phase == "batch-links") {
			break
		}
		if workClock.expired() || remainingBytes <= 0 || remainingRows <= 0 {
			break
		}
	}
	if deadlineExpired {
		page.Cursor, err = historyCursorAfterDeadline(working, initialCursor)
	} else {
		page.Cursor, err = encodeHistoryCursor(working)
	}
	if err != nil {
		return HistoryPage{}, err
	}
	page.Integrity, page.MatchCount, page.EvidenceMatches = cursorIntegrity(working), working.Seen, working.EvidenceSeen
	return page, nil
}

func historyCursorAfterDeadline(state *historyCursor, previous string) (string, error) {
	cursor, err := encodeHistoryCursor(state)
	if err != nil {
		return "", err
	}
	if cursor == previous {
		return "", context.DeadlineExceeded
	}
	return cursor, nil
}

func cloneHistoryCursor(state *historyCursor) (*historyCursor, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	var clone historyCursor
	if err := json.Unmarshal(raw, &clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func historyPartialPageWithCounters(page HistoryPage, state *historyCursor) (HistoryPage, error) {
	page.Integrity = cursorIntegrity(state)
	page.MatchCount = state.Seen
	page.EvidenceMatches = state.EvidenceSeen
	cursor, err := encodeHistoryCursor(state)
	if err != nil {
		return HistoryPage{}, err
	}
	page.Cursor = cursor
	page.Candidates = []HistoryCandidate{}
	page.Links = HistoryLinks{Acceptances: []Decision{}, ReviewedResolutionFor: []string{}}
	return page, nil
}

func queryHistoryCapturedPhase(file *historyPrefixFile, state *historyCursor, query HistoryQuery, page HistoryPage, ctx context.Context, workDeadline, overallDeadline time.Time, priorWork bool) (HistoryPage, error) {
	var refs []*historyOrder
	var selectedCount int
	var selectedBytes int64
	var selectedRows int
	limited := false
	switch state.Phase {
	case "candidate-finalize":
		refs = append(refs, &state.Pending.Order, state.PendingAcceptance)
		if repair, _ := selectedHistoryRepair(state.PendingAcceptance, state.PendingRepairOutcome, state.PendingRepairAttempt); repair != nil {
			refs = append(refs, repair)
		}
		selectedCount = 1
	case "batch-finalize":
		for index := state.BatchIndex; index < len(state.Batch); index++ {
			ref, link := state.Batch[index], state.BatchLinks[index]
			bundleBytes, bundleRows := ref.Order.Size, 1
			bundle := []*historyOrder{&ref.Order, link.Acceptance}
			if link.Acceptance != nil {
				bundleBytes += link.Acceptance.Size
				bundleRows++
			}
			if repair, _ := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); repair != nil {
				bundle = append(bundle, repair)
				bundleBytes += repair.Size
				bundleRows++
			}
			if selectedCount > 0 && (selectedBytes+bundleBytes > query.ByteBudget || selectedRows+bundleRows > query.RowBudget) {
				limited = true
				break
			}
			refs = append(refs, bundle...)
			selectedCount++
			selectedBytes += bundleBytes
			selectedRows += bundleRows
		}
	case "materialize":
		count := len(state.Candidates)
		if count > query.Limit {
			count = query.Limit
		}
		for index := 0; index < count; index++ {
			ref, link := state.Candidates[index], state.Links[index]
			bundleBytes, bundleRows := ref.Order.Size, 1
			bundle := []*historyOrder{&ref.Order, link.Acceptance, link.Review}
			if link.Acceptance != nil {
				bundleBytes += link.Acceptance.Size
				bundleRows++
			}
			if link.Review != nil {
				bundleBytes += link.Review.Size
				bundleRows++
			}
			if repair, _ := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); repair != nil {
				bundle = append(bundle, repair)
				bundleBytes += repair.Size
				bundleRows++
			}
			if selectedCount > 0 && (selectedBytes+bundleBytes > query.ByteBudget || selectedRows+bundleRows > query.RowBudget) {
				limited = true
				break
			}
			refs = append(refs, bundle...)
			selectedCount++
			selectedBytes += bundleBytes
			selectedRows += bundleRows
		}
	default:
		return HistoryPage{}, fmt.Errorf("maat decision history: invalid captured phase")
	}
	if selectedCount == 0 && state.Phase != "materialize" {
		return HistoryPage{}, fmt.Errorf("maat decision history: empty finalization phase")
	}
	var workBytes int64
	var workRows int
	var capture map[int64]Decision
	var captureErr error
	passDigest, err := historyPrefixPass(file, state.PrefixBytes, state.PrefixSHA, ctx, overallDeadline, func(reader *bufio.Reader) error {
		capture, workBytes, workRows, captureErr = captureHistoryDecisions(ctx, workDeadline, reader, refs)
		if captureErr != nil {
			return captureErr
		}
		switch state.Phase {
		case "candidate-finalize":
			matched, err := finishCapturedHistoryCandidate(state.Pending, state.PendingAcceptance, state.PendingRepairOutcome, state.PendingRepairAttempt, state.PendingReviewed, state.PendingSystemOneSourceCount, capture, query)
			if err != nil {
				return err
			}
			if matched {
				insertHistoryCandidate(state, *state.Pending, state.Limit+1)
				state.Seen++
			}
			state.Offset = state.ResumeOffset
			clearPendingHistoryCursor(state)
			state.Phase = "scan"
		case "batch-finalize":
			batchEnd := state.BatchIndex + selectedCount
			if batchEnd > len(state.Batch) {
				batchEnd = len(state.Batch)
			}
			for state.BatchIndex < batchEnd {
				index := state.BatchIndex
				ref, link := state.Batch[index], state.BatchLinks[index]
				matched, err := finishCapturedHistoryCandidate(&ref, link.Acceptance, link.RepairOutcome, link.RepairAttempt, link.Review != nil, link.SystemOneSourceCount, capture, query)
				if err != nil {
					return err
				}
				if matched {
					insertHistoryCandidate(state, ref, state.Limit+1)
					state.Seen++
				}
				state.BatchIndex++
			}
			if state.BatchIndex == len(state.Batch) {
				state.Batch, state.BatchLinks, state.BatchIndex = nil, nil, 0
				state.Offset, state.ResumeOffset, state.Phase = state.ResumeOffset, 0, "scan"
			}
		case "materialize":
			if err := materializeCapturedHistory(state, query, &page, capture, selectedCount, limited); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return HistoryPage{}, err
	}
	if passDigest != state.PrefixSHA {
		return HistoryPage{}, ErrHistorySnapshotChanged
	}
	page.VerificationBytes = state.PrefixBytes
	page.ScannedBytes, page.ScannedRecords = workBytes, workRows
	if state.Phase == "materialize" {
		if limited && priorWork {
			return historyPartialPageWithCounters(page, state)
		}
		page.Coverage, page.Integrity = HistoryCoverageComplete, cursorIntegrity(state)
		page.MatchCount, page.EvidenceMatches = state.Seen, state.EvidenceSeen
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
	return historyPartialPageWithCounters(page, state)
}

func clearPendingHistoryCursor(state *historyCursor) {
	state.ResumeOffset = 0
	state.Pending = nil
	state.PendingAccepted = false
	state.PendingReviewed = false
	state.PendingAcceptance = nil
	state.PendingRepairOutcome = nil
	state.PendingRepairAttempt = nil
	state.PendingSystemOneSourceCount = 0
}

func finishCapturedHistoryCandidate(ref *historyCandidateRef, acceptance, repairOutcome, repairAttempt *historyOrder, hidden bool, sourceCount int, captured map[int64]Decision, query HistoryQuery) (bool, error) {
	decision, ok := captured[ref.Order.Offset]
	if !ok || decision.Time != ref.Order.Time || historyKeyHash(decision.Evidence) != ref.EvidenceHash || historyKeyHash(decision.ResolutionFor) != ref.ResolutionHash || (decision.Kind == "diagnostic owner acceptance") != ref.OwnerAcceptance {
		return false, ErrHistorySnapshotChanged
	}
	state := HistoryMatchState{Resolved: acceptance != nil, Hidden: hidden, SystemOneSourceCount: sourceCount}
	if acceptance != nil {
		resolution, ok := captured[acceptance.Offset]
		if !ok || resolution.Kind != "diagnostic owner acceptance" || historyKeyHash(resolution.ResolutionFor) != ref.EvidenceHash || resolution.Time != acceptance.Time {
			return false, ErrHistorySnapshotChanged
		}
		state.Resolution = &resolution
	}
	if repair, kind := selectedHistoryRepair(acceptance, repairOutcome, repairAttempt); repair != nil {
		row, ok := captured[repair.Offset]
		if !ok || row.Kind != kind || row.Resource != "binary-drift" || row.OriginEvidence == "" || historyKeyHash(row.OriginEvidence) != ref.EvidenceHash || row.Time != repair.Time {
			return false, ErrHistorySnapshotChanged
		}
		if kind == "diagnostic verified repair" {
			state.RepairOutcome = &row
		} else {
			state.RepairAttempt = &row
		}
	}
	if query.Scope == HistoryScopeSystemOne && decision.SystemOne == nil && decision.SystemOneCalibration == nil || query.Evidence != "" && decision.Evidence != query.Evidence || query.Match != nil && !query.Match(decision) {
		return false, ErrHistorySnapshotChanged
	}
	if query.MatchState != nil && !query.MatchState(decision, state) {
		return false, nil
	}
	return true, nil
}

func materializeCapturedHistory(state *historyCursor, query HistoryQuery, page *HistoryPage, captured map[int64]Decision, count int, limited bool) error {
	page.Candidates = make([]HistoryCandidate, 0, count)
	acceptances := make(map[int64]Decision)
	for index := 0; index < count; index++ {
		ref, link := state.Candidates[index], state.Links[index]
		decision, ok := captured[ref.Order.Offset]
		if !ok || decision.Time != ref.Order.Time || historyKeyHash(decision.Evidence) != ref.EvidenceHash || historyKeyHash(decision.ResolutionFor) != ref.ResolutionHash || (decision.Kind == "diagnostic owner acceptance") != ref.OwnerAcceptance || link.EvidenceHash != ref.EvidenceHash || link.ResolutionHash != ref.ResolutionHash || link.OwnerAcceptance != ref.OwnerAcceptance {
			return ErrHistorySnapshotChanged
		}
		if query.Scope == HistoryScopeSystemOne && decision.SystemOne == nil && decision.SystemOneCalibration == nil || query.Evidence != "" && decision.Evidence != query.Evidence || query.Match != nil && !query.Match(decision) || state.After != nil && !historyOrderAfter(historyOrder{Time: decision.Time, Offset: ref.Order.Offset}, *state.After) {
			return ErrHistorySnapshotChanged
		}
		matchState := HistoryMatchState{}
		if ref.OwnerAcceptance && link.Review != nil && decision.ResolutionFor != "" {
			review, found := captured[link.Review.Offset]
			if !found || review.Kind != "diagnostic owner review" || historyKeyHash(review.Evidence) != ref.ResolutionHash || review.Time != link.Review.Time {
				return ErrHistorySnapshotChanged
			}
			page.Links.ReviewedResolutionFor = append(page.Links.ReviewedResolutionFor, decision.ResolutionFor)
			matchState.Hidden = true
		}
		if link.Acceptance != nil {
			acceptance, found := captured[link.Acceptance.Offset]
			if !found || acceptance.Kind != "diagnostic owner acceptance" || historyKeyHash(acceptance.ResolutionFor) != ref.EvidenceHash || acceptance.Time != link.Acceptance.Time {
				return ErrHistorySnapshotChanged
			}
			acceptances[link.Acceptance.Offset] = acceptance
			matchState.Resolved, matchState.Resolution = true, &acceptance
		}
		if repairRef, kind := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); repairRef != nil {
			repair, found := captured[repairRef.Offset]
			if !found || repair.Kind != kind || repair.Resource != "binary-drift" || repair.OriginEvidence == "" || historyKeyHash(repair.OriginEvidence) != link.EvidenceHash || repair.Time != repairRef.Time {
				return ErrHistorySnapshotChanged
			}
			if kind == "diagnostic verified repair" {
				matchState.RepairOutcome = &repair
				page.Links.RepairOutcomes = append(page.Links.RepairOutcomes, repair)
			} else {
				matchState.RepairAttempt = &repair
				page.Links.RepairAttempts = append(page.Links.RepairAttempts, repair)
			}
		}
		if query.MatchState != nil && !query.MatchState(decision, matchState) {
			return ErrHistorySnapshotChanged
		}
		page.Candidates = append(page.Candidates, HistoryCandidate{Decision: decision, Offset: ref.Order.Offset, SystemOneSourceCount: link.SystemOneSourceCount})
	}
	offsets := make([]int64, 0, len(acceptances))
	for offset := range acceptances {
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	for _, offset := range offsets {
		page.Links.Acceptances = append(page.Links.Acceptances, acceptances[offset])
	}
	_ = limited
	return nil
}

// historyPrefixFile retains the resolved parent and leaf descriptors for one
// bounded history request. Size and timestamps are deliberately excluded from
// object authority so an append after the frozen boundary remains admissible.
type historyPrefixFile struct {
	file       *os.File
	parentFD   int
	original   string
	parentPath string
	base       string
	parent     historyPrefixIdentity
	leaf       historyPrefixIdentity
}

func (f *historyPrefixFile) snapshotID() string {
	return digestBytes([]byte(fmt.Sprintf("%s\x00%+v\x00%+v", filepath.Clean(f.original), f.parent, f.leaf)))
}

func (f *historyPrefixFile) prefixSnapshotID(size int64, digest string) string {
	return digestBytes([]byte(fmt.Sprintf("%s\x00%d\x00%s", f.snapshotID(), size, digest)))
}

type historyPrefixIdentity struct {
	dev, ino uint64
	uid, gid uint32
	mode     uint32
	nlink    uint64
}

func historyPrefixStat(stat *unix.Stat_t) historyPrefixIdentity {
	return historyPrefixIdentity{dev: uint64(stat.Dev), ino: stat.Ino, uid: stat.Uid, gid: stat.Gid, mode: uint32(stat.Mode), nlink: uint64(stat.Nlink)}
}

func historyPrefixDirectoryStat(stat *unix.Stat_t) historyPrefixIdentity {
	identity := historyPrefixStat(stat)
	identity.nlink = 0
	return identity
}

func openHistoryPrefix(path string) (*historyPrefixFile, int64, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, err
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return nil, 0, fmt.Errorf("maat decision history: resolve parent: %w", err)
	}
	parentFD, err := openAbsoluteDirectory(resolvedParent)
	if err != nil {
		return nil, 0, fmt.Errorf("maat decision history: open retained parent: %w", err)
	}
	closeParent := true
	defer func() {
		if closeParent {
			_ = unix.Close(parentFD)
		}
	}()
	base := filepath.Base(absolute)
	fd, err := unix.Openat(parentFD, base, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("maat decision history: open journal without following links: %w", err)
	}
	var parentStat, leafStat unix.Stat_t
	if err := unix.Fstat(parentFD, &parentStat); err != nil {
		_ = unix.Close(fd)
		return nil, 0, err
	}
	if err := unix.Fstat(fd, &leafStat); err != nil {
		_ = unix.Close(fd)
		return nil, 0, err
	}
	leaf := historyPrefixStat(&leafStat)
	if leaf.mode&unix.S_IFMT != unix.S_IFREG || leaf.nlink != 1 || leaf.uid != uint32(os.Geteuid()) {
		_ = unix.Close(fd)
		return nil, 0, fmt.Errorf("maat decision history: journal is not an owned singly-linked regular file")
	}
	file := os.NewFile(uintptr(fd), base)
	closeParent = false
	return &historyPrefixFile{file: file, parentFD: parentFD, original: absolute, parentPath: resolvedParent, base: base, parent: historyPrefixDirectoryStat(&parentStat), leaf: leaf}, leafStat.Size, nil
}

func (f *historyPrefixFile) close() {
	if f == nil {
		return
	}
	if f.file != nil {
		_ = f.file.Close()
	}
	if f.parentFD >= 0 {
		_ = unix.Close(f.parentFD)
		f.parentFD = -1
	}
}

func (f *historyPrefixFile) verify(size int64) error {
	if f == nil || f.file == nil || f.parentFD < 0 {
		return ErrHistorySnapshotChanged
	}
	var parentStat, leafStat, namedStat unix.Stat_t
	if err := unix.Fstat(f.parentFD, &parentStat); err != nil || historyPrefixDirectoryStat(&parentStat) != f.parent {
		return ErrHistorySnapshotChanged
	}
	if err := unix.Fstat(int(f.file.Fd()), &leafStat); err != nil {
		return ErrHistorySnapshotChanged
	}
	if err := unix.Fstatat(f.parentFD, f.base, &namedStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return ErrHistorySnapshotChanged
	}
	leaf := historyPrefixStat(&leafStat)
	if leaf != f.leaf || historyPrefixStat(&namedStat) != f.leaf || leafStat.Size < size || size < 0 {
		return ErrHistorySnapshotChanged
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(f.original))
	if err != nil || resolvedParent != f.parentPath {
		return ErrHistorySnapshotChanged
	}
	currentParentFD, err := openAbsoluteDirectory(resolvedParent)
	if err != nil {
		return ErrHistorySnapshotChanged
	}
	defer unix.Close(currentParentFD)
	var currentParentStat unix.Stat_t
	if err := unix.Fstat(currentParentFD, &currentParentStat); err != nil || historyPrefixDirectoryStat(&currentParentStat) != f.parent {
		return ErrHistorySnapshotChanged
	}
	return nil
}

type historyDeadlineReader struct {
	ctx      context.Context
	deadline time.Time
	reader   io.Reader
	clock    *historyWorkClock
}

type historyHashingReader struct {
	reader io.Reader
	hash   hash.Hash
	clock  *historyWorkClock
}

func (r historyHashingReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 {
		if r.clock != nil {
			r.clock.pause()
		}
		_, _ = r.hash.Write(buffer[:n])
		if r.clock != nil {
			r.clock.resume()
		}
	}
	return n, err
}

func (r historyDeadlineReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if !time.Now().Before(r.deadline) {
		return 0, context.DeadlineExceeded
	}
	started := time.Now()
	n, err := r.reader.Read(buffer)
	if r.clock != nil && r.clock.pausedAt.IsZero() {
		r.clock.excluded += time.Since(started)
	}
	return n, err
}

// historyPrefixPass hashes exactly the anchored prefix while exposing those
// same bytes to one bounded parser callback. Any unconsumed tail is drained
// through the same hash stream before the retained identities are rechecked.
func historyPrefixPass(file *historyPrefixFile, size int64, expected string, ctx context.Context, deadline time.Time, consume func(*bufio.Reader) error) (string, error) {
	if file == nil || file.file == nil || size < 0 {
		return "", ErrHistorySnapshotChanged
	}
	if err := file.verify(size); err != nil {
		return "", err
	}
	if _, err := file.file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("maat decision history: rewind retained descriptor: %w", err)
	}
	digest, err := historyPrefixPassReader(file.file, size, ctx, deadline, consume)
	if err != nil {
		return "", err
	}
	if err := file.verify(size); err != nil {
		return "", err
	}
	if expected != "" && digest != expected {
		return "", ErrHistorySnapshotChanged
	}
	return digest, nil
}

type historyExactPrefixReader struct {
	reader       io.Reader
	remaining    int64
	prematureEOF bool
}

func (reader *historyExactPrefixReader) Read(buffer []byte) (int, error) {
	if reader.prematureEOF {
		return 0, ErrHistorySnapshotChanged
	}
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(buffer)) > reader.remaining {
		buffer = buffer[:reader.remaining]
	}
	n, err := reader.reader.Read(buffer)
	reader.remaining -= int64(n)
	if err == io.EOF && reader.remaining > 0 {
		reader.prematureEOF = true
		return n, ErrHistorySnapshotChanged
	}
	return n, err
}

func historyPrefixPassReader(source io.Reader, size int64, ctx context.Context, deadline time.Time, consume func(*bufio.Reader) error) (string, error) {
	if source == nil || size < 0 {
		return "", ErrHistorySnapshotChanged
	}
	hasher := sha256.New()
	clock, _ := ctx.Value(historyWorkClockKey{}).(*historyWorkClock)
	exact := &historyExactPrefixReader{reader: historyDeadlineReader{ctx: ctx, deadline: deadline, reader: source, clock: clock}, remaining: size}
	reader := bufio.NewReaderSize(historyHashingReader{reader: exact, hash: hasher, clock: clock}, 64*1024)
	if consume != nil {
		if err := consume(reader); err != nil {
			return "", err
		}
	}
	if exact.prematureEOF {
		return "", ErrHistorySnapshotChanged
	}
	if clock != nil {
		clock.pause()
	}
	_, drainErr := io.Copy(io.Discard, historyDeadlineReader{ctx: ctx, deadline: deadline, reader: reader, clock: clock})
	if clock != nil {
		clock.resume()
	}
	if drainErr != nil {
		return "", drainErr
	}
	if exact.prematureEOF || exact.remaining != 0 {
		return "", ErrHistorySnapshotChanged
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !time.Now().Before(deadline) {
		return "", context.DeadlineExceeded
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}
