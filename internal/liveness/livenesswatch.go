// Package liveness — the OS-level (launchd) liveness watch.
//
// The router conduit, the gemma generation broker, and the menubar are watched
// today only by Claude-app scheduled tasks: those survive a session close but
// NOT a full OS reboot until a human relaunches the app — and (worse) every
// scheduled run leaks a resident claude-desktop session, the very leak that
// swap-killed the broker on 2026-07-17. This LaunchAgent closes both gaps: it is
// OS-managed (RunAtLoad + StartInterval), needs no Claude app, and spawns NO
// claude session, so it is zero-leak by construction. Routed by claude-home;
// canon A32 (overseer) + the launchd-durability item. Decision: this is the thin
// liveness+alert safety net ONLY — it never runs the router's heavy passes.
//
// Load-bearing safety (A32/ADR-040): Run() is strictly read-and-route. It NEVER
// signals, kills, resizes, or restarts any process, never touches /Applications,
// and never acts on an unverified PID. Remediation stays with the owner (routed
// as one non-duplicate item) and the existing governors — the watch only sees.
package liveness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/reaper"
)

// Label is the LaunchAgent label for the liveness watch.
const Label = "ai.sirsi.liveness-watch"

// StartInterval is how often launchd fires the watch (~15 min): frequent enough
// to catch a wedge within one cadence, rare enough to cost nothing.
const StartInterval = 900

// probeTimeout bounds the gemma generation probe. Longer than a healthy answer,
// short enough that a wedged broker (>30s to first token) reads as wedged. var
// (not const) so tests can shrink it to exercise the timeout/retry path fast.
var probeTimeout = 30 * time.Second

// minWeightedBytes is the Metal-allocation floor below which a running broker
// is treated as weightless — no model loaded onto the GPU. Any serious gemma
// model (even the smallest 2B variant) allocates several GB via MLX; an
// allocation under this floor cannot be serving real inference regardless of
// what a bare /health 200 implies. Read from mlx_active_bytes — the number MLX
// itself reports as currently allocated, not inferred from a proxy metric.
//
// Superseded 2026-08-06 (PRD R6, sirsi-pantheon-fabric
// docs/prd/SNE_HETEROGENEOUS_COMPUTE.md): this used to be an RSS floor
// (minWeightedRSSKB) whose comment claimed it "can never false-positive on a
// loaded model." That claim was false — measured the same day, broker RSS was
// 4.2 GB while mlx_active_bytes was 24.9-31.4 GB, a 27 GB gap RSS cannot see in
// either direction. minWeightedRSSKB is retained ONLY as the fallback for when
// /health itself is unreachable (see brokerMLXBytesFn below).
const minWeightedBytes = 1 * 1024 * 1024 * 1024 // 1 GB

// minWeightedRSSKB is the fallback RSS floor (in KB), used only when the
// broker's /health endpoint cannot be reached (port file present but the
// process is not actually answering). See minWeightedBytes.
const minWeightedRSSKB = 1 * 1024 * 1024 // 1 GB in KB

// WeightsAbsentSentinel is the canonical marker embedded in ProbeGemmaState's
// GemmaWedged detail string when the broker process is running but has no model
// weights loaded (RSS below the weight floor). Exported so the self-healing
// duty in internal/router/gemmaliveness.go can match it without duplicating the
// string — a restart cannot fix this class of wedge.
const WeightsAbsentSentinel = "weights likely absent"

// brokerRSSFn reads the RSS (in KB) of the gemma broker process. Returns 0 if
// the PID file is absent or the process cannot be queried. Injectable (A16/A21)
// so tests can stub it without a live process.
var (
	brokerRSSMu sync.RWMutex
	brokerRSSFn = defaultBrokerRSS
)

func getBrokerRSSFn() func(pidFile string) int64 {
	brokerRSSMu.RLock()
	defer brokerRSSMu.RUnlock()
	return brokerRSSFn
}

func setBrokerRSSFn(fn func(pidFile string) int64) {
	brokerRSSMu.Lock()
	defer brokerRSSMu.Unlock()
	brokerRSSFn = fn
}

// brokerMLXBytesFn reads the broker's true current Metal allocation from its
// own /health endpoint (guard.BrokerMLXActiveBytes — mlx_active_bytes).
// Returns ok=false when the broker is unreachable, in which case the caller
// falls back to brokerRSSFn. Injectable (A16/A21) so tests can stub it
// without a live broker.
var (
	brokerMLXMu      sync.RWMutex
	brokerMLXBytesFn = guard.BrokerMLXActiveBytes
)

func getBrokerMLXBytesFn() func() (int64, bool) {
	brokerMLXMu.RLock()
	defer brokerMLXMu.RUnlock()
	return brokerMLXBytesFn
}

func setBrokerMLXBytesFn(fn func() (int64, bool)) {
	brokerMLXMu.Lock()
	defer brokerMLXMu.Unlock()
	brokerMLXBytesFn = fn
}

// defaultBrokerRSS reads the PID from pidFile and returns the process RSS in KB
// via `ps -o rss=`. Returns 0 on any error — fail-open (never falsely reap a
// live broker because the PID file is missing or ps is unavailable).
func defaultBrokerRSS(pidFile string) int64 {
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid := strings.TrimSpace(string(raw))
	if pid == "" {
		return 0
	}
	out, err := exec.Command("ps", "-o", "rss=", "-p", pid).Output()
	if err != nil {
		return 0
	}
	rss, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return rss
}

// PlistPath is where the LaunchAgent is written (per-user).
func PlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist")
}

// Installed reports whether the LaunchAgent plist is in place. macOS only.
func Installed() bool {
	return runtime.GOOS == "darwin" && fileExists(PlistPath())
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// PlistContent renders the LaunchAgent. RunAtLoad fires it once at login/load;
// StartInterval repeats it. KeepAlive is deliberately ABSENT: the watch runs and
// exits, so KeepAlive would restart it in a tight loop. WorkingDirectory pins the
// repo root so the router root under .agents/idea-router/ resolves.
func PlistContent(sirsiBin, workDir string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>WorkingDirectory</key>
	<string>%[3]s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%[2]s</string>
		<string>liveness-watch</string>
		<string>run</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StartInterval</key>
	<integer>%[4]d</integer>
	<key>StandardOutPath</key>
	<string>/tmp/sirsi-liveness-watch.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/sirsi-liveness-watch.err</string>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`, Label, sirsiBin, workDir, StartInterval)
}

// Install writes the LaunchAgent and loads it so the watch runs now and at every
// login. macOS only; idempotent (a re-install reloads). Returns a status line.
func Install(sirsiBin, workDir string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "liveness watch is macOS only", nil
	}
	if sirsiBin == "" {
		return "", fmt.Errorf("sirsi binary path could not be resolved")
	}
	path := PlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(PlistContent(sirsiBin, workDir)), 0o644); err != nil {
		return "", err
	}
	// Reload: unload first so a re-install picks up changes (ignore the not-loaded error).
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		return "", fmt.Errorf("wrote LaunchAgent but launchctl load failed: %w", err)
	}
	return "liveness watch installed and started (fires now + every " + strconv.Itoa(StartInterval/60) + " min)", nil
}

// Uninstall unloads and removes the LaunchAgent. Idempotent.
func Uninstall() (string, error) {
	path := PlistPath()
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return "liveness watch uninstalled", nil
}

// Finding is one liveness signal read by Run.
type Finding struct {
	Check   string // "gemma-broker" | "menubar" | "memory-death"
	OK      bool
	Detail  string
	Fixable bool   // an owner-fixable, currently-actionable blocker (route it)
	Title   string // deterministic router title for dedup (empty when OK)
	Body    string // routed instructions when !OK && Fixable
}

// Run performs the liveness checks and routes ONE non-duplicate item to owner
// for the worst current owner-fixable blocker. routerRoot is the idea-router
// root (…/.agents/idea-router). Read-and-route only — never kills a process.
func Run(routerRoot string, w io.Writer) error {
	home, _ := os.UserHomeDir()
	// Order = severity: a wedged broker strands every agent; a memory spiral
	// swap-kills the broker; a leaked-session pileup is the precursor that CAUSES
	// that spiral; a dead menubar only loses the operator surface.
	findings := []Finding{
		probeLaunchdDisabled(),
		probeGemma(home),
		probeMemoryDeath(),
		probeSessionLeak(),
		probeMenubar(),
	}

	for _, f := range findings {
		status := "ok"
		if !f.OK {
			status = "WEDGED"
		}
		fmt.Fprintf(w, "%-14s %-7s %s\n", f.Check, status, f.Detail)
	}

	// Route the single worst blocker (order = severity: broker, then memory, then
	// menubar). One item per pass, deduplicated against the open `user` inbox so a
	// persistent wedge does not flood the router every 15 minutes.
	worst := pickWorst(findings)
	if worst == nil {
		return nil
	}
	// Route to the agent that OWNS the remediation, not the owner queue (owner
	// standing correction 2026-07-24: liveness + machine-health remediation
	// belongs to Pantheon, not `user`). Every machine-health condition here —
	// a wedged broker, a memory spiral, a leaked-session pileup, a dead menubar
	// — is one claude-pantheon now remediates under A32/ADR-040 (broker
	// right-size/restart, safe caller-protected reap, launchd kickstart). Only
	// a genuinely owner-only condition (none today) would fall through to user.
	recipient := recipientFor(worst.Check)

	// Send through the ONE dispatch facade, never internal/work directly. A raw
	// work.Send writes items/<id>.md with no store row at all — which post-cutover
	// means the alert never reaches the wake path (consumers block on `router
	// wait`, which reads the store) while still piling into the legacy file dir
	// that observers union back in as phantom open work. This was the last
	// file-only writer in the tree.
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		return fmt.Errorf("route blocker: open dispatch: %w", err)
	}
	defer func() { _ = f.Close() }()

	if hasOpen(f, recipient, worst.Title) {
		fmt.Fprintf(w, "route          skip    already open: %q\n", worst.Title)
		return nil
	}
	res, err := f.Send("horus", recipient, worst.Title, "decision", worst.Body)
	if err != nil {
		return fmt.Errorf("route blocker: %w", err)
	}
	fmt.Fprintf(w, "route          sent    %s → %s (%s)\n", worst.Title, recipient, res.ID)
	return nil
}

// recipientFor maps a finding's Check to the agent that owns its remediation.
// Machine-health conditions go to claude-pantheon (which fixes them under
// A32/ADR-040); anything unclassified falls through to `user` so a novel
// condition still reaches the owner rather than being silently misrouted.
func recipientFor(check string) string {
	switch check {
	case "gemma-broker", "memory-death", "session-leak", "menubar", "launchd-disabled":
		return "claude-pantheon"
	default:
		return "owner"
	}
}

// pickWorst returns the highest-severity fixable non-OK finding, or nil.
// Severity order is the findings slice order in Run: broker > memory > menubar.
func pickWorst(fs []Finding) *Finding {
	for i := range fs {
		if !fs[i].OK && fs[i].Fixable {
			return &fs[i]
		}
	}
	return nil
}

// hasOpen reports whether the recipient already has an open item with this
// title. It reads through the same facade the send goes through — reading files
// while writing the store is what let a persistent blocker re-alert on every
// 15-minute tick: the dedupe could never see its own previous send.
func hasOpen(f *dispatch.Facade, agent, title string) bool {
	items, err := f.Inbox(agent)
	if err != nil {
		return false // can't read the inbox → don't suppress the alert
	}
	for _, it := range items {
		if it.Title == title {
			return true
		}
	}
	return false
}

// GemmaStatus classifies the broker's liveness for consumers that need to ACT on
// it (the router's self-healing gemma-liveness duty), not just alert.
type GemmaStatus int

const (
	// GemmaHealthy — answered a real generation.
	GemmaHealthy GemmaStatus = iota
	// GemmaDown — nothing is serving: no port file, or the connection is refused.
	// The correct restore is to START the broker.
	GemmaDown
	// GemmaWedged — the port responds but the broker is not generating (non-200,
	// zero tokens produced, abnormal finish_reason, or slower than probeTimeout).
	// "Health lies" — /v1/models can return 200 while completions return nothing
	// (2026-07-17). The correct restore is a GRACEFUL restart (stop + start),
	// never a SIGKILL (A32/ADR-040).
	GemmaWedged
	// GemmaBusy — the broker timed out while the local self-hosted runner is
	// active. That is degraded capacity, not proof of a wedge; the watch must not
	// route a wedged item and the restore duty must not restart the broker.
	GemmaBusy
)

// probeRetryPause is the drain between a timed-out probe and its single retry.
// The broker serializes requests (mlx_lm.server), and the triage loop /
// gemma-worker / conduit keep it generating — so a probe that fires mid-request
// queues behind an in-flight generation and can exceed probeTimeout on a
// perfectly HEALTHY broker (observed live: 21.58s queued vs 0.39s idle). A short
// pause lets the blocking generation drain; the retry then answers fast, while a
// truly wedged broker fails both attempts.
var probeRetryPause = 3 * time.Second

var (
	runnerWorkerMu     sync.RWMutex
	runnerWorkerActive = defaultRunnerWorkerActive
)

func getRunnerWorkerActive() func() bool {
	runnerWorkerMu.RLock()
	defer runnerWorkerMu.RUnlock()
	return runnerWorkerActive
}

func setRunnerWorkerActive(fn func() bool) {
	runnerWorkerMu.Lock()
	defer runnerWorkerMu.Unlock()
	runnerWorkerActive = fn
}

func defaultRunnerWorkerActive() bool {
	return exec.Command("pgrep", "-f", "Runner.Worker").Run() == nil
}

// ProbeGemmaState runs a real chat completion against the warm broker and
// classifies the result. Health lies, so it generates and inspects the answer
// rather than trusting a 200. A single timeout can't distinguish WEDGED from
// merely BUSY (serialized behind real work), so a timed-out attempt is retried
// once. Read-only. Shared by the read-only liveness watch (probeGemma) and the
// router's self-healing duty so both agree on "wedged".
func ProbeGemmaState(home string) (GemmaStatus, string) {
	portRaw, err := os.ReadFile(filepath.Join(home, ".sirsi/gemma-server.port"))
	if err != nil {
		return GemmaDown, "no port file (broker not running)"
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(portRaw)))
	if err != nil || port == 0 {
		return GemmaDown, "port file unreadable"
	}

	// Weight floor: a broker with no model weights loaded onto the GPU cannot
	// serve real inference regardless of what a bare /health 200 implies. This
	// catches the "weightless broker" class (mlx_active_bytes ~0, /health ok,
	// zero generation capacity) that a restart cannot fix when the HF cache is
	// absent. Primary signal is mlx_active_bytes (ground truth, read from the
	// broker's own /health) — RSS is understated by up to 27 GB (measured
	// 2026-08-06, PRD R6) and can swing either side of any floor. RSS is used
	// ONLY when /health itself is unreachable, so the check still fails safe
	// instead of skipping entirely.
	if bytes, ok := getBrokerMLXBytesFn()(); ok {
		if bytes < minWeightedBytes {
			return GemmaWedged, fmt.Sprintf(
				"broker mlx_active_bytes=%d MB is below the %d MB weight floor — model "+WeightsAbsentSentinel+"; "+
					"a restart will not fix this (check HF model cache and re-download weights)",
				bytes/(1024*1024), minWeightedBytes/(1024*1024),
			)
		}
	} else {
		// /health unreachable even though the port file exists — fall back to
		// the RSS floor. Fail-open: if the PID file is missing or ps fails,
		// rss==0 and we skip the floor check rather than falsely classifying a
		// healthy broker as weightless.
		pidFile := filepath.Join(home, ".sirsi/gemma-server.pid")
		if rss := getBrokerRSSFn()(pidFile); rss > 0 && rss < minWeightedRSSKB {
			return GemmaWedged, fmt.Sprintf(
				"broker RSS %d MB is below the %d MB weight floor (fallback: /health unreachable) — model "+WeightsAbsentSentinel+"; "+
					"a restart will not fix this (check HF model cache and re-download weights)",
				rss/1024, minWeightedRSSKB/1024,
			)
		}
	}

	model := resolveModel(home)
	status, detail, timedOut := probeGemmaAttempt(port, model)
	if timedOut {
		// Busy vs wedged: let the blocking generation drain, then retry once.
		time.Sleep(probeRetryPause)
		status, detail, timedOut = probeGemmaAttempt(port, model)
		switch {
		case timedOut:
			if getRunnerWorkerActive()() {
				return GemmaBusy, detail + " (twice while self-hosted runner is active — degraded, not wedged)"
			}
			detail += " (twice — wedged, not just busy)"
		case status == GemmaHealthy:
			detail += " (on retry — broker was busy, not wedged)"
		}
	}
	return status, detail
}

// probeGemmaAttempt is ONE generation probe. The third return is true ONLY when
// the request itself timed out — the busy-vs-wedged ambiguity ProbeGemmaState
// resolves with a retry — never for a decisive non-200 or zero-token result.
func probeGemmaAttempt(port int, model string) (GemmaStatus, string, bool) {
	body, _ := json.Marshal(map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": "Reply with the single word: OK"}},
		"max_tokens":  32, // we assert on tokens-produced, not content, so a tiny budget is enough (cheap probe)
		"temperature": 0,
	})
	start := time.Now()
	cl := &http.Client{Timeout: probeTimeout}
	resp, err := cl.Post(fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port), "application/json", bytes.NewReader(body))
	if err != nil {
		// A timeout means the server is there but not answering in time (wedged
		// OR busy — the caller retries to tell them apart); anything else
		// (connection refused, no listener) means nothing is serving (down).
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return GemmaWedged, fmt.Sprintf("no answer in %s (timed out)", time.Since(start).Round(time.Second)), true
		}
		return GemmaDown, fmt.Sprintf("connection failed in %s (%v)", time.Since(start).Round(time.Second), err), false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GemmaWedged, fmt.Sprintf("HTTP %d (up but erroring)", resp.StatusCode), false
	}
	var out struct {
		Choices []struct {
			// A reasoning model (gemma-4) fills `reasoning` first, then `content`.
			// At a small max_tokens the whole budget is spent in `reasoning` and
			// `content` is legitimately empty on a perfectly healthy broker — so
			// "empty content" is a MODEL-BEHAVIOR assumption, not a liveness fact.
			Message struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return GemmaWedged, "unparseable response (up but not generating)", false
	}
	if len(out.Choices) == 0 {
		return GemmaWedged, fmt.Sprintf("no choices after %s (up but not generating)", time.Since(start).Round(time.Second)), false
	}
	// Transport truth, not content shape: the broker IS generating if it stopped
	// for a normal reason AND produced tokens — whether the budget landed in
	// content, in reasoning, or is only visible in usage. Reading content alone
	// false-flags a reasoning model as "wedged" and pages the owner every cycle.
	ch := out.Choices[0]
	producedTokens := out.Usage.CompletionTokens > 0 ||
		strings.TrimSpace(ch.Message.Content) != "" ||
		strings.TrimSpace(ch.Message.Reasoning) != ""
	normalFinish := ch.FinishReason == "stop" || ch.FinishReason == "length" || ch.FinishReason == ""
	if producedTokens && normalFinish {
		return GemmaHealthy, fmt.Sprintf("answered in %s (%d tok, finish=%q)", time.Since(start).Round(time.Second), out.Usage.CompletionTokens, ch.FinishReason), false
	}
	return GemmaWedged, fmt.Sprintf("no tokens generated after %s (finish=%q, up but not generating)", time.Since(start).Round(time.Second), ch.FinishReason), false
}

// probeGemma is the read-only liveness-watch wrapper over ProbeGemmaState.
func probeGemma(home string) Finding {
	f := Finding{Check: "gemma-broker", Fixable: true,
		Title: "liveness-watch: gemma broker (" + BrokerBinary + ") wedged",
		Body: "The launchd liveness watch found the warm gemma broker unresponsive " +
			"(no port, connection error, non-200, zero tokens produced, >30s, or RSS below the 1 GB weight floor). " +
			"NOTE ON THE NAME: the label is " + BrokerLabel + " but the process it starts is " + BrokerBinary + " " +
			"(Go, engine sirsi-go-mlx) — NOT the legacy python/mlx_lm gemma path, which no longer exists. " +
			"Do not act on this finding as if a Python service were involved; probing or killing by process name finds the wrong thing. " +
			"The broker is the Tier-0 substrate the router/reconcile/gemma-builder depend on. " +
			"If the detail says 'weights likely absent': the HF model cache was deleted — a restart will NOT fix this; " +
			"re-download the model weights first (`huggingface-cli download <model>`). " +
			"Otherwise the router's gemma-liveness duty auto-restores it " +
			"(load-bearing-safe, A32/ADR-040: right-size, never SIGKILL — `sirsi gemma serve --stop && sirsi gemma serve`); " +
			"this alert fires when a restore did not stick (e.g. RAM won't fit the model — free memory)."}
	status, detail := ProbeGemmaState(home)
	if suppressGemmaDown(status, launchAgentPlistPresent(BrokerLabel)) {
		f.OK = true
		f.Fixable = false
		f.Detail = detail + " — broker not installed (no " + BrokerLabel +
			".plist in LaunchAgents): deliberately absent, nothing to restore"
		return f
	}
	f.OK = status == GemmaHealthy || status == GemmaBusy
	f.Detail = detail
	return f
}

// BrokerLabel is the LaunchAgent label for the warm gemma broker. The label
// says "gemma" but the process it starts is sne-server, not python — probing
// by process name finds the wrong thing.
const BrokerLabel = "ai.sirsi.gemma-broker"

// BrokerBinary is the executable BrokerLabel actually starts. It is named in
// every owner-facing finding about the broker because the label alone has
// already caused real harm: an agent read "gemma-broker", concluded it was the
// legacy python/mlx_lm gemma service, and issued a directive to permanently
// down it — taking the Tier-0 substrate offline. The label cannot be renamed
// without a bootout/bootstrap of a live load-bearing server, so the cheaper
// repair is that no report about this service ever says only "gemma".
const BrokerBinary = "sne-server"

// suppressGemmaDown reports whether a GemmaDown result describes a DELIBERATE
// absence rather than a failure, and so must not raise a finding.
//
// A35 (scope the check to the claim): probeGemma claims "the gemma broker is
// wedged" and prescribes a restore. Its actual scope is "nothing answered on
// the port" — which is equally what a broker that was never started looks
// like. Retiring or quarantining a service moves its plist out of
// LaunchAgents, so there is nothing to bootstrap and the prescribed restore
// cannot succeed; firing anyway pages the owner every StartInterval about an
// intended state, with a guessed cause ("RAM won't fit the model") that was
// never measured. Observed 2026-08-06: 23 plists parked in
// ~/.sirsi/quarantined-wake-plists/ produced a standing alarm loop on the
// owner board while the binary and model were both present on disk.
//
// Only GemmaDown collapses this way. A wedged or busy broker is a running
// process, so whether its plist is installed says nothing about its health.
func suppressGemmaDown(status GemmaStatus, plistPresent bool) bool {
	return status == GemmaDown && !plistPresent
}

// resolveModel mirrors the worker/CLI: env > ~/.sirsi/gemma-model.conf > fallback.
func resolveModel(home string) string {
	if m := os.Getenv("GEMMA_MODEL"); m != "" {
		return m
	}
	if b, err := os.ReadFile(filepath.Join(home, ".sirsi/gemma-model.conf")); err == nil {
		if m := strings.TrimSpace(string(b)); m != "" {
			return m
		}
	}
	return "mlx-community/gemma-2-27b-it-bf16-4bit"
}

// approximateModelGB returns a rough RAM estimate for a Gemma model based on
// its name — enough to classify "fits" vs "too large" without any network or
// disk I/O beyond the conf file. Returns 0 for unknown names.
//
// Two-phase parse: (1) parameter-count bucket sets the 4-bit base; (2) the
// quantizer suffix scales it. Arms are checked largest-first — "2b" also
// matches "12b" and "32b", so ordering is load-bearing; do not reorder.
func approximateModelGB(modelID string) float64 {
	id := strings.ToLower(modelID)
	var base float64
	switch {
	case strings.Contains(id, "27b"):
		base = 14.0 // ~14 GB at 4bit
	case strings.Contains(id, "12b"):
		base = 7.0 // ~7 GB at 4bit
	case strings.Contains(id, "9b"):
		base = 5.0 // ~5 GB at 4bit
	case strings.Contains(id, "2b"):
		base = 1.5 // ~1.5 GB at 4bit
	default:
		return 0
	}
	// Scale by quantizer. "4bit" (explicit or implied) → ×1; "8bit" → ×2;
	// "bf16"/"fp16"/"16bit" without a "4bit" suffix → ×4.
	// "bf16-4bit" contains "4bit" so the first branch wins (×1, correct).
	switch {
	case strings.Contains(id, "4bit"):
		return base
	case strings.Contains(id, "8bit"):
		return base * 2
	case strings.Contains(id, "bf16"), strings.Contains(id, "fp16"), strings.Contains(id, "16bit"):
		return base * 4
	default:
		return base // unspecified → assume 4bit
	}
}

// brokerMLXActiveFn reads mlx_active_bytes from the broker's /health endpoint.
// Returns (0, nil) when the endpoint is absent or field is missing; returns
// error only for a 200 with an unparseable body. Injectable (A16/A21).
var (
	brokerMLXActiveMu sync.RWMutex
	brokerMLXActive   = defaultBrokerMLXActive
)

func getBrokerMLXActive() func(port int) (int64, error) {
	brokerMLXActiveMu.RLock()
	defer brokerMLXActiveMu.RUnlock()
	return brokerMLXActive
}

func setBrokerMLXActive(fn func(port int) (int64, error)) {
	brokerMLXActiveMu.Lock()
	defer brokerMLXActiveMu.Unlock()
	brokerMLXActive = fn
}

func defaultBrokerMLXActive(port int) (int64, error) {
	cl := &http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return 0, nil // broker not running or not answering
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil
	}
	var h struct {
		MLXActiveBytes int64 `json:"mlx_active_bytes"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&h); err != nil {
		return 0, fmt.Errorf("parse /health: %w", err)
	}
	return h.MLXActiveBytes, nil
}

// extractModelGen returns the "gemma-N" generation prefix from a lowercased
// model id (e.g. "gemma-2", "gemma-4"), or "" if the pattern is not found.
func extractModelGen(idLower string) string {
	const prefix = "gemma-"
	i := strings.Index(idLower, prefix)
	if i < 0 || i+len(prefix) >= len(idLower) {
		return ""
	}
	c := idLower[i+len(prefix)]
	if c < '1' || c > '9' {
		return ""
	}
	return idLower[i : i+len(prefix)+1]
}

// rightSizeAdvice returns a runnable command string when the active broker
// model is clearly too large for availableGB of RAM; empty string otherwise.
//
// Size authority (in order):
//  1. /health mlx_active_bytes — allocator-truthful; model weights are
//     mmap'd file-backed and excluded from ps RSS, so RSS is ~185 MB while
//     the real footprint is 37 GB. mlx_active_bytes counts the actual
//     allocator pages (already including KV cache and transient pages).
//  2. approximateModelGB — name-based fallback for when the broker is not
//     running or /health is unreachable (e.g. during a memory death spiral).
//
// Headroom: modelGB+4 in both cases. mlx_active_bytes already includes KV cache;
// approximateModelGB already scales for the quantizer suffix (8bit→×2), so
// a second ×2 would double-count (12b-8bit: base 7×2=14 GB, 2×14+4=32 vs
// actual ~11.5 GB peak). RSS is never used here.
func rightSizeAdvice(home string, availableGB float64) string {
	model := resolveModel(home)
	var modelGB float64

	// Prefer /health mlx_active_bytes (reads port from gemma-server.port).
	portRaw, err := os.ReadFile(filepath.Join(home, ".sirsi/gemma-server.port"))
	if err == nil {
		if port, pErr := strconv.Atoi(strings.TrimSpace(string(portRaw))); pErr == nil && port > 0 {
			if activeBytes, hErr := getBrokerMLXActive()(port); hErr == nil && activeBytes > 0 {
				modelGB = float64(activeBytes) / (1 << 30)
			}
		}
	}

	// Fall back to name-based estimate when broker is unreachable.
	if modelGB == 0 {
		modelGB = approximateModelGB(model)
	}

	if modelGB == 0 || modelGB+4 <= availableGB {
		return "" // unknown size or fits
	}

	label := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		label = model[i+1:]
	}
	confPath := filepath.Join(home, ".sirsi", "gemma-model.conf")

	// Offer a smaller same-generation tier if one fits.
	allTiers := []struct {
		id string
		gb float64
	}{
		{"mlx-community/gemma-2-9b-it-4bit", 5.0},
		{"mlx-community/gemma-2-2b-it-4bit", 1.5},
	}
	modelLower := strings.ToLower(model)
	for _, t := range allTiers {
		if modelGen := extractModelGen(modelLower); modelGen != "" {
			if !strings.Contains(strings.ToLower(t.id), modelGen) {
				continue // skip cross-generation downgrade
			}
		}
		if t.gb+4 <= availableGB {
			return fmt.Sprintf("Right-size command: current model %s (~%.0f GB) is too large for %.1f GB "+
				"available — switch to the %s tier: "+
				"`echo '%s' > %s && sirsi gemma serve --stop && sirsi gemma serve`",
				label, modelGB, availableGB, t.id, t.id, confPath)
		}
	}
	return fmt.Sprintf("Right-size command: current model %s (~%.0f GB) cannot fit in %.1f GB available — "+
		"stop the broker to recover RAM: `sirsi gemma serve --stop`",
		label, modelGB, availableGB)
}

// sessionLeakThreshold is how many leaked claude-desktop sessions must
// accumulate before the watch proactively alerts. Matches the `sirsi diagnose`
// leaked-sessions lever so the two surfaces agree.
const sessionLeakThreshold = 8

// sessionLeakCountFn returns (candidate count, reclaimable MB) for leaked
// sessions — the caller's own ancestry is never counted (reaper.Plan). Injectable
// (A16/A21) so the threshold branch is testable without a live process table.
var (
	sessionLeakMu    sync.RWMutex
	sessionLeakCount = func() (int, int) {
		p, err := reaper.Plan(reaper.Options{}, reaper.RealDeps())
		if err != nil {
			return 0, 0
		}
		return len(p.Candidates), p.ReclaimMBEst
	}
)

func getSessionLeakCount() func() (int, int) {
	sessionLeakMu.RLock()
	defer sessionLeakMu.RUnlock()
	return sessionLeakCount
}

func setSessionLeakCount(fn func() (int, int)) {
	sessionLeakMu.Lock()
	defer sessionLeakMu.Unlock()
	sessionLeakCount = fn
}

// probeSessionLeak surfaces a leaked-session pileup PROACTIVELY (before an
// operator runs `sirsi diagnose`) so it can be reclaimed before it swap-kills
// the broker. Alert-ONLY: it never reaps (the hard reap stays owner/doctor-
// invoked via `sirsi reap-sessions --apply` — no auto-kill from a headless net).
func probeSessionLeak() Finding {
	n, mb := getSessionLeakCount()()
	f := Finding{Check: "session-leak", Fixable: true,
		Title: "liveness-watch: leaked claude-desktop sessions piling up",
		Body: fmt.Sprintf("The launchd liveness watch found %d leaked claude-desktop task-runner sessions "+
			"(~%d MB reclaimable). These accumulate from scheduled-task/wakeup runs that never self-exit and "+
			"drive the swap-death that wedges the gemma broker (2026-07-17). Reclaim them safely — your live "+
			"session is protected: `sirsi reap-sessions` (dry-run) then `sirsi reap-sessions --apply`. Alert-only; "+
			"nothing was killed.", n, mb)}
	if n < sessionLeakThreshold {
		f.OK = true
		f.Detail = fmt.Sprintf("%d leaked (below %d threshold)", n, sessionLeakThreshold)
		return f
	}
	f.Detail = fmt.Sprintf("%d leaked sessions (~%d MB) — reap to prevent swap-death", n, mb)
	return f
}

// probeMenubar checks the SwiftUI menubar is alive by its exact executable path
// (never a bare "SirsiMenubar" that could match a build process).
func probeMenubar() Finding {
	f := Finding{Check: "menubar", Fixable: true,
		Title: "liveness-watch: menubar not running",
		Body: "The launchd liveness watch found no running SwiftUI menubar " +
			"(Sirsi Menubar.app/Contents/MacOS/SirsiMenubar). Relaunch it: open the installed " +
			"~/Applications/Sirsi Menubar.app (or rebuild via macapp/build-app.sh, then `open -n`)."}
	if runtime.GOOS != "darwin" {
		f.OK, f.Fixable, f.Detail = true, false, "not macOS"
		return f
	}
	if exec.Command("pgrep", "-f", "Sirsi Menubar.app/Contents/MacOS/SirsiMenubar").Run() == nil {
		f.OK, f.Detail = true, "running"
		return f
	}
	f.Detail = "no SwiftUI menubar process"
	return f
}

// probeMemoryDeath reads the swap/free-RAM spiral signals via guard (one shared
// definition of "dying"). It routes only the live-critical spiral; ordinary
// pressure is not a currently-owner-fixable emergency (A32: alarm only when a
// current fixable condition exists). When dying, it appends a runnable
// right-size command so the alert is actionable, not just descriptive.
func probeMemoryDeath() Finding {
	home, homeErr := os.UserHomeDir()
	md := guard.SampleMemoryDeath()
	body := fmt.Sprintf("The launchd liveness watch measured a memory death spiral: swap %.0f%% (%.1f GB), "+
		"available %.2f GB, load %.1f on %d cores. The machine is paging itself to death — this swap-kills the "+
		"gemma broker (2026-07-17). Fix: close/restart the heaviest leaked sessions (leaked claude-desktop "+
		"scheduled-task sessions have NO safe auto-reap signature — restart Claude.app to reap them) and "+
		"right-size the broker; never SIGKILL a load-bearing serving process (A32/ADR-040).",
		md.SwapPct, md.SwapUsedGB, md.AvailableGB, md.Load1, md.Cores)
	if md.Dying && homeErr == nil {
		if advice := rightSizeAdvice(home, md.AvailableGB); advice != "" {
			body += " " + advice
		}
	}
	f := Finding{Check: "memory-death", Fixable: true,
		Title: "liveness-watch: memory death spiral",
		Body:  body}
	if !md.Readable {
		f.OK, f.Fixable, f.Detail = true, false, "no signal readable"
		return f
	}
	if md.Dying {
		f.Detail = fmt.Sprintf("swap %.0f%% / available %.2f GB / load %.1f — SPIRAL", md.SwapPct, md.AvailableGB, md.Load1)
		return f
	}
	f.OK = true
	f.Detail = fmt.Sprintf("swap %.0f%% / available %.2f GB / load %.1f/%d", md.SwapPct, md.AvailableGB, md.Load1, md.Cores)
	return f
}

// launchAgentPlistPresent reports whether a loadable .plist still exists for
// label. Retiring a service renames its plist (….plist.retired-*, …
// .plist.superseded-*) rather than deleting it, and launchd's own cleanup then
// parks the orphaned label as "disabled" in the override DB. Such a label is
// intentionally dead: there is nothing to re-enable and nothing to bootstrap,
// so the repair this probe prescribes cannot succeed. Only the exact
// "<label>.plist" counts — a retired suffix must not match.
func launchAgentPlistPresent(label string) bool {
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", label+".plist")); err == nil {
			return true
		}
	}
	_, err := os.Stat(filepath.Join("/Library/LaunchAgents", label+".plist"))
	return err == nil
}

// probeLaunchdDisabled checks for ai.sirsi.* or actions.runner.* labels that
// are marked disabled in launchd's override DB. A disabled-but-running label
// is the "latency fuse" class: every current-state probe sees green, but after
// the next reboot launchd will refuse to restart it. This probe surfaces the
// flag BEFORE the reboot so the supervisor can re-enable them on its next pass.
// macOS only; fail-open on any exec error.
func probeLaunchdDisabled() Finding {
	f := Finding{
		Check:   "launchd-disabled",
		Fixable: true,
		Title:   "liveness-watch: ai.sirsi/runner labels disabled in launchd override DB",
		Body: "The launchd liveness watch found Sirsi or self-hosted-runner labels marked " +
			"disabled in launchd's override DB (launchctl print-disabled). These labels are " +
			"currently running IF launched before the disable swept them — but launchd will " +
			"NOT restart them after the next reboot. This is the 2026-07-31 fabric-loss class. " +
			"The supervisor duty (KickstartDeadLabels) now calls `launchctl enable` before " +
			"`bootstrap` to clear this flag automatically on its next pass. Alternatively fix " +
			"manually: launchctl enable gui/<uid>/<label> && launchctl bootstrap gui/<uid> " +
			"~/Library/LaunchAgents/<label>.plist — repeat per disabled label.",
	}
	if runtime.GOOS != "darwin" {
		f.OK, f.Fixable, f.Detail = true, false, "not macOS"
		return f
	}
	uid := os.Getuid()
	out, err := exec.Command("launchctl", "print-disabled", fmt.Sprintf("gui/%d", uid)).Output()
	if err != nil {
		// print-disabled may be unavailable (older macOS, restricted sandbox). Fail-open.
		f.OK, f.Fixable, f.Detail = true, false, "launchctl print-disabled unavailable"
		return f
	}
	var disabled, retired []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasSuffix(line, "=> disabled") {
			continue
		}
		start := strings.Index(line, `"`)
		end := strings.LastIndex(line, `"`)
		if start < 0 || end <= start {
			continue
		}
		label := line[start+1 : end]
		if strings.HasPrefix(label, "ai.sirsi.") || strings.HasPrefix(label, "actions.runner.") {
			if !launchAgentPlistPresent(label) {
				retired = append(retired, label)
				continue
			}
			disabled = append(disabled, label)
		}
	}
	// Report what was filtered — a silently dropped label reads as "all clear".
	retiredNote := ""
	if len(retired) > 0 {
		retiredNote = fmt.Sprintf(" (%d retired label(s) ignored, no live plist to bootstrap: %s)",
			len(retired), strings.Join(retired, ", "))
	}
	if len(disabled) == 0 {
		f.OK = true
		f.Detail = "no Sirsi/runner labels disabled" + retiredNote
		return f
	}
	f.Detail = fmt.Sprintf("%d label(s) disabled (will not start after reboot): %s",
		len(disabled), strings.Join(disabled, ", ")) + retiredNote
	f.Body += "\nDisabled now: " + strings.Join(disabled, ", ")
	// Append uid into the body for the repair command.
	f.Body = strings.ReplaceAll(f.Body, "<uid>", strconv.Itoa(uid))
	return f
}
