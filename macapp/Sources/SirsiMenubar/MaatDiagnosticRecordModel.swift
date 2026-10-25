import Foundation
import SwiftUI

// One engine-owned invocation/consent lifetime, independent of Casebook rows.
// All effects remain in the canonical CLI; this model never opens local stores.
@MainActor final class MaatDiagnosticRecordModel: ObservableObject {
    typealias Reader = @MainActor ([String]) async -> SirsiEngine.JSONCommandRead
    enum Action: Equatable { case record, initialize, reconcile(String) }
    struct Approval: Equatable {
        let action: Action
        let generation: UUID
        let context: Data
    }
    struct Pending: Identifiable {
        let id: String
        let store: String
        let observed: String
        let bytes: Data
    }
    @Published private(set) var busy = false
    @Published private(set) var observation: MaatSystemOneVerdict?
    @Published private(set) var recorded: MaatSystemOneCommandResult?
    @Published private(set) var memory: MaatFailureMemoryStatus?
    @Published private(set) var pending: [Pending] = []
    @Published private(set) var message: String?
    @Published private(set) var deliveryUnknown = false
    private var generation = UUID()
    private var memoryBytes = Data()
    private var historyVerified = false
    private var unknownRecordID: String?
    private var task: Task<SirsiEngine.JSONCommandRead, Never>?
    private let reader: Reader
    private let uid: UInt32
    init(uid: UInt32 = getuid(), reader: Reader? = nil) {
        self.uid = uid
        self.reader = reader ?? { await SirsiEngine.runJSONRead(args: $0, timeoutSeconds: 30) }
    }
    static let inspectArguments = ["maat", "repair", "liveness-watch", "--inspect-failure-memory", "--json"]
    static let pendingArguments = ["maat", "triage", "--inspect-records", "--json"]
    func cancel() { task?.cancel() }
    private func read(_ args: [String]) async -> SirsiEngine.JSONCommandRead? {
        let child = Task { @MainActor in await self.reader(args) }
        task = child
        let result = await withTaskCancellationHandler(operation: { await child.value }, onCancel: { child.cancel() })
        task = nil
        guard !child.isCancelled, !Task.isCancelled, !result.timedOut, result.launchError == nil,
              result.exitStatus != nil, result.data.count <= 8 * 1024 * 1024 else { return nil }
        return result
    }
    private func begin() -> Bool {
        guard !busy, !Task.isCancelled else { return false }
        generation = UUID(); busy = true
        return true
    }
    // Observation does not initialize memory and is never called a record.
    func observe() async {
        guard begin() else { return }; defer { busy = false }
        guard let result = await read(["maat", "triage", "--json"]), result.exitStatus == 0,
              let value = try? JSONDecoder().decode(MaatSystemOneVerdict.self, from: result.data) else {
            message = "Local observation is unavailable or was canceled. Prior evidence remains; try a fresh observation."; return
        }
        observation = value; recorded = nil
        message = "Local observation only · not recorded. Recording performs a new observation and its own scoped preflight."
    }
    func inspect() async {
        guard begin() else { return }; defer { busy = false }
        pending = []; memory = nil; memoryBytes = Data(); historyVerified = false
        guard let result = await read(Self.inspectArguments), result.exitStatus == 0,
              let status = try? JSONDecoder().decode(MaatFailureMemoryStatus.self, from: result.data) else {
            message = "Local memory is unverifiable. Observation remains available; inspect again or use prepared receipt recovery below. No reset or record was attempted."; return
        }
        memory = status; memoryBytes = result.data
        guard status.state == "ready", let store = status.storeUUID,
              let result = await read(Self.pendingArguments), result.exitStatus == 0,
              let values = decodePending(result.data, store: store) else {
            message = status.state == "uninitialized"
                ? "Local memory has not been initialized. Review first-use initialization; no incidents will be imported."
                : "Local memory or record history is incomplete. Inspect prepared receipts below; no record will be resent."
            return
        }
        pending = values
        historyVerified = true
        message = values.isEmpty
            ? (deliveryUnknown ? "No matching pending record was verified. Prior delivery remains unknown; no resend is offered. Inspect prepared receipt custody below." : "Local memory is ready. Use a fresh observation and new confirmation to record.")
            : "Retained record delivery needs exact data-only reconciliation. Review a selected intent; no diagnostic is rerun."
    }
    func approval(_ action: Action) -> Approval? {
        guard !busy else { return nil }
        switch action {
        case .record:
            guard memory?.state == "ready", historyVerified, !deliveryUnknown, pending.isEmpty,
                  !memoryBytes.isEmpty else { return nil }
            return Approval(action: action, generation: generation, context: memoryBytes)
        case .initialize:
            guard memory?.state == "uninitialized", !deliveryUnknown else { return nil }
            return Approval(action: action, generation: generation, context: memoryBytes)
        case .reconcile(let id):
            guard let value = pending.first(where: { $0.id == id }) else { return nil }
            return Approval(action: action, generation: generation, context: value.bytes)
        }
    }
    @discardableResult func confirm(_ approval: Approval) async -> Bool {
        guard approval.generation == generation, self.approval(approval.action) == approval, begin() else { return false }
        defer { busy = false }
        historyVerified = false
        // Consume consent before the first suspension, then recheck custody.
        guard let statusRead = await read(Self.inspectArguments), statusRead.exitStatus == 0,
              let status = try? JSONDecoder().decode(MaatFailureMemoryStatus.self, from: statusRead.data) else {
            message = "Memory inspection failed before submission. Inspect again for new consent."; return false
        }
        switch approval.action {
        case .initialize:
            guard status.state == "uninitialized", statusRead.data == approval.context else {
                message = "Memory changed after review. Inspect again; initialization was not submitted."; return false
            }
            let args = maatCasebookFailureMemoryArguments(.initialize)
            guard let result = await read(args), result.exitStatus == 0,
                  let object = try? JSONSerialization.jsonObject(with: result.data) as? [String: Any],
                  object["status"] as? String == "initialized", object["imported_incidents"] as? Bool == false,
                  let uuid = object["store_uuid"] as? String,
                  let verified = await read(Self.inspectArguments), verified.exitStatus == 0,
                  let after = try? JSONDecoder().decode(MaatFailureMemoryStatus.self, from: verified.data),
                  after.state == "ready", after.storeUUID == uuid else {
                message = "Initialization outcome is not verified. Inspect local memory before another action; it will not be reset or initialized again automatically."; return false
            }
            memory = after; memoryBytes = verified.data
            message = "Private local memory initialized and read back. No incidents imported or observation recorded. Observe afresh and review a new record confirmation."
            return true
        case .record:
            guard status.state == "ready", statusRead.data == approval.context,
                  let store = status.storeUUID, let history = await read(Self.pendingArguments), history.exitStatus == 0,
                  let values = decodePending(history.data, store: store), values.isEmpty else {
                message = "Memory or delivery history changed after review. Inspect before recording; nothing was submitted."; return false
            }
            // A timeout/cancel/unknown append can never become an automatic retry.
            // Prior success belongs to its old observation, not this invocation.
            // In particular, a structured refusal may replace observation below.
            recorded = nil
            deliveryUnknown = true
            unknownRecordID = nil
            guard let result = await read(["maat", "triage", "--confirm", "--json"]) else {
                message = "Record outcome is unknown. Inspect exact record delivery; do not resend."; return false
            }
            if result.exitStatus == 0,
               let object = try? JSONSerialization.jsonObject(with: result.data) as? [String: Any],
               object["record_state"] as? String == "recorded_verified",
               let id = object["record_id"] as? String, MaatCapturedPreflightIdentity.digest(id),
               let value = try? JSONDecoder().decode(MaatSystemOneCommandResult.self, from: result.data),
               !value.decisionEvidence.isEmpty, !value.snapshotEvidence.isEmpty {
                recorded = value; observation = value.verdict; deliveryUnknown = false
                message = value.summary; return true
            }
            if let refusal = try? JSONDecoder().decode(Refusal.self, from: result.data), refusal.schema == Self.schema {
                observation = refusal.observation
                deliveryUnknown = !Self.knownNotSubmitted.contains(refusal.record_state)
                unknownRecordID = refusal.record_id.flatMap { MaatCapturedPreflightIdentity.digest($0) ? $0 : nil }
                message = "Observation remains available · record \(refusal.record_state). Inspect local memory and exact delivery; no record was inferred or resent."
            } else { message = "Record reply was unverifiable. Inspect exact delivery; do not resend." }
            return false
        case .reconcile(let id):
            guard status.state == "ready", let store = status.storeUUID,
                  let history = await read(Self.pendingArguments), history.exitStatus == 0,
                  let values = decodePending(history.data, store: store),
                  values.first(where: { $0.id == id })?.bytes == approval.context else {
                message = "Selected record custody changed. Inspect again; reconciliation was not submitted."; return false
            }
            guard let result = await read(["maat", "triage", "--reconcile-record", id, "--confirm", "--json"]), result.exitStatus == 0,
                  let value = try? JSONDecoder().decode(Reconciliation.self, from: result.data),
                  value.schema == Self.schema, value.id == id, MaatCapturedPreflightIdentity.digest(value.intent_sha256),
                  ["recorded_verified", "not_submitted"].contains(value.state),
                  value.state != "recorded_verified" || (!value.decision_evidence.isEmpty && MaatCapturedPreflightIdentity.digest(value.verdict_sha256)) else {
                message = "Exact delivery remains unverifiable. Retained intent remains available; inspect again. No observation or record was resent."; return false
            }
            pending.removeAll { $0.id == id }
            // Only resolution of the matching unknown intent may clear this latch.
            // A fresh inspection alone (including an empty list) cannot do so.
            if unknownRecordID == id { deliveryUnknown = false; unknownRecordID = nil }
            message = "Exact record reconciled as \(value.state). No observation was rerun. Inspect memory and use fresh observation/new consent for further work."
            memoryBytes = Data()
            return true
        }
    }
    private static let schema = "sirsi.maat.diagnostic-record.v1"
    private static let knownNotSubmitted: Set<String> = ["provenance_unknown", "provenance_changed", "memory_unavailable", "manifest_unverifiable", "preflight_refused", "receipt_unavailable", "journal_unavailable", "journal_readback_unavailable", "not_submitted"]
    private struct Refusal: Decodable { let schema: String; let record_state: String; let record_id: String?; let observation: MaatSystemOneVerdict }
    private struct Reconciliation: Decodable {
        let schema: String; let id: String; let state: String
        let intent_sha256: String; let decision_evidence: String; let verdict_sha256: String
    }
    private func decodePending(_ data: Data, store: String) -> [Pending]? {
        guard let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              object["schema"] as? String == Self.schema else { return nil }
        // Go encodes a nil pending slice as null; null is an explicit empty list.
        let rows: [[String: Any]]
        if object["pending"] is NSNull { rows = [] }
        else if let values = object["pending"] as? [[String: Any]], values.count <= 128 { rows = values }
        else { return nil }
        var seen = Set<String>(), values: [Pending] = []
        for row in rows {
            guard row["schema"] as? String == Self.schema, let id = row["id"] as? String,
                  MaatCapturedPreflightIdentity.digest(id), seen.insert(id).inserted,
                  let manifest = row["manifest"] as? [String: Any], manifest["schema"] as? String == Self.schema,
                  (manifest["uid"] as? NSNumber)?.uint32Value == uid, manifest["store_uuid"] as? String == store,
                  let observation = manifest["observation"] as? [String: Any],
                  observation["schema"] as? String == "sirsi.maat.local-diagnostic-observation.v1",
                  let source = observation["source_sha256"] as? String, MaatCapturedPreflightIdentity.digest(source),
                  let host = observation["host_sha256"] as? String, MaatCapturedPreflightIdentity.digest(host),
                  let observed = observation["observed_at_utc"] as? String,
                  let receipt = row["receipt"], let receiptBytes = try? JSONSerialization.data(withJSONObject: receipt),
                  let reference = try? JSONDecoder().decode(MaatPreflightReceiptReference.self, from: receiptBytes),
                  reference.store_binding.valid, reference.store_binding.state == "initialized",
                  reference.store_binding.store_uuid == store, reference.registry_snapshot_version == 2,
                  reference.action == MaatPreflightScope(component: "pantheon", profile: "local-macos", operation: "record-system-one-diagnostic"),
                  reference.action_sha256 == id, MaatCapturedPreflightIdentity.digest(reference.receipt_sha256),
                  MaatCapturedPreflightIdentity.digest(reference.registry_sha256),
                  let bytes = try? JSONSerialization.data(withJSONObject: row, options: [.sortedKeys]) else { return nil }
            values.append(Pending(id: id, store: store, observed: observed, bytes: bytes))
        }
        return values
    }
}
