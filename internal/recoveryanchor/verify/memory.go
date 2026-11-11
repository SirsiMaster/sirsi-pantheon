package verify

import (
	"errors"
	"maps"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store FOR TESTS ONLY. It is not durable: a
// restart loses everything, which would defeat replay and rollback defense in
// production. It models the transactional contract (all-or-nothing Update)
// by running fn against a copy and swapping it in on success.
type MemoryStore struct {
	mu sync.Mutex
	st memState

	// FailNextCommit makes the next otherwise-successful Update discard its
	// effects and return an error, to exercise crash-before-commit paths.
	FailNextCommit bool
}

type memEpoch struct{ ratchet, mark int64 }

type resKey struct{ fp, id string }

type resVal struct {
	hash      [32]byte
	expiresAt int64
}

type memState struct {
	epoch  uint64
	epochs map[uint64]memEpoch
	res    map[resKey]resVal
}

func (s memState) clone() memState {
	return memState{epoch: s.epoch, epochs: maps.Clone(s.epochs), res: maps.Clone(s.res)}
}

// NewMemoryStore starts at epoch 1 with ratchet and mark initialized from one
// trusted observation of now.
func NewMemoryStore(now time.Time) *MemoryStore {
	u := now.Unix()
	return &MemoryStore{st: memState{
		epoch:  1,
		epochs: map[uint64]memEpoch{1: {ratchet: u, mark: u}},
		res:    map[resKey]resVal{},
	}}
}

// Update implements Store.
func (m *MemoryStore) Update(fn func(Tx) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	work := m.st.clone()
	if err := fn(&memTx{s: &work}); err != nil {
		return err
	}
	if m.FailNextCommit {
		m.FailNextCommit = false
		return errors.New("injected commit failure")
	}
	m.st = work
	return nil
}

// EpochState returns a (possibly retired) epoch's ratchet and mark.
func (m *MemoryStore) EpochState(epoch uint64) (ratchet, mark int64, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.st.epochs[epoch]
	return e.ratchet, e.mark, ok
}

// ReservationCount returns the number of replay rows.
func (m *MemoryStore) ReservationCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.st.res)
}

type memTx struct{ s *memState }

func (t *memTx) Epoch() uint64  { return t.s.epoch }
func (t *memTx) Ratchet() int64 { return t.s.epochs[t.s.epoch].ratchet }
func (t *memTx) Mark() int64    { return t.s.epochs[t.s.epoch].mark }
func (t *memTx) SetRatchet(u int64) {
	e := t.s.epochs[t.s.epoch]
	e.ratchet = u
	t.s.epochs[t.s.epoch] = e
}
func (t *memTx) SetMark(u int64)                 { e := t.s.epochs[t.s.epoch]; e.mark = u; t.s.epochs[t.s.epoch] = e }
func (t *memTx) DeleteReservation(fp, id string) { delete(t.s.res, resKey{fp, id}) }

func (t *memTx) Reservation(fp, id string) ([32]byte, bool) {
	v, ok := t.s.res[resKey{fp, id}]
	return v.hash, ok
}

func (t *memTx) Reserve(fp, id string, h [32]byte, exp int64) {
	t.s.res[resKey{fp, id}] = resVal{hash: h, expiresAt: exp}
}

func (t *memTx) Reservations() []ReservationInfo {
	out := make([]ReservationInfo, 0, len(t.s.res))
	for k, v := range t.s.res {
		out = append(out, ReservationInfo{Fingerprint: k.fp, RequestID: k.id, ExpiresAt: v.expiresAt})
	}
	return out
}

func (t *memTx) StartEpoch(ratchet, mark int64) uint64 {
	t.s.epoch++
	t.s.epochs[t.s.epoch] = memEpoch{ratchet: ratchet, mark: mark}
	return t.s.epoch
}
