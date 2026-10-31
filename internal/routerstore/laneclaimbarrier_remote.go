package routerstore

import "context"

func (rs *RemoteStore) LaneClaimBarrier(ctx context.Context, req LaneClaimBarrierRequest) (LaneClaimBarrierResult, error) {
	var out LaneClaimBarrierResult
	err := rs.callCtx(ctx, "LaneClaimBarrier", []any{req}, &out)
	return out, err
}
