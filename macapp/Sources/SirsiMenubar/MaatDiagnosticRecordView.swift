import SwiftUI

// Mounted at the ordinary workspace root: no MaatCase or successful load is
// required to observe, initialize genuine first-use memory, or inspect debt.
struct MaatDiagnosticRecordView: View {
    @ObservedObject var model: MaatDiagnosticRecordModel
    let refresh: () async -> Void
    @State private var approval: MaatDiagnosticRecordModel.Approval?
    @State private var confirming = false
    let observeTitle = "Observe locally"
    let inspectTitle = "Inspect record readiness"

    var completedDetails: [String] { (model.completed?.details ?? []) + (model.requestCompletion?.details ?? []) }

    func inspect() async { await model.inspect() }
    func observe() async { await model.observe() }
    private func review(_ action: MaatDiagnosticRecordModel.Action) {
        approval = model.approval(action); confirming = approval != nil
    }
    func confirmed(_ value: MaatDiagnosticRecordModel.Approval) async {
        if await model.confirm(value) {
            let recorded = model.lastOperationRecorded
            await model.inspect()
            if recorded { await refresh() }
        }
    }
    var body: some View {
        DisclosureGroup("Local observation and record recovery") {
            VStack(alignment: .leading, spacing: 8) {
                Text("Observe without recording, or inspect local memory before a separately confirmed record. No service repair or software installation is performed.")
                    .sirsiFont(.caption).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                HStack {
                    Button(observeTitle) { Task { await observe() } }.buttonStyle(.bordered)
                    Button(inspectTitle) { Task { await inspect() } }.buttonStyle(.bordered)
                }.disabled(model.busy)
                if let value = model.observation {
                    Text("Observed gate: \(value.gate) · \(model.recorded == nil ? "not a verified record" : "record verified")")
                        .sirsiFont(.caption, weight: .semibold)
                    ForEach(maatSystemOneDisplayFindings(value.findings)) { finding in
                        Text("\(finding.severity): \(finding.claim)").sirsiFont(.caption)
                    }
                }
                if !completedDetails.isEmpty {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Last verified completed operation — historical result")
                            .sirsiFont(.caption, weight: .semibold)
                        ForEach(Array(completedDetails.enumerated()), id: \.offset) { _, line in
                            Text(line).sirsiFont(.caption).textSelection(.enabled)
                        }
                    }
                }
                if let message = model.message {
                    Text(message).sirsiFont(.caption).textSelection(.enabled)
                }
                if model.approval(.initialize) != nil {
                    Button("Review first-use initialization") { review(.initialize) }.buttonStyle(.bordered)
                }
                if model.approval(.record) != nil {
                    Button("Review observe and record") { review(.record) }.buttonStyle(.bordered)
                }
                if let id = model.activeRequestID {
                    Text("Retained request: \(id) · resolve first; never replay").textSelection(.enabled)
                    Button("Read exact retained request") { Task { if await model.readRequest(id) { await refresh() } } }.disabled(model.busy)
                    Button("Review exact request resolution") { review(.resolveRequest(id)) }.disabled(model.busy)
                }
                if model.discoveryCursor != nil && !model.discoveryRestartRequired {
                    Button("Continue discovery — same snapshot") { Task { await model.continueDiscovery() } }.disabled(model.busy)
                }
                if model.discoveryRestartRequired {
                    Button("Restart discovery") { Task { await inspect() } }.disabled(model.busy)
                }
                Text(model.discoveryComplete ? "Discovery complete" : "Discovery incomplete — not proof of empty history")
                    .sirsiFont(.caption)
                ForEach(model.requests) { request in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Request \(request.id) · \(request.state)").textSelection(.enabled)
                        if let result = request.result {
                            Text("Record \(result.id) · intent \(result.intent_sha256) · decision \(result.decision_evidence) · verdict \(result.verdict_sha256)").textSelection(.enabled)
                        }
                        Button("Read exact request") { Task { if await model.readRequest(request.id) { await refresh() } } }.disabled(model.busy)
                        if !request.terminal && request.id != model.activeRequestID {
                            Button("Review exact request resolution") { review(.resolveRequest(request.id)) }.disabled(model.busy)
                        }
                    }
                }
                ForEach(model.records) { record in
                    Text("Historical record \(record.id) · \(record.state)").sirsiFont(.caption).textSelection(.enabled)
                }
                ForEach(model.pending) { pending in
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Retained record \(pending.id)\nStore \(pending.store) · observed \(pending.observed)")
                            .sirsiFont(.caption).textSelection(.enabled)
                        Button("Review exact delivery reconciliation") { review(.reconcile(pending.id)) }
                            .buttonStyle(.bordered).disabled(model.busy)
                    }
                }
                if model.busy {
                    HStack {
                        ProgressView("Reading local diagnostic custody…")
                        Button("Cancel") { model.cancel() }.buttonStyle(.bordered)
                    }
                }
            }.padding(.top, 8)
        }
        .sirsiFont(.caption, weight: .semibold)
        .padding(.horizontal, 16).padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .confirmationDialog("Confirm this local memory operation?", isPresented: $confirming, titleVisibility: .visible) {
            Button("Confirm reviewed operation") {
                let captured = approval; approval = nil
                if let captured { Task { await confirmed(captured) } }
            }
            Button("Cancel", role: .cancel) { approval = nil }
        } message: { Text(confirmationText) }
    }
    private var confirmationText: String {
        guard let approval else { return "No operation is selected." }
        switch approval.action {
        case .initialize:
            return "Create private local failure memory only if it is still genuinely uninitialized. No incidents are imported. Initialization does not record an observation; inspect again and give fresh consent afterward."
        case .record:
            return "Run a new local health observation, then its scoped failure-memory preflight, retained receipt and exact journal readback. This does not record an older displayed observation, perform a repair or authorize other work. Unknown delivery will not be resent."
        case .resolveRequest(let id):
            return "Fence and resolve exact request \(id) under its original actor/store. Read retained intent/result and exact journal only; never diagnose, append or replay. Missing or changed custody remains debt."
        case .reconcile(let id):
            return "Read exact retained record \(id) and journal evidence. Complete only its data receipt if verified. No diagnostic, append, reset, repair or replay is requested. Changed custody requires a fresh review."
        }
    }
}
