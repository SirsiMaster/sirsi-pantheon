package routerstore

import (
	"context"
	"fmt"
)

// ExportItemLocalContext writes the canonical audit projection into this
// client's local directory. The item itself is fetched through the existing
// authenticated, context-aware Get RPC; a caller-local path is never sent to
// the router service.
func (rs *RemoteStore) ExportItemLocalContext(ctx context.Context, dir, id string) (string, error) {
	it, err := rs.GetContext(ctx, id)
	if err != nil {
		return "", fmt.Errorf("routerstore: local ExportItem %q: get canonical item: %w", id, err)
	}
	if it.ID != id {
		return "", fmt.Errorf("routerstore: local ExportItem %q: canonical Get returned item %q", id, it.ID)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("routerstore: local ExportItem %q: context: %w", id, err)
	}
	return writeItemExport(dir, id, it)
}
