package routerstore

import "context"

func (rs *RemoteStore) ResumeTaskClaim(ctx context.Context, req TaskResumeRequest) (TaskResumeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, rs.perCall)
	defer cancel()
	var out TaskResumeResult
	err := rs.callCtx(ctx, "ResumeTaskClaim", []any{req}, &out)
	return out, err
}
