package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

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
INSERT INTO items (id, from_agent, to_agent, title, type, repo, status, opened, closed, instructions, result)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    from_agent=excluded.from_agent,
    to_agent=excluded.to_agent,
    title=excluded.title,
    type=excluded.type,
    repo=excluded.repo,
    status=excluded.status,
    opened=excluded.opened,
    closed=excluded.closed,
    instructions=excluded.instructions,
    result=excluded.result;`
	_, err := s.db.Exec(q, it.ID, it.From, it.To, it.Title, it.Type, it.Repo, it.Status, it.Opened, it.Closed, it.Instructions, it.Result)
	if err != nil {
		return fmt.Errorf("routerstore: Put %q: %w", it.ID, err)
	}
	return nil
}

// Send creates a new open item from→to and returns its id. The id follows the
// same convention as internal/work.SendScoped (timestamp-from-to-slug) so the
// store and the filesystem agree on ids; callers that already have a file id
// (e.g. mirroring an existing file) should use Put instead.
//
// This is the durable-dispatch write primitive. It does NOT write any file —
// wiring the file writer and this store behind one facade is PRD Phase 3.
func (s *Store) Send(from, to, title, msgType, repo, instructions string) (string, error) {
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
		Repo:         repo,
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
	const q = `
SELECT id, from_agent, to_agent, title, type, repo, status, opened, closed, instructions, result
FROM items WHERE id = ?;`
	var it Item
	err := s.db.QueryRow(q, id).Scan(
		&it.ID, &it.From, &it.To, &it.Title, &it.Type, &it.Repo,
		&it.Status, &it.Opened, &it.Closed, &it.Instructions, &it.Result,
	)
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
		rows, err = s.db.Query(`
SELECT id, from_agent, to_agent, title, type, repo, status, opened, closed, instructions, result
FROM items WHERE status = 'open' ORDER BY id ASC;`)
	} else {
		rows, err = s.db.Query(`
SELECT id, from_agent, to_agent, title, type, repo, status, opened, closed, instructions, result
FROM items WHERE status = 'open' AND to_agent = ? ORDER BY id ASC;`, agent)
	}
	if err != nil {
		return nil, fmt.Errorf("routerstore: Inbox %q: %w", agent, err)
	}
	defer rows.Close()
	return scanItems(rows)
}

// ListAll returns every item regardless of status, oldest id first.
func (s *Store) ListAll() ([]Item, error) {
	rows, err := s.db.Query(`
SELECT id, from_agent, to_agent, title, type, repo, status, opened, closed, instructions, result
FROM items ORDER BY id ASC;`)
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
func (s *Store) CloseItem(id, result string) error {
	it, err := s.Get(id)
	if err != nil {
		return err
	}
	if it.Status == "closed" {
		return ErrAlreadyClosed
	}
	closedAt := s.clock().Format(time.RFC3339)
	body := strings.TrimSpace(result)
	if body == "" {
		body = "(closed without result)"
	}
	const q = `UPDATE items SET status='closed', closed=?, result=? WHERE id=?;`
	res, err := s.db.Exec(q, closedAt, body, id)
	if err != nil {
		return fmt.Errorf("routerstore: Close %q: %w", id, err)
	}
	// Guard against a concurrent delete between Get and Exec.
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanItems drains a *sql.Rows of item columns into a slice.
func scanItems(rows *sql.Rows) ([]Item, error) {
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(
			&it.ID, &it.From, &it.To, &it.Title, &it.Type, &it.Repo,
			&it.Status, &it.Opened, &it.Closed, &it.Instructions, &it.Result,
		); err != nil {
			return nil, fmt.Errorf("routerstore: scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routerstore: rows: %w", err)
	}
	return items, nil
}

// validateMessageType mirrors internal/work.validateMessageType so the store
// enforces the same message-type vocabulary as the file router.
func validateMessageType(msgType string) error {
	switch strings.TrimSpace(msgType) {
	case "", "proposal", "task", "review", "decision":
		return nil
	default:
		return fmt.Errorf("routerstore: invalid type %q: use proposal, task, review, or decision", msgType)
	}
}
