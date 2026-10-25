package knowledge

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type searchCursor struct {
	Generation int64  `json:"g"`
	Offset     int    `json:"o"`
	QueryHash  string `json:"q"`
}

func (a *Archive) Search(ctx context.Context, query SearchQuery) (SearchPage, error) {
	if err := a.verifyLocation(); err != nil {
		return SearchPage{}, err
	}
	tx, err := a.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge read snapshot is unavailable")
	}
	defer tx.Rollback()
	page, err := a.searchSnapshot(ctx, query, tx)
	if err != nil {
		return SearchPage{}, err
	}
	if err := a.verifyLocation(); err != nil {
		return SearchPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge read snapshot could not finish")
	}
	return page, nil
}

func (a *Archive) searchSnapshot(ctx context.Context, query SearchQuery, db archiveQueryer) (SearchPage, error) {
	return a.searchSnapshotWithHook(ctx, query, db, nil)
}

func (a *Archive) searchSnapshotWithHook(ctx context.Context, query SearchQuery, db archiveQueryer, afterGeneration func()) (SearchPage, error) {
	if a == nil || a.db == nil {
		return SearchPage{}, errors.New("Ma'at knowledge archive is unavailable")
	}
	limit := query.Limit
	if limit < 0 {
		return SearchPage{}, errors.New("knowledge page limit cannot be negative")
	}
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		return SearchPage{}, errors.New("knowledge page limit exceeds its bound")
	}
	if len(query.Text) > 512 || len(query.Classification) > 48 || len(query.Source) > 80 {
		return SearchPage{}, errors.New("Ma'at knowledge query exceeds its bound")
	}
	if query.Resolution != "" && query.Resolution != ResolutionOpen && query.Resolution != ResolutionReviewed && query.Resolution != ResolutionAccepted && query.Resolution != ResolutionRejected {
		return SearchPage{}, errors.New("Ma'at knowledge resolution filter is invalid")
	}
	if query.Classification != "" && query.Classification != "conversation" && query.Classification != "search" && query.Classification != "note" && query.Classification != "document" && query.Classification != "unclassified" {
		return SearchPage{}, errors.New("Ma'at knowledge classification filter is invalid")
	}
	query.Text = strings.TrimSpace(query.Text)
	queryHash := normalizedQueryHash(query)
	var generation int64
	if err := db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM archive_meta WHERE key='generation'`).Scan(&generation); err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge archive generation is unavailable")
	}
	if afterGeneration != nil {
		afterGeneration()
	}
	offset := 0
	if query.Cursor != "" {
		cursor, err := decodeSearchCursor(query.Cursor)
		if err != nil || cursor.QueryHash != queryHash || cursor.Offset < 0 {
			return SearchPage{}, errors.New("Ma'at knowledge page cursor is invalid")
		}
		if cursor.Generation != generation {
			return SearchPage{}, errors.New("Ma'at knowledge changed; restart the search")
		}
		offset = cursor.Offset
	}
	if query.WithheldOnly {
		if query.Text != "" || query.Classification != "" || query.Resolution != "" || query.Source != "" || query.IncludeHistory {
			return SearchPage{}, errors.New("withheld-item listing cannot combine content or record filters")
		}
		var total int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM records WHERE withheld_reason<>''`).Scan(&total); err != nil {
			return SearchPage{}, errors.New("Ma'at withheld knowledge count is unavailable")
		}
		rows, err := db.QueryContext(ctx, `SELECT id,source,resolution,title,summary,current_revision,withheld_reason,created_at,updated_at FROM records WHERE withheld_reason<>'' ORDER BY id LIMIT ? OFFSET ?`, limit+1, offset)
		if err != nil {
			return SearchPage{}, errors.New("Ma'at withheld knowledge list is unavailable")
		}
		page := SearchPage{Items: []ArchiveRecord{}, Total: total, Withheld: total}
		for rows.Next() {
			var item ArchiveRecord
			var created, updated string
			if err := rows.Scan(&item.ID, &item.Source, &item.Resolution, &item.Title, &item.Summary, &item.Revision, &item.WithheldReason, &created, &updated); err != nil {
				_ = rows.Close()
				return SearchPage{}, errors.New("Ma'at withheld knowledge list is malformed")
			}
			item.Classification = "withheld"
			item.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
			item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
			page.Items = append(page.Items, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return SearchPage{}, errors.New("Ma'at withheld knowledge list could not finish")
		}
		if err := rows.Close(); err != nil {
			return SearchPage{}, errors.New("Ma'at withheld knowledge list could not finish")
		}
		more := len(page.Items) > limit
		if more {
			page.Items = page.Items[:limit]
			page.NextCursor = encodeSearchCursor(searchCursor{Generation: generation, Offset: offset + limit, QueryHash: queryHash})
		}
		page.Coverage, err = a.coverageWith(ctx, db, 50)
		if err != nil {
			return SearchPage{}, err
		}
		return page, nil
	}
	var withheld int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM records WHERE withheld_reason<>''`).Scan(&withheld); err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge withheld count is unavailable")
	}
	where := `r.withheld_reason='' AND (?='' OR r.classification=?) AND (?='' OR r.resolution=?) AND (?='' OR r.source=?)`
	args := []any{query.Classification, query.Classification, string(query.Resolution), string(query.Resolution), query.Source, query.Source}
	if query.Text != "" {
		where += ` AND knowledge_fts MATCH ?`
		args = append(args, query.Text)
	}
	if !query.IncludeHistory {
		where += ` AND v.revision=r.current_revision`
	}
	countSQL := `SELECT COUNT(DISTINCT v.content_digest) FROM records r JOIN versions v ON v.record_id=r.id`
	if query.Text != "" {
		countSQL = `SELECT COUNT(DISTINCT v.content_digest) FROM records r JOIN versions v ON v.record_id=r.id JOIN knowledge_fts ON knowledge_fts.record_id=r.id AND knowledge_fts.revision=v.revision`
	}
	var total int
	if err := db.QueryRowContext(ctx, countSQL+` WHERE `+where, args...).Scan(&total); err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge search query is invalid or unavailable")
	}
	selectSQL := `SELECT r.id,v.revision,v.content_digest,ROW_NUMBER() OVER(PARTITION BY v.content_digest ORDER BY r.id ASC,v.revision DESC) AS duplicate_rank FROM records r JOIN versions v ON v.record_id=r.id`
	selectArgs := []any{}
	if query.Text != "" {
		selectSQL += ` JOIN knowledge_fts ON knowledge_fts.record_id=r.id AND knowledge_fts.revision=v.revision`
	}
	selectSQL += ` WHERE ` + where
	selectArgs = append(selectArgs, args...)
	selectSQL = `SELECT id,revision,content_digest FROM (` + selectSQL + `) WHERE duplicate_rank=1 ORDER BY content_digest ASC,id ASC LIMIT ? OFFSET ?`
	selectArgs = append(selectArgs, limit+1, offset)
	rows, err := db.QueryContext(ctx, selectSQL, selectArgs...)
	if err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge search query is invalid or unavailable")
	}
	defer rows.Close()
	type hit struct {
		id  string
		rev int
	}
	hits := make([]hit, 0, limit+1)
	for rows.Next() {
		var item hit
		var digest string
		if err := rows.Scan(&item.id, &item.rev, &digest); err != nil {
			return SearchPage{}, errors.New("Ma'at knowledge search result is malformed")
		}
		hits = append(hits, item)
	}
	if err := rows.Err(); err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge search could not finish")
	}
	if err := rows.Close(); err != nil {
		return SearchPage{}, errors.New("Ma'at knowledge search could not finish")
	}
	more := len(hits) > limit
	if more {
		hits = hits[:limit]
	}
	page := SearchPage{Items: make([]ArchiveRecord, 0, len(hits)), Total: total, Withheld: withheld}
	page.Coverage, err = a.coverageWith(ctx, db, 50)
	if err != nil {
		return SearchPage{}, err
	}
	for _, item := range hits {
		record, err := a.detailWith(ctx, db, item.id, item.rev)
		if err != nil {
			return SearchPage{}, err
		}
		if record.WithheldReason != "" {
			page.Withheld++
		} else {
			record.Content = ""
			if query.Text != "" {
				// FTS5 auxiliary functions need the FTS cursor in the SELECT that
				// evaluates them. Keep ranking/window work out of that query and
				// resolve excerpts only for this already-bounded page of hits.
				if err := db.QueryRowContext(ctx, `SELECT snippet(knowledge_fts,4,'[',']','…',18) FROM knowledge_fts WHERE knowledge_fts MATCH ? AND record_id=? AND revision=?`, query.Text, item.id, item.rev).Scan(&record.Excerpt); err != nil {
					return SearchPage{}, errors.New("Ma'at knowledge search excerpt is unavailable")
				}
			}
		}
		page.Items = append(page.Items, record)
	}
	if more {
		page.NextCursor = encodeSearchCursor(searchCursor{Generation: generation, Offset: offset + limit, QueryHash: queryHash})
	}
	return page, nil
}

func normalizedQueryHash(query SearchQuery) string {
	query.Cursor = ""
	query.Limit = 0
	data, _ := json.Marshal(query)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func encodeSearchCursor(cursor searchCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeSearchCursor(value string) (searchCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return searchCursor{}, err
	}
	var cursor searchCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return searchCursor{}, err
	}
	return cursor, nil
}
