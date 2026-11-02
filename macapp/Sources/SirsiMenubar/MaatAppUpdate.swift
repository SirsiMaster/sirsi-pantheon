import SwiftUI
import Foundation

// One app-update action in the existing engine-owned Ma'at operation store.
// Memory initialization/recovery stays with its existing owner; this never
// creates a store, interprets stderr as authority, or opens Installer itself.
@MainActor final class MaatAppUpdateModel: ObservableObject {
    enum Action: Equatable {
        case update
        case rollback(AppRollbackSelection)
        var arguments: [String] {
            switch self {
            case .update: return ["update", "--app"]
            case let .rollback(selection): return selection.commandArguments
            }
        }
        var title: String {
            switch self {
            case .update: return "Pantheon update"
            case let .rollback(selection): return "Pantheon rollback to \(selection.release)"
            }
        }
    }
    struct Approval: Equatable {
        let action: Action
        let storeUUID: String
        let generation: UUID
        let maintenanceEpoch: UUID
    }
    typealias Capture = @MainActor ([String], @escaping ProcessLaunchAuthorization) async -> CommandExecution
    let memory: MaatDiagnosticRecordModel
    let recovery: MaatPreparedPreflightRecovery
    let maintenance: LegacyRelayMaintenanceModel
    @Published private(set) var pendingAction: Action?
    @Published private(set) var approval: Approval?
    @Published private(set) var busy = false
    @Published private(set) var message: String?
    @Published private(set) var commandReceipt: CommandExecution?
    @Published private(set) var readbackReceipt: CommandExecution?
    @Published private(set) var attemptReport: AppUpdateReconciliationReport?
    private var generation = UUID()
    private var work: Task<Void, Never>?
    private let capture: Capture

    init(memory: MaatDiagnosticRecordModel, recovery: MaatPreparedPreflightRecovery,
         maintenance: LegacyRelayMaintenanceModel, capture: Capture? = nil) {
        self.memory = memory; self.recovery = recovery; self.maintenance = maintenance
        self.capture = capture ?? { args, authorize in
            await LegacyRelayInstallerCapture().capture(args: args, authorize: authorize)
        }
    }
    private func readyStore(_ status: MaatFailureMemoryStatus?) -> String? {
        guard let status, status.state == "ready", let store = status.storeUUID,
              MaatDiagnosticWire.uuid(store), !status.incompleteAttempt, !status.projectionDebt,
              status.preparedPreflight == nil, status.receiptRecoveryError == nil else { return nil }
        return store
    }
    func review(_ action: Action) -> Task<Void, Never>? {
        guard !busy, !memory.busy, !Task.isCancelled else { return nil }
        guard !maintenance.busy else {
            pendingAction = action; approval = nil
            message = "Another app/legacy operation is still in flight. Wait for owned cleanup, then review this saved request again."
            return nil
        }
        let epoch = maintenance.managedConsentEpoch
        generation = UUID(); let token = generation
        pendingAction = action; approval = nil; busy = true
        message = "Inspecting local memory before reviewing \(action.title). No update or Installer command has started."
        let task = Task { @MainActor in
            defer { self.busy = false; self.work = nil }
            let status = await self.memory.inspectForManagedAction()
            guard self.generation == token, !Task.isCancelled else { return }
            guard !self.maintenance.busy, self.maintenance.managedConsentEpoch == epoch else {
                self.message = "Another app/legacy operation ran during inspection. Review this request again after its cleanup."
                return
            }
            guard let store = self.readyStore(status) else {
                self.message = "Local memory is not ready. Use the initialization or recovery controls below, then recheck this saved update. Nothing was installed."
                return
            }
            self.approval = Approval(action: action, storeUUID: store, generation: token, maintenanceEpoch: epoch)
            self.message = "Local memory was read back. Review \(action.title) and give fresh confirmation; Go still performs its complete preflight and retained-attempt checks."
        }
        work = task; return task
    }
    func reviewSavedAction() -> Task<Void, Never>? {
        guard let pendingAction else { return nil }
        return review(pendingAction)
    }
    func confirm(_ approved: Approval) -> Task<Void, Never>? {
        guard !busy, !memory.busy, approval == approved, generation == approved.generation else { return nil }
        let token = generation
        guard let lease = maintenance.beginManagedAppRequest(expectedEpoch: approved.maintenanceEpoch, cancel: { [weak self] in self?.cancel() }) else {
            approval = nil
            message = "Another app/legacy operation changed custody. Wait for cleanup and review afresh; nothing was submitted."
            return nil
        }
        guard generation == token else { maintenance.finishManagedAppRequest(lease); return nil }
        approval = nil; busy = true; attemptReport = nil; readbackReceipt = nil
        message = "Rechecking the reviewed memory store before the canonical app-update command…"
        let task = Task { @MainActor in
            defer { self.maintenance.finishManagedAppRequest(lease); self.busy = false; self.work = nil }
            let status = await self.memory.inspectForManagedAction()
            guard self.generation == token, self.maintenance.managedRequestIsCurrent(lease), !Task.isCancelled else { return }
            guard self.readyStore(status) == approved.storeUUID else {
                self.message = "Memory changed or is not verifiable. No update command was submitted; inspect recovery and review again."
                return
            }
            let result = await self.capture(approved.action.arguments, {
                self.generation == token && self.maintenance.managedRequestIsCurrent(lease) && !Task.isCancelled
            })
            self.commandReceipt = result
            guard self.generation == token, self.maintenance.managedRequestIsCurrent(lease), !Task.isCancelled else {
                self.message = "The request was canceled. Read its retained attempt before another review; cancellation is not proof of installation or rollback."
                return
            }
            self.message = result.complete && result.exitCode == 0
                ? "The canonical command completed. Read the retained app attempt below; command completion alone does not prove installation."
                : "The canonical command did not return a verified completion. Its receipt is retained. Inspect local memory and the app attempt; nothing is retried automatically."
        }
        work = task; return task
    }
    func inspectAttempt() -> Task<Void, Never>? {
        guard !busy, !Task.isCancelled else { return nil }
        approval = nil; attemptReport = nil; generation = UUID(); let token = generation; busy = true
        let task = Task { @MainActor in
            defer { self.busy = false; self.work = nil }
            let result = await self.capture(["update", "--reconcile-app-attempt"], {
                self.generation == token && !Task.isCancelled
            })
            self.readbackReceipt = result
            guard self.generation == token, !Task.isCancelled, result.complete,
                  let report = try? JSONDecoder().decode(AppUpdateReconciliationReport.self, from: result.stdoutData),
                  report.isBound,
                  result.exitCode == 0 || !["resolved", "no_pending_attempt"].contains(report.state) else {
                self.message = "App-attempt readback is unverified. The prior command receipt remains; no update was replayed."
                return
            }
            self.attemptReport = report
            self.message = report.state == "no_pending_attempt"
                ? "No pending attempt was observed. This does not establish the outcome of an earlier unknown command. Review fresh state before another request."
                : "Retained app-attempt observation: \(report.state) · target \(report.targetRelease ?? "unidentified"). No Installer replay was requested."
        }
        work = task; return task
    }
    func cancel() {
        generation = UUID(); approval = nil; work?.cancel()
        message = "Request canceled; retained receipts remain. No automatic update or initialization retry."
    }
}

@MainActor struct MaatAppUpdateControl: View {
    @ObservedObject var model: MaatAppUpdateModel
    @ObservedObject private var maintenance: LegacyRelayMaintenanceModel
    init(model: MaatAppUpdateModel) { self.model = model; self.maintenance = model.maintenance }
    @State private var rollbackRelease = ""
    @State private var reviewed: MaatAppUpdateModel.Approval?
    @State private var confirming = false
    var memoryControl: MaatDiagnosticRecordView { MaatDiagnosticRecordView(model: model.memory, refresh: {}) }
    var recoveryControl: MaatWorkspaceMemoryEntry { MaatWorkspaceMemoryEntry(model: model.recovery) }
    func reviewUpdate() -> Task<Void, Never>? { model.review(.update) }
    func reviewRollback(_ selection: AppRollbackSelection) -> Task<Void, Never>? { model.review(.rollback(selection)) }
    func reviewSavedAction() -> Task<Void, Never>? { model.reviewSavedAction() }
    func confirm(_ value: MaatAppUpdateModel.Approval) -> Task<Void, Never>? { model.confirm(value) }
    func cancel() { model.cancel() }
    var body: some View {
        DisclosureGroup("Managed Pantheon update and local memory recovery") {
            VStack(alignment: .leading, spacing: 8) {
                Text("Initialization is a separate confirmed local action. It never starts Installer. After verified memory, recheck the saved update and confirm it separately.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                Button("Review Pantheon update") {
                    Task { await self.reviewUpdate()?.value; self.reviewed = self.model.approval; self.confirming = self.reviewed != nil }
                }.buttonStyle(.bordered).disabled(model.busy || maintenance.busy)
                HStack {
                    TextField("Exact older release", text: $rollbackRelease).textFieldStyle(.roundedBorder)
                    Button("Review rollback") {
                        guard let selection = AppRollbackSelection(release: self.rollbackRelease) else { return }
                        Task { await self.reviewRollback(selection)?.value; self.reviewed = self.model.approval; self.confirming = self.reviewed != nil }
                    }.buttonStyle(.bordered).disabled(model.busy || maintenance.busy || AppRollbackSelection(release: rollbackRelease) == nil)
                }
                if model.pendingAction != nil {
                    Button("Recheck saved update after memory recovery") {
                        Task { await self.reviewSavedAction()?.value; self.reviewed = self.model.approval; self.confirming = self.reviewed != nil }
                    }.buttonStyle(.bordered).disabled(model.busy || maintenance.busy)
                }
                if let message = model.message { Text(message).sirsiFont(.caption).textSelection(.enabled) }
                if let receipt = model.commandReceipt {
                    Text("Last command receipt · \(receipt.complete ? "terminal" : "unverified") · exit \(receipt.exitCode.map { String($0) } ?? "unknown")")
                        .sirsiFont(.caption)
                    if !receipt.stderr.isEmpty { Text(receipt.stderr).sirsiFont(.caption).textSelection(.enabled) }
                }
                Button("Read retained app attempt") { _ = self.model.inspectAttempt() }
                    .buttonStyle(.bordered).disabled(model.busy)
                if let report = model.attemptReport {
                    Text("Retained observation: \(report.state) · attempt \(report.attemptID ?? "not identified") · target \(report.targetRelease ?? "unidentified")")
                        .sirsiFont(.caption).textSelection(.enabled)
                }
                if model.busy {
                    HStack { ProgressView("Reading the selected update operation…"); Button("Cancel request") { self.cancel() } }
                }
                memoryControl.disabled(model.busy)
                recoveryControl.disabled(model.busy)
            }.padding(.top, 8)
        }
        .onDisappear { self.reviewed = nil; self.confirming = false }
        .confirmationDialog("Confirm this managed app request?", isPresented: $confirming, titleVisibility: .visible) {
            Button("Confirm reviewed request") { if let value = self.reviewed { _ = self.confirm(value) }; self.reviewed = nil }
            Button("Cancel", role: .cancel) { self.reviewed = nil; self.cancel() }
        } message: {
            Text("\(reviewed?.action.title ?? "No reviewed action")\nMemory store: \(reviewed?.storeUUID ?? "unverified")\nThe canonical Go owner must retain complete preflight and a durable attempt before Installer. Ambiguous outcomes require readback; no automatic replay.")
        }
    }
}
