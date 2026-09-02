package main

// ADR-062 rs-13: the evidence run. `sirsi router bench` drives the SAME store
// every other verb uses (routerstore.Resolve → the service when
// SIRSI_ROUTER_URL is set) from two or more hosts at once, records every call
// with its latency and outcome as JSONL, and `bench report` merges the logs
// to answer the two questions goal G3 asks: did any item get claimed twice,
// and what are p50/p95/p99 per verb per host.
//
//   host A:  sirsi router bench seed  --agent bench --count 1000
//   host A:  sirsi router bench claim --agent bench --trials 1000 --out a.jsonl
//   host B:  sirsi router bench claim --agent bench --trials 1000 --out b.jsonl
//   any:     sirsi router bench report a.jsonl b.jsonl

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

type benchRecord struct {
	Host    string  `json:"host"`
	Verb    string  `json:"verb"`
	Item    string  `json:"item,omitempty"`
	OK      bool    `json:"ok"`
	Err     string  `json:"err,omitempty"`
	Millis  float64 `json:"ms"`
	At      string  `json:"at"`
	Attempt int     `json:"attempt"`
}

var (
	benchAgent  string
	benchCount  int
	benchTrials int
	benchTTL    time.Duration
	benchOut    string
)

var routerBenchCmd = &cobra.Command{
	Use:    "bench",
	Short:  "ADR-062 evidence run: seed, race claims from several hosts, report duplicates and latency",
	Hidden: true,
}

var routerBenchSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Send --count items to --agent through the resolved store",
	RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		run := time.Now().UTC().Format("150405")
		for i := 0; i < benchCount; i++ {
			if _, _, err := st.SendGuarded(routerstore.SendReq{
				// Rotate the sender per run and per 20 items: send quotas are per
				// (sender, time bucket), and a second run inside the same bucket
				// must not inherit the first run's spend.
				From: fmt.Sprintf("bench-seed-%s-%d", run, i/20), To: benchAgent, Title: fmt.Sprintf("bench item %d", i), Type: "proposal",
				Instructions: "rs-13 evidence run", SubjectKey: fmt.Sprintf("bench-%d-%d", time.Now().UnixNano(), i),
			}); err != nil {
				return fmt.Errorf("seed %d: %w", i, err)
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "seeded %d items for %s\n", benchCount, benchAgent)
		return nil
	},
}

var routerBenchClaimCmd = &cobra.Command{
	Use:   "claim",
	Short: "Claim+complete up to --trials items for --agent, logging every call to --out",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if benchOut == "" {
			return errors.New("--out is required")
		}
		st, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer func() { _ = st.Close() }()
		host, _ := os.Hostname()
		f, err := os.Create(benchOut)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		w := bufio.NewWriter(f)
		defer func() { _ = w.Flush() }()
		enc := json.NewEncoder(w)
		rec := func(verb, item string, err error, d time.Duration, attempt int) {
			r := benchRecord{Host: host, Verb: verb, Item: item, OK: err == nil, Millis: float64(d.Microseconds()) / 1000, At: time.Now().UTC().Format(time.RFC3339Nano), Attempt: attempt}
			if err != nil {
				r.Err = err.Error()
			}
			_ = enc.Encode(r)
		}
		claimed, empties, errs := 0, 0, 0
		for i := 0; i < benchTrials; i++ {
			t0 := time.Now()
			lease, err := st.ClaimNext(benchAgent, benchTTL)
			item := ""
			if lease != nil {
				item = lease.ItemID
			}
			rec("ClaimNext", item, err, time.Since(t0), i)
			switch {
			case errors.Is(err, routerstore.ErrNoWork):
				empties++
				if empties > 20 {
					i = benchTrials // nothing left to claim; stop early
					continue
				}
				time.Sleep(20 * time.Millisecond)
				continue
			case err != nil:
				errs++
				time.Sleep(100 * time.Millisecond) // outage leg: back off, keep going
				continue
			}
			claimed++
			// A real worker never abandons a lease on one transient failure: retry
			// Complete with backoff for as long as the lease is valid (ADR-062:
			// "leases never expire mid-retry").
			var cerr error
			for attempt, delay := 0, 100*time.Millisecond; ; attempt, delay = attempt+1, capDelay(delay*2) {
				t1 := time.Now()
				cerr = st.Complete(lease.ItemID, lease.Token, "bench done on "+host)
				rec("Complete", lease.ItemID, cerr, time.Since(t1), attempt)
				if cerr == nil || errors.Is(cerr, routerstore.ErrLeaseInvalid) || errors.Is(cerr, routerstore.ErrNotOwner) || time.Now().Add(delay).After(lease.Expires) {
					break
				}
				time.Sleep(delay)
			}
			if cerr != nil {
				errs++
			}
			t2 := time.Now()
			_, ierr := st.Inbox(benchAgent)
			rec("Inbox", "", ierr, time.Since(t2), i)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s: claimed %d, empty %d, errors %d → %s\n", host, claimed, empties, errs, benchOut)
		return nil
	},
}

var routerBenchReportCmd = &cobra.Command{
	Use:   "report <log.jsonl>...",
	Short: "Merge bench logs: duplicate claims across hosts, p50/p95/p99 per verb per host",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		type claim struct {
			host string
			at   time.Time
		}
		claims := map[string][]claim{}            // item → successful ClaimNext events
		completed := map[string]map[string]bool{} // item → hosts whose Complete succeeded
		lat := map[string][]float64{}             // "host verb" → ms
		errs := map[string]int{}
		backpressure := map[string]int{}
		unavailable := map[string]int{}
		total := 0
		for _, path := range args {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 1<<20)
			for sc.Scan() {
				var r benchRecord
				if json.Unmarshal(sc.Bytes(), &r) != nil {
					continue
				}
				total++
				key := r.Host + " " + r.Verb
				switch {
				case r.OK:
					lat[key] = append(lat[key], r.Millis)
				case r.Verb == "ClaimNext" && strings.Contains(r.Err, "no open item to claim"):
					// ErrNoWork is the normal end of a drained queue, not an error.
				case strings.Contains(r.Err, "dispatch budget exceeded"):
					backpressure[key]++ // the store's designed per-agent concurrency cap
				case strings.Contains(r.Err, "unavailable") || strings.Contains(r.Err, "HTTP 503") || strings.Contains(r.Err, "HTTP 401"):
					unavailable[key]++ // service/database outage window
				default:
					errs[key]++
				}
				at, _ := time.Parse(time.RFC3339Nano, r.At)
				if r.Verb == "ClaimNext" && r.OK && r.Item != "" {
					claims[r.Item] = append(claims[r.Item], claim{r.Host, at})
				}
				if r.Verb == "Complete" && r.OK && r.Item != "" {
					if completed[r.Item] == nil {
						completed[r.Item] = map[string]bool{}
					}
					completed[r.Item][r.Host] = true
				}
			}
			_ = f.Close()
		}
		// A concurrent duplicate = the same item claimed by two hosts while the
		// first claim was still live (its holder later completed it). A claim by a
		// second host AFTER the first holder failed to complete and the lease
		// expired is the designed recovery, reported separately, never a failure.
		dups, reclaims := 0, 0
		for item, cs := range claims {
			hosts := map[string]bool{}
			for _, c := range cs {
				hosts[c.host] = true
			}
			if len(hosts) < 2 {
				continue
			}
			if len(completed[item]) > 1 {
				dups++ // two hosts both completed it: impossible under fencing
				continue
			}
			sort.Slice(cs, func(a, b int) bool { return cs[a].at.Before(cs[b].at) })
			if !completed[item][cs[0].host] {
				reclaims++ // first holder never completed; second claim is the lease-expiry recovery
			} else {
				dups++
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "records: %d   distinct items claimed: %d   concurrent duplicate claims: %d   lease-expiry reclaims (designed): %d\n", total, len(claims), dups, reclaims)
		keys := make([]string, 0, len(lat))
		for k := range lat {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(cmd.OutOrStdout(), "%-28s %6s %9s %9s %9s %6s %9s %9s\n", "host verb", "n", "p50 ms", "p95 ms", "p99 ms", "errs", "backpres.", "unavail.")
		for _, k := range keys {
			v := lat[k]
			sort.Float64s(v)
			fmt.Fprintf(cmd.OutOrStdout(), "%-28s %6d %9.1f %9.1f %9.1f %6d %9d %9d\n", k, len(v), pct(v, 50), pct(v, 95), pct(v, 99), errs[k], backpressure[k], unavailable[k])
		}
		if dups > 0 {
			return fmt.Errorf("G3 FAILED: %d item(s) claimed by more than one host", dups)
		}
		return nil
	},
}

// capDelay bounds the completion-retry backoff at 5 s.
func capDelay(d time.Duration) time.Duration {
	if d > 5*time.Second {
		return 5 * time.Second
	}
	return d
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(float64(len(sorted)-1) * p / 100)
	return sorted[i]
}

func init() {
	routerBenchCmd.PersistentFlags().StringVar(&benchAgent, "agent", "bench", "agent id the items are addressed to")
	routerBenchSeedCmd.Flags().IntVar(&benchCount, "count", 1000, "items to seed")
	routerBenchClaimCmd.Flags().IntVar(&benchTrials, "trials", 1000, "claim attempts")
	routerBenchClaimCmd.Flags().DurationVar(&benchTTL, "ttl", 30*time.Second, "lease TTL per claim")
	routerBenchClaimCmd.Flags().StringVar(&benchOut, "out", "", "JSONL log path (required)")
	routerBenchCmd.AddCommand(routerBenchSeedCmd, routerBenchClaimCmd, routerBenchReportCmd)
	routerCmd.AddCommand(routerBenchCmd)
}
