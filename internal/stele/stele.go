// Package stele implements The Stele — an append-only hash-chained event ledger
// for Pantheon's governance loop (ADR-014).
//
// The Stele is the single source of truth for all Pantheon inter-deity communication.
// Every deity appends signed entries. Every consumer mmap's the file and reads from
// its tracked offset. On Apple Silicon unified memory, this is zero-copy.
//
// Write: stele.Append(deity, eventType, scope, data)
// Read:  reader := stele.NewReader("command-center"); entries := reader.ReadNew()
// Verify: stele.Verify(path) — walks the hash chain and reports breaks
package stele

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// maxEntryBytes bounds a single ledger line when scanning. Entries carry small
// string maps, so this is far above any real record and only guards against a
// corrupt file scanning into memory.
const maxEntryBytes = 1 << 20

// Entry is a single record in the Stele ledger.
type Entry struct {
	Seq   int64             `json:"seq"`
	Prev  string            `json:"prev"`  // SHA-256 of previous entry
	Deity string            `json:"deity"` // e.g. "ra", "maat", "agent:assiduous"
	Type  string            `json:"type"`  // event type
	Scope string            `json:"scope"` // repo scope (empty for global events)
	Data  map[string]string `json:"data"`  // event-specific payload
	TS    string            `json:"ts"`    // ISO-8601
	Hash  string            `json:"hash"`  // SHA-256 of this entry (computed on write)
}

// Event types — Ra orchestration.
const (
	TypeDeployStart = "deploy_start"
	TypeDeployEnd   = "deploy_end"
	TypeSprintStart = "sprint_start"
	TypeSprintEnd   = "sprint_end"
	TypeToolUse     = "tool_use"
	TypeText        = "text"
	TypeGovernance  = "governance"
	TypeCommit      = "commit"
	TypeCompact     = "compact"
	TypeScribe      = "scribe"
	TypeDriftCheck  = "drift_check"
	TypeScopeWeave  = "scope_weave"
	TypeComplete    = "complete"
	TypeFailed      = "failed"
)

// Event types — deity operations.
const (
	// Thoth — memory & persistence
	TypeThothSync    = "thoth_sync"
	TypeThothCompact = "thoth_compact"
	TypeThothPrune   = "thoth_prune"

	// Ma'at — quality & governance
	TypeMaatWeigh = "maat_weigh"
	TypeMaatPulse = "maat_pulse"
	TypeMaatAudit = "maat_audit"
	TypeMaatHeal  = "maat_heal"

	// Seshat — knowledge grafting
	TypeSeshatIngest = "seshat_ingest"
	TypeSeshatExport = "seshat_export"

	// Neith — scope weaving
	TypeNeithWeave = "neith_weave"
	TypeNeithDrift = "neith_drift"

	// Ka — ghost detection
	TypeKaHunt  = "ka_hunt"
	TypeKaClean = "ka_clean"

	// Anubis — scanning & cleanup
	TypeAnubisScan  = "anubis_scan"
	TypeAnubisJudge = "anubis_judge"
	TypeAnubisClean = "anubis_clean"

	// Isis — health, guard, remediation
	TypeGuardStart = "guard_start"
	TypeGuardAlert = "guard_alert"
	TypeGuardStop  = "guard_stop"

	// Seba — architecture mapping
	TypeSebaMap    = "seba_map"
	TypeSebaRender = "seba_render"

	// Hapi — hardware detection
	TypeHapiDetect = "hapi_detect"

	// RTK — output filtering
	TypeRTKFilter = "rtk_filter"

	// Vault — context sandbox
	TypeVaultStore      = "vault_store"
	TypeVaultSearch     = "vault_search"
	TypeVaultPrune      = "vault_prune"
	TypeVaultIndex      = "vault_index"
	TypeVaultCodeSearch = "vault_code_search"

	// Horus — structural code graph
	TypeHorusScan  = "horus_scan"
	TypeHorusQuery = "horus_query"
	// TypeThreadReap records a stray/superseded thread record retired by the
	// ADR-024 supersession reaper, carrying any salvaged continuation state so
	// nothing is lost when the ghost is swept.
	TypeThreadReap = "thread_reap"

	// Notify — notification system
	TypeNotify      = "notify"
	TypeNotifyPrune = "notify_prune"
)

// Ledger manages writes to the Stele file.
type Ledger struct {
	mu       sync.Mutex
	path     string
	seq      int64
	prevHash string
}

// global is the singleton ledger for Inscribe().
var (
	globalMu     sync.Mutex
	globalLedger *Ledger
)

// Inscribe is the convenience entry point for any deity to write to the Stele.
// It lazily opens a global ledger on first call. Thread-safe.
//
//	stele.Inscribe("thoth", stele.TypeThothSync, "", map[string]string{"facts": "12"})
func Inscribe(deity, eventType, scope string, data map[string]string) {
	globalMu.Lock()
	if globalLedger == nil {
		l, err := Open(DefaultPath())
		if err != nil {
			globalMu.Unlock()
			return // best-effort — never block a deity on ledger failure
		}
		globalLedger = l
	}
	globalMu.Unlock()
	_ = globalLedger.Append(deity, eventType, scope, data)
}

// DefaultPath returns the default Stele path.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ra", "stele.jsonl")
}

const openTailBytes int64 = 1 << 20

// Open opens or creates the Stele ledger at the given path.
// It reads only a bounded tail to initialize the hash chain. The previous
// implementation read, copied, and split the entire lifetime JSONL ledger;
// a long-lived workstation expanded that history into hundreds of megabytes
// every time a resident Pantheon process inscribed its first event.
func Open(path string) (*Ledger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("stele: create dir: %w", err)
	}

	l := &Ledger{
		path:     path,
		prevHash: strings.Repeat("0", 64), // genesis
	}

	last, found, err := readLastEntry(path)
	if err != nil {
		return nil, err
	}
	if found {
		l.seq = last.Seq + 1
		l.prevHash = last.Hash
	}

	return l, nil
}

func readLastEntry(path string) (Entry, bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("stele: open tail: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Entry{}, false, fmt.Errorf("stele: stat tail: %w", err)
	}
	if info.Size() == 0 {
		return Entry{}, false, nil
	}
	start := info.Size() - openTailBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return Entry{}, false, fmt.Errorf("stele: read tail: %w", err)
	}
	if start > 0 {
		if cut := bytes.IndexByte(buf, '\n'); cut >= 0 {
			buf = buf[cut+1:]
		} else {
			return Entry{}, false, errors.New("stele: final entry exceeds bounded tail")
		}
	}
	lines := bytes.Split(bytes.TrimSpace(buf), []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}
		var last Entry
		if err := json.Unmarshal(line, &last); err == nil {
			return last, true, nil
		}
	}
	return Entry{}, false, errors.New("stele: no valid entry in bounded tail")
}

// Append writes a new hash-chained entry to the Stele.
func (l *Ledger) Append(deity, eventType, scope string, data map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := Entry{
		Seq:   l.seq,
		Prev:  l.prevHash,
		Deity: deity,
		Type:  eventType,
		Scope: scope,
		Data:  data,
		TS:    time.Now().Format(time.RFC3339),
	}

	// Compute hash of entry (without the Hash field)
	entry.Hash = computeHash(entry)

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("stele: marshal entry: %w", err)
	}
	line = append(line, '\n')

	// Atomic append (O_APPEND is atomic on macOS for writes <= PIPE_BUF)
	f, err := os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("stele: open for append: %w", err)
	}
	_, err = f.Write(line)
	f.Close()
	if err != nil {
		return fmt.Errorf("stele: write entry: %w", err)
	}

	l.prevHash = entry.Hash
	l.seq++
	return nil
}

// computeHash returns the SHA-256 hex digest of an entry (with Hash field zeroed).
func computeHash(e Entry) string {
	// Zero the hash field before computing
	e.Hash = ""
	data, _ := json.Marshal(e)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TailByType returns entries of the given type found in the last maxBytes of
// the ledger, oldest first. The Stele is append-only and unbounded (it passed
// 170MB in July 2026), so a surface that only needs recent activity reads a
// window rather than the whole file — a Reader would consume the offset a real
// consumer depends on.
//
// ponytail: fixed byte window, not an index. A caller needing lifetime totals
// needs a real aggregate, not a bigger window.
func TailByType(eventType string, maxBytes int64) ([]Entry, error) {
	return TailByTypeFrom(DefaultPath(), eventType, maxBytes)
}

// TailByTypeFrom is TailByType against an explicit ledger path (Rule A16).
func TailByTypeFrom(path, eventType string, maxBytes int64) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("stele: open ledger: %w", err)
	}
	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stele: stat ledger: %w", err)
	}
	start := max(st.Size()-maxBytes, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("stele: seek ledger: %w", err)
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxEntryBytes)
	if start > 0 {
		sc.Scan() // discard the partial line the seek landed mid-way through
	}
	var out []Entry
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue // a torn or future-schema line is skipped, never fatal
		}
		if e.Type == eventType {
			out = append(out, e)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("stele: read ledger: %w", err)
	}
	return out, nil
}

// Reader reads new entries from the Stele with offset tracking.
type Reader struct {
	stelePath  string
	offsetPath string
	offset     int64
}

// NewReader creates a reader for the given consumer.
// The consumer's read offset is persisted in ~/.config/ra/offsets/<name>.offset.
func NewReader(consumerName string) *Reader {
	home, _ := os.UserHomeDir()
	stelePath := filepath.Join(home, ".config", "ra", "stele.jsonl")
	offsetDir := filepath.Join(home, ".config", "ra", "offsets")
	_ = os.MkdirAll(offsetDir, 0755)
	offsetPath := filepath.Join(offsetDir, consumerName+".offset")

	r := &Reader{
		stelePath:  stelePath,
		offsetPath: offsetPath,
	}

	// Load saved offset
	if data, err := os.ReadFile(offsetPath); err == nil {
		r.offset, _ = strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	}

	return r
}

// ReadNew reads all entries appended since the last read.
func (r *Reader) ReadNew() ([]Entry, error) {
	f, err := os.Open(r.stelePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stele reader: open: %w", err)
	}
	defer f.Close()

	// Get file size
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stele reader: stat: %w", err)
	}

	if info.Size() <= r.offset {
		return nil, nil // nothing new
	}

	// Seek to last position
	if r.offset > 0 {
		if _, err := f.Seek(r.offset, 0); err != nil {
			r.offset = 0
			_, _ = f.Seek(0, 0)
		}
	}

	// Read new bytes
	buf := make([]byte, info.Size()-r.offset)
	n, _ := f.Read(buf)
	if n == 0 {
		return nil, nil
	}

	r.offset += int64(n)

	// Parse entries
	var entries []Entry
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			entries = append(entries, e)
		}
	}

	// Save offset
	r.saveOffset()

	return entries, nil
}

// saveOffset persists the current read position.
func (r *Reader) saveOffset() {
	_ = os.WriteFile(r.offsetPath, []byte(strconv.FormatInt(r.offset, 10)), 0644)
}

// Reset resets the reader to the beginning of the Stele.
func (r *Reader) Reset() {
	r.offset = 0
	r.saveOffset()
}

// Verify walks the entire Stele and checks the hash chain.
// Returns the number of valid entries and any chain break errors.
func Verify(path string) (int, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, []error{fmt.Errorf("stele verify: read: %w", err)}
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var errs []error
	valid := 0
	prevHash := strings.Repeat("0", 64)

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			errs = append(errs, fmt.Errorf("entry %d: invalid JSON: %w", i, err))
			continue
		}

		// Verify prev hash chain
		if e.Prev != prevHash {
			errs = append(errs, fmt.Errorf("entry %d (seq %d): chain break — prev=%s expected=%s",
				i, e.Seq, e.Prev[:16]+"...", prevHash[:16]+"..."))
		}

		// Verify entry hash
		computed := computeHash(e)
		if e.Hash != computed {
			errs = append(errs, fmt.Errorf("entry %d (seq %d): hash mismatch — stored=%s computed=%s",
				i, e.Seq, e.Hash[:16]+"...", computed[:16]+"..."))
		} else {
			prevHash = e.Hash
			valid++
		}
	}

	return valid, errs
}

// Clear removes the Stele file and all consumer offsets. Used before a fresh deploy.
func Clear() error {
	home, _ := os.UserHomeDir()
	raDir := filepath.Join(home, ".config", "ra")

	// Remove stele
	os.Remove(filepath.Join(raDir, "stele.jsonl"))

	// Remove offsets
	offsetDir := filepath.Join(raDir, "offsets")
	entries, _ := os.ReadDir(offsetDir)
	for _, e := range entries {
		os.Remove(filepath.Join(offsetDir, e.Name()))
	}

	return nil
}
