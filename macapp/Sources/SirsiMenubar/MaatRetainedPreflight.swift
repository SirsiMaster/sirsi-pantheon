import Foundation
import SwiftUI

struct MaatPreflightScope: Decodable, Equatable {
    let component: String
    let profile: String
    let operation: String
}

struct MaatPreflightStoreBinding: Decodable, Equatable {
    let state: String
    let store_uuid: String?
    var valid: Bool {
        (state == "initialized" && store_uuid.flatMap { UUID(uuidString: $0)?.uuidString.lowercased() } == store_uuid && store_uuid != nil) ||
        (state == "legacy-explicit" && store_uuid == nil)
    }
}

struct MaatPreflightReceiptReference: Decodable, Equatable {
    let receipt_sha256: String
    let action_sha256: String
    let registry_sha256: String
    let registry_snapshot_version: Int
    let action: MaatPreflightScope
    let store_binding: MaatPreflightStoreBinding
}

struct MaatPreflightGuard: Decodable {
    let ID: String
    let Version: String
    let SHA256: String
}
struct MaatPreflightCheck: Decodable {
    let IncidentKey: String
    let EvidenceSHA256: String
    let Status: String
    let Outcome: String
}
struct MaatPreflightRecovery: Decodable {
    let id: String
    let label: String
    let instruction: String
}
struct MaatRetainedPreflight: Decodable {
    let schema: String
    let action: MaatPreflightScope
    let action_manifest_sha256: String
    let registry_snapshot_sha256: String
    let evaluated_guards: [MaatPreflightGuard]?
    let measured_checks: [MaatPreflightCheck]?
    let decision: String
    let incident_keys: [String]?
    let recovery_actions: [MaatPreflightRecovery]?
    let recovery_reference: String
    let evaluated_at_utc: String
    let registry_snapshot_version: Int?
}
struct MaatRetainedPreflightRead: Decodable {
    let schema: String
    let receipt_sha256: String
    let receipt: MaatRetainedPreflight
    let store_binding: MaatPreflightStoreBinding
    let read_at_utc: String
    let freshness: String
    let registry_relation: String
    let action_authorized: Bool
}

struct MaatCapturedPreflightIdentity: Equatable {
    let receipt: String
    let action: String
    let registry: String

    init?(evidence: String) {
        let parts = evidence.components(separatedBy: ";")
        guard parts.count == 3,
              parts[0].hasPrefix("maat-failure-memory:receipt-sha256="),
              parts[1].hasPrefix("action-sha256="), parts[2].hasPrefix("registry-sha256=") else { return nil }
        receipt = String(parts[0].dropFirst("maat-failure-memory:receipt-sha256=".count))
        action = String(parts[1].dropFirst("action-sha256=".count))
        registry = String(parts[2].dropFirst("registry-sha256=".count))
        guard [receipt, action, registry].allSatisfy(Self.digest) else { return nil }
    }
    static func digest(_ value: String) -> Bool {
        value.utf8.count == 64 && value.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
    }
    var arguments: [String] { ["maat", "failure-memory", "receipt", "--sha256", receipt, "--json"] }
    var recoveryArguments: [String] { arguments + ["--recover", "--confirm"] }

    func matches(_ read: MaatRetainedPreflightRead, entry: MaatCase) -> Bool {
        let r = read.receipt
        guard read.schema == "sirsi.maat.failure-memory-receipt-read.v1", read.receipt_sha256 == receipt,
              r.schema == "sirsi.maat.failure-memory.v1", r.action_manifest_sha256 == action,
              r.registry_snapshot_sha256 == registry, [0, 2].contains(r.registry_snapshot_version ?? 0),
              read.freshness == "historical", !read.action_authorized,
              ["same", "changed", "unverifiable"].contains(read.registry_relation), read.store_binding.valid,
              ["pass", "reject", "unverifiable"].contains(r.decision),
              r.decision == entry.determination, r.action.component == entry.resource,
              r.action.profile == entry.affected, r.action.operation == entry.assessed,
              !r.evaluated_at_utc.isEmpty, !read.read_at_utc.isEmpty else { return false }
        if let reference = entry.failureMemoryReceipt {
            guard reference.receipt_sha256 == receipt, reference.action_sha256 == action,
                  reference.registry_sha256 == registry, reference.action == r.action,
                  reference.registry_snapshot_version == r.registry_snapshot_version,
                  reference.store_binding == read.store_binding else { return false }
        }
        return true
    }
}

struct MaatRetainedPreflightReadState {
    var receipt: MaatRetainedPreflightRead?
    var canonicalCLIResponse: Data?
    var message: String?
    var busy = false

    mutating func finish(_ read: SirsiEngine.JSONCommandRead, entry: MaatCase, cancelled: Bool) {
        busy = false
        guard !cancelled else { message = "Receipt inspection cancelled. The original case and any retained detail remain available."; return }
        guard let identity = MaatCapturedPreflightIdentity(evidence: entry.evidence) else {
            message = "The captured receipt identity is invalid. The original case is preserved."; return
        }
        guard !read.timedOut, read.launchError == nil, read.exitStatus == 0,
              let decoded = MaatRetainedPreflightRead.decode(read.data),
              identity.matches(decoded, entry: entry) else {
            struct Missing: Decodable { let schema: String; let receipt_sha256: String; let error_code: String }
            if read.exitStatus != 0, let missing = try? JSONDecoder().decode(Missing.self, from: read.data),
               missing.schema == "sirsi.maat.failure-memory-receipt-read.v1", missing.receipt_sha256 == identity.receipt,
               missing.error_code == "historical-receipt-unavailable" {
                message = "Historical receipt bytes unavailable. A new preflight cannot reconstruct the missing historical receipt. The original case is preserved."
            } else {
                message = "The historical receipt could not be verified against this case. Retry inspection; the original case and prior detail are preserved. No action was authorized."
            }
            return
        }
        receipt = decoded
        canonicalCLIResponse = read.data
        message = nil
    }
}

// Owned by the engine's existing Casebook operation store, so a custom
// navigation push/pop cannot discard the previously verified receipt.
@MainActor final class MaatPreflightReceiptReadStore: ObservableObject {
    typealias Read = @Sendable ([String]) async -> SirsiEngine.JSONCommandRead
    private let readCommand: Read
    init(read: @escaping Read = { await SirsiEngine.runJSONRead(args: $0) }) {
        self.readCommand = read
    }
    @Published private(set) var states: [String: MaatRetainedPreflightReadState] = [:]
    func state(_ entry: MaatCase) -> MaatRetainedPreflightReadState { states[entry.id + "|" + entry.evidence] ?? MaatRetainedPreflightReadState() }
    func inspect(_ entry: MaatCase) async { await read(entry, recovering: false) }
    func recover(_ entry: MaatCase) async { await read(entry, recovering: true) }
    private func read(_ entry: MaatCase, recovering: Bool) async {
        guard let identity = MaatCapturedPreflightIdentity(evidence: entry.evidence), !state(entry).busy else { return }
        var pending = state(entry); pending.busy = true; states[entry.id + "|" + entry.evidence] = pending
        let read = await readCommand(recovering ? identity.recoveryArguments : identity.arguments)
        var terminal = state(entry); terminal.finish(read, entry: entry, cancelled: Task.isCancelled)
        states[entry.id + "|" + entry.evidence] = terminal
    }
}

struct MaatRetainedPreflightDetail: View {
    @ObservedObject var model: MaatPreflightReceiptReadStore
    let entry: MaatCase
    @State private var confirmRecovery = false
    var body: some View {
        let state = model.state(entry)
        VStack(alignment: .leading, spacing: 8) {
            Button(state.receipt == nil ? "Inspect retained preflight" : "Reinspect historical receipt") {
                Task { await model.inspect(entry) }
            }
            .disabled(state.busy || MaatCapturedPreflightIdentity(evidence: entry.evidence) == nil)
            if state.busy { ProgressView("Reading retained receipt…") }
            if let message = state.message {
                Text(message).foregroundStyle(.secondary)
                Button("Recover exact interrupted receipt") { confirmRecovery = true }
                    .disabled(state.busy || MaatCapturedPreflightIdentity(evidence: entry.evidence) == nil)
            }
            if let read = state.receipt {
                Text("Historical preflight — \(read.receipt.decision)").font(.headline)
                Text("Evaluated \(read.receipt.evaluated_at_utc); read \(read.read_at_utc). Registry \(read.registry_relation). This historical receipt authorizes no action.")
                Text("Schema: \(read.receipt.schema)")
                Text("Store: \(read.store_binding.state) \(read.store_binding.store_uuid ?? "")")
                Text("\(read.receipt.action.component) / \(read.receipt.action.profile) / \(read.receipt.action.operation)")
                Text("Receipt \(read.receipt_sha256)\nAction \(read.receipt.action_manifest_sha256)\nRegistry v\(read.receipt.registry_snapshot_version ?? 0) \(read.receipt.registry_snapshot_sha256)")
                ForEach(Array((read.receipt.evaluated_guards ?? []).enumerated()), id: \.offset) { _, guardRecord in
                    Text("Guard \(guardRecord.ID) / \(guardRecord.Version): \(guardRecord.SHA256)")
                }
                ForEach(Array((read.receipt.measured_checks ?? []).enumerated()), id: \.offset) { _, check in
                    Text("\(check.Status): \(check.Outcome)\nIncident \(check.IncidentKey)\nProof \(check.EvidenceSHA256)")
                }
                ForEach(Array((read.receipt.incident_keys ?? []).enumerated()), id: \.offset) { _, key in Text("Incident \(key)") }
                ForEach(Array((read.receipt.recovery_actions ?? []).enumerated()), id: \.offset) { _, action in
                    Text("\(action.label) [\(action.id)]\n\(action.instruction)")
                }
                Text("Recovery reference: \(read.receipt.recovery_reference)")
            }
        }.textSelection(.enabled)
        .confirmationDialog("Complete only this exact retained receipt write?", isPresented: $confirmRecovery) {
            Button("Complete exact historical receipt") { Task { await model.recover(entry) } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Only complete original bytes with their exact prepared intent can be committed. Missing or changed bytes and unrelated debt remain refused. No receipt time is rewritten, no Casebook decision is appended, and no repair or new action is authorized.")
        }
    }
}

// Required nullable arrays stay required: a truncated projection is never a
// complete receipt. This checks wire shape, not a second action authority.
extension MaatRetainedPreflightRead {
    static func decode(_ data: Data) -> Self? {
        guard data.count <= 16 * 1024 * 1024,
              let root = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
              Set(root.keys) == Set(["schema", "receipt_sha256", "receipt", "store_binding", "read_at_utc", "freshness", "registry_relation", "action_authorized"]),
              let r = root["receipt"] as? [String: Any] else { return nil }
        let required: Set<String> = ["schema", "action", "action_manifest_sha256", "registry_snapshot_sha256", "evaluated_guards", "measured_checks", "decision", "incident_keys", "recovery_actions", "recovery_reference", "evaluated_at_utc"]
        guard required.isSubset(of: Set(r.keys)), Set(r.keys).isSubset(of: required.union(["registry_snapshot_version"])),
              let scope = r["action"] as? [String: Any], Set(scope.keys) == Set(["component", "profile", "operation"]),
              let binding = root["store_binding"] as? [String: Any], Set(binding.keys).isSubset(of: Set(["state", "store_uuid"])), binding["state"] != nil,
              let decoded = try? JSONDecoder().decode(Self.self, from: data) else { return nil }
        let receipt = decoded.receipt
        guard receipt.action.component.utf8.count <= 160, !receipt.action.component.isEmpty,
              receipt.action.profile.utf8.count <= 160, !receipt.action.profile.isEmpty,
              receipt.action.operation.utf8.count <= 160, !receipt.action.operation.isEmpty,
              (receipt.incident_keys ?? []).allSatisfy(MaatCapturedPreflightIdentity.digest),
              (receipt.evaluated_guards ?? []).allSatisfy({ $0.ID == "maat.active-incident" && $0.Version == "v1" && MaatCapturedPreflightIdentity.digest($0.SHA256) }),
              (receipt.measured_checks ?? []).allSatisfy({ MaatCapturedPreflightIdentity.digest($0.IncidentKey) && MaatCapturedPreflightIdentity.digest($0.EvidenceSHA256) && $0.Status == "active" && $0.Outcome == "active-incident-recurrence-rejected" }),
              (receipt.recovery_actions ?? []).allSatisfy({ !$0.id.isEmpty && !$0.label.isEmpty && !$0.instruction.isEmpty && $0.id.utf8.count <= 512 && $0.label.utf8.count <= 512 && $0.instruction.utf8.count <= 512 }) else { return nil }
        if receipt.decision == "pass", !(receipt.incident_keys ?? []).isEmpty || !(receipt.measured_checks ?? []).isEmpty || !(receipt.evaluated_guards ?? []).isEmpty || !(receipt.recovery_actions ?? []).isEmpty || !receipt.recovery_reference.isEmpty { return nil }
        if receipt.decision == "reject", (receipt.incident_keys ?? []).isEmpty || (receipt.measured_checks ?? []).isEmpty || (receipt.evaluated_guards ?? []).isEmpty { return nil }
        return decoded
    }
}
