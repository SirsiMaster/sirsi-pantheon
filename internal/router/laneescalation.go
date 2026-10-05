package router

import (
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/supervision"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// RouteLaneEscalations delivers supervision escalations to the owner.
//
// This is the caller supervision.Escalates() never had. Before it, the
// classifier could decide a lane was beyond automatic repair, the board could
// paint it, and the information stopped there — the owner learned about stuck
// lanes by asking, which is the failure mode the whole supervision layer exists
// to remove. ADR-054 §3 already required "two failed wake cycles -> owner
// escalation card (never silent)"; that sentence had no implementation.
//
// Dedup is by exact item title against the owner's OPEN inbox, matching
// gemmaRouteRestoreFail. A supervisor pass runs on a 60s cadence, so without
// dedup this would mint one item per lane per minute and be muted within the
// hour — an escalation channel nobody reads is worse than none, because it
// looks like coverage.
//
// Returns the escalations actually sent (excludes deduped ones) so a caller can
// report honestly rather than claim every candidate was delivered.
func RouteLaneEscalations(routerRoot string, escalations []supervision.Escalation) ([]supervision.Escalation, error) {
	if routerRoot == "" {
		return nil, nil
	}
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		return nil, fmt.Errorf("lane escalation: open dispatch: %w", err)
	}
	defer func() { _ = f.Close() }()

	open, err := f.Inbox("owner")
	if err != nil {
		// Fail closed. Sending without reading the inbox first would duplicate
		// every still-open escalation on every pass, which is precisely the
		// noise dedup exists to prevent.
		return nil, fmt.Errorf("lane escalation: read owner inbox: %w", err)
	}
	openTitles := make(map[string]bool, len(open))
	for _, it := range open {
		openTitles[it.Title] = true
	}
	resolveClearedAlerts(f, open, escalations)

	var sent []supervision.Escalation
	var firstErr error
	for _, e := range escalations {
		if openTitles[e.Title()] {
			continue
		}
		if _, sendErr := f.Send("horus", "owner", e.Title(), "decision", e.Why); sendErr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("lane escalation: send %s: %w", e.Agent, sendErr)
			}
			continue
		}
		// Record locally too: two lanes escalated in one pass must not collide
		// if they somehow share a title.
		openTitles[e.Title()] = true
		sent = append(sent, e)
	}
	return sent, firstErr
}

// resolveClearedAlerts closes Horus's own "lane needs you" alerts once the
// condition that raised them is gone. The sender only ever opened these (deduped
// by title) and nothing closed them, so every lane that recovered left a stale
// card on the owner's board (six such cards were dismissed by hand on 2026-10-02
// for lanes that were already wakeable). It closes only what Horus itself sent, by
// title, and only when the current pass no longer escalates that lane; a still-true
// alert is never touched. Failures are ignored: a stale card is the old behavior,
// never a reason to skip the escalation pass.
func resolveClearedAlerts(f *dispatch.Facade, open []work.Item, current []supervision.Escalation) {
	still := make(map[string]bool, len(current))
	for _, e := range current {
		still[e.Title()] = true
	}
	for _, it := range open {
		if it.From != "horus" || still[it.Title] {
			continue
		}
		if !strings.HasPrefix(it.Title, "Lane needs you:") && !strings.HasPrefix(it.Title, "Lanes need you:") {
			continue
		}
		_ = f.Store().CloseItem(it.ID, "Auto-resolved by Horus: the lane is reachable again (no current escalation for it).")
	}
}
