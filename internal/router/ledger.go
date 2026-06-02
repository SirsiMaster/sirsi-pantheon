package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DispatchLedger persists delivered router notifications so daemon restarts
// do not relaunch the same unchanged inbox item.
type DispatchLedger struct {
	path  string
	Items map[string]string `json:"items"`
}

// LoadDispatchLedger opens an existing ledger or creates an empty in-memory
// ledger when the file does not exist yet.
func LoadDispatchLedger(path string) (*DispatchLedger, error) {
	ledger := &DispatchLedger{
		path:  path,
		Items: map[string]string{},
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if uErr := json.Unmarshal(data, ledger); uErr != nil {
			return nil, fmt.Errorf("parse dispatch ledger: %w", uErr)
		}
		if ledger.Items == nil {
			ledger.Items = map[string]string{}
		}
		ledger.path = path
		return ledger, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return ledger, nil
	}
	return nil, fmt.Errorf("read dispatch ledger: %w", err)
}

// WasDispatched reports whether key has already been delivered for fingerprint.
func (l *DispatchLedger) WasDispatched(key, fingerprint string) bool {
	if l == nil {
		return false
	}
	return l.Items[key] == fingerprint
}

// MarkDispatched records and persists a successful dispatch.
func (l *DispatchLedger) MarkDispatched(key, fingerprint string) error {
	if l == nil {
		return nil
	}
	l.Items[key] = fingerprint
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dispatch ledger: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return fmt.Errorf("create dispatch ledger dir: %w", err)
	}
	if err := os.WriteFile(l.path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write dispatch ledger: %w", err)
	}
	return nil
}
