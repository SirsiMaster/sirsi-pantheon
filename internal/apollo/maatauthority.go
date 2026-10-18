package apollo

// Ma'at is the installed Sirsi capacity and reservation authority.  This
// adapter gives a native Apollo backend the narrow authority it needs without
// turning Pantheon into a second model runner or creating a new credential
// issuer.  A successful admission is a set of live, exact resource leases;
// any floor-share or partial admission is rejected and rolled back.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

const nativeAdmissionLeaseTTL = 120 * time.Second

// NativeRankRequest is one profile-bound Apollo rank. Resource and host IDs
// are supplied by the installed profile, never command-line input.
type NativeRankRequest struct {
	Rank        int
	HostID      string
	Resource    string
	MemoryBytes int64
}

// NativeAdmissionRequest is the profile-bound information Ma'at needs before
// Apollo may start an already-selected native plan.
type NativeAdmissionRequest struct {
	ExecutionID string
	PlanSHA256  string
	Epoch       uint64
	Holder      string
	ExpiresAt   time.Time
	Ranks       []NativeRankRequest
}

// NativeRankFence binds an admitted rank to its durable Ma'at reservation.
// Fence is the reservation identity; Generation identifies the exact frozen
// execution/plan/rank tuple and is reproducible for terminal evidence.
type NativeRankFence struct {
	Rank                 int       `json:"rank"`
	HostID               string    `json:"host_id"`
	Fence                string    `json:"fence"`
	Generation           string    `json:"generation"`
	ExpiresAt            time.Time `json:"expires_at"`
	MemoryAdmissionBytes int64     `json:"memory_admission_bytes"`
	resource             string
}

// NativeAdmission is the all-rank outcome consumed by the Apollo bridge.
type NativeAdmission struct {
	ExecutionID string            `json:"execution_id"`
	Epoch       uint64            `json:"epoch"`
	Ranks       []NativeRankFence `json:"rank_fences"`
}

// MaatAuthority owns only the live reservation mapping for in-process native
// Apollo work. It neither chooses a model nor starts a worker.
type MaatAuthority struct {
	ledger nativeReservationLedger
	now    func() time.Time
	mu     sync.Mutex
	active map[string]NativeAdmission
}

// nativeReservationLedger keeps reservation failures testable without live holds.
type nativeReservationLedger interface {
	Reserve(schedule.Reservation, bool) (schedule.ReserveResult, error)
	Release(string) (*schedule.Reservation, error)
	Status(string) ([]schedule.Reservation, error)
}

func NewMaatAuthority(ledger *schedule.Ledger) (*MaatAuthority, error) {
	if ledger == nil {
		return nil, fmt.Errorf("Apollo native admission requires Ma'at's reservation ledger")
	}
	return &MaatAuthority{ledger: ledger, now: time.Now, active: make(map[string]NativeAdmission)}, nil
}

// Admit creates all profile-bound holds or attempts rollback of every returned
// reservation, reporting cleanup failures rather than claiming no holds remain.
// Ma'at normally
// offers a floor share under contention; native distributed inference cannot
// use that degraded allocation, so a floor grant is an admission failure.
func (a *MaatAuthority) Admit(ctx context.Context, req NativeAdmissionRequest) (NativeAdmission, error) {
	if err := validateNativeAdmissionRequest(req, a.now()); err != nil {
		return NativeAdmission{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.active[req.ExecutionID]; exists {
		return NativeAdmission{}, fmt.Errorf("Apollo execution %s already has a Ma'at admission", req.ExecutionID)
	}
	admission := NativeAdmission{ExecutionID: req.ExecutionID, Epoch: req.Epoch, Ranks: make([]NativeRankFence, 0, len(req.Ranks))}
	for _, rank := range req.Ranks {
		if err := ctx.Err(); err != nil {
			return NativeAdmission{}, errors.Join(err, a.releaseLocked(admission))
		}
		remaining := time.Until(req.ExpiresAt)
		if remaining <= 0 {
			return NativeAdmission{}, errors.Join(fmt.Errorf("Apollo native admission expired before rank %d", rank.Rank), a.releaseLocked(admission))
		}
		ttl := int(remaining.Seconds())
		if ttl > int(nativeAdmissionLeaseTTL.Seconds()) {
			ttl = int(nativeAdmissionLeaseTTL.Seconds())
		}
		if ttl < 1 {
			return NativeAdmission{}, errors.Join(fmt.Errorf("Apollo native admission has no remaining lease for rank %d", rank.Rank), a.releaseLocked(admission))
		}
		result, err := a.ledger.Reserve(schedule.Reservation{
			Resource: rank.Resource, Holder: req.Holder,
			Work:   "apollo-native:" + req.ExecutionID,
			Regime: schedule.RegimeLoaded,
			Cores:  1, MemGB: ceilGiB(rank.MemoryBytes),
			Start:       a.now().UTC().Format(time.RFC3339),
			EstEnd:      req.ExpiresAt.UTC().Format(time.RFC3339),
			LeaseTTLSec: ttl,
		}, true)
		// Queued and floor-share outcomes also own durable rows. Track the
		// current rank before rejecting it so rollback covers every returned ID.
		if result.Reservation != nil {
			admission.Ranks = append(admission.Ranks, NativeRankFence{
				Rank: rank.Rank, HostID: rank.HostID, Fence: result.Reservation.ID,
				Generation: nativeFenceGeneration(req, rank, result.Reservation.ID),
				ExpiresAt:  req.ExpiresAt.UTC(), MemoryAdmissionBytes: rank.MemoryBytes, resource: rank.Resource,
			})
		}
		if err != nil || !result.Granted || result.Reservation == nil || result.Reservation.Share != schedule.ShareFull {
			failure := fmt.Errorf("Apollo rank %d on %s lacks an exclusive Ma'at reservation", rank.Rank, rank.Resource)
			if err != nil {
				failure = fmt.Errorf("reserve Apollo rank %d on %s: %w", rank.Rank, rank.Resource, err)
			}
			return NativeAdmission{}, errors.Join(failure, a.releaseLocked(admission))
		}
	}
	a.active[req.ExecutionID] = cloneNativeAdmission(admission)
	return cloneNativeAdmission(admission), nil
}

// Verify refuses changed, expired, released, or foreign rank holds before the
// Apollo engine is allowed to start any profile-pinned worker.
func (a *MaatAuthority) Verify(ctx context.Context, admission NativeAdmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	stored, ok := a.active[admission.ExecutionID]
	if !ok || !sameNativeAdmission(stored, admission) {
		return fmt.Errorf("Apollo native admission is unknown or altered")
	}
	if !admission.Ranks[0].ExpiresAt.After(a.now()) {
		return fmt.Errorf("Apollo native admission expired")
	}
	for _, fence := range admission.Ranks {
		rows, err := a.ledger.Status(fence.resource)
		if err != nil {
			return fmt.Errorf("read Ma'at fence %s: %w", fence.Fence, err)
		}
		if !liveExactFence(rows, fence, admission.ExecutionID, a.now()) {
			return fmt.Errorf("Apollo rank %d no longer holds its Ma'at fence", fence.Rank)
		}
	}
	return nil
}

// Release ends every exact hold. It reports every persistence error while
// attempting all releases so an owner has a complete cleanup outcome.
func (a *MaatAuthority) Release(ctx context.Context, admission NativeAdmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	stored, ok := a.active[admission.ExecutionID]
	if !ok || !sameNativeAdmission(stored, admission) {
		return fmt.Errorf("Apollo native admission is unknown or altered")
	}
	delete(a.active, admission.ExecutionID)
	return a.releaseLocked(stored)
}

func (a *MaatAuthority) releaseLocked(admission NativeAdmission) error {
	var failures []error
	for _, fence := range admission.Ranks {
		if _, err := a.ledger.Release(fence.Fence); err != nil {
			failures = append(failures, fmt.Errorf("release Apollo Ma'at fence %s: %w", fence.Fence, err))
		}
	}
	return errors.Join(failures...)
}

func validateNativeAdmissionRequest(req NativeAdmissionRequest, now time.Time) error {
	if strings.TrimSpace(req.ExecutionID) == "" || strings.TrimSpace(req.PlanSHA256) == "" || strings.TrimSpace(req.Holder) == "" || req.Epoch == 0 || !req.ExpiresAt.After(now) || req.ExpiresAt.Sub(now) > nativeAdmissionLeaseTTL || len(req.Ranks) < 1 {
		return fmt.Errorf("invalid Apollo native admission request")
	}
	seenHosts, seenResources := map[string]bool{}, map[string]bool{}
	for index, rank := range req.Ranks {
		if rank.Rank != index || strings.TrimSpace(rank.HostID) == "" || strings.TrimSpace(rank.Resource) == "" || rank.MemoryBytes <= 0 || seenHosts[rank.HostID] || seenResources[rank.Resource] {
			return fmt.Errorf("invalid Apollo native rank %d", index)
		}
		seenHosts[rank.HostID], seenResources[rank.Resource] = true, true
	}
	return nil
}

func ceilGiB(bytes int64) int {
	const gib = int64(1024 * 1024 * 1024)
	return int((bytes + gib - 1) / gib)
}

func nativeFenceGeneration(req NativeAdmissionRequest, rank NativeRankRequest, fence string) string {
	s := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", req.ExecutionID, req.PlanSHA256, req.Epoch, rank.Rank, fence)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func sameNativeAdmission(a, b NativeAdmission) bool {
	if a.ExecutionID != b.ExecutionID || a.Epoch != b.Epoch || len(a.Ranks) != len(b.Ranks) {
		return false
	}
	for i := range a.Ranks {
		if a.Ranks[i] != b.Ranks[i] {
			return false
		}
	}
	return true
}

func cloneNativeAdmission(in NativeAdmission) NativeAdmission {
	out := in
	out.Ranks = append([]NativeRankFence(nil), in.Ranks...)
	return out
}

func liveExactFence(rows []schedule.Reservation, fence NativeRankFence, execution string, now time.Time) bool {
	for _, row := range rows {
		if row.ID == fence.Fence && row.Status == schedule.StatusActive && row.Work == "apollo-native:"+execution && row.EstEnd == fence.ExpiresAt.UTC().Format(time.RFC3339) {
			hb, err := time.Parse(time.RFC3339, row.HeartbeatAt)
			return err == nil && now.Before(hb.Add(time.Duration(row.LeaseTTLSec)*time.Second))
		}
	}
	return false
}

// SortedFences returns a stable rank order for external terminal receipts.
func (a NativeAdmission) SortedFences() []NativeRankFence {
	out := append([]NativeRankFence(nil), a.Ranks...)
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out
}
