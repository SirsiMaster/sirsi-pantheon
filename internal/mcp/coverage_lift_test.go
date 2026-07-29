package mcp

// Coverage-lift tests for the MCP tool handlers. Everything runs against
// sandboxed state: HOME is repointed at t.TempDir() so vault/notify/stele
// writes never touch the real machine, and the router handlers operate on a
// throwaway .agents/idea-router tree with an explicit guard that refuses to
// run against a live router. No live network, no t.Parallel() — these tests
// mutate process-wide state (HOME, PATH, cwd, os.Stdin/os.Stdout).

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// ── helpers ──────────────────────────────────────────────────────────────

// sandboxHome points HOME at a fresh temp dir so vault/notify/stele default
// paths resolve inside the test sandbox.
func sandboxHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// resultText extracts the first text block of a ToolResult.
func resultText(t *testing.T, res *ToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty tool result")
	}
	return res.Content[0].Text
}

// clearGitEnv unsets every GIT_* variable for the duration of the test.
// Under the Ma'at pre-push gate the suite runs inside `git push`, which
// exports GIT_DIR/GIT_PREFIX/GIT_INDEX_FILE/… — router.FindRepoRoot shells
// `git rev-parse`, which honors GIT_DIR over the working directory and
// would resolve the REAL repo instead of the sandbox (the in-process
// counterpart of the leak fixed for subprocess helpers in
// sirsi-pantheon#99). t.Setenv snapshots each value for restore.
func clearGitEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			continue
		}
		key, _, _ := strings.Cut(kv, "=")
		t.Setenv(key, "") // registers restore of the original value
		os.Unsetenv(key)
	}
}

// setupRouterRepoRoot builds a throwaway repo root with a minimal
// .agents/idea-router layout, chdirs into it, and VERIFIES that
// router.FindRepoRoot resolves to it — refusing to run if resolution would
// land on a live router (e.g. via a leaked GIT_DIR).
func setupRouterRepoRoot(t *testing.T) string {
	t.Helper()
	clearGitEnv(t)
	// Sandbox the dispatch store too: without this a test send would write a
	// row into the LIVE ~/.sirsi/router.db (the test-side-effects storm class).
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))
	root := t.TempDir()
	rdir := filepath.Join(root, ".agents", "idea-router")
	for _, d := range []string{"proposals", "reviews", "decisions", "items"} {
		if err := os.MkdirAll(filepath.Join(rdir, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	if err := os.WriteFile(filepath.Join(rdir, "state.json"), []byte(`{
		"version": 1,
		"active_topics": ["test-topic"],
		"completed_topics": ["done-topic"],
		"last_codex_read": null,
		"last_claude_read": null,
		"rules": {}
	}`), 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rdir, "agents.json"), []byte(`{
		"agents": {
			"claude-home": {"type": "claude", "command": ["true"], "cwd": "/tmp"},
			"claude-pantheon": {"type": "claude", "command": ["true"], "cwd": "/tmp"},
			"codex-pantheon": {"type": "codex", "command": ["true"], "cwd": "/tmp"}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write agents.json: %v", err)
	}

	t.Chdir(root)

	found, err := router.FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot after chdir: %v", err)
	}
	wantReal, _ := filepath.EvalSymlinks(root)
	gotReal, _ := filepath.EvalSymlinks(found)
	if gotReal != wantReal {
		t.Fatalf("FindRepoRoot resolved to %q, want sandbox %q — refusing to touch a live router", found, root)
	}
	return root
}

// extractDocID pulls the "(ID: …)" out of a router_submit result.
func extractDocID(t *testing.T, text string) string {
	t.Helper()
	i := strings.Index(text, "(ID: ")
	if i < 0 {
		t.Fatalf("no document ID in %q", text)
	}
	rest := text[i+len("(ID: "):]
	j := strings.Index(rest, ")")
	if j < 0 {
		t.Fatalf("unterminated document ID in %q", text)
	}
	return rest[:j]
}

// ── Router handlers ──────────────────────────────────────────────────────

func TestRouterHandlers_SubmitPollAckListGet(t *testing.T) {
	setupRouterRepoRoot(t)

	// Phase 3: an UNADDRESSED submit is refused — every item is addressed to
	// ONE inbox (ADR-024; the proposals/ directory this used to write to was
	// retired).
	res, err := handleRouterSubmit(map[string]interface{}{
		"type":    "proposal",
		"author":  "claude",
		"title":   "Test Proposal Alpha",
		"content": "# Test Proposal Alpha\nauthor: claude\n\nBody of the proposal.",
	})
	if err != nil {
		t.Fatalf("handleRouterSubmit: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(t, res), "addressed_to is required") {
		t.Fatalf("unaddressed submit must be refused with guidance, got: %s", resultText(t, res))
	}

	// Addressed submit lands in the target's items/ inbox via the facade.
	res, err = handleRouterSubmit(map[string]interface{}{
		"type":         "review",
		"author":       "claude",
		"title":        "Addressed Review Beta",
		"content":      "# Addressed Review Beta\nreviewer: claude\n\nPlease read.",
		"addressed_to": "codex-pantheon",
	})
	if err != nil {
		t.Fatalf("handleRouterSubmit addressed: %v", err)
	}
	if res.IsError {
		t.Fatalf("addressed submit failed: %s", resultText(t, res))
	}
	text := resultText(t, res)
	if !strings.Contains(text, "Added to codex-pantheon's inbox (items/)") {
		t.Errorf("expected items/ inbox note, got: %s", text)
	}
	addressedID := extractDocID(t, text)

	// Poll lists the canonical items/ inbox — the same view as `router pull`.
	res, err = handleRouterPoll(map[string]interface{}{"agent": "codex-pantheon"})
	if err != nil {
		t.Fatalf("handleRouterPoll: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "1 open item(s)") || !strings.Contains(text, addressedID) {
		t.Errorf("poll should list the open item, got: %s", text)
	}

	// ack only clears LEGACY state.json pending entries — the items/ inbox is
	// closed by `sirsi router close`, never by an ack.
	res, err = handleRouterPoll(map[string]interface{}{"agent": "codex-pantheon", "ack": true})
	if err != nil {
		t.Fatalf("handleRouterPoll ack: %v", err)
	}
	if !strings.Contains(resultText(t, res), "1 open item(s)") {
		t.Errorf("ack must not clear the items/ inbox, got: %s", resultText(t, res))
	}

	// Invalid since falls back to the 24h default (legacy time-based view).
	res, err = handleRouterPoll(map[string]interface{}{"since": "not-a-timestamp"})
	if err != nil {
		t.Fatalf("handleRouterPoll bad since: %v", err)
	}
	if res.IsError {
		t.Errorf("bad since should fall back to default, got error: %s", resultText(t, res))
	}

	// router_list still renders the legacy status view.
	res, err = handleRouterList(nil)
	if err != nil {
		t.Fatalf("handleRouterList: %v", err)
	}
	text = resultText(t, res)
	for _, want := range []string{"Idea Router Status", "test-topic", "done-topic"} {
		if !strings.Contains(text, want) {
			t.Errorf("router_list missing %q in: %s", want, text)
		}
	}

	// router_get serves the canonical items/ file for the dispatched id.
	res, err = handleRouterGet(map[string]interface{}{"id": addressedID})
	if err != nil {
		t.Fatalf("handleRouterGet: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "Please read.") || !strings.Contains(text, "## Instructions") {
		t.Errorf("router_get should serve the items/ markdown, got: %s", text)
	}

	// Missing and unknown IDs are error results.
	res, _ = handleRouterGet(map[string]interface{}{})
	if !res.IsError {
		t.Error("router_get without id should error")
	}
	res, _ = handleRouterGet(map[string]interface{}{"id": "no-such-doc"})
	if !res.IsError {
		t.Error("router_get with unknown id should error")
	}
}

func TestHandleRouterSubmit_InvalidAuthorAndType(t *testing.T) {
	setupRouterRepoRoot(t)

	// Phase 3: the legacy two-agent author whitelist is gone — the items
	// model carries the full multi-agent id space (claude-pantheon, gemma,
	// registry-police, …), exactly like `sirsi router send --from`.
	res, err := handleRouterSubmit(map[string]interface{}{
		"type":         "proposal",
		"author":       "mallory",
		"title":        "Any Agent Id",
		"content":      "x",
		"addressed_to": "claude-home",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.IsError {
		t.Errorf("arbitrary agent ids are valid senders in the items model, got: %s", resultText(t, res))
	}

	// The ADR-024 §5 type vocabulary is still enforced (by the store facade).
	res, _ = handleRouterSubmit(map[string]interface{}{
		"type":         "junk-type",
		"author":       "claude",
		"title":        "Bad Type",
		"content":      "x",
		"addressed_to": "codex-pantheon",
	})
	if !res.IsError {
		t.Error("unknown doc type should be rejected")
	}
}

func TestHandleRouterWait_PendingItemsReturnImmediately(t *testing.T) {
	setupRouterRepoRoot(t)

	res, err := handleRouterSubmit(map[string]interface{}{
		"type":         "proposal",
		"author":       "claude",
		"title":        "Wake Up Codex",
		"content":      "# Wake Up Codex\nauthor: claude\n\nWork arrived.",
		"addressed_to": "codex-pantheon",
	})
	if err != nil || res.IsError {
		t.Fatalf("seed submit failed: %v / %+v", err, res)
	}

	start := time.Now()
	// timeout_s above the cap also exercises the clamp branch; the pending
	// item guarantees an immediate return either way.
	res, err = handleRouterWait(map[string]interface{}{
		"agent":     "codex-pantheon",
		"timeout_s": float64(100),
	})
	if err != nil {
		t.Fatalf("handleRouterWait: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("router_wait blocked %v despite a pending item", elapsed)
	}
	text := resultText(t, res)
	if res.IsError || !strings.Contains(text, "item(s) waiting for codex-pantheon") {
		t.Errorf("expected waiting notice, got: %s", text)
	}
	if !strings.Contains(text, "Wake Up Codex") {
		t.Errorf("expected document title in waiting list, got: %s", text)
	}
}

func TestHandleRouterWait_EmptyInboxReturnsClearNote(t *testing.T) {
	setupRouterRepoRoot(t)

	// timeout_s = 0.5 truncates to 0 → single poll, immediate return.
	start := time.Now()
	res, err := handleRouterWait(map[string]interface{}{
		"agent":     "claude",
		"timeout_s": float64(0.5),
	})
	if err != nil {
		t.Fatalf("handleRouterWait: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("router_wait blocked %v on an empty inbox with expired deadline", elapsed)
	}
	if res.IsError || !strings.Contains(resultText(t, res), "no inbox items for claude") {
		t.Errorf("expected clear-inbox note, got: %s", resultText(t, res))
	}
}

func TestRouterHandlers_NoRouterFound(t *testing.T) {
	t.Chdir(t.TempDir())
	if root, err := router.FindRepoRoot(); err == nil {
		t.Skipf("environment resolves a live router at %s; skipping no-router error-path test", root)
	}

	handlers := map[string]func(map[string]interface{}) (*ToolResult, error){
		"router_poll": handleRouterPoll,
		"router_list": handleRouterList,
		"router_get":  handleRouterGet,
		"router_wait": handleRouterWait,
	}
	args := map[string]map[string]interface{}{
		"router_poll": {},
		"router_list": {},
		"router_get":  {"id": "anything"},
		"router_wait": {"agent": "claude", "timeout_s": float64(1)},
	}
	for name, h := range handlers {
		res, err := h(args[name])
		if err != nil {
			t.Fatalf("%s: unexpected Go error: %v", name, err)
		}
		if !res.IsError {
			t.Errorf("%s should return an error result without a router", name)
		}
	}
}

// ── Vault handlers ───────────────────────────────────────────────────────

func TestVaultHandlers_Lifecycle(t *testing.T) {
	sandboxHome(t)

	// Missing required args.
	res, _ := handleVaultStore(map[string]interface{}{})
	if !res.IsError {
		t.Error("vault_store without content should error")
	}
	res, _ = handleVaultSearch(map[string]interface{}{})
	if !res.IsError {
		t.Error("vault_search without query should error")
	}
	res, _ = handleVaultGet(map[string]interface{}{})
	if !res.IsError {
		t.Error("vault_get without id should error")
	}

	// Store.
	res, err := handleVaultStore(map[string]interface{}{
		"content": "alpha bravo charlie sandboxed payload for the vault",
		"source":  "unit-test",
		"tag":     "logs",
	})
	if err != nil {
		t.Fatalf("handleVaultStore: %v", err)
	}
	text := resultText(t, res)
	if res.IsError || !strings.Contains(text, "Stored in vault (ID: 1") {
		t.Fatalf("expected first entry ID 1, got: %s", text)
	}

	// Search finds it.
	res, err = handleVaultSearch(map[string]interface{}{
		"query": "bravo",
		"limit": float64(5),
	})
	if err != nil {
		t.Fatalf("handleVaultSearch: %v", err)
	}
	text = resultText(t, res)
	if res.IsError || !strings.Contains(text, "unit-test") {
		t.Errorf("search should hit the stored entry, got: %s", text)
	}

	// Get returns full content.
	res, err = handleVaultGet(map[string]interface{}{"id": float64(1)})
	if err != nil {
		t.Fatalf("handleVaultGet: %v", err)
	}
	text = resultText(t, res)
	if res.IsError || !strings.Contains(text, "alpha bravo charlie") {
		t.Errorf("get should return full content, got: %s", text)
	}

	// Unknown ID is an error result.
	res, _ = handleVaultGet(map[string]interface{}{"id": float64(9999)})
	if !res.IsError {
		t.Error("vault_get with unknown id should error")
	}

	// Stats reflect the single entry and its tag.
	res, err = handleVaultStats(nil)
	if err != nil {
		t.Fatalf("handleVaultStats: %v", err)
	}
	text = resultText(t, res)
	if res.IsError || !strings.Contains(text, "Entries: 1") || !strings.Contains(text, "logs") {
		t.Errorf("stats should show 1 tagged entry, got: %s", text)
	}
}

// ── Code index handlers ──────────────────────────────────────────────────

func TestCodeIndexAndSearchHandlers(t *testing.T) {
	sandboxHome(t)

	proj := t.TempDir()
	src := `package mathy

// AddNumbers adds two integers together for the coverage suite.
func AddNumbers(a, b int) int {
	return a + b
}
`
	if err := os.WriteFile(filepath.Join(proj, "mathy.go"), []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	res, err := handleCodeIndex(map[string]interface{}{"path": proj})
	if err != nil {
		t.Fatalf("handleCodeIndex: %v", err)
	}
	text := resultText(t, res)
	if res.IsError || !strings.Contains(text, "Code index built") {
		t.Fatalf("indexing failed: %s", text)
	}

	res, err = handleCodeSearch(map[string]interface{}{
		"query": "AddNumbers",
		"limit": float64(3),
	})
	if err != nil {
		t.Fatalf("handleCodeSearch: %v", err)
	}
	text = resultText(t, res)
	if res.IsError || !strings.Contains(text, "Code search") {
		t.Errorf("search failed: %s", text)
	}

	res, _ = handleCodeSearch(map[string]interface{}{})
	if !res.IsError {
		t.Error("code_search without query should error")
	}
}

// ── Horus handlers ───────────────────────────────────────────────────────

// writeHorusProject drops a small Go file with a type, method, and function
// so symbol extraction has something structural to report.
func writeHorusProject(t *testing.T) (dir, file string) {
	t.Helper()
	dir = t.TempDir()
	src := `package shapes

// Circle is a round thing.
type Circle struct {
	Radius float64
}

// Area returns the circle's area.
func (c Circle) Area() float64 {
	return 3.14159 * c.Radius * c.Radius
}

// NewCircle builds a Circle.
func NewCircle(r float64) Circle {
	return Circle{Radius: r}
}
`
	file = filepath.Join(dir, "shapes.go")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return dir, file
}

func TestHandleCodeSymbols(t *testing.T) {
	sandboxHome(t)
	dir, _ := writeHorusProject(t)

	res, err := handleCodeSymbols(map[string]interface{}{"path": dir})
	if err != nil {
		t.Fatalf("handleCodeSymbols: %v", err)
	}
	text := resultText(t, res)
	if res.IsError {
		t.Fatalf("symbols failed: %s", text)
	}
	for _, want := range []string{"Horus:", "Circle", "NewCircle"} {
		if !strings.Contains(text, want) {
			t.Errorf("symbols output missing %q in: %s", want, text)
		}
	}

	// Kind filter narrows to functions.
	res, err = handleCodeSymbols(map[string]interface{}{"path": dir, "kind": "func"})
	if err != nil {
		t.Fatalf("handleCodeSymbols kind filter: %v", err)
	}
	if res.IsError {
		t.Errorf("kind filter failed: %s", resultText(t, res))
	}

	// Name filter.
	res, err = handleCodeSymbols(map[string]interface{}{"path": dir, "filter": "New*"})
	if err != nil {
		t.Fatalf("handleCodeSymbols name filter: %v", err)
	}
	if res.IsError {
		t.Errorf("name filter failed: %s", resultText(t, res))
	}
}

func TestHandleCodeOutline(t *testing.T) {
	sandboxHome(t)
	_, file := writeHorusProject(t)

	res, err := handleCodeOutline(map[string]interface{}{"path": file})
	if err != nil {
		t.Fatalf("handleCodeOutline: %v", err)
	}
	text := resultText(t, res)
	if res.IsError || !strings.Contains(text, "of original file size") {
		t.Errorf("outline failed: %s", text)
	}

	// Missing path.
	res, _ = handleCodeOutline(map[string]interface{}{})
	if !res.IsError {
		t.Error("code_outline without path should error")
	}

	// Unreadable path.
	res, _ = handleCodeOutline(map[string]interface{}{"path": filepath.Join(t.TempDir(), "missing.go")})
	if !res.IsError {
		t.Error("code_outline for a missing file should error")
	}

	// Unparseable source.
	bad := filepath.Join(t.TempDir(), "broken.go")
	os.WriteFile(bad, []byte("this is not valid go source"), 0o644)
	res, _ = handleCodeOutline(map[string]interface{}{"path": bad})
	if !res.IsError {
		t.Error("code_outline for invalid Go source should error")
	}
}

func TestHandleCodeContext(t *testing.T) {
	sandboxHome(t)
	dir, _ := writeHorusProject(t)

	res, _ := handleCodeContext(map[string]interface{}{})
	if !res.IsError {
		t.Error("code_context without symbol should error")
	}

	res, err := handleCodeContext(map[string]interface{}{"path": dir, "symbol": "NewCircle"})
	if err != nil {
		t.Fatalf("handleCodeContext: %v", err)
	}
	if res.IsError {
		t.Errorf("context lookup failed: %s", resultText(t, res))
	}
	if resultText(t, res) == "" {
		t.Error("context should return text")
	}
}

// ── RTK filter handler ───────────────────────────────────────────────────

func TestHandleFilterOutput(t *testing.T) {
	sandboxHome(t)

	res, _ := handleFilterOutput(map[string]interface{}{})
	if !res.IsError {
		t.Error("filter_output without text should error")
	}

	res, err := handleFilterOutput(map[string]interface{}{
		"text": "line one\nline one\nline one\nline two\n",
	})
	if err != nil {
		t.Fatalf("handleFilterOutput: %v", err)
	}
	text := resultText(t, res)
	if res.IsError || !strings.Contains(text, "RTK Stats") {
		t.Errorf("expected RTK stats footer, got: %s", text)
	}

	// max_lines forces truncation of a long distinct-line payload.
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("distinct line number ")
		sb.WriteString(strings.Repeat("x", i%7))
		sb.WriteString("\n")
	}
	res, err = handleFilterOutput(map[string]interface{}{
		"text":      sb.String(),
		"max_lines": float64(5),
	})
	if err != nil {
		t.Fatalf("handleFilterOutput max_lines: %v", err)
	}
	if res.IsError {
		t.Errorf("truncating filter failed: %s", resultText(t, res))
	}
}

// ── Notification history handler ─────────────────────────────────────────

func TestHandleNotificationHistory(t *testing.T) {
	sandboxHome(t)
	// Neutralize the best-effort osascript toast that Store.Record fires:
	// with PATH pointing at an empty dir the exec lookup fails silently.
	t.Setenv("PATH", t.TempDir())

	// Empty store.
	res, err := handleNotificationHistory(map[string]interface{}{})
	if err != nil {
		t.Fatalf("handleNotificationHistory empty: %v", err)
	}
	if res.IsError || !strings.Contains(resultText(t, res), "No notifications yet") {
		t.Errorf("expected empty-store note, got: %s", resultText(t, res))
	}

	// Seed two entries from different sources.
	store, err := notify.Open(notify.DefaultPath())
	if err != nil {
		t.Fatalf("notify.Open: %v", err)
	}
	if recErr := store.Record(notify.Notification{
		Source: "ka", Action: "scan", Severity: "info",
		Summary: "found two ghosts", DurationMs: 42,
	}); recErr != nil {
		t.Fatalf("record ka: %v", recErr)
	}
	if recErr := store.Record(notify.Notification{
		Source: "maat", Action: "gate", Severity: "success",
		Summary: "push allowed",
	}); recErr != nil {
		t.Fatalf("record maat: %v", recErr)
	}
	store.Close()

	// Recent listing includes both, with the duration suffix.
	res, err = handleNotificationHistory(map[string]interface{}{"limit": float64(10)})
	if err != nil {
		t.Fatalf("handleNotificationHistory recent: %v", err)
	}
	text := resultText(t, res)
	for _, want := range []string{"Notification History (2 results)", "found two ghosts", "push allowed", "(42ms)"} {
		if !strings.Contains(text, want) {
			t.Errorf("history missing %q in: %s", want, text)
		}
	}

	// Source filter narrows to one.
	res, err = handleNotificationHistory(map[string]interface{}{"source": "ka"})
	if err != nil {
		t.Fatalf("handleNotificationHistory by source: %v", err)
	}
	text = resultText(t, res)
	if !strings.Contains(text, "found two ghosts") || strings.Contains(text, "push allowed") {
		t.Errorf("source filter should return only ka rows, got: %s", text)
	}
}

// ── Thoth sync handler ───────────────────────────────────────────────────

func TestHandleThothSync_GoFallback(t *testing.T) {
	sandboxHome(t)
	// Empty PATH dir: no npm thoth-sync binary (forces the Go fallback) and
	// no git (journal sync fails deterministically, which is non-fatal).
	t.Setenv("PATH", t.TempDir())

	// Without .thoth/memory.yaml the Go sync fails.
	res, err := handleThothSync(map[string]interface{}{"path": t.TempDir()})
	if err != nil {
		t.Fatalf("handleThothSync: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(t, res), "Thoth memory sync failed") {
		t.Errorf("expected memory sync failure, got: %s", resultText(t, res))
	}

	// With a memory file the sync succeeds; journal sync failure is non-fatal.
	proj := t.TempDir()
	if mkErr := os.MkdirAll(filepath.Join(proj, ".thoth"), 0o755); mkErr != nil {
		t.Fatalf("mkdir .thoth: %v", mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(proj, ".thoth", "memory.yaml"),
		[]byte("project: coverage-test\nmodule_count: 0\n"), 0o644); wErr != nil {
		t.Fatalf("write memory.yaml: %v", wErr)
	}

	res, err = handleThothSync(map[string]interface{}{"path": proj})
	if err != nil {
		t.Fatalf("handleThothSync with memory: %v", err)
	}
	if res.IsError {
		t.Fatalf("sync should not be an error result, got: %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "Memory synced") {
		t.Errorf("expected memory-synced note, got: %s", resultText(t, res))
	}
}

func TestHandleThothSync_DelegatesToNpmBinary(t *testing.T) {
	sandboxHome(t)
	binDir := t.TempDir()
	script := filepath.Join(binDir, "thoth-sync")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	t.Setenv("PATH", binDir)

	res, err := handleThothSync(map[string]interface{}{"path": t.TempDir()})
	if err != nil {
		t.Fatalf("handleThothSync delegated: %v", err)
	}
	if res.IsError || !strings.Contains(resultText(t, res), "via sirsi-thoth") {
		t.Errorf("expected npm delegation success, got: %s", resultText(t, res))
	}

	// A failing binary surfaces as an error result.
	if wErr := os.WriteFile(script, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), 0o755); wErr != nil {
		t.Fatalf("rewrite fake binary: %v", wErr)
	}
	res, err = handleThothSync(map[string]interface{}{"path": t.TempDir()})
	if err != nil {
		t.Fatalf("handleThothSync delegated failure: %v", err)
	}
	if !res.IsError || !strings.Contains(resultText(t, res), "Thoth sync (npm) failed") {
		t.Errorf("expected npm delegation failure, got: %s", resultText(t, res))
	}
}

// ── Hardware detection handler ───────────────────────────────────────────

func TestHandleDetectHardware(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping live hardware detection in short mode")
	}
	sandboxHome(t) // sandbox the stele inscription

	res, err := handleDetectHardware(nil)
	if err != nil {
		t.Fatalf("handleDetectHardware: %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatal("expected content in result")
	}
	if !res.IsError && !strings.Contains(res.Content[0].Text, "Hardware Profile Detected") {
		t.Errorf("expected profile header, got: %s", res.Content[0].Text)
	}
}

// ── Server plumbing: bare server, Run, parse errors, marshal failures ────

func TestNewBareServer_CustomIdentity(t *testing.T) {
	srv := NewBareServer("gemma-test", "9.9.9", "custom instructions here", "[gemma-test] ")
	if len(srv.tools) != 0 {
		t.Errorf("bare server should have no pre-registered tools, got %d", len(srv.tools))
	}
	if len(srv.resources) != 0 {
		t.Errorf("bare server should have no pre-registered resources, got %d", len(srv.resources))
	}

	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"t","version":"1"}}}` + "\n"
	var out bytes.Buffer
	if err := srv.RunWithIO(strings.NewReader(input), &out); err != nil {
		t.Fatalf("RunWithIO: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, _ := json.Marshal(resp.Result)
	var result InitializeResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.ServerInfo.Name != "gemma-test" {
		t.Errorf("Name = %q, want gemma-test", result.ServerInfo.Name)
	}
	if result.ServerInfo.Version != "9.9.9" {
		t.Errorf("Version = %q, want 9.9.9", result.ServerInfo.Version)
	}
	if result.Instructions != "custom instructions here" {
		t.Errorf("Instructions = %q, want custom", result.Instructions)
	}
}

func TestServerRun_StdioLoop(t *testing.T) {
	srv := NewBareServer("run-test", "0.0.1", "i", "[run-test] ")

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	restore := func() { os.Stdin, os.Stdout = oldIn, oldOut }
	defer restore()

	done := make(chan error, 1)
	go func() { done <- srv.Run() }()

	if _, wErr := inW.Write([]byte(`{"jsonrpc":"2.0","id":7,"method":"ping"}` + "\n")); wErr != nil {
		t.Fatalf("write request: %v", wErr)
	}
	inW.Close() // EOF ends the serve loop

	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("Run returned error: %v", runErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not exit on stdin EOF")
	}

	restore()
	outW.Close()
	data, err := io.ReadAll(outR)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(string(data), `"jsonrpc":"2.0"`) {
		t.Errorf("expected a JSON-RPC response on stdout, got: %s", data)
	}
}

func TestServe_ParseErrorRecovery(t *testing.T) {
	srv := NewBareServer("parse-test", "0.0.1", "i", "[parse-test] ")

	// A bare JSON string is a type error (decoder recovers and continues),
	// so the loop must emit a parse error, then still answer the ping.
	input := "\"not an object\"\n" + `{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n"
	var out bytes.Buffer
	if err := srv.RunWithIO(strings.NewReader(input), &out); err != nil {
		t.Fatalf("RunWithIO: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "Parse error") {
		t.Errorf("expected a parse error response, got: %s", text)
	}
	if !strings.Contains(text, `"id":2`) {
		t.Errorf("expected the ping to still be answered, got: %s", text)
	}
}

func TestWriteResponse_MarshalFailure(t *testing.T) {
	srv := NewBareServer("marshal-test", "0.0.1", "i", "[marshal-test] ")
	var buf bytes.Buffer
	// A channel cannot be marshaled — the failure branch logs and writes nothing.
	srv.writeResponse(&buf, Response{JSONRPC: "2.0", Result: make(chan int)})
	if buf.Len() != 0 {
		t.Errorf("nothing should be written on marshal failure, got: %s", buf.String())
	}
}
