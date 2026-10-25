import SwiftUI
import Foundation
import UniformTypeIdentifiers

struct LegacyRelayMaintenancePanel: View {
    let directory: URL
    @Environment(\.snapshotMode) private var snapshotMode
    @ObservedObject private var model: LegacyRelayMaintenanceModel
    @State private var picker=false
    @State private var confirming=false
    @State private var approved: LegacyRelayMaintenanceModel.Approval?
    @State private var selectedAction: String?
    @State private var selectedTransaction: LegacyRelayTransaction?
    @State private var confirmRecovery=false
    @State private var pickingRestore=false
    @State private var confirmRestore=false
    @State private var approvedRestore: LegacyRelayMaintenanceModel.RestoreApproval?
    @State private var approvedInstaller: LegacyRelayMaintenanceModel.InstallerApproval?
    @State private var confirmInstaller=false
    @State private var rollbackRelease=""
    @State private var approvedRollback:AppRollbackSelection?
    @State private var confirmRollback=false
    @State private var approvedPublicBinding:LegacyRelayMaintenanceModel.PublicBindingApproval?
    @State private var confirmPublicBinding=false

    init(directory:URL,model:LegacyRelayMaintenanceModel){self.directory=directory;self.model=model}

    var body: some View {
        DisclosureGroup("Pantheon and legacy relay maintenance") {
            VStack(alignment:.leading,spacing:10){
                Text("Optional fallback for a recognized legacy root relay. Pantheon stays unprivileged; macOS asks permission for only the reviewed operation using the protected installed engine. Producer-covered maintenance remains separate.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                HStack {
                    Button("Choose retained legacy payload"){picker=true}.buttonStyle(.bordered).disabled(model.busy)
                    Button("Review"){Task{await model.review()}}.buttonStyle(.bordered).disabled(model.busy||model.selectedFile == nil||model.outcomeUnknown||model.lifecycleDebt)
                    Button("Read current transaction"){Task{await model.readStatus()}}.buttonStyle(.bordered)
                }.disabled(snapshotMode)
                if let notice=model.statusNotice {Text(notice).sirsiFont(.caption).foregroundStyle(.secondary)}
                if let report=model.maatAppAttemptReport, report.state == "unresolved" || report.state == "resolved" {
                    Text("Retained Ma’at attempt \(report.attemptID ?? "unavailable") · source \(report.currentRelease ?? "unknown") · target \(report.targetRelease ?? "unknown") · \(report.state)")
                        .sirsiFont(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                }
                HStack {
                    Button("Check compatible Installer"){Task{await model.prepareInstaller()}}.buttonStyle(.bordered)
                    Button("Recheck installed pair"){Task{await model.recheckInstalledPair()}}.buttonStyle(.bordered)
                    Button("Recheck retained Ma’at app attempt"){Task{await model.recheckMaatAppAttempt()}}.buttonStyle(.bordered)
                }.disabled(model.busy||snapshotMode)
                Divider()
                Text("Restore one exact older Pantheon release through the retained Ma'at package workflow.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                HStack {
                    TextField("Exact older release (target recovery support is checked from its signed helper)", text:$rollbackRelease).textFieldStyle(.roundedBorder)
                    Button("Review rollback") {
                        if let selection=AppRollbackSelection(release:rollbackRelease) { approvedRollback=selection;confirmRollback=true }
                    }.buttonStyle(.bordered).disabled(model.busy||snapshotMode||AppRollbackSelection(release:rollbackRelease)==nil)
                }.disabled(model.busy||snapshotMode)
                if let installer=model.installer {
                    Text("Verified normal PKG · \(installer.release)").sirsiFont(.caption)
                    Button("Open verified Installer"){approvedInstaller=installer;confirmInstaller=true}.buttonStyle(.bordered).disabled(model.busy||snapshotMode)
                }
                if let file=model.selectedFile{Text(file.path).sirsiFont(.caption).textSelection(.enabled)}
                if let plan=model.preview {
                    Text("Selected root relay · PID \(plan.original.pid) · \(plan.original.started)").sirsiFont(.caption)
                    Button("Review maintenance confirmation") {approved=model.confirmation();confirming=approved != nil}
                        .buttonStyle(.borderedProminent).tint(gold).disabled(!model.canConfirm||model.busy||snapshotMode)
                }
                if let transaction=model.transaction {
                    Text("\(transaction.phase) · \(transaction.preserved) envelopes retained · old remote outcomes unknown").sirsiFont(.subheadline)
                    Text(transaction.nextAction).sirsiFont(.caption).foregroundStyle(.secondary)
                    HStack {
                        Button("Resume activation"){select("recover",transaction)}.buttonStyle(.bordered)
                        if ["prepared","abort_requested","aborted"].contains(transaction.phase){Button("Abort preparation"){select("abort",transaction)}.buttonStyle(.bordered)}
                        Button("Review replacement restore"){pickingRestore=true;picker=true}.buttonStyle(.bordered)
                    }.disabled(model.busy||model.outcomeUnknown||snapshotMode)
                    if let restore=model.restoreApproval {
                        Text("Only the originally approved signed replacement will be restored. No legacy code or retained envelope will be replayed.").sirsiFont(.caption).foregroundStyle(.secondary)
                        Button("Confirm reviewed replacement restore"){approvedRestore=restore;confirmRestore=true}.buttonStyle(.bordered).disabled(model.busy||snapshotMode)
                    }
                }
                if model.busy{HStack{ProgressView().controlSize(.small);Text("Inspecting selected maintenance state…").sirsiFont(.caption);Button("Cancel request"){model.cancelInvocation()}.buttonStyle(.bordered)}}
                if let error=model.error{Text(error).sirsiFont(.caption).foregroundStyle(.secondary).textSelection(.enabled)}
                if model.outcomeUnknown { Text("The previous operation may still have started. Read its transaction before another confirmation; cancellation did not prove rollback.").sirsiFont(.caption).foregroundStyle(.secondary) }
                if model.publicStateUnreadable {
                    Button("Inspect public binding recovery"){Task{await model.inspectPublicBinding()}}.buttonStyle(.bordered)
                        .disabled(model.busy||model.outstanding != nil||snapshotMode)
                    if let binding=model.publicBindingApproval {
                        Button("Review public binding reconciliation"){approvedPublicBinding=binding;confirmPublicBinding=true}.buttonStyle(.bordered)
                            .disabled(model.busy||snapshotMode)
                    }
                }
                if let unreadable=model.unreadableReceipt {
                    Text("Retained unreadable public receipt · \(unreadable.bytes) bytes · SHA256 of first\(unreadable.prefixBytes) bytes: \(unreadable.prefixSHA256) · past execution unknown")
                        .sirsiFont(.caption).foregroundStyle(.secondary).textSelection(.enabled)
                }
                if let operation=model.outstanding {
                    Text("Outstanding \(operation.operation) · \(operation.ticket?.id ?? "unreadable") · generation \(operation.ticket?.generation ?? 0)").sirsiFont(.caption).textSelection(.enabled)
                    Button("Check result and prevent delayed submission"){_ = model.resolveOutstanding(operation)}.buttonStyle(.bordered).disabled(model.busy||snapshotMode)
                }
                if let previous=model.lastResolved {
                    DisclosureGroup("Retained previous operation receipt"){
                        Text("\(previous.operation) · \(previous.ticket?.id ?? "unavailable")\n\(previous.receipt?.issue ?? "Matching terminal result or verified fence recorded; inspect current transaction separately.")").sirsiFont(.caption).textSelection(.enabled)
                    }
                }
            }.padding(.top,8)
        }
        .fileImporter(isPresented:$picker,allowedContentTypes:[.item],allowsMultipleSelection:false){result in
            let restoring=pickingRestore;pickingRestore=false
            if case .success(let files)=result,let file=files.first {
                if restoring,let transaction=model.transaction {Task{await model.reviewRestore(artifact:file,expected:transaction)}}
                else {model.select(file)}
            }
        }
        .confirmationDialog("Restore this exact approved replacement?",isPresented:$confirmRestore,titleVisibility:.visible){
            Button("Restore reviewed replacement"){if let approvedRestore{_ = model.restore(approvedRestore)};approvedRestore=nil}
            Button("Cancel",role:.cancel){approvedRestore=nil}
        }message:{Text("Transaction: \(approvedRestore?.transaction.id ?? "unavailable")\nArtifact: \(approvedRestore?.artifact.path ?? "unavailable")\nRestoration: \(approvedRestore?.digest ?? "unavailable")\nNamed backup and stage debt is retained. Service activation is a separate explicit action.")}
        .confirmationDialog("Open normal signed Installer?",isPresented:$confirmInstaller,titleVisibility:.visible){
            Button("Open verified Installer"){if let approvedInstaller{_ = model.install(approvedInstaller)};approvedInstaller=nil}
            Button("Cancel",role:.cancel){approvedInstaller=nil}
        }message:{Text("Release: \(approvedInstaller?.release ?? "unavailable")\nPackage: \(approvedInstaller?.package ?? "unavailable")\nSHA256: \(approvedInstaller?.digest ?? "unavailable")\nPantheon rechecks the exact package before opening macOS Installer. Installation and a fresh installed-pair check remain separate; no service is restarted.")}
        .confirmationDialog("Restore this exact Pantheon release?",isPresented:$confirmRollback,titleVisibility:.visible){
            Button("Confirm exact app rollback",role:.destructive){if let approvedRollback{Task{await model.rollback(to:approvedRollback)}};approvedRollback=nil}
            Button("Cancel",role:.cancel){approvedRollback=nil}
        }message:{Text("Selected signed release: \(approvedRollback?.release ?? "unavailable")\nThe existing whole-app owner rechecks the installed app, live schema, official PKG identity and dependencies; it retains a Ma'at receipt and attempt before one normal Installer handoff. Exact installed app/helper readback is required. An ambiguous handoff is never replayed automatically.")}
        .confirmationDialog("Reconcile this public operation binding?",isPresented:$confirmPublicBinding,titleVisibility:.visible){
            Button("Confirm public binding reconciliation"){if let approvedPublicBinding{_ = model.rebindPublicState(approvedPublicBinding)};approvedPublicBinding=nil}
            Button("Cancel",role:.cancel){approvedPublicBinding=nil}
        }message:{Text("\(publicBindingSummary)\nThe controller is inspected again. An existing exact ticket is recovered, or a fresh serialized preview reservation is issued for the selected retained payload. The public binding is persisted and read back; then matching result/fence resolution is required before a new service preview and consent. No service operation is replayed and no empty status proves that old work never ran.")}
        .confirmationDialog("Interrupt this exact legacy root relay?",isPresented:$confirming,titleVisibility:.visible){
            Button("Confirm maintenance",role:.destructive){if let approved{_ = model.confirm(approved)};approved=nil}
            Button("Cancel",role:.cancel){approved=nil;model.cancelReview()}
        }message:{Text("Only the reviewed root relay is selected. Old remote outcomes may remain unknown; retained envelopes will not be replayed. No model or unrelated service is selected.\nPID: \(approved?.plan.original.pid ?? 0)\nPlan: \(approved?.plan.sha256 ?? "No reviewed plan")")}
        .confirmationDialog("Resolve this exact retained transaction?",isPresented:$confirmRecovery,titleVisibility:.visible){
            Button(selectedAction == "abort" ? "Abort preparation" : "Resume activation"){if let selectedTransaction{if selectedAction == "abort"{_ = model.abortBeforeRetirement(selectedTransaction)}else{_ = model.resume(selectedTransaction)}};selectedTransaction=nil}
            Button("Cancel",role:.cancel){selectedTransaction=nil}
        }message:{Text("Transaction: \(selectedTransaction?.id ?? "unavailable")\nPlan: \(selectedTransaction?.plan.sha256 ?? "unavailable")\nAbort only restores scoped admission while the original is verified alive. Resume never certifies old remote outcomes or replays retained envelopes.")}
        .onChange(of:directory){value in model.changeDirectory(value)}
        .onAppear{model.changeDirectory(directory)}
        .onDisappear{approved=nil;approvedRestore=nil;approvedPublicBinding=nil;approvedRollback=nil;confirmPublicBinding=false;confirmRollback=false}
    }
    private var publicBindingSummary:String {
        guard let approvedPublicBinding else{return "No reviewed public binding"}
        if let raw=approvedPublicBinding.ticketJSON,let data=raw.data(using:.utf8),
           let ticket=try?JSONDecoder().decode(LegacyRelayAppTicket.self,from:data){
            return "Target: system/ai.sirsi.router.relay\nOperation: \(ticket.operation) · \(ticket.id)\nGeneration: \(ticket.generation)\nPlan: \(ticket.plan.sha256)"
        }
        return "Target: system/ai.sirsi.router.relay\nFresh reservation for retained payload: \(approvedPublicBinding.file?.path ?? "unavailable")"
    }
    private func select(_ action:String,_ transaction:LegacyRelayTransaction){
        Task{
            await model.reviewAction(action,expected:transaction)
            if model.actionApproval?.operation==action,model.actionApproval?.transaction==transaction{
                selectedAction=action;selectedTransaction=transaction;confirmRecovery=true
            }
        }
    }
}
