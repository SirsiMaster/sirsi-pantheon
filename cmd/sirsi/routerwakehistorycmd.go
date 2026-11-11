package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

var wakeHistoryID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var wakeHistoryExit = regexp.MustCompile(`\bexit status [0-9]{1,3}\b`)
var wakeHistoryHTTP = regexp.MustCompile(`\bapi-call: wake endpoint returned [1-5][0-9]{2}\b`)

// Never print LastError, Reason, event keys, source IDs, acknowledgment refs or
// leases. Errors can contain URLs, credentials, argv or consumer output. Only
// fixed diagnostic vocabulary and numeric statuses cross this projection.
func wakeHistoryError(raw string) string {
	if raw == "" {
		return "no error recorded"
	}
	if len(raw) > 8192 {
		return "oversized error withheld"
	}
	text := strings.ToLower(raw)
	var parts []string
	for _, phrase := range []string{
		"wake lease expired without a store acknowledgment",
		"cli-spawn: empty command", "executable file not found", "no such file or directory",
		"permission denied", "connection refused", "no route to host", "connection reset by peer",
		"could not resolve hostname", "host key verification failed", "i/o timeout",
		"operation timed out", "context deadline exceeded", "context canceled", "signal: killed",
	} {
		if strings.Contains(text, phrase) {
			parts = append(parts, phrase)
		}
	}
	if status := wakeHistoryExit.FindString(text); status != "" {
		parts = append(parts, status)
	}
	if status := wakeHistoryHTTP.FindString(text); status != "" {
		parts = append(parts, status)
	}
	if len(parts) == 0 {
		return "unrecognized error withheld"
	}
	return strings.Join(parts, "; ")
}

type wakeHistoryRow struct {
	EventID     string `json:"event_id"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	Created     string `json:"created"`
	Updated     string `json:"updated"`
	Summary     string `json:"error_summary"`
	ErrorBytes  int    `json:"error_bytes"`
	ErrorSHA256 string `json:"error_sha256"`
}

// Resolve the existing authenticated service/relay only. Refuse an explicit DB
// before Resolve can open/create it; never use a retired local ledger as live.
func readRouterWakeHistory(agent string) ([]routerstore.WakeEvent, error) {
	if strings.TrimSpace(os.Getenv("SIRSI_ROUTER_DB")) != "" {
		return nil, fmt.Errorf("wake history requires the canonical service; explicit local DB refused")
	}
	store, err := routerstore.Resolve()
	if err != nil {
		return nil, fmt.Errorf("wake history service resolution failed: %s", wakeHistoryError(err.Error()))
	}
	defer func() { _ = store.Close() }()
	remote, ok := store.(*routerstore.RemoteStore)
	if !ok {
		return nil, fmt.Errorf("wake history requires the canonical remote service")
	}
	// This is the existing signed, per-agent RPC and its normal session/auth
	// behavior. No caller identity, token, authority or mutation API is added.
	return remote.ListWakeEvents(agent)
}

func newRouterWakeHistoryCommand(read func(string) ([]routerstore.WakeEvent, error)) *cobra.Command {
	last, maxBytes, status := 10, 32768, "terminal_failed"
	cmd := &cobra.Command{
		Use: "wake-history <agent>", Short: "Read bounded, sanitized wake history for exactly one agent",
		Long: `Read one agent's canonical wake-event history without waking or reconciling it.
The existing RPC fetches that agent's FULL history (existing 64 MiB transport
limit); --last and --max-bytes bound the CLIENT OUTPUT, not the server query.
Newest means updated time, then event ID. JSON output only; no raw errors,
consumer content, source IDs, leases or acknowledgment refs. Error summaries
are recognized text, not independently verified transport/exec outcomes.
error_bytes and error_sha256 bind the complete original error bytes without
printing them; a fingerprint is not a child exit receipt or a recovery proof.
WakeEvent contains no invocation log path or child exit receipt. A heartbeat
does not acknowledge a wake or resolve its historical failure.`,
		Args: cobra.ExactArgs(1), SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := args[0]
			if !wakeHistoryID.MatchString(agent) || last < 1 || last > 50 || maxBytes < 2048 || maxBytes > 65536 || (status != "terminal_failed" && status != "all") {
				return fmt.Errorf("require one literal agent, --last 1..50, --max-bytes 2048..65536, --status terminal_failed|all")
			}
			if err := cmd.Context().Err(); err != nil {
				return fmt.Errorf("wake history canceled before read")
			}
			events, err := read(agent)
			if err != nil {
				return fmt.Errorf("wake history read failed: %s", wakeHistoryError(err.Error()))
			}
			if err := cmd.Context().Err(); err != nil {
				return fmt.Errorf("wake history canceled after read")
			}
			rows := make([]wakeHistoryRow, 0)
			for _, event := range events {
				if event.Agent != agent {
					return fmt.Errorf("wake history response violated the one-agent scope")
				}
				if status != "all" && event.Status != status {
					continue
				}
				if !wakeHistoryID.MatchString(event.EventID) || event.Attempts < 0 || (event.Status != "pending" && event.Status != "leased" && event.Status != "acked" && event.Status != "terminal_failed") {
					return fmt.Errorf("wake history response contains invalid event metadata")
				}
				created, cerr := time.Parse(time.RFC3339Nano, event.Created)
				updated, uerr := time.Parse(time.RFC3339Nano, event.Updated)
				if cerr != nil || uerr != nil {
					return fmt.Errorf("wake history response contains invalid timestamps; latest ordering unknown")
				}
				rows = append(rows, wakeHistoryRow{
					EventID: event.EventID, Status: event.Status, Attempts: event.Attempts,
					Created: created.UTC().Format(time.RFC3339Nano), Updated: updated.UTC().Format(time.RFC3339Nano),
					Summary: wakeHistoryError(event.LastError), ErrorBytes: len(event.LastError),
					ErrorSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(event.LastError))),
				})
			}
			sort.Slice(rows, func(i, j int) bool {
				a, _ := time.Parse(time.RFC3339Nano, rows[i].Updated)
				b, _ := time.Parse(time.RFC3339Nano, rows[j].Updated)
				if a.Equal(b) {
					return rows[i].EventID < rows[j].EventID
				}
				return a.After(b)
			})
			matched := len(rows)
			if len(rows) > last {
				rows = rows[:last]
			}
			value := struct {
				Agent               string           `json:"agent"`
				Status              string           `json:"status_filter"`
				HistoryRows         int              `json:"rpc_history_rows"`
				Matched             int              `json:"matched_rows"`
				ClientSideSelection bool             `json:"client_side_selection"`
				RawErrorOmitted     bool             `json:"raw_error_omitted"`
				Events              []wakeHistoryRow `json:"events"`
			}{agent, status, len(events), matched, true, true, rows}
			data, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("wake history JSON encoding failed")
			}
			if len(data)+1 > maxBytes {
				return fmt.Errorf("wake history exceeds --max-bytes; reduce --last (no partial output)")
			}
			_, err = cmd.OutOrStdout().Write(append(data, '\n'))
			return err
		},
	}
	cmd.Flags().IntVar(&last, "last", last, "Newest matching events (1..50)")
	cmd.Flags().IntVar(&maxBytes, "max-bytes", maxBytes, "Maximum complete JSON output bytes, including newline (2048..65536)")
	cmd.Flags().StringVar(&status, "status", status, "terminal_failed or all")
	cmd.Flags().Bool("json", true, "JSON output (always enabled)")
	return cmd
}

func init() { routerCmd.AddCommand(newRouterWakeHistoryCommand(readRouterWakeHistory)) }
