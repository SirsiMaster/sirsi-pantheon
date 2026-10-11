package main

import (
	"context"
	"fmt"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"os"
	"strings"
)

func dashboardOwnerAction(ctx context.Context, req dashboard.RouterActionRequest) (dashboard.RouterActionResult, error) {
	out := dashboard.RouterActionResult{}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	// Browser operations must never select a fixture/local SQLite database.
	if strings.TrimSpace(os.Getenv("SIRSI_ROUTER_URL")) == "" {
		return out, fmt.Errorf("canonical router service not configured; no local-store fallback")
	}
	repo, err := router.FindRepoRoot()
	if err != nil {
		return out, err
	}
	f, err := dispatch.Open(repo)
	if err != nil {
		return out, err
	}
	defer f.Close()
	r, err := f.ActAsOwner(dispatch.OwnerAction{Action: req.Action, ItemID: req.ItemID, Expected: req.Expected, Recipient: req.Recipient, Message: req.Message})
	return dashboard.RouterActionResult{ItemID: r.ItemID, Action: r.Action, ResponseID: r.ResponseID, Recipient: r.Recipient, Status: r.Status}, err
}
