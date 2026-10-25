import SwiftUI

// The ordinary workspace root owns this frame, before any Casebook branch.
// Reading memory needs neither a retained case nor a successful history load.
struct MaatWorkspaceRecoveryFrame: View {
    @ObservedObject var recovery: MaatPreparedPreflightRecovery
    let content: AnyView
    var diagnostic: MaatDiagnosticRecordModel? = nil

    var localDiagnostics: MaatDiagnosticRecordView? {
        diagnostic.map { MaatDiagnosticRecordView(model: $0, refresh: {}) }
    }

    var localMemory: MaatWorkspaceMemoryEntry {
        MaatWorkspaceMemoryEntry(model: recovery)
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
    let inspectTitle = "Inspect local memory"

    func inspect() async {
        await model.inspect()
    }

    var recoveryView: MaatPreparedPreflightRecoveryView {
        MaatPreparedPreflightRecoveryView(model: model, refresh: { await inspect() })
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
            recoveryView
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.panelRaised)
    }
}
