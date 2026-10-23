package routerstore

import "context"

func (rs *RemoteStore) ReconcileMailboxNotice(ctx context.Context, req MailboxReconcileRequest) (MailboxReconcileResult, error) {
	ctx, cancel := context.WithTimeout(ctx, rs.perCall)
	defer cancel()
	var out MailboxReconcileResult
	err := rs.callCtx(ctx, "ReconcileMailboxNotice", []any{req}, &out)
	return out, err
}
