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
struct RouterBoard: Decodable {
    let schemaVersion: String?
    let totalPending: Int?
    let agentHealth: [RBAgentHealth]?
    let launchAgents: [RBLaunchAgent]?
    let strandedInbox: [RBStranded]?
    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case totalPending = "total_pending"
        case agentHealth = "agent_health"
        case launchAgents = "launch_agents"
        case strandedInbox = "stranded_inbox"
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

    func checkFDA() { hasFDA = Self.probeFullDiskAccess() }

    // probeFullDiskAccess attempts a TCC-guarded open(); success == we have FDA.
    // TCC.db exists on every Mac and is readable only with Full Disk Access.
    nonisolated static func probeFullDiskAccess() -> Bool {
        let probe = FileManager.default.homeDirectoryForCurrentUser.path
            + "/Library/Application Support/com.apple.TCC/TCC.db"
        let fd = open(probe, O_RDONLY)
        if fd >= 0 { close(fd); return true }
        return false
    }

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
    private let routerBoardPath = (("~/.sirsi/router-board.json") as NSString).expandingTildeInPath

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
    // Home-row status: red if a real blocker, green otherwise (stranded inboxes are
    // work-to-do, not an alarm — they show a count, not a red dot).
    var routerStatus: String { routerHasBlockers ? "red" : "green" }
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
    func loadRouterBoard() async {
        routerLoading = true
        defer { routerLoading = false }
        if let data = FileManager.default.contents(atPath: routerBoardPath),
           let board = try? JSONDecoder().decode(RouterBoard.self, from: data) {
            routerBoard = board
            return
        }
        // Fallback: the conduit hasn't written the lean board — read the live fabric
        // directly. runJSON captures stdout only so a banner can't corrupt the JSON.
        let out = await Self.runJSON(args: ["router", "node-status", "--json"])
        if let board = try? JSONDecoder().decode(RouterBoard.self, from: out) {
            routerBoard = board
        }
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

    // SAFE = the only set the one-click surface ever trashes (regenerable,
    // trash-first). CAUTION is shown for transparency but never one-click cleaned.
    var safe: [Finding] { findings.filter { $0.severity == "safe" }.sorted { $0.sizeBytes > $1.sizeBytes } }
    var caution: [Finding] { findings.filter { $0.severity == "caution" }.sorted { $0.sizeBytes > $1.sizeBytes } }
    var safeBytes: Int64 { safe.reduce(0) { $0 + $1.sizeBytes } }
    var cautionBytes: Int64 { caution.reduce(0) { $0 + $1.sizeBytes } }

    // refresh re-reads the persisted scan (cheap; no rescan). Drives the title.
    func refresh() {
        checkFDA()
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

    // lastDiagnoseAt throttles diagnose to once per 5 minutes: the popover used
    // to spawn a full multi-second `sirsi diagnose` on EVERY open (the 2026-07-03
    // "menubar feels slow" report — same storm class as the session-hook cache).
    // Reopens inside the window render the last-known health instantly; a fresh
    // run happens in the background only when the cache has aged out.
    private var lastDiagnoseAt: Date?
    static let diagnoseTTL: TimeInterval = 300

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

    // ── Local-LLM query (on-device, NEVER cloud) ─────────────────────────────
    // Owner directive 20260709-182003: NL questions about system state route to
    // local Gemma every time (127.0.0.1:11434 warm MLX server via ~/.local/bin/gemma),
    // never a cloud model. Cloud is only ever reached on an explicit escalate.
    nonisolated static func gemmaBinary() -> String {
        NSHomeDirectory() + "/.local/bin/gemma"
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
                p.arguments = ["-s", system, prompt]
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
