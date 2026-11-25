package main

// `sirsi maat reserve r1 r2 ...` — all-or-nothing multi-resource reserve
// (ledger task maat-multi-resource-reserve). Admission is the router's: one
// transaction, one lock, one write (schedule.ReserveMulti). This file is only
// the CLI shell: argument parsing, the journal projection, and output.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// parseExemptPIDs resolves repeated --exempt-pid values to resource -> PID. A
// bare PID applies to every listed resource; resource=PID applies to that one
// (and wins). A resource that was not asked for is an error, never ignored.
func parseExemptPIDs(specs, resources []string) (map[string]int, error) {
	pids := map[string]int{}
	named := map[string]int{}
	for _, spec := range specs {
		res, pid, hasRes := strings.Cut(spec, "=")
		if !hasRes {
			res, pid = "", spec
		}
		n, err := strconv.Atoi(pid)
		if err != nil {
			return nil, fmt.Errorf("maat: --exempt-pid %q: want PID or resource=PID", spec)
		}
		if !hasRes {
			for _, r := range resources {
				pids[r] = n
			}
			continue
		}
		found := false
		for _, r := range resources {
			found = found || r == res
		}
		if !found {
			return nil, fmt.Errorf("maat: --exempt-pid %q names a resource that is not being reserved", spec)
		}
		named[res] = n
	}
	for r, n := range named {
		pids[r] = n
	}
	return pids, nil
}

func reserveMulti(l *schedule.Ledger, resources []string, base schedule.Reservation, pids map[string]int) error {
	if resQueue || resWait > 0 || maatOperationNonce != "" || os.Getenv("SIRSI_MAAT_POD") != "" {
		return fmt.Errorf("maat: reserving several resources does not combine with --queue, --wait, --operation-nonce or pod routing")
	}
	reqs := make([]schedule.Reservation, len(resources))
	for i, r := range resources {
		reqs[i] = base
		reqs[i].Resource, reqs[i].ExemptPID = r, pids[r]
	}
	res, err := l.ReserveMulti(reqs)
	if err != nil {
		// Same contract as a single refusal: reason on stdout under --json, error
		// returned (exit non-zero), nothing reserved. "not-supported" tells an
		// old-server caller to fall back to one reserve per resource.
		if maatJSON {
			_ = emitJSON(schedule.MultiRefusal(err))
		}
		return err
	}
	// The grant is already durable; a journal failure only warns (see reserve).
	var journalErrs []error
	for _, m := range res.Reservations {
		row := &schedule.Reservation{ID: m.ID, Resource: m.Resource, Holder: base.Holder, Share: m.Share,
			Cores: m.Cores, Reason: m.Reason, PendingCedes: m.PendingCedes}
		if m.Share == schedule.ShareFloor {
			conflictHolder := ""
			if m.Conflict != nil {
				conflictHolder = m.Conflict.Holder
			}
			if err := logFloorGrant(l, base.Holder, m.Resource, row, conflictHolder); err != nil {
				journalErrs = append(journalErrs, err)
			}
		}
		one := schedule.Reservation{Resource: m.Resource, Holder: base.Holder}
		if err := recordReservationDecision(one, schedule.ReserveResult{Granted: true, Reservation: row, Conflict: m.Conflict}); err != nil {
			journalErrs = append(journalErrs, err)
		}
	}
	journalErr := journalWarn("reservation", journalErrs...)
	if maatJSON {
		return emitJSONWithJournalError(res, journalErr)
	}
	for _, m := range res.Reservations {
		if m.Share == schedule.ShareFloor {
			printFloorGrant(&schedule.Reservation{Resource: m.Resource, Cores: m.Cores, Reason: m.Reason, PendingCedes: m.PendingCedes})
			continue
		}
		fmt.Printf("𓆄 reserved %s for %s until %s  (id %s)\n", m.Resource, base.Holder, orNow(base.EstEnd), m.ID)
	}
	return nil
}
