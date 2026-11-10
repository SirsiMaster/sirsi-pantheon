package routerstore

import (
	"context"
	"fmt"
)

// ExportReport says what a paged export wrote. Snapshot is always false: a keyset walk is not a
// point-in-time image (docs/router/BULK_EXPORT_CONTRACT.md). Point-in-time recovery is Cloud SQL PITR.
type ExportReport struct {
	Items    int    `json:"items"`
	Pages    int    `json:"pages"`
	LastID   string `json:"last_id"`
	Snapshot bool   `json:"snapshot"`
}

// ExportMarkdownPaged writes every item, one canonical <id>.md per item, into the caller's own dir,
// reading through ListPage one bounded page at a time. Nothing about the path reaches the service, and
// memory stays one page wide. On error the files already written stay, and the report counts them.
func ExportMarkdownPaged(ctx context.Context, s interface {
	ListPage(context.Context, string, int) ([]Item, error)
}, dir string, pageSize int) (ExportReport, error) {
	if pageSize < 1 || pageSize > ListPageMax {
		pageSize = ListPageMax
	}
	var r ExportReport
	for {
		page, err := s.ListPage(ctx, r.LastID, pageSize)
		if err != nil {
			return r, err
		}
		r.Pages++
		for _, it := range page {
			if it.ID <= r.LastID {
				return r, fmt.Errorf("routerstore: export: cursor did not advance past %q", r.LastID)
			}
			if _, err := writeItemExport(dir, it.ID, it); err != nil {
				return r, err
			}
			r.Items++
			r.LastID = it.ID
		}
		if len(page) < pageSize {
			return r, nil
		}
	}
}
