package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	ghStatusURL   = "https://www.githubstatus.com/api/v2/summary.json"
	ghActionsName = "Actions" // live API component name, not "GitHub Actions"
)

// ghStatusFetch is the injectable HTTP fetch for tests.
var ghStatusFetch = func(url string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("githubstatus.com returned %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<10))
}

type ghStatusSummary struct {
	Components []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"components"`
}

// ActionsOutage queries githubstatus.com and returns a non-empty warning
// string when GitHub Actions is not fully operational. Returns "" when
// Actions is operational or the status endpoint is unreachable (network
// errors do not block the caller — we prefer a false-negative over a hang).
func ActionsOutage() string {
	body, err := ghStatusFetch(ghStatusURL)
	if err != nil {
		// Unreachable status page is not a reason to block local repair.
		return ""
	}
	var summary ghStatusSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return ""
	}
	for _, c := range summary.Components {
		if c.Name == ghActionsName && c.Status != "operational" {
			return fmt.Sprintf("GitHub Actions is reporting a %s — runner status may reflect a GitHub outage, not a local problem", c.Status)
		}
	}
	return ""
}
