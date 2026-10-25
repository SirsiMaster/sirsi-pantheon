package maat

import (
	"encoding/json"
	"fmt"
)

// Qualification batches amortize the relationship pass across references,
// not raw records. They do not select a result window until each candidate has
// been evaluated against its complete same-snapshot relationship state.
func beginHistoryQualificationBatch(state *historyCursor) {
	state.ResumeOffset = state.Offset
	state.Offset = 0
	state.BatchIndex = 0
	state.BatchLinks = makeHistoryLinkRefs(state.Batch)
	state.Phase = "batch-links"
}

func historyQualificationBatchCapacity(state *historyCursor) int {
	// Reserve cursor space for the existing top-K/integrity metadata and at
	// most two selected relationship refs per batched candidate. The canonical
	// encoder remains the final strict 64KiB limit, including unusual timestamps.
	base := *state
	base.Batch = nil
	base.BatchLinks = nil
	raw, _ := json.Marshal(base)
	capacity := ((maxHistoryCursorBytes * 3 / 4) - len(raw) - 2048) / 768
	if capacity < 1 {
		return 1
	}
	if capacity > maxHistoryQualificationBatch {
		return maxHistoryQualificationBatch
	}
	return capacity
}

func validateHistoryQualificationBatch(state *historyCursor, fileSize int64) error {
	invalid := func() error { return fmt.Errorf("maat decision history: invalid qualification continuation") }
	if len(state.Batch) > maxHistoryQualificationBatch || state.BatchIndex < 0 || state.BatchIndex > len(state.Batch) {
		return invalid()
	}
	batching := state.Phase == "batch-links" || state.Phase == "batch-finalize"
	if batching {
		if len(state.Batch) == 0 || len(state.BatchLinks) != len(state.Batch) || state.BatchIndex >= len(state.Batch) || state.ResumeOffset <= 0 {
			return invalid()
		}
		if state.Phase == "batch-links" && state.BatchIndex != 0 {
			return invalid()
		}
	} else if len(state.BatchLinks) != 0 || state.BatchIndex != 0 || (state.Phase != "scan" && len(state.Batch) != 0) {
		return invalid()
	}
	for i, candidate := range state.Batch {
		ref := candidate.Order
		if ref.Offset < 0 || ref.Offset >= fileSize || ref.Time == "" || ref.Size <= 0 || ref.Size > maxHistoryRecordBytes+1 || ref.Size > fileSize-ref.Offset {
			return invalid()
		}
		if (candidate.EvidenceHash != "" && !validHistoryDigest(candidate.EvidenceHash)) || (candidate.ResolutionHash != "" && !validHistoryDigest(candidate.ResolutionHash)) {
			return invalid()
		}
		if i > 0 && state.Batch[i-1].Order.Offset >= ref.Offset {
			return invalid()
		}
		if !batching && ref.Offset+ref.Size > state.Offset {
			return invalid()
		}
		if batching {
			if ref.Offset+ref.Size > state.ResumeOffset {
				return invalid()
			}
			link := state.BatchLinks[i]
			if link.EvidenceHash != candidate.EvidenceHash || link.ResolutionHash != candidate.ResolutionHash || link.OwnerAcceptance != candidate.OwnerAcceptance {
				return invalid()
			}
		}
	}
	return nil
}
