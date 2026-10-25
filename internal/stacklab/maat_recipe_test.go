package stacklab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestMaatSystemOneRecipeIsComplete keeps the Stack Lab recipe from becoming
// prose that drifts away from code. Every listed source and test path must
// exist, and every independently upgradeable component must name its inputs,
// outputs, and safe upgrade recipe.
func TestMaatSystemOneRecipeIsComplete(t *testing.T) {
	type component struct {
		ID            string   `json:"id"`
		Source        []string `json:"source"`
		Tests         []string `json:"tests"`
		Inputs        []string `json:"inputs"`
		Outputs       []string `json:"outputs"`
		Writes        []string `json:"writes"`
		UpgradeRecipe []string `json:"upgrade_recipe"`
	}
	var recipe struct {
		Schema     string      `json:"schema"`
		ID         string      `json:"id"`
		Wing       string      `json:"wing"`
		Components []component `json:"components"`
	}
	path := filepath.Join("..", "..", "contracts", "stacklab", "maat-system-one-recipe-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if unmarshalErr := json.Unmarshal(raw, &recipe); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if recipe.Schema != "sirsi.stacklab.recipe.v1" || recipe.ID != "stacklab.recipe.maat-system-one" || recipe.Wing != "stacklab.wing.maat" {
		t.Fatalf("recipe identity = %+v", recipe)
	}
	want := []string{
		"maat-canon", "maat-casebook", "maat-cli", "maat-confirmed-diagnostic-repair", "maat-core", "maat-coverage",
		"maat-decision-journal", "maat-failure-memory", "maat-guided-managed-repair", "maat-horus-surface", "maat-knowledge-surface", "maat-known-failure-proposals", "maat-mcp-surface", "maat-native-release-bootstrap", "maat-native-resolution-surface", "maat-pipeline", "maat-pulse-proof-platform",
		"maat-binary-drift-updater", "maat-host-health-screen", "maat-knowledge-archive", "maat-local-metadata-liveness-watch", "maat-release-contract-preflight", "maat-release-credential-preflight", "maat-scheduler", "maat-scoped-service-lifecycle", "maat-system-one-screen", "maat-terminal-console-surface", "maat-wing-contract", "stacklab-apollo-run-planner",
	}
	got := make([]string, 0, len(recipe.Components))
	for _, component := range recipe.Components {
		got = append(got, component.ID)
		if len(component.Source) == 0 || len(component.Tests) == 0 || len(component.Inputs) == 0 || len(component.Outputs) == 0 || len(component.UpgradeRecipe) == 0 {
			t.Fatalf("component %q is not independently upgradeable: %+v", component.ID, component)
		}
		for _, listed := range append(append([]string{}, component.Source...), component.Tests...) {
			if _, statErr := os.Stat(filepath.Join("..", "..", listed)); statErr != nil {
				t.Fatalf("component %q references missing path %q: %v", component.ID, listed, statErr)
			}
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("component ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("component ids = %v, want %v", got, want)
		}
	}
	componentsByID := make(map[string]component, len(recipe.Components))
	for _, component := range recipe.Components {
		componentsByID[component.ID] = component
	}
	for _, required := range []component{
		{
			ID: "maat-knowledge-archive",
			Source: []string{
				"internal/maat/knowledge/archive.go", "internal/maat/knowledge/archive_path_unix.go",
				"internal/maat/knowledge/archive_vfs_darwin.go", "internal/maat/knowledge/archive_vfs_darwin_abi.go",
				"internal/maat/knowledge/archive_vfs_darwin_copy_cgo.go", "internal/maat/knowledge/archive_vfs_darwin_nocgo.go",
				"internal/maat/knowledge/archive_path_windows.go", "internal/maat/knowledge/archive_search.go",
				"internal/maat/knowledge/legacy.go", "internal/maat/knowledge/refresh.go", "internal/maat/decisions.go",
				"internal/seshat/adapter_claude.go", "cmd/sirsi/maatknowledge.go", "cmd/sirsi/seshat.go",
				"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
			},
			Tests: []string{
				"internal/maat/knowledge/archive_test.go", "internal/maat/knowledge/archive_search_regression_test.go",
				"internal/maat/knowledge/archive_path_unix_test.go", "internal/maat/knowledge/archive_vfs_darwin_test.go",
				"internal/maat/knowledge/archive_lock_close_darwin_test.go", "internal/maat/knowledge/knowledge_test.go",
				"internal/maat/decisions_test.go", "internal/seshat/adapter_claude_archive_test.go",
				"cmd/sirsi/maatknowledge_test.go", "macapp/Tests/SirsiMenubarTests/MaatKnowledgeArchiveTests.swift",
				"internal/mcp/maat_test.go", "internal/dashboard/dashboard_test.go",
			},
			Inputs: []string{
				"stable source/instance/native-item identity",
				"bounded full source record with extraction/schema metadata and provenance",
				"legacy Seshat snapshots selected by explicit local refresh",
				"owner resolution annotation kept separate from source versions",
			},
			Outputs: []string{
				"owner-private canonical Ma'at SQLite archive and rebuildable FTS5 index",
				"immutable content-digest versions with predecessor links and retained origins",
				"deterministic full-text/detail/history/status projections with stable bounded pagination",
				"typed per-source complete/partial/failed/unavailable/unverifiable/no-new-items coverage",
				"native knowledge view with archive search, detail, explicit local refresh, decoded coverage/history cursor, exact-revision+digest once-only owner review, and typed projection reconciliation counts/reload after confirmed success",
			},
			UpgradeRecipe: []string{
				"keep internal/maat/knowledge the sole archive and use the existing modernc SQLite dependency; never create another Seshat knowledge authority",
				"keep refresh local and explicit; never ingest/network from read-only UI or MCP; the hidden Seshat ingest/list/export aliases read or write through Ma'at's archive contract, legacy snapshot files are migration-only inputs, and failed/unsupported refreshes return typed nonzero outcomes",
				"preserve owner annotations separately; reconcile projection debt through the existing decision journal using stable annotation evidence and lock-protected idempotent append before archive acknowledgement",
				"keep remaining local adapters, online workspace consent, richer MCP/TUI/dashboard detail and cross-record conflict review explicitly open; do not infer implementation from this slice",
			},
		},
		{
			ID: "maat-local-metadata-liveness-watch",
			Source: []string{
				"internal/liveness/localwatch.go", "internal/liveness/localinstall_unix.go",
				"cmd/sirsi/livenesswatchcmd.go", "cmd/sirsi/maatrepair_liveness.go", "docs/LOCAL_LIVENESS_WATCH.md",
			},
			Tests: []string{
				"internal/liveness/localwatch_test.go", "internal/liveness/localinstall_darwin_test.go",
				"cmd/sirsi/livenesswatch_local_test.go", "cmd/sirsi/maatrepair_liveness_test.go",
			},
			Inputs: []string{"ordinary per-user HOME", "current absolute canonical engine", "bounded local OS metadata", "existing Ma'at DecisionJournal"},
			Outputs: []string{
				"typed local observation and exact Casebook screen", "unknown on failed metadata", "Ra delivery explicitly not attempted",
			},
			UpgradeRecipe: []string{
				"keep the local entry rootless before selecting HOME cwd",
				"never invoke generation or router constructors in local mode",
				"bind Ma'at repair to CurrentLocalEngine, ordinary HOME, LocalPlistContent, InspectLocalWatch, and InstallLocal; never resolve a developer checkout",
				"preserve explicit disabled intent and verify idle loaded jobs without requiring a PID",
				"use the same Screen/RecordSystemOne/Casebook contract", "retain legacy configured delivery separately",
				"require exact review, confirmation, fresh preflight and post-observation; source fixtures do not claim installed behavior",
			},
		},
	} {
		actual := componentsByID[required.ID]
		for _, field := range []struct {
			name       string
			got, wants []string
		}{
			{"source", actual.Source, required.Source}, {"tests", actual.Tests, required.Tests},
			{"inputs", actual.Inputs, required.Inputs}, {"outputs", actual.Outputs, required.Outputs},
			{"upgrade_recipe", actual.UpgradeRecipe, required.UpgradeRecipe},
		} {
			for _, want := range field.wants {
				if !contains(field.got, want) {
					t.Fatalf("component %q omits %s contract %q", required.ID, field.name, want)
				}
			}
		}
	}
	binaryDrift, ok := componentsByID["maat-binary-drift-updater"]
	if !ok {
		t.Fatal("independently upgradeable Ma'at binary-drift updater component is missing")
	}
	for _, path := range []string{
		"cmd/sirsi/maatrepair_binary_drift.go", "cmd/sirsi/fix.go", "cmd/sirsi/owned_updater_unix.go",
		"internal/selfupdate/selfupdate.go", "internal/maat/decisions.go", "internal/maat/casebook/casebook.go",
		"internal/maat/history_query.go", "cmd/sirsi/maatcasebook.go", "macapp/Sources/SirsiMenubar/MaatOperationState.swift",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
	} {
		if !contains(binaryDrift.Source, path) {
			t.Fatalf("binary-drift recipe omits implementation source %q", path)
		}
	}
	for _, path := range []string{
		"cmd/sirsi/maatrepair_binary_drift_test.go", "cmd/sirsi/owned_updater_unix_test.go",
		"internal/selfupdate/probe_bounded_unix_test.go", "internal/maat/casebook/casebook_test.go",
		"internal/maat/decisions_test.go", "macapp/Tests/SirsiMenubarTests/CoreContractsTests.swift",
		"internal/maat/history_repair_query_test.go", "internal/maat/casebook/history_repair_test.go",
		"cmd/sirsi/maatcasebook_repair_history_test.go", "macapp/Tests/SirsiMenubarTests/MaatOperationStateTests.swift",
	} {
		if !contains(binaryDrift.Tests, path) {
			t.Fatalf("binary-drift recipe omits regression suite %q", path)
		}
	}
	for _, input := range []string{
		"exact open canonical System One binary-drift finding and its evidence",
		"explicit confirmation of the immutable preview binding",
		"existing Ma'at decision journal and operation-wide pending-attempt state",
	} {
		if !contains(binaryDrift.Inputs, input) {
			t.Fatalf("binary-drift recipe omits required input %q", input)
		}
	}
	for _, output := range []string{
		"read-only Homebrew-only updater preview bound to the exact source finding and target inventory",
		"durable pre-launch claim and distinct verified, failed, or unresolved factual outcome",
		"original screen remains open when other findings or deterministic floor failures remain",
		"snapshot-bound binary repair outcome and pending-attempt fields before status/text filtering and page limiting",
	} {
		if !contains(binaryDrift.Outputs, output) {
			t.Fatalf("binary-drift recipe omits required output %q", output)
		}
	}
	for _, write := range []string{
		"existing Ma'at decision journal with the exact attempt and factual outcome",
		"only the existing Homebrew-managed Sirsi package through its supported updater after exact confirmation",
	} {
		if !contains(binaryDrift.Writes, write) {
			t.Fatalf("binary-drift recipe misstates or omits write boundary %q", write)
		}
	}
	for _, boundary := range []string{
		"accept only the canonical typed binary-drift finding; never dispatch finding text, FixHint, or arbitrary producer command",
		"atomically retain the operation-wide attempt claim before launch and refuse another updater while any prior claim lacks a terminal receipt",
		"run the existing Homebrew updater once in its owned process group with bounded cancellation, group-only termination, and direct-child reap; unknown termination retains the claim and forbids replay",
		"close only an exact binary-only screen with a passed floor; preserve other findings, failed floors, original evidence, and owner acceptance as separate state",
		"use inert updater fixtures only; source qualification does not run the live updater or claim installed release behavior",
		"retain same-snapshot OriginEvidence links in both pre-window qualification and page materialization; share Casebook overlay, preserve acceptance/outcome/attempt precedence and bounded versioned continuation; browse state never grants updater replay",
	} {
		if !contains(binaryDrift.UpgradeRecipe, boundary) {
			t.Fatalf("binary-drift recipe omits safety boundary %q", boundary)
		}
	}
	lifecycle := componentsByID["maat-scoped-service-lifecycle"]
	for _, path := range []string{"internal/setup/launchdmaintenance.go", "internal/setup/launchdmaintenance_darwin.go", "internal/setup/launchdmaintenance_image_darwin.go", "internal/setup/launchdmaintenance_journal_darwin.go", "internal/setup/launchdmaintenance_spool_darwin.go", "internal/servicedrain/maintenance.go", "internal/routerstore/spoolmaintenance_darwin.go", "cmd/sirsi/launchdmaintenance.go", "docs/runbooks/LEGACY-ROOT-RELAY-MAINTENANCE.md", "internal/setup/launchdmaintenance_test.go", "internal/setup/launchdmaintenance_spool_darwin_test.go", "internal/servicedrain/maintenance_test.go", "internal/routerstore/spoolmaintenance_test.go", "internal/routerstore/spoolmaintenance_unix_test.go", "cmd/sirsi/launchdmaintenance_test.go"} {
		if !contains(append(append([]string{}, lifecycle.Source...), lifecycle.Tests...), path) {
			t.Fatalf("selected maintenance recipe omits %q", path)
		}
	}
	if !contains(lifecycle.Outputs, "legacy root transaction status and held successor activation distinct from old remote outcome settlement") {
		t.Fatal("legacy activation must not become an old-work drain or settlement claim")
	}
	for _, boundary := range []string{
		"ordinary service routes never acquire privilege or display prompts; the separate closed local-app fallback uses existing OS admin authorization only for the selected root relay; never enable disabled services or refresh unrelated jobs",
		"keep caller drain assertion distinct from verified application drain",
		"require exact signed package, selected service and preserved unrelated-process evidence before claiming deployment",
	} {
		if !contains(lifecycle.UpgradeRecipe, boundary) {
			t.Fatalf("scoped lifecycle recipe omits safety boundary %q", boundary)
		}
	}
	memory, ok := componentsByID["maat-failure-memory"]
	if !ok {
		t.Fatal("Ma'at failure-memory component is missing")
	}
	for _, path := range []string{"internal/maat/memory.go", "internal/maat/memoryguard.go", "internal/maat/memory_test.go", "internal/maat/memoryguard_test.go", "internal/maat/memory_intake.go", "internal/maat/memory_local_events.go", "internal/maat/memory_local_disposition.go", "internal/maat/memory_local_lifecycle_test.go", "cmd/sirsi/maatmemory_local.go", "cmd/sirsi/maatmemory_local_test.go", "macapp/Sources/SirsiMenubar/MaatLocalPrevention.swift", "macapp/Tests/SirsiMenubarTests/MaatLocalPreventionTests.swift", "internal/liveness/localwatch_completion_darwin_test.go"} {
		if !contains(append(append([]string{}, memory.Source...), memory.Tests...), path) {
			t.Fatalf("Ma'at failure-memory recipe omits canonical surface %q: %+v", path, memory)
		}
	}
	if !contains(memory.Outputs, "pass/reject/unverifiable preflight receipt") ||
		!contains(memory.UpgradeRecipe, "make every recovery instruction presentation data rather than executable input") {
		t.Fatalf("Ma'at failure-memory recipe omits its fail-closed recovery contract: %+v", memory)
	}
	for _, boundary := range []string{
		"open catalog leaves nonblocking before retained-FD regular-file and size checks",
		"dispatch only the closed typed read-only active-incident guard for active exact matches; bind evaluated guard identity to embedded memoryguard.go and memory.go implementation bytes",
		"unsupported matching guard id or version is unverifiable; never dispatch terminal or out-of-scope incidents or count them as measured checks",
		"one liveness-watch operational row is implemented in source; keep the other seven rows, other projections, and installed qualification open; recurrence exclusion is not a host probe or new authorization",
	} {
		if !contains(memory.UpgradeRecipe, boundary) {
			t.Fatalf("Ma'at failure-memory recipe omits boundary %q", boundary)
		}
	}
	for _, boundary := range []string{
		"preserve Incident.Key as FailureSignature.Digest; reduce the closed local event chain without rewriting introduction records or normalizing SNE identities",
		"claim invocation before callbacks; never replay an installer from an incomplete action, intake, disposition or journal projection",
		"keep current health and future action authorization separate from historical owner disposition; after retirement a later verified recurrence appends one child under the same stable key",
	} {
		if !contains(memory.UpgradeRecipe, boundary) {
			t.Fatalf("local memory recipe omits exact G01 boundary %q", boundary)
		}
	}
	proposals, ok := componentsByID["maat-known-failure-proposals"]
	if !ok {
		t.Fatal("Ma'at known-failure proposals component is missing")
	}
	for _, path := range []string{
		"internal/maat/knownfail/proposal.go",
		"internal/maat/knownfail/proposal_test.go",
		"cmd/sirsi/maatknownfail.go",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
	} {
		if !contains(append(append([]string{}, proposals.Source...), proposals.Tests...), path) {
			t.Fatalf("Ma'at known-failure proposal recipe omits canonical surface %q: %+v", path, proposals)
		}
	}
	if !contains(proposals.Outputs, "typed local proposal queue shared by CLI JSON and native Ma'at") ||
		!contains(proposals.UpgradeRecipe, "never let an unreviewed proposal affect matching or fabric-wide recognition") {
		t.Fatalf("Ma'at known-failure proposal recipe omits its local-review boundary: %+v", proposals)
	}
	journal, ok := componentsByID["maat-decision-journal"]
	if !ok {
		t.Fatal("Ma'at decision-journal component is missing")
	}
	for _, path := range []string{
		"internal/maat/journalrepair_unix.go",
		"internal/maat/journalrepair_unix_test.go",
		"cmd/sirsi/maatcasebook.go",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
	} {
		if !contains(append(append([]string{}, journal.Source...), journal.Tests...), path) {
			t.Fatalf("Ma'at decision journal recipe omits integrity-recovery surface %q: %+v", path, journal)
		}
	}
	if !contains(journal.Outputs, "confirmation-gated preservation repair with a retained original and strict active-projection verification") ||
		!contains(journal.UpgradeRecipe, "retain parent and source descriptor identity through repair, reread source bytes before install, and refuse substitution") {
		t.Fatalf("Ma'at decision journal recipe omits retained repair contract: %+v", journal)
	}
	knowledge, ok := componentsByID["maat-knowledge-surface"]
	if !ok {
		t.Fatal("Ma'at knowledge component is missing")
	}
	for _, path := range []string{
		"internal/maat/knowledge/knowledge.go",
		"cmd/sirsi/maatknowledge.go",
		"internal/mcp/tools.go",
		"internal/dashboard/maat.go",
		"cmd/sirsi/dashboard.go",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
		"internal/maat/knowledge/knowledge_test.go",
		"internal/mcp/maat_test.go",
		"internal/dashboard/dashboard_test.go",
	} {
		if !contains(append(append([]string{}, knowledge.Source...), knowledge.Tests...), path) {
			t.Fatalf("Ma'at knowledge recipe omits canonical surface %q: %+v", path, knowledge)
		}
	}
	mcp, ok := componentsByID["maat-mcp-surface"]
	if !ok {
		t.Fatal("Ma'at MCP component is missing")
	}
	if !contains(mcp.Outputs, "read-only maat_knowledge MCP JSON projection equivalent to the CLI and Horus knowledge view") {
		t.Fatalf("Ma'at MCP recipe omits the shared knowledge projection: %+v", mcp.Outputs)
	}
	horus, ok := componentsByID["maat-horus-surface"]
	if !ok {
		t.Fatal("Ma'at Horus component is missing")
	}
	if !contains(horus.Inputs, "MaatKnowledgeProducer") || !contains(horus.Outputs, "GET /api/maat/knowledge with the same sensitivity-filtered local view as CLI and MCP") {
		t.Fatalf("Ma'at Horus recipe omits the shared knowledge projection: %+v", horus)
	}
	credentials, ok := componentsByID["maat-release-credential-preflight"]
	if !ok {
		t.Fatal("Ma'at release credential preflight component is missing")
	}
	for _, path := range []string{
		"internal/maat/credentialpreflight.go",
		"cmd/sirsi/maatpreflight.go",
		"macapp/Sources/SirsiMenubar/MaatCasebookView.swift",
		"macapp/Sources/SirsiMenubar/ReleaseWorkspaceView.swift",
		"internal/maat/credentialpreflight_test.go",
		"macapp/Tests/SirsiMenubarTests/CoreContractsTests.swift",
		"scripts/verify-maat-credential-preflight-contract.sh",
	} {
		if !contains(append(append([]string{}, credentials.Source...), credentials.Tests...), path) {
			t.Fatalf("Ma'at credential recipe omits canonical surface %q: %+v", path, credentials)
		}
	}
	scheduler, ok := componentsByID["maat-scheduler"]
	if !ok {
		t.Fatal("Ma'at scheduler component is missing")
	}
	for _, path := range []string{"cmd/sirsi/maatcapacity.go", "internal/maat/schedule/capacity.go"} {
		if !contains(scheduler.Source, path) {
			t.Fatalf("Ma'at scheduler recipe omits capacity surface %q: %+v", path, scheduler.Source)
		}
	}
	if !contains(scheduler.Outputs, "explicit per-resource core and memory capacity/floor-share control plane") ||
		!contains(scheduler.UpgradeRecipe, "keep unknown peer memory explicitly unconfigured; never guess a machine capacity") {
		t.Fatalf("Ma'at scheduler recipe omits explicit memory-capacity contract: %+v", scheduler)
	}
	maatCLI, ok := componentsByID["maat-cli"]
	if !ok {
		t.Fatal("Ma'at CLI component is missing")
	}
	if !contains(maatCLI.Tests, "cmd/sirsi/maat_scales_test.go") ||
		!contains(maatCLI.Outputs, "post-heal observation failures retain a same-policy read-only recheck and Casebook resolution path") ||
		!contains(maatCLI.UpgradeRecipe, "never repeat a mutation after an unverified post-heal observation; offer re-observation plus retained evidence") {
		t.Fatalf("Ma'at CLI recipe omits post-heal recovery contract: %+v", maatCLI)
	}
	pulse, ok := componentsByID["maat-pulse-proof-platform"]
	if !ok {
		t.Fatal("Ma'at pulse component is missing")
	}
	if !contains(pulse.Outputs, "metrics with explicit measured, partial, skipped, or unavailable test scope") ||
		!contains(pulse.UpgradeRecipe, "never serialize skipped or unavailable coverage as a real zero measurement") {
		t.Fatalf("Ma'at pulse recipe omits measurement-availability truthfulness: %+v", pulse)
	}
	terminal, ok := componentsByID["maat-terminal-console-surface"]
	if !ok {
		t.Fatal("Ma'at terminal console component is missing")
	}
	if !contains(terminal.Source, "internal/tui/screen_health.go") {
		t.Fatalf("Ma'at terminal recipe omits the Health resolution surface: %+v", terminal.Source)
	}
	if !contains(terminal.Outputs, "confirmation-gated Ma'at evidence review for every guidance-only health finding") {
		t.Fatalf("Ma'at terminal recipe omits guidance resolution behavior: %+v", terminal.Outputs)
	}
	wingRaw, err := os.ReadFile(filepath.Join("..", "..", "contracts", "stacklab", "maat-wing-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	wing, err := ValidateWing(wingRaw)
	if err != nil {
		t.Fatalf("Ma'at wing: %v", err)
	}
	if wing.Provenance.ComponentCatalog != "docs/qa/MAAT_SYSTEM_ONE_CATALOG.md" {
		t.Fatalf("component catalog = %q", wing.Provenance.ComponentCatalog)
	}
	if _, err := os.Stat(filepath.Join("..", "..", wing.Provenance.ComponentCatalog)); err != nil {
		t.Fatalf("component catalog unavailable: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
