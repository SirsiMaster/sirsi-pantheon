package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func wakeHistoryFixture(id, updated string) routerstore.WakeEvent {
	return routerstore.WakeEvent{EventID: id, Agent: "lane", Status: "terminal_failed", Attempts: 3,
		Created: "2026-11-01T00:00:00Z", Updated: updated, LastError: "exit status 127",
		LeaseToken: "PRIVATE-LEASE", AckRef: "PRIVATE-ACK", Reason: "PRIVATE-REASON", SourceID: "PRIVATE-SOURCE", EventKey: "PRIVATE-KEY"}
}

func TestRouterWakeHistorySelectionAndPrivacy(t *testing.T) {
	events := []routerstore.WakeEvent{wakeHistoryFixture("older", "2026-11-10T09:00:00Z"), wakeHistoryFixture("newest", "2026-11-10T11:00:00+01:00"), wakeHistoryFixture("middle", "2026-11-10T09:30:00Z")}
	events[1].LastError = "fork/exec /PRIVATE/path: permission denied Bearer PRIVATE"
	pending := wakeHistoryFixture("pending", "2026-11-10T12:00:00Z")
	pending.Status = "pending"
	events = append(events, pending)
	calls := 0
	cmd := newRouterWakeHistoryCommand(func(agent string) ([]routerstore.WakeEvent, error) {
		calls++
		if agent != "lane" {
			t.Fatalf("wrong scope %q", agent)
		}
		return events, nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"lane", "--last", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got struct {
		HistoryRows int              `json:"rpc_history_rows"`
		Matched     int              `json:"matched_rows"`
		Events      []wakeHistoryRow `json:"events"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.HistoryRows != 4 || got.Matched != 3 || len(got.Events) != 2 || got.Events[0].EventID != "newest" || got.Events[1].EventID != "middle" || got.Events[0].Summary != "permission denied" {
		t.Fatalf("wrong bounded selection: calls=%d, %+v", calls, got)
	}
	if got.Events[0].ErrorBytes != 57 || got.Events[0].ErrorSHA256 != "8b67c75fbce8674723d1ad0cf36487c963f795d6d1bddae4dae479717cd36286" || got.Events[1].ErrorBytes != 15 || got.Events[1].ErrorSHA256 != "df806131952f330236473c1a12636610eaa280f7bd8f32f0093a59990234f777" {
		t.Fatal("complete raw-error byte count/fingerprint binding mismatch")
	}
	for _, forbidden := range []string{"PRIVATE", "lease_id", "ack_ref", "source_id", "event_key", "reason"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("private field leaked: %q", forbidden)
		}
	}
}

func TestRouterWakeHistoryRefusals(t *testing.T) {
	for _, args := range [][]string{{}, {""}, {"*"}, {"lane", "other"}, {" lane"}, {"lane", "--last=0"}, {"lane", "--last=51"}, {"lane", "--max-bytes=2047"}, {"lane", "--max-bytes=65537"}, {"lane", "--status=failed"}} {
		t.Run(strings.Join(args, "/"), func(t *testing.T) {
			cmd := newRouterWakeHistoryCommand(func(string) ([]routerstore.WakeEvent, error) {
				t.Fatal("invalid arguments reached RPC")
				return nil, nil
			})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil || out.Len() != 0 {
				t.Fatalf("expected refusal without output, err=%v bytes=%d", err, out.Len())
			}
		})
	}
	for _, kind := range []string{"foreign", "malformed-time", "invalid-id", "negative-attempts", "invalid-status", "rpc-error", "canceled", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			row := wakeHistoryFixture("event", "2026-11-10T10:00:00Z")
			switch kind {
			case "foreign":
				row.Agent = "other"
			case "malformed-time":
				row.Updated = "PRIVATE-BAD-TIME"
			case "invalid-id":
				row.EventID = "PRIVATE\nID"
			case "negative-attempts":
				row.Attempts = -1
			case "invalid-status":
				row.Status = "PRIVATE-BAD-STATUS"
			}
			cmd := newRouterWakeHistoryCommand(func(string) ([]routerstore.WakeEvent, error) {
				if kind == "canceled" {
					t.Fatal("canceled command reached RPC")
				}
				if kind == "rpc-error" {
					return nil, errors.New("Bearer PRIVATE-CREDENTIAL https://user:PRIVATE@host")
				}
				rows := []routerstore.WakeEvent{row}
				if kind == "oversize" {
					rows = make([]routerstore.WakeEvent, 50)
					for i := range rows {
						rows[i] = row
					}
				}
				return rows, nil
			})
			if kind == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				cmd.SetContext(ctx)
			}
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"lane", "--last=50", "--max-bytes=2048", "--status=all"})
			err := cmd.Execute()
			if err == nil || out.Len() != 0 || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatalf("unsafe refusal: err=%v bytes=%d", err, out.Len())
			}
		})
	}
}

func TestRouterWakeHistoryEmptyAndAll(t *testing.T) {
	for _, empty := range []bool{true, false} {
		cmd := newRouterWakeHistoryCommand(func(string) ([]routerstore.WakeEvent, error) {
			if empty {
				return nil, nil
			}
			a := wakeHistoryFixture("a", "2026-11-10T10:00:00Z")
			b := a
			b.EventID, b.Status = "b", "acked"
			b.LastError = ""
			return []routerstore.WakeEvent{b, a}, nil
		})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"lane", "--status=all", "--max-bytes=2048"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Events []wakeHistoryRow `json:"events"`
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Events == nil || out.Len() > 2048 || (empty && len(got.Events) != 0) || (!empty && (len(got.Events) != 2 || got.Events[0].EventID != "a" || got.Events[1].Status != "acked")) {
			t.Fatalf("wrong empty/all/tie output: %+v", got)
		}
		if !empty && (got.Events[1].ErrorBytes != 0 || got.Events[1].ErrorSHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || got.Events[1].Summary != "no error recorded") {
			t.Fatal("empty error must retain its exact zero-byte SHA256 binding")
		}
	}
}

func TestRouterWakeHistoryErrorVocabulary(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"", "no error recorded"},
		{"wake lease expired without a store acknowledgment", "wake lease expired without a store acknowledgment"},
		{"fork/exec /PRIVATE/path: permission denied Bearer PRIVATE", "permission denied"},
		{"ssh PRIVATE: Connection refused; exit status 255; token=PRIVATE", "connection refused; exit status 255"},
		{"api-call: wake endpoint returned 503 PRIVATE", "api-call: wake endpoint returned 503"},
		{"unrecognized PRIVATE secret", "unrecognized error withheld"},
		{strings.Repeat("PRIVATE", 2000), "oversized error withheld"},
	} {
		if got := wakeHistoryError(tc.raw); got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
}

func TestRouterWakeHistoryRejectsLocalDBBeforeResolve(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_DB", "not-a-database-to-open")
	if _, err := readRouterWakeHistory("lane"); err == nil || !strings.Contains(err.Error(), "explicit local DB refused") {
		t.Fatalf("expected refusal before opening a local DB: %v", err)
	}
}
