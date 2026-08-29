package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
)

const canonicalActionBaseURL = "http://127.0.0.1:9119"

type runStatus struct {
	Running bool                 `json:"running"`
	Current string               `json:"current"`
	Last    dashboard.RunReceipt `json:"last"`
}

func postCanonicalAction(ctx context.Context, request dashboard.ActionRequest, target any) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, canonicalActionBaseURL+"/api/run", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("action API %s: %s", response.Status, string(body))
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func waitCanonicalAction(ctx context.Context, key string) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, canonicalActionBaseURL+"/api/run/status", nil)
		if err != nil {
			return err
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return err
		}
		var status runStatus
		decodeErr := json.NewDecoder(response.Body).Decode(&status)
		response.Body.Close()
		if decodeErr != nil {
			return decodeErr
		}
		if !status.Running {
			if status.Last.Key != key {
				return fmt.Errorf("action %s ended without a matching receipt", key)
			}
			if status.Last.Status != "success" {
				if status.Last.Error != "" {
					return fmt.Errorf("action %s failed: %s", key, status.Last.Error)
				}
				return fmt.Errorf("action %s ended with status %s", key, status.Last.Status)
			}
			return nil
		}
		if status.Current != "" && status.Current != key {
			return fmt.Errorf("action %s displaced by %s", key, status.Current)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func runCanonicalAction(clicked *systray.MenuItem, key string, store *notify.Store, rr *resultRow) {
	spec, ok := dashboard.LookupAction(key)
	if clicked == nil || !ok || spec.Destructive {
		return
	}
	label := spec.Label
	clicked.SetTitle("⏳ " + label)
	clicked.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		var started dashboard.ActionResult
		err := postCanonicalAction(ctx, dashboard.ActionRequest{Action: key}, &started)
		if err == nil {
			err = waitCanonicalAction(ctx, key)
		}
		icon, summary, severity := "✓", label+" completed", notify.SeveritySuccess
		if err != nil {
			icon, summary, severity = "✗", err.Error(), notify.SeverityError
		}
		recordNotify(store, label, key, severity, summary, summary)
		if rr != nil {
			rr.set(label, icon, summary, summary)
		}
		clicked.SetTitle(icon + " " + label)
		clicked.Enable()
		time.AfterFunc(5*time.Second, func() { clicked.SetTitle(label) })
	}()
}

type preparedMenuAction struct {
	request dashboard.ActionRequest
	prep    dashboard.PreparedAction
}

func prepareCanonicalAction(preview, confirm *systray.MenuItem, key, target string, store *notify.Store, rr *resultRow, state *preparedMenuAction) {
	spec, ok := dashboard.LookupAction(key)
	if preview == nil || confirm == nil || state == nil || !ok || !spec.Destructive {
		return
	}
	preview.SetTitle("⏳ Preparing " + spec.Label + "…")
	preview.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		request := dashboard.ActionRequest{Action: key, Target: target}
		var prep dashboard.PreparedAction
		err := postCanonicalAction(ctx, request, &prep)
		preview.SetTitle(spec.Label)
		preview.Enable()
		if err != nil {
			recordNotify(store, spec.Label, key+":prepare", notify.SeverityError, err.Error(), err.Error())
			if rr != nil {
				rr.set(spec.Label, "✗", err.Error(), err.Error())
			}
			return
		}
		state.request, state.prep = request, prep
		confirm.SetTitle("  ⚠ Confirm: " + prep.Preview)
		confirm.SetTooltip("Single-use authorization expires " + prep.ExpiresAt.Format(time.Kitchen))
		confirm.Show()
		time.AfterFunc(time.Until(prep.ExpiresAt), func() { confirm.Hide() })
	}()
}

func commitCanonicalAction(confirm *systray.MenuItem, store *notify.Store, rr *resultRow, state *preparedMenuAction) {
	if confirm == nil || state == nil || state.prep.ConfirmToken == "" {
		return
	}
	spec, ok := dashboard.LookupAction(state.request.Action)
	if !ok {
		return
	}
	confirm.SetTitle("  ⏳ Executing " + spec.Label + "…")
	confirm.Disable()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		request := state.request
		request.ConfirmToken = state.prep.ConfirmToken
		request.ActionHash = state.prep.ActionHash
		var started dashboard.ActionResult
		err := postCanonicalAction(ctx, request, &started)
		if err == nil {
			err = waitCanonicalAction(ctx, request.Action)
		}
		icon, summary, severity := "✓", spec.Label+" completed", notify.SeveritySuccess
		if err != nil {
			icon, summary, severity = "✗", err.Error(), notify.SeverityError
		}
		recordNotify(store, spec.Label, request.Action, severity, summary, summary)
		if rr != nil {
			rr.set(spec.Label, icon, summary, summary)
		}
		state.prep = dashboard.PreparedAction{}
		confirm.Enable()
		confirm.Hide()
	}()
}
