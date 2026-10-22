// Package authority implements the pod-local Ma'at booking authority
// (ADR-120 step 2): the process a live delegation names as authority_machine.
// It runs ordinary schedule.Ledger admission against its own local store,
// journals every grant it makes, and write-throughs to the canonical (cloud)
// ledger when reachable -- the journal, catch-up and self-fence described in
// ADR-120 §2-5.
package authority

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// Entry is one journaled grant: a reservation stamped with the monotonic Seq
// it was appended under. Catch-up replays Seq > acked_seq in order as
// idempotent upserts (ADR-120 §5).
type Entry struct {
	Seq         int64                `json:"seq"`
	Reservation schedule.Reservation `json:"reservation"`
}

// Journal is the pod authority's local append-only log. Append must not
// return until the entry is durable: the authority answers its own caller
// only after this call returns (ADR-120 §5: "fsynced before the caller gets
// its answer").
type Journal interface {
	// Append assigns the next seq, persists the entry durably, and returns it.
	Append(r schedule.Reservation) (Entry, error)
	// Since returns every entry with Seq > after, in order.
	Since(after int64) ([]Entry, error)
	// Head returns the highest seq appended so far (0 if empty).
	Head() (int64, error)
}

// memJournal is an in-memory Journal for tests; no fsync, no durability.
type memJournal struct {
	mu      sync.Mutex
	entries []Entry
}

// NewMemJournal returns a Journal with no durability, for tests only.
func NewMemJournal() Journal { return &memJournal{} }

func (j *memJournal) Append(r schedule.Reservation) (Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e := Entry{Seq: int64(len(j.entries)) + 1, Reservation: r}
	j.entries = append(j.entries, e)
	return e, nil
}

func (j *memJournal) Since(after int64) ([]Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []Entry
	for _, e := range j.entries {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

func (j *memJournal) Head() (int64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return int64(len(j.entries)), nil
}

// FileJournal is a durable, fsynced, append-only JSON-lines journal: one
// Entry per line, Seq assigned as the running line count. Append fsyncs
// before returning, so a crash never loses an acknowledged grant.
type FileJournal struct {
	mu   sync.Mutex
	path string
}

// OpenFileJournal opens (creating if absent) a durable journal at path.
func OpenFileJournal(path string) (*FileJournal, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("maat: open journal %s: %w", path, err)
	}
	f.Close()
	return &FileJournal{path: path}, nil
}

func (j *FileJournal) Append(r schedule.Reservation) (Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	head, err := j.headLocked()
	if err != nil {
		return Entry{}, err
	}
	e := Entry{Seq: head + 1, Reservation: r}
	line, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}
	f, err := os.OpenFile(j.path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return Entry{}, fmt.Errorf("maat: open journal %s: %w", j.path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return Entry{}, fmt.Errorf("maat: write journal %s: %w", j.path, err)
	}
	if err := f.Sync(); err != nil {
		return Entry{}, fmt.Errorf("maat: fsync journal %s: %w", j.path, err)
	}
	return e, nil
}

func (j *FileJournal) Since(after int64) ([]Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	all, err := j.readAllLocked()
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, e := range all {
		if e.Seq > after {
			out = append(out, e)
		}
	}
	return out, nil
}

func (j *FileJournal) Head() (int64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.headLocked()
}

func (j *FileJournal) headLocked() (int64, error) {
	all, err := j.readAllLocked()
	if err != nil {
		return 0, err
	}
	if len(all) == 0 {
		return 0, nil
	}
	return all[len(all)-1].Seq, nil
}

func (j *FileJournal) readAllLocked() ([]Entry, error) {
	f, err := os.Open(j.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("maat: read journal %s: %w", j.path, err)
	}
	defer f.Close()
	var out []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("maat: parse journal %s: %w", j.path, err)
		}
		out = append(out, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("maat: scan journal %s: %w", j.path, err)
	}
	return out, nil
}
