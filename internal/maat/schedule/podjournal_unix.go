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
	"sync"
	"syscall"
	"time"
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
}
type podFrame struct {
	Grant Delegation    `json:"grant"`
	Seq   int64         `json:"seq"`
	Rows  []Reservation `json:"rows"`
}

func OpenPodJournal(path, machine string, grant Delegation) (*PodJournal, error) {
	if err := validateDelegation(grant); err != nil {
		return nil, err
	}
	if machine != grant.AuthorityMachine || grant.Epoch < 1 || grant.expired(time.Now()) {
		return nil, fmt.Errorf("maat: invalid native delegation")
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_NOFOLLOW, 0600)
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
	j := &PodJournal{f: f, grant: grant, now: time.Now, state: podFrame{Grant: grant}}
	r := bufio.NewReader(f)
	for {
		line, e := r.ReadBytes('\n')
		if len(line) > 0 {
			if e != nil {
				return fail(fmt.Errorf("maat: incomplete journal tail; native recovery required"))
			}
			var frame podFrame
			if err = json.Unmarshal(line, &frame); err != nil {
				return fail(err)
			}
			if !reflect.DeepEqual(frame.Grant, grant) || frame.Seq != j.state.Seq+1 {
				return fail(fmt.Errorf("maat: journal epoch/sequence mismatch"))
			}
			j.state = frame
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return fail(e)
		}
	}
	if _, err = f.Seek(0, io.SeekEnd); err != nil {
		return fail(err)
	}
	if err = f.Sync(); err != nil {
		return fail(err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fail(err)
	}
	err = dir.Sync()
	dir.Close()
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
	next := podFrame{Grant: j.grant, Seq: row.Seq, Rows: store.rows}
	encoded, err := json.Marshal(next)
	if err != nil {
		return ReserveResult{}, err
	}
	encoded = append(encoded, '\n')
	if _, err = j.f.Write(encoded); err == nil {
		err = j.f.Sync()
	}
	if err != nil {
		j.failed = true
		return ReserveResult{}, fmt.Errorf("maat: durable grant failed: %w", err)
	}
	j.state = next
	out.Reservation = &row
	return out, nil
}

// Replay returns bounded pending rows in sequence order. Acknowledgement belongs
// to the canonical transaction; a failed relay never deletes local evidence.
func (j *PodJournal) Replay(after int64) []Reservation {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := []Reservation{}
	for _, r := range j.state.Rows {
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
