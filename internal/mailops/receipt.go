package mailops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Receipt records one archive batch for a signed, replayable audit trail.
// Mirrors internal/cleaner.DecisionLog's shape (Rule A7/A11): local-only,
// never transmitted, one file per session.
type Receipt struct {
	Reason    string    `json:"reason"`
	IDs       []string  `json:"ids"`
	Timestamp time.Time `json:"timestamp"`
}

// ReceiptLog persists mailops archive batches under
// ~/.sirsi/mailops/receipts/. Local-only: never uploaded (Rule A11).
type ReceiptLog struct {
	path string
}

// NewReceiptLog opens today's receipt log for the given account label.
func NewReceiptLog(label string) (*ReceiptLog, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".sirsi", "mailops", "receipts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("mailops: create receipt dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl", label, time.Now().Format("20060102-150405")))
	return &ReceiptLog{path: path}, nil
}

// RecordArchive appends one receipt line for an archive batch.
func (l *ReceiptLog) RecordArchive(reason string, ids []string) error {
	r := Receipt{Reason: reason, IDs: ids, Timestamp: time.Now()}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("mailops: open receipt log: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(r)
}

// Path returns the receipt file location, for CLI reporting.
func (l *ReceiptLog) Path() string { return l.path }
