import Foundation
import Combine
import Darwin
import Security

struct LegacyRelayPlan: Decodable, Equatable {
    struct Instance: Decodable, Equatable {
        let label: String
        let target: String
        let pid: Int
        let started: String
    }
    let profile: String
    let original: Instance
    let legacyPayload: String
    let sha256: String
    enum CodingKeys: String, CodingKey { case profile, original, sha256; case legacyPayload = "legacy_payload" }
    var isRootProfile: Bool {
        profile == "root-relay-legacy-maintenance-v1" && original.label == "ai.sirsi.router.relay" &&
        original.target == "system/ai.sirsi.router.relay" && original.pid > 1 && !original.started.isEmpty &&
        sha256.count == 64 && sha256.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }
}

struct LegacyRelayTransaction: Decodable, Equatable {
    let id: String
    let revision: UInt64
    let plan: LegacyRelayPlan
    let phase: String
    let preserved: Int
    let oldOutcomes: String
    let nextAction: String
    enum CodingKeys: String, CodingKey {
        case id, revision, plan, phase
        case preserved = "preserved_envelopes", oldOutcomes = "old_outcomes", nextAction = "next_action"
    }
    var isBound: Bool { !id.isEmpty && plan.isRootProfile && preserved >= 0 && oldOutcomes == "unknown_not_certified_drained" }
}

enum LegacyRelayAppHost {
    // The GUI stays unprivileged. The typed bridge independently qualifies the
    // protected engine and lets macOS own its one-shot administrator dialog.
    static func refusal() -> String? {
        guard Bundle.main.bundleIdentifier == "ai.sirsi.pantheon" else { return "The canonical signed Pantheon app host is required." }
        var code: SecCode?
        var requirement: SecRequirement?
        let text = "anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and identifier \"ai.sirsi.pantheon\""
        guard SecCodeCopySelf(SecCSFlags(rawValue: 0), &code) == errSecSuccess, let code,
              SecRequirementCreateWithString(text as CFString, SecCSFlags(rawValue: 0), &requirement) == errSecSuccess, let requirement,
              SecCodeCheckValidity(code, SecCSFlags(rawValue: 0), requirement) == errSecSuccess else {
            return "Pantheon's existing signed app identity was not verified. No service action was started."
        }
        return nil
    }
}

@MainActor final class LegacyRelayMaintenanceModel: ObservableObject {
    typealias Capture = @MainActor ([String], String?, URL, @escaping @MainActor () -> Bool) async -> CommandExecution
    struct Approval: Equatable { let plan: LegacyRelayPlan; let bytes: Data; let file: URL; let directory: URL; let generation: UUID }
    struct RestoreApproval: Equatable {
        let transaction: LegacyRelayTransaction
        let artifact: URL
        let digest: String
        let bytes: Data
        let directory: URL
        let generation: UUID
    }
    @Published private(set) var selectedFile: URL?
    @Published private(set) var preview: LegacyRelayPlan?
    @Published private(set) var transaction: LegacyRelayTransaction?
    @Published private(set) var receipt: CommandExecution?
    @Published private(set) var busy = false
    @Published private(set) var error: String?
    @Published private(set) var canConfirm = false
    @Published private(set) var restoreApproval: RestoreApproval?
    @Published private(set) var outcomeUnknown = false
    private var approval: Approval?
    private var generation = UUID()
    private var directory: URL
    private var transactionDirectory: URL?
    private var activeTask: Task<Void, Never>?
    private let capture: Capture

    init(directory: URL, capture: Capture? = nil) {
        self.directory = directory
        self.capture = capture ?? { args, stdin, _, authorize in
            await LegacyRelayPrivilegedBridge().capture(args: args, stdin: stdin, authorize: authorize)
        }
    }

    func select(_ file: URL) { selectedFile = file; cancelReview() }
    func changeDirectory(_ directory: URL) { if self.directory != directory { self.directory = directory; cancelReview() } }
    func cancelReview() { generation = UUID(); approval = nil; restoreApproval = nil; canConfirm = false; preview = nil }

    func review() async {
        guard !busy, !outcomeUnknown, let file = selectedFile else { return }
        cancelReview(); let token = generation; let scope = directory
        busy = true; error = nil
        let result = await capture(["launchd", "legacy-maintenance", "preview", "--legacy-payload", file.path], nil, scope, { self.generation == token && self.selectedFile == file && self.directory == scope })
        busy = false
        guard !Task.isCancelled, generation == token, selectedFile == file, directory == scope else { return }
        receipt = result
        guard result.complete, result.exitCode == 0,
              let plan = try? JSONDecoder().decode(LegacyRelayPlan.self, from: result.stdoutData),
              plan.isRootProfile, plan.legacyPayload == file.path else { error = detail(result); return }
        preview = plan; approval = Approval(plan: plan, bytes: result.stdoutData, file: file, directory: scope, generation: token); canConfirm = true
    }

    // Consume consent before creating the task. Navigation may invalidate the
    // preview but cannot replay or cancel a once-started root transaction.
    func confirmation() -> Approval? { guard !busy,let approval,current(approval) else{return nil};return approval }
    func confirm(_ approved: Approval) -> Task<Void, Never>? {
        guard !busy, approval == approved, current(approved) else { return nil }
        approval = nil; canConfirm = false; busy = true; error = nil
        let task = Task { @MainActor in
            let result = await self.capture(["launchd", "legacy-maintenance", "confirm", "--legacy-payload", approved.file.path,
                                            "--expected-plan-sha256", approved.plan.sha256, "--confirm-maintenance"], nil, approved.directory, { self.current(approved) })
            self.accept(result, expected: approved.plan.sha256, scope: approved.directory); self.busy = false
        }
        activeTask=task;return task
    }

    func readStatus() async {
        guard !busy else { return }; busy = true; let scope = directory;let token=generation
        let result = await capture(["launchd", "legacy-maintenance", "status"], nil, scope, { self.directory == scope })
        busy = false
        guard !Task.isCancelled, directory == scope, generation == token else { return }
        accept(result, expected: nil, scope: scope, mayMutate: false)
    }

    func resume(_ expected: LegacyRelayTransaction) -> Task<Void, Never>? { perform("recover",expected:expected) }
    func abortBeforeRetirement(_ expected: LegacyRelayTransaction) -> Task<Void, Never>? { perform("abort",expected:expected) }
    func cancelInvocation(){cancelReview();activeTask?.cancel()}

    private func perform(_ operation: String,expected:LegacyRelayTransaction) -> Task<Void, Never>? {
        guard !busy, let approved = transaction, approved == expected, approved.isBound, transactionDirectory == directory else { return nil }
        let scope = directory; busy = true; error = nil
        let task = Task { @MainActor in
            let result = await self.capture(["launchd", "legacy-maintenance", operation, "--transaction", approved.id,
                                            "--expected-plan-sha256", approved.plan.sha256, "--confirm-maintenance"], nil, scope,
                                           { self.directory == scope && self.transaction == approved })
            self.accept(result, expected: approved.plan.sha256,scope:scope); self.busy = false
        }
        activeTask=task;return task
    }

    func reviewRestore(artifact: URL, expected: LegacyRelayTransaction) async {
        guard !busy, expected == transaction, expected.isBound, transactionDirectory == directory else { return }
        restoreApproval = nil
        let token = generation
        let scope = directory
        busy = true; error = nil
        let result = await capture(["launchd", "legacy-maintenance", "restore-preview", "--transaction", expected.id,
                                    "--expected-plan-sha256", expected.plan.sha256, "--artifact", artifact.path], nil, scope,
                                   { self.generation == token && self.directory == scope && self.transaction == expected })
        busy = false
        guard !Task.isCancelled, generation == token, directory == scope, transaction == expected else { return }
        receipt = result
        guard result.complete, result.exitCode == 0, result.stdoutData.count <= 8192,
              let value = try? JSONDecoder().decode(RestoreEnvelope.self, from: result.stdoutData),
              value.transaction == expected.id, value.revision == expected.revision,
              value.maintenanceSHA256 == expected.plan.sha256, value.artifact == artifact.path,
              Self.digest(value.sha256) else { error = detail(result); return }
        restoreApproval = RestoreApproval(transaction: expected, artifact: artifact, digest: value.sha256,
                                          bytes: result.stdoutData, directory: scope, generation: token)
    }

    func restore(_ approved: RestoreApproval) -> Task<Void, Never>? {
        guard !busy, restoreApproval == approved, transaction == approved.transaction,
              directory == approved.directory, generation == approved.generation else { return nil }
        restoreApproval = nil; busy = true; error = nil
        let task = Task { @MainActor in
            let result = await self.capture(["launchd", "legacy-maintenance", "restore-confirm",
                                            "--expected-restore-sha256", approved.digest, "--confirm-maintenance"],
                                           String(decoding: approved.bytes, as: UTF8.self), approved.directory,
                                           { self.directory == approved.directory && self.generation == approved.generation &&
                                               self.transaction == approved.transaction })
            self.accept(result, expected: approved.transaction.plan.sha256, scope: approved.directory)
            self.busy = false
        }
        activeTask = task; return task
    }

    private struct RestoreEnvelope: Decodable {
        let transaction: String; let revision: UInt64; let artifact: String
        let maintenanceSHA256: String; let sha256: String
        enum CodingKeys: String, CodingKey { case transaction, revision, artifact, sha256; case maintenanceSHA256 = "maintenance_sha256" }
    }
    private static func digest(_ value: String) -> Bool { value.count == 64 && value.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) } }
    private func current(_ value: Approval) -> Bool { value.generation == generation && value.file == selectedFile && value.directory == directory && value.plan == preview }
    private func accept(_ result: CommandExecution, expected: String?,scope:URL,mayMutate:Bool = true) {
        receipt = result
        var matched=false
        if result.complete, let decoded = try? JSONDecoder().decode(LegacyRelayTransaction.self, from: result.stdoutData),
           decoded.isBound, expected == nil || decoded.plan.sha256 == expected { transaction = decoded;transactionDirectory=scope;restoreApproval=nil;matched=true;outcomeUnknown=false }
        if result.captureError != nil && mayMutate { outcomeUnknown=true }
        if result.complete && result.exitCode == 0 && matched { error = nil }
        else { error = detail(result) }
    }
    private func detail(_ value: CommandExecution) -> String {
        value.issue ?? (value.stderr.isEmpty ? "No verified matching transaction result. Read current status; do not repeat automatically." : String(value.stderr.prefix(1200)))
    }
}
