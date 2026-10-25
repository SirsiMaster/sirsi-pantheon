import SwiftUI
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

    init(directory:URL,model:LegacyRelayMaintenanceModel){self.directory=directory;self.model=model}

    var body: some View {
        DisclosureGroup("Legacy root relay maintenance") {
            VStack(alignment:.leading,spacing:10){
                Text("Optional fallback for a recognized legacy root relay. Pantheon stays unprivileged; macOS asks permission for only the reviewed operation using the protected installed engine. Producer-covered maintenance remains separate.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                HStack {
                    Button("Choose retained legacy payload"){picker=true}.buttonStyle(.bordered)
                    Button("Review"){Task{await model.review()}}.buttonStyle(.bordered).disabled(model.selectedFile == nil||model.outcomeUnknown)
                    Button("Read current transaction"){Task{await model.readStatus()}}.buttonStyle(.bordered)
                }.disabled(model.busy||snapshotMode)
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
                    }.disabled(model.busy||snapshotMode)
                    if let restore=model.restoreApproval {
                        Text("Only the originally approved signed replacement will be restored. No legacy code or retained envelope will be replayed.").sirsiFont(.caption).foregroundStyle(.secondary)
                        Button("Confirm reviewed replacement restore"){approvedRestore=restore;confirmRestore=true}.buttonStyle(.bordered).disabled(model.busy||snapshotMode)
                    }
                }
                if model.busy{HStack{ProgressView().controlSize(.small);Text("Inspecting selected maintenance state…").sirsiFont(.caption);Button("Cancel request"){model.cancelInvocation()}.buttonStyle(.bordered)}}
                if let error=model.error{Text(error).sirsiFont(.caption).foregroundStyle(.secondary).textSelection(.enabled)}
                if model.outcomeUnknown { Text("The previous operation may still have started. Read its transaction before another confirmation; cancellation did not prove rollback.").sirsiFont(.caption).foregroundStyle(.secondary) }
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
        .onDisappear{approved=nil;approvedRestore=nil}
    }
    private func select(_ action:String,_ transaction:LegacyRelayTransaction){selectedAction=action;selectedTransaction=transaction;confirmRecovery=true}
}
