import Foundation
import Combine

// Presentation of the explicit Go source-import contract, never a second
// policy evaluator. In particular a provider string cannot classify a packet.
enum MaatSourceImportContract {
    static let id = "maat.adr070.source-screen"
    static let version = 2
    static let guidance = "Use contract maat.adr070.source-screen, schema_version 2, a source subject with explicit boundary, score/confidence, findings (including evidence_id), floor/checks, and model with evidence_set_digest, calibration_id and calibration: unqualified. Zero/false fields are required; keys are exact-case. Host observations use Observe this Mac instead."
    // Same inert example as contracts/maat/source-screen-v2-fixture.json;
    // cross-language fixtures keep this copyable format from silently drifting.
    static let example = #"""
    {
      "contract":"maat.adr070.source-screen","schema_version":2,
      "subject":{"kind":"commit","repo":"fixture/repo","ref":"main","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","boundary":""},
      "feather_weight":92,"confidence":0.96,"findings":[],
      "floor":{"passed":true,"checks":[{"name":"gofmt","passed":true},{"name":"vet","passed":true},{"name":"golangci-lint","passed":true},{"name":"test-race","passed":true}]},
      "model":{"provider":"local:fixture","version":"fixture-v1","local":true,"latency_ms":0,
      "evidence_set_digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      "calibration_id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","calibration":"unqualified"}
    }
    """#

    static func matches(_ verdict: MaatSystemOneVerdict, head: String) -> Bool {
        verdict.contract == id && verdict.schemaVersion == version &&
        verdict.subject.headSHA == head && ["pr", "diff", "file", "commit"].contains(verdict.subject.kind) &&
        verdict.model.calibration == "unqualified" && verdict.floorEvidence.contains("declared-only") &&
        (verdict.gate == "escalate" || verdict.gate == "block")
    }
}

struct MaatSourceImportApproval: Equatable, Sendable {
    let file: URL
    let expectedHead: String
    let inputSHA256: String
    let generation: UUID

    var arguments: [String] {
        ["maat", "screen", "--input", file.path, "--expect-head", expectedHead,
         "--expect-input-sha256", inputSHA256, "--confirm", "--json"]
    }
}

private struct MaatSourceImportPreview: Decodable {
    let inputSHA256: String
    let verdict: MaatSystemOneVerdict
    enum CodingKeys: String, CodingKey { case inputSHA256 = "input_sha256" }
    init(from decoder: Decoder) throws {
        verdict = try MaatSystemOneVerdict(from: decoder)
        inputSHA256 = try decoder.container(keyedBy: CodingKeys.self).decode(String.self, forKey: .inputSHA256)
    }
}

@MainActor final class MaatSourceImportModel: ObservableObject {
    typealias Capture = @MainActor ([String]) async -> CommandExecution
    @Published private(set) var selectedFile: URL?
    @Published var expectedHead = "" { didSet { if oldValue != expectedHead { invalidateReview() } } }
    @Published private(set) var approval: MaatSourceImportApproval?
    @Published private(set) var busy = false
    @Published private(set) var recording = false
    @Published private(set) var error: String?
    @Published private(set) var preview: MaatSystemOneVerdict?
    @Published private(set) var result: MaatSystemOneCommandResult?
    @Published private(set) var receipt: CommandExecution?
    private var generation = UUID()

    var canReview: Bool { !busy && selectedFile != nil && !expectedHead.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }

    func select(_ file: URL) { selectedFile = file; invalidateReview() }
    func cancelReview() { invalidateReview() }
    func selectionFailed(_ message: String) { invalidateReview(); error = message }
    private func invalidateReview() {
        generation = UUID(); approval = nil; preview = nil
        // Prior operation receipt/result survives correction, refresh or dismissal.
    }

    func review(capture: Capture? = nil) async {
        guard canReview, let file = selectedFile else {
            error = "Choose a source JSON and enter its independently selected expected head, then Review."
            return
        }
        let head = expectedHead.trimmingCharacters(in: .whitespacesAndNewlines)
        invalidateReview()
        let token = generation
        busy = true; recording = false; error = nil
        let args = ["maat", "screen", "--input", file.path, "--expect-head", head, "--preview", "--json"]
        let captured: CommandExecution
        if let capture { captured = await capture(args) }
        else { captured = await SirsiEngine.runCaptured(args: args) }
        busy = false
        guard generation == token, selectedFile == file, expectedHead.trimmingCharacters(in: .whitespacesAndNewlines) == head else {
            error = "Selection or expected head changed during preview. Review the current selection again."
            return
        }
        guard captured.complete, captured.exitCode == 0,
              let decoded = try? JSONDecoder().decode(MaatSourceImportPreview.self, from: captured.stdoutData),
              MaatSourceImportContract.matches(decoded.verdict, head: head),
              decoded.inputSHA256.count == 64,
              decoded.inputSHA256.utf8.allSatisfy({ (48...57).contains($0) || (97...102).contains($0) }) else {
            error = failure(captured, prefix: "Preview was not validated. Correct the expected head or choose a v2 file and review again.")
            return
        }
        preview = decoded.verdict
        approval = .init(file: file, expectedHead: head, inputSHA256: decoded.inputSHA256, generation: token)
    }

    // Consume confirmation synchronously, BEFORE creating a task/first await.
    // Refresh, double-click, malformed output and readback failure never replay.
    func confirm(capture: Capture? = nil) -> Task<Void, Never>? {
        guard !busy, let approved = approval, isCurrent(approved) else { return nil }
        approval = nil; busy = true; recording = true; error = nil
        return Task { @MainActor in
            let captured: CommandExecution
            if !self.isCurrent(approved) {
                captured = CommandExecution(launchError: "Selection changed after confirmation. Nothing launched; review again.")
            } else if let capture { captured = await capture(approved.arguments) }
            else { captured = await SirsiEngine.runCaptured(args: approved.arguments, authorize: { self.isCurrent(approved) }) }
            self.receipt = captured; self.busy = false; self.recording = false
            guard captured.complete, captured.exitCode == 0,
                  let decoded = try? JSONDecoder().decode(MaatSystemOneCommandResult.self, from: captured.stdoutData),
                  MaatSourceImportContract.matches(decoded.verdict, head: approved.expectedHead) else {
                self.error = self.failure(captured, prefix: "Recording was not confirmed. Do not repeat automatically; inspect retained Casebook evidence, then review the file and head again.")
                return
            }
            self.result = decoded
        }
    }

    private func isCurrent(_ approved: MaatSourceImportApproval) -> Bool {
        generation == approved.generation && selectedFile == approved.file &&
        expectedHead.trimmingCharacters(in: .whitespacesAndNewlines) == approved.expectedHead
    }

    private func failure(_ captured: CommandExecution, prefix: String) -> String {
        let detail = captured.issue ?? (captured.stderr.isEmpty ? "No complete matching v2 response." : String(captured.stderr.prefix(1200)))
        return prefix + " " + detail
    }
}
