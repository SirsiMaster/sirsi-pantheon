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

    enum CodingKeys: String, CodingKey {
        case path, severity, description
        case sizeBytes = "size_bytes"
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

    enum CodingKeys: String, CodingKey { case check, severity, message, detail, trend }
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
    enum CodingKeys: String, CodingKey { case command, summary, evidence; case nextActions = "next_actions" }
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
        guard let data = FileManager.default.contents(atPath: scanPath),
              let res = try? JSONDecoder().decode(ScanResult.self, from: data) else {
            onTitle?("𓁢")
            return
        }
        findings = res.findings
        totalSize = res.totalSize
        scannedAt = Self.prettyDate(res.timestamp)
        onTitle?(titleLabel())
    }

    func titleLabel() -> String {
        safeBytes > 0 ? "🟡 \(Self.human(safeBytes))" : "🟢 Clean"
    }

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

    // diagnose runs `sirsi diagnose --json` and parses the health report. Uses a
    // stdout-only run so a banner on stderr can't corrupt the JSON.
    func diagnose() async {
        healthLoading = true
        let data = await Self.runJSON(args: ["diagnose", "--json"])
        if let rep = try? JSONDecoder().decode(DiagReport.self, from: data) {
            health = rep.findings
            healthStatus = rep.status ?? "green"
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

    static func firstMeaningful(_ s: String) -> String {
        for line in s.split(separator: "\n") {
            let t = line.trimmingCharacters(in: .whitespaces)
            if !t.isEmpty { return t }
        }
        return "done"
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
                let data = outPipe.fileHandleForReading.readDataToEndOfFile()
                p.waitUntilExit()
                cont.resume(returning: stripANSI(String(data: data, encoding: .utf8) ?? ""))
            }
        }
    }

    // runJSON shells `sirsi` capturing STDOUT ONLY (stderr discarded) so JSON
    // output is never corrupted by a styled banner written to stderr.
    nonisolated static func runJSON(args: [String]) async -> Data {
        await withCheckedContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let p = Process()
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
