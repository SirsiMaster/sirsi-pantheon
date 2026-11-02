//go:build darwin

package trashreceipt

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const (
	operationEventSchema = "pantheon.trash-operation-event.v1"
	maxOperationBytes    = 1 << 20
	maxHistoryPage       = 20
	defaultHistoryPage   = 20
	maxHistoryLineBytes  = 1024
	maxHistoryScanBytes  = 64 << 10
)

// The maximum event volume is below 1 MiB: a <=64 KiB reviewed manifest,
// at most two small index-only events per 1,024 entries, one bounded error,
// and a terminal Report whose deleted paths are already bounded by that
// manifest. The cap is therefore above the valid full-batch worst case.

type operationEvent struct {
	Schema      string    `json:"schema"`
	OperationID string    `json:"operation_id"`
	Kind        string    `json:"kind"`
	At          time.Time `json:"at"`
	Manifest    *Manifest `json:"manifest,omitempty"`
	ManifestSHA string    `json:"manifest_sha256"`
	EntryIndex  int       `json:"entry_index"`
	State       string    `json:"state,omitempty"`
	Error       string    `json:"error,omitempty"`
	Report      *Report   `json:"report,omitempty"`
}

type operationJournal interface {
	OperationID() string
	AttemptStarted(int) error
	AttemptResult(int, string, string) error
	Finish(Report) error
	Close() error
}

type durableJournal struct {
	rootFD      int
	fileFD      int
	id          string
	fault       func(string) error
	manifestSHA string
	poisoned    bool
	writeFn     func(int, []byte) (int, error)
	syncFn      func(int) error
}

func newOperationJournalForHome(home string, m Manifest) (*durableJournal, error) {
	rootFD, err := openReceiptDirectory(home, true)
	if err != nil {
		return nil, err
	}
	return newOperationJournalAtFD(rootFD, m, nil)
}

// newOperationJournalAt exists for inert filesystem fixtures; production
// resolves the fixed owner-local directory through openReceiptDirectory.
func newOperationJournalAt(root string, m Manifest, fault func(string) error) (*durableJournal, error) {
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := verifyReceiptDir(rootFD); err != nil {
		_ = unix.Close(rootFD)
		return nil, err
	}
	return newOperationJournalAtFD(rootFD, m, fault)
}

func newOperationJournalAtFD(rootFD int, m Manifest, fault func(string) error) (*durableJournal, error) {
	var journal *durableJournal
	transferred := false
	defer func() {
		if !transferred {
			if journal != nil {
				_ = journal.Close()
			} else {
				_ = unix.Close(rootFD)
			}
		}
	}()
	if err := Validate(m); err != nil {
		return nil, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, fmt.Errorf("create Trash receipt identity: %w", err)
	}
	id := fmt.Sprintf("%x", random[:])
	digest, err := manifestDigest(m)
	if err != nil {
		return nil, err
	}
	started := time.Now().UTC()
	// Index first: even an intent-file creation failure remains visible as an
	// unknown row. No unlink occurs until both this row and the full intent are
	// durable.
	if err := appendHistoryIndex(rootFD, id, started, digest, fault); err != nil {
		return nil, fmt.Errorf("index Trash receipt before deletion: %w", err)
	}
	fd, err := unix.Openat(rootFD, id+".jsonl", unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create Trash receipt: %w", err)
	}
	if err := verifyReceiptFile(fd); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	journal = &durableJournal{rootFD: rootFD, fileFD: fd, id: id, fault: fault, manifestSHA: digest}
	manifest := m
	if err := journal.append(operationEvent{Schema: operationEventSchema, OperationID: id, Kind: "intent", At: started, Manifest: &manifest}, "intent"); err != nil {
		return nil, fmt.Errorf("persist Trash intent before deletion: %w", err)
	}
	if err := unix.Fsync(rootFD); err != nil {
		return nil, fmt.Errorf("persist Trash intent directory entry before deletion: %w", err)
	}
	transferred = true
	return journal, nil
}

func (j *durableJournal) OperationID() string { return j.id }

func (j *durableJournal) AttemptStarted(index int) error {
	return j.append(operationEvent{Schema: operationEventSchema, OperationID: j.id, Kind: "attempt_started", At: time.Now().UTC(), EntryIndex: index}, "attempt-start")
}

func (j *durableJournal) AttemptResult(index int, state, message string) error {
	return j.append(operationEvent{Schema: operationEventSchema, OperationID: j.id, Kind: "attempt_result", At: time.Now().UTC(), EntryIndex: index, State: state, Error: boundedJournalError(message)}, "attempt-result")
}

func (j *durableJournal) Finish(r Report) error {
	r.OperationID = j.id
	return j.append(operationEvent{Schema: operationEventSchema, OperationID: j.id, Kind: "terminal", At: time.Now().UTC(), Report: &r}, "terminal")
}

func (j *durableJournal) append(e operationEvent, stage string) error {
	if j == nil || j.fileFD < 0 {
		return errors.New("Trash receipt journal is closed")
	}
	if j.poisoned {
		return errors.New("Trash receipt writer is poisoned after an earlier persistence error")
	}
	if j.fault != nil {
		if err := j.fault(stage); err != nil {
			j.poisoned = true
			return err
		}
	}
	e.ManifestSHA = j.manifestSHA
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	var st unix.Stat_t
	if err := unix.Fstat(j.fileFD, &st); err != nil {
		j.poisoned = true
		return err
	}
	if st.Size+int64(len(data)) > maxOperationBytes {
		j.poisoned = true
		return errors.New("Trash receipt exceeds its 1 MiB bound")
	}
	write := j.writeFn
	if write == nil {
		write = unix.Write
	}
	if err := writeAllWith(j.fileFD, data, write); err != nil {
		j.poisoned = true
		return err
	}
	sync := j.syncFn
	if sync == nil {
		sync = unix.Fsync
	}
	if err := sync(j.fileFD); err != nil {
		j.poisoned = true
		return err
	}
	return nil
}

func (j *durableJournal) Close() error {
	if j == nil {
		return nil
	}
	var first error
	if j.fault != nil {
		first = j.fault("close")
	}
	if j.fileFD >= 0 {
		if err := unix.Close(j.fileFD); first == nil && err != nil {
			first = err
		}
		j.fileFD = -1
	}
	if j.rootFD >= 0 {
		if err := unix.Close(j.rootFD); err != nil && first == nil {
			first = err
		}
		j.rootFD = -1
	}
	return first
}

func appendHistoryIndex(rootFD int, id string, at time.Time, digest string, fault func(string) error) error {
	fd, err := unix.Openat(rootFD, "history-index.jsonl", unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err := verifyReceiptFile(fd); err != nil {
		return err
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if fault != nil {
		if err := fault("index"); err != nil {
			return err
		}
	}
	row, err := json.Marshal(historyIndexRow{OperationID: id, StartedAt: at.UTC().Format(time.RFC3339Nano), ManifestSHA: digest})
	if err != nil {
		return err
	}
	row = append(row, '\n')
	if err := writeAll(fd, row); err != nil {
		return err
	}
	if err := unix.Fsync(fd); err != nil {
		return err
	}
	return unix.Fsync(rootFD)
}

func writeAll(fd int, p []byte) error {
	return writeAllWith(fd, p, unix.Write)
}

func writeAllWith(fd int, p []byte, write func(int, []byte) (int, error)) error {
	for len(p) > 0 {
		n, err := write(fd, p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func boundedJournalError(s string) string {
	if len(s) > 1024 {
		s = s[:1024]
		for !utf8Valid(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

func utf8Valid(s string) bool { return json.Valid([]byte(strconv.Quote(s))) }

func openReceiptDirectory(home string, create bool) (int, error) {
	homeFD, err := unix.Open(home, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open owner home for Trash receipts: %w", err)
	}
	defer unix.Close(homeFD)
	sirsiFD, err := unix.Openat(homeFD, ".sirsi", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if err = unix.Mkdirat(homeFD, ".sirsi", 0o700); err == nil || errors.Is(err, unix.EEXIST) {
			if syncErr := unix.Fsync(homeFD); syncErr != nil {
				return -1, syncErr
			}
			sirsiFD, err = unix.Openat(homeFD, ".sirsi", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
	}
	if err != nil {
		return -1, fmt.Errorf("open private Sirsi state for Trash receipts: %w", err)
	}
	defer unix.Close(sirsiFD)
	if err := verifyReceiptParent(sirsiFD); err != nil {
		return -1, err
	}
	rootFD, err := unix.Openat(sirsiFD, "trash-receipts", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if err = unix.Mkdirat(sirsiFD, "trash-receipts", 0o700); err == nil || errors.Is(err, unix.EEXIST) {
			if syncErr := unix.Fsync(sirsiFD); syncErr != nil {
				return -1, syncErr
			}
			rootFD, err = unix.Openat(sirsiFD, "trash-receipts", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		}
	}
	if err != nil {
		return -1, fmt.Errorf("open Trash receipt directory: %w", err)
	}
	if err := verifyReceiptDir(rootFD); err != nil {
		_ = unix.Close(rootFD)
		return -1, err
	}
	return rootFD, nil
}

func verifyReceiptParent(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0o022 != 0 {
		return errors.New(".sirsi must be an owner-controlled directory without group/world write access")
	}
	return nil
}

func verifyReceiptDir(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 {
		return errors.New("Trash receipt directory must be private and owner-owned")
	}
	return nil
}

func verifyReceiptFile(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Geteuid()) || st.Mode&0o077 != 0 {
		return errors.New("Trash receipt file must be private, owner-owned and singly linked")
	}
	return nil
}

type historyIndexRow struct {
	OperationID string `json:"operation_id"`
	StartedAt   string `json:"started_at"`
	ManifestSHA string `json:"manifest_sha256"`
}

type historyCursor struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Offset int64  `json:"offset"`
}

func ReadHistoryPage(limit int, cursor string) HistoryPage {
	page := HistoryPage{Schema: HistoryPageSchema, Items: []HistoryItem{}, Issues: []string{}}
	home, err := os.UserHomeDir()
	if err != nil {
		page.Incomplete = true
		page.Issues = append(page.Issues, "owner home unavailable")
		return page
	}
	rootFD, err := openReceiptDirectory(home, false)
	if err != nil {
		if !errors.Is(err, unix.ENOENT) {
			page.Incomplete = true
			page.Issues = append(page.Issues, boundedJournalError(err.Error()))
		}
		return page
	}
	defer unix.Close(rootFD)
	return historyPageAtFD(rootFD, limit, cursor)
}

func historyPageAtFD(rootFD, limit int, cursor string) HistoryPage {
	page := HistoryPage{Schema: HistoryPageSchema, Items: []HistoryItem{}, Issues: []string{}}
	if limit <= 0 {
		limit = defaultHistoryPage
	}
	if limit > maxHistoryPage {
		limit = maxHistoryPage
	}
	fd, err := unix.Openat(rootFD, "history-index.jsonl", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return page
		}
		page.Incomplete = true
		if !errors.Is(err, unix.ENOENT) {
			page.Issues = append(page.Issues, boundedJournalError(err.Error()))
		}
		return page
	}
	file := os.NewFile(uintptr(fd), "trash-history-index")
	defer file.Close()
	if err := verifyReceiptFile(fd); err != nil {
		page.Incomplete = true
		page.Issues = append(page.Issues, boundedJournalError(err.Error()))
		return page
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		page.Incomplete = true
		page.Issues = append(page.Issues, boundedJournalError(err.Error()))
		return page
	}
	position := st.Size
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		var c historyCursor
		if err != nil || decodeExact(decoded, &c) != nil || c.Device != uint64(st.Dev) || c.Inode != st.Ino || c.Offset < 0 || c.Offset > st.Size {
			page.Incomplete = true
			page.Issues = append(page.Issues, "history cursor is invalid or stale; refresh history")
			return page
		}
		position = c.Offset
		if position > 0 {
			var previous [1]byte
			if n, err := unix.Pread(fd, previous[:], position-1); err != nil || n != 1 || previous[0] != '\n' {
				page.Incomplete = true
				page.Issues = append(page.Issues, "history cursor is not at a record boundary; refresh history")
				return page
			}
		}
	}
	var scanned int64
	stalled := false
	for len(page.Items) < limit && scanned < maxHistoryScanBytes {
		rowEnd := position
		line, start, consumed, terminated, readErr := historyRowBefore(fd, position)
		if readErr == io.EOF {
			break
		}
		position = start
		scanned += int64(consumed)
		if readErr != nil {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, "history index row is invalid or exceeds its read bound")
			if errors.Is(readErr, errHistoryLineTooLong) && consumed > 0 && start < rowEnd {
				continue
			}
			stalled = true
			break
		}
		if !terminated {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, "history index ends with an incomplete row")
			continue
		}
		var row historyIndexRow
		if err := decodeExact(line, &row); err != nil || !validOperationID(row.OperationID) || !validTimestamp(row.StartedAt) || !validManifestDigest(row.ManifestSHA) {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, "history index contains an invalid record")
			continue
		}
		detail := historyDetailAtFD(rootFD, row.OperationID)
		if detail.manifestSHA != "" && detail.manifestSHA != row.ManifestSHA {
			detail.State = "unknown"
			detail.TerminalReport = nil
			detail.IntegrityError = "history index manifest digest differs from operation record"
		}
		if detail.StartedAt != "" && detail.StartedAt != row.StartedAt {
			detail.State = "unknown"
			detail.TerminalReport = nil
			detail.IntegrityError = "history index start time differs from operation record"
		}
		if detail.StartedAt == "" {
			detail.StartedAt = row.StartedAt
		}
		item := historyItem(detail)
		if detail.IntegrityError != "" {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, row.OperationID+": "+detail.IntegrityError)
		}
		page.Items = append(page.Items, item)
	}
	if scanned >= maxHistoryScanBytes && position > 0 {
		page.Incomplete = true
		page.Issues = appendIssue(page.Issues, "history page scan reached its 64 KiB bound")
	}
	if position > 0 && !stalled {
		page.NextCursor = encodeHistoryCursor(historyCursor{Device: uint64(st.Dev), Inode: st.Ino, Offset: position})
	}
	return page
}

var errHistoryLineTooLong = errors.New("history index row exceeds its bound")

func historyRowBefore(fd int, end int64) (line []byte, start int64, consumed int, terminated bool, err error) {
	if end <= 0 {
		return nil, 0, 0, true, io.EOF
	}
	contentEnd := end
	var last [1]byte
	if n, readErr := unix.Pread(fd, last[:], end-1); readErr != nil || n != 1 {
		return nil, end, 0, false, io.ErrUnexpectedEOF
	} else if last[0] == '\n' {
		terminated = true
		contentEnd--
	}
	searchStart := contentEnd - maxHistoryScanBytes
	if searchStart < 0 {
		searchStart = 0
	}
	for chunkEnd := contentEnd; chunkEnd > searchStart; {
		chunkStart := chunkEnd - 4096
		if chunkStart < searchStart {
			chunkStart = searchStart
		}
		chunk := make([]byte, int(chunkEnd-chunkStart))
		n, readErr := unix.Pread(fd, chunk, chunkStart)
		if readErr != nil || n != len(chunk) {
			return nil, end, 0, terminated, io.ErrUnexpectedEOF
		}
		for index := len(chunk) - 1; index >= 0; index-- {
			if chunk[index] == '\n' {
				start = chunkStart + int64(index) + 1
				goto found
			}
		}
		chunkEnd = chunkStart
	}
	if searchStart > 0 {
		return nil, end, maxHistoryScanBytes, terminated, errHistoryLineTooLong
	}
	start = 0
found:
	consumed = int(end - start)
	lineEnd := contentEnd
	if !terminated && end == contentEnd {
		// The final row has no newline; expose it as debt, then continue to the
		// preceding newline so older verified rows remain reachable.
		lineEnd = contentEnd
	}
	length := lineEnd - start
	if length < 0 || length > maxHistoryLineBytes {
		return nil, start, consumed, terminated, errHistoryLineTooLong
	}
	line = make([]byte, int(length))
	if len(line) > 0 {
		n, readErr := unix.Pread(fd, line, start)
		if readErr != nil || n != len(line) {
			return nil, start, consumed, terminated, io.ErrUnexpectedEOF
		}
	}
	return line, start, consumed, terminated, nil
}

func appendIssue(issues []string, issue string) []string {
	if len(issues) < 32 {
		issues = append(issues, boundedJournalError(issue))
	}
	return issues
}

func encodeHistoryCursor(c historyCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func historyItem(d HistoryDetail) HistoryItem {
	i := HistoryItem{OperationID: d.OperationID, StartedAt: d.StartedAt, State: d.State, ErrorsCount: 0}
	i.ManifestEntryCount = len(d.ReviewedManifest.Entries)
	for _, a := range d.Attempts {
		switch a.State {
		case "outcome_unknown", "unlink_succeeded", "unlink_failed":
			i.Attempted++
		}
		if a.State == "unlink_succeeded" {
			i.Deleted++
		}
	}
	if d.TerminalReport != nil {
		i.ErrorsCount += len(d.TerminalReport.Errors)
	} else {
		for _, a := range d.Attempts {
			if a.State == "unlink_failed" && a.Error != "" {
				i.ErrorsCount++
			}
		}
	}
	return i
}

func ReadHistoryDetail(operationID string) HistoryDetail {
	result := HistoryDetail{Schema: HistoryDetailSchema, OperationID: operationID, State: "unknown", Attempts: []HistoryAttempt{}}
	if !validOperationID(operationID) {
		result.IntegrityError = "operation identity is invalid"
		return result
	}
	home, err := os.UserHomeDir()
	if err != nil {
		result.IntegrityError = "owner home unavailable"
		return result
	}
	rootFD, err := openReceiptDirectory(home, false)
	if err != nil {
		result.IntegrityError = boundedJournalError(err.Error())
		return result
	}
	defer unix.Close(rootFD)
	return historyDetailAtFD(rootFD, operationID)
}

func historyDetailAtFD(rootFD int, id string) HistoryDetail {
	d := HistoryDetail{Schema: HistoryDetailSchema, OperationID: id, State: "unknown", Attempts: []HistoryAttempt{}}
	fd, err := unix.Openat(rootFD, id+".jsonl", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		d.IntegrityError = boundedJournalError(err.Error())
		return d
	}
	file := os.NewFile(uintptr(fd), "trash-operation")
	defer file.Close()
	if err := verifyReceiptFile(fd); err != nil {
		d.IntegrityError = boundedJournalError(err.Error())
		return d
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Size > maxOperationBytes {
		d.IntegrityError = "operation record exceeds its read bound or cannot be inspected"
		return d
	}
	data, err := io.ReadAll(io.LimitReader(file, maxOperationBytes+1))
	if err != nil || len(data) > maxOperationBytes {
		d.IntegrityError = "operation record is unreadable or exceeds its bound"
		return d
	}
	parseOperation(data, id, &d)
	return d
}

func parseOperation(data []byte, id string, d *HistoryDetail) {
	terminated := len(data) > 0 && data[len(data)-1] == '\n'
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) == 0 || len(lines[0]) == 0 {
		d.IntegrityError = "operation intent is missing"
		return
	}
	var active = -1
	var previous = len(d.ReviewedManifest.Entries)
	failedAttempt := false
	for n, line := range lines {
		if len(line) == 0 {
			if n == len(lines)-1 { // final newline
				break
			}
			d.IntegrityError = "operation contains an empty event"
			break
		}
		var e operationEvent
		if err := decodeExact(line, &e); err != nil || e.Schema != operationEventSchema || e.OperationID != id || e.At.IsZero() || !validManifestDigest(e.ManifestSHA) {
			d.IntegrityError = "operation event is invalid"
			break
		}
		if n == 0 {
			if e.Kind != "intent" || e.Manifest == nil || Validate(*e.Manifest) != nil {
				d.IntegrityError = "operation intent or reviewed manifest is invalid"
				break
			}
			digest, err := manifestDigest(*e.Manifest)
			if err != nil || digest != e.ManifestSHA {
				d.IntegrityError = "reviewed manifest digest does not match its intent"
				break
			}
			d.manifestSHA = digest
			d.ReviewedManifest = *e.Manifest
			d.StartedAt = e.At.UTC().Format(time.RFC3339Nano)
			d.State = "not_finalized"
			d.Attempts = make([]HistoryAttempt, len(e.Manifest.Entries))
			for index, entry := range e.Manifest.Entries {
				d.Attempts[index] = HistoryAttempt{EntryIndex: index, Path: entry.Path, State: "not_attempted"}
			}
			previous = len(e.Manifest.Entries)
			continue
		}
		switch e.Kind {
		case "attempt_started":
			if e.EntryIndex < 0 || e.EntryIndex >= len(d.Attempts) || e.EntryIndex != previous-1 || active >= 0 || failedAttempt || e.Manifest != nil || e.Report != nil || e.State != "" || e.Error != "" {
				d.IntegrityError = "attempt event order or member identity is invalid"
				break
			}
			active = e.EntryIndex
			previous = e.EntryIndex
			d.Attempts[e.EntryIndex].State = "outcome_unknown"
		case "attempt_result":
			if active != e.EntryIndex || (e.State != "unlink_succeeded" && e.State != "unlink_failed") || e.Manifest != nil || e.Report != nil || (e.State == "unlink_succeeded" && e.Error != "") || (e.State == "unlink_failed" && e.Error == "") {
				d.IntegrityError = "attempt result does not match its durable start"
				break
			}
			d.Attempts[e.EntryIndex].State = e.State
			d.Attempts[e.EntryIndex].Error = e.Error
			failedAttempt = e.State == "unlink_failed"
			active = -1
		case "terminal":
			if active >= 0 || e.Report == nil || e.Report.Schema != ReportSchema || e.Report.OperationID != id || (e.Report.Status != "completed" && e.Report.Status != "partial" && e.Report.Status != "failed" && e.Report.Status != "review_required") || e.Manifest != nil || e.EntryIndex != 0 || e.State != "" || e.Error != "" {
				d.IntegrityError = "terminal report is inconsistent with operation events"
				break
			}
			if n != len(lines)-2 && !(n == len(lines)-1) {
				d.IntegrityError = "events follow the terminal report"
				break
			}
			reportCopy := *e.Report
			if err := validateTerminalReport(d, reportCopy); err != nil {
				d.IntegrityError = err.Error()
				break
			}
			d.TerminalReport = &reportCopy
			d.State = e.Report.Status
		default:
			d.IntegrityError = "operation event kind is unknown"
		}
		if d.IntegrityError != "" {
			break
		}
		if e.ManifestSHA != d.manifestSHA {
			d.IntegrityError = "operation event manifest digest changed"
			break
		}
	}
	if d.IntegrityError == "" && !terminated {
		d.IntegrityError = "operation record ends with an incomplete event"
	}
	if d.IntegrityError != "" {
		d.State = "unknown"
		d.TerminalReport = nil
	}
}

func validateTerminalReport(detail *HistoryDetail, report Report) error {
	if report.Preview != nil || report.Deleted == nil || report.Errors == nil {
		return errors.New("terminal report has an invalid operation shape")
	}
	var attempted int
	var deleted []string
	var logicalBytes int64
	for index := len(detail.Attempts) - 1; index >= 0; index-- {
		attempt := detail.Attempts[index]
		switch attempt.State {
		case "outcome_unknown":
			return errors.New("terminal report hides an unresolved unlink outcome")
		case "unlink_succeeded":
			attempted++
			deleted = append(deleted, attempt.Path)
			entry := detail.ReviewedManifest.Entries[index]
			if entry.Identity.Type != "directory" {
				logicalBytes += entry.Identity.Size
			}
		case "unlink_failed":
			attempted++
			want := attempt.Path + ": " + attempt.Error
			found := false
			for _, message := range report.Errors {
				if strings.HasPrefix(message, want) || strings.HasPrefix(want, message) {
					found = true
					break
				}
			}
			if !found {
				return errors.New("terminal report omits a recorded unlink failure")
			}
		case "not_attempted":
		default:
			return errors.New("terminal report has an unknown member state")
		}
	}
	if report.Attempted != attempted || report.LogicalBytes != logicalBytes || len(report.Deleted) != len(deleted) {
		return errors.New("terminal report counts differ from durable member events")
	}
	for index := range deleted {
		if report.Deleted[index] != deleted[index] {
			return errors.New("terminal report deleted prefix differs from durable member events")
		}
	}
	switch report.Status {
	case "completed":
		if attempted != len(detail.Attempts) || len(deleted) != len(detail.Attempts) || len(report.Errors) != 0 {
			return errors.New("completed report is inconsistent with the reviewed batch")
		}
	case "partial":
		if len(deleted) == 0 || len(report.Errors) == 0 {
			return errors.New("partial report lacks a deleted prefix or error")
		}
	case "failed":
		if attempted == 0 || len(deleted) != 0 || len(report.Errors) == 0 {
			return errors.New("failed report is inconsistent with attempted members")
		}
	case "review_required":
		if attempted != 0 || len(deleted) != 0 || len(report.Errors) == 0 {
			return errors.New("review-required report follows an unlink attempt")
		}
	}
	return nil
}

func manifestDigest(m Manifest) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func decodeExact(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := uniqueJSON(d, 0); err != nil {
		return err
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func validOperationID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := strconv.ParseUint(id[:16], 16, 64)
	if err != nil {
		return false
	}
	_, err = strconv.ParseUint(id[16:], 16, 64)
	return err == nil
}

func validManifestDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validTimestamp(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC
}

func historyPageAt(root string, limit int, cursor string) HistoryPage {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return HistoryPage{Schema: HistoryPageSchema, Items: []HistoryItem{}, Incomplete: true, Issues: []string{boundedJournalError(err.Error())}}
	}
	defer unix.Close(fd)
	return historyPageAtFD(fd, limit, cursor)
}
