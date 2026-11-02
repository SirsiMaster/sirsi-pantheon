//go:build darwin

package trashreceipt

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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
	rootFD int
	fileFD int
	id     string
	seq    int
	fault  func(string) error
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
	fd, err := unix.Openat(rootFD, id+".jsonl", unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create Trash receipt: %w", err)
	}
	if err := verifyReceiptFile(fd); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	journal = &durableJournal{rootFD: rootFD, fileFD: fd, id: id, fault: fault}
	manifest := m
	if err := journal.append(operationEvent{Schema: operationEventSchema, OperationID: id, Kind: "intent", At: time.Now().UTC(), Manifest: &manifest}, "intent"); err != nil {
		return nil, fmt.Errorf("persist Trash intent before deletion: %w", err)
	}
	if err := appendHistoryIndex(rootFD, id, time.Now().UTC(), fault); err != nil {
		return nil, fmt.Errorf("index durable Trash intent before deletion: %w", err)
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
	if j.fault != nil {
		if err := j.fault(stage); err != nil {
			return err
		}
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	var st unix.Stat_t
	if err := unix.Fstat(j.fileFD, &st); err != nil {
		return err
	}
	if st.Size+int64(len(data)) > maxOperationBytes {
		return errors.New("Trash receipt exceeds its 1 MiB bound")
	}
	if err := writeAll(j.fileFD, data); err != nil {
		return err
	}
	j.seq++
	if err := unix.Fsync(j.fileFD); err != nil {
		return err
	}
	return nil
}

func (j *durableJournal) Close() error {
	if j == nil {
		return nil
	}
	var first error
	if j.fileFD >= 0 {
		if err := unix.Close(j.fileFD); err != nil {
			first = err
		}
		j.fileFD = -1
	}
	if j.rootFD >= 0 {
		if err := unix.Close(j.rootFD); first == nil && err != nil {
			first = err
		}
		j.rootFD = -1
	}
	return first
}

func appendHistoryIndex(rootFD int, id string, at time.Time, fault func(string) error) error {
	fd, err := unix.Openat(rootFD, "history-index.jsonl", unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if err := verifyReceiptFile(fd); err != nil {
		return err
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if fault != nil {
		if err := fault("index"); err != nil {
			return err
		}
	}
	row, err := json.Marshal(historyIndexRow{OperationID: id, StartedAt: at.UTC().Format(time.RFC3339Nano)})
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
	for len(p) > 0 {
		n, err := unix.Write(fd, p)
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
	fd, err := unix.Openat(rootFD, "history-index.jsonl", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
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
	position := int64(0)
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		var c historyCursor
		if err != nil || decodeExact(decoded, &c) != nil || c.Device != uint64(st.Dev) || c.Inode != st.Ino || c.Offset < 0 || c.Offset > st.Size {
			page.Incomplete = true
			page.Issues = append(page.Issues, "history cursor is invalid or stale; refresh history")
			return page
		}
		position = c.Offset
	}
	if _, err := unix.Seek(fd, position, io.SeekStart); err != nil {
		page.Incomplete = true
		page.Issues = append(page.Issues, boundedJournalError(err.Error()))
		return page
	}
	reader := bufio.NewReaderSize(file, maxHistoryLineBytes+1)
	var scanned int64
	stalled := false
	for len(page.Items) < limit && scanned < maxHistoryScanBytes {
		start := position
		line, consumed, readErr := readBoundedLine(reader, maxHistoryLineBytes)
		position += int64(consumed)
		scanned += int64(consumed)
		if readErr == io.EOF && len(line) == 0 {
			break
		}
		if errors.Is(readErr, errHistoryLineTooLong) {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, "history index contains an overlong row")
			continue
		}
		if readErr != nil && readErr != io.EOF {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, "history index line is invalid or exceeds its bound")
			position = start
			stalled = true
			break
		}
		var row historyIndexRow
		if err := decodeExact(bytes.TrimSuffix(line, []byte{'\n'}), &row); err != nil || !validOperationID(row.OperationID) || row.StartedAt == "" {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, "history index contains an invalid record")
			continue
		}
		detail := historyDetailAtFD(rootFD, row.OperationID)
		item := historyItem(detail)
		if detail.IntegrityError != "" {
			page.Incomplete = true
			page.Issues = appendIssue(page.Issues, row.OperationID+": "+detail.IntegrityError)
		}
		page.Items = append(page.Items, item)
		if readErr == io.EOF {
			break
		}
	}
	if scanned >= maxHistoryScanBytes && position < st.Size {
		page.Incomplete = true
		page.Issues = appendIssue(page.Issues, "history page scan reached its 64 KiB bound")
	}
	if position < st.Size && !stalled {
		page.NextCursor = encodeHistoryCursor(historyCursor{Device: uint64(st.Dev), Inode: st.Ino, Offset: position})
	}
	return page
}

var (
	errUnterminatedLine   = errors.New("unterminated history row")
	errHistoryLineTooLong = errors.New("history index row exceeds its bound")
)

func readBoundedLine(r *bufio.Reader, max int) ([]byte, int, error) {
	line := make([]byte, 0, 256)
	consumed := 0
	tooLong := false
	for consumed <= maxHistoryScanBytes {
		part, err := r.ReadSlice('\n')
		consumed += len(part)
		if !tooLong && len(line)+len(part) <= max {
			line = append(line, part...)
		} else {
			tooLong = true
		}
		if err == nil {
			if tooLong {
				return nil, consumed, errHistoryLineTooLong
			}
			return line, consumed, nil
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF {
			if consumed == 0 {
				return nil, 0, io.EOF
			}
			if tooLong || consumed > maxHistoryScanBytes {
				return nil, consumed, errUnterminatedLine
			}
			return line, consumed, errUnterminatedLine
		}
		return line, consumed, err
	}
	return nil, consumed, errUnterminatedLine
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
	fd, err := unix.Openat(rootFD, id+".jsonl", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
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
	for n, line := range lines {
		if len(line) == 0 {
			if n == len(lines)-1 { // final newline
				break
			}
			d.IntegrityError = "operation contains an empty event"
			break
		}
		var e operationEvent
		if err := decodeExact(line, &e); err != nil || e.Schema != operationEventSchema || e.OperationID != id || e.At.IsZero() {
			d.IntegrityError = "operation event is invalid"
			break
		}
		if n == 0 {
			if e.Kind != "intent" || e.Manifest == nil || Validate(*e.Manifest) != nil {
				d.IntegrityError = "operation intent or reviewed manifest is invalid"
				break
			}
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
			if e.EntryIndex < 0 || e.EntryIndex >= len(d.Attempts) || e.EntryIndex >= previous || active >= 0 || e.Manifest != nil || e.Report != nil {
				d.IntegrityError = "attempt event order or member identity is invalid"
				break
			}
			active = e.EntryIndex
			previous = e.EntryIndex
			d.Attempts[e.EntryIndex].State = "outcome_unknown"
		case "attempt_result":
			if active != e.EntryIndex || (e.State != "unlink_succeeded" && e.State != "unlink_failed") || e.Manifest != nil || e.Report != nil {
				d.IntegrityError = "attempt result does not match its durable start"
				break
			}
			d.Attempts[e.EntryIndex].State = e.State
			d.Attempts[e.EntryIndex].Error = e.Error
			active = -1
		case "terminal":
			if active >= 0 || e.Report == nil || e.Report.Schema != ReportSchema || e.Report.OperationID != id || (e.Report.Status != "completed" && e.Report.Status != "partial" && e.Report.Status != "failed" && e.Report.Status != "review_required") || e.Manifest != nil {
				d.IntegrityError = "terminal report is inconsistent with operation events"
				break
			}
			if n != len(lines)-2 && !(n == len(lines)-1) {
				d.IntegrityError = "events follow the terminal report"
				break
			}
			reportCopy := *e.Report
			d.TerminalReport = &reportCopy
			d.State = e.Report.Status
		default:
			d.IntegrityError = "operation event kind is unknown"
		}
		if d.IntegrityError != "" {
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

func historyPageAt(root string, limit int, cursor string) HistoryPage {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return HistoryPage{Schema: HistoryPageSchema, Items: []HistoryItem{}, Incomplete: true, Issues: []string{boundedJournalError(err.Error())}}
	}
	defer unix.Close(fd)
	return historyPageAtFD(fd, limit, cursor)
}
