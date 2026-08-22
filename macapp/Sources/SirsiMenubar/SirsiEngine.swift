import Foundation
import Combine

// Finding mirrors one entry in ~/.config/pantheon/findings/latest-scan.json,
// the manifest the Go cleaner persists. The Swift surface NEVER computes what is
// safe to delete — it only renders what Go already decided and shells `sirsi`
// to act. Go stays the single source of truth (and the only thing that deletes).
struct Finding: Decodable, Identifiable {
    let id = UUID()
    let path: String
    let sizeBytes: Int64
    let severity: String
    let description: String
    // Optional richer fields the Go scanner already persists — used by the
    // drill-in detail so every row can answer "what / where / whose is this."
    let rule: String?
    let category: String?
    let advisory: String?
    let remediation: String?
    let fileCount: Int?

    enum CodingKeys: String, CodingKey {
        case path, severity, description, rule, category, advisory, remediation
        case sizeBytes = "size_bytes"
        case fileCount = "file_count"
    }
}

struct ScanResult: Decodable {
    let totalSize: Int64
    let timestamp: String
    let findings: [Finding]

    enum CodingKeys: String, CodingKey {
        case timestamp, findings
        case totalSize = "total_size"
    }
}

// DiagFinding mirrors one entry of `sirsi diagnose --json`.
// severity (Go scale): 0 = OK, 1 = Info, 2 = Warn, 3 = Critical.
// trend: true for a 7-day historical pattern (Jetsam/crashes/hangs) — amber, not
// act-now red. See the green/amber/red rubric in guard.HealthStatus.
struct DiagFinding: Decodable, Identifiable {
    let id = UUID()
    let check: String
    let severity: Int
    let message: String
    let detail: String?
    let trend: Bool?
    let fix: String?   // safe CLI command that resolves this finding (nil = informational)
    let fixKind: String?  // "instant" | "relief" | "guidance" — how honest to be about the fix

    enum CodingKeys: String, CodingKey { case check, severity, message, detail, trend, fix, fixKind }
}

// DiagReport carries the findings plus the CANONICAL roll-up `status`
// (green/amber/red) the surface must show — never re-derive it from severities.
struct DiagReport: Decodable {
    let findings: [DiagFinding]
    let status: String?
}

// CommandResult is the uniform structured output the Go CLI emits for the
// scan-family commands (clean, audit, maat audit, scan, risk…): a one-line
// summary, evidence facts, and next_actions — follow-up commands the UI turns
// into buttons, including the `--confirm` applies. This contract is the spine of
// one-click remediation: the CLI tells the surface what can be done next.
struct CRFact: Decodable, Identifiable {
    let id = UUID()
    let label: String
    let value: String
    enum CodingKeys: String, CodingKey { case label, value }
}

struct CRAction: Decodable, Identifiable {
    let id = UUID()
    let label: String
    let command: String
    let description: String?
    enum CodingKeys: String, CodingKey { case label, command, description }
    // A --confirm action mutates state (trash, heal) — the UI confirms first.
    var isApply: Bool { command.contains("--confirm") }
}

struct CommandResult: Decodable {
    let command: String?
    let summary: String
    let evidence: [CRFact]
    let nextActions: [CRAction]
    let status: String?     // "ok" | "error" — drives success/failure affordances
    let errors: [String]
    enum CodingKeys: String, CodingKey { case command, summary, evidence, status, errors; case nextActions = "next_actions" }

    // Some results omit evidence/next_actions entirely (e.g. `maat audit`'s
    // honest "not inside a code repository"). Treat absent as empty so those
    // still render as the structured summary card, not the raw-text fallback.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        command = try c.decodeIfPresent(String.self, forKey: .command)
        summary = try c.decode(String.self, forKey: .summary)
        evidence = try c.decodeIfPresent([CRFact].self, forKey: .evidence) ?? []
        nextActions = try c.decodeIfPresent([CRAction].self, forKey: .nextActions) ?? []
        status = try c.decodeIfPresent(String.self, forKey: .status)
        errors = try c.decodeIfPresent([String].self, forKey: .errors) ?? []
    }

    // Did the command actually succeed? An explicit "error" status or any
    // errors entry means no — so the toast must not flash a green ✓ over it
    // (owner, 2026-07-09: a green checkmark sat above "Error: … not installed").
    var ok: Bool { status != "error" && errors.isEmpty }
}

// ── Router board (fabric liveness) ───────────────────────────────────────────
//
// The Router view reads ~/.sirsi/router-board.json — the lean board the
// claude-home conduit regenerates each cycle — falling back to shelling
// `sirsi router node-status --json` when that file is absent. Both share the
// NodeStatus contract (schema_version 1.0.0), so ONE set of Decodables covers
// both sources. The surface only RENDERS what Go already decided; it never
// re-aggregates fabric state.

// RBAgentHealth mirrors agent_health[]. The three auth outcomes are distinct on
// purpose (honest-auth, ADR-026): only needsLogin is an ACTIONABLE blocker the
// operator can clear by re-authing. degraded is an inconclusive probe (a cold CLI
// start that timed out) — informational, never an alarm. authOk==false alone is
// ambiguous, so the surface branches on needsLogin / degraded, never authOk.
struct RBAgentHealth: Decodable, Identifiable {
    var id: String { agentType }
    let agentType: String
    let cliFound: Bool
    let authOk: Bool
    let needsLogin: Bool?
    let degraded: Bool?
    let blockedItems: Int?
    let authError: String?
    enum CodingKeys: String, CodingKey {
        case agentType = "agent_type"
        case cliFound = "cli_found"
        case authOk = "auth_ok"
        case needsLogin = "needs_login"
        case degraded
        case blockedItems = "blocked_items"
        case authError = "auth_error"
    }
}

// RBLaunchAgent mirrors launch_agents[]. A router daemon that is NOT installed (or
// installed but its program is missing) is a CURRENT, fixable blocker: work
// strands until it is installed. The menubar's "router" daemons exclude the
// menubar app itself and the legacy daemon.
struct RBLaunchAgent: Decodable, Identifiable {
    var id: String { label }
    let label: String
    let role: String
    let installed: Bool
    let programFound: Bool?
    let legacy: Bool?
    enum CodingKeys: String, CodingKey {
        case label, role, installed
        case programFound = "program_found"
        case legacy
    }
    // A router-relay daemon (not the menubar app, not the legacy daemon) that is
    // missing or broken — the actionable set `sirsi router install-daemons` fixes.
    var isRouterDaemon: Bool {
        !(legacy ?? false) && role != "menubar"
    }
    var isBroken: Bool { isRouterDaemon && (!installed || !(programFound ?? true)) }
}

// RBStranded mirrors stranded_inbox[]: an agent with open items and no armed
// thread to watch them — work that sits until the agent is (re)armed.
struct RBStranded: Decodable, Identifiable {
    var id: String { agentId }
    let agentId: String
    let openItems: Int
    enum CodingKeys: String, CodingKey {
        case agentId = "agent_id"
        case openItems = "open_items"
    }
}

// RouterBoard is the decoded fabric view. Only the fields the surface renders are
// modeled; unknown fields are ignored (additive-tolerant, ADR-026).
// MemoryVitals decodes `sirsi vitals --json` — the memory-first read.
struct MemoryVitals: Decodable {
    let totalBytes: Int64
    let usedBytes: Int64
    let freeBytes: Int64
    let swapUsedBytes: Int64
    let pressure: String          // "normal" | "warn" | "critical"
    let top: [VitalsProc]?
    enum CodingKeys: String, CodingKey {
        case totalBytes = "total_bytes"
        case usedBytes = "used_bytes"
        case freeBytes = "free_bytes"
        case swapUsedBytes = "swap_used_bytes"
        case pressure, top
    }
}

struct VitalsProc: Decodable, Identifiable {
    let name: String
    let pid: Int
    let rssBytes: Int64
    var id: Int { pid }
    enum CodingKeys: String, CodingKey {
        case name, pid
        case rssBytes = "rss_bytes"
    }
}

// OwnerGated mirrors one owner_gated[] record (board schema 1.1.0): an open
// `to: user` router item awaiting the OWNER's hand. The full body is
// intentionally not on the board — the action screen shells
// `sirsi router show <id>` on open.
struct OwnerGated: Decodable, Identifiable {
    let id: String
    let title: String
    let type: String
    let from: String
    let opened: String?
    let ageHours: Double?
    let why: String?
    let refs: [String]?
    enum CodingKeys: String, CodingKey {
        case id, title, type, from, opened, why, refs
        case ageHours = "age_hours"
    }
    var ageLabel: String {
        guard let h = ageHours else { return "" }
        return h < 48 ? String(format: "%.0f h old", h) : String(format: "%.0f days old", h / 24)
    }
}


// MARK: - Fleet board (shared producer)
//
// Decoded verbatim from `sirsi router fleet --json` — the SAME computation
// Horus serves at /api/fleet. The menubar used to render its own fabric view
// from `router node-status` while Horus rendered from ledger.Build, so the two
// disagreed by construction: different read models cannot produce the same
// numbers, and no fix to either surface makes them agree. There is now one
// producer and the surfaces are consumers of it.
struct FleetLane: Decodable, Identifiable {
    let agent: String
    let state: String
    let open: Int
    let active: Int
    let stalled: Int
    let blocked: Int
    let touchedAgo: String?
    var id: String { agent }

    enum CodingKeys: String, CodingKey {
        case agent, state, open, active, stalled, blocked
        case touchedAgo = "touched_ago"
    }
}

struct FleetSummary: Decodable {
    let total: Int, done: Int, inFlight: Int, active: Int
    let assigned: Int, stalled: Int, blocked: Int
    let idleLanes: Int, pctDone: Int, lanesTotal: Int, lanesWorking: Int

    enum CodingKeys: String, CodingKey {
        case total, done, active, assigned, stalled, blocked
        case inFlight = "in_flight"
        case idleLanes = "idle_lanes"
        case pctDone = "pct_done"
        case lanesTotal = "lanes_total"
        case lanesWorking = "lanes_working"
    }
}

struct FleetBoard: Decodable {
    let summary: FleetSummary
    let lanes: [FleetLane]
}

struct RouterBoard: Decodable {
    let schemaVersion: String?
    let totalPending: Int?
    let liveThreadCount: Int?
    let agentHealth: [RBAgentHealth]?
    let launchAgents: [RBLaunchAgent]?
    let strandedInbox: [RBStranded]?
    // agent id → open item ids: the fabric's actual work map.
    let pendingByAgent: [String: [String]]?
    let ownerGated: [OwnerGated]?
    // On-device model state (board 1.2.0) — feeds the Ask Sirsi panel. `var`
    // because loadRouterBoard grafts it onto a live node-status read (which
    // does not carry local_llm) when the board file has gone stale.
    var localLLM: LocalLLM?
    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case totalPending = "total_pending"
        case liveThreadCount = "live_thread_count"
        case agentHealth = "agent_health"
        case launchAgents = "launch_agents"
        case strandedInbox = "stranded_inbox"
        case pendingByAgent = "pending_by_agent"
        case ownerGated = "owner_gated"
        case localLLM = "local_llm"
    }
}

// LocalLLM is the conduit's on-device model block (board schema 1.2.0). The
// GUI shows this as "Local AI" — model identifiers stay out of user-facing
// copy per the brand-over-model-name rule; `model` is rendered only as the
// small technical footnote line.
struct LocalLLM: Decodable {
    let port: Int?
    let healthy: Bool?
    let endpoint: String?
    let queryAPI: String?
    let model: String?
    let rssMB: Int?
    let uptime: String?
    let kvCacheCapBytes: Int64?
    enum CodingKeys: String, CodingKey {
        case port, healthy, endpoint, model, uptime
        case queryAPI = "query_api"
        case rssMB = "rss_mb"
        case kvCacheCapBytes = "kv_cache_cap_bytes"
    }
}

// ── CTR thread roster — the ambient "live threads / heartbeat" board ──────────
// Owner directive 20260709-182003: the CTR board must be an ALWAYS-VISIBLE
// passive surface, not something you run `sirsi thread list` to see. TWRow is
// one raw per-thread record from `sirsi thread list --json`; AgentHeartbeat
// aggregates them by agent (the raw list is ~72 rows, many claude-home CCD
// sessions — reference_claude_home_ccd_duplicate_records — so a per-agent roll-up
// is the at-a-glance ambient view the owner asked for).
struct TWRow: Decodable {
    let idleSeconds: Double
    let stale: Bool
    let thread: Inner
    struct Inner: Decodable {
        let agentId: String?
        let surface: String?
        let status: String?
        let watches: [String]?
        enum CodingKeys: String, CodingKey { case agentId = "agent_id", surface, status, watches }
    }
    enum CodingKeys: String, CodingKey { case idleSeconds = "idle_seconds", stale, thread }
}

// AgentHeartbeat is one roster row: an agent, its thread liveness counts, the
// freshest thread's idle age (the heartbeat), and the surfaces it runs on.
struct AgentHeartbeat: Identifiable {
    let agent: String
    let live: Int
    let idle: Int
    let staleN: Int
    let freshestIdle: Double     // seconds since the freshest thread was last seen
    let surfaces: [String]
    var id: String { agent }
    var total: Int { live + idle + staleN }
    // Status semantics match the rest of the surface (surfaces-current+actionable):
    // 🟢 something live, 💤 only idle, ⚠️ only stale (the one genuinely actionable
    // state — a stale thread may need reaping).
    var glyph: String {
        if live > 0 { return "🟢" }
        if idle > 0 { return "💤" }
        return "⚠️"
    }
    var isStale: Bool { live == 0 && idle == 0 && staleN > 0 }
    // Heartbeat pulse fraction (1 = just seen, →0 as it goes quiet over ~10 min).
    var pulse: Double { max(0, min(1, 1 - freshestIdle / 600)) }
}

// ActivityEntry is one line of the provenance ledger — every action taken from
// the UI, with cause + outcome, persisted so the user can see (and trust) what
// Pantheon did. Reversibility + provenance is what earns autonomy.
struct ActivityEntry: Codable, Identifiable {
    var id = UUID()
    let title: String
    let command: String
    let when: String
    let result: String
    enum CodingKeys: String, CodingKey { case title, command, when, result }
}

// SirsiEngine is the observable model behind every view. All deletion happens in
// the Go `sirsi` binary (safety-gated, trash-first, protected paths hardcoded);
// this type only reads the persisted scan and runs the CLI.
@MainActor
final class SirsiEngine: ObservableObject {
    @Published var findings: [Finding] = []
    @Published var totalSize: Int64 = 0
    @Published var scannedAt: String = ""
    @Published var busy = false
    @Published var lastError: String?

    // Whether the app currently holds Full Disk Access. Drives whether the
    // "Grant Full Disk Access…" row is shown — once granted, the nag disappears.
    // Re-probed on every refresh()/popover open, so granting then reopening the
    // panel clears it without a relaunch.
    @Published var hasFDA = false

    // Bumped each time the panel is (re)opened. RootView pops navigation to
    // Home on change so a reopened panel never shows a screen whose command
    // output predates the reopen (the stale-RTK-tutorial bug, 2026-07-16).
    @Published var reopenTick = 0

    // Memory-First (canon: RAM is the pre-eminent view, not storage). Live
    // vitals from `sirsi vitals --json`; drives the Home lead card.
    @Published var vitals: MemoryVitals?

    func fetchVitals() async {
        let data = await Self.runJSON(args: ["vitals", "--json"])
        if let v = try? JSONDecoder().decode(MemoryVitals.self, from: data) {
            vitals = v
        }
    }

    // Autonomous mode — the master action switch (`sirsi autonomous`, #203):
    // ON = Pantheon applies approved fixes unattended (the auto-heal loop);
    // OFF = observe + propose only. Mirrors ~/.sirsi/brain.yaml via the CLI.
    @Published var autonomousOn = false

    func fetchAutonomous() async {
        let data = await Self.runJSON(args: ["autonomous", "status", "--json"])
        if let obj = try? JSONDecoder().decode([String: Bool].self, from: data) {
            autonomousOn = obj["autonomous"] ?? false
        }
    }

    func setAutonomous(_ on: Bool) async {
        _ = await Self.run(args: ["autonomous", on ? "on" : "off"], stdin: nil)
        await fetchAutonomous()
        recordActivity(title: "Autonomous mode turned \(on ? "ON" : "OFF")",
                       command: "sirsi autonomous \(on ? "on" : "off")",
                       result: on ? "self-managing" : "observe + propose")
    }

    // Provenance ledger — actions taken from the UI, newest first.
    @Published var activity: [ActivityEntry] = []
    private let activityPath = (("~/.config/pantheon/menubar-activity.json") as NSString).expandingTildeInPath

    // macOS exposes no public, silent Full Disk Access status API. A resident
    // refresh must not touch a protected store merely to infer permission, so
    // this stays false until an explicit protected operation proves access.
    func checkFDA() { hasFDA = false }

    // Health (Horus — Ops): findings from `sirsi diagnose`.
    @Published var health: [DiagFinding] = []
    @Published var healthLoading = false
    // Canonical green/amber/red roll-up from diagnose --json `status` — the surface
    // shows THIS, never a re-derived worst-severity (which made trends read red).
    @Published var healthStatus: String = "green"
    // Issues = Warn (2) or Critical (3) in the Go severity scale; Info (1) is not.
    var healthIssueCount: Int { health.filter { $0.severity >= 2 }.count }
    var healthSummary: String {
        if health.isEmpty { return "tap to check" }
        let n = healthIssueCount
        return n == 0 ? "all healthy" : "\(n) issue\(n == 1 ? "" : "s")"
    }

    // ── Router board (fabric liveness) ───────────────────────────────────────
    @Published var routerBoard: RouterBoard?
    @Published var routerLoading = false
    @Published var fleetBoard: FleetBoard?
    @Published var fleetLoading = false
    // Non-nil when the fleet read itself failed. Rendering an empty board on a
    // failed read would say "no work" when the truth is "we could not look".
    @Published var fleetError: String?
    private let routerBoardPath = (("~/.sirsi/router-board.json") as NSString).expandingTildeInPath

    // ── Owner-facing run report (local sovereignty, owner directive
    // 2026-07-23): what the fabric DID — the supervisor/conduit write
    // ~/.sirsi/conduit-report.json (internal/report contract) and Home shows
    // the newest run as the "Last check" line, so a reboot recovery or broker
    // restore reaches the OWNER without asking.
    @Published var lastRun: ReportRun?
    private let runReportPath = (("~/.sirsi/conduit-report.json") as NSString).expandingTildeInPath

    struct ReportRun: Decodable {
        let ts: String?
        let source: String?
        let outcome: String?
        let heals: [String]?
        let escalations: [String]?
        let apiReachable: Bool?
        enum CodingKeys: String, CodingKey {
            case ts, source, outcome, heals, escalations
            case apiReachable = "api_reachable"
        }
    }
    private struct ReportFile: Decodable { let runs: [ReportRun]? }

    // loadRunReport reads the newest run (file-only; the writers own freshness).
    func loadRunReport() {
        guard let data = FileManager.default.contents(atPath: runReportPath),
              let f = try? JSONDecoder().decode(ReportFile.self, from: data) else { return }
        lastRun = f.runs?.first
    }

    // lastRunSentence mirrors internal/report.Sentence — one plain-English
    // line, same grading, so CLI and menubar never tell different stories.
    var lastRunSentence: String? {
        guard let r = lastRun else { return nil }
        var t = r.ts ?? ""
        if let ts = r.ts {
            let iso = ISO8601DateFormatter()
            if let d = iso.date(from: ts) {
                let df = DateFormatter(); df.dateFormat = "HH:mm"
                t = df.string(from: d)
            }
        }
        var body: String
        switch r.outcome {
        case "green":  body = "all green"
        case "healed": body = (r.heals?.isEmpty == false) ? r.heals!.joined(separator: "; ") : "self-healed"
        default:       body = (r.escalations?.isEmpty == false) ? "needs attention: " + r.escalations!.joined(separator: "; ") : "degraded"
        }
        if r.apiReachable == false { body += " · cloud unreachable, local AI holding the fort" }
        return "\(t) — \(body)"
    }

    // ── CTR thread roster (ambient heartbeat board) ──────────────────────────
    @Published var threadRoster: [AgentHeartbeat] = []
    @Published var threadsLoading = false
    @Published var threadsTotal = 0   // total live threads across all agents

    // loadThreads reads the live CTR board (`sirsi thread list --json`) and rolls
    // it up per agent for the ambient roster. Read-only; never blocks the UI.
    func loadThreads() async {
        threadsLoading = true
        defer { threadsLoading = false }
        let out = await Self.runJSON(args: ["thread", "list", "--json"])
        guard let rows = try? JSONDecoder().decode([TWRow].self, from: out) else {
            threadRoster = []
            return
        }
        // Aggregate by agent. "live" = active & fresh; "stale" honors the CLI's
        // own stale flag; everything else counts as idle.
        var byAgent: [String: (live: Int, idle: Int, staleN: Int, fresh: Double, surf: Set<String>)] = [:]
        for r in rows {
            let agent = r.thread.agentId ?? "unknown"
            var acc = byAgent[agent] ?? (0, 0, 0, .greatestFiniteMagnitude, [])
            if r.stale {
                acc.staleN += 1
            } else if (r.thread.status ?? "") == "active" {
                acc.live += 1
            } else {
                acc.idle += 1
            }
            acc.fresh = min(acc.fresh, r.idleSeconds)
            if let s = r.thread.surface, !s.isEmpty { acc.surf.insert(s) }
            byAgent[agent] = acc
        }
        let roster = byAgent.map { agent, a in
            AgentHeartbeat(agent: agent, live: a.live, idle: a.idle, staleN: a.staleN,
                           freshestIdle: a.fresh == .greatestFiniteMagnitude ? 0 : a.fresh,
                           surfaces: a.surf.sorted())
        }
        // Liveliest first: live agents by freshness, then idle, then stale.
        threadRoster = roster.sorted {
            if ($0.live > 0) != ($1.live > 0) { return $0.live > 0 }
            return $0.freshestIdle < $1.freshestIdle
        }
        threadsTotal = roster.reduce(0) { $0 + $1.live }
    }

    // Blockers = CURRENT, fixable conditions only (feedback_surfaces_current_
    // actionable_only). A real logout (needsLogin) and a broken router daemon are
    // blockers; a degraded/inconclusive auth probe is NOT (nothing to click clears
    // it), and neither is a stranded inbox (that's its own drillable section).
    var routerAuthBlockers: [RBAgentHealth] {
        (routerBoard?.agentHealth ?? []).filter { $0.cliFound && ($0.needsLogin ?? false) }
    }
    var routerDaemonBlockers: [RBLaunchAgent] {
        (routerBoard?.launchAgents ?? []).filter { $0.isBroken }
    }
    // Degraded (inconclusive) probes — surfaced as plain INFO, never an alarm.
    var routerDegraded: [RBAgentHealth] {
        (routerBoard?.agentHealth ?? []).filter { $0.cliFound && !$0.authOk && !($0.needsLogin ?? false) }
    }
    var routerStranded: [RBStranded] {
        (routerBoard?.strandedInbox ?? []).sorted { $0.openItems > $1.openItems }
    }
    var routerHasBlockers: Bool { !routerAuthBlockers.isEmpty || !routerDaemonBlockers.isEmpty }
    // Home-row status: red for a real blocker, amber while items WAIT on agents
    // (pending work is not "all good" — a green dot beside "48 pending" is the
    // exact contradiction the owner flagged 2026-07-22), green when idle.
    var routerStatus: String {
        if routerHasBlockers { return "red" }
        if (routerBoard?.totalPending ?? 0) > 0 { return "amber" }
        return "green"
    }
    var routerSummary: String {
        if routerLoading && routerBoard == nil { return "checking…" }
        // Honesty gate (#147 review, minor 6): a board that never loaded is
        // UNKNOWN, not healthy — "healthy" may only describe data we actually
        // read. Without this guard a missing board file + failed CLI fallback
        // rendered a false-green "healthy".
        guard routerBoard != nil else { return "no data yet" }
        if routerHasBlockers {
            let n = routerAuthBlockers.count + routerDaemonBlockers.count
            return "\(n) blocker\(n == 1 ? "" : "s")"
        }
        let pending = routerBoard?.totalPending ?? 0
        if pending > 0 { return "\(pending) pending" }
        return "healthy"
    }

    // loadRouterBoard reads ~/.sirsi/router-board.json; if absent, shells
    // `sirsi router node-status --json` (same contract). Never blocks the UI.

    // loadFleetBoard reads the shared producer. No local aggregation: the whole
    // point is that this surface renders what Horus renders.
    func loadFleetBoard() async {
        fleetLoading = true
        defer { fleetLoading = false }
        // Read the ROUTER BOARD's own output, not a parallel aggregation.
        //
        // This used to call `router fleet --json`, whose summary counts
        // differently from the board's BoardSummary (the board treats blocked as
        // a SUBSET of active; fleet reports them as separate tallies). Two
        // careful aggregations still disagree, and on 2026-08-05 the owner was
        // shown three surfaces reporting three different numbers under
        // interchangeable labels. `board-serve --once` runs the SAME code the
        // served board runs, so parity is structural rather than maintained.
        let out = await Self.runJSON(args: ["board-serve", "--once", "--shape", "fleet"])
        if let board = try? JSONDecoder().decode(FleetBoard.self, from: out) {
            fleetBoard = board
            fleetError = nil
        } else {
            // Keep the last good board on screen and say the refresh failed,
            // rather than blanking to something that reads as an empty fleet.
            fleetError = out.isEmpty ? "fleet read returned nothing" : "could not decode fleet board"
        }
    }

    func loadRouterBoard() async {
        routerLoading = true
        defer { routerLoading = false }
        // Freshness gate: the producer-written board is trusted only while
        // recent. A stale file is worse than no file — it rendered "48 pending"
        // against a live fabric of 17 (owner, 2026-07-22) — so past 5 minutes
        // we fall through to the live CLI read below.
        let attrs = try? FileManager.default.attributesOfItem(atPath: routerBoardPath)
        let fresh = (attrs?[.modificationDate] as? Date).map { Date().timeIntervalSince($0) < 300 } ?? false
        let fileBoard = FileManager.default.contents(atPath: routerBoardPath)
            .flatMap { try? JSONDecoder().decode(RouterBoard.self, from: $0) }
        if fresh, let board = fileBoard {
            routerBoard = board
            return
        }
        // Stale or missing board — read the live fabric directly (runJSON
        // captures stdout only so a banner can't corrupt the JSON). node-status
        // does NOT carry local_llm (conduit-enriched only), so graft the file's
        // block onto the live read: pending counts stay honest-live while the
        // Ask Sirsi panel keeps its last-known on-device state.
        let out = await Self.runJSON(args: ["router", "node-status", "--json"])
        if var board = try? JSONDecoder().decode(RouterBoard.self, from: out) {
            if board.localLLM == nil { board.localLLM = fileBoard?.localLLM }
            routerBoard = board
        }
    }

    // ── owner-gated items (board schema 1.1.0) ───────────────────────────────
    // Open `to: user` router items — work only the OWNER can move. The board
    // producer (claude-home's conduit) enriches these; the surface toasts NEW
    // ones and gives an action screen with real levers (read refs, mark
    // handled, reply to decisions).

    var ownerGatedItems: [OwnerGated] { routerBoard?.ownerGated ?? [] }

    // Set by the AppDelegate when the owner clicks a toast — RootView deep-links
    // straight to that item's action screen.
    @Published var pendingOwnerItemID: String?

    // Toast only genuinely-new ids: the persisted set survives restarts (no
    // backlog re-spam) and a resolved-then-reopened id never re-toasts.
    private static let toastedKey = "ownerGatedToasted"

    // newOwnerGated returns the not-yet-toasted items and marks them toasted.
    func claimNewOwnerGated() -> [OwnerGated] {
        let items = ownerGatedItems
        guard !items.isEmpty else { return [] }
        var seen = Set(UserDefaults.standard.stringArray(forKey: Self.toastedKey) ?? [])
        let fresh = items.filter { !seen.contains($0.id) }
        guard !fresh.isEmpty else { return [] }
        fresh.forEach { seen.insert($0.id) }
        UserDefaults.standard.set(Array(seen), forKey: Self.toastedKey)
        return fresh
    }

    // ownerItemBody shells `sirsi router show <id>` — the full item text lives
    // in the router, not on the lean board.
    nonisolated static func ownerItemBody(id: String) async -> String {
        await run(args: ["router", "show", id], stdin: nil)
    }

    // closeOwnerItem marks the item handled: `sirsi router close <id> --result @f`.
    func closeOwnerItem(id: String, note: String) async -> String {
        busy = true; defer { busy = false }
        let out = await Self.run(args: ["router", "close", id, "--result",
                                        note.isEmpty ? "Handled by owner via menubar." : note],
                                 stdin: nil)
        let line = Self.firstMeaningful(out)
        recordActivity(title: "Owner action — mark handled", command: "router close \(id)", result: line)
        await loadRouterBoard()
        return line
    }

    // replyOwnerDecision routes the owner's decision text BACK to the sender as
    // fresh inbound (sirsi-respond.sh notifies; a bare close is only an audit
    // trail the sender never sees).
    func replyOwnerDecision(id: String, text: String) async -> String {
        busy = true; defer { busy = false }
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let tmp = NSTemporaryDirectory() + "owner-decision-\(UUID().uuidString.prefix(8)).md"
        try? text.write(toFile: tmp, atomically: true, encoding: .utf8)
        defer { try? FileManager.default.removeItem(atPath: tmp) }
        let script = home + "/.local/bin/sirsi-respond.sh"
        let out: String
        if FileManager.default.isExecutableFile(atPath: script) {
            out = await Self.runProgram(script, args: [id, tmp])
        } else {
            // Fallback: close with the decision as the result (audit-only).
            out = await Self.run(args: ["router", "close", id, "--result", text], stdin: nil)
        }
        let line = Self.firstMeaningful(out)
        recordActivity(title: "Owner action — decision sent", command: "respond \(id)", result: line)
        await loadRouterBoard()
        return line
    }

    // installWake shells `sirsi router wake-install <agent>` to arm a stranded
    // agent's pull-loop wake channel. Returns the CLI's first line for inline
    // feedback; records provenance. Re-loads the board so the row updates.
    func installWake(agent: String) async -> String {
        busy = true; defer { busy = false }
        let out = await Self.run(args: ["router", "wake-install", agent], stdin: nil)
        let line = Self.firstMeaningful(out)
        recordActivity(title: "Arm wake channel — \(agent)", command: "router wake-install \(agent)", result: line)
        await loadRouterBoard()
        return line
    }

    // installRouterDaemons shells `sirsi router install-daemons` to (re)install the
    // missing router LaunchAgents. Records provenance and re-loads the board.
    func installRouterDaemons() async -> String {
        busy = true; defer { busy = false }
        let out = await Self.run(args: ["router", "install-daemons"], stdin: nil)
        let line = Self.firstMeaningful(out)
        recordActivity(title: "Install router daemons", command: "router install-daemons", result: line)
        await loadRouterBoard()
        return line
    }

    // ── project root (repo-scoped verbs) ─────────────────────────────────────
    //
    // Ma'at and Net weigh a CODE REPOSITORY, but the app shells `sirsi` from
    // $HOME (see run/runJSON), where those verbs honestly report "unmeasured —
    // not inside a code repository" (#170). The optional project root lets the
    // owner point them at a repo: stored in UserDefaults under the app's domain,
    // so it is settable from the UI picker or the command line:
    //   defaults write ai.sirsi.pantheon projectRoot -string ~/Development/sirsi-pantheon
    // No project configured (or an invalid path) keeps the honest unmeasured
    // default — the surface never silently weighs the wrong thing.
    nonisolated static let projectRootKey = "projectRoot"

    // The verbs that measure/act on a repository — they run from the selected
    // project root. Everything else stays pinned to $HOME. "thoth" joins so the
    // Thoth — Memory surface reads/syncs the SELECTED project's .thoth/memory.yaml
    // (owner, 2026-07-10: make Thoth project-aware like Ma'at/Net).
    nonisolated static let repoScopedVerbs: Set<String> = ["maat", "net", "risk", "osiris", "thoth"]

    // Validated project root (or nil), mirrored for the views.
    @Published var projectRoot: String?
    var projectName: String? { projectRoot.map { ($0 as NSString).lastPathComponent } }

    func loadProjectRoot() { projectRoot = Self.validatedProjectRoot() }

    func setProjectRoot(_ path: String?) {
        if let path, !path.isEmpty {
            UserDefaults.standard.set(path, forKey: Self.projectRootKey)
        } else {
            UserDefaults.standard.removeObject(forKey: Self.projectRootKey)
        }
        loadProjectRoot()
    }

    // validatedProjectRoot returns the configured root only when it is an
    // existing directory that is a git repository (.git may be a dir or, in a
    // worktree, a file). Anything else → nil, i.e. the honest default.
    nonisolated static func validatedProjectRoot() -> String? {
        guard let raw = UserDefaults.standard.string(forKey: projectRootKey), !raw.isEmpty
        else { return nil }
        let path = (raw as NSString).expandingTildeInPath
        var isDir: ObjCBool = false
        guard FileManager.default.fileExists(atPath: path, isDirectory: &isDir), isDir.boolValue,
              FileManager.default.fileExists(atPath: path + "/.git")
        else { return nil }
        return path
    }

    // discoverProjectRoots lists git repositories one level under ~/Development —
    // the picker's candidate set. Cheap: one directory listing + a .git probe.
    nonisolated static func discoverProjectRoots() -> [String] {
        let dev = FileManager.default.homeDirectoryForCurrentUser.path + "/Development"
        guard let kids = try? FileManager.default.contentsOfDirectory(atPath: dev) else { return [] }
        return kids.compactMap { name in
            guard !name.hasPrefix(".") else { return nil }
            let p = dev + "/" + name
            return FileManager.default.fileExists(atPath: p + "/.git") ? p : nil
        }.sorted()
    }

    // workingDirectory picks the child's cwd: a repo-scoped verb with a valid
    // project root runs from that repo; everything else runs from $HOME — never
    // the app's launchd cwd (/), where a path-scoped `sirsi scan` walks the
    // entire disk (the 2026-07-02 infinite-spinner bug).
    nonisolated static func workingDirectory(for args: [String]) -> URL {
        if let verb = args.first, repoScopedVerbs.contains(verb),
           let root = validatedProjectRoot() {
            return URL(fileURLWithPath: root)
        }
        return FileManager.default.homeDirectoryForCurrentUser
    }

    // Title callback so the AppDelegate can update the menubar label.
    var onTitle: ((String) -> Void)?

    private let scanPath = (("~/.config/pantheon/findings/latest-scan.json") as NSString).expandingTildeInPath
    private let healthSnapshotPath = (("~/Library/Application Support/Sirsi/Pantheon/health-snapshot.json") as NSString).expandingTildeInPath

    // SAFE = the only set the one-click surface ever trashes (regenerable,
    // trash-first). CAUTION is shown for transparency but never one-click cleaned.
    var safe: [Finding] { findings.filter { $0.severity == "safe" }.sorted { $0.sizeBytes > $1.sizeBytes } }
    var caution: [Finding] { findings.filter { $0.severity == "caution" }.sorted { $0.sizeBytes > $1.sizeBytes } }
    var safeBytes: Int64 { safe.reduce(0) { $0 + $1.sizeBytes } }
    var cautionBytes: Int64 { caution.reduce(0) { $0 + $1.sizeBytes } }

    // refresh re-reads the persisted scan (cheap; no rescan). Drives the title.
    func refresh() {
        checkFDA()
        loadHealthSnapshot()
        // Ambient CTR roster: keep the live-thread count fresh for the Home row +
        // Threads surface without the user querying (owner directive 20260709-182003).
        Task { await loadThreads() }
        guard let data = FileManager.default.contents(atPath: scanPath),
              let res = try? JSONDecoder().decode(ScanResult.self, from: data) else {
            onTitle?("")   // no scan yet → just the Eye, no waste figure
            return
        }
        findings = res.findings
        totalSize = res.totalSize
        scannedAt = Self.prettyDate(res.timestamp)
        onTitle?(titleLabel())
    }

    // Below this, reclaimable waste is trivial and must NOT colour the glyph amber
    // (230 KB of caches is not "attention"). Only meaningful reclaim counts. 1 GB.
    static let wasteThreshold: Int64 = 1_073_741_824

    // The menubar glyph is the OVERALL at-a-glance light: the worse of system
    // health (the green/amber/red rubric) and whether there is MEANINGFUL
    // reclaimable waste. The waste figure is shown only when it's worth a click.
    func titleLabel() -> String {
        return safeBytes >= Self.wasteThreshold ? Self.human(safeBytes) : ""
    }

    // titleStatus is the health band the menu-bar Eye is TINTED with — that tint
    // (set in AppDelegate) carries green/amber/red, so the icon is a branded mark
    // (Horus, the watchful protector) and NOT a bare colored dot. Defaults to
    // healthy until the first diagnose populates healthStatus.
    var titleStatus: String { healthStatus.isEmpty ? "green" : healthStatus }

    // rescan runs a fresh `sirsi scan`, then reloads.
    func rescan() async {
        busy = true; lastError = nil
        _ = await Self.run(args: ["scan"], stdin: nil)
        busy = false
        refresh()
    }

    // cleanSafe applies the SAFE-only clean via the Go CLI (no --include-caution),
    // feeding the [y/N] confirm. Returns the CLI's first result line. Trash-first.
    func cleanSafe() async -> String {
        busy = true; lastError = nil
        let out = await Self.run(args: ["anubis", "clean", "--dry-run=false"], stdin: "y\n")
        busy = false
        refresh()
        return Self.firstMeaningful(out)
    }

    // cleanSelected trashes ONLY the given paths, via the Go `--only` flag (one
    // per path). The flag is intersection-only in Go — it can never widen scope
    // beyond the safe set the scanner already approved — so a user-curated subset
    // is safe by construction. Empty selection is treated as a no-op by the
    // caller (the button is disabled), never as "clean everything."
    func cleanSelected(paths: [String]) async -> String {
        guard !paths.isEmpty else { return "Nothing selected." }
        busy = true; lastError = nil
        var args = ["anubis", "clean", "--dry-run=false"]
        for p in paths { args.append("--only"); args.append(p) }
        let out = await Self.run(args: args, stdin: "y\n")
        busy = false
        refresh()
        return Self.firstMeaningful(out)
    }

    // trashList reads what is in the Trash. Read-only, safe to call on render.
    // Returns (count, humanSize, rawLines) — empty count means nothing to purge.
    func trashList() async -> (count: Int, size: String, lines: [String]) {
        let out = await Self.run(args: ["anubis", "empty-trash"], stdin: nil)
        // "𓁟 N item(s) in Trash, SIZE total:" — parse the header, keep the
        // item lines for display. An "already empty" reply yields count 0.
        var count = 0
        var size = ""
        var lines: [String] = []
        for raw in out.split(separator: "\n", omittingEmptySubsequences: false) {
            let line = String(raw)
            if line.contains("item(s) in Trash") {
                let digits = line.split(whereSeparator: { !$0.isNumber })
                count = Int(digits.first.map(String.init) ?? "") ?? 0
                if let r = line.range(of: "Trash, "), let e = line.range(of: " total") {
                    size = String(line[r.upperBound..<e.lowerBound])
                }
            } else if line.hasPrefix("   · ") {
                lines.append(String(line.dropFirst(5)))
            }
        }
        return (count, size, lines)
    }

    // emptyTrash PERMANENTLY deletes the Trash contents. The ONLY Sirsi action
    // with no undo — every other clean path is trash-first and recoverable — so
    // the UI gates it behind an explicit second confirmation and this method is
    // never called from a one-click path.
    func emptyTrash() async -> String {
        busy = true; lastError = nil
        let out = await Self.run(args: ["anubis", "empty-trash", "--yes"], stdin: nil)
        busy = false
        refresh()
        return Self.firstMeaningful(out)
    }

    // lastDiagnoseAt throttles diagnose to once per 5 minutes: the popover used
    // to spawn a full multi-second `sirsi diagnose` on EVERY open (the 2026-07-03
    // "menubar feels slow" report — same storm class as the session-hook cache).
    // Reopens inside the window render the last-known health instantly; a fresh
    // run happens in the background only when the cache has aged out.
    private var lastDiagnoseAt: Date?
    static let diagnoseTTL: TimeInterval = 300

    // Resident Pantheon is a projection, not a probing daemon. Launch, timer,
    // and ordinary panel-open paths load this owner-approved diagnostic receipt
    // instead of running a fresh host probe that could touch a protected macOS
    // location and provoke TCC. Only an explicit diagnostic action calls
    // diagnose(force: true), which then atomically refreshes this snapshot.
    private func loadHealthSnapshot() {
        guard health.isEmpty,
              let data = FileManager.default.contents(atPath: healthSnapshotPath),
              let report = try? JSONDecoder().decode(DiagReport.self, from: data)
        else { return }
        health = report.findings
        healthStatus = report.status ?? "green"
    }

    private func persistHealthSnapshot(_ data: Data) {
        let url = URL(fileURLWithPath: healthSnapshotPath)
        do {
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(),
                                                    withIntermediateDirectories: true)
            try data.write(to: url, options: .atomic)
        } catch {
            // A failed cache write must not turn a successful explicit diagnosis
            // into a false failure. The live result remains authoritative.
        }
    }

    // diagnose runs `sirsi diagnose --json` and parses the health report. Uses a
    // stdout-only run so a banner on stderr can't corrupt the JSON.
    func diagnose(force: Bool = false) async {
        if !force, let t = lastDiagnoseAt, Date().timeIntervalSince(t) < Self.diagnoseTTL,
           !health.isEmpty {
            return  // fresh enough — render last-known instantly
        }
        lastDiagnoseAt = Date()
        healthLoading = true
        let data = await Self.runJSON(args: ["diagnose", "--json"])
        if let rep = try? JSONDecoder().decode(DiagReport.self, from: data) {
            health = rep.findings
            healthStatus = rep.status ?? "green"
            persistHealthSnapshot(data)
            onTitle?(titleLabel())   // health now drives the glyph, not just waste
        }
        healthLoading = false
    }

    // runResult runs `sirsi <args> --json` and decodes the uniform CommandResult,
    // or nil if this command doesn't emit that shape (caller falls back to raw
    // text so nothing ever dead-ends). Tolerates a banner before the JSON.
    nonisolated static func runResult(args: [String]) async -> CommandResult? {
        var a = args
        if !a.contains("--json") { a.append("--json") }
        let data = await runJSON(args: a)
        guard let s = String(data: data, encoding: .utf8),
              let i = s.firstIndex(of: "{") else { return nil }
        let cr = try? JSONDecoder().decode(CommandResult.self, from: Data(String(s[i...]).utf8))
        guard let cr, !cr.summary.isEmpty else { return nil }
        return cr
    }

    // ── provenance ledger ──────────────────────────────────────────────────────

    func loadActivity() {
        guard let data = FileManager.default.contents(atPath: activityPath),
              let all = try? JSONDecoder().decode([ActivityEntry].self, from: data) else { return }
        activity = all.reversed()   // stored oldest-first; show newest-first
    }

    // recordActivity appends one line to the on-disk ledger and the live list.
    func recordActivity(title: String, command: String, result: String) {
        let entry = ActivityEntry(title: title, command: command, when: Self.nowStamp(),
                                  result: Self.firstMeaningful(result))
        var all = [ActivityEntry]()
        if let data = FileManager.default.contents(atPath: activityPath),
           let existing = try? JSONDecoder().decode([ActivityEntry].self, from: data) {
            all = existing
        }
        all.append(entry)
        if all.count > 200 { all = Array(all.suffix(200)) }   // bounded ledger
        let dir = (activityPath as NSString).deletingLastPathComponent
        try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
        if let out = try? JSONEncoder().encode(all) {
            try? out.write(to: URL(fileURLWithPath: activityPath))
        }
        activity.insert(entry, at: 0)
    }

    static func nowStamp() -> String {
        let f = DateFormatter(); f.dateFormat = "MMM d, h:mm a"
        return f.string(from: Date())
    }

    // ── helpers ──────────────────────────────────────────────────────────────

    // humanDuration renders seconds as "3w2d" / "5h12m" / "4m" — plain reading
    // for "time since last checkpoint".
    static func humanDuration(_ seconds: Double) -> String {
        let s = Int(seconds)
        let w = s / 604_800, d = (s % 604_800) / 86_400, h = (s % 86_400) / 3_600, m = (s % 3_600) / 60
        if w > 0 { return "\(w)w\(d)d" }
        if d > 0 { return "\(d)d\(h)h" }
        if h > 0 { return "\(h)h\(m)m" }
        return "\(max(m, 1))m"
    }

    static func human(_ bytes: Int64) -> String {
        let units = ["B", "KB", "MB", "GB", "TB"]
        var v = Double(bytes); var i = 0
        while v >= 1024 && i < units.count - 1 { v /= 1024; i += 1 }
        return i == 0 ? "\(Int(v)) B" : String(format: "%.1f %@", v, units[i])
    }

    static func prettyDate(_ iso: String) -> String {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let d = f.date(from: iso) ?? ISO8601DateFormatter().date(from: iso)
        guard let d else { return iso }
        let out = DateFormatter()
        out.dateFormat = "EEE h:mm a"
        return out.string(from: d)
    }

    // summaryLine pulls a human result line from a command's output: the
    // CommandResult JSON "summary" when present (structured verbs), else the
    // first meaningful text line. Used to toast what a follow-up action did.
    static func summaryLine(_ out: String) -> String {
        if let i = out.firstIndex(of: "{"),
           let data = String(out[i...]).data(using: .utf8),
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let summary = obj["summary"] as? String, !summary.isEmpty {
            return summary
        }
        return firstMeaningful(stripANSI(out))
    }

    static func firstMeaningful(_ s: String) -> String {
        for line in s.split(separator: "\n") {
            let t = line.trimmingCharacters(in: .whitespaces)
            if !t.isEmpty { return t }
        }
        return "done"
    }

    // resultOK reports whether a command's JSON output signals success — an
    // explicit "status":"error" or any errors[] entry is a failure. Falls back
    // to true when the output isn't structured (no signal ≠ failure), but a
    // bare "Error:" prefix in unstructured text still reads as failure so the
    // toast never flashes green over an error.
    static func resultOK(_ out: String) -> Bool {
        if let i = out.firstIndex(of: "{"),
           let data = String(out[i...]).data(using: .utf8),
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            if let status = obj["status"] as? String, status == "error" { return false }
            if let errs = obj["errors"] as? [Any], !errs.isEmpty { return false }
            return true
        }
        return !firstMeaningful(stripANSI(out)).lowercased().hasPrefix("error")
    }

    // run shells the `sirsi` binary off the main actor and returns combined
    // (stdout+stderr) output. If `stdin` is non-nil it is written to the child
    // AFTER `p.run()` and the write end is then closed — the only ordering that
    // reliably answers an interactive `[y/N]` prompt (e.g. `clean --confirm`);
    // writing before launch can leave the child's stdin empty/closed, silently
    // cancelling the apply. Pass `stdin: "y\n"` to auto-confirm, nil otherwise.
    nonisolated static func run(args: [String], stdin: String?) async -> String {
        await withCheckedContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let p = Process()
                // Repo-scoped verbs (maat, net) run from the configured project
                // root so they weigh a real repository; everything else runs
                // from $HOME, never the app's launchd cwd (/), where a
                // path-scoped `sirsi scan` walks the entire disk (the
                // 2026-07-02 infinite-spinner bug).
                p.currentDirectoryURL = workingDirectory(for: args)
                p.executableURL = URL(fileURLWithPath: sirsiBinary())
                p.arguments = args
                let outPipe = Pipe()
                p.standardOutput = outPipe
                p.standardError = outPipe
                // Feed stdin AFTER launch. Writing+closing the pipe before
                // p.run() means the child's stdin can be closed/empty by the time
                // it reads the [y/N] prompt — so `clean --confirm` silently
                // cancels and nothing is trashed. Launch first, then answer.
                let inPipe = Pipe()
                if stdin != nil { p.standardInput = inPipe }
                do { try p.run() } catch {
                    cont.resume(returning: "error: \(error.localizedDescription)")
                    return
                }
                if let stdin, let d = stdin.data(using: .utf8) {
                    inPipe.fileHandleForWriting.write(d)
                    inPipe.fileHandleForWriting.closeFile()
                }
                // HARD RUNTIME BOUND. A popover button must never spin
                // forever: `maat heal` on a cold coverage cache silently ran
                // the full test suite for minutes behind a greyed screen with
                // no result and no cancel (owner, 2026-07-09: "no interaction
                // whatsoever"). The verb side is fixed to be fast, but the
                // SURFACE enforces its own bound — defense in depth, same
                // shape as the supervisor's duty timeout.
                let deadline = DispatchTime.now() + .seconds(120)
                let timeoutWork = DispatchWorkItem {
                    if p.isRunning {
                        p.terminate()
                    }
                }
                DispatchQueue.global().asyncAfter(deadline: deadline, execute: timeoutWork)
                let data = outPipe.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                timeoutWork.cancel()
                var text = stripANSI(String(data: data, encoding: .utf8) ?? "")
                if p.terminationReason == .uncaughtSignal {
                    text = "Stopped after 2 minutes — this action is taking too long for the menubar. Run `sirsi \(args.joined(separator: " "))` in a terminal to let it finish.\n" + text
                }
                cont.resume(returning: text)
            }
        }
    }

    // runProgram runs an arbitrary local executable (e.g. sirsi-respond.sh)
    // with the same $HOME cwd, output capture, and 120s bound as run().
    nonisolated static func runProgram(_ path: String, args: [String]) async -> String {
        await withCheckedContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let p = Process()
                p.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
                p.executableURL = URL(fileURLWithPath: path)
                p.arguments = args
                let outPipe = Pipe()
                p.standardOutput = outPipe
                p.standardError = outPipe
                do { try p.run() } catch {
                    cont.resume(returning: "error: \(error.localizedDescription)")
                    return
                }
                let deadline = DispatchTime.now() + .seconds(120)
                let timeoutWork = DispatchWorkItem { if p.isRunning { p.terminate() } }
                DispatchQueue.global().asyncAfter(deadline: deadline, execute: timeoutWork)
                let data = outPipe.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                timeoutWork.cancel()
                cont.resume(returning: stripANSI(String(data: data, encoding: .utf8) ?? ""))
            }
        }
    }

    // ── Local-LLM query (on-device, NEVER cloud) ─────────────────────────────
    // Owner directive 20260709-182003: NL questions about system state route to
    // local inference every time, never a cloud model. The native app calls the
    // Go `sirsi gemma` client directly; the retired ~/.local/bin/gemma Python
    // helper is not part of the application or inference path.
    nonisolated static func gemmaBinary() -> String {
        sirsiBinary()
    }
    nonisolated static func runGemma(prompt: String, system: String) async -> String {
        await withCheckedContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let bin = gemmaBinary()
                guard FileManager.default.isExecutableFile(atPath: bin) else {
                    cont.resume(returning: "Sirsi's on-device model isn't set up yet. This query stays on-device — never cloud.")
                    return
                }
                let p = Process()
                p.currentDirectoryURL = FileManager.default.homeDirectoryForCurrentUser
                p.executableURL = URL(fileURLWithPath: bin)
                p.arguments = ["gemma", "--max-tokens", "2048", system + "\n\n" + prompt]
                let outPipe = Pipe(); let errPipe = Pipe()
                p.standardOutput = outPipe
                p.standardError = errPipe   // captured as a fallback so refusals
                                            // ("start the warm broker") reach the user
                do { try p.run() } catch {
                    cont.resume(returning: "Couldn't reach Sirsi's on-device model: \(error.localizedDescription)")
                    return
                }
                let deadline = DispatchTime.now() + .seconds(60)
                let timeoutWork = DispatchWorkItem { if p.isRunning { p.terminate() } }
                DispatchQueue.global().asyncAfter(deadline: deadline, execute: timeoutWork)
                let data = outPipe.fileHandleForReading.readDataToEndOfFile()
                let errData = errPipe.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                timeoutWork.cancel()
                let text = stripANSI(String(data: data, encoding: .utf8) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                if !text.isEmpty { cont.resume(returning: text); return }
                // No stdout — the on-device model didn't answer (commonly: its
                // broker isn't running or there isn't enough free RAM to load it).
                // Keep the message BRAND-neutral: the user never sees the model
                // name (Gemma today) — Sirsi is a switchable fabric (owner,
                // 2026-07-10). The specific cause stays in errData/logs.
                let refusedForRAM = stripANSI(String(data: errData, encoding: .utf8) ?? "").lowercased().contains("ram")
                cont.resume(returning: refusedForRAM
                    ? "Sirsi's on-device model needs more free memory to answer right now. Your query stayed on-device — never cloud."
                    : "Sirsi's on-device model isn't running right now. Your query stayed on-device — never cloud.")
            }
        }
    }

    var localLLM: LocalLLM? { routerBoard?.localLLM }

    private func askSirsiSystemPrompt() -> String {
        """
        You are Ask Sirsi, the local AI interface of Sirsi Pantheon running on Cylton Collymore's Mac.
        You are part of Sirsi's on-device intelligence layer. Do not answer as a generic Google model,
        Gemini model, Gemma model, or unaffiliated chatbot. If asked what you are, say you are Ask Sirsi:
        Sirsi's local, private, on-device assistant for Pantheon and the wider Sirsi portfolio. It is OK
        to acknowledge that local model weights power you under the hood, but your operating identity is Sirsi.

        Core identity:
        - Sirsi is Cylton Collymore's system for local-first AI, agent routing, infrastructure hygiene,
          project memory, and portfolio orchestration.
        - Pantheon is the local Mac application, CLI, TUI, menubar, router, and deity-governed operations layer.
        - Ra owns routing/orchestration. Horus owns workstation visibility. Thoth preserves memory.
          Ma'at governs quality/truth. Seshat moves knowledge. Hapi governs pressure/admission.
          Seba maps hardware and architecture. Anubis/Ka handle scan, cleanup, and app remnants.
        - The router/CTR coordinates Claude, Codex, Gemini, Gemma, Qwen, and future agents through
          repo-scoped ids such as claude-pantheon, codex-pantheon, claude-home, codex-home,
          claude-finalwishes, codex-nexus, and others.
        - Claude Home is the routing owner. Codex Pantheon is an independent Pantheon review/build lane.
        - Sirsi IO is the conduit/knowledge/messaging surface that should move context between apps,
          agents, repos, and local models without making the user the message bus.
        - Hypergraph is Sirsi's event-derived knowledge graph direction: Hedera HCS as ordered event
          substrate, local replay/projection as queryable graph, and local models/accelerators for
          embeddings, relation classification, routing intelligence, and summaries.
        - The portfolio includes Sirsi Nexus, Pantheon, FinalWishes, Assiduous, Ask Eliot, Porch and Alley,
          and deck/investor material. Treat them as one Sirsi ecosystem with repo-specific boundaries.
        - The user is Cylton Collymore, founder/operator of Sirsi. Address him plainly and directly.

        Operating rules:
        - Prefer concise, useful answers. If the live context does not contain a fact, say so and offer the
          nearest Sirsi command or surface that would know.
        - Do not invent router state, PR status, financial claims, legal facts, or live system metrics.
        - Never expose hidden chain-of-thought. Return final answers only.
        - Maintain brand-over-model-name: user-facing answer is Ask Sirsi / Local AI, not vendor identity.
        - Treat the bounded canon excerpts in live context as grounding material. Where the excerpts differ
          from this summary, prefer the canon excerpts and say when live proof is needed.
        """
    }

    private func askSirsiKnowledgeRoot() -> String? {
        if let projectRoot { return projectRoot }
        let fallback = FileManager.default.homeDirectoryForCurrentUser.path + "/Development/sirsi-pantheon"
        var isDir: ObjCBool = false
        guard FileManager.default.fileExists(atPath: fallback, isDirectory: &isDir), isDir.boolValue,
              FileManager.default.fileExists(atPath: fallback + "/.git")
        else { return nil }
        return fallback
    }

    private func compactCanonExcerpt(_ text: String, limit: Int) -> String {
        let normalized = text
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\t", with: " ")
        let useful = normalized
            .split(separator: "\n", omittingEmptySubsequences: false)
            .map { line in line.trimmingCharacters(in: .whitespaces) }
            .filter { line in
                !line.isEmpty &&
                !line.hasPrefix("![") &&
                !line.hasPrefix("<img") &&
                !line.hasPrefix("| :") &&
                !line.hasPrefix("<!--")
            }
            .joined(separator: "\n")
        guard useful.count > limit else { return useful }
        return String(useful.prefix(limit)) + "\n[excerpt truncated]"
    }

    private static let askSirsiCanonDocuments: [(String, String, Int)] = [
        ("Pantheon rules", "AGENTS.md", 650),
        ("Sirsi overview", "README.md", 550),
        ("Deity registry", "docs/DEITY_REGISTRY.md", 750),
        ("Portfolio standard", "docs/SIRSI_PORTFOLIO_STANDARD.md", 550),
        ("Orchestration brain", "docs/prd/ORCHESTRATION_BRAIN.md", 750),
        ("Pantheon unification", "docs/ADR-005-PANTHEON-UNIFICATION.md", 550),
        ("Local model doctrine", "docs/ADR-034-ORCHESTRATION-BRAIN.md", 650),
        ("Knowledge substrate", "docs/ADR-019-KNOWLEDGE-SUBSTRATE.md", 600),
        ("Seshat specification", "docs/SESHAT_SPECIFICATION.md", 450),
        ("Thoth specification", "docs/THOTH_SPECIFICATION.md", 450),
        ("Thoth memory", ".thoth/memory.yaml", 450),
    ]

    func askSirsiCanonGroundingStatus() -> (value: String, detail: String, healthy: Bool) {
        guard let root = askSirsiKnowledgeRoot() else {
            return ("unavailable", "no Pantheon canon root configured or discoverable", false)
        }
        let readable = Self.askSirsiCanonDocuments.filter {
            FileManager.default.isReadableFile(atPath: root + "/" + $0.1)
        }.count
        let total = Self.askSirsiCanonDocuments.count
        guard readable > 0 else {
            return ("unavailable", "Pantheon root found, but canon files are unreadable", false)
        }
        return readable == total
            ? ("\(readable)/\(total) sources", "live bounded canon pack is readable", true)
            : ("\(readable)/\(total) sources", "canon grounding is partial", false)
    }

    private func askSirsiCanonPack() -> String {
        guard let root = askSirsiKnowledgeRoot() else {
            return "CANON PACK: no Sirsi Pantheon project root is configured or discoverable."
        }

        var chunks: [String] = []
        var total = 0
        let maxTotal = 4_400
        for (title, relative, perDocLimit) in Self.askSirsiCanonDocuments {
            guard total < maxTotal else { break }
            let path = root + "/" + relative
            guard let data = FileManager.default.contents(atPath: path),
                  let text = String(data: data, encoding: .utf8)
            else { continue }
            let remaining = max(0, maxTotal - total)
            let excerpt = compactCanonExcerpt(text, limit: min(perDocLimit, remaining))
            guard !excerpt.isEmpty else { continue }
            chunks.append("### \(title) (\(relative))\n\(excerpt)")
            total += excerpt.count
        }

        if chunks.isEmpty {
            return "CANON PACK: Sirsi Pantheon root found at \(root), but no canon files were readable."
        }
        return "CANON PACK (bounded excerpts from \(root))\n" + chunks.joined(separator: "\n\n")
    }

    private func askSirsiLiveContext() -> String {
        var lines: [String] = []
        lines.append("LIVE SIRSI CONTEXT")
        lines.append("Project root: \(projectRoot ?? "unknown")")
        if let name = projectName { lines.append("Current project: \(name)") }

        if let board = routerBoard {
            lines.append("Router pending total: \(board.totalPending ?? 0)")
            lines.append("Live thread count: \(board.liveThreadCount ?? threadsTotal)")
            let pending = (board.pendingByAgent ?? [:])
                .filter { !$0.value.isEmpty }
                .sorted { $0.key < $1.key }
                .prefix(18)
                .map { "\($0.key)=\($0.value.count)" }
                .joined(separator: ", ")
            if !pending.isEmpty { lines.append("Pending by agent: \(pending)") }
            let stranded = (board.strandedInbox ?? [])
                .sorted { $0.openItems > $1.openItems }
                .prefix(12)
                .map { "\($0.agentId)=\($0.openItems)" }
                .joined(separator: ", ")
            if !stranded.isEmpty { lines.append("Stranded inboxes: \(stranded)") }
            if !ownerGatedItems.isEmpty {
                lines.append("Owner-gated items: \(ownerGatedItems.count)")
            }
        }

        if !threadRoster.isEmpty {
            let roster = threadRoster.prefix(18).map { a in
                "\(a.agent): live=\(a.live), idle=\(a.idle), stale=\(a.staleN), surfaces=\(a.surfaces.joined(separator: "/"))"
            }.joined(separator: "\n")
            lines.append("Threads:\n\(roster)")
        }

        if let llm = localLLM {
            lines.append("Local AI health: \(llm.healthy == true ? "online" : "offline")")
            if let rss = llm.rssMB { lines.append("Local AI memory: \(SirsiEngine.human(Int64(rss) * 1_048_576))") }
            if let uptime = llm.uptime { lines.append("Local AI uptime: \(uptime)") }
        }

        lines.append("""
        KNOWLEDGE SURFACES TO MENTION WHEN RELEVANT
        CLI: sirsi, ctr, router, thread, workstream, setup, seba, hapi, thoth, seshat, maat, anubis, ka.
        TUI: terminal-guided Sirsi operation when no IDE/app surface is active.
        Menubar: local Mac operator surface for health, router fabric, owner actions, cleanup, Ask Sirsi, and thread visibility.
        Local model: Gemma/MLX is the Tier-0 reasoning engine; cloud/frontier agents bind or review where needed.
        Acceleration doctrine: ANE + MLX/GPU + Metal + multithreaded CPU are AND lanes, governed by Hapi admission.
        """)
        lines.append(askSirsiCanonPack())

        return lines.joined(separator: "\n")
    }

    private func askSirsiShortContext() -> String {
        var lines = [
            "SHORT SIRSI CONTEXT",
            "You are Ask Sirsi, the local on-device assistant for Sirsi Pantheon.",
            "Pantheon includes the Mac menubar, CLI, TUI, CTR/router fabric, cleanup, health, memory, and knowledge surfaces.",
            "Ra routes work; Horus sees the workstation; Thoth preserves memory; Ma'at governs quality; Seshat moves knowledge; Hapi/Seba govern compute pressure and hardware visibility.",
            "Hypergraph/Sirsi IO connect routed events, local knowledge, Hedera HCS direction, portfolio context, and agent coordination.",
            "Portfolio: Sirsi Nexus, Pantheon, FinalWishes, Assiduous, Ask Eliot, Porch and Alley, and the Sirsi deck.",
            "User: Cylton Collymore, founder/operator of Sirsi.",
        ]
        if let board = routerBoard {
            lines.append("Live router pending total: \(board.totalPending ?? 0); live threads: \(board.liveThreadCount ?? threadsTotal).")
        }
        return lines.joined(separator: "\n")
    }

    func askLocalAIKnowledgeReport() async -> String {
        await askLocalAI("""
        Report what you know about Sirsi now in exactly six bullets, max 18 words each:
        identity; Pantheon; router/CTR and Claude/Codex threads; Hypergraph/Sirsi IO;
        portfolio apps; Cylton Collymore.
        Do not mention generic vendor identity.
        """)
    }

    // askLocalAI POSTs a question straight to the on-device model's OpenAI-style
    // endpoint from the board feed (port-move-proof: the board carries the live
    // endpoint, we never hardcode a port). Quirks handled per the conduit's live
    // verification: the model may spend tokens in a `reasoning` field, but the
    // UI must render only final `content`; first token can lag during a model
    // swap (90s timeout, and a timeout reads as "busy loading", not an error).
    // All copy is brand-level ("Local AI") — never a model name.
    func askLocalAI(_ question: String, includeCanon: Bool = true) async -> String {
        guard let llm = localLLM, let endpoint = llm.endpoint else {
            return "Local AI state hasn't loaded yet — try again in a moment."
        }
        guard llm.healthy == true else {
            return "Local AI is offline — Sirsi restores it automatically each cycle."
        }
        guard let url = URL(string: endpoint + (llm.queryAPI ?? "/v1/chat/completions")) else {
            return "Local AI endpoint is malformed in the board feed."
        }
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.timeoutInterval = 90
        let body: [String: Any] = [
            "messages": [
                ["role": "system", "content": askSirsiSystemPrompt()],
                ["role": "user", "content": includeCanon ? askSirsiLiveContext() : askSirsiShortContext()],
                ["role": "user", "content": question],
            ],
            "max_tokens": 1_024,
            "temperature": 0.0,
            "stream": false,
            "chat_template_kwargs": ["enable_thinking": false],
        ]
        req.httpBody = try? JSONSerialization.data(withJSONObject: body)
        do {
            let (data, _) = try await URLSession.shared.data(for: req)
            struct Resp: Decodable {
                struct Choice: Decodable {
                    struct Msg: Decodable { let content: String?; let reasoning: String? }
                    let message: Msg?
                    let finishReason: String?
                    enum CodingKeys: String, CodingKey {
                        case message
                        case finishReason = "finish_reason"
                    }
                }
                let choices: [Choice]?
            }
            guard let resp = try? JSONDecoder().decode(Resp.self, from: data),
                  let choice = resp.choices?.first,
                  let msg = choice.message else {
                return "Local AI answered in a shape Sirsi didn't recognize."
            }
            let content = (msg.content ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if !content.isEmpty { return content }
            if includeCanon {
                return await askLocalAI(question, includeCanon: false)
            }
            if choice.finishReason == "length" {
                return "Local AI used its answer budget before producing final text — ask a narrower question."
            }
            return "Local AI returned no final answer. It did not expose hidden reasoning."
        } catch let e as URLError where e.code == .timedOut {
            return "Local AI is busy loading a model — try again shortly."
        } catch {
            return "Couldn't reach Local AI: \(error.localizedDescription)"
        }
    }

    // askAboutThreads answers an NL question about the live fabric using the
    // current roster as context — on-device, zero cloud tokens.
    func askAboutThreads(_ question: String) async -> String {
        let ctx = threadRoster.map { a in
            "\(a.agent): \(a.live) live, \(a.idle) idle, \(a.staleN) stale; freshest seen \(Int(a.freshestIdle))s ago; surfaces \(a.surfaces.joined(separator: "/"))"
        }.joined(separator: "\n")
        let system = "You answer questions about the Sirsi router thread fabric concisely (2-4 sentences), using ONLY the live state provided. If the state doesn't contain the answer, say so plainly."
        let prompt = "Live thread fabric (\(threadsTotal) live threads across \(threadRoster.count) agents):\n\(ctx.isEmpty ? "(no agents)" : ctx)\n\nQuestion: \(question)"
        return await Self.runGemma(prompt: prompt, system: system)
    }

    // runJSON shells `sirsi` capturing STDOUT ONLY (stderr discarded) so JSON
    // output is never corrupted by a styled banner written to stderr.
    nonisolated static func runJSON(args: [String]) async -> Data {
        await withCheckedContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let p = Process()
                // Repo-scoped verbs (maat, net) run from the configured project
                // root; everything else from $HOME — see run() above.
                p.currentDirectoryURL = workingDirectory(for: args)
                p.executableURL = URL(fileURLWithPath: sirsiBinary())
                p.arguments = args
                let outPipe = Pipe()
                p.standardOutput = outPipe
                p.standardError = FileHandle.nullDevice
                do { try p.run() } catch {
                    cont.resume(returning: Data()); return
                }
                let data = outPipe.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                cont.resume(returning: data)
            }
        }
    }

    nonisolated static func sirsiBinary() -> String {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        for c in ["\(home)/.local/bin/sirsi", "/opt/homebrew/bin/sirsi", "/usr/local/bin/sirsi"] {
            if FileManager.default.isExecutableFile(atPath: c) { return c }
        }
        return "sirsi"
    }

    nonisolated static func stripANSI(_ s: String) -> String {
        guard let re = try? NSRegularExpression(pattern: "\\x1B\\[[0-9;]*[A-Za-z]") else { return s }
        let r = NSRange(s.startIndex..., in: s)
        return re.stringByReplacingMatches(in: s, range: r, withTemplate: "")
    }
}
