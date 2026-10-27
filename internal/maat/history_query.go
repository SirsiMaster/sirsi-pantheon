package maat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	HistoryScopeAll              = "all"
	HistoryScopeSystemOne        = "system-one"
	HistoryCoverageComplete      = "complete"
	HistoryCoveragePartial       = "partial"
	maxHistoryPageSize           = 100
	maxHistoryCursorBytes        = 64 << 10
	maxHistoryRecordBytes        = 256 << 10
	defaultHistoryByteBudget     = 4 << 20
	maxHistoryByteBudget         = 8 << 20
	defaultHistoryRowBudget      = 5000
	maxHistoryRowBudget          = 10000
	defaultHistoryTimeBudget     = 300 * time.Millisecond
	maxHistoryTimeBudget         = 2 * time.Second
	maxHistoryQualificationBatch = 48
)

var ErrHistorySnapshotChanged = errors.New("maat decision history: snapshot changed; refresh the query")

var errHistoryWorkBudget = errors.New("maat decision history: continue bounded materialization")

// HistoryQuery is a bounded read-only view over the append-only journal. Match
// is used for stable pre-page predicates such as kind/category; evidence is an
// exact identity predicate and scope is always applied before page limiting.
// QueryKey must bind every caller-visible filter so a cursor cannot be reused
// for another query.
type HistoryQuery struct {
	Scope      string
	Evidence   string
	QueryKey   string
	Limit      int
	Cursor     string
	Match      func(Decision) bool
	NeedsState func(Decision) bool
	// MatchState is evaluated before page limiting, after the candidate's
	// owner-review/acceptance and binary-updater links have been resolved from
	// the same snapshot.
	// It is used only for read-model predicates; it never grants mutation
	// authority.
	MatchState func(Decision, HistoryMatchState) bool
	Context    context.Context
	ByteBudget int64
	RowBudget  int
	TimeBudget time.Duration
}

type HistoryMatchState struct {
	Resolved      bool
	Hidden        bool
	Resolution    *Decision
	RepairOutcome *Decision
	RepairAttempt *Decision
}

// HistoryCandidate is one selected journal row with its physical byte offset.
// Offset is internal pagination identity, not an authorization token.
type HistoryCandidate struct {
	Decision Decision
	Offset   int64
}

// HistoryLinks contains only the bounded relationships needed to project the
// selected page. It is derived from the same journal snapshot, never stored as
// a second status authority.
type HistoryLinks struct {
	Acceptances           []Decision
	ReviewedResolutionFor []string
	RepairOutcomes        []Decision
	RepairAttempts        []Decision
}

// HistoryPage describes either an incomplete scan (with a resumable Cursor)
// or a completed page. NextCursor continues a bounded scan; NextPageCursor
// starts the next older page on this exact snapshot.
type HistoryPage struct {
	Candidates        []HistoryCandidate
	Links             HistoryLinks
	Integrity         JournalIntegrity
	Coverage          string
	Snapshot          string
	HasMore           bool
	Cursor            string
	NextPageCursor    string
	ScannedBytes      int64
	VerificationBytes int64 `json:"verification_bytes,omitempty"`
	ScannedRecords    int
	MatchCount        int64
	EvidenceMatches   int64
	AfterCursor       bool
}

type historyOrder struct {
	Time   string `json:"time"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size,omitempty"`
}

type historyCandidateRef struct {
	Order           historyOrder `json:"order"`
	EvidenceHash    string       `json:"evidence_hash,omitempty"`
	ResolutionHash  string       `json:"resolution_hash,omitempty"`
	OwnerAcceptance bool         `json:"owner_acceptance,omitempty"`
}

type historyLinkRef struct {
	EvidenceHash    string        `json:"evidence_hash,omitempty"`
	ResolutionHash  string        `json:"resolution_hash,omitempty"`
	OwnerAcceptance bool          `json:"owner_acceptance,omitempty"`
	Acceptance      *historyOrder `json:"acceptance,omitempty"`
	Review          *historyOrder `json:"review,omitempty"`
	RepairOutcome   *historyOrder `json:"repair_outcome,omitempty"`
	RepairAttempt   *historyOrder `json:"repair_attempt,omitempty"`
}

type historyCursor struct {
	Batch                []historyCandidateRef `json:"batch,omitempty"`
	BatchLinks           []historyLinkRef      `json:"batch_links,omitempty"`
	BatchIndex           int                   `json:"batch_index,omitempty"`
	Version              int                   `json:"v"`
	Snapshot             string                `json:"snapshot"`
	PrefixBytes          int64                 `json:"prefix_bytes"`
	PrefixSHA            string                `json:"prefix_sha256"`
	QueryKey             string                `json:"query"`
	Phase                string                `json:"phase"`
	Offset               int64                 `json:"offset"`
	Line                 int                   `json:"line,omitempty"`
	Limit                int                   `json:"limit"`
	After                *historyOrder         `json:"after,omitempty"`
	Candidates           []historyCandidateRef `json:"candidates,omitempty"`
	Seen                 int64                 `json:"seen,omitempty"`
	EvidenceSeen         int64                 `json:"evidence_seen,omitempty"`
	InvalidCount         int                   `json:"invalid_count,omitempty"`
	Issues               []JournalIssue        `json:"issues,omitempty"`
	Links                []historyLinkRef      `json:"links,omitempty"`
	DiscardingLine       bool                  `json:"discarding_line,omitempty"`
	DiscardLine          int                   `json:"discard_line,omitempty"`
	DiscardDigest        string                `json:"discard_digest,omitempty"`
	ResumeOffset         int64                 `json:"resume_offset,omitempty"`
	Pending              *historyCandidateRef  `json:"pending,omitempty"`
	PendingAccepted      bool                  `json:"pending_accepted,omitempty"`
	PendingReviewed      bool                  `json:"pending_reviewed,omitempty"`
	PendingAcceptance    *historyOrder         `json:"pending_acceptance,omitempty"`
	PendingRepairOutcome *historyOrder         `json:"pending_repair_outcome,omitempty"`
	PendingRepairAttempt *historyOrder         `json:"pending_repair_attempt,omitempty"`
}

// QueryHistory scans one fixed journal snapshot with bounded page/link state.
// A partial response contains no provisional cases; its cursor resumes the
// same scan or relationship pass. Recent and RecentTolerant retain their
// existing strict/tolerant contracts for mutation callers.
func (j *FileDecisionJournal) QueryHistory(query HistoryQuery) (HistoryPage, error) {
	return j.queryHistoryPrefixBound(query)
}

func scanHistoryChunk(ctx context.Context, deadline time.Time, file *os.File, state *historyCursor, byteBudget int64, rowBudget int, scope, evidence string, match func(Decision) bool, needsState func(Decision) bool, matchState func(Decision, HistoryMatchState) bool) (bool, int64, int, error) {
	if _, err := file.Seek(state.Offset, io.SeekStart); err != nil {
		return false, 0, 0, fmt.Errorf("maat decision history: seek: %w", err)
	}
	return scanHistoryChunkReader(ctx, deadline, bufio.NewReaderSize(file, 64*1024), state, byteBudget, rowBudget, scope, evidence, match, needsState, matchState)
}

func scanHistoryChunkReader(ctx context.Context, deadline time.Time, reader *bufio.Reader, state *historyCursor, byteBudget int64, rowBudget int, scope, evidence string, match func(Decision) bool, needsState func(Decision) bool, matchState func(Decision, HistoryMatchState) bool) (bool, int64, int, error) {
	var bytesRead int64
	rowsRead := 0
	for rowsRead == 0 || (bytesRead < byteBudget && rowsRead < rowBudget) {
		if ctx.Err() != nil || historyWorkExpired(ctx, deadline) {
			return false, bytesRead, rowsRead, nil
		}
		if state.DiscardingLine {
			done, consumed, err := discardHistoryLine(reader, byteBudget-bytesRead, deadline, ctx)
			if err != nil {
				return false, bytesRead, rowsRead, fmt.Errorf("maat decision history: skip oversized record: %w", err)
			}
			state.Offset += consumed
			bytesRead += consumed
			if consumed > 0 && rowsRead == 0 {
				rowsRead++
			}
			if !done {
				return false, bytesRead, rowsRead, nil
			}
			state.InvalidCount++
			appendHistoryIssue(state, state.DiscardDigest, "oversized record; digest covers bounded prefix")
			state.DiscardingLine = false
			state.DiscardLine = 0
			state.DiscardDigest = ""
			continue
		}
		start := state.Offset
		line, consumed, oversized, complete, digest, err := readHistoryLine(reader)
		if err == io.EOF {
			if len(state.Batch) > 0 {
				beginHistoryQualificationBatch(state)
				return false, bytesRead, rowsRead, nil
			}
			return true, bytesRead, rowsRead, nil
		}
		if err != nil {
			return false, bytesRead, rowsRead, fmt.Errorf("maat decision history: read: %w", err)
		}
		state.Offset += consumed
		bytesRead += consumed
		rowsRead++
		state.Line++
		line = bytes.TrimSpace(line)
		if len(line) == 0 && !oversized {
			continue
		}
		if oversized {
			if complete {
				state.InvalidCount++
				appendHistoryIssue(state, digest, "record exceeds bounded size")
			} else {
				state.DiscardingLine = true
				state.DiscardLine = state.Line
				state.DiscardDigest = digest
			}
			continue
		}
		var decision Decision
		if err := json.Unmarshal(line, &decision); err != nil {
			state.InvalidCount++
			appendHistoryIssue(state, digest, "malformed JSON record")
			continue
		}
		if err := validateDecision(decision); err != nil {
			state.InvalidCount++
			appendHistoryIssue(state, digest, "record violates the Ma'at journal contract")
			continue
		}
		if scope == HistoryScopeSystemOne && decision.SystemOne == nil && decision.SystemOneCalibration == nil {
			continue
		}
		if evidence != "" && decision.Evidence != evidence {
			continue
		}
		if state.After != nil && !historyOrderAfter(historyOrder{Time: decision.Time, Offset: start}, *state.After) {
			continue
		}
		// Keep exact-evidence multiplicity independent of projection filters
		// and hidden relationship rows. A browse projection cannot turn two
		// physical identities into unique mutation-preflight evidence.
		if evidence != "" {
			state.EvidenceSeen++
		}
		if match != nil && !match(decision) {
			continue
		}
		candidate := historyCandidateRef{
			Order:        historyOrder{Time: decision.Time, Offset: start, Size: consumed},
			EvidenceHash: historyKeyHash(decision.Evidence), ResolutionHash: historyKeyHash(decision.ResolutionFor),
			OwnerAcceptance: decision.Kind == "diagnostic owner acceptance",
		}
		if matchState != nil {
			if needsState == nil || needsState(decision) {
				state.Batch = append(state.Batch, candidate)
				if len(state.Batch) >= historyQualificationBatchCapacity(state) {
					beginHistoryQualificationBatch(state)
					return false, bytesRead, rowsRead, nil
				}
				continue
			}
			if !matchState(decision, HistoryMatchState{}) {
				continue
			}
		}
		state.Seen++
		insertHistoryCandidate(state, candidate, state.Limit+1)
	}
	return false, bytesRead, rowsRead, nil
}

func scanHistoryLinks(ctx context.Context, deadline time.Time, file *os.File, state *historyCursor, byteBudget int64, rowBudget int) (bool, int64, int, error) {
	if _, err := file.Seek(state.Offset, io.SeekStart); err != nil {
		return false, 0, 0, fmt.Errorf("maat decision history: seek links: %w", err)
	}
	return scanHistoryLinksReader(ctx, deadline, bufio.NewReaderSize(file, 64*1024), state, byteBudget, rowBudget)
}

func scanHistoryLinksReader(ctx context.Context, deadline time.Time, reader *bufio.Reader, state *historyCursor, byteBudget int64, rowBudget int) (bool, int64, int, error) {
	var bytesRead int64
	rowsRead := 0
	for rowsRead == 0 || (bytesRead < byteBudget && rowsRead < rowBudget) {
		if ctx.Err() != nil || historyWorkExpired(ctx, deadline) {
			return false, bytesRead, rowsRead, nil
		}
		if state.DiscardingLine {
			done, consumed, err := discardHistoryLine(reader, byteBudget-bytesRead, deadline, ctx)
			if err != nil {
				return false, bytesRead, rowsRead, fmt.Errorf("maat decision history: skip oversized linked record: %w", err)
			}
			state.Offset += consumed
			bytesRead += consumed
			if consumed > 0 && rowsRead == 0 {
				rowsRead++
			}
			if !done {
				return false, bytesRead, rowsRead, nil
			}
			state.DiscardingLine = false
			state.DiscardLine = 0
			state.DiscardDigest = ""
			continue
		}
		line, consumed, oversized, complete, digest, err := readHistoryLine(reader)
		if err == io.EOF {
			return true, bytesRead, rowsRead, nil
		}
		if err != nil {
			return false, bytesRead, rowsRead, fmt.Errorf("maat decision history: read links: %w", err)
		}
		start := state.Offset
		state.Offset += consumed
		bytesRead += consumed
		rowsRead++
		state.Line++
		if oversized {
			if !complete {
				state.DiscardingLine = true
				state.DiscardLine = state.Line
				state.DiscardDigest = digest
			}
			continue
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var decision Decision
		if json.Unmarshal(line, &decision) != nil || validateDecision(decision) != nil {
			continue
		}
		if decision.Kind == "diagnostic owner acceptance" && decision.ResolutionFor != "" {
			target := historyKeyHash(decision.ResolutionFor)
			for index := range state.Links {
				link := &state.Links[index]
				if link.EvidenceHash != target {
					continue
				}
				order := historyOrder{Time: decision.Time, Offset: start, Size: consumed}
				if link.Acceptance == nil || decision.Time > link.Acceptance.Time || (decision.Time == link.Acceptance.Time && start < link.Acceptance.Offset) {
					link.Acceptance = &order
				}
			}
		}
		if decision.Kind == "diagnostic owner review" && decision.Evidence != "" {
			target := historyKeyHash(decision.Evidence)
			for index := range state.Links {
				link := &state.Links[index]
				if link.OwnerAcceptance && link.ResolutionHash == target {
					order := historyOrder{Time: decision.Time, Offset: start, Size: consumed}
					if link.Review == nil || historyOrderBefore(order, *link.Review) {
						link.Review = &order
					}
				}
			}
		}
		for index := range state.Links {
			retainHistoryRepairLink(&state.Links[index], decision, historyOrder{Time: decision.Time, Offset: start, Size: consumed})
		}
	}
	return false, bytesRead, rowsRead, nil
}

func retainHistoryRepairLink(link *historyLinkRef, decision Decision, order historyOrder) {
	// Keep only the effective relationship: acceptance wins over outcome,
	// which wins over attempt. The cursor still retains at most two linked
	// row references per candidate, including the existing hidden-review link.
	if link.Acceptance != nil {
		link.RepairOutcome, link.RepairAttempt = nil, nil
		return
	}
	if link.EvidenceHash == "" || decision.Resource != "binary-drift" || decision.OriginEvidence == "" || historyKeyHash(decision.OriginEvidence) != link.EvidenceHash {
		return
	}
	switch decision.Kind {
	case "diagnostic verified repair":
		if link.RepairOutcome == nil || historyOrderBefore(order, *link.RepairOutcome) {
			link.RepairOutcome = &order
		}
		link.RepairAttempt = nil
	case "diagnostic repair attempt":
		if link.RepairOutcome == nil && (link.RepairAttempt == nil || historyOrderBefore(order, *link.RepairAttempt)) {
			link.RepairAttempt = &order
		}
	}
}

func selectedHistoryRepair(acceptance, outcome, attempt *historyOrder) (*historyOrder, string) {
	if acceptance != nil {
		return nil, ""
	}
	if outcome != nil {
		return outcome, "diagnostic verified repair"
	}
	return attempt, "diagnostic repair attempt"
}

func readHistoryRepairAtCount(ctx context.Context, deadline time.Time, file *os.File, ref *historyOrder, kind, originHash string) (Decision, int64, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, 0, err
	}
	if time.Now().After(deadline) {
		return Decision{}, 0, context.DeadlineExceeded
	}
	decision, consumed, err := readHistoryDecisionAtCount(file, ref.Offset)
	if err != nil {
		return Decision{}, consumed, err
	}
	if decision.Kind != kind || decision.Resource != "binary-drift" || decision.OriginEvidence == "" || historyKeyHash(decision.OriginEvidence) != originHash || decision.Time != ref.Time {
		return Decision{}, consumed, ErrHistorySnapshotChanged
	}
	return decision, consumed, nil
}

// scanPendingHistoryLinks resolves one candidate before it enters the bounded
// top-K, retaining only hashes and references until finalization.
func scanPendingHistoryLinks(ctx context.Context, deadline time.Time, file *os.File, state *historyCursor, byteBudget int64, rowBudget int) (bool, int64, int, error) {
	if state.Pending == nil {
		return false, 0, 0, fmt.Errorf("maat decision history: invalid pending candidate")
	}
	if _, err := file.Seek(state.Offset, io.SeekStart); err != nil {
		return false, 0, 0, fmt.Errorf("maat decision history: seek candidate links: %w", err)
	}
	return scanPendingHistoryLinksReader(ctx, deadline, bufio.NewReaderSize(file, 64*1024), state, byteBudget, rowBudget)
}

func scanPendingHistoryLinksReader(ctx context.Context, deadline time.Time, reader *bufio.Reader, state *historyCursor, byteBudget int64, rowBudget int) (bool, int64, int, error) {
	if state.Pending == nil {
		return false, 0, 0, fmt.Errorf("maat decision history: invalid pending candidate")
	}
	var bytesRead int64
	rowsRead := 0
	for rowsRead == 0 || (bytesRead < byteBudget && rowsRead < rowBudget) {
		if err := ctx.Err(); err != nil {
			return false, bytesRead, rowsRead, nil
		}
		if historyWorkExpired(ctx, deadline) {
			return false, bytesRead, rowsRead, nil
		}
		if state.DiscardingLine {
			done, consumed, err := discardHistoryLine(reader, byteBudget-bytesRead, deadline, ctx)
			if err != nil {
				return false, bytesRead, rowsRead, fmt.Errorf("maat decision history: skip oversized candidate link: %w", err)
			}
			state.Offset += consumed
			bytesRead += consumed
			if consumed > 0 && rowsRead == 0 {
				rowsRead++
			}
			if !done {
				return false, bytesRead, rowsRead, nil
			}
			state.DiscardingLine = false
			state.DiscardLine = 0
			state.DiscardDigest = ""
			continue
		}
		line, consumed, oversized, complete, digest, err := readHistoryLine(reader)
		if err == io.EOF {
			return true, bytesRead, rowsRead, nil
		}
		if err != nil {
			return false, bytesRead, rowsRead, fmt.Errorf("maat decision history: read candidate links: %w", err)
		}
		start := state.Offset
		state.Offset += consumed
		bytesRead += consumed
		rowsRead++
		if oversized || !complete || len(bytes.TrimSpace(line)) == 0 {
			if oversized && !complete {
				state.DiscardingLine = true
				state.DiscardLine = state.Line
				state.DiscardDigest = digest
			}
			continue
		}
		var decision Decision
		if json.Unmarshal(bytes.TrimSpace(line), &decision) != nil || validateDecision(decision) != nil {
			continue
		}
		if decision.Kind == "diagnostic owner acceptance" && decision.ResolutionFor != "" && historyKeyHash(decision.ResolutionFor) == state.Pending.EvidenceHash {
			state.PendingAccepted = true
			order := historyOrder{Time: decision.Time, Offset: start, Size: consumed}
			// Match Build's canonical latest-acceptance selection before applying
			// status/text filters and the bounded result window.
			if state.PendingAcceptance == nil || historyOrderBefore(order, *state.PendingAcceptance) {
				state.PendingAcceptance = &order
			}
		}
		if state.Pending.OwnerAcceptance && decision.Kind == "diagnostic owner review" && historyKeyHash(decision.Evidence) == state.Pending.ResolutionHash {
			state.PendingReviewed = true
		}
		link := historyLinkRef{EvidenceHash: state.Pending.EvidenceHash, Acceptance: state.PendingAcceptance, RepairOutcome: state.PendingRepairOutcome, RepairAttempt: state.PendingRepairAttempt}
		retainHistoryRepairLink(&link, decision, historyOrder{Time: decision.Time, Offset: start, Size: consumed})
		state.PendingRepairOutcome, state.PendingRepairAttempt = link.RepairOutcome, link.RepairAttempt
	}
	return false, bytesRead, rowsRead, nil
}

func finishPendingHistoryCandidate(ctx context.Context, deadline time.Time, file *os.File, state *historyCursor, query HistoryQuery) (bool, int64, int, error) {
	if state.Pending == nil {
		return false, 0, 0, fmt.Errorf("maat decision history: invalid pending candidate")
	}
	if err := ctx.Err(); err != nil {
		return false, 0, 0, err
	}
	if time.Now().After(deadline) {
		return false, 0, 0, context.DeadlineExceeded
	}
	decision, candidateBytes, err := readHistoryDecisionAtCount(file, state.Pending.Order.Offset)
	if err != nil {
		return false, candidateBytes, 1, err
	}
	rowsRead := 1
	bytesRead := candidateBytes
	matchState := HistoryMatchState{Resolved: state.PendingAccepted, Hidden: state.PendingReviewed}
	if state.PendingAcceptance != nil {
		if err := ctx.Err(); err != nil {
			return false, bytesRead, rowsRead, err
		}
		if time.Now().After(deadline) {
			return false, bytesRead, rowsRead, context.DeadlineExceeded
		}
		resolution, resolutionBytes, err := readHistoryDecisionAtCount(file, state.PendingAcceptance.Offset)
		bytesRead += resolutionBytes
		rowsRead++
		if err != nil {
			return false, bytesRead, rowsRead, err
		}
		if resolution.Kind != "diagnostic owner acceptance" || historyKeyHash(resolution.ResolutionFor) != state.Pending.EvidenceHash || resolution.Time != state.PendingAcceptance.Time {
			return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		matchState.Resolution = &resolution
	}
	if ref, kind := selectedHistoryRepair(state.PendingAcceptance, state.PendingRepairOutcome, state.PendingRepairAttempt); ref != nil {
		repair, consumed, readErr := readHistoryRepairAtCount(ctx, deadline, file, ref, kind, state.Pending.EvidenceHash)
		bytesRead += consumed
		if consumed > 0 {
			rowsRead++
		}
		if readErr != nil {
			return false, bytesRead, rowsRead, readErr
		}
		if kind == "diagnostic verified repair" {
			matchState.RepairOutcome = &repair
		} else {
			matchState.RepairAttempt = &repair
		}
	}
	if decision.Time != state.Pending.Order.Time || historyKeyHash(decision.Evidence) != state.Pending.EvidenceHash || historyKeyHash(decision.ResolutionFor) != state.Pending.ResolutionHash || (decision.Kind == "diagnostic owner acceptance") != state.Pending.OwnerAcceptance {
		return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
	}
	if query.MatchState != nil && !query.MatchState(decision, matchState) {
		return false, bytesRead, rowsRead, nil
	}
	if err := ctx.Err(); err != nil {
		return false, bytesRead, rowsRead, err
	}
	if time.Now().After(deadline) {
		return false, bytesRead, rowsRead, context.DeadlineExceeded
	}
	// Candidate-finalization is atomic at one record plus its selected link;
	// both bounded reads are included in the returned scan counters.
	insertHistoryCandidate(state, *state.Pending, state.Limit+1)
	return true, bytesRead, rowsRead, nil
}

func materializeHistoryPage(ctx context.Context, deadline time.Time, file *os.File, path, snapshot string, state *historyCursor, query HistoryQuery, page *HistoryPage, byteBudget int64, rowBudget int) (bool, int64, int, error) {
	if err := verifyHistorySnapshot(file, path, snapshot); err != nil {
		return false, 0, 0, err
	}
	count := len(state.Candidates)
	if count > query.Limit {
		count = query.Limit
	}
	page.Candidates = make([]HistoryCandidate, 0, count)
	acceptanceDecisions := map[int64]Decision{}
	var bytesRead int64
	rowsRead := 0
	limited := false
	for index := 0; index < count; index++ {
		if err := ctx.Err(); err != nil {
			return false, bytesRead, rowsRead, err
		}
		if time.Now().After(deadline) {
			return false, bytesRead, rowsRead, context.DeadlineExceeded
		}
		ref := state.Candidates[index]
		link := state.Links[index]
		estimatedBytes := ref.Order.Size
		estimatedRows := 1
		if link.Acceptance != nil {
			estimatedBytes += link.Acceptance.Size
			estimatedRows++
		}
		if link.Review != nil {
			estimatedBytes += link.Review.Size
			estimatedRows++
		}
		if repair, _ := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); repair != nil {
			estimatedBytes += repair.Size
			estimatedRows++
		}
		if bytesRead+estimatedBytes > byteBudget || rowsRead+estimatedRows > rowBudget {
			if index > 0 {
				limited = true
				break
			}
			if byteBudget != query.ByteBudget || rowBudget != query.RowBudget {
				return false, 0, 0, errHistoryWorkBudget
			}
			// A fresh request may materialize one atomic bounded record and
			// its at-most-two dependencies even with a smaller injected budget.
			// Charge every reread. This fixed allowance ensures continuation
			// progresses without admitting a whole page beyond the budget.
		}
		decision, decisionBytes, err := readHistoryDecisionAtCount(file, ref.Order.Offset)
		bytesRead += decisionBytes
		rowsRead++
		if err != nil {
			return false, bytesRead, rowsRead, err
		}
		if err := ctx.Err(); err != nil {
			return false, bytesRead, rowsRead, err
		}
		if time.Now().After(deadline) {
			return false, bytesRead, rowsRead, context.DeadlineExceeded
		}
		if decision.Time != ref.Order.Time || historyKeyHash(decision.Evidence) != ref.EvidenceHash || historyKeyHash(decision.ResolutionFor) != ref.ResolutionHash || (decision.Kind == "diagnostic owner acceptance") != ref.OwnerAcceptance || (query.Scope == HistoryScopeSystemOne && decision.SystemOne == nil && decision.SystemOneCalibration == nil) || (query.Evidence != "" && decision.Evidence != query.Evidence) || (query.Match != nil && !query.Match(decision)) || (state.After != nil && !historyOrderAfter(historyOrder{Time: decision.Time, Offset: ref.Order.Offset}, *state.After)) {
			return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		if link.EvidenceHash != ref.EvidenceHash || link.ResolutionHash != ref.ResolutionHash || link.OwnerAcceptance != ref.OwnerAcceptance {
			return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		matchState := HistoryMatchState{}
		if ref.OwnerAcceptance && link.Review != nil && decision.ResolutionFor != "" {
			review, reviewBytes, err := readHistoryDecisionAtCount(file, link.Review.Offset)
			bytesRead += reviewBytes
			rowsRead++
			if err != nil || review.Kind != "diagnostic owner review" || historyKeyHash(review.Evidence) != ref.ResolutionHash || review.Time != link.Review.Time {
				return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
			}
			page.Links.ReviewedResolutionFor = append(page.Links.ReviewedResolutionFor, decision.ResolutionFor)
			matchState.Hidden = true
		}
		if link.Acceptance != nil {
			acceptance, acceptanceBytes, err := readHistoryDecisionAtCount(file, link.Acceptance.Offset)
			bytesRead += acceptanceBytes
			rowsRead++
			if err != nil || acceptance.Kind != "diagnostic owner acceptance" || historyKeyHash(acceptance.ResolutionFor) != ref.EvidenceHash || acceptance.Time != link.Acceptance.Time {
				return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
			}
			acceptanceDecisions[link.Acceptance.Offset] = acceptance
			matchState.Resolved = true
			matchState.Resolution = &acceptance
		}
		if ref, kind := selectedHistoryRepair(link.Acceptance, link.RepairOutcome, link.RepairAttempt); ref != nil {
			repair, consumed, readErr := readHistoryRepairAtCount(ctx, deadline, file, ref, kind, link.EvidenceHash)
			bytesRead += consumed
			if consumed > 0 {
				rowsRead++
			}
			if readErr != nil {
				return false, bytesRead, rowsRead, readErr
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
			return false, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		if err := ctx.Err(); err != nil {
			return false, bytesRead, rowsRead, err
		}
		if time.Now().After(deadline) {
			return false, bytesRead, rowsRead, context.DeadlineExceeded
		}
		page.Candidates = append(page.Candidates, HistoryCandidate{Decision: decision, Offset: ref.Order.Offset})
	}
	offsets := make([]int64, 0, len(acceptanceDecisions))
	for offset := range acceptanceDecisions {
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	for _, offset := range offsets {
		page.Links.Acceptances = append(page.Links.Acceptances, acceptanceDecisions[offset])
	}
	if err := verifyHistorySnapshot(file, path, snapshot); err != nil {
		return false, bytesRead, rowsRead, err
	}
	return limited, bytesRead, rowsRead, nil
}

func verifyHistorySnapshot(file *os.File, path, expected string) error {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || historySnapshotID(path, info) != expected {
		return ErrHistorySnapshotChanged
	}
	pathInfo, err := os.Stat(path)
	if err != nil || !pathInfo.Mode().IsRegular() || historySnapshotID(path, pathInfo) != expected {
		return ErrHistorySnapshotChanged
	}
	return nil
}

func readHistoryDecisionAtCount(file *os.File, offset int64) (Decision, int64, error) {
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return Decision{}, 0, fmt.Errorf("maat decision history: seek selected row: %w", err)
	}
	line, consumed, oversized, complete, _, err := readHistoryLine(bufio.NewReaderSize(file, 64*1024))
	if err != nil || oversized || !complete {
		return Decision{}, consumed, fmt.Errorf("maat decision history: selected row is no longer readable")
	}
	var decision Decision
	if err := json.Unmarshal(bytes.TrimSpace(line), &decision); err != nil || validateDecision(decision) != nil {
		return Decision{}, consumed, ErrHistorySnapshotChanged
	}
	return decision, consumed, nil
}

// captureHistoryDecisions reads only referenced records, in physical order,
// from the request's already-hashed forward stream. Returned values are then
// evaluated without reopening or seeking the journal.
func captureHistoryDecisions(ctx context.Context, deadline time.Time, reader *bufio.Reader, refs []*historyOrder) (map[int64]Decision, int64, int, error) {
	type target struct {
		offset int64
		size   int64
	}
	targetsByOffset := make(map[int64]int64, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		if previous, exists := targetsByOffset[ref.Offset]; exists && previous != ref.Size {
			return nil, 0, 0, ErrHistorySnapshotChanged
		}
		targetsByOffset[ref.Offset] = ref.Size
	}
	targets := make([]target, 0, len(targetsByOffset))
	for offset, size := range targetsByOffset {
		targets = append(targets, target{offset: offset, size: size})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].offset < targets[j].offset })
	selected := make(map[int64]Decision, len(targets))
	var position int64
	var bytesRead int64
	rowsRead := 0
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return nil, bytesRead, rowsRead, err
		}
		if historyWorkExpired(ctx, deadline) {
			return nil, bytesRead, rowsRead, errHistoryWorkBudget
		}
		if target.offset < position || target.size <= 0 || target.size > maxHistoryRecordBytes+1 {
			return nil, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		clock, _ := ctx.Value(historyWorkClockKey{}).(*historyWorkClock)
		if clock != nil {
			clock.pause()
		}
		skipped, err := io.CopyN(io.Discard, reader, target.offset-position)
		if clock != nil {
			clock.resume()
		}
		position += skipped
		if err != nil {
			return nil, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		raw := make([]byte, target.size)
		if _, err := io.ReadFull(reader, raw); err != nil {
			return nil, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		position += target.size
		bytesRead += target.size
		rowsRead++
		var decision Decision
		if err := json.Unmarshal(bytes.TrimSpace(raw), &decision); err != nil || validateDecision(decision) != nil {
			return nil, bytesRead, rowsRead, ErrHistorySnapshotChanged
		}
		selected[target.offset] = decision
	}
	return selected, bytesRead, rowsRead, nil
}

func readHistoryLine(reader *bufio.Reader) ([]byte, int64, bool, bool, string, error) {
	var line []byte
	var consumed int64
	var digest hash.Hash
	oversized := false
	for {
		part, err := reader.ReadSlice('\n')
		consumed += int64(len(part))
		if oversized {
			if _, writeErr := digest.Write(part); writeErr != nil {
				return nil, consumed, true, false, "", writeErr
			}
		} else if len(line)+len(part) > maxHistoryRecordBytes+1 {
			oversized = true
			digest = sha256.New()
			_, _ = digest.Write(line)
			_, _ = digest.Write(part)
			line = nil
		} else {
			line = append(line, part...)
		}
		if err == nil {
			if oversized {
				return nil, consumed, true, true, fmt.Sprintf("sha256:%x", digest.Sum(nil)), nil
			}
			line = bytes.TrimSuffix(line, []byte{'\n'})
			digestValue := digestBytes(line)
			line = bytes.TrimSuffix(line, []byte{'\r'})
			return line, consumed, false, true, digestValue, nil
		}
		if err == bufio.ErrBufferFull {
			if oversized {
				return nil, consumed, true, false, fmt.Sprintf("sha256:%x", digest.Sum(nil)), nil
			}
			continue
		}
		if err == io.EOF {
			if consumed == 0 {
				return nil, 0, false, false, "", io.EOF
			}
			if oversized {
				return nil, consumed, true, true, fmt.Sprintf("sha256:%x", digest.Sum(nil)), nil
			}
			digestValue := digestBytes(line)
			line = bytes.TrimSuffix(line, []byte{'\r'})
			return line, consumed, false, true, digestValue, nil
		}
		return nil, consumed, oversized, false, "", err
	}
}

// discardHistoryLine consumes only one bounded chunk of an oversized row.
// The caller persists the offset and resumes later if the newline is not yet
// reached, so one corrupt line cannot bypass the request's scan budget.
func discardHistoryLine(reader *bufio.Reader, byteBudget int64, deadline time.Time, ctx context.Context) (bool, int64, error) {
	if byteBudget <= 0 {
		return false, 0, nil
	}
	var consumed int64
	for consumed == 0 || consumed < byteBudget {
		if ctx.Err() != nil || historyWorkExpired(ctx, deadline) {
			return false, consumed, nil
		}
		part, err := reader.ReadSlice('\n')
		consumed += int64(len(part))
		if err == nil || (err == io.EOF && len(part) > 0) {
			return true, consumed, nil
		}
		if err == io.EOF {
			return true, consumed, nil
		}
		if err != bufio.ErrBufferFull {
			return false, consumed, err
		}
	}
	return false, consumed, nil
}

func appendHistoryIssue(state *historyCursor, digest, reason string) {
	if len(state.Issues) >= maxRetainedJournalIssues {
		return
	}
	state.Issues = append(state.Issues, JournalIssue{Line: state.Line, Digest: digest, Reason: reason})
}

func cursorIntegrity(state *historyCursor) JournalIntegrity {
	return JournalIntegrity{InvalidCount: state.InvalidCount, Issues: append([]JournalIssue(nil), state.Issues...)}
}

func historySnapshotID(path string, info os.FileInfo) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	material := strings.Join([]string{
		filepath.Clean(path), info.Mode().String(), fmt.Sprint(info.Size()),
		info.ModTime().UTC().Format(time.RFC3339Nano), stableHistoryFileIdentity(info.Sys()),
	}, "\x00")
	return digestBytes([]byte(material))
}

// stableHistoryFileIdentity binds continuation to device/inode and change
// time where the platform exposes them, without including access time (which
// a read itself may update). The type fallback keeps unsupported FileInfo
// implementations distinct without pretending they expose a stable inode.
func stableHistoryFileIdentity(value any) string {
	raw := reflect.ValueOf(value)
	for raw.IsValid() && (raw.Kind() == reflect.Pointer || raw.Kind() == reflect.Interface) {
		if raw.IsNil() {
			return ""
		}
		raw = raw.Elem()
	}
	if !raw.IsValid() || raw.Kind() != reflect.Struct {
		return fmt.Sprintf("%T", value)
	}
	identity := []string{raw.Type().String()}
	for index := 0; index < raw.NumField(); index++ {
		field := raw.Type().Field(index)
		name := strings.ToLower(field.Name)
		if strings.Contains(name, "atim") || strings.Contains(name, "access") {
			continue
		}
		value := raw.Field(index)
		if !value.CanInterface() {
			continue
		}
		encoded, err := json.Marshal(value.Interface())
		if err != nil {
			encoded = []byte(fmt.Sprint(value.Interface()))
		}
		identity = append(identity, field.Name+":"+string(encoded))
	}
	return strings.Join(identity, "\x00")
}

func historyKeyHash(value string) string {
	if value == "" {
		return ""
	}
	return digestBytes([]byte(value))
}

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func insertHistoryCandidate(state *historyCursor, candidate historyCandidateRef, cap int) {
	index := sort.Search(len(state.Candidates), func(index int) bool {
		return historyOrderBefore(candidate.Order, state.Candidates[index].Order)
	})
	if index >= cap {
		return
	}
	state.Candidates = append(state.Candidates, historyCandidateRef{})
	copy(state.Candidates[index+1:], state.Candidates[index:])
	state.Candidates[index] = candidate
	if len(state.Candidates) > cap {
		state.Candidates = state.Candidates[:cap]
	}
}

func makeHistoryLinkRefs(candidates []historyCandidateRef) []historyLinkRef {
	links := make([]historyLinkRef, len(candidates))
	for index, candidate := range candidates {
		links[index] = historyLinkRef{EvidenceHash: candidate.EvidenceHash, ResolutionHash: candidate.ResolutionHash, OwnerAcceptance: candidate.OwnerAcceptance}
	}
	return links
}

func historyOrderBefore(left, right historyOrder) bool {
	if left.Time != right.Time {
		return left.Time > right.Time
	}
	return left.Offset < right.Offset
}

func historyOrderAfter(candidate, after historyOrder) bool {
	return historyOrderBefore(after, candidate)
}

func nextHistoryPageCursor(state *historyCursor, last historyOrder) *historyCursor {
	return &historyCursor{Version: state.Version, Snapshot: state.Snapshot, PrefixBytes: state.PrefixBytes, PrefixSHA: state.PrefixSHA, QueryKey: state.QueryKey, Phase: "scan", Limit: state.Limit, After: &last}
}

func encodeHistoryCursor(state *historyCursor) (string, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > maxHistoryCursorBytes {
		return "", fmt.Errorf("maat decision history: continuation exceeds its size bound")
	}
	return encoded, nil
}

func decodeHistoryCursor(encoded string) (*historyCursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > maxHistoryCursorBytes {
		return nil, fmt.Errorf("maat decision history: continuation exceeds its size bound")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("maat decision history: invalid continuation")
	}
	var state historyCursor
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("maat decision history: invalid continuation")
	}
	return &state, nil
}

func validateHistoryCursor(state *historyCursor, limit int, fileSize int64) error {
	if state == nil || state.Offset < 0 || state.Line < 0 || state.Seen < 0 || state.EvidenceSeen < 0 || state.InvalidCount < 0 || state.Limit != limit || len(state.Candidates) > limit+1 || len(state.Issues) > maxRetainedJournalIssues || state.PrefixBytes < 0 || state.PrefixBytes > fileSize {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if state.Version != historyCursorVersion {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if historyCursorVersion == 4 {
		if state.Phase == "anchor" || state.PrefixBytes != 0 || state.PrefixSHA != "" {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
	} else if state.PrefixSHA != "" && !validHistoryDigest(state.PrefixSHA) || state.Phase != "anchor" && !validHistoryDigest(state.PrefixSHA) {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if state.DiscardingLine && (state.DiscardLine <= 0 || state.DiscardLine > state.Line || !validHistoryDigest(state.DiscardDigest)) {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	for _, issue := range state.Issues {
		if issue.Line <= 0 || issue.Line > state.Line || !validHistoryDigest(issue.Digest) || !validHistoryIssueReason(issue.Reason) {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
	}
	if state.Offset > fileSize || state.ResumeOffset < 0 || state.ResumeOffset > fileSize || (state.After != nil && (state.After.Offset < 0 || state.After.Offset >= fileSize || state.After.Time == "")) {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if state.Phase != "anchor" && state.Phase != "scan" && state.Phase != "links" && state.Phase != "candidate-links" && state.Phase != "candidate-finalize" && state.Phase != "materialize" && state.Phase != "batch-links" && state.Phase != "batch-finalize" {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if state.Phase == "candidate-links" || state.Phase == "candidate-finalize" {
		if state.Pending == nil || state.ResumeOffset <= 0 || state.Pending.Order.Offset < 0 || state.Pending.Order.Offset >= fileSize || state.Pending.Order.Time == "" || state.Pending.Order.Size <= 0 || state.Pending.Order.Size > maxHistoryRecordBytes+1 {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
		if (state.Pending.EvidenceHash != "" && !validHistoryDigest(state.Pending.EvidenceHash)) || (state.Pending.ResolutionHash != "" && !validHistoryDigest(state.Pending.ResolutionHash)) || state.PendingAccepted != (state.PendingAcceptance != nil) || (state.PendingReviewed && !state.Pending.OwnerAcceptance) {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
	} else if state.Pending != nil || (state.ResumeOffset != 0 && state.Phase != "batch-links" && state.Phase != "batch-finalize") || state.PendingAcceptance != nil || state.PendingRepairOutcome != nil || state.PendingRepairAttempt != nil || state.PendingAccepted || state.PendingReviewed {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if state.PendingAcceptance != nil && (state.PendingAcceptance.Offset < 0 || state.PendingAcceptance.Offset >= fileSize || state.PendingAcceptance.Time == "" || state.PendingAcceptance.Size <= 0 || state.PendingAcceptance.Size > maxHistoryRecordBytes+1) {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if !validHistoryRepairRef(state.PendingRepairOutcome, fileSize) || !validHistoryRepairRef(state.PendingRepairAttempt, fileSize) {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if !exclusiveHistoryOutcome(state.PendingAcceptance, state.PendingRepairOutcome, state.PendingRepairAttempt) {
		return fmt.Errorf("maat decision history: conflicting continuation outcomes")
	}
	for index, candidate := range state.Candidates {
		if candidate.Order.Offset < 0 || candidate.Order.Offset >= fileSize || candidate.Order.Time == "" || candidate.Order.Size <= 0 || candidate.Order.Size > maxHistoryRecordBytes+1 || (candidate.EvidenceHash != "" && !validHistoryDigest(candidate.EvidenceHash)) || (candidate.ResolutionHash != "" && !validHistoryDigest(candidate.ResolutionHash)) {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
		if index > 0 && !historyOrderBefore(state.Candidates[index-1].Order, candidate.Order) {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
	}
	for _, link := range append(append([]historyLinkRef{}, state.Links...), state.BatchLinks...) {
		if !exclusiveHistoryOutcome(link.Acceptance, link.RepairOutcome, link.RepairAttempt) {
			return fmt.Errorf("maat decision history: conflicting continuation outcomes")
		}
		if !validHistoryRepairRef(link.RepairOutcome, fileSize) || !validHistoryRepairRef(link.RepairAttempt, fileSize) {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
		if (link.Acceptance != nil && (link.Acceptance.Offset < 0 || link.Acceptance.Offset >= fileSize || link.Acceptance.Time == "" || link.Acceptance.Size <= 0 || link.Acceptance.Size > maxHistoryRecordBytes+1)) || (link.Review != nil && (link.Review.Offset < 0 || link.Review.Offset >= fileSize || link.Review.Time == "" || link.Review.Size <= 0 || link.Review.Size > maxHistoryRecordBytes+1)) || (link.EvidenceHash != "" && !validHistoryDigest(link.EvidenceHash)) || (link.ResolutionHash != "" && !validHistoryDigest(link.ResolutionHash)) {
			return fmt.Errorf("maat decision history: invalid continuation")
		}
	}
	if (state.Phase == "links" || state.Phase == "materialize") && len(state.Links) != len(state.Candidates) {
		return fmt.Errorf("maat decision history: invalid continuation")
	}
	if state.Phase == "links" || state.Phase == "materialize" {
		for index, candidate := range state.Candidates {
			link := state.Links[index]
			if link.EvidenceHash != candidate.EvidenceHash || link.ResolutionHash != candidate.ResolutionHash || link.OwnerAcceptance != candidate.OwnerAcceptance {
				return fmt.Errorf("maat decision history: invalid continuation")
			}
		}
	}
	if err := validateHistoryQualificationBatch(state, fileSize); err != nil {
		return err
	}
	return nil
}

func validHistoryRepairRef(ref *historyOrder, fileSize int64) bool {
	return ref == nil || (ref.Offset >= 0 && ref.Offset < fileSize && ref.Time != "" && ref.Size > 0 && ref.Size <= maxHistoryRecordBytes+1)
}

func exclusiveHistoryOutcome(acceptance, outcome, attempt *historyOrder) bool {
	return (acceptance == nil || (outcome == nil && attempt == nil)) && (outcome == nil || attempt == nil)
}

func validHistoryDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validHistoryIssueReason(value string) bool {
	switch value {
	case "record exceeds bounded size", "oversized record; digest covers bounded prefix", "malformed JSON record", "record violates the Ma'at journal contract":
		return true
	default:
		return false
	}
}
