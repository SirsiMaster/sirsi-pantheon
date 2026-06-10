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

// DiagFinding mirrors one entry of `sirsi diagnose --json` — the health→cause
// surface (RAM, Swap, Disk, Spotlight storm, Jetsam/panic trends, binary drift).
// severity: 0 = OK, 1 = Warn, 2 = Critical.
struct DiagFinding: Decodable, Identifiable {
    let id = UUID()
    let check: String
    let severity: Int
    let message: String
    let detail: String?
}

struct DiagReport: Decodable { let findings: [DiagFinding] }

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

    // Health (Horus — Ops): findings from `sirsi diagnose`.
    @Published var health: [DiagFinding] = []
    @Published var healthLoading = false
    var healthWorst: Int { health.map(\.severity).max() ?? 0 }
    var healthSummary: String {
        if health.isEmpty { return "tap to check" }
        let issues = health.filter { $0.severity >= 1 }.count
        return issues == 0 ? "all healthy" : "\(issues) issue\(issues == 1 ? "" : "s")"
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
        }
        healthLoading = false
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

    // run shells the `sirsi` binary off the main actor and returns combined output.
    nonisolated static func run(args: [String], stdin: String?) async -> String {
        await withCheckedContinuation { cont in
            DispatchQueue.global(qos: .userInitiated).async {
                let p = Process()
                p.executableURL = URL(fileURLWithPath: sirsiBinary())
                p.arguments = args
                let outPipe = Pipe()
                p.standardOutput = outPipe
                p.standardError = outPipe
                if let stdin {
                    let inPipe = Pipe()
                    p.standardInput = inPipe
                    inPipe.fileHandleForWriting.write(stdin.data(using: .utf8)!)
                    inPipe.fileHandleForWriting.closeFile()
                }
                do { try p.run() } catch {
                    cont.resume(returning: "error: \(error.localizedDescription)")
                    return
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
