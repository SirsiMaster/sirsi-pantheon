import Foundation
import SwiftUI

// One engine-owned invocation/consent lifetime, independent of Casebook rows.
// All effects remain in the canonical CLI; this model never opens local stores.
@MainActor final class MaatDiagnosticRecordModel: ObservableObject {
    typealias Reader = @MainActor ([String]) async -> SirsiEngine.JSONCommandRead
    enum Action: Equatable { case record, initialize, reconcile(String), resolveRequest(String) }
    struct Approval: Equatable {
        let action: Action
        let generation: UUID
        let context: Data
    }
    // One bounded historical completion, separate from current observation and
    // readiness. Inspection/navigation must not erase its exact result evidence.
    enum Completion: Equatable {
        case initialized(store: String, readbackState: String, importedIncidents: Bool)
        case recorded(id: String, store: String, snapshot: String, decision: String)
        case reconciled(id: String, store: String, state: String, intent: String, decision: String, verdict: String)

        var details: [String] {
            switch self {
            case let .initialized(store, readback, imported):
                return ["Initialization verified", "Store: \(store)", "Readback: \(readback)", "Imported incidents: \(imported ? "yes" : "no")", "No observation or action was recorded by initialization."]
            case let .recorded(id, store, snapshot, decision):
                return ["Record verified", "Record: \(id)", "Store: \(store)", "Snapshot: \(snapshot)", "Decision: \(decision)", "Recording is not a health or repair success."]
            case let .reconciled(id, store, state, intent, decision, verdict):
                return ["Exact delivery reconciled", "Record: \(id)", "Store: \(store)", "State: \(state)", "Intent SHA256: \(intent)", "Decision: \(decision.isEmpty ? "none" : decision)", "Verdict SHA256: \(verdict.isEmpty ? "none" : verdict)", "No observation was rerun or record resent."]
            }
        }
    }
    @Published private(set) var completed: Completion?
    struct Pending: Identifiable {
        let id: String
        let store: String
        let observed: String
        let bytes: Data
    }
    @Published private(set) var requestCompletion: MaatDiagnosticWire.Completion?
    @Published private(set) var activeRequestID: String?
    @Published private(set) var requests: [MaatDiagnosticWire.Request] = []
    @Published private(set) var records: [MaatDiagnosticWire.Record] = []
    @Published private(set) var discoveryComplete = false
    @Published private(set) var discoveryRestartRequired = false
    @Published private(set) var discoveryCursor: String?
    private var discoveryQuery: String?
    private var discoverySnapshot: String?
    private var seenRows = Set<String>()
    private var seenCursors = Set<String>()
    private var discoveryStore: String?
    private(set) var lastOperationRecorded = false
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
    private var queryOwnedHistoryRefresh: (@MainActor () async -> Void)?
    func bindQueryOwnedHistoryRefresh(_ refresh: @escaping @MainActor () async -> Void) {
        queryOwnedHistoryRefresh = refresh
    }
    func refreshRecordedHistory() async { await queryOwnedHistoryRefresh?() }
    private var activeRequestStore: String?
    private var task: Task<SirsiEngine.JSONCommandRead, Never>?
    private var operationCancelled = false
    private let reader: Reader
    private let uid: UInt32
    private let localHost: @MainActor () -> String?
    private let newRequestID: @MainActor () -> UUID
    init(uid: UInt32 = getuid(), localHost: @escaping @MainActor () -> String? = maatLocalDiagnosticHost,
         newRequestID: @escaping @MainActor () -> UUID = { UUID() }, reader: Reader? = nil) {
        self.uid = uid; self.localHost = localHost; self.newRequestID = newRequestID
        self.reader = reader ?? { await SirsiEngine.runJSONRead(args: $0, timeoutSeconds: 30) }
    }
    static let inspectArguments = ["maat", "repair", "liveness-watch", "--inspect-failure-memory", "--json"]
    static let pendingArguments = ["maat", "triage", "--inspect-record-requests", "--json", "--request-limit", "50"]
    static func discoveryArguments(cursor: String?) -> [String] {
        pendingArguments + (cursor.map { ["--request-cursor", $0] } ?? [])
    }
    static func requestArguments(_ operation: String, id: String, confirm: Bool = false) -> [String] {
        let flag = operation == "record" ? "--record-request" : "--" + operation + "-record-request"
        return ["maat", "triage", flag, id] + (confirm ? ["--confirm"] : []) + ["--json"]
    }
    func cancel() { operationCancelled = true; task?.cancel() }
    private func read(_ args: [String]) async -> SirsiEngine.JSONCommandRead? {
        guard !operationCancelled, !Task.isCancelled else { return nil }
        let child = Task { @MainActor in await self.reader(args) }
        task = child
        let result = await withTaskCancellationHandler(operation: { await child.value }, onCancel: { child.cancel() })
        task = nil
        guard !operationCancelled, !child.isCancelled, !Task.isCancelled, !result.timedOut, result.launchError == nil,
              result.exitStatus != nil, result.data.count <= 8 * 1024 * 1024 else { return nil }
        return result
    }
    private func begin() -> Bool {
        guard !busy, !Task.isCancelled else { return false }
        generation = UUID(); operationCancelled = false; busy = true
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
        memory = nil; memoryBytes = Data(); historyVerified = false
        discoveryComplete = false; discoveryRestartRequired = true
        guard let result = await read(Self.inspectArguments), result.exitStatus == 0,
              let status = try? JSONDecoder().decode(MaatFailureMemoryStatus.self, from: result.data) else {
            message = "Local memory is unverifiable. Retained results remain available; inspect again. No reset or record was attempted."; return
        }
        memory = status; memoryBytes = result.data
        guard status.state == "ready", let store = status.storeUUID else {
            message = status.state == "uninitialized" ? "Local memory has not been initialized. Review first-use initialization; no incidents will be imported." : "Local memory is unavailable. No record was attempted."
            return
        }
        requests = []; records = []; pending = []; seenRows = []; seenCursors = []
        discoveryQuery = nil; discoverySnapshot = nil; discoveryCursor = nil
        discoveryComplete = false; discoveryRestartRequired = false; discoveryStore = store
        await loadDiscoveryBatch(store: store)
    }
    // Finite work per UI turn, with explicit continuation rather than a total
    // history cap. Never infer completion from an empty requests collection.
    func continueDiscovery() async {
        guard discoveryCursor != nil, !discoveryRestartRequired, let store = discoveryStore, begin() else { return }
        defer { busy = false }
        await loadDiscoveryBatch(store: store)
    }
    private func loadDiscoveryBatch(store: String) async {
        historyVerified = false
        for _ in 0..<8 {
            let cursor = discoveryCursor
            guard let result = await read(Self.discoveryArguments(cursor: cursor)), result.exitStatus == 0,
                  let page = try? JSONDecoder().decode(MaatDiagnosticWire.Page.self, from: result.data),
                  page.valid(uid: uid, store: store, after: cursor != nil),
                  discoveryQuery == nil || (discoveryQuery == page.history.query_id && discoverySnapshot == page.history.snapshot) else {
                discoveryRestartRequired = true; discoveryComplete = false
                message = "Discovery is partial or changed. Retained rows and completed receipts remain historical; restart discovery. Known request IDs can still be read or resolved exactly."
                return
            }
            let keys = page.records.map { "record:" + $0.id } + page.requests.map { "request:" + $0.id }
            guard Set(keys).count == keys.count, seenRows.isDisjoint(with: keys),
                  page.history.next_cursor.map({ !seenCursors.contains($0) && $0 != cursor }) ?? true else {
                discoveryRestartRequired = true; message = "Discovery repeated a row or cursor. Restart without splicing snapshots."; return
            }
            discoveryQuery = page.history.query_id; discoverySnapshot = page.history.snapshot
            seenRows.formUnion(keys)
            records.append(contentsOf: page.records); requests.append(contentsOf: page.requests)
            discoveryCursor = page.history.next_cursor
            if let next = discoveryCursor { seenCursors.insert(next) }
            let linked = Set(requests.compactMap { $0.intent?.id })
            pending = records.filter { $0.state == "pending" && !linked.contains($0.id) }.compactMap { row in
                // Legacy exact reconciliation uses only validated canonical row
                // identity, captured before review, never an invented request.
                Pending(id: row.id, store: store, observed: row.intent.manifest.observation.observed_at_utc,
                        bytes: Data((row.id + ":" + row.intent.receipt.receipt_sha256).utf8))
            }
            if page.history.coverage == "complete" {
                discoveryComplete = true; historyVerified = true
                message = requests.contains(where: { !$0.terminal }) || !pending.isEmpty
                    ? "Exact retained delivery debt must be resolved first. No observation will be replayed."
                    : (deliveryUnknown ? "Discovery complete, but the retained unknown request still needs exact read or confirmed resolution. No resend is offered." : "Discovery complete. Local memory is ready for a fresh separately confirmed record.")
                return
            }
        }
        message = "Discovery is partial. Continue the same snapshot before new recording; completed receipts remain available."
    }
    func approval(_ action: Action) -> Approval? {
        guard !busy else { return nil }
        switch action {
        case .record:
            guard memory?.state == "ready", historyVerified, discoveryComplete, !deliveryUnknown, pending.isEmpty,
                  !requests.contains(where: { !$0.terminal }),
                  !memoryBytes.isEmpty else { return nil }
            return Approval(action: action, generation: generation, context: memoryBytes)
        case .initialize:
            guard memory?.state == "uninitialized", !deliveryUnknown else { return nil }
            return Approval(action: action, generation: generation, context: memoryBytes)
        case .resolveRequest(let id):
            guard MaatDiagnosticWire.uuid(id), let store = memory?.storeUUID, memory?.state == "ready",
                  activeRequestID != id || activeRequestStore == store,
                  (activeRequestID == id && activeRequestStore == store || requests.contains(where: { $0.id == id && !$0.terminal })) else { return nil }
            return Approval(action: action, generation: generation, context: resolutionContext(requests.first { $0.id == id }, id: id, store: store))
        case .reconcile(let id):
            guard historyVerified, discoveryComplete, !deliveryUnknown, let value = pending.first(where: { $0.id == id }) else { return nil }
            return Approval(action: action, generation: generation, context: value.bytes)
        }
    }
    @discardableResult func confirm(_ approval: Approval) async -> Bool {
        guard approval.generation == generation, self.approval(approval.action) == approval, begin() else { return false }
        defer { busy = false }
        historyVerified = false; lastOperationRecorded = false
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
                  after.state == "ready", after.storeUUID == uuid, uuid.utf8.count <= 64 else {
                message = "Initialization outcome is not verified. Inspect local memory before another action; it will not be reset or initialized again automatically."; return false
            }
            memory = after; memoryBytes = verified.data
            completed = .initialized(store: uuid, readbackState: after.state, importedIncidents: false)
            message = "Private local memory initialized and read back. No incidents imported or observation recorded. Observe afresh and review a new record confirmation."
            return true
        case .record:
            guard status.state == "ready", statusRead.data == approval.context, let store = status.storeUUID else {
                message = "Memory changed after review. Nothing was submitted."; return false
            }
            // Approval was possible only after a COMPLETE chain. The Go
            // page snapshot covers the full strict namespace, not just this
            // page, so revalidate that exact snapshot without restarting a
            // large history at page one forever. Go repeats actual preflight.
            guard discoveryComplete, !deliveryUnknown, pending.isEmpty,
                  !requests.contains(where: { !$0.terminal }),
                  await sameDiscoverySnapshot(store: store) else {
                discoveryRestartRequired = true; discoveryComplete = false
                message = "Delivery history changed after review. Restart discovery and resolve debt first; nothing was submitted."; return false
            }
            // Retain the UUID BEFORE the first preparation suspension. Even a
            // lost preparation reply is resolved by this key, never retried.
            let id = newRequestID().uuidString.lowercased()
            activeRequestID = id; activeRequestStore = store; deliveryUnknown = true; recorded = nil
            guard let preparedRead = await read(Self.requestArguments("prepare", id: id)), preparedRead.exitStatus == 0,
                  let prepared = decodeRequest(preparedRead.data, id: id, store: store), prepared.state == "prepared" else {
                message = "Preparation outcome is unknown. Read or explicitly resolve the retained request; no record was submitted or replayed."; return false
            }
            let result = await read(Self.requestArguments("record", id: id, confirm: true))
            // A nonzero reply can still contain an observation; it is not proof
            // of delivery. Only the independent exact request read can settle it.
            if let result, let refusal = try? JSONDecoder().decode(Refusal.self, from: result.data), refusal.schema == Self.schema {
                observation = refusal.observation
            }
            guard let exactRead = await read(Self.requestArguments("read", id: id)), exactRead.exitStatus == 0,
                  let exact = decodeRequest(exactRead.data, id: id, store: store) else {
                message = "Record outcome remains unknown. Read or resolve this exact request; do not resend."; return false
            }
            retain(exact, completing: true)
            guard let result, result.exitStatus == 0, exact.state == "recorded_verified", let proof = exact.result,
                  let value = try? JSONDecoder().decode(MaatSystemOneCommandResult.self, from: result.data),
                  let object = try? JSONSerialization.jsonObject(with: result.data) as? [String: Any],
                  object["record_id"] as? String == proof.id, object["record_state"] as? String == "recorded_verified",
                  value.decisionEvidence == proof.decision_evidence,
                  value.snapshotEvidence == "diagnostic:sha256=" + (exact.intent?.manifest.observation.source_sha256 ?? ""),
                  value.verdict.subject.headSHA == exact.intent?.manifest.observation.source_sha256,
                  value.verdict.contract.isEmpty, value.verdict.schemaVersion == 1,
                  maatDiagnosticHostMatches(value.verdict.subject, localHost: localHost() ?? ""),
                  ["pass", "changes", "block", "escalate"].contains(value.verdict.gate) else {
                message = exact.terminal ? "Exact request disposition retained. No verified new screen was inferred; inspect its receipt." : "Retained request needs exact data-only resolution. No record is replayed."
                return false
            }
            recorded = value; observation = value.verdict
            completed = .recorded(id: proof.id, store: store, snapshot: value.snapshotEvidence, decision: value.decisionEvidence)
            lastOperationRecorded = true; message = value.summary
            return true
        case .resolveRequest(let id):
            guard status.state == "ready", let store = status.storeUUID,
                  approval.context == resolutionContext(requests.first { $0.id == id }, id: id, store: store),
                  await sameResolutionTuple(id: id, store: store, context: approval.context),
                  let result = await read(Self.requestArguments("resolve", id: id, confirm: true)), result.exitStatus == 0,
                  let exact = decodeRequest(result.data, id: id, store: store), exact.terminal else {
                message = "Exact request remains unverifiable. Retain its ID; inspect/resolve only, never replay."; return false
            }
            retain(exact, completing: true)
            lastOperationRecorded = exact.state == "recorded_verified"
            message = "Exact request resolved as \(exact.state). No observation or append was rerun."
            return true
        case .reconcile(let id):
            guard status.state == "ready", let store = status.storeUUID,
                  discoveryComplete, await sameDiscoverySnapshot(store: store),
                  let selected = pending.first(where: { $0.id == id }), selected.bytes == approval.context else {
                message = "Selected record custody changed. Inspect again; reconciliation was not submitted."; return false
            }
            guard let result = await read(["maat", "triage", "--reconcile-record", id, "--confirm", "--json"]), result.exitStatus == 0,
                  let value = try? JSONDecoder().decode(Reconciliation.self, from: result.data),
                  value.schema == Self.schema, value.id == id, MaatCapturedPreflightIdentity.digest(value.intent_sha256),
                  ["recorded_verified", "not_submitted"].contains(value.state),
                  value.state != "recorded_verified" || (!value.decision_evidence.isEmpty && MaatCapturedPreflightIdentity.digest(value.verdict_sha256)),
                  value.decision_evidence.utf8.count <= 256, value.verdict_sha256.utf8.count <= 64,
                  store.utf8.count <= 64 else {
                message = "Exact delivery remains unverifiable. Retained intent remains available; inspect again. No observation or record was resent."; return false
            }
            lastOperationRecorded = value.state == "recorded_verified"
            completed = .reconciled(id: id, store: store, state: value.state,
                                    intent: value.intent_sha256, decision: value.decision_evidence, verdict: value.verdict_sha256)
            pending.removeAll { $0.id == id }
            message = "Exact record reconciled as \(value.state). No observation was rerun. Inspect memory and use fresh observation/new consent for further work."
            memoryBytes = Data()
            return true
        }
    }
    private static let schema = "sirsi.maat.diagnostic-record.v1"
    private struct Refusal: Decodable { let schema: String; let record_state: String; let record_id: String?; let observation: MaatSystemOneVerdict }
    private struct Reconciliation: Decodable {
        let schema: String; let id: String; let state: String
        let intent_sha256: String; let decision_evidence: String; let verdict_sha256: String
    }
    private func sameDiscoverySnapshot(store: String) async -> Bool {
        guard let read = await read(Self.pendingArguments), read.exitStatus == 0,
              let page = try? JSONDecoder().decode(MaatDiagnosticWire.Page.self, from: read.data),
              page.valid(uid: uid, store: store, after: false) else { return false }
        return page.history.query_id == discoveryQuery && page.history.snapshot == discoverySnapshot
    }
    private func resolutionContext(_ value: MaatDiagnosticWire.Request?, id: String, store: String) -> Data {
        // Exact bounded metadata shown in review, not an invented digest/ID.
        let parts = [id, store, String(uid), value?.state ?? "unknown", value?.intent?.id ?? "",
                     value?.intent?.receipt.receipt_sha256 ?? "", value?.result?.intent_sha256 ?? "",
                     value?.result?.decision_evidence ?? "", value?.result?.verdict_sha256 ?? ""]
        return (try? JSONSerialization.data(withJSONObject: parts)) ?? Data()
    }
    private func sameResolutionTuple(id: String, store: String, context: Data) async -> Bool {
        // An unknown preparation may have created NO leaf. Explicit resolution
        // is allowed to establish/fence that same retained UUID. It never calls
        // record/prepare. Known tuples must survive an exact read unchanged.
        guard requests.contains(where: { $0.id == id }) else { return activeRequestID == id && activeRequestStore == store }
        guard let read = await read(Self.requestArguments("read", id: id)), read.exitStatus == 0,
              let value = decodeRequest(read.data, id: id, store: store) else { return false }
        return resolutionContext(value, id: id, store: store) == context
    }
    private func decodeRequest(_ data: Data, id: String, store: String) -> MaatDiagnosticWire.Request? {
        guard let value = try? JSONDecoder().decode(MaatDiagnosticWire.Request.self, from: data),
              value.request.id == id, value.valid(uid: uid, store: store) else { return nil }
        return value
    }
    private func retain(_ value: MaatDiagnosticWire.Request, completing: Bool = false) {
        if let index = requests.firstIndex(where: { $0.id == value.id }) { requests[index] = value }
        else { requests.append(value) }
        guard value.terminal else { return }
        if completing || activeRequestID == value.id && activeRequestStore == value.request.store_uuid {
            requestCompletion = MaatDiagnosticWire.Completion(value)
        }
        if activeRequestID == value.id && activeRequestStore == value.request.store_uuid {
            deliveryUnknown = false; activeRequestID = nil; activeRequestStore = nil
        }
    }
    @discardableResult func readRequest(_ id: String) async -> Bool {
        guard MaatDiagnosticWire.uuid(id),
              let store = activeRequestID == id ? activeRequestStore : memory?.storeUUID,
              MaatDiagnosticWire.uuid(store), begin() else { return false }
        defer { busy = false }
        guard let result = await read(Self.requestArguments("read", id: id)), result.exitStatus == 0,
              let value = decodeRequest(result.data, id: id, store: store) else {
            message = "Exact request lookup unavailable. Unknown custody is retained; resolve only, never resend."; return false
        }
        retain(value)
        message = "Exact request state: \(value.state). Reading never resubmits the request."
        return value.state == "recorded_verified"
    }
}
