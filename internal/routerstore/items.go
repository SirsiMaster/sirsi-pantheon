package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// itemCols is the canonical items column list, in the exact order scanItem
// reads them. One definition keeps Put/Get/Inbox/ListAll from drifting;
// TestFieldFidelityWithWorkItem enforces the columns↔Item-fields bijection.
const itemCols = "id, from_agent, to_agent, title, type, status, opened, closed, instructions, result, wake_status, wake_attempted_at, wake_adapter, wake_error"

// validStatus reports whether status is one of the accepted item states.
func validStatus(status string) bool {
	return status == "open" || status == "closed"
}

// Put inserts or replaces (upserts) an item by id. It is the low-level mirror
// primitive used by Backfill to project a markdown item into the index. Callers
// that want the send/close lifecycle should use Send and Close instead.
//
// Put is idempotent on id: re-putting the same id overwrites the row, so a
// backfill can run repeatedly without duplicating rows (PRD Phase 4 importer
// idempotency, applied here at the store layer).
//
// Put deliberately does NOT validate Type against the ADR-024 §5 vocabulary:
// internal/work.SendTyped never rejects unknown type strings, so an existing
// items/*.md may carry one — a mirror that refused it would drop the item and
// violate the zero-data-loss goal (PRD /goal #4). Vocabulary enforcement lives
// on Send, the store's new-item write path, matching the file router.
func (s *Store) Put(it Item) error {
	if strings.TrimSpace(it.ID) == "" {
		return fmt.Errorf("routerstore: Put: id is required")
	}
	if it.Status == "" {
		it.Status = "open"
	}
	if !validStatus(it.Status) {
		return fmt.Errorf("routerstore: Put: invalid status %q", it.Status)
	}
	const q = `
INSERT INTO items (` + itemCols + `)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    from_agent=excluded.from_agent,
    to_agent=excluded.to_agent,
    title=excluded.title,
    type=excluded.type,
    status=excluded.status,
    opened=excluded.opened,
    closed=excluded.closed,
    instructions=excluded.instructions,
    result=excluded.result,
    wake_status=excluded.wake_status,
    wake_attempted_at=excluded.wake_attempted_at,
    wake_adapter=excluded.wake_adapter,
    wake_error=excluded.wake_error;`
	_, err := s.db.Exec(q,
		it.ID, it.From, it.To, it.Title, it.Type, it.Status, it.Opened, it.Closed,
		it.Instructions, it.Result,
		it.WakeStatus, it.WakeAttemptedAt, it.WakeAdapter, it.WakeError,
	)
	if err != nil {
		return fmt.Errorf("routerstore: Put %q: %w", it.ID, err)
	}
	return nil
}

// Send creates a new open item from→to and returns its id. The id follows the
// same convention as internal/work.SendTyped (timestamp-from-to-slug) so the
// store and the filesystem agree on ids; callers that already have a file id
// (e.g. mirroring an existing file) should use Put instead. Wake fields start
// empty, exactly as SendTyped writes no wake_* frontmatter on a new item.
//
// This is the durable-dispatch write primitive. It does NOT write any file —
// wiring the file writer and this store behind one facade is PRD Phase 3.
func (s *Store) Send(from, to, title, msgType, instructions string) (string, error) {
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return "", fmt.Errorf("routerstore: Send: from and to are required")
	}
	if err := validateMessageType(msgType); err != nil {
		return "", err
	}
	now := s.clock()
	id := fmt.Sprintf("%s-%s-%s-%s", now.Format("20060102-150405"), slugify(from), slugify(to), slugify(title))
	it := Item{
		ID:           id,
		From:         from,
		To:           to,
		Title:        title,
		Type:         msgType,
		Status:       "open",
		Opened:       now.Format(time.RFC3339),
		Instructions: strings.TrimSpace(instructions),
	}
	if err := s.Put(it); err != nil {
		return "", err
	}
	return id, nil
}

// Get loads one item by id. It returns ErrNotFound if no such item exists.
func (s *Store) Get(id string) (Item, error) {
	row := s.db.QueryRow(`SELECT `+itemCols+` FROM items WHERE id = ?;`, id)
	it, err := scanItem(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	if err != nil {
		return Item{}, fmt.Errorf("routerstore: Get %q: %w", id, err)
	}
	return it, nil
}

// Inbox returns open items addressed to agent, oldest id first. If agent is
// empty it returns every open item. This is the indexed equivalent of
// internal/work.ListInbox and uses the (to_agent, status) index.
func (s *Store) Inbox(agent string) ([]Item, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if strings.TrimSpace(agent) == "" {
		rows, err = s.db.Query(`SELECT ` + itemCols + ` FROM items WHERE status = 'open' ORDER BY id ASC;`)
	} else {
		rows, err = s.db.Query(`SELECT `+itemCols+` FROM items WHERE status = 'open' AND to_agent = ? ORDER BY id ASC;`, agent)
	}
	if err != nil {
		return nil, fmt.Errorf("routerstore: Inbox %q: %w", agent, err)
	}
	defer rows.Close()
	return scanItems(rows)
}

// ListAll returns every item regardless of status, oldest id first.
func (s *Store) ListAll() ([]Item, error) {
	rows, err := s.db.Query(`SELECT ` + itemCols + ` FROM items ORDER BY id ASC;`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: ListAll: %w", err)
	}
	defer rows.Close()
	return scanItems(rows)
}

// CloseItem marks an item closed and records its result, mirroring
// internal/work.Close. It returns ErrNotFound if the id is unknown and
// ErrAlreadyClosed if the item is already closed. (Named CloseItem, not Close,
// so it does not collide with Store.Close, which releases the DB handle.)
//
// The close is a single atomic UPDATE guarded on status='open' — there is no
// Get→check→UPDATE window, so two concurrent closers race safely: exactly one
// wins, the other gets ErrAlreadyClosed (TestCloseItemConcurrentDoubleClose).
func (s *Store) CloseItem(id, result string) error {
	closedAt := s.clock().Format(time.RFC3339)
	body := strings.TrimSpace(result)
	if body == "" {
		body = "(closed without result)"
	}
	const q = `UPDATE items SET status='closed', closed=?, result=? WHERE id=? AND status='open';`
	res, err := s.db.Exec(q, closedAt, body, id)
	if err != nil {
		return fmt.Errorf("routerstore: Close %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("routerstore: Close %q: rows affected: %w", id, err)
	}
	if n == 1 {
		return nil
	}
	// Zero rows: the guard didn't match. Re-read to disambiguate a missing id
	// (ErrNotFound) from a lost close race (ErrAlreadyClosed).
	it, getErr := s.Get(id)
	if getErr != nil {
		return getErr
	}
	if it.Status == "closed" {
		return ErrAlreadyClosed
	}
	return fmt.Errorf("routerstore: Close %q: item is %q, not open", id, it.Status)
}

// scanItem reads one items row (in itemCols order) via the given scan func,
// shared by the single-row and multi-row readers.
func scanItem(scan func(dest ...any) error) (Item, error) {
	var it Item
	err := scan(
		&it.ID, &it.From, &it.To, &it.Title, &it.Type,
		&it.Status, &it.Opened, &it.Closed, &it.Instructions, &it.Result,
		&it.WakeStatus, &it.WakeAttemptedAt, &it.WakeAdapter, &it.WakeError,
	)
	return it, err
}

// scanItems drains a *sql.Rows of item columns into a slice.
func scanItems(rows *sql.Rows) ([]Item, error) {
	var items []Item
	for rows.Next() {
		it, err := scanItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("routerstore: scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routerstore: rows: %w", err)
	}
	return items, nil
}

// validateMessageType enforces the ADR-024 §5 message-type vocabulary:
// "proposal" | "review" | "decision", or "" for a plain work item. That is
// exactly the vocabulary the file router carries and enforces:
//   - internal/work.Item.Type documents it (internal/work/work.go),
//   - `sirsi router send --type` documents it (cmd/sirsi/routercmd.go), and
//   - the MCP path enforces it hard: router_submit (internal/mcp/tools.go) →
//     internal/router.Submit, whose DocType switch rejects anything else
//     ("unknown doc type").
//
// internal/work.SendTyped itself does not reject unknown strings (de-facto
// laxity at the file layer) — that is why Put/Backfill accept any type (see
// Put), while Send, the store's NEW-item path, holds the documented line.
func validateMessageType(msgType string) error {
	switch strings.TrimSpace(msgType) {
	case "", "proposal", "review", "decision":
		return nil
	default:
		return fmt.Errorf("routerstore: invalid type %q: ADR-024 §5 allows proposal, review, or decision", msgType)
	}
}
