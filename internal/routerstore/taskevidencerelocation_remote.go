package routerstore

import "context"

// Context methods follow the existing handwritten mailbox/desktop RPC pattern;
// the context-free generated stub file is intentionally unchanged.
func (rs *RemoteStore) RelocateTaskEvidence(ctx context.Context, req TaskEvidenceRequest) (TaskEvidenceResult, error) {
	ctx, cancel := context.WithTimeout(ctx, rs.perCall)
	defer cancel()
	var out TaskEvidenceResult
	err := rs.callCtx(ctx, "RelocateTaskEvidence", []any{req}, &out)
	return out, err
}
