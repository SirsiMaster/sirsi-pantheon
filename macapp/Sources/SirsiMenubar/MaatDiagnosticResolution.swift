import SwiftUI
import CryptoKit
import Darwin

// Match Go's local hostname without DNS aliases or display-name normalization.
// Unknown/truncated/non-UTF8 identity remains nonbinding.
func maatLocalDiagnosticHost() -> String? {
    let capacity = 256
    var buffer = [CChar](repeating: 0, count: capacity)
    guard gethostname(&buffer, capacity) == 0,
          let end = buffer.firstIndex(of: 0), end > 0 else { return nil }
    return String(bytes: buffer[..<end].map { UInt8(bitPattern: $0) }, encoding: .utf8)
}

private func maatDiagnosticSHA256(_ value: String) -> Bool {
    value.utf8.count == 64 && value.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
}

private func maatHostDiagnosticID(_ check: String) -> String {
    let digest = SHA256.hash(data: Data(check.trimmingCharacters(in: .whitespacesAndNewlines).utf8)).prefix(6).map { String(format: "%02x", $0) }.joined()
    return "host-health-" + digest
}

func maatDiagnosticHostMatches(_ subject: MaatSystemOneSubject, localHost: String) -> Bool {
    let host = localHost.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !host.isEmpty, host.utf8.count <= 1024, subject.kind == "host", subject.ref == "diagnostic" else { return false }
    guard let binding = subject.hostBinding, !binding.isEmpty else { return subject.repo == host }
    guard binding == "sirsi.maat.local-host-sha256.v1" else { return false }
    let digest = SHA256.hash(data: Data(host.utf8)).map { String(format: "%02x", $0) }.joined()
    return subject.repo == "local-host-" + digest
}

func maatUsesDedicatedDiagnosticCustody(_ finding: MaatSystemOneFinding) -> Bool {
    ["binary-drift", "liveness-watch", "launchd Disabled Override"].contains { maatHostDiagnosticID($0) == finding.id }
}

// A historical finding selects a current typed check, never a command. Keep
// the original host and exact v1 diagnostic identity; source imports and hints
// cannot become repair authority. The existing closed Ma'at repairs keep their
// own preview/journal/fence routes instead of entering the generic resolver.
func maatHasLocalDiagnosticCustody(_ verdict: MaatSystemOneVerdict,
                                  finding: MaatSystemOneFinding, localHost: String) -> Bool {
    let head = verdict.subject.headSHA
    return verdict.contract.isEmpty && verdict.schemaVersion == 1 &&
        verdict.subject.kind == "host" && verdict.subject.ref == "diagnostic" &&
        maatDiagnosticHostMatches(verdict.subject, localHost: localHost) &&
        maatDiagnosticSHA256(head) &&
        finding.category == "host-health" &&
        finding.id.hasPrefix("host-health-") && finding.id.utf8.count == 24 &&
        finding.evidence == "diagnostic:sha256=\(head):\(finding.id.dropFirst(12))" &&
        verdict.findings.filter { $0.id == finding.id }.count == 1
}

enum MaatCurrentDiagnosticResolution {
    case current(DiagFinding)
    case unavailable(String)
}

func maatCurrentDiagnosticResolution(verdict: MaatSystemOneVerdict, finding: MaatSystemOneFinding,
                                     localHost: String, read: SirsiEngine.JSONCommandRead) -> MaatCurrentDiagnosticResolution {
    guard maatHasLocalDiagnosticCustody(verdict, finding: finding, localHost: localHost) else {
        return .unavailable("This retained finding does not identify this Mac's canonical local diagnostic. Re-observe this Mac in Casebook; no repair is authorized.")
    }
    guard read.exitStatus == 0, !read.timedOut, read.launchError == nil,
          let report = try? JSONDecoder().decode(DiagReport.self, from: read.data),
          let status = report.status, ["green", "amber", "red"].contains(status) else {
        return .unavailable("The current diagnostic is unavailable or incomplete. Read it again before reviewing a repair; the original case remains open.")
    }
    let matches = report.findings.filter { current in
        finding.id == maatHostDiagnosticID(current.check)
    }
    guard matches.count == 1, let current = matches.first, current.severity >= 2 else {
        return .unavailable("The retained finding is no longer a unique active warning in the fresh diagnostic. No repair will run. Record a fresh screen below; the old case is preserved.")
    }
    guard !["binary-drift", "liveness-watch", "launchd Disabled Override"].contains(current.check) else {
        return .unavailable("This finding requires Ma'at's dedicated preview and retained repair route. Record a fresh screen below and open its named recovery; no generic repair will run.")
    }
    return .current(current)
}

@MainActor final class MaatDiagnosticResolutionModel: ObservableObject {
    @Published private(set) var finding: DiagFinding?
    @Published private(set) var message = "Read the current local diagnostic, then review its controlled resolution. Reading does not repair anything."
    @Published private(set) var busy = false
    @Published private(set) var screenResult: MaatSystemOneCommandResult?
    @Published private(set) var screenReceipt: CommandExecution?
    @Published private(set) var screenAttempted = false
    private var generation = UUID()
    private var screenInvocation: RemediationInvocation?

    func refresh(verdict: MaatSystemOneVerdict, finding retained: MaatSystemOneFinding,
                 localHost: String, read: () async -> SirsiEngine.JSONCommandRead) async {
        guard !Task.isCancelled else { cancelReview(); return }
        let token = UUID()
        generation = token
        finding = nil
        busy = true
        let result = await read()
        guard generation == token else { return }
        busy = false
        guard !Task.isCancelled else {
            message = "Diagnostic review was cancelled. No repair was authorized; read again when ready."
            return
        }
        switch maatCurrentDiagnosticResolution(verdict: verdict, finding: retained, localHost: localHost, read: result) {
        case .current(let current):
            finding = current
            message = "The current finding is ready for review. Its operands and severity may differ from the historical screen. Repair still requires the existing resolver's separate confirmation."
        case .unavailable(let reason): message = reason
        }
    }

    func cancelReview() {
        generation = UUID()
        finding = nil
        busy = false
        message = "Diagnostic review was cancelled. No repair was authorized; read again when ready."
    }

    func canOpen(snapshot: FindingObservationSnapshot) -> Bool {
        guard let finding else { return false }
        return snapshot == FindingObservationSnapshot(current: true, finding: finding)
    }

    func beginReview(snapshot: FindingObservationSnapshot) -> DiagFinding? {
        guard canOpen(snapshot: snapshot) else {
            finding = nil
            message = "The current finding changed, expired, or became unavailable. Read it again before reviewing a repair."
            return nil
        }
        return finding
    }

    // An explicit confirmation records one new screen. A view refresh or a
    // repeated completion cannot replay even this journal write. It is never
    // an updater/installer retry, and never closes the historical case.
    func recordFreshScreen(localHost: String = maatLocalDiagnosticHost() ?? "",
                           capture: ([String]) async -> CommandExecution) async {
        guard !busy, screenInvocation == nil else { return }
        let invocation = RemediationInvocation()
        screenInvocation = invocation
        screenAttempted = true
        finding = nil
        generation = UUID()
        busy = true
        defer { busy = false }
        let receipt = await invocation.executeOnce(args: ["maat", "triage", "--confirm", "--json"],
            authorize: { !Task.isCancelled }, capture: capture)
        screenReceipt = receipt
        guard let receipt, receipt.complete, receipt.exitCode == 0,
              let result = try? JSONDecoder().decode(MaatSystemOneCommandResult.self, from: receipt.stdoutData),
              result.verdict.contract.isEmpty, result.verdict.schemaVersion == 1,
              result.verdict.subject.kind == "host", result.verdict.subject.ref == "diagnostic",
              maatDiagnosticHostMatches(result.verdict.subject, localHost: localHost),
              maatDiagnosticSHA256(result.verdict.subject.headSHA),
              result.snapshotEvidence == "diagnostic:sha256=" + result.verdict.subject.headSHA,
              result.decisionEvidence.hasPrefix("maat-system-one:sha256="),
              maatDiagnosticSHA256(String(result.decisionEvidence.dropFirst("maat-system-one:sha256=".count))),
              ["pass", "changes", "block", "escalate"].contains(result.verdict.gate) else {
            message = "A fresh screen was not confirmed. Inspect Casebook before attempting another write; this action will not replay. The original case remains preserved."
            return
        }
        screenResult = result
        message = "A separate fresh observation was recorded. It does not prove that a repair caused recovery or accept the original case. Continue in Casebook to review its evidence and any remaining findings."
    }
}

@MainActor struct MaatDiagnosticResolutionView: View {
    @EnvironmentObject private var nav: Nav
    @ObservedObject var engine: SirsiEngine
    let origin: MaatCase
    let finding: MaatSystemOneFinding
    // Nav.Frame retains this destination value while another frame is visible.
    // Keep the workflow strongly owned by that value (as EmptyTrashView does),
    // not by remount-scoped StateObject storage. Back must retain write debt.
    @ObservedObject var model: MaatDiagnosticResolutionModel
    @State private var confirmScreen = false
    @State private var diagnosticTask: Task<Void, Never>?
    @State private var screenTask: Task<Void, Never>?

    init(engine: SirsiEngine, origin: MaatCase, finding: MaatSystemOneFinding,
         model: MaatDiagnosticResolutionModel? = nil) {
        self.engine = engine
        self.origin = origin
        self.finding = finding
        self.model = model ?? MaatDiagnosticResolutionModel()
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Resolve the current finding")
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text("Retained case: \(origin.evidence)").sirsiFont(.caption, design: .monospaced).textSelection(.enabled)
                    Text(finding.claim).sirsiFont(.body)
                    Text(model.message).sirsiFont(.callout).fixedSize(horizontal: false, vertical: true)
                    Button("Read current diagnostic") {
                        guard let verdict = origin.systemOne else { return }
                        guard diagnosticTask == nil, screenTask == nil else { return }
                        diagnosticTask = Task {
                            await model.refresh(verdict: verdict, finding: finding, localHost: maatLocalDiagnosticHost() ?? "") {
                                await engine.readHealthForVerification()
                            }
                            diagnosticTask = nil
                        }
                    }
                    .buttonStyle(.bordered).disabled(model.busy)
                    if let current = model.finding {
                        Text("Current: \(current.message)").sirsiFont(.body)
                        if let detail = current.detail { Text(detail).sirsiFont(.callout).textSelection(.enabled) }
                        Button {
                            if let reviewed = model.beginReview(snapshot: engine.diagnosticSnapshot(check: current.check)) {
                                nav.push(FindingView(engine: engine, finding: reviewed))
                            }
                        } label: {
                            Label("Review controlled resolution", systemImage: "wrench.and.screwdriver.fill")
                        }
                        .buttonStyle(.borderedProminent).tint(gold)
                        .disabled(!model.canOpen(snapshot: engine.diagnosticSnapshot(check: current.check)))
                        Text("Use Back after reviewing the result to return to this exact retained case. A new diagnostic read grants no repair consent.")
                            .sirsiFont(.caption).foregroundStyle(.secondary)
                    }
                    if model.busy {
                        ProgressView("Reading local evidence…")
                        if diagnosticTask != nil {
                            Button("Cancel diagnostic read") {
                                diagnosticTask?.cancel()
                                model.cancelReview()
                            }.buttonStyle(.bordered)
                        } else if screenTask != nil {
                            Button("Stop waiting and inspect the outcome") { screenTask?.cancel() }
                                .buttonStyle(.bordered)
                            Text("Cancellation may follow a completed journal write. Inspect the retained result in Casebook before a new action; this screen will not replay the write.")
                                .sirsiFont(.caption).foregroundStyle(.secondary)
                        }
                    }
                    Divider()
                    Text("After repair, guidance, or changed conditions, record and review a fresh screen. The original evidence and explicit owner-acceptance route remain in Casebook.")
                        .sirsiFont(.callout)
                    Button("Review and record a fresh System One screen") { confirmScreen = true }
                        .buttonStyle(.bordered).disabled(model.busy || model.screenAttempted)
                    if let result = model.screenResult {
                        Text(result.summary).sirsiFont(.body)
                        Text(result.decisionEvidence).sirsiFont(.caption, design: .monospaced).textSelection(.enabled)
                    }
                    if model.screenResult == nil, let receipt = model.screenReceipt {
                        Text(receipt.displayText).sirsiFont(.caption, design: .monospaced).textSelection(.enabled)
                    }
                    NavLink { MaatCasebookView(engine: engine) } label: {
                        Label("Continue in Ma'at Casebook", systemImage: "book.closed")
                    }.buttonStyle(.bordered).disabled(model.busy)
                }.padding(16)
            }
        }
        .confirmationDialog("Record a fresh local observation?", isPresented: $confirmScreen, titleVisibility: .visible) {
            Button("Record fresh screen") {
                guard screenTask == nil, diagnosticTask == nil else { return }
                screenTask = Task {
                    await model.recordFreshScreen { args in await SirsiEngine.runCaptured(args: args) }
                    screenTask = nil
                }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Pantheon will run one local System One observation and retain its exact result. It will not run a repair, replay an updater, or accept the original case \(origin.evidence).")
        }
        .onDisappear {
            if diagnosticTask != nil { diagnosticTask?.cancel(); model.cancelReview() }
            screenTask?.cancel()
        }
    }
}
