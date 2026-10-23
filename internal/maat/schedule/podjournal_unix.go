//go:build darwin || linux

package schedule

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// PodJournal is a durable projection of one canonical delegation, not another
// canonical ledger. Callers must obtain grant through authenticated canonical
// access and bind machine to native identity before opening it. One process owns
// the journal; a successful grant is returned only after the append is fsynced.
type PodJournal struct {
	mu     sync.Mutex
	f      *os.File
	grant  Delegation
	state  podFrame
	now    func() time.Time
	failed bool
	events []Reservation
}
type podFrame struct {
	Grant Delegation    `json:"grant"`
	Seq   int64         `json:"seq"`
	Rows  []Reservation `json:"rows,omitempty"`
	Event *Reservation  `json:"event,omitempty"`
}

func OpenPodJournal(path, machine string, grant Delegation) (*PodJournal, error) {
	if err := validateDelegation(grant); err != nil {
		return nil, err
	}
	if machine != grant.AuthorityMachine || grant.Epoch < 1 || grant.expired(time.Now()) {
		return nil, fmt.Errorf("maat: invalid native delegation")
	}
	// Retain every parent descriptor; refuse substituted ancestors as well as leaf symlinks.
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("maat: canonical absolute journal path required")
	}
	parent, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, component := range strings.Split(strings.TrimPrefix(filepath.Dir(path), "/"), "/") {
		if component == "" {
			continue
		}
		next, e := unix.Openat(parent, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(parent)
		if e != nil {
			return nil, e
		}
		parent = next
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, filepath.Base(path), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	fail := func(err error) (*PodJournal, error) { f.Close(); return nil, err }
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fail(fmt.Errorf("maat: journal already owned: %w", err))
	}
	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fail(fmt.Errorf("maat: journal must be a private regular file"))
	}
	encodedGrant, err := json.Marshal(grant)
	if err != nil {
		return fail(err)
	}
	if err = json.Unmarshal(encodedGrant, &grant); err != nil {
		return fail(err)
	}
	j := &PodJournal{f: f, grant: grant, now: time.Now, state: podFrame{Grant: grant}}
	if info.Size() > 64<<20 {
		return fail(fmt.Errorf("maat: journal exceeds recovery bound"))
	}
	r := bufio.NewScanner(f)
	r.Buffer(make([]byte, 4096), 1<<20)
	for r.Scan() {
		line := r.Bytes()
		if len(line) > 0 {
			var frame podFrame
			if err = json.Unmarshal(line, &frame); err != nil {
				return fail(err)
			}
			if !reflect.DeepEqual(frame.Grant, grant) || frame.Seq != j.state.Seq+1 {
				return fail(fmt.Errorf("maat: journal epoch/sequence mismatch"))
			}
			event := frame.Event
			if event == nil {
				for i := range frame.Rows {
					if frame.Rows[i].Seq == frame.Seq {
						copy := frame.Rows[i]
						event = &copy
						break
					}
				}
			}
			if event == nil || event.Seq != frame.Seq || event.Authority != "pod:"+grant.Pod || event.Epoch != grant.Epoch {
				return fail(fmt.Errorf("maat: journal event missing or foreign"))
			}
			j.applyEvent(*event)
		}
	}
	if err = r.Err(); err != nil {
		return fail(err)
	}
	if info.Size() > 0 {
		if _, err = f.Seek(-1, io.SeekEnd); err != nil {
			return fail(err)
		}
		tail := make([]byte, 1)
		if _, err = f.Read(tail); err != nil || tail[0] != '\n' {
			return fail(fmt.Errorf("maat: incomplete journal tail; native recovery required"))
		}
	}
	if _, err = f.Seek(0, io.SeekEnd); err != nil {
		return fail(err)
	}
	if err = f.Sync(); err != nil {
		return fail(err)
	}
	err = unix.Fsync(parent)
	if err != nil {
		return fail(err)
	}
	return j, nil
}

func (j *PodJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil {
		return nil
	}
	e := j.f.Close()
	j.f = nil
	return e
}

// Reserve applies the existing Ma'at scheduling policy locally, then stamps the
// result with canonical epoch and a monotonically increasing replay sequence.
func (j *PodJournal) Reserve(req Reservation, queue bool) (ReserveResult, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil || j.failed {
		return ReserveResult{}, fmt.Errorf("maat: journal unavailable")
	}
	now := j.now()
	issued, _ := time.Parse(time.RFC3339, j.grant.IssuedAt)
	end, _ := time.Parse(time.RFC3339, j.grant.NotAfter)
	if now.Before(issued) || j.grant.expired(now) || !j.grant.coversResource(req.Resource) {
		return ReserveResult{}, fmt.Errorf("maat: finite delegation unavailable")
	}
	_, requestedEnd := parseWindow(req)
	if requestedEnd.After(end) || !requestedEnd.After(now) {
		return ReserveResult{}, fmt.Errorf("maat: reservation exceeds finite delegation")
	}
	if req.Authority != "" || req.Epoch != 0 || req.Seq != 0 {
		return ReserveResult{}, fmt.Errorf("maat: caller cannot supply authority fence")
	}
	if req.LeaseTTLSec <= 0 {
		req.LeaseTTLSec = 120
	}
	if now.Add(time.Duration(req.LeaseTTLSec) * time.Second).After(end) {
		req.LeaseTTLSec = int(end.Sub(now) / time.Second)
	}
	if req.LeaseTTLSec < 1 {
		return ReserveResult{}, fmt.Errorf("maat: delegation ending")
	}
	copyRows := append([]Reservation(nil), j.state.Rows...)
	store := &podProjection{rows: copyRows}
	l := NewLedger(store).WithClock(j.now)
	out, err := l.Reserve(req, queue)
	if err != nil || out.Reservation == nil {
		return out, err
	}
	row := *out.Reservation
	row.Authority = "pod:" + j.grant.Pod
	row.Epoch = j.grant.Epoch
	row.Seq = j.state.Seq + 1
	for i := range store.rows {
		if store.rows[i].ID == row.ID {
			store.rows[i] = row
		}
	}
	if !out.Granted || row.Status != StatusActive {
		out.Reservation = nil
		return out, nil
	}
	if err = j.commitEvent(row); err != nil {
		return ReserveResult{}, err
	}
	out.Reservation = &row
	return out, nil
}

// applyEvent rebuilds latest state while retaining every replay sequence.
func (j *PodJournal) applyEvent(row Reservation) {
	replaced := false
	for i := range j.state.Rows {
		if j.state.Rows[i].ID == row.ID {
			j.state.Rows[i] = row
			replaced = true
			break
		}
	}
	if !replaced {
		j.state.Rows = append(j.state.Rows, row)
	}
	j.state.Seq = row.Seq
	j.events = append(j.events, row)
}
func (j *PodJournal) commitEvent(row Reservation) error {
	frame := podFrame{Grant: j.grant, Seq: row.Seq, Event: &row}
	encoded, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(encoded) >= 1<<20 || len(j.events) >= 65536 {
		return fmt.Errorf("maat: journal event bound reached")
	}
	info, err := j.f.Stat()
	if err != nil {
		return err
	}
	if info.Size()+int64(len(encoded))+1 > 64<<20 {
		return fmt.Errorf("maat: journal size bound reached")
	}
	encoded = append(encoded, '\n')
	if _, err = j.f.Write(encoded); err == nil {
		err = j.f.Sync()
	}
	if err != nil {
		j.failed = true
		return fmt.Errorf("maat: durable event failed: %w", err)
	}
	j.applyEvent(row)
	return nil
}

// Mutate accepts only an authenticated holder supplied by the native transport.
// Every successful mutation is a distinct fsynced sequence, including release.
func (j *PodJournal) Mutate(holder, action, id, value string) (*Reservation, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.f == nil || j.failed {
		return nil, fmt.Errorf("maat: journal unavailable")
	}
	var old *Reservation
	for _, r := range j.state.Rows {
		if r.ID == id {
			copy := r
			old = &copy
			break
		}
	}
	if old == nil || old.Holder != holder {
		return nil, fmt.Errorf("maat: foreign or unknown reservation")
	}
	if old.Status != StatusActive || old.leaseExpired(j.now()) {
		return nil, fmt.Errorf("maat: terminal or expired reservation")
	}
	if (action == "heartbeat" || action == "extend") && j.grant.expired(j.now()) {
		return nil, fmt.Errorf("maat: delegation expired")
	}
	store := &podProjection{rows: append([]Reservation(nil), j.state.Rows...)}
	l := NewLedger(store).WithClock(j.now)
	var row *Reservation
	var err error
	switch action {
	case "heartbeat":
		row, err = l.Heartbeat(id)
	case "extend":
		end, e := time.Parse(time.RFC3339, value)
		limit, _ := time.Parse(time.RFC3339, j.grant.NotAfter)
		if e != nil || end.After(limit) || !end.After(j.now()) {
			return nil, fmt.Errorf("maat: extension exceeds finite delegation")
		}
		row, err = l.Extend(id, value)
	case "release":
		row, err = l.Release(id)
	case "invalidate":
		row, err = l.Invalidate(id, value)
	default:
		return nil, fmt.Errorf("maat: unsupported mutation")
	}
	if err != nil {
		return nil, err
	}
	row.Seq = j.state.Seq + 1
	limit, _ := time.Parse(time.RFC3339, j.grant.NotAfter)
	if row.Status == StatusActive && j.now().Add(time.Duration(row.LeaseTTLSec)*time.Second).After(limit) {
		row.LeaseTTLSec = int(limit.Sub(j.now()) / time.Second)
		if row.LeaseTTLSec < 1 {
			return nil, fmt.Errorf("maat: finite lease ending")
		}
	}
	if err = j.commitEvent(*row); err != nil {
		return nil, err
	}
	return row, nil
}

// Replay returns bounded pending rows in sequence order. Acknowledgement belongs
// to the canonical transaction; a failed relay never deletes local evidence.
func (j *PodJournal) Replay(after int64) []Reservation {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []Reservation{}
	for _, r := range j.events {
		if r.Seq > after {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Seq < out[k].Seq })
	return out
}

// RelayOnce is finite catch-up to the same canonical ledger. The remote service
// derives machine identity from authentication and commits rows plus ack together.
// No local row is discarded, even when the reply is lost after canonical commit.
func (j *PodJournal) RelayOnce(ctx context.Context, canonical AtomicDelegationScheduler) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d, err := canonical.MaatGetDelegation(j.grant.Pod)
	if err != nil {
		return 0, err
	}
	if d == nil || d.Epoch != j.grant.Epoch || d.AuthorityMachine != j.grant.AuthorityMachine {
		return 0, fmt.Errorf("maat: relay authority superseded")
	}
	rows := j.Replay(d.AckedSeq)
	if len(rows) > 256 {
		rows = rows[:256]
	}
	if len(rows) == 0 {
		return d.AckedSeq, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	ack, err := canonical.MaatReconcile(j.grant.AuthorityMachine, j.grant.Pod, j.grant.Epoch, rows)
	if err == nil && ack != rows[len(rows)-1].Seq {
		return 0, fmt.Errorf("maat: canonical acknowledgement mismatch")
	}
	return ack, err
}

// This disposable policy projection is never a second store or database.
type podProjection struct{ rows []Reservation }

func (p *podProjection) GetState(key string) (string, bool, error) {
	if key != stateKey {
		return "", false, nil
	}
	b, e := json.Marshal(map[string]any{"reservations": p.rows})
	return string(b), true, e
}
func (p *podProjection) SetState(key, value string) error {
	return fmt.Errorf("maat: projection requires CAS")
}
func (p *podProjection) CompareAndSwapState(key, old string, exists bool, value string) (bool, error) {
	actual, ok, e := p.GetState(key)
	if e != nil || actual != old || ok != exists {
		return false, e
	}
	var doc struct {
		Reservations []Reservation `json:"reservations"`
	}
	if e = json.Unmarshal([]byte(value), &doc); e != nil {
		return false, e
	}
	p.rows = doc.Reservations
	return true, nil
}
