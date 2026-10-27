import SwiftUI

// The ordinary workspace root owns this frame, before any Casebook branch.
// Reading memory needs neither a retained case nor a successful history load.
struct MaatWorkspaceRecoveryFrame: View {
    @ObservedObject var recovery: MaatPreparedPreflightRecovery
    @State private var diagnosticExpanded=false
    let content: AnyView
    var engine:SirsiEngine? = nil
    var diagnostic: MaatDiagnosticRecordModel? = nil
    var diagnosticRefresh: () async -> Void = {}

    var localDiagnostics: MaatDiagnosticRecordView? {
        diagnostic.map { MaatDiagnosticRecordView(model: $0, refresh: diagnosticRefresh,
            expanded: $diagnosticExpanded, initializationRefresh: { await recovery.inspect() }) }
    }

    var localMemory: MaatWorkspaceMemoryEntry {
        MaatWorkspaceMemoryEntry(model: recovery, engine:engine, diagnostic: diagnostic,
            reviewFirstUse: { await reviewFirstUseInitialization() })
    }

    func reviewFirstUseInitialization() async {
        guard recovery.canReviewFirstUseInitialization, let diagnostic else{return}
        diagnosticExpanded=true
        await diagnostic.inspect()
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Ma'at")
            localMemory
            if let localDiagnostics { localDiagnostics }
            content
        }
    }
}

struct MaatWorkspaceMemoryEntry: View {
    @ObservedObject var model: MaatPreparedPreflightRecovery
    var engine:SirsiEngine? = nil
    var diagnostic:MaatDiagnosticRecordModel? = nil
    var reviewFirstUse:()async->Void = {}
    let inspectTitle = "Inspect local memory"

    func inspect() async {
        await model.inspect()
    }

    var recoveryView: MaatPreparedPreflightRecoveryView {
        MaatPreparedPreflightRecoveryView(model: model, refresh: { await inspect() })
    }

    var journalIntegrityCard:MaatJournalIntegrityCard? {
        guard let engine,let integrity=model.inspection?.journalIntegrity,integrity.invalidCount>0 else{return nil}
        return MaatJournalIntegrityCard(engine:engine,integrity:integrity,repairCompleted:{await inspect()})
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Label("Local memory recovery", systemImage: "externaldrive.badge.shield.checkmark")
                    .sirsiFont(.caption, weight: .semibold)
                Spacer()
                Button(inspectTitle) { Task { await inspect() } }
                    .buttonStyle(.bordered)
                    .disabled(model.busy)
            }
            Text(model.summary).sirsiFont(.caption).foregroundStyle(.secondary).fixedSize(horizontal:false,vertical:true)
            if let journalIntegrityCard { journalIntegrityCard }
            if model.canReviewFirstUseInitialization,diagnostic != nil {
                Button("Review first-use initialization") { Task{await reviewFirstUse()} }
                    .buttonStyle(.bordered).disabled(model.busy || diagnostic?.busy == true)
            }
            recoveryView
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.panelRaised)
    }
}
