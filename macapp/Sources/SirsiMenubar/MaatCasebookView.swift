import SwiftUI
import UniformTypeIdentifiers

// MaatWorkspaceView joins decision quality and retained local knowledge under
// one operator authority. Seshat's ingestion compatibility commands are not a
// second Pantheon surface: Ma'at is where a person inspects the resulting
// evidence, classifications, and decisions.
@MainActor struct MaatWorkspaceView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
    @State private var section: MaatWorkspaceSection = .systemOne
    @StateObject private var systemOneHistory: MaatHistoryRefreshStore
    @StateObject private var decisionHistory: MaatHistoryRefreshStore
    let preloadedCasebook: MaatCasebookProjection?
    let preloadedKnowledge: MaatKnowledgeProjection?
    let opensReleasePreflight: Bool
    let apolloContext: ApolloPlan?
    let relayOutboxContext: RelayOutboxResolutionContext?

    init(engine: SirsiEngine, preloadedCasebook: MaatCasebookProjection? = nil,
         preloadedKnowledge: MaatKnowledgeProjection? = nil,
         opensReleasePreflight: Bool = false, apolloContext: ApolloPlan? = nil,
         relayOutboxContext: RelayOutboxResolutionContext? = nil,
         systemOneHistory: MaatHistoryRefreshStore? = nil,
         decisionHistory: MaatHistoryRefreshStore? = nil) {
        self.engine = engine
        _systemOneHistory = StateObject(wrappedValue: systemOneHistory ?? MaatHistoryRefreshStore(projection: preloadedCasebook))
        _decisionHistory = StateObject(wrappedValue: decisionHistory ?? MaatHistoryRefreshStore(projection: preloadedCasebook))
        self.preloadedCasebook = preloadedCasebook
        self.preloadedKnowledge = preloadedKnowledge
        self.opensReleasePreflight = opensReleasePreflight
        self.apolloContext = apolloContext
        self.relayOutboxContext = relayOutboxContext
    }

    var body: some View {
        // Bind the ACTUAL query owners without a new model or remount reset.
        // Capturing stores directly avoids an engine -> callback -> engine cycle.
        let system = systemOneHistory, decisions = decisionHistory
        engine.maatCaseOperations.diagnosticRecord.bindQueryOwnedHistoryRefresh {
            await system.refresh(scope: "system-one")
            await decisions.refresh(scope: "all")
        }
        return MaatWorkspaceRecoveryFrame(
            recovery: engine.maatCaseOperations.preparedPreflightRecovery,
            content: AnyView(workspaceContent),
            engine: engine,
            diagnostic: engine.maatCaseOperations.diagnosticRecord,
            diagnosticRefresh: refreshDiagnosticHistory)
    }

    @MainActor private func refreshDiagnosticHistory() async {
        await systemOneHistory.refresh(scope: "system-one")
        await decisionHistory.refresh(scope: "all")
    }

    private var workspaceContent: some View {
        VStack(spacing: 0) {
            if let context = relayOutboxContext {
                RelayOutboxResolutionView(engine: engine, context: context)
            } else {
                if let plan = apolloContext {
                    HStack(alignment: .top, spacing: 12) {
                        Image(systemName: "cpu").foregroundStyle(gold)
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Reviewing an Apollo plan").sirsiFont(.subheadline, weight: .semibold)
                            Text("\(plan.machineID) · \(plan.engineID) · \(plan.cpuCores) cores · \(SirsiEngine.human(plan.memoryBytes)) memory · \(SirsiEngine.human(plan.swapBytes)) swap ceiling")
                                .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                                .textSelection(.enabled)
                            Text("Estates: \(plan.chipEstates.joined(separator: ", "))")
                                .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                        }
                        Spacer()
                    }
                    .padding(16).frame(maxWidth: .infinity, alignment: .leading)
                    .background(PantheonTheme.panelRaised)
                }
                Group {
                    if snapshotMode {
                        HStack(spacing: 6) {
                            ForEach(MaatWorkspaceSection.allCases) { option in
                                Label(option.title, systemImage: option.symbol)
                                    .sirsiFont(.caption, weight: .semibold)
                                    .foregroundStyle(option == section ? Color.black : Color.primary)
                                    .frame(maxWidth: .infinity)
                                    .padding(.vertical, 7)
                                    .background(RoundedRectangle(cornerRadius: 7).fill(option == section ? gold : Color.primary.opacity(0.07)))
                            }
                        }
                        .accessibilityElement(children: .combine)
                        .accessibilityLabel("Ma'at workspace: \(section.title)")
                    } else {
                        Picker("Ma'at workspace", selection: $section) {
                            ForEach(MaatWorkspaceSection.allCases) { option in
                                Label(option.title, systemImage: option.symbol).tag(option)
                            }
                        }
                        .pickerStyle(.segmented)
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)

                switch section {
                case .systemOne:
                    MaatSystemOneView(engine: engine, section: $section, preloaded: preloadedCasebook,
                                      opensReleasePreflight: opensReleasePreflight,
                                      historyStore: systemOneHistory, diagnosticRefresh: refreshDiagnosticHistory)
                case .decisions:
                    MaatCasebookView(engine: engine, preloaded: preloadedCasebook, showsBackBar: false, historyStore: decisionHistory)
                case .knowledge:
                    MaatKnowledgeView(engine: engine, preloaded: preloadedKnowledge, showsBackBar: false)
                case .proposals:
                    MaatKnownFailureProposalsView(engine: engine, showsBackBar: false)
                }
            }
        }
    }
}

private enum MaatWorkspaceSection: String, CaseIterable, Identifiable {
    case systemOne, decisions, knowledge, proposals

    var id: String { rawValue }
    var title: String {
        switch self {
        case .systemOne: return "System One"
        case .decisions: return "Decisions"
        case .knowledge: return "Knowledge"
        case .proposals: return "Proposals"
        }
    }
    var symbol: String {
        switch self {
        case .systemOne: return "scalemass"
        case .decisions: return "checkmark.seal"
        case .knowledge: return "books.vertical"
        case .proposals: return "lightbulb.max"
        }
    }
}

// MaatSystemOneView makes the local JEV-like system visible as an operational
// surface, not a hidden field in an individual case. It projects only the
// canonical Casebook: no ambient state is classified here and no local screen
// becomes mutation or release authority merely because it is rendered.
@MainActor private struct MaatSystemOneView: View {
    @ObservedObject var engine: SirsiEngine
    @Binding var section: MaatWorkspaceSection
    @Environment(\.snapshotMode) private var snapshotMode
    @StateObject private var historyRefresh: MaatHistoryRefreshStore
    private var casebook: MaatCasebookProjection? { historyRefresh.state.projection }
    private var loading: Bool { historyRefresh.state.loading }
    private var loadError: String? { historyRefresh.state.error }
    @State private var showScreenPicker = false
    @StateObject private var sourceImport = MaatSourceImportModel()
    @State private var confirmScreenImport = false
    @State private var releasePreflight: MaatReleaseContractPreflight?
    @State private var releasePreflightInFlight = false
    @State private var releasePreflightError: String?
    @State private var confirmReleasePreflight = false
    @State private var releasePreflightRecorded = false
    @State private var credentialPreflight: MaatReleaseCredentialPreflight?
    @State private var credentialPreflightInFlight = false
    @State private var credentialPreflightError: String?
    @State private var confirmCredentialPreflight = false
    @State private var credentialPreflightRecorded = false
    @State private var didOpenReleasePreflight = false
    let opensReleasePreflight: Bool
    let diagnosticRefresh: (() async -> Void)?

    init(engine: SirsiEngine, section: Binding<MaatWorkspaceSection>, preloaded: MaatCasebookProjection? = nil,
         opensReleasePreflight: Bool = false, historyStore: MaatHistoryRefreshStore? = nil,
         diagnosticRefresh: (() async -> Void)? = nil) {
        self.engine = engine
        _section = section
        _historyRefresh = StateObject(wrappedValue: historyStore ?? MaatHistoryRefreshStore(projection: preloaded))
        self.opensReleasePreflight = opensReleasePreflight
        self.diagnosticRefresh = diagnosticRefresh
    }

    var body: some View {
        Group {
            if let casebook {
                systemOneBody(casebook)
            } else if loading {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("Searching retained history…")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(.secondary)
                    Button("Cancel") { historyRefresh.cancel() }
                        .buttonStyle(.bordered)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let loadError {
                MaybeScroll {
                    VStack(spacing: 12) {
                        unavailableState(loadError)
                        screenImportControl
                        MaatLifecycleControl(engine: engine)
                        LegacyRelayMaintenancePanel(directory: URL(fileURLWithPath: NSHomeDirectory()),appUpdate:engine.maatCaseOperations.appUpdate)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                MaybeScroll {
                    unavailableState("Pantheon did not receive a Ma'at Casebook projection. No System One result was inferred.")
                }
            }
        }
        .task {
            guard casebook == nil else { return }
            await load()
        }
        .fileImporter(isPresented: $showScreenPicker, allowedContentTypes: [.json], allowsMultipleSelection: false) { result in
            switch result {
            case .success(let urls):
                guard let url = urls.first else { return }
                sourceImport.select(url)
            case .failure(let error):
                sourceImport.selectionFailed("Pantheon did not open the selected evidence file: \(error.localizedDescription). Choose another JSON file.")
            }
        }
        .confirmationDialog("Record this System One screen?", isPresented: $confirmScreenImport, titleVisibility: .visible) {
            Button("Validate and record") { Task { await importScreen() } }
            Button("Cancel", role: .cancel) { sourceImport.cancelReview() }
        } message: {
            Text("Ma'at will revalidate the reviewed v2 JSON bytes and expected head before recording one nonbinding screen. It will not execute the assessed payload, authorize work, or treat this as a release decision.\nFile: \(sourceImport.approval?.file.path ?? "No reviewed file")\nExpected head: \(sourceImport.approval?.expectedHead ?? "No reviewed head")")
        }
        .confirmationDialog("Record this release-contract preflight?", isPresented: $confirmReleasePreflight, titleVisibility: .visible) {
            Button("Record in Ma'at Casebook") { Task { await recordReleasePreflight() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This records the exact non-executing source observation in the local Casebook. It does not build, package, sign, notarize, publish, or authorize a release.")
        }
        .confirmationDialog("Record this release credential readiness check?", isPresented: $confirmCredentialPreflight, titleVisibility: .visible) {
            Button("Record in Ma'at Casebook") { Task { await recordCredentialPreflight() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This retains public certificate-name and fingerprint evidence only. It does not read private keys, passwords, notarization material, or contact Apple.")
        }
        .onDisappear {
            historyRefresh.cancel()
            sourceImport.cancelReview()
        }
    }

    private func unavailableState(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(.orange)
            Text("System One evidence is unavailable")
                .sirsiFont(.headline)
            Text(message)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            SnapshotActionButton {
                Task { await load() }
            } label: {
                Label("Read retained evidence again", systemImage: "arrow.clockwise")
            }
            SnapshotActionButton {
                section = .decisions
            } label: {
                Label("Open decisions", systemImage: "checkmark.seal")
            }
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab authority", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
            NavLink { ResultView(engine: engine, title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                Label("Run a fresh Ma'at audit", systemImage: "checkmark.seal")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(20)
    }

    private func systemOneBody(_ casebook: MaatCasebookProjection) -> some View {
        let screens = casebook.cases.filter { $0.systemOne != nil }
        let calibrations = casebook.cases.filter { $0.systemOneCalibration != nil }
        return ScrollViewReader { proxy in
            VStack(spacing: 0) {
                MaybeScroll {
                    VStack(alignment: .leading, spacing: 16) {
                        summary(screens: screens, calibrations: calibrations, history: casebook.history)
                        if loading {
                            HStack {
                                Label("Searching retained history…", systemImage: "arrow.clockwise")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.secondary)
                                Spacer()
                                Button("Cancel") { historyRefresh.cancel() }
                                    .buttonStyle(.borderless)
                            }
                        }
                        if casebook.journalIntegrity.invalidCount > 0 {
                            MaatJournalIntegrityCard(engine: engine, integrity: casebook.journalIntegrity)
                        }
                        if let loadError {
                            VStack(alignment: .leading, spacing: 8) {
                                Label(loadError, systemImage: "exclamationmark.triangle.fill")
                                    .sirsiFont(.subheadline)
                                    .foregroundStyle(.orange)
                                if historyRefresh.state.restartRequired {
                                    Button("Restart history from the latest snapshot") { Task { await load() } }
                                        .buttonStyle(.borderedProminent)
                                        .disabled(loading)
                                }
                            }
                        }
                        resolutionLane(screens)
                        hostTriageControl
                        releasePreflightControl
                            .id("maat-release-preflight")
                        screenImportControl
                        MaatLifecycleControl(engine: engine)
                        LegacyRelayMaintenancePanel(directory: URL(fileURLWithPath: NSHomeDirectory()),appUpdate:engine.maatCaseOperations.appUpdate)
                        if screens.isEmpty && calibrations.isEmpty {
                            historyEmptyState(casebook)
                        } else {
                            if !screens.isEmpty { screenList(screens) }
                            if !calibrations.isEmpty { calibrationList(calibrations) }
                        }
                    }
                    .padding(16)
                }
                Divider()
                HStack {
                    Text("Retained local evidence · screens never grant execution authority")
                        .sirsiFont(.caption)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Button { Task { await load() } } label: {
                        Label("Refresh", systemImage: "arrow.clockwise")
                    }
                    .buttonStyle(.borderless)
                    .disabled(loading)
                    if let history = casebook.history, let cursor = history.nextCursor {
                        Button { Task { await load(cursor: cursor) } } label: {
                            Label(history.coverage == "partial" ? "Continue history search" : "Load older", systemImage: "clock.arrow.circlepath")
                        }
                        .buttonStyle(.borderless)
                        .disabled(loading || historyRefresh.state.restartRequired)
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 11)
            }
            .onAppear {
                guard opensReleasePreflight, !snapshotMode, !didOpenReleasePreflight else { return }
                didOpenReleasePreflight = true
                DispatchQueue.main.async {
                    withAnimation(.easeOut(duration: 0.2)) {
                        proxy.scrollTo("maat-release-preflight", anchor: .top)
                    }
                }
            }
        }
    }

    @ViewBuilder private func historyEmptyState(_ casebook: MaatCasebookProjection) -> some View {
        let history = casebook.history
        if casebook.historyEmptyState == .incomplete {
            VStack(alignment: .leading, spacing: 8) {
                Label("History search is incomplete", systemImage: "clock")
                    .sirsiFont(.headline)
                Text("No empty-history conclusion is available until this snapshot finishes scanning.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                if let cursor = history?.nextCursor {
                    Button("Continue history search") { Task { await load(cursor: cursor) } }
                        .buttonStyle(.borderedProminent)
                        .disabled(loading || historyRefresh.state.restartRequired)
                }
            }
        } else if casebook.historyEmptyState == .olderPage {
            VStack(alignment: .leading, spacing: 8) {
                Label("No System One evidence on this page", systemImage: "clock.arrow.circlepath")
                    .sirsiFont(.headline)
                Text("Older retained history is available; no result was inferred from this page alone.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                if let cursor = history?.nextCursor {
                    Button("Load older System One history") { Task { await load(cursor: cursor) } }
                        .buttonStyle(.borderedProminent)
                        .disabled(loading || historyRefresh.state.restartRequired)
                }
            }
        } else if casebook.historyEmptyState == .invalidHistory {
            VStack(alignment: .leading, spacing: 8) {
                Label("No valid System One record was found", systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.headline)
                Text("The complete scan found invalid journal rows, so it cannot establish that the retained history contains no System One evidence.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
            }
        } else if casebook.historyEmptyState == .afterCursor {
            VStack(alignment: .leading, spacing: 8) {
                Label("No System One evidence remains after this cursor", systemImage: "clock.arrow.circlepath")
                    .sirsiFont(.headline)
                Text("This page ends a continued history query; it does not say that the full retained history is empty.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                Button("Refresh full history") { Task { await load() } }
                    .buttonStyle(.borderedProminent)
            }
        } else if casebook.historyEmptyState == .empty {
            emptyState
        } else {
            unavailableState("History coverage is unknown, so Ma'at cannot say that no System One evidence exists.")
        }
    }

    private func summary(screens: [MaatCase], calibrations: [MaatCase], history: MaatHistoryWindow?) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Ma'at System One")
                .sirsiFont(.title3, weight: .bold)
            Text("Local deterministic screens remain model-free. Legacy comparison rows are declared history, not verified source-profile qualification.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if let history, history.coverage == "partial" {
                Text(history.countScope == "after-cursor-scanned-so-far"
                    ? "\(history.candidateMatches) matches scanned after this cursor · remaining history is incomplete."
                    : "\(history.candidateMatches) matches scanned so far · history is incomplete.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                if let progress = history.boundedProgressText {
                    Text(progress).sirsiFont(.caption).foregroundStyle(.secondary)
                }
            } else if let history, history.countScope == "after-cursor" {
                Text("This page shows matches after its cursor; whole-query totals are unknown.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            } else if let history, history.hasMore, history.candidateMatchesComplete {
                Text("Showing this page of \(history.candidateMatches) matching System One records.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            } else if let history, !history.totalComplete {
                Text("Loaded page · invalid history prevents complete totals.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            }
            systemMetrics(screens: screens, calibrations: calibrations)
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    @ViewBuilder private func systemMetrics(screens: [MaatCase], calibrations: [MaatCase]) -> some View {
        let screensMetric = systemMetric("Screens", screens.count, .secondary)
        let changesMetric = systemMetric("Changes", screens.filter { $0.systemOne?.gate == "changes" }.count, .orange)
        let blockedMetric = systemMetric("Blocked", screens.filter { $0.systemOne?.gate == "block" }.count, .red)
        let escalatedMetric = systemMetric("Escalated", screens.filter { $0.systemOne?.gate == "escalate" }.count, gold)
        let legacyComparisonsMetric = systemMetric("Legacy comparisons", calibrations.count, .secondary)

        // A single metric row becomes unreadable at accessibility text sizes.
        // Keep the same facts, but let SwiftUI select a two-line layout when
        // the operator's window or text size needs it.
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 8) {
                screensMetric
                changesMetric
                blockedMetric
                escalatedMetric
                legacyComparisonsMetric
            }
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    screensMetric
                    changesMetric
                    blockedMetric
                }
                HStack(spacing: 8) {
                    escalatedMetric
                    legacyComparisonsMetric
                }
            }
        }
    }

    private func systemMetric(_ title: String, _ value: Int, _ tint: Color) -> some View {
        Text("\(value) \(title)")
            .sirsiFont(.caption, weight: .semibold)
            .padding(.horizontal, 8)
            .padding(.vertical, 5)
            .background(Capsule().fill(tint.opacity(0.12)))
            .foregroundStyle(tint)
    }

    // System One must lead people to the next accountable step, rather than
    // presenting an undifferentiated archive of red and amber rows. This is a
    // navigation aid only: the detail view preserves the exact evidence and
    // owns any confirmation-gated owner review or acceptance.
    @ViewBuilder private func resolutionLane(_ screens: [MaatCase]) -> some View {
        let unresolved = screens.filter {
            $0.status != "resolved" && ["changes", "block", "escalate"].contains($0.systemOne?.gate ?? "")
        }.sorted(by: { (lhs: MaatCase, rhs: MaatCase) -> Bool in
            let left = gateRank(lhs.systemOne?.gate ?? "")
            let right = gateRank(rhs.systemOne?.gate ?? "")
            if left != right { return left < right }
            return lhs.priority.rank > rhs.priority.rank
        })

        if let next = unresolved.first, let verdict = next.systemOne {
            VStack(alignment: .leading, spacing: 8) {
                Label(resolutionTitle(for: verdict.gate), systemImage: resolutionSymbol(for: verdict.gate))
                    .sirsiFont(.headline)
                    .foregroundStyle(gateTint(verdict.gate))
                Text(resolutionDetail(for: next, gate: verdict.gate))
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                NavLink { MaatCaseDetailView(engine: engine, operations: engine.maatCaseOperations, entry: next) } label: {
                    Label("Open next resolution", systemImage: "arrow.right.circle.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(gold)
                .accessibilityHint("Shows the retained evidence and the exact confirmation-gated resolution path.")
            }
            .padding(14)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 10).fill(gateTint(verdict.gate).opacity(0.10)))
        } else if !screens.isEmpty {
            Label("No unresolved System One action is waiting", systemImage: "checkmark.seal.fill")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(.green)
                .padding(12)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 10).fill(Color.green.opacity(0.08)))
        }
    }

    private func gateRank(_ gate: String) -> Int {
        switch gate {
        case "escalate": return 0
        case "block": return 1
        case "changes": return 2
        default: return 3
        }
    }

    private func resolutionTitle(for gate: String) -> String {
        switch gate {
        case "escalate": return "Independent review is required"
        case "block": return "A blocking decision needs resolution"
        default: return "Changes need an accountable decision"
        }
    }

    private func resolutionSymbol(for gate: String) -> String {
        switch gate {
        case "escalate": return "arrow.triangle.branch"
        case "block": return "hand.raised.fill"
        default: return "checkmark.circle.badge.questionmark"
        }
    }

    private func resolutionDetail(for entry: MaatCase, gate: String) -> String {
        let subject = entry.assessed.isEmpty ? entry.resource : entry.assessed
        switch gate {
        case "escalate":
            return "\(subject) is held at the independent-review boundary. Open the retained evidence to record the required review; this screen does not grant execution authority."
        case "block":
            return "\(subject) is blocked. Open the retained evidence to follow the prescribed remedy or record an accountable owner decision."
        default:
            return "\(subject) needs changes. Open the retained evidence to review the exact findings and record the next accountable step."
        }
    }

    private var emptyState: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("No System One evidence yet", systemImage: "checkmark.seal")
                .sirsiFont(.headline)
            Text("This is an empty evidence history, not a pass or a failure. Choose Observe this Mac to create one qualified local health screen, or record a closed typed screen from another qualified Pantheon producer. Ma'at always shows the deterministic gate before retaining it here.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Text("System One does not invent a screen: until a bounded observation is hashed and validated, Ma'at shows no result and grants no authority.")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            SnapshotActionButton {
                section = .decisions
            } label: {
                Label("Open decisions", systemImage: "checkmark.seal")
            }
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab recipes", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private var hostTriageControl: some View {
        MaatDiagnosticRecordView(model: engine.maatCaseOperations.diagnosticRecord,
                                 refresh: {
                                     if let diagnosticRefresh { await diagnosticRefresh() }
                                     else { await load() }
                                 })
    }

    private var screenImportControl: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Record a closed screen")
                .sirsiFont(.headline)
            Text("Choose an unqualified ADR070 v2 source screen. Enter the expected head independently; it is never copied from the packet. Preview validates declared evidence only—it does not run checks or qualify a model.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            SnapshotActionButton(disabled: sourceImport.busy) {
                showScreenPicker = true
            } label: {
                Label("Choose System One JSON", systemImage: "doc.badge.plus")
                    .frame(maxWidth: .infinity)
            }
            if let selectedScreenURL = sourceImport.selectedFile {
                Text("Selected: \(selectedScreenURL.lastPathComponent)")
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            TextField("Expected source head", text: $sourceImport.expectedHead)
                .textFieldStyle(.roundedBorder)
                .disabled(sourceImport.busy)
                .accessibilityLabel("Expected source head, independently selected")
            Text(MaatSourceImportContract.guidance)
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            DisclosureGroup("V2 JSON format (example identifiers, not proof)") {
                Text(MaatSourceImportContract.example)
                    .sirsiFont(.caption2, design: .monospaced)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }
            SnapshotActionButton(disabled: !sourceImport.canReview) {
                Task {
                    await sourceImport.review()
                    if sourceImport.approval != nil { confirmScreenImport = true }
                }
            } label: {
                Label("Review selected v2 screen", systemImage: "doc.text.magnifyingglass")
            }
            if let preview = sourceImport.preview, sourceImport.approval != nil {
                Text("Preview: \(preview.gate) · expected head \(preview.subject.headSHA). Floor evidence is declared-only. No journal write yet.")
                    .sirsiFont(.caption)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if sourceImport.busy {
                ProgressView(sourceImport.recording ? "Validating and recording…" : "Previewing v2 evidence (no journal write)…")
                    .sirsiFont(.caption)
            }
            if let result = sourceImport.result {
                Label(result.summary, systemImage: result.verdict.gate == "pass" ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(result.verdict.gate == "pass" ? .green : .orange)
                Text("Retained operation for head \(result.verdict.subject.headSHA). Casebook refresh is a separate read; a failed refresh does not undo the recorded evidence or repeat the import.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            }
            if let screenImportError = sourceImport.error {
                Label(screenImportError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private var releasePreflightControl: some View {
        VStack(alignment: .leading, spacing: 9) {
            Text("Preflight the release contract")
                .sirsiFont(.headline)
            if let root = engine.projectRoot {
                Text("Inspect the selected project only: \(root)")
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            } else {
                Text("Choose a project below before preflighting. Ma'at will not guess a checkout or inspect an ambient directory.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                ProjectBar(engine: engine) {
                    releasePreflight = nil
                    releasePreflightError = nil
                    releasePreflightRecorded = false
                }
            }
            if engine.projectRoot != nil {
                Text("This reads and hashes the release-source contract in-process. It never runs a build, package, signing, notarization, network, or release command.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                if releasePreflightInFlight {
                    operationState(
                        "Inspecting source contract…",
                        detail: "Ma'at is reading the selected project's retained source contract. This check has not started a build or release.",
                        symbol: "clock.arrow.circlepath",
                        tint: gold
                    )
                } else {
                    SnapshotActionButton {
                        Task { await inspectReleasePreflight() }
                    } label: {
                        Label("Inspect release contract", systemImage: "checklist")
                            .frame(maxWidth: .infinity)
                    }
                }
            }
            credentialPreflightControl
            if let releasePreflight {
                releasePreflightSummary(releasePreflight)
                if releasePreflightRecorded {
                    operationState(
                        "Recorded in Ma'at Casebook",
                        detail: "This exact observation is retained. Review the decision below or start a fresh inspection after the source changes.",
                        symbol: "checkmark.seal.fill",
                        tint: .green
                    )
                } else {
                    SnapshotActionButton {
                        confirmReleasePreflight = true
                    } label: {
                        Label("Record in Ma'at Casebook", systemImage: "checkmark.shield")
                    }
                }
                Text(releasePreflightRecorded
                     ? "The Casebook was refreshed from this exact retained preflight. The delivery boundary still requires separate credentialed release proof."
                     : "Review the checks below, then explicitly record this exact observation so it appears in the shared Ma'at Casebook.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let releasePreflightError {
                Label(releasePreflightError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                if engine.projectRoot != nil && !releasePreflightInFlight {
                    Button("Try inspection again") { Task { await inspectReleasePreflight() } }
                        .buttonStyle(.bordered)
                }
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private var credentialPreflightControl: some View {
        VStack(alignment: .leading, spacing: 8) {
            Divider()
            Text("Check signing readiness")
                .sirsiFont(.headline)
            Text("Read public local Developer ID certificate metadata only. Ma'at never reads a private key, password, or notarization secret here, and it never contacts Apple or starts a release.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if credentialPreflightInFlight {
                operationState(
                    "Checking public certificate metadata…",
                    detail: "Ma'at is reading public local signing identity metadata only. It cannot read private keys or notarization credentials here.",
                    symbol: "clock.arrow.circlepath",
                    tint: gold
                )
            } else {
                SnapshotActionButton {
                    Task { await inspectCredentialPreflight() }
                } label: {
                    Label("Check local Developer ID readiness", systemImage: "checkmark.shield")
                        .frame(maxWidth: .infinity)
                }
            }
            if let credentialPreflight {
                credentialPreflightSummary(credentialPreflight)
                if credentialPreflightRecorded {
                    operationState(
                        "Readiness evidence recorded",
                        detail: "The public metadata result is retained in Casebook. It is not a signing or notarization authorization.",
                        symbol: "checkmark.seal.fill",
                        tint: .green
                    )
                } else {
                    SnapshotActionButton {
                        confirmCredentialPreflight = true
                    } label: {
                        Label("Record readiness evidence", systemImage: "checkmark.shield")
                    }
                }
            }
            if let credentialPreflightError {
                Label(credentialPreflightError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                if !credentialPreflightInFlight {
                    Button("Try readiness check again") { Task { await inspectCredentialPreflight() } }
                        .buttonStyle(.bordered)
                }
            }
        }
    }

    private func operationState(_ title: String, detail: String, symbol: String, tint: Color) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Label(title, systemImage: symbol)
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(tint)
            Text(detail)
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .padding(.vertical, 3)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder private func releasePreflightSummary(_ preflight: MaatReleaseContractPreflight) -> some View {
        let passed = preflight.verdict.floor.passed
        VStack(alignment: .leading, spacing: 7) {
            Label(passed ? "Source contract floor passed" : "Source contract needs repair", systemImage: passed ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(passed ? .green : .orange)
            Text("Fingerprint: \(preflight.fingerprint)")
                .sirsiFont(.caption2, design: .monospaced)
                .foregroundStyle(.secondary)
                .lineLimit(1)
            ForEach(preflight.verdict.floor.checks, id: \.name) { check in
                Label(check.detail, systemImage: check.passed ? "checkmark.circle.fill" : "xmark.octagon.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(check.passed ? Color.secondary : Color.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
            ForEach(preflight.verdict.findings) { finding in
                if !finding.fixHint.isEmpty {
                    Text("Fix: \(finding.fixHint)")
                        .sirsiFont(.caption)
                        .foregroundStyle(.primary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill((passed ? Color.green : Color.orange).opacity(0.09)))
    }

    @ViewBuilder private func credentialPreflightSummary(_ preflight: MaatReleaseCredentialPreflight) -> some View {
        let projection = MaatCredentialPreflightProjection(preflight)
        let recoverySteps = protectedReleaseRecoverySteps(for: preflight)
        VStack(alignment: .leading, spacing: 7) {
            Label(projection.identityHeading, systemImage: "person.crop.rectangle.stack")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(projection.identityObservationFailed || !projection.allRequiredKindsObserved ? Color.orange : Color.secondary)
            Text("Team \(preflight.teamID) · \(projection.requiredKindsSummary)")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
            ForEach(projection.additionalIdentityLabels, id: \.self) { label in
                Text(label)
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            }
            ForEach(projection.requiredIdentityChecks, id: \.name) { check in
                Label(check.detail, systemImage: check.passed ? "checkmark.circle.fill" : "xmark.octagon.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(check.passed ? Color.secondary : Color.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Label(projection.notarizationStatus, systemImage: "questionmark.circle")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            ForEach(preflight.verdict.findings) { finding in
                if finding.id != "notarization-credential" && !finding.fixHint.isEmpty {
                    Text("Next: \(finding.fixHint)")
                        .sirsiFont(.caption)
                    .fixedSize(horizontal: false, vertical: true)
                }
            }
            VStack(alignment: .leading, spacing: 6) {
                Text("Resolution plan")
                    .sirsiFont(.caption, weight: .semibold)
                ForEach(Array(recoverySteps.enumerated()), id: \.offset) { index, step in
                    Label("\(index + 1). \(step)", systemImage: "checklist")
                        .sirsiFont(.caption)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(.top, 2)
            SnapshotActionButton {
                Task { await inspectCredentialPreflight() }
            } label: {
                Label("Recheck local identities", systemImage: "arrow.clockwise")
            }
            .disabled(credentialPreflightInFlight)
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect release evidence and runbook", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
            Text("Fingerprint: \(preflight.fingerprint)")
                .sirsiFont(.caption2, design: .monospaced)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill((projection.identityObservationFailed || !projection.allRequiredKindsObserved ? Color.orange : Color.secondary).opacity(0.08)))
    }

    @ViewBuilder private func screenList(_ screens: [MaatCase]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Recorded screens")
                .sirsiFont(.headline)
            ForEach(screens) { entry in
                NavLink { MaatCaseDetailView(engine: engine, operations: engine.maatCaseOperations, entry: entry) } label: {
                    HStack(alignment: .top, spacing: 10) {
                        Circle().fill(gateTint(entry.systemOne?.gate ?? "")).frame(width: 9, height: 9).padding(.top, 4)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(entry.assessed.isEmpty ? entry.resource : entry.assessed)
                                .sirsiFont(.headline)
                            Text(screenDetail(entry))
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                            if !entry.evidence.isEmpty {
                                Text(entry.evidence)
                                    .sirsiFont(.caption2, design: .monospaced)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                        }
                        Spacer(minLength: 8)
                        Image(systemName: "chevron.right")
                            .sirsiFont(.caption, weight: .semibold)
                            .foregroundStyle(.tertiary)
                    }
                    .padding(13)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
                }
                .buttonStyle(.plain)
            }
        }
    }

    @ViewBuilder private func calibrationList(_ calibrations: [MaatCase]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Recorded independent calibrations")
                .sirsiFont(.headline)
            ForEach(calibrations) { entry in
                NavLink { MaatCaseDetailView(engine: engine, operations: engine.maatCaseOperations, entry: entry) } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Screen \(entry.systemOneCalibration?.screenGate ?? "unknown") · independent outcome \(entry.systemOneCalibration?.frontierGate ?? "unknown")")
                            .sirsiFont(.subheadline, weight: .semibold)
                        Text("\(entry.systemOneCalibration?.screenEvidence ?? "No screen evidence") · \(entry.systemOneCalibration?.frontierEvidence ?? "No independent evidence")")
                            .sirsiFont(.caption2, design: .monospaced)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(12)
                    .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
                }
                .buttonStyle(.plain)
            }
        }
    }

    private func screenDetail(_ entry: MaatCase) -> String {
        guard let verdict = entry.systemOne else { return entry.why }
        var detail = "\(verdict.gate.capitalized) · \(Int(verdict.confidence * 100))% confidence · feather \(verdict.featherWeight)/100"
        if let escalation = verdict.escalation { detail += " · \(escalation.reason)" }
        return detail
    }

    private func gateTint(_ gate: String) -> Color {
        switch gate {
        case "pass": return .green
        case "changes": return .orange
        case "block": return .red
        case "escalate": return gold
        default: return .secondary
        }
    }

    @MainActor private func importScreen() async {
        guard let invocation = sourceImport.confirm() else { return }
        await invocation.value
        if sourceImport.error == nil && sourceImport.result != nil { await load() }
    }

    @MainActor private func inspectReleasePreflight() async {
        guard let root = engine.projectRoot else { return }
        releasePreflightInFlight = true
        releasePreflightError = nil
        releasePreflightRecorded = false
        let raw = await SirsiEngine.run(args: ["maat", "preflight", "release", "--root", root, "--json"], stdin: nil)
        guard let result = MaatReleaseContractPreflight.decode(raw) else {
            releasePreflight = nil
            releasePreflightError = "Ma'at could not read a typed release-contract result. Nothing was recorded or executed. Check the selected project, then retry this exact inspection. \(SirsiEngine.firstMeaningful(raw))"
            releasePreflightInFlight = false
            return
        }
        releasePreflight = result
        releasePreflightInFlight = false
    }

    @MainActor private func recordReleasePreflight() async {
        guard let root = engine.projectRoot else { return }
        releasePreflightInFlight = true
        releasePreflightError = nil
        let raw = await SirsiEngine.run(args: ["maat", "preflight", "release", "--root", root, "--confirm", "--json"], stdin: nil)
        guard let result = MaatReleaseContractPreflight.decode(raw) else {
            releasePreflightError = "Ma'at could not record this release-contract observation. The project was not changed and no release action ran. Retry the inspection, then confirm only after reviewing the typed checks. \(SirsiEngine.firstMeaningful(raw))"
            releasePreflightInFlight = false
            return
        }
        releasePreflight = result
        releasePreflightRecorded = !result.decisionEvidence.isEmpty
        if releasePreflightRecorded {
            await load()
        } else {
            releasePreflightError = "Ma'at returned the observation but did not confirm a Casebook evidence record. Nothing was treated as accepted; inspect the result and retry confirmation."
        }
        releasePreflightInFlight = false
    }

    @MainActor private func inspectCredentialPreflight() async {
        guard !credentialPreflightInFlight else { return }
        credentialPreflightInFlight = true
        credentialPreflightError = nil
        credentialPreflightRecorded = false
        let raw = await SirsiEngine.run(args: ["maat", "preflight", "credentials", "--json"], stdin: nil)
        guard let result = MaatReleaseCredentialPreflight.decode(raw) else {
            credentialPreflight = nil
            credentialPreflightError = "Ma'at could not read a typed local credential readiness result. Nothing was recorded, signed, or released. Unlock the release keychain/session if needed, then retry. \(SirsiEngine.firstMeaningful(raw))"
            credentialPreflightInFlight = false
            return
        }
        credentialPreflight = result
        credentialPreflightInFlight = false
    }

    @MainActor private func recordCredentialPreflight() async {
        guard !credentialPreflightInFlight else { return }
        credentialPreflightInFlight = true
        credentialPreflightError = nil
        let raw = await SirsiEngine.run(args: ["maat", "preflight", "credentials", "--confirm", "--json"], stdin: nil)
        guard let result = MaatReleaseCredentialPreflight.decode(raw) else {
            credentialPreflightError = "Ma'at could not record this credential readiness evidence. No release action ran. Retry the check, then confirm only after reviewing the public metadata. \(SirsiEngine.firstMeaningful(raw))"
            credentialPreflightInFlight = false
            return
        }
        credentialPreflight = result
        credentialPreflightRecorded = !result.decisionEvidence.isEmpty
        if credentialPreflightRecorded {
            await load()
        } else {
            credentialPreflightError = "Ma'at returned the readiness check but did not confirm a Casebook evidence record. Nothing was treated as accepted."
        }
        credentialPreflightInFlight = false
    }

    @MainActor private func load(cursor: String? = nil) async {
        await historyRefresh.load(scope: "system-one", cursor: cursor) { args in
            await SirsiEngine.runJSONRead(args: args)
        }
    }
}

// MaatKnownFailureProposalsView makes the local intake for recurring failures
// visible in the product. A proposal is evidence for Stack Lab review, not a
// hidden source edit and not a new recognition rule. It therefore gives an
// operator a complete next step without making an unreviewed report affect
// other Pantheon installations.
struct MaatKnownFailureProposalsView: View {
    @ObservedObject var engine: SirsiEngine
    let showsBackBar: Bool
    @State private var proposals: [MaatKnownFailureProposal] = []
    @State private var loading = true
    @State private var loadError: String?
    @State private var showRecorder = false
    @State private var confirmRecord = false
    @State private var recording = false
    @State private var recordError: String?
    @State private var proposalID = ""
    @State private var proposalTitle = ""
    @State private var proposalSignature = ""
    @State private var proposalCause = ""

    var body: some View {
        VStack(spacing: 0) {
            if showsBackBar { BackBar(title: "Ma'at proposals") }
            if loading {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("Opening local Ma'at proposals…")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let loadError {
                unavailableState(loadError)
            } else {
                proposalBody
            }
        }
        .task { await load() }
        .sheet(isPresented: $showRecorder) { proposalRecorder }
        .confirmationDialog("Record this local proposal?", isPresented: $confirmRecord, titleVisibility: .visible) {
            Button("Record local proposal") { Task { await recordProposal() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Ma'at will create a durable local proposal for Stack Lab review. It will not change source, enable recognition across the fabric, run a repair, or contact a remote service.")
        }
    }

    private var proposalBody: some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    VStack(alignment: .leading, spacing: 8) {
                        Label("Known-failure proposals", systemImage: "lightbulb.max")
                            .sirsiFont(.title3, weight: .bold)
                        Text("Capture a recurring failure once, then route it through Stack Lab review. Local proposals never become fabric-wide recognition rules until they are reviewed and promoted.")
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                        SnapshotActionButton(disabled: recording) {
                            resetRecorder()
                            showRecorder = true
                        } label: {
                            Label("Record an observation", systemImage: "plus.circle.fill")
                                .frame(maxWidth: .infinity)
                        }
                        .accessibilityHint("Creates a local Ma'at proposal for Stack Lab review. It does not alter a shared failure catalog.")
                    }
                    .padding(14)
                    .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))

                    if proposals.isEmpty {
                        VStack(alignment: .leading, spacing: 7) {
                            Text("No local proposals")
                                .sirsiFont(.headline)
                            Text("This is an empty local review queue, not confirmation that recurring failures are resolved. Record an observation when a failure has a stable signature and a clear cause.")
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(14)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.045)))
                    } else {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("Ready for Stack Lab review")
                                .sirsiFont(.headline)
                            Text("Review each proposal against the current catalog before promoting it into a release recipe.")
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.secondary)
                            ForEach(proposals) { proposal in
                                proposalRow(proposal)
                            }
                        }
                    }

                    NavLink { StackLabView(engine: engine) } label: {
                        Label("Open Stack Lab review", systemImage: "cube.transparent")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)
                }
                .padding(16)
            }
            Divider()
            HStack {
                Text("Local evidence · no shared catalog mutation")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button { Task { await load() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .disabled(loading || recording)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 11)
        }
    }

    private func proposalRow(_ proposal: MaatKnownFailureProposal) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(alignment: .firstTextBaseline) {
                Text(proposal.titleOrID)
                    .sirsiFont(.subheadline, weight: .semibold)
                Spacer()
                Text(proposal.status.capitalized)
                    .sirsiFont(.caption, weight: .semibold)
                    .foregroundStyle(gold)
            }
            Text(proposal.cause)
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Text("Signature: \(proposal.signature)")
                .sirsiFont(.caption, design: .monospaced)
                .foregroundStyle(.secondary)
                .lineLimit(2)
            Text("Recorded \(proposal.createdAtUTC)")
                .sirsiFont(.caption2)
                .foregroundStyle(.tertiary)
            Text(proposal.catalogRelationLabel)
                .sirsiFont(.caption2)
                .foregroundStyle(.secondary)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.045)))
    }

    private func unavailableState(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(.orange)
            Text("Local proposal evidence is unavailable")
                .sirsiFont(.headline)
            Text(message)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Try again") { Task { await load() } }
                .buttonStyle(.borderedProminent)
                .tint(gold)
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab authority", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private var proposalRecorder: some View {
        NavigationStack {
            Form {
                Section("Recurring failure") {
                    TextField("Stable id (for example, package-input-reopen)", text: $proposalID)
                    TextField("Short title", text: $proposalTitle)
                    TextField("Failure signature (regular expression)", text: $proposalSignature, axis: .vertical)
                        .lineLimit(2...4)
                    TextField("Cause", text: $proposalCause, axis: .vertical)
                        .lineLimit(2...4)
                }
                Section("What happens next") {
                    Text("Pantheon stores this as a local Ma'at proposal. Stack Lab review decides whether it should become a shipped recognition rule. Nothing is repaired or changed automatically.")
                        .sirsiFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                if let recordError {
                    Section("Needs attention") {
                        Text(recordError)
                            .sirsiFont(.footnote)
                            .foregroundStyle(.orange)
                    }
                }
            }
            .navigationTitle("Record observation")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { showRecorder = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Continue") {
                        showRecorder = false
                        confirmRecord = true
                    }
                    .disabled(!canRecord || recording)
                }
            }
        }
        .frame(minWidth: 520, minHeight: 410)
    }

    private var canRecord: Bool {
        !proposalID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !proposalSignature.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !proposalCause.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    private func resetRecorder() {
        proposalID = ""
        proposalTitle = ""
        proposalSignature = ""
        proposalCause = ""
        recordError = nil
    }

    @MainActor private func load() async {
        loading = true
        loadError = nil
        let raw = await SirsiEngine.run(args: ["maat", "known-failures", "proposals", "--json"], stdin: nil)
        guard let result = Self.decode(raw) else {
            loadError = "Pantheon could not read the typed local Ma'at proposal queue. No proposal was treated as reviewed or promoted. \(SirsiEngine.firstMeaningful(raw))"
            loading = false
            return
        }
        proposals = result
        loading = false
    }

    @MainActor private func recordProposal() async {
        guard canRecord, !recording else { return }
        recording = true
        recordError = nil
        var args = ["maat", "known-failures", "register", proposalID.trimmingCharacters(in: .whitespacesAndNewlines), "--signature", proposalSignature.trimmingCharacters(in: .whitespacesAndNewlines), "--cause", proposalCause.trimmingCharacters(in: .whitespacesAndNewlines)]
        if !proposalTitle.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            args += ["--title", proposalTitle.trimmingCharacters(in: .whitespacesAndNewlines)]
        }
        args.append("--json")
        let raw = await SirsiEngine.run(args: args, stdin: nil)
        guard Self.decodeProposal(raw) != nil else {
            recordError = "Pantheon did not create a verified local proposal. Nothing was promoted. \(SirsiEngine.firstMeaningful(raw))"
            recording = false
            showRecorder = true
            return
        }
        recording = false
        await load()
    }

    nonisolated static func decode(_ raw: String) -> [MaatKnownFailureProposal]? {
        guard let start = raw.firstIndex(of: "[") else { return nil }
        return try? JSONDecoder().decode([MaatKnownFailureProposal].self, from: Data(raw[start...].utf8))
    }

    nonisolated static func decodeProposal(_ raw: String) -> MaatKnownFailureProposal? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatKnownFailureProposal.self, from: Data(raw[start...].utf8))
    }
}

// MaatCasebookView is the native operator surface for Ma'at's recorded
// decisions. It reads the same local casebook projection exposed by the CLI
// and dashboard; it does not rescore, alter, or authorize a decision.
@MainActor struct MaatCasebookView: View {
    @ObservedObject var engine: SirsiEngine
    let showsBackBar: Bool
    @StateObject private var historyRefresh: MaatHistoryRefreshStore
    private var casebook: MaatCasebookProjection? { historyRefresh.state.projection }
    private var loading: Bool { historyRefresh.state.loading }
    private var loadError: String? { historyRefresh.state.error }
    private var activeQuery: String { historyRefresh.state.query }
    @State private var query = ""
    @State private var showWithheld = false
    @State private var confirmKnowledgeRefresh = false
    @State private var knowledgeRefreshInFlight = false
    @State private var knowledgeRefreshResult: String?
    @State private var knowledgeRefreshError: String?

    init(engine: SirsiEngine, preloaded: MaatCasebookProjection? = nil, showsBackBar: Bool = true,
         historyStore: MaatHistoryRefreshStore? = nil) {
        self.engine = engine
        self.showsBackBar = showsBackBar
        _query = State(initialValue: historyStore?.state.query ?? "")
        _historyRefresh = StateObject(wrappedValue: historyStore ?? MaatHistoryRefreshStore(projection: preloaded))
    }

    var body: some View {
        VStack(spacing: 0) {
            if showsBackBar { BackBar(title: "Ma'at casebook") }
            if let casebook {
                casebookBody(casebook)
            } else if loading {
                loadingState
            } else if let loadError {
                failureState(loadError)
            } else {
                emptyState
            }
        }
        .task {
            guard casebook == nil else { return }
            await load()
        }
        .onDisappear { historyRefresh.cancel() }
    }

    private var loadingState: some View {
        VStack(spacing: 10) {
            ProgressView()
            Text("Searching retained history…")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
            Button("Cancel") { historyRefresh.cancel() }
                .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func failureState(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(.orange)
            Text("The casebook is unavailable")
                .sirsiFont(.headline)
            Text(message)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            MaatLifecycleControl(engine: engine)
            Button("Try again") { Task { await load() } }
                .buttonStyle(.borderedProminent)
                .tint(gold)
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab authority", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
            NavLink { ResultView(engine: engine, title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                Label("Run a fresh Ma'at audit", systemImage: "checkmark.seal")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private var emptyState: some View {
        VStack(alignment: .leading, spacing: 10) {
            Image(systemName: "checkmark.seal")
                .sirsiFont(26, weight: .semibold)
                .foregroundStyle(.secondary)
            Text("No recorded Ma'at decisions yet")
                .sirsiFont(.headline)
            Text("Run a Ma'at audit to add evidence-linked quality and governance decisions here.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            MaatLifecycleControl(engine: engine)
            NavLink { ResultView(engine: engine, title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                Label("Run Ma'at audit", systemImage: "checkmark.seal")
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(RoundedRectangle(cornerRadius: 8).fill(Color.accentColor.opacity(0.16)))
            }
            .foregroundStyle(.primary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func casebookBody(_ casebook: MaatCasebookProjection) -> some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    summary(casebook.summary, history: casebook.history)
                    MaatLifecycleControl(engine: engine)
                    if loading {
                        HStack {
                            Label("Searching retained history…", systemImage: "arrow.clockwise")
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                            Spacer()
                            Button("Cancel") { historyRefresh.cancel() }
                                .buttonStyle(.borderless)
                        }
                    }
                    if casebook.journalIntegrity.invalidCount > 0 {
                        MaatJournalIntegrityCard(engine: engine, integrity: casebook.journalIntegrity)
                    }
                    searchField
                    if let loadError {
                        VStack(alignment: .leading, spacing: 8) {
                            Label(loadError, systemImage: "exclamationmark.triangle.fill")
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.orange)
                            if historyRefresh.state.restartRequired {
                                Button("Restart history from the latest snapshot") { Task { await load() } }
                                    .buttonStyle(.borderedProminent)
                                    .disabled(loading)
                            }
                        }
                    }
                    let visible = casebook.cases
                    if visible.isEmpty, casebook.historyEmptyState == .incomplete, let cursor = casebook.history?.nextCursor {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("This bounded history scan is incomplete; no empty result is available yet.")
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.secondary)
                            Button("Continue history search") { Task { await load(cursor: cursor) } }
                                .buttonStyle(.borderedProminent)
                                .disabled(loading || historyRefresh.state.restartRequired)
                        }
                    } else if visible.isEmpty, casebook.historyEmptyState == .olderPage, let cursor = casebook.history?.nextCursor {
                        VStack(alignment: .leading, spacing: 8) {
                            Text("No matching decisions on this page; older history remains available.")
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.secondary)
                            Button("Load older decisions") { Task { await load(cursor: cursor) } }
                                .buttonStyle(.borderedProminent)
                                .disabled(loading || historyRefresh.state.restartRequired)
                        }
                    } else if visible.isEmpty, casebook.historyEmptyState == .invalidHistory {
                        Text("No valid decision matched this page. Invalid journal rows prevent a complete absence conclusion.")
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.secondary)
                    } else if visible.isEmpty, casebook.historyEmptyState == .afterCursor {
                        Text("No matching decisions remain after this cursor. Refresh to search the full history again.")
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.secondary)
                    } else if visible.isEmpty, casebook.historyEmptyState == .unavailable {
                        Text("History coverage is unavailable; no absence conclusion was made.")
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.secondary)
                    } else if visible.isEmpty, activeQuery.isEmpty {
                        emptyState
                    } else {
                        caseList(for: visible)
                    }
                }
                .padding(16)
            }
            Divider()
            HStack {
                Text("Local evidence journal · read only")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button {
                    Task { await load() }
                } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .disabled(loading)
                if let history = casebook.history, let cursor = history.nextCursor {
                    Button { Task { await load(cursor: cursor) } } label: {
                        Label(history.coverage == "partial" ? "Continue history search" : "Load older", systemImage: "clock.arrow.circlepath")
                    }
                    .buttonStyle(.borderless)
                    .disabled(loading || historyRefresh.state.restartRequired)
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 11)
        }
    }

    private func summary(_ summary: MaatCasebookSummary, history: MaatHistoryWindow?) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Decision casebook")
                .sirsiFont(.title3, weight: .bold)
            Text(Self.summaryDetail(summary, history: history))
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
            if let history, history.coverage == "partial" {
                Text(history.boundedProgressText ?? "History search remains incomplete; no whole-history total is available.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            } else if let history, history.countScope == "after-cursor" {
                Text("This page covers matches after its cursor; whole-query totals are unknown.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            } else if let history, history.hasMore, history.candidateMatchesComplete {
                Text("Showing this page of \(history.candidateMatches) matching decisions.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            } else if let history, !history.totalComplete {
                Text("Loaded page · counts are not whole-history totals.")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
            }
            HStack(spacing: 8) {
                caseMetric("Open", value: summary.open, tint: .orange)
                caseMetric("Urgent", value: summary.urgent, tint: .red)
                caseMetric("Resolved", value: summary.resolved, tint: .green)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    static func summaryDetail(_ summary: MaatCasebookSummary, history: MaatHistoryWindow?) -> String {
        guard summary.total > 0 else {
            return history?.coverage == "partial"
                ? "No matches in this bounded scan chunk; history remains incomplete."
                : "No decisions match this page"
        }
        if summary.urgent > 0 { return "Start with the decisions on this page that need a response." }
        if summary.open > 0 { return "\(summary.open) decision\(summary.open == 1 ? "" : "s") on this page still need attention." }
        return "All decisions on this page are resolved."
    }

    private func caseMetric(_ title: String, value: Int, tint: Color) -> some View {
        HStack(spacing: 5) {
            Circle().fill(tint).frame(width: 7, height: 7)
            Text("\(value) \(title)")
                .sirsiFont(.caption, weight: .semibold)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
        .background(Capsule().fill(tint.opacity(0.12)))
    }

    private var searchField: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Filter decisions, evidence, or resource", text: $query)
                .textFieldStyle(.plain)
                .accessibilityLabel("Filter Ma'at decisions")
                .onSubmit { Task { await load(query: query) } }
            Button("Search") { Task { await load(query: query) } }
                .buttonStyle(.borderless)
                .accessibilityHint("Searches retained history before applying the page limit.")
            if !query.isEmpty {
                Button {
                    query = ""
                    Task { await load(query: "") }
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(.tertiary)
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Clear filter")
            }
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 9)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.07)))
    }

    @ViewBuilder private func caseList(for cases: [MaatCase]) -> some View {
        if cases.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                Text("No matching decisions")
                    .sirsiFont(.headline)
                Text("Try a broader term or clear the filter.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
            }
            .padding(.vertical, 28)
            .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            VStack(spacing: 1) {
                ForEach(cases) { entry in
                    NavLink { MaatCaseDetailView(engine: engine, operations: engine.maatCaseOperations, entry: entry) } label: {
                        MaatCaseRow(entry: entry)
                    }
                    if entry.id != cases.last?.id { Divider().padding(.leading, 14) }
                }
            }
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.045)))
        }
    }

    @MainActor private func load(query requestedQuery: String? = nil, cursor: String? = nil) async {
        await historyRefresh.load(scope: "all", query: requestedQuery, cursor: cursor) { args in
            await SirsiEngine.runJSONRead(args: args)
        }
    }

    nonisolated static func fetchRead(query: String = "", cursor: String? = nil) async -> SirsiEngine.JSONCommandRead {
        await SirsiEngine.runJSONRead(args: maatCasebookHistoryArguments(scope: "all", query: query, cursor: cursor))
    }

    // Snapshot callers keep the existing projection API while sharing the
    // same successful-process requirement as interactive history browsing.
    nonisolated static func fetch(cursor: String? = nil) async -> MaatCasebookProjection? {
        decode(await fetchRead(cursor: cursor))
    }

    nonisolated static func decode(_ read: SirsiEngine.JSONCommandRead) -> MaatCasebookProjection? {
        guard read.exitStatus == 0, !read.timedOut, read.launchError == nil,
              let raw = String(data: read.data, encoding: .utf8) else { return nil }
        return decode(raw)
    }

    nonisolated static func decode(_ raw: String) -> MaatCasebookProjection? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatCasebookProjection.self,
                                         from: Data(raw[start...].utf8))
    }
}

// MaatKnowledgeView presents the retained local knowledge library as a typed
// product surface. It calls the canonical `maat knowledge` read model rather
// than putting a Seshat command transcript in the menubar.
struct MaatKnowledgeView: View {
    private let maximumLoadedKnowledgeItems = 100
    @ObservedObject var engine: SirsiEngine
    let showsBackBar: Bool
    @State private var knowledge: MaatKnowledgeProjection?
    @State private var query = ""
    @State private var showWithheld = false
    @State private var loading = true
    @State private var loadError: String?
    @State private var confirmKnowledgeRefresh = false
    @State private var knowledgeRefreshInFlight = false
    @State private var knowledgeRefreshResult: String?
    @State private var knowledgeRefreshError: String?
    @State private var knowledgeProjectionInFlight = false
    @State private var knowledgeProjectionReceipt: MaatKnowledgeProjectionReceipt?
    @State private var knowledgeProjectionError: String?

    init(engine: SirsiEngine, preloaded: MaatKnowledgeProjection? = nil, showsBackBar: Bool = true) {
        self.engine = engine
        self.showsBackBar = showsBackBar
        _knowledge = State(initialValue: preloaded)
        _loading = State(initialValue: preloaded == nil)
    }

    var body: some View {
        VStack(spacing: 0) {
            if showsBackBar { BackBar(title: "Ma'at knowledge") }
            if loading {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("Opening the local knowledge library…")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let knowledge {
                knowledgeBody(knowledge)
            } else if let loadError {
                failureState(loadError)
            }
        }
        .task {
            guard knowledge == nil else { return }
            await load()
        }
        .confirmationDialog("Refresh Ma'at knowledge?", isPresented: $confirmKnowledgeRefresh, titleVisibility: .visible) {
            Button("Refresh local knowledge") { Task { await refreshKnowledge() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Ma'at will read configured local sources and update its local knowledge cache. It will not export knowledge, open a browser, authorize work, or make a remote decision.")
        }
    }

    private func failureState(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(.orange)
            Text("The knowledge library is unavailable")
                .sirsiFont(.headline)
            Text(message)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Try again") { Task { await load() } }
                .buttonStyle(.borderedProminent)
                .tint(gold)
            Button("Refresh local knowledge") { confirmKnowledgeRefresh = true }
                .buttonStyle(.bordered)
                .disabled(knowledgeRefreshInFlight)
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab authority", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func knowledgeBody(_ knowledge: MaatKnowledgeProjection) -> some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    if let loadError {
                        Label(loadError, systemImage: "exclamationmark.triangle.fill")
                            .sirsiFont(.caption)
                            .foregroundStyle(.orange)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    VStack(alignment: .leading, spacing: 5) {
                        Text("Local knowledge")
                            .sirsiFont(.title3, weight: .bold)
                        Text(knowledge.total == 0 ? "No local knowledge has been ingested." : "\(knowledge.total) retained item\(knowledge.total == 1 ? "" : "s") available to Ma'at.")
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.secondary)
                        if knowledge.withheld > 0 {
                            Label("\(knowledge.withheld) sensitive item\(knowledge.withheld == 1 ? " was" : "s were") withheld from this view", systemImage: "lock.fill")
                                .sirsiFont(.caption, weight: .semibold)
                                .foregroundStyle(.orange)
                        }
                    }
                    if let coverage = knowledge.coverage {
                        if coverage.pendingProjectionDebt > 0 {
                            Label("\(coverage.pendingProjectionDebt) owner decision(s) still need Casebook projection", systemImage: "arrow.triangle.2.circlepath")
                                .sirsiFont(.caption, weight: .semibold)
                                .foregroundStyle(.orange)
                            Button("Retry Casebook projection") { Task { await reconcileKnowledgeProjection() } }
                                .buttonStyle(.bordered)
                                .disabled(knowledgeProjectionInFlight)
                            if let knowledgeProjectionReceipt {
                                Label(knowledgeProjectionReceipt.summary,
                                      systemImage: knowledgeProjectionReceipt.isComplete ? "checkmark.seal" : "arrow.triangle.2.circlepath")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(knowledgeProjectionReceipt.isComplete ? .green : .orange)
                            }
                            if let knowledgeProjectionError {
                                Label(knowledgeProjectionError, systemImage: "exclamationmark.triangle")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.orange)
                            }
                        }
                        ForEach(coverage.sources) { source in
                            if source.state != "complete" && source.state != "no_new_items" {
                                Label("\(source.source): \(source.state) · \(source.errorCode ?? "review required")", systemImage: "exclamationmark.triangle")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.orange)
                            }
                        }
                    }
                    knowledgeRefreshControl
                    searchField
                    knowledgeList(knowledge.items)
                    if let cursor = knowledge.nextCursor {
                        Button("Load next stable page") { Task { await loadNext(cursor) } }
                            .buttonStyle(.bordered)
                            .disabled(loading)
                    } else if knowledge.items.count >= maximumLoadedKnowledgeItems && knowledge.total > knowledge.items.count {
                        Text("The native view is capped at \(maximumLoadedKnowledgeItems) loaded items. Narrow the search or use the explicit CLI cursor for older pages.")
                            .sirsiFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
                .padding(16)
            }
            Divider()
            HStack {
                Text("Local cache · refresh requires confirmation")
                    .sirsiFont(.caption)
                    .foregroundStyle(.secondary)
                Spacer()
                Button { Task { await load() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.borderless)
                .disabled(loading)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 11)
        }
    }

    private var searchField: some View {
        HStack(spacing: 8) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Filter knowledge", text: $query)
                .textFieldStyle(.plain)
                .accessibilityLabel("Filter Ma'at knowledge")
                .onSubmit { Task { await load(query: query) } }
            Button { Task { await load(query: query) } } label: {
                Image(systemName: "arrow.right.circle.fill").foregroundStyle(gold)
            }
            .buttonStyle(.borderless)
            .accessibilityLabel("Search local knowledge")
            Button {
                showWithheld.toggle()
                query = ""
                Task { await load(query: "") }
            } label: {
                Label("Withheld", systemImage: showWithheld ? "lock.fill" : "lock")
                    .labelStyle(.iconOnly)
                    .foregroundStyle(showWithheld ? .orange : .secondary)
            }
            .buttonStyle(.borderless)
            .accessibilityLabel(showWithheld ? "Show searchable knowledge" : "Show withheld items and reasons")
            if !query.isEmpty {
                Button { query = ""; Task { await load(query: "") } } label: {
                    Image(systemName: "xmark.circle.fill").foregroundStyle(.tertiary)
                }
                .buttonStyle(.borderless)
                .accessibilityLabel("Clear filter")
            }
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 9)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.07)))
    }

    private var knowledgeRefreshControl: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Keep local knowledge current")
                .sirsiFont(.headline)
            Text("Refresh reads local Claude transcript files into Ma'at's private archive. Confirm before reading; this does not contact online workspaces, export data, open a browser, or make a remote decision.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button {
                confirmKnowledgeRefresh = true
            } label: {
                Label("Refresh local knowledge", systemImage: "arrow.triangle.2.circlepath")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
            .disabled(knowledgeRefreshInFlight)
            if knowledgeRefreshInFlight {
                ProgressView("Refreshing configured local sources…")
                    .sirsiFont(.caption)
            }
            if let knowledgeRefreshResult {
                Label(knowledgeRefreshResult, systemImage: "checkmark.seal.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.green)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let knowledgeRefreshError {
                Label(knowledgeRefreshError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                Button("Try refresh again") { confirmKnowledgeRefresh = true }
                    .buttonStyle(.bordered)
                    .disabled(knowledgeRefreshInFlight)
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    @ViewBuilder private func knowledgeList(_ items: [MaatKnowledgeItem]) -> some View {
        if items.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                Text(query.isEmpty ? "Nothing in the local library" : "No matching knowledge")
                    .sirsiFont(.headline)
                Text(query.isEmpty ? "Use the confirmed local refresh to read the enabled transcript source." : "Try a broader term or clear the filter.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.vertical, 28)
            .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            VStack(spacing: 1) {
                ForEach(items) { item in
                    NavLink { MaatKnowledgeDetailView(item: item) } label: {
                        VStack(alignment: .leading, spacing: 4) {
                            HStack(spacing: 7) {
                                Image(systemName: "doc.text").foregroundStyle(.secondary)
                                Text(item.title)
                                    .sirsiFont(.headline)
                                    .lineLimit(1)
                                Spacer()
                                Image(systemName: "chevron.right")
                                    .sirsiFont(.caption, weight: .semibold)
                                    .foregroundStyle(.tertiary)
                            }
                            if !item.summary.isEmpty {
                                Text(item.summary)
                                    .sirsiFont(.subheadline)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(2)
                            }
                            if let source = item.source {
                                Label(source, systemImage: "tray.full")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.tertiary)
                            }
                            if let reason = item.withheldReason {
                                Label("Withheld: \(reason)", systemImage: "lock.fill")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.orange)
                            }
                        }
                        .padding(.horizontal, 13)
                        .padding(.vertical, 11)
                        .contentShape(Rectangle())
                    }
                    if item.id != items.last?.id { Divider().padding(.leading, 14) }
                }
            }
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.045)))
        }
    }

    @MainActor private func load(query requestedQuery: String? = nil) async {
        loading = true
        loadError = nil
        let result = await Self.fetch(query: showWithheld ? "" : requestedQuery ?? query, withheld: showWithheld)
        if let result {
            knowledge = result
        } else {
            loadError = "Pantheon could not read Ma'at's local knowledge projection. Check the project and try again."
        }
        loading = false
    }

    @MainActor private func loadNext(_ cursor: String) async {
        guard let knowledge else { return }
        loading = true
        loadError = nil
        let result = await Self.fetch(query: showWithheld ? "" : query, cursor: cursor, withheld: showWithheld)
        if let result {
            let boundedItems = Array((knowledge.items + result.items).prefix(maximumLoadedKnowledgeItems))
            self.knowledge = MaatKnowledgeProjection(
                items: boundedItems, total: result.total, withheld: result.withheld,
                coverage: result.coverage,
                nextCursor: boundedItems.count < maximumLoadedKnowledgeItems ? result.nextCursor : nil,
                state: result.state
            )
        } else {
            loadError = "Ma'at could not continue this stable archive page. Restart the search and try again."
        }
        loading = false
    }

    @MainActor private func refreshKnowledge() async {
        knowledgeRefreshInFlight = true
        knowledgeRefreshResult = nil
        knowledgeRefreshError = nil
        let result = await SirsiEngine.runResult(args: ["maat", "knowledge", "refresh"])
        if let result, result.ok {
            knowledgeRefreshResult = result.summary
            await load()
        } else {
            let detail = result?.errors.first ?? "No typed completion receipt was returned."
            knowledgeRefreshError = Self.refreshFailureMessage(
                retainedViewAvailable: knowledge != nil,
                detail: detail
            )
        }
        knowledgeRefreshInFlight = false
    }

    @MainActor private func reconcileKnowledgeProjection() async {
        knowledgeProjectionInFlight = true
        knowledgeProjectionReceipt = nil
        knowledgeProjectionError = nil
        let result = await SirsiEngine.runResult(args: ["maat", "knowledge", "reconcile-projections"])
        if let result, let receipt = MaatKnowledgeProjectionReceipt(commandResult: result) {
            knowledgeProjectionReceipt = receipt
            await load()
        } else {
            knowledgeProjectionError = "Ma'at could not confirm the Casebook projection receipt. Check current knowledge status before deciding whether to retry."
        }
        knowledgeProjectionInFlight = false
    }

    nonisolated static func refreshFailureMessage(retainedViewAvailable: Bool, detail: String) -> String {
        let retainedViewStatus = retainedViewAvailable
            ? "The displayed retained view remains available and unchanged."
            : "The archive could not be read; its retained contents are unverified."
        return "Ma'at could not refresh local knowledge. \(retainedViewStatus) Check the reported source issue, then retry this confirmed refresh. \(detail)"
    }

    nonisolated static func fetch(query: String = "", cursor: String? = nil, withheld: Bool = false) async -> MaatKnowledgeProjection? {
        var args = ["maat", "knowledge"]
        if withheld {
            args.append("--withheld")
        } else if !query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            args += ["--query", query]
        }
        if let cursor, !cursor.isEmpty { args += ["--cursor", cursor] }
        args.append("--json")
        let read = await SirsiEngine.runJSONRead(args: args)
        return decodeProjection(read)
    }

    nonisolated static func decodeProjection(_ read: SirsiEngine.JSONCommandRead) -> MaatKnowledgeProjection? {
        guard read.exitStatus == 0, !read.timedOut, read.launchError == nil else { return nil }
        return try? JSONDecoder().decode(MaatKnowledgeProjection.self, from: read.data)
    }
}

private struct MaatKnowledgeDetailView: View {
    let item: MaatKnowledgeItem
    @State private var detail: MaatKnowledgeItem?
    @State private var resolutionState = "reviewed"
    @State private var resolutionNote = ""
    @State private var confirmResolution = false
    @State private var resolutionSubmission = MaatKnowledgeResolutionSubmission()
    @State private var resolutionReceipt: String?
    @State private var resolutionError: String?
    @State private var sourceHistory: [MaatKnowledgeItem] = []
    @State private var sourceHistoryCursor: String?
    @State private var sourceHistoryTotal = 0
    @State private var sourceHistoryLoading = false
    @State private var sourceHistoryError: String?
    private let historyPageSize = 10
    private let maximumHistoryItems = 100

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Ma'at knowledge")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    Text(item.title).sirsiFont(.title3, weight: .bold)
                    if let classification = item.classification {
                        Text("\(classification) · \(item.resolution ?? "open") · revision \(item.revision ?? 1)")
                            .sirsiFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                    if !item.summary.isEmpty {
                        Text(item.summary)
                            .sirsiFont(.body)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    if let content = detail?.content, !content.isEmpty {
                        Text(content)
                            .sirsiFont(.body)
                            .fixedSize(horizontal: false, vertical: true)
                            .textSelection(.enabled)
                    }
                    if let reason = item.withheldReason ?? detail?.withheldReason {
                        Label("Withheld because this record matched \(reason). The original text is not shown.", systemImage: "lock.fill")
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.orange)
                    }
                    if !item.references.isEmpty {
                        VStack(alignment: .leading, spacing: 7) {
                            Text("Sources")
                                .sirsiFont(.caption, weight: .bold)
                                .foregroundStyle(.secondary)
                            ForEach(item.references) { reference in
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(reference.type).sirsiFont(.caption, weight: .semibold)
                                    Text(reference.value)
                                        .sirsiFont(.subheadline)
                                        .foregroundStyle(.secondary)
                                        .textSelection(.enabled)
                                }
                            }
                        }
                    }
                    if let detail, !detail.origins.isEmpty {
                        VStack(alignment: .leading, spacing: 7) {
                            Text("Retained provenance")
                                .sirsiFont(.caption, weight: .bold)
                                .foregroundStyle(.secondary)
                            ForEach(detail.origins) { origin in
                                VStack(alignment: .leading, spacing: 2) {
                                    Text("\(origin.source) · \(origin.instanceID)").sirsiFont(.caption, weight: .semibold)
                                    if !origin.reference.isEmpty {
                                        Text(origin.reference).sirsiFont(.caption).foregroundStyle(.secondary)
                                    }
                                }
                            }
                        }
                    }
                    if let detail, !detail.annotations.isEmpty {
                        VStack(alignment: .leading, spacing: 7) {
                            Text("Owner review history")
                                .sirsiFont(.caption, weight: .bold)
                                .foregroundStyle(.secondary)
                            ForEach(detail.annotations) { annotation in
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(annotation.state.capitalized).sirsiFont(.caption, weight: .semibold)
                                    Text(annotation.text).sirsiFont(.subheadline).fixedSize(horizontal: false, vertical: true)
                                }
                            }
                        }
                    }
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Retained source history")
                            .sirsiFont(.caption, weight: .bold)
                            .foregroundStyle(.secondary)
                        if sourceHistory.isEmpty {
                            Button("Load version history") { Task { await loadSourceHistory(cursor: nil) } }
                                .buttonStyle(.bordered)
                        }
                        ForEach(Array(sourceHistory.enumerated()), id: \.offset) { entry in
                            let version = entry.element
                            DisclosureGroup("Revision \(version.revision ?? 0) · \(version.title)") {
                                if !version.summary.isEmpty {
                                    Text(version.summary)
                                        .sirsiFont(.subheadline)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                                if let content = version.content, !content.isEmpty {
                                    Text(content)
                                        .sirsiFont(.caption)
                                        .fixedSize(horizontal: false, vertical: true)
                                        .textSelection(.enabled)
                                }
                                ForEach(version.origins) { origin in
                                    Text("\(origin.source) · \(origin.instanceID) · \(origin.reference)")
                                        .sirsiFont(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                        }
                        if let sourceHistoryCursor, sourceHistory.count < maximumHistoryItems {
                            Button("Load older versions") { Task { await loadSourceHistory(cursor: sourceHistoryCursor) } }
                                .buttonStyle(.bordered)
                                .disabled(sourceHistoryLoading)
                        }
                        if sourceHistoryTotal > 0 {
                            Text("Showing \(sourceHistory.count) of \(sourceHistoryTotal) retained versions.")
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if let sourceHistoryError {
                            Label(sourceHistoryError, systemImage: "exclamationmark.triangle")
                                .sirsiFont(.caption)
                                .foregroundStyle(.orange)
                        }
                    }
                    VStack(alignment: .leading, spacing: 9) {
                        Text("Review this knowledge item")
                            .sirsiFont(.headline)
                        Picker("Owner conclusion", selection: $resolutionState) {
                            Text("Reviewed").tag("reviewed")
                            Text("Accepted").tag("accepted")
                            Text("Rejected").tag("rejected")
                        }
                        .pickerStyle(.segmented)
                        .disabled(resolutionSubmission.submissionStarted)
                        TextField("Why this conclusion?", text: $resolutionNote, axis: .vertical)
                            .lineLimit(2...5)
                            .textFieldStyle(.roundedBorder)
                            .disabled(resolutionSubmission.submissionStarted)
                        if let revision = detail?.revision, let digest = detail?.contentDigest, !digest.isEmpty {
                            Text("This review is bound to revision \(revision) · \(digest.prefix(12))…")
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                        } else {
                            Text("Exact version details are unavailable; reopen the item before recording a review.")
                                .sirsiFont(.caption)
                                .foregroundStyle(.orange)
                        }
                        Button("Record owner review") { confirmResolution = true }
                            .buttonStyle(.borderedProminent)
                            .disabled(resolutionNote.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || detail?.revision == nil || detail?.contentDigest?.isEmpty != false || resolutionSubmission.submissionStarted)
                        if let resolutionReceipt {
                            Label(resolutionReceipt, systemImage: "checkmark.seal")
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if let resolutionError {
                            Label(resolutionError, systemImage: "exclamationmark.triangle")
                                .sirsiFont(.caption)
                                .foregroundStyle(.orange)
                        }
                    }
                    .padding(14)
                    .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
                }
                .padding(16)
            }
        }
        .task {
            let raw = await SirsiEngine.run(args: ["maat", "knowledge", "detail", item.id, "--json"], stdin: nil)
            guard let start = raw.firstIndex(of: "{"),
                  let loaded = try? JSONDecoder().decode(MaatKnowledgeItem.self, from: Data(raw[start...].utf8)) else {
                resolutionError = "Ma'at could not load the exact version details. No owner review can be recorded until the item is reopened."
                return
            }
            detail = loaded
            resolutionError = nil
        }
        .confirmationDialog("Record this owner conclusion?", isPresented: $confirmResolution, titleVisibility: .visible) {
            Button("Record \(resolutionState)") { beginResolutionSubmission() }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This saves a sensitivity-filtered owner note in Ma'at's archive. Casebook projection is tracked separately and may remain pending.")
        }
    }

    @MainActor private func beginResolutionSubmission() {
        guard let reviewed = detail,
              let request = MaatKnowledgeResolutionRequest.capture(
                recordID: item.id, revision: reviewed.revision, digest: reviewed.contentDigest,
                state: resolutionState, note: resolutionNote
              ) else {
            resolutionSubmission.finish()
            resolutionError = "The displayed version is unavailable. Reload its exact detail and review it again before recording a decision."
            return
        }
        guard resolutionSubmission.begin() else { return }
        Task { await recordResolution(request) }
    }

    @MainActor private func recordResolution(_ request: MaatKnowledgeResolutionRequest) async {
        defer { resolutionSubmission.finish() }
        resolutionError = nil
        let output = await SirsiEngine.run(
            args: request.arguments,
            stdin: request.note
        )
        guard let start = output.firstIndex(of: "{"),
              let receipt = try? JSONDecoder().decode(MaatKnowledgeResolutionReceipt.self, from: Data(output[start...].utf8)) else {
            resolutionError = "Ma'at could not confirm this exact revision-and-digest review. No newer version was substituted; reopen the current item for a deliberate fresh review."
            return
        }
        resolutionReceipt = "Recorded as \(receipt.state); Casebook projection is \(receipt.casebookProjection)."
        resolutionNote = ""
        let raw = await SirsiEngine.run(args: ["maat", "knowledge", "detail", request.recordID, "--json"], stdin: nil)
        if let start = raw.firstIndex(of: "{") {
            detail = try? JSONDecoder().decode(MaatKnowledgeItem.self, from: Data(raw[start...].utf8))
        }
    }

    @MainActor private func loadSourceHistory(cursor: String?) async {
        sourceHistoryLoading = true
        sourceHistoryError = nil
        var args = ["maat", "knowledge", "history", item.id, "--limit", "\(historyPageSize)"]
        if let cursor, !cursor.isEmpty { args += ["--cursor", cursor] }
        args.append("--json")
        let raw = await SirsiEngine.run(args: args, stdin: nil)
        guard let start = raw.firstIndex(of: "{"),
              let page = try? JSONDecoder().decode(MaatKnowledgeHistoryPage.self, from: Data(raw[start...].utf8)) else {
            sourceHistoryError = "Ma'at could not read this stable history page. Existing retained versions are unchanged."
            sourceHistoryLoading = false
            return
        }
        sourceHistory = Array((sourceHistory + page.items).prefix(maximumHistoryItems))
        sourceHistoryTotal = page.total
        sourceHistoryCursor = sourceHistory.count < maximumHistoryItems ? page.nextCursor : nil
        sourceHistoryLoading = false
    }
}

private struct MaatKnowledgeResolutionReceipt: Decodable {
    let state: String
    let casebookProjection: String
    enum CodingKeys: String, CodingKey {
        case state
        case casebookProjection = "casebook_projection"
    }
}

private struct MaatKnowledgeHistoryPage: Decodable {
    let items: [MaatKnowledgeItem]
    let total: Int
    let nextCursor: String?

    private enum CodingKeys: String, CodingKey {
        case items, total
        case nextCursor = "next_cursor"
    }
}

private struct MaatCaseRow: View {
    let entry: MaatCase

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Circle()
                .fill(entry.priority.tint)
                .frame(width: 8, height: 8)
                .padding(.top, 5)
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text(entry.category.capitalized)
                        .sirsiFont(.headline)
                        .foregroundStyle(.primary)
                    Text(entry.status.capitalized)
                        .sirsiFont(.caption, weight: .semibold)
                        .foregroundStyle(entry.status == "resolved" ? .green : .secondary)
                }
                Text(entry.why.isEmpty ? entry.determination : entry.why)
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
                if !entry.resource.isEmpty {
                    Label(entry.resource, systemImage: "cube.transparent")
                        .sirsiFont(.caption)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
            }
            Spacer(minLength: 6)
            Image(systemName: "chevron.right")
                .sirsiFont(.caption, weight: .semibold)
                .foregroundStyle(.tertiary)
                .padding(.top, 3)
        }
        .padding(.horizontal, 13)
        .padding(.vertical, 11)
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
    }
}

enum MaatFailureMemoryAction: Equatable {
    case initialize
    case reobserve
}

struct MaatFailureMemoryStatus: Decodable {
    let state: String
    let journalReadiness:String?
    let journalIntegrity:MaatJournalIntegrity?
    let storeUUID: String?
    let detail: String?
    let incompleteAttempt: Bool
    let projectionDebt: Bool
    let preparedPreflight:MaatPreparedPreflight?
    let receiptRecoveryError:String?

    enum CodingKeys: String, CodingKey {
        case state, detail
        case journalReadiness = "journal_readiness"
        case journalIntegrity = "journal_integrity"
        case storeUUID = "store_uuid"
        case incompleteAttempt = "incomplete_attempt"
        case projectionDebt = "projection_debt"
        case preparedPreflight = "prepared_preflight"
        case receiptRecoveryError = "receipt_recovery_error"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        state = try values.decode(String.self, forKey: .state)
        journalReadiness = try values.decodeIfPresent(String.self, forKey: .journalReadiness)
        journalIntegrity = try values.decodeIfPresent(MaatJournalIntegrity.self, forKey: .journalIntegrity)
        storeUUID = try values.decodeIfPresent(String.self, forKey: .storeUUID)
        detail = try values.decodeIfPresent(String.self, forKey: .detail)
        incompleteAttempt = try values.decodeIfPresent(Bool.self, forKey: .incompleteAttempt) ?? false
        projectionDebt = try values.decodeIfPresent(Bool.self, forKey: .projectionDebt) ?? false
        preparedPreflight = try values.decodeIfPresent(MaatPreparedPreflight.self,forKey:.preparedPreflight)
        receiptRecoveryError = try values.decodeIfPresent(String.self,forKey:.receiptRecoveryError)
    }
}

struct MaatFirstUseJournalRefusal:Decodable {
    let status:String
    let stage:String
    let journalReadiness:String
    let journalIntegrity:MaatJournalIntegrity

    enum CodingKeys:String,CodingKey {
        case status,stage
        case journalReadiness="journal_readiness"
        case journalIntegrity="journal_integrity"
    }
}

func maatCasebookFailureMemoryArguments(_ action: MaatFailureMemoryAction) -> [String] {
    switch action {
    case .initialize:
        return ["maat", "repair", "liveness-watch", "--initialize-failure-memory", "--confirm", "--json"]
    case .reobserve:
        return ["maat", "repair", "liveness-watch", "--reobserve-incomplete", "--confirm", "--json"]
    }
}

func maatExactCasebookReadArguments(evidence: String, cursor: String? = nil) -> [String] {
	var arguments = ["maat", "casebook", "--json", "--scope", "all", "--evidence", evidence, "--limit", "2"]
	if let cursor, !cursor.isEmpty { arguments += ["--cursor", cursor] }
	return arguments
}

func maatCasebookHistoryArguments(scope: String, query: String = "", cursor: String? = nil) -> [String] {
    var arguments = ["maat", "casebook", "--json", "--scope", scope, "--limit", "50"]
    if let cursor, !cursor.isEmpty { arguments += ["--cursor", cursor] }
    let normalizedQuery = query.trimmingCharacters(in: .whitespacesAndNewlines)
    if !normalizedQuery.isEmpty { arguments.append(normalizedQuery) }
    return arguments
}

private struct MaatCaseDetailView: View {
    @ObservedObject var engine: SirsiEngine
    @ObservedObject var operations: MaatCaseOperationStore
    let entry: MaatCase
    @State private var conclusion = ""
    @State private var confirmAction = false
    @State private var evidenceCopied = false
    @State private var copiedFindingID: String?
    @State private var failureMemoryStatus: MaatFailureMemoryStatus?
    @State private var failureMemoryLoading = false
    @State private var failureMemoryBusy = false
    @State private var failureMemoryMessage: String?
    @State private var pendingFailureMemoryAction: MaatFailureMemoryAction?
    @State private var binaryDriftPreview: MaatBinaryDriftPreview?
    @State private var binaryDriftPreviewLoading = false
    @State private var binaryDriftPreviewError: String?
    @State private var failureMemoryHistory: MaatExactHistoryContinuation?
    @State private var failureMemoryHistoryKey: MaatOperationKey?
    @State private var failureMemoryHistoryAction: MaatFailureMemoryAction?
    @State private var retryReviewBusy = false

    private var operationKey: MaatOperationKey { MaatOperationKey(case: entry) }
    private var operationRecord: MaatOperationRecord? { operations.record(for: operationKey) }
    private var operationInFlight: Bool { operationRecord?.phase.isInFlight == true }
    private var canBeginOperation: Bool {
        guard let phase = operationRecord?.phase else { return true }
        return phase == .retryReady || phase == .historyIncomplete
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Ma'at decision")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    HStack(spacing: 8) {
                        Circle().fill(entry.priority.tint).frame(width: 9, height: 9)
                        Text(entry.category.capitalized)
                            .sirsiFont(.title3, weight: .bold)
                        Spacer()
                        Text(entry.status.capitalized)
                            .sirsiFont(.caption, weight: .semibold)
                            .foregroundStyle(entry.status == "resolved" ? .green : .orange)
                    }
                    detailSection("Assessment", entry.assessed.isEmpty ? entry.why : entry.assessed)
                    detailSection("Determination", entry.determination)
                    if !entry.resource.isEmpty { detailSection("Resource", entry.resource) }
                    if !entry.affected.isEmpty { detailSection("Affected", entry.affected) }
                    if !entry.requester.isEmpty { detailSection("Requested by", entry.requester) }
                    if !entry.evidence.isEmpty { detailSection("Evidence", entry.evidence) }
					if let systemOne = entry.systemOne {
						detailSection("System One gate", "\(systemOne.gate.capitalized) · \(Int(systemOne.confidence * 100))% confidence · feather \(systemOne.featherWeight)/100")
						detailSection("Screen subject", "\(systemOne.subject.kind) \(systemOne.subject.ref) · \(systemOne.subject.headSHA)")
						detailSection("Screen model", systemOne.model.detail)
						if systemOne.contract == MaatSourceImportContract.id && systemOne.schemaVersion == 2 {
							detailSection("Source contract", "\(systemOne.contract) v\(systemOne.schemaVersion) · nonbinding")
							detailSection("Declared floor evidence", systemOne.floorEvidence)
							detailSection("Declared calibration", "\(systemOne.model.calibration) · \(systemOne.model.calibrationID)")
							detailSection("Declared evidence recipe", systemOne.model.evidenceSetDigest)
						}
						systemOneFloor(systemOne.floor)
						systemOneFindings(systemOne.findings, closedActionID: entry.nextAction?.actionID)
						if let escalation = systemOne.escalation {
							detailSection("Required review", escalation.reason)
						}
						if !systemOne.floor.passed {
							let failed = systemOne.floor.checks.filter { !$0.passed }.map(\.name).joined(separator: ", ")
							detailSection("Deterministic floor", failed.isEmpty ? "failed" : "failed: \(failed)")
						}
					}
					if let calibration = entry.systemOneCalibration {
						detailSection("System One calibration", "Local \(calibration.screenGate) → independent \(calibration.frontierGate)")
						detailSection("Screen evidence", calibration.screenEvidence)
						detailSection("Independent evidence", calibration.frontierEvidence)
						Text("This is a completed evidence comparison, not a new authorization or unresolved repair.")
							.sirsiFont(.caption)
							.foregroundStyle(.secondary)
					}
                    if let action = entry.nextAction {
                        detailSection("Next step", action.title)
                        Text(action.detail)
                            .sirsiFont(.subheadline)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                        resolutionSteps(action.steps)
                        if action.requiresConfirmation {
                            Label("Requires explicit confirmation", systemImage: "checkmark.shield")
                                .sirsiFont(.caption, weight: .semibold)
                                .foregroundStyle(gold)
                        }
                    }
                    if !entry.resolution.isEmpty {
                        detailSection("Owner acceptance", entry.resolution)
                        Text("This records an owner conclusion. It does not claim that Pantheon repaired the diagnosed system.")
                            .sirsiFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                    if entry.kind == "failure memory preflight" {
                        MaatRetainedPreflightDetail(model: operations.failureMemoryReceipts, entry: entry)
                        MaatLocalPreventionCollection(model: operations.localPreventions, receiptReads: operations.failureMemoryReceipts, entry: entry) { fresh in
                            AnyView(MaatCaseDetailView(engine: engine, operations: operations, entry: fresh))
                        }
                    }
                    if entry.systemOne?.findings.contains(where: { $0.repairID == "liveness-watch" }) == true {
                        MaatLocalPreventionEntry(model: operations.localPreventions, entry: entry) { fresh in
                            AnyView(MaatCaseDetailView(engine: engine, operations: operations, entry: fresh))
                        }
                    }
                    if entry.nextAction?.kind == "maat_repair", entry.nextAction?.actionID == "liveness-watch" {
                        failureMemorySetup
                    }
                    if let repairOutcome = maatCasebookRepairOutcomePresentation(entry) {
                        detailSection(repairOutcome.title, repairOutcome.evidence)
                        Text(repairOutcome.detail)
                            .sirsiFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                    if !entry.pendingRepairAttemptEvidence.isEmpty {
                        detailSection("Retained updater attempt", entry.pendingRepairAttemptEvidence)
                    }
                    if let action = entry.nextAction, entry.status.lowercased() != "resolved" { resolutionAction(action) }
                }
                .padding(16)
            }
        }
        .task {
            guard entry.nextAction?.actionID == "liveness-watch" else { return }
            await refreshFailureMemoryStatus()
        }
        .confirmationDialog(confirmationTitle, isPresented: $confirmAction, titleVisibility: .visible) {
            Button(confirmationButton) {
                let confirmedNote = conclusion.trimmingCharacters(in: .whitespacesAndNewlines)
                if let action = pendingFailureMemoryAction {
                    Task { await performFailureMemoryAction(action) }
                } else {
                    Task { await performResolution(confirmedNote: confirmedNote) }
                }
            }
            Button("Cancel", role: .cancel) { pendingFailureMemoryAction = nil }
        } message: {
            Text(confirmationMessage)
        }
    }

    private var confirmationTitle: String {
        if let pendingFailureMemoryAction {
            return pendingFailureMemoryAction == .initialize ? "Initialize local failure memory?" : "Re-observe and reconcile the retained outcome?"
        }
        switch entry.nextAction?.kind {
        case "system_one_review" where entry.nextAction?.actionID == "inspect_binary_drift_attempt": return "Inspect the retained updater attempt?"
        case "system_one_review" where entry.nextAction?.actionID == "rescreen_system_one": return "Run a fresh System One screen?"
        case "maat_repair" where entry.nextAction?.actionID == "binary-drift": return "Run this reviewed Homebrew update?"
        case "maat_repair": return "Apply Ma'at's bounded recovery?"
        case "owner_acceptance": return "Record this owner acceptance?"
        case "system_one_floor_recovery": return "Record this recovery review?"
        default: return "Create an owner review?"
        }
    }

    private var confirmationButton: String {
        if let pendingFailureMemoryAction {
            return pendingFailureMemoryAction == .initialize ? "Initialize local failure memory" : "Inspect and re-observe"
        }
        if operationRecord?.phase == .historyIncomplete { return "Continue review and confirm action" }
        switch entry.nextAction?.kind {
        case "system_one_review" where entry.nextAction?.actionID == "inspect_binary_drift_attempt": return "Inspect attempt"
        case "system_one_review" where entry.nextAction?.actionID == "rescreen_system_one": return "Run fresh screen"
        case "maat_repair" where entry.nextAction?.actionID == "binary-drift": return "Run Homebrew updater"
        case "maat_repair": return "Apply bounded recovery"
        case "owner_acceptance": return "Record owner acceptance"
        case "system_one_floor_recovery": return "Record recovery review"
        default: return "Create owner review"
        }
    }

    private var confirmationMessage: String {
        if let pendingFailureMemoryAction {
            return pendingFailureMemoryAction == .initialize
                ? "Create Ma'at's private per-user failure-memory store for genuine first use. No prior incidents are imported, and no liveness-watch repair runs until you review and confirm it separately."
                : "Read back the exact prior liveness-watch target and reconcile its durable outcome into Casebook. Installation will not replay; any later repair requires a fresh review and confirmation."
        }
        if entry.nextAction?.kind == "system_one_review", entry.nextAction?.actionID == "inspect_binary_drift_attempt" {
            return "Ma'at will make a read-only bounded observation for this exact retained attempt. This does not prove attribution or authorize an updater replay."
        }
        if entry.nextAction?.kind == "system_one_review", entry.nextAction?.actionID == "rescreen_system_one" {
            return "Ma'at will perform and retain a fresh local System One observation. It will not replay the prior updater."
        }
        if entry.nextAction?.kind == "maat_repair", entry.nextAction?.actionID == "binary-drift" {
            let targets = binaryDriftPreview?.targets.map(\.path).joined(separator: ", ") ?? "unavailable"
            let updater = binaryDriftPreview?.updaterPath ?? "unavailable"
            let binding = binaryDriftPreview?.binding ?? "unavailable"
            return "Run the reviewed Homebrew executable at \(updater) with `upgrade sirsi-pantheon` for these targets: \(targets), under preview binding \(binding). Ma'at will recheck binary drift and retain the result. A changed preview is refused; Pantheon will not replace or re-sign a CLI directly."
        }
        return entry.nextAction?.kind == "maat_repair"
            ? "Ma'at will re-check the exact managed disabled labels, change only the bounded verified set, re-check the same diagnostic, and retain the outcome. It will not touch unrelated services."
            : entry.nextAction?.kind == "owner_acceptance"
            ? "This records an owner conclusion for the exact retained evidence. It does not change or claim to repair the system."
            : entry.nextAction?.kind == "system_one_floor_recovery"
                ? "This records the evidence-bound recovery review after you complete the stated correction. It does not run a repair; the case remains open until the new evidence is accepted."
                : "This records an evidence-bound owner review for this exact case. It does not change the system. A conclusion must be accepted explicitly later."
    }

    @ViewBuilder private var failureMemorySetup: some View {
        VStack(alignment: .leading, spacing: 9) {
            Label("Local failure memory", systemImage: "externaldrive.badge.shield.checkmark")
                .sirsiFont(.headline)
            MaatPreparedPreflightRecoveryView(model:operations.preparedPreflightRecovery,refresh:{await refreshFailureMemoryStatus()})
            if failureMemoryLoading {
                ProgressView("Inspecting this Mac's local store…")
                    .sirsiFont(.caption)
            } else if let status = failureMemoryStatus {
                switch status.state {
                case "uninitialized":
                    Text("First use is explicit. Initialization creates a private local store and imports no prior incidents.")
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                    Button {
                        pendingFailureMemoryAction = .initialize
                        confirmAction = true
                    } label: {
                        Label("Initialize local failure memory", systemImage: "lock.shield")
                    }
                    .buttonStyle(.bordered)
                    .disabled(failureMemoryBusy)
                case "ready" where status.projectionDebt:
                    Text("A durable liveness-watch outcome is not yet projected into Casebook. Ma'at will re-observe it and reconcile the exact attempt without replaying installation.")
                        .sirsiFont(.caption).foregroundStyle(.orange)
                    Button {
                        pendingFailureMemoryAction = .reobserve
                        confirmAction = true
                    } label: {
                        Label("Re-observe and reconcile outcome", systemImage: "arrow.clockwise.circle")
                    }
                    .buttonStyle(.bordered)
                    .disabled(failureMemoryBusy)
                case "ready" where status.incompleteAttempt:
                    Text("A prior repair attempt is incomplete. Ma'at will re-observe it without replaying installation.")
                        .sirsiFont(.caption).foregroundStyle(.orange)
                    Button {
                        pendingFailureMemoryAction = .reobserve
                        confirmAction = true
                    } label: {
                        Label("Inspect and re-observe incomplete attempt", systemImage: "arrow.clockwise.circle")
                    }
                    .buttonStyle(.bordered)
                    .disabled(failureMemoryBusy)
                case "ready":
                    Text("Private local store is initialized. No incidents were imported during genesis.")
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                default:
                    Text(status.detail ?? "Initialization is incomplete or unverifiable. Ma'at will not reset or adopt this store.")
                        .sirsiFont(.caption).foregroundStyle(.orange)
                    Button {
                        Task { await refreshFailureMemoryStatus() }
                    } label: {
                        Label("Inspect local state again", systemImage: "arrow.clockwise")
                    }
                    .buttonStyle(.bordered)
                    .disabled(failureMemoryBusy)
                }
            } else {
                Text("Ma'at could not read the local initialization state. No repair was attempted.")
                    .sirsiFont(.caption).foregroundStyle(.orange)
                Button {
                    Task { await refreshFailureMemoryStatus() }
                } label: {
                    Label("Inspect local state again", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.bordered)
                .disabled(failureMemoryBusy)
            }
            if failureMemoryBusy { ProgressView("Recording local Ma'at action…").sirsiFont(.caption) }
            if let failureMemoryMessage {
                Text(failureMemoryMessage)
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if failureMemoryHistory != nil {
                Button("Start fresh case review") {
                    failureMemoryHistory = nil
                    failureMemoryHistoryKey = nil
                    failureMemoryHistoryAction = nil
                    failureMemoryMessage = "History progress was discarded. Confirm the local action to start a fresh review."
                }
                .buttonStyle(.bordered)
                .disabled(failureMemoryBusy)
            }
        }
        .padding(12)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    @MainActor private func refreshFailureMemoryStatus() async {
        failureMemoryLoading = true
        defer { failureMemoryLoading = false }
        let read = await SirsiEngine.runJSONRead(args: ["maat", "repair", "liveness-watch", "--inspect-failure-memory", "--json"])
        guard !Task.isCancelled else { return }
        guard read.exitStatus == 0, !read.timedOut, read.launchError == nil,
              let status = try? JSONDecoder().decode(MaatFailureMemoryStatus.self, from: read.data) else {
            failureMemoryStatus = nil
            failureMemoryMessage = read.launchError ?? "Ma'at's local state could not be decoded; no change was made."
            operations.preparedPreflightRecovery.inspectionUnavailable()
            return
        }
        failureMemoryStatus = status
        operations.preparedPreflightRecovery.accept(status)
    }

    @MainActor private func performFailureMemoryAction(_ action: MaatFailureMemoryAction) async {
        pendingFailureMemoryAction = nil
        guard !failureMemoryBusy else { return }
        let reconcilingProjectionDebt = action == .reobserve && failureMemoryStatus?.projectionDebt == true
        failureMemoryBusy = true
        defer { failureMemoryBusy = false }

        let key = operationKey
        let prior = failureMemoryHistoryKey == key && failureMemoryHistoryAction == action ? failureMemoryHistory : nil
        let read = await MaatOperationPreflight.readCompleteExactHistory(evidence: entry.evidence, continuing: prior) { args, timeout in
            await SirsiEngine.runJSONRead(args: args, timeoutSeconds: timeout)
        }
        failureMemoryHistory = read.continuation
        failureMemoryHistoryKey = read.continuation == nil ? nil : key
        failureMemoryHistoryAction = read.continuation == nil ? nil : action
        guard !Task.isCancelled else {
            failureMemoryMessage = "The exact history read was cancelled; no Ma'at action ran. Review the case again before retrying."
            return
        }
        if read.continuation != nil {
            failureMemoryMessage = "The exact history read is incomplete. Confirm the same local action to continue its review, or start fresh. No action has run."
            return
        }
        let projection = MaatOperationPreflight.decodeProjection(from: read.data)
        guard case .ready = MaatOperationPreflight.evaluate(
            original: entry,
            exitStatus: read.exitStatus,
            timedOut: read.timedOut,
            launchError: read.launchError,
            projection: projection
        ) else {
            failureMemoryMessage = "The reviewed Ma'at case changed or could not be re-read. Refresh this case before continuing."
            return
        }
        let args = maatCasebookFailureMemoryArguments(action)
        guard !Task.isCancelled else {
            failureMemoryMessage = "The exact history read was cancelled; no Ma'at action ran. Review the case again before retrying."
            return
        }
        let command = await SirsiEngine.runJSONRead(args: args, timeoutSeconds: 30)
        guard command.exitStatus == 0, !command.timedOut, command.launchError == nil else {
            failureMemoryMessage = command.launchError ?? "Ma'at did not complete the requested local action. Inspect the state before continuing."
            await refreshFailureMemoryStatus()
            return
        }
        failureMemoryMessage = action == .initialize
            ? "Failure memory was initialized without importing incidents. Review this finding again before applying the separate liveness-watch repair."
            : reconcilingProjectionDebt
                ? "The retained outcome was projected into Casebook without installation replay. Review the recorded outcome before any new repair consent."
                : "The incomplete attempt was re-observed without installation replay. Review this finding again before any new repair consent."
        await refreshFailureMemoryStatus()
    }

    @ViewBuilder private func resolutionAction(_ action: MaatCaseNextAction) -> some View {
        let failureMemoryNeedsReview = action.actionID == "liveness-watch" &&
            (failureMemoryStatus?.state != "ready" || failureMemoryStatus?.incompleteAttempt == true || failureMemoryStatus?.projectionDebt == true)
        VStack(alignment: .leading, spacing: 9) {
            if !action.evidence.isEmpty {
                Button {
                    copyToClipboard(action.evidence)
                    evidenceCopied = true
                } label: {
                    Label(evidenceCopied ? "Evidence copied" : "Copy evidence reference", systemImage: evidenceCopied ? "checkmark" : "doc.on.doc")
                }
                .buttonStyle(.bordered)
            }
            if action.kind == "owner_acceptance" {
                TextField("Accepted owner conclusion", text: $conclusion, axis: .vertical)
                    .textFieldStyle(.roundedBorder)
                    .lineLimit(2...4)
            }
            if action.kind == "maat_repair", action.actionID == "binary-drift" {
                binaryDriftRecoveryControls(action)
            } else {
                Button {
                    confirmAction = true
                } label: {
                    Label(operationRecord?.phase == .retryReady ? "Confirm reviewed retry" : confirmationButton, systemImage: action.kind == "maat_repair" ? "wrench.and.screwdriver.fill" : action.kind == "owner_acceptance" ? "checkmark.circle.fill" : "arrow.triangle.branch")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(gold)
                .disabled(operationInFlight || !canBeginOperation || failureMemoryNeedsReview || (action.kind == "owner_acceptance" && conclusion.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
                .accessibilityHint(action.kind == "maat_repair" ? "Runs only Ma'at's named bounded recovery after confirmation, then retains its post-repair evidence." : "Records an evidence-bound owner decision; it does not repair the system.")
            }
            if let receipt = operationRecord?.receipt {
                Label(receipt.summary, systemImage: receipt.succeeded ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(receipt.succeeded ? .green : .orange)
            }
            if action.kind == "maat_repair", action.actionID == "binary-drift", operationInFlight {
                ProgressView("Applying the reviewed updater…")
                    .sirsiFont(.caption)
                Text("This operation stays locked until the owned updater exits and Ma’at records a fresh verification or an explicit failure. The updater is bounded to two minutes before verification.")
                    .sirsiFont(.caption2)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let guidance = operationRecord?.guidance {
                Label(guidance, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
            }
            if operationRecord != nil && !operationInFlight {
                Button {
                    Task { await reviewRetry() }
                } label: {
                    Label(operations.hasHistoryContinuation(for: operationKey) ? "Continue exact history review" : operationRecord?.phase == .retryReady ? "Review again" : "Refresh and review retry", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.bordered)
                .disabled(operationInFlight || retryReviewBusy)
                if operations.hasHistoryContinuation(for: operationKey) {
                    Button("Start fresh history review") { operations.restartHistory(operationKey) }
                        .buttonStyle(.bordered)
                        .disabled(operationInFlight || retryReviewBusy)
                }
            }
        }
        .padding(12)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    @ViewBuilder private func binaryDriftRecoveryControls(_ action: MaatCaseNextAction) -> some View {
        Button {
            Task { await loadBinaryDriftPreview(action) }
        } label: {
            Label(binaryDriftPreviewLoading ? "Refreshing updater preview…" : "Preview supported updater", systemImage: "magnifyingglass")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(.bordered)
        .disabled(operationInFlight || binaryDriftPreviewLoading)
        .accessibilityHint("Reads the current installation method and exact drifted targets. It does not update anything.")

        if let preview = binaryDriftPreview, preview.caseEvidence == action.evidence {
            VStack(alignment: .leading, spacing: 6) {
                Text("Supported updater: \(preview.updater)")
                    .sirsiFont(.subheadline, weight: .semibold)
                Text("Executable: \(preview.updaterPath) · sha256 \(preview.updaterSHA256)")
                    .sirsiFont(.caption, design: .monospaced)
                    .fixedSize(horizontal: false, vertical: true)
                    .textSelection(.enabled)
                ForEach(preview.targets) { target in
                    Text("\(target.path) · \(target.method) · \(target.version) · sha256 \(target.sha256)")
                        .sirsiFont(.caption, design: .monospaced)
                        .fixedSize(horizontal: false, vertical: true)
                        .textSelection(.enabled)
                }
                Text("Preview binding \(preview.binding)")
                    .sirsiFont(.caption2, design: .monospaced)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                Button {
                    confirmAction = true
                } label: {
                    Label("Confirm this exact update", systemImage: "wrench.and.screwdriver.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(gold)
                .disabled(operationInFlight || !canBeginOperation)
                .accessibilityHint("Rechecks this exact preview binding before calling the existing Homebrew updater.")
            }
            .padding(10)
            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.045)))
        }
        if let binaryDriftPreviewError {
            Label(binaryDriftPreviewError, systemImage: "exclamationmark.triangle.fill")
                .sirsiFont(.caption)
                .foregroundStyle(.orange)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    @MainActor private func loadBinaryDriftPreview(_ action: MaatCaseNextAction) async {
        binaryDriftPreviewLoading = true
        binaryDriftPreviewError = nil
        defer { binaryDriftPreviewLoading = false }
        let capture = await SirsiEngine.runCaptured(
            args: maatCasebookBinaryDriftPreviewArguments(caseEvidence: action.evidence),
            timeout: 30
        )
        guard capture.complete, capture.exitCode == 0,
              let preview = MaatBinaryDriftPreview.decode(capture.stdout),
              preview.caseEvidence == action.evidence,
              preview.updater == "brew upgrade sirsi-pantheon",
              preview.updaterPath.hasPrefix("/"), preview.updaterSHA256.count == 64,
              !preview.targets.isEmpty,
              preview.targets.allSatisfy({ !$0.path.isEmpty && $0.method == "homebrew" && $0.sha256.count == 64 }),
              preview.binding.count == 64 else {
            binaryDriftPreview = nil
            if capture.complete {
                let message = capture.result?.summary ?? (capture.stderr.isEmpty
                    ? "Ma'at could not produce a valid Homebrew-only preview. No updater was run."
                    : capture.stderr)
                binaryDriftPreviewError = SirsiEngine.stripANSI(message)
            } else {
                let message = capture.issue ?? "Ma'at's updater preview was incomplete. No updater was run."
                binaryDriftPreviewError = SirsiEngine.stripANSI(message)
            }
            return
        }
        binaryDriftPreview = preview
    }

    @ViewBuilder private func resolutionSteps(_ steps: [MaatResolutionStep]) -> some View {
        if !steps.isEmpty {
            VStack(alignment: .leading, spacing: 8) {
                Text("Resolution path")
                    .sirsiFont(.caption, weight: .bold)
                    .foregroundStyle(.secondary)
                ForEach(steps) { step in
                    HStack(alignment: .top, spacing: 9) {
                        Text("\(step.level)")
                            .sirsiFont(.caption, weight: .bold)
                            .foregroundStyle(.black)
                            .frame(width: 20, height: 20)
                            .background(Circle().fill(gold))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(step.title)
                                .sirsiFont(.subheadline, weight: .semibold)
                            Text(step.detail)
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                            if step.requiresConfirmation {
                                Text("Requires explicit confirmation")
                                    .sirsiFont(.caption2, weight: .semibold)
                                    .foregroundStyle(.orange)
                            }
                        }
                    }
                }
            }
            .padding(11)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.045)))
        }
    }

    @MainActor private func performResolution(confirmedNote: String) async {
        guard let action = entry.nextAction else { return }
        guard action.kind != "owner_acceptance" || !confirmedNote.isEmpty else { return }
        let key = operationKey
        guard operations.begin(key) else { return }
        guard let generation = operations.record(for: key)?.generation else { return }

        let read = await MaatOperationPreflight.readCompleteExactHistory(evidence: entry.evidence,
            continuing: operations.historyContinuation(for: key, generation: generation)) { args, timeout in
            await SirsiEngine.runJSONRead(args: args, timeoutSeconds: timeout)
        }
        guard operations.record(for: key)?.generation == generation else { return }
        guard !Task.isCancelled else {
            operations.rejectPreflight(key, guidance: "The exact history read was cancelled. No Ma'at action ran; review again before retrying.")
            return
        }
        if let continuation = read.continuation {
            operations.pauseHistory(key, generation: generation, continuation: continuation)
            return
        }
        let projection = MaatOperationPreflight.decodeProjection(from: read.data)
        switch MaatOperationPreflight.evaluate(
            original: entry,
            exitStatus: read.exitStatus,
            timedOut: read.timedOut,
            launchError: read.launchError,
            projection: projection
        ) {
        case .rejected(let guidance):
            operations.rejectPreflight(key, guidance: guidance)
            return
        case .ready:
            break
        }

        operations.markRunning(key)
        let result: CommandResult?
        if let args = maatCasebookSystemOneReviewArguments(kind: action.kind, actionID: action.actionID, evidence: action.evidence) {
            result = await SirsiEngine.runResult(args: args)
            let succeeded = result?.ok == true
            let missing = action.actionID == "inspect_binary_drift_attempt"
                ? "Ma'at could not read the retained attempt. Refresh and review the case."
                : "Ma'at could not complete the fresh screen. Refresh and review the case."
            operations.complete(key, receipt: MaatOperationReceipt(summary: result?.summary ?? missing, succeeded: succeeded))
            return
        }
        if action.kind == "maat_repair" {
            if action.actionID == "binary-drift" {
                guard let preview = binaryDriftPreview, preview.caseEvidence == action.evidence else {
                    operations.rejectPreflight(key, guidance: "The updater preview is missing or belongs to another case. Refresh and review a new preview before confirming.")
                    return
                }
                let capture = await SirsiEngine.runCaptured(
                    args: maatCasebookBinaryDriftRepairArguments(caseEvidence: action.evidence, previewBinding: preview.binding),
                    timeout: 180
                )
                let command = capture.result
                let succeeded = capture.complete && capture.exitCode == 0 && command?.status == "resolved"
                operations.complete(key, receipt: MaatOperationReceipt(
                    summary: command?.summary ?? capture.displayText,
                    succeeded: succeeded
                ))
                binaryDriftPreview = nil
                return
            }
            guard let args = maatCasebookRepairArguments(actionID: action.actionID) else {
                operations.rejectPreflight(key, guidance: "Ma'at refused an unknown repair reference. Refresh and review the case before trying again.")
                return
            }
            guard !Task.isCancelled else {
                operations.rejectPreflight(key, guidance: "The exact history read was cancelled. No Ma'at action ran; review again before retrying.")
                return
            }
            result = await SirsiEngine.runResult(args: args)
        } else if action.kind == "owner_acceptance" {
            guard !Task.isCancelled else {
                operations.rejectPreflight(key, guidance: "The exact history read was cancelled. No Ma'at action ran; review again before retrying.")
                return
            }
            result = await SirsiEngine.runResult(args: ["maat", "accept-resolution", "--evidence", action.evidence, "--note", confirmedNote, "--confirm"])
        } else {
            let check = entry.resource.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? entry.kind : entry.resource
            guard !Task.isCancelled else {
                operations.rejectPreflight(key, guidance: "The exact history read was cancelled. No Ma'at action ran; review again before retrying.")
                return
            }
            result = await SirsiEngine.runResult(args: ["maat", "record-resolution", "--check", check, "--message", entry.why.isEmpty ? entry.determination : entry.why, "--origin-evidence", entry.evidence, "--confirm"])
        }
        let succeeded = result?.ok == true
        let summary = result?.summary
            ?? "Ma'at did not return a readable result. Refresh and review before attempting this action again."
        operations.complete(key, receipt: MaatOperationReceipt(summary: summary, succeeded: succeeded))
    }

    @MainActor private func reviewRetry() async {
        let key = operationKey
        guard !retryReviewBusy, let prior = operations.record(for: key), !prior.phase.isInFlight else { return }
        retryReviewBusy = true
        defer { retryReviewBusy = false }
        let read = await MaatOperationPreflight.readCompleteExactHistory(evidence: entry.evidence,
            continuing: operations.historyContinuation(for: key, generation: prior.generation)) { args, timeout in
            await SirsiEngine.runJSONRead(args: args, timeoutSeconds: timeout)
        }
        guard operations.record(for: key)?.generation == prior.generation else { return }
        guard !Task.isCancelled else {
            operations.rejectRetryReview(key, reviewedGeneration: prior.generation, guidance: "The exact history read was cancelled. Review the case again before retrying.")
            return
        }
        if let continuation = read.continuation {
            operations.pauseHistory(key, generation: prior.generation, continuation: continuation, retryReview: true)
            return
        }
        let projection = MaatOperationPreflight.decodeProjection(from: read.data)
        switch MaatOperationPreflight.evaluate(
            original: entry,
            exitStatus: read.exitStatus,
            timedOut: read.timedOut,
            launchError: read.launchError,
            projection: projection
        ) {
        case .ready:
            operations.authorizeReviewedRetry(key, reviewedGeneration: prior.generation)
        case .rejected(let guidance):
            operations.rejectRetryReview(key, reviewedGeneration: prior.generation, guidance: guidance)
        }
    }

    private func detailSection(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title)
                .sirsiFont(.caption, weight: .bold)
                .foregroundStyle(.secondary)
            Text(value)
                .sirsiFont(.body)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    // A qualified producer may describe a bounded remediation in a System One
    // finding. Treat it as evidence and an operator aid, never as executable
    // input: the app shows and copies it, while the exact owner-review path
    // remains responsible for acknowledgement or a separately safe repair.
    @ViewBuilder private func systemOneFindings(_ findings: [MaatSystemOneFinding], closedActionID: String?) -> some View {
        if !findings.isEmpty {
            VStack(alignment: .leading, spacing: 9) {
                Text(findings.count == 1 ? "Screen finding" : "Screen findings")
                    .sirsiFont(.caption, weight: .bold)
                    .foregroundStyle(.secondary)
                ForEach(maatSystemOneDisplayFindings(findings)) { finding in
                    VStack(alignment: .leading, spacing: 6) {
                        HStack(alignment: .firstTextBaseline, spacing: 7) {
                            Text(finding.severity.capitalized)
                                .sirsiFont(.caption, weight: .semibold)
                                .foregroundStyle(findingTint(finding.severity))
                            Text(finding.category)
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                            Spacer(minLength: 6)
                            if !finding.location.isEmpty {
                                Text(finding.location)
                                    .sirsiFont(.caption2, design: .monospaced)
                                    .foregroundStyle(.tertiary)
                                    .lineLimit(1)
                            }
                        }
                        Text(finding.claim)
                            .sirsiFont(.body)
                            .fixedSize(horizontal: false, vertical: true)
                        if !finding.fixHint.isEmpty, closedActionID != "binary-drift" {
                            Text("PRESCRIBED NEXT STEP")
                                .sirsiFont(.caption2, weight: .semibold)
                                .foregroundStyle(.secondary)
                            Text(finding.fixHint)
                                .sirsiFont(.callout)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                            Button {
                                copyToClipboard(finding.fixHint)
                                copiedFindingID = finding.id
                            } label: {
                                Label(copiedFindingID == finding.id ? "Next step copied" : "Copy next step", systemImage: copiedFindingID == finding.id ? "checkmark" : "doc.on.doc")
                            }
                            .buttonStyle(.bordered)
                            .accessibilityHint("Copies the producer-supplied recovery step for review; it does not run it.")
                        } else if closedActionID == "binary-drift" {
                            Text("The retained suggestion is not executed; Ma'at's typed updater preview below governs this repair.")
                                .sirsiFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        if !finding.evidence.isEmpty {
                            Text("Evidence: \(finding.evidence)")
                                .sirsiFont(.caption2, design: .monospaced)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                        }
                        if let verdict = entry.systemOne,
                           maatHasLocalDiagnosticCustody(verdict, finding: finding, localHost: maatLocalDiagnosticHost() ?? ""),
                           !maatUsesDedicatedDiagnosticCustody(finding) {
                            NavLink {
                                MaatDiagnosticResolutionView(engine: engine, origin: entry, finding: finding)
                            } label: {
                                Label("Review current resolution in Pantheon", systemImage: "wrench.and.screwdriver")
                            }
                            .buttonStyle(.bordered)
                            .accessibilityHint("Reads the current local diagnostic before opening Pantheon's existing controlled resolver. The retained suggestion is never executed.")
                        }
                    }
                    .padding(11)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.045)))
                }
            }
        }
    }

    private func findingTint(_ severity: String) -> Color {
        switch severity.lowercased() {
        case "block", "critical", "error": return .red
        case "changes", "warn", "warning": return .orange
        default: return .secondary
        }
    }

    @ViewBuilder private func systemOneFloor(_ floor: MaatSystemOneFloor) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("Deterministic floor")
                .sirsiFont(.caption, weight: .bold)
                .foregroundStyle(.secondary)
            Label(floor.passed ? "All required floor checks passed" : "One or more required floor checks failed", systemImage: floor.passed ? "checkmark.seal.fill" : "xmark.seal.fill")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(floor.passed ? .green : .red)
            ForEach(Array(floor.checks.enumerated()), id: \.offset) { _, check in
                VStack(alignment: .leading, spacing: 2) {
                    Label(check.name, systemImage: check.passed ? "checkmark.circle.fill" : "xmark.octagon.fill")
                        .sirsiFont(.caption, weight: .semibold)
                        .foregroundStyle(check.passed ? .green : .red)
                    if !check.detail.isEmpty {
                        Text(check.detail)
                            .sirsiFont(.caption)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
        }
        .padding(11)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.045)))
    }
}

// Casebook executes only repair identifiers which are closed, native Ma'at
// operations. Keeping this mapping here — rather than accepting producer text
// or a command string from evidence — lets a System One screen offer a real
// recovery without becoming a shell-launch surface.
func maatCasebookRepairArguments(actionID: String) -> [String]? {
    switch actionID {
    case "launchd-disabled":
        return ["maat", "repair", "launchd-disabled", "--confirm"]
    case "liveness-watch":
        return ["maat", "repair", "liveness-watch", "--confirm"]
    default:
        return nil
    }
}

func maatCasebookBinaryDriftPreviewArguments(caseEvidence: String) -> [String] {
    ["maat", "repair", "binary-drift", "--case-evidence", caseEvidence, "--json"]
}

func maatCasebookBinaryDriftRepairArguments(caseEvidence: String, previewBinding: String) -> [String] {
    ["maat", "repair", "binary-drift", "--case-evidence", caseEvidence,
     "--preview-binding", previewBinding, "--confirm", "--json"]
}

func maatCasebookInspectBinaryDriftAttemptArguments(caseEvidence: String) -> [String] {
    ["maat", "repair", "binary-drift", "--inspect-attempt", "--case-evidence", caseEvidence, "--json"]
}

func maatCasebookSystemOneReviewArguments(kind: String, actionID: String, evidence: String) -> [String]? {
    guard kind == "system_one_review" else { return nil }
    switch actionID {
    case "inspect_binary_drift_attempt":
        return maatCasebookInspectBinaryDriftAttemptArguments(caseEvidence: evidence)
    case "rescreen_system_one":
        return maatCasebookFreshSystemOneArguments()
    default:
        return nil
    }
}

func maatCasebookFreshSystemOneArguments() -> [String] {
    ["maat", "triage", "--confirm", "--json"]
}

struct MaatRepairOutcomePresentation: Equatable {
    let title: String
    let evidence: String
    let detail: String
}

func maatCasebookRepairOutcomePresentation(_ entry: MaatCase) -> MaatRepairOutcomePresentation? {
    if !entry.verifiedRepairEvidence.isEmpty {
        return MaatRepairOutcomePresentation(
            title: "Verified binary repair receipt",
            evidence: entry.verifiedRepairEvidence,
            detail: "The reviewed binary-drift updater and fresh binary observation were verified. This does not itself resolve other findings or failed floor checks."
        )
    }
    if !entry.repairOutcomeEvidence.isEmpty {
        return MaatRepairOutcomePresentation(
            title: "Unverified repair outcome evidence",
            evidence: entry.repairOutcomeEvidence,
            detail: "This retained receipt records an unsuccessful or unverified updater attempt; it does not prove the binary was repaired or the system is clear."
        )
    }
    return nil
}

struct MaatBinaryDriftPreview: Decodable, Equatable {
    let caseEvidence: String
    let updater: String
    let updaterPath: String
    let updaterSHA256: String
    let selfPath: String
    let selfVersion: String
    let selfSHA256: String
    let targets: [MaatBinaryDriftTarget]
    let binding: String

    enum CodingKeys: String, CodingKey {
        case updater, targets, binding
        case caseEvidence = "case_evidence"
        case updaterPath = "updater_path"
        case updaterSHA256 = "updater_sha256"
        case selfPath = "self_path"
        case selfVersion = "self_version"
        case selfSHA256 = "self_sha256"
    }

    static func decode(_ raw: String) -> MaatBinaryDriftPreview? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatBinaryDriftPreview.self, from: Data(raw[start...].utf8))
    }
}

struct MaatBinaryDriftTarget: Decodable, Equatable, Identifiable {
    let path: String
    let method: String
    let version: String
    let sha256: String
    var id: String { path }
}

// MaatJournalRepairView owns the one confirmation-gated data recovery action
// for a damaged local Casebook projection. It never asks the operator to
// assemble a command or locate a file manually: the underlying engine
// preserves the original journal, verifies the active projection, and returns
// the exact retained evidence path.
struct MaatJournalRepairView: View {
    @ObservedObject var engine: SirsiEngine
    let integrity: MaatJournalIntegrity
    var repairCompleted:()async->Void = {}
    @State private var confirmRepair = false
    @State private var repairing = false
    @State private var result: MaatJournalRepairResult?
    @State private var error: String?

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Repair Ma'at casebook")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    Label("Preserve and recover the casebook", systemImage: "shield.lefthalf.filled")
                        .sirsiFont(.title3, weight: .bold)
                    Text("Ma'at found \(integrity.countDescription) \(integrity.recordNoun) that cannot satisfy the current local journal contract. This repair does not invent or overwrite those decisions. It first preserves the full original journal, then rebuilds the active Casebook only from records that pass strict verification.")
                        .sirsiFont(.body)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    if !integrity.issues.isEmpty {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("Affected records")
                                .sirsiFont(.headline)
                            ForEach(integrity.issues) { issue in
                                Text("Line \(issue.line) · \(issue.reason) · \(issue.digest)")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.secondary)
                                    .textSelection(.enabled)
                            }
                            if integrity.invalidCount > integrity.issues.count {
                                Text("\(integrity.invalidCount - integrity.issues.count) additional record\(integrity.invalidCount - integrity.issues.count == 1 ? "" : "s") are retained without being expanded here.")
                                    .sirsiFont(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.05)))
                    }
                    if let result {
                        VStack(alignment: .leading, spacing: 7) {
                            Label("Casebook recovered", systemImage: "checkmark.seal.fill")
                                .sirsiFont(.headline)
                                .foregroundStyle(.green)
                            Text("Preserved original: \(result.repair.backupPath)")
                                .sirsiFont(.caption)
                                .textSelection(.enabled)
                            Text("Original SHA256: \(result.repair.originalDigest)")
                                .sirsiFont(.caption)
                                .textSelection(.enabled)
                            Text("Removed \(result.repair.removedCount) invalid record\(result.repair.removedCount == 1 ? "" : "s") from the active projection; \(result.repair.retainedCount) verified record\(result.repair.retainedCount == 1 ? "" : "s") remain.")
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.secondary)
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Color.green.opacity(0.10)))
                    } else if let error {
                        VStack(alignment: .leading, spacing: 6) {
                            Label("Recovery did not complete", systemImage: "exclamationmark.triangle.fill")
                                .sirsiFont(.headline)
                                .foregroundStyle(.orange)
                            Text(error)
                                .sirsiFont(.subheadline)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Color.orange.opacity(0.10)))
                    }
                    Button {
                        confirmRepair = true
                    } label: {
                        if repairing {
                            ProgressView().controlSize(.small)
                            Text("Preserving and verifying…")
                        } else {
                            Label("Preserve and repair casebook", systemImage: "wrench.and.screwdriver")
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(gold)
                    .disabled(repairing || result != nil)
                }
                .padding(20)
            }
        }
        .confirmationDialog("Preserve and repair the Ma'at casebook?", isPresented: $confirmRepair, titleVisibility: .visible) {
            Button("Preserve original and repair", role: .destructive) {
                Task { await repair() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Pantheon will save the original local journal before rebuilding only the active Casebook projection from strictly verified records.")
        }
    }

    @MainActor private func repair() async {
        repairing = true
        error = nil
        let raw = await SirsiEngine.run(args: ["maat", "casebook", "repair", "--confirm", "--json"], stdin: nil)
        if let decoded = MaatJournalRepairResult.decode(raw), decoded.status == "ok" {
            result = decoded
            await repairCompleted()
        } else {
            error = SirsiEngine.firstMeaningful(raw)
        }
        repairing = false
    }
}

struct MaatJournalRepairResult: Decodable {
    let status: String
    let summary: String
    let repair: Repair

    struct Repair: Decodable {
        let backupPath: String
        let originalDigest: String
        let removedCount: Int
        let retainedCount: Int

        enum CodingKeys: String, CodingKey {
            case backupPath = "backup_path"
            case originalDigest = "original_digest"
            case removedCount = "removed_count"
            case retainedCount = "retained_count"
        }
    }

    static func decode(_ raw: String) -> MaatJournalRepairResult? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatJournalRepairResult.self, from: Data(raw[start...].utf8))
    }
}

// This card deliberately appears beside healthy Casebook content instead of
// replacing it with a generic failure state. Ma'at identifies the damaged
// records without rendering their raw contents and routes the operator to the
// confirmation-gated preservation repair.
struct MaatJournalIntegrityCard: View {
    @ObservedObject var engine: SirsiEngine
    let integrity: MaatJournalIntegrity
    var repairCompleted:()async->Void = {}

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            Label("Casebook integrity needs resolution", systemImage: "exclamationmark.shield.fill")
                .sirsiFont(.headline)
                .foregroundStyle(.orange)
            Text("\(integrity.countDescription.capitalized) legacy or damaged decision \(integrity.recordNoun) could not satisfy Ma'at’s journal contract. Valid cases remain visible. Pantheon can preserve the original journal, remove only invalid records from the active view, and verify the recovered casebook.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            NavLink {
                MaatJournalRepairView(engine: engine, integrity: integrity,repairCompleted:repairCompleted)
            } label: {
                Label("Review and preserve invalid records", systemImage: "wrench.and.screwdriver")
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.orange.opacity(0.10)))
    }
}

enum MaatHistoryEmptyState: Equatable {
    case records, incomplete, olderPage, invalidHistory, afterCursor, empty, unavailable
}

enum MaatHistoryRefreshFailure: Equatable {
    case cancelled
    case timedOut
    case unavailable
    case commandFailed
    case malformedResponse
    case invalidResponse
    case staleSnapshot

    var message: String {
        switch self {
        case .cancelled:
            return "History refresh was cancelled; the previous page remains visible. Retry when ready."
        case .timedOut:
            return "History refresh timed out; the previous page remains visible. Retry when the local read can finish."
        case .unavailable:
            return "The local history command could not be completed; the previous page remains visible. Retry the read."
        case .commandFailed:
            return "The local history command returned an unclassified error; the previous page remains visible. Retry or refresh."
        case .malformedResponse:
            return "The local history response was malformed; the previous page remains visible. Retry the read."
        case .invalidResponse:
            return "The local history response did not match the requested scope or continuation; the previous page remains visible. Refresh to start a new snapshot."
        case .staleSnapshot:
            return "The saved history cursor belongs to an older snapshot. The previous page remains visible; restart from the latest snapshot."
        }
    }
}

struct MaatCasebookProjection: Decodable {
    let cases: [MaatCase]
    let summary: MaatCasebookSummary
    let journalIntegrity: MaatJournalIntegrity
    let history: MaatHistoryWindow?

    var historyEmptyState: MaatHistoryEmptyState {
        guard cases.isEmpty else { return .records }
        guard let history else { return .unavailable }
        if history.coverage == "partial" {
            return history.nextCursor?.isEmpty == false ? .incomplete : .unavailable
        }
        guard history.coverage == "complete" else { return .unavailable }
        if history.hasMore { return history.nextCursor?.isEmpty == false ? .olderPage : .unavailable }
        if journalIntegrity.invalidCount > 0 { return .invalidHistory }
        if history.countScope == "after-cursor" { return .afterCursor }
        if history.countScope == "query", history.hasMoreKnown, history.totalComplete,
           history.candidateMatchesComplete, history.candidateMatches == 0,
           history.returned == 0, history.total == 0 {
            return .empty
        }
        return .unavailable
    }

    enum CodingKeys: String, CodingKey {
        case cases
        case summary
        case journalIntegrity = "journal_integrity"
        case history
    }

    init(cases: [MaatCase], summary: MaatCasebookSummary, journalIntegrity: MaatJournalIntegrity = .clean, history: MaatHistoryWindow? = nil) {
        self.cases = cases
        self.summary = summary
        self.journalIntegrity = journalIntegrity
        self.history = history
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        cases = try values.decode([MaatCase].self, forKey: .cases)
        summary = try values.decode(MaatCasebookSummary.self, forKey: .summary)
        journalIntegrity = try values.decodeIfPresent(MaatJournalIntegrity.self, forKey: .journalIntegrity) ?? .clean
        history = try values.decodeIfPresent(MaatHistoryWindow.self, forKey: .history)
    }

    // Snapshot mode must render a complete, deterministic operator state even
    // when a developer machine's installed CLI predates `maat casebook`. This
    // is snapshot-only UI evidence; the live screen always fetches the local
    // decision journal through SirsiEngine.
    static let snapshotPreview = MaatCasebookProjection(
        cases: [
            MaatCase(id: "snapshot-urgent", time: "", kind: "assessment report", category: "assessment", status: "open", priority: .urgent, requester: "sirsi maat audit", resource: "release", affected: "Pantheon", determination: "fail", assessed: "Release evidence is incomplete", why: "A release gate needs an evidence-backed decision.", evidence: "receipt:example"),
            MaatCase(id: "snapshot-high", time: "", kind: "reservation", category: "allocation", status: "open", priority: .high, requester: "router", resource: "shared host", affected: "Pantheon", determination: "pending", assessed: "Host work is serialized", why: "A shared-host reservation is still active.", evidence: "reservation:example"),
            MaatCase(id: "snapshot-resolved", time: "", kind: "assessment", category: "governance", status: "resolved", priority: .normal, requester: "sirsi maat audit", resource: "casebook", affected: "Pantheon", determination: "passed", assessed: "Local evidence view is available", why: "The decision journal is ready to inspect.", evidence: "casebook:example"),
        ],
        summary: MaatCasebookSummary(total: 3, open: 2, urgent: 1, high: 1, resolved: 1)
    )
}

struct MaatHistoryWindow: Decodable {
    let scope: String
    let queryID: String
    let snapshot: String
    let returned: Int
    let candidateMatches: Int
    let countScope: String
    let evidenceMatches: Int?
    let evidenceMatchesComplete: Bool?
    let candidateMatchesComplete: Bool
    let pageSize: Int
    let coverage: String
    let hasMore: Bool
    let hasMoreKnown: Bool
    let nextCursor: String?
    let totalComplete: Bool
    let total: Int?
    let scannedRecords: Int?
    let scannedBytes: Int64?

    var boundedProgressText: String? {
        guard coverage == "partial" else { return nil }
        guard let scannedRecords, scannedRecords >= 0, let scannedBytes, scannedBytes >= 0 else {
            return "This history scan is incomplete; no whole-history result is available yet."
        }
        return "This bounded read scanned \(scannedRecords) journal rows (\(scannedBytes) bytes). Results remain incomplete."
    }

    enum CodingKeys: String, CodingKey {
        case scope, snapshot, returned, coverage, total
        case queryID = "query_id"
        case candidateMatches = "candidate_matches"
        case countScope = "count_scope"
        case evidenceMatches = "evidence_matches"
        case evidenceMatchesComplete = "evidence_matches_complete"
        case candidateMatchesComplete = "candidate_matches_complete"
        case pageSize = "page_size"
        case hasMore = "has_more"
        case hasMoreKnown = "has_more_known"
        case nextCursor = "next_cursor"
        case totalComplete = "total_complete"
        case scannedRecords = "scanned_records"
        case scannedBytes = "scanned_bytes"
    }
}

struct MaatHistoryRefreshState {
    private(set) var projection: MaatCasebookProjection?
    private(set) var error: String?
    private(set) var failure: MaatHistoryRefreshFailure?
    private(set) var query: String
    private(set) var loading: Bool
    private(set) var continuationPaused: Bool
    private(set) var restartRequired: Bool
    private var pending: MaatHistoryRefreshRequest?

    init(projection: MaatCasebookProjection?, error: String? = nil, query: String = "") {
        self.projection = projection
        self.error = error
        failure = nil
        self.query = query
        loading = projection == nil
        continuationPaused = false
        restartRequired = false
    }

    // A write refresh must not replace an in-flight newly selected filter with
    // the older committed query. Cursor reuse still requires a fresh snapshot.
    var refreshQuery: String { pending?.query ?? query }

    mutating func begin(scope: String, query requestedQuery: String? = nil, cursor: String? = nil) -> MaatHistoryRefreshRequest? {
        let nextQuery = (requestedQuery ?? query).trimmingCharacters(in: .whitespacesAndNewlines)
        let nextCursor = cursor.flatMap { $0.isEmpty ? nil : $0 }
        let history = projection?.history
        if let nextCursor {
            guard !restartRequired, history?.scope == scope, history?.nextCursor == nextCursor, nextQuery == query,
                  history?.queryID.isEmpty == false, history?.snapshot.isEmpty == false else {
                pending = nil
                loading = false
                failure = .invalidResponse
                error = MaatHistoryRefreshFailure.invalidResponse.message
                continuationPaused = false
                return nil
            }
        }
        if failure == .cancelled {
            failure = nil
            error = nil
        }
        continuationPaused = false
        let request = MaatHistoryRefreshRequest(scope: scope, query: nextQuery, cursor: nextCursor,
            queryID: nextCursor == nil ? nil : history?.queryID, snapshot: nextCursor == nil ? nil : history?.snapshot)
        pending = request
        loading = true
        return request
    }

    @discardableResult
    mutating func complete(
        _ read: SirsiEngine.JSONCommandRead,
        for request: MaatHistoryRefreshRequest,
        cancelled: Bool,
        keepLoading: Bool = false,
        seenCursors: Set<String> = []
    ) -> Bool {
        guard isCurrent(request) else { return false }
        pending = nil
        loading = false
        if cancelled {
            setFailure(.cancelled)
            return true
        }
        if read.timedOut {
            setFailure(.timedOut)
            return true
        }
        if read.launchError != nil || read.exitStatus == nil {
            setFailure(.unavailable)
            return true
        }
        if let exitStatus = read.exitStatus, exitStatus != 0 {
            setFailure(request.cursor != nil && Self.isExactStaleSnapshotFailure(read) ? .staleSnapshot : .commandFailed)
            return true
        }
        guard let next = MaatCasebookView.decode(read) else {
            setFailure(.malformedResponse)
            return true
        }
        guard let history = next.history, history.scope == request.scope else {
            setFailure(.invalidResponse)
            return true
        }
        guard !history.queryID.isEmpty, !history.snapshot.isEmpty else {
            setFailure(.invalidResponse)
            return true
        }
        if request.cursor != nil {
            guard history.queryID == request.queryID, history.snapshot == request.snapshot else {
                setFailure(.staleSnapshot)
                return true
            }
        }
        if history.coverage == "partial" {
            let validCountScope = history.countScope == "scanned-so-far" ||
                history.countScope == "after-cursor-scanned-so-far"
            guard validCountScope, let nextCursor = history.nextCursor, !nextCursor.isEmpty,
                  nextCursor != request.cursor, !seenCursors.contains(nextCursor) else {
                setFailure(.invalidResponse)
                return true
            }
        }
        projection = next
        query = request.query
        error = nil
        failure = nil
        restartRequired = false
        continuationPaused = false
        loading = keepLoading && history.coverage == "partial"
        return true
    }

    mutating func pauseContinuation(failure: MaatHistoryRefreshFailure? = nil) {
        pending = nil
        loading = false
        let hasAcceptedCursor = projection?.history?.coverage == "partial" &&
            projection?.history?.nextCursor?.isEmpty == false && !restartRequired
        continuationPaused = hasAcceptedCursor
        if hasAcceptedCursor {
            error = nil
            self.failure = nil
        } else if let failure {
            setFailure(failure)
        } else {
            error = nil
            self.failure = nil
        }
    }

    mutating func rejectContinuationForRestart() {
        pending = nil
        loading = false
        continuationPaused = false
        setFailure(.invalidResponse)
    }

    func isCurrent(_ request: MaatHistoryRefreshRequest) -> Bool { pending == request }

    private mutating func setFailure(_ failure: MaatHistoryRefreshFailure) {
        self.failure = failure
        error = failure.message
        continuationPaused = false
        if failure == .invalidResponse || failure == .staleSnapshot {
            restartRequired = true
        }
    }

    private static func isExactStaleSnapshotFailure(_ read: SirsiEngine.JSONCommandRead) -> Bool {
        guard read.exitStatus == 1, !read.timedOut, read.launchError == nil, read.data.isEmpty,
              let stderr = String(data: read.stderrData, encoding: .utf8) else { return false }
        let message = "maat decision history: snapshot changed; refresh the query"
        let normalized = SirsiEngine.stripANSI(stderr).trimmingCharacters(in: .whitespacesAndNewlines)
        return normalized == message || normalized == "𓁵 \(message)"
    }
}

struct MaatHistoryRefreshRequest: Equatable, Sendable {
    private let generation = UUID()
    let scope: String
    let query: String
    let cursor: String?
    let queryID: String?
    let snapshot: String?

    fileprivate init(scope: String, query: String, cursor: String?, queryID: String?, snapshot: String?) {
        self.scope = scope
        self.query = query
        self.cursor = cursor
        self.queryID = queryID
        self.snapshot = snapshot
    }

    var arguments: [String] { maatCasebookHistoryArguments(scope: scope, query: query, cursor: cursor) }
}

struct MaatKnownFailureProposal: Decodable, Identifiable {
    let schema: String
    let id: String
    let title: String?
    let signature: String
    let cause: String
    let status: String
    let createdAtUTC: String
    let catalogSHA256: String
    let catalogRelation: String?

    enum CodingKeys: String, CodingKey {
        case schema, id, title, signature, cause, status
        case createdAtUTC = "created_at_utc"
        case catalogSHA256 = "catalog_sha256"
        case catalogRelation = "catalog_relation"
    }

    var titleOrID: String {
        let normalized = title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return normalized.isEmpty ? id : normalized
    }

    var catalogRelationLabel: String {
        switch catalogRelation {
        case .some("current"): "Recorded against the current known-failure catalog"
        case .some("historical"): "Recorded against a prior catalog · review against current catalog"
        default: "Catalog generation unavailable"
        }
    }
}

struct MaatJournalIntegrity: Decodable {
    let invalidCount: Int
    let countIsLowerBound:Bool
    let issues: [MaatJournalIssue]

    enum CodingKeys: String, CodingKey {
        case invalidCount = "invalid_count"
        case countIsLowerBound = "count_is_lower_bound"
        case issues
    }

    static let clean = MaatJournalIntegrity(invalidCount: 0, issues: [],countIsLowerBound:false)
    var countDescription:String { countIsLowerBound ? "at least one" : "\(invalidCount)" }
    var recordNoun:String { invalidCount == 1 ? "record" : "records" }

    init(invalidCount: Int, issues: [MaatJournalIssue],countIsLowerBound:Bool=false) {
        self.invalidCount = invalidCount
        self.countIsLowerBound=countIsLowerBound
        self.issues = issues
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        invalidCount = try values.decodeIfPresent(Int.self, forKey: .invalidCount) ?? 0
        countIsLowerBound = try values.decodeIfPresent(Bool.self,forKey:.countIsLowerBound) ?? false
        issues = try values.decodeIfPresent([MaatJournalIssue].self, forKey: .issues) ?? []
    }
}

struct MaatJournalIssue: Decodable, Identifiable {
    let line: Int
    let digest: String
    let reason: String

    var id: String { "\(line):\(digest)" }
}

struct MaatCasebookSummary: Decodable {
    let total: Int
    let open: Int
    let urgent: Int
    let high: Int
    let resolved: Int
}

struct MaatCase: Decodable, Identifiable {
    let id: String
    let time: String
    let kind: String
    let category: String
    let status: String
    let priority: MaatCasePriority
    let requester: String
    let resource: String
    let affected: String
    let determination: String
    let assessed: String
    let why: String
    let evidence: String
	let verifiedRepairEvidence: String
	let repairOutcomeEvidence: String
	let pendingRepairAttemptEvidence: String
	let resolution: String
	let nextAction: MaatCaseNextAction?
	let systemOne: MaatSystemOneVerdict?
	let systemOneCalibration: MaatSystemOneCalibration?
    let failureMemoryReceipt: MaatPreflightReceiptReference?
    let failureMemoryIncidentKeys: [String]

    init(id: String, time: String, kind: String, category: String, status: String,
         priority: MaatCasePriority, requester: String, resource: String,
         affected: String, determination: String, assessed: String, why: String,
         evidence: String, resolution: String = "", verifiedRepairEvidence: String = "", repairOutcomeEvidence: String = "", pendingRepairAttemptEvidence: String = "", nextAction: MaatCaseNextAction? = nil, systemOne: MaatSystemOneVerdict? = nil, systemOneCalibration: MaatSystemOneCalibration? = nil, failureMemoryReceipt: MaatPreflightReceiptReference? = nil, failureMemoryIncidentKeys: [String] = []) {
        self.id = id
        self.time = time
        self.kind = kind
        self.category = category
        self.status = status
        self.priority = priority
        self.requester = requester
        self.resource = resource
        self.affected = affected
        self.determination = determination
        self.assessed = assessed
        self.why = why
        self.evidence = evidence
		self.verifiedRepairEvidence = verifiedRepairEvidence
		self.repairOutcomeEvidence = repairOutcomeEvidence
		self.pendingRepairAttemptEvidence = pendingRepairAttemptEvidence
		self.resolution = resolution
		self.nextAction = nextAction
		self.systemOne = systemOne
		self.systemOneCalibration = systemOneCalibration
        self.failureMemoryReceipt = failureMemoryReceipt
        self.failureMemoryIncidentKeys = failureMemoryIncidentKeys
    }

    private enum CodingKeys: String, CodingKey {
        case id, time, kind, category, status, priority, requester, resource
        case affected, determination, assessed, why, evidence, resolution
		case verifiedRepairEvidence = "verified_repair_evidence"
		case repairOutcomeEvidence = "repair_outcome_evidence"
		case pendingRepairAttemptEvidence = "pending_repair_attempt_evidence"
        case nextAction = "next_action"
		case systemOne = "system_one"
		case systemOneCalibration = "system_one_calibration"
        case failureMemoryReceipt = "failure_memory_receipt"
        case failureMemoryIncidentKeys = "failure_memory_incident_keys"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        id = try values.decode(String.self, forKey: .id)
        time = try values.decodeIfPresent(String.self, forKey: .time) ?? ""
        kind = try values.decodeIfPresent(String.self, forKey: .kind) ?? ""
        category = try values.decodeIfPresent(String.self, forKey: .category) ?? "assessment"
        status = try values.decodeIfPresent(String.self, forKey: .status) ?? "open"
        priority = try values.decodeIfPresent(MaatCasePriority.self, forKey: .priority) ?? .normal
        requester = try values.decodeIfPresent(String.self, forKey: .requester) ?? ""
        resource = try values.decodeIfPresent(String.self, forKey: .resource) ?? ""
        affected = try values.decodeIfPresent(String.self, forKey: .affected) ?? ""
        determination = try values.decodeIfPresent(String.self, forKey: .determination) ?? ""
        assessed = try values.decodeIfPresent(String.self, forKey: .assessed) ?? ""
        why = try values.decodeIfPresent(String.self, forKey: .why) ?? ""
        evidence = try values.decodeIfPresent(String.self, forKey: .evidence) ?? ""
		verifiedRepairEvidence = try values.decodeIfPresent(String.self, forKey: .verifiedRepairEvidence) ?? ""
		repairOutcomeEvidence = try values.decodeIfPresent(String.self, forKey: .repairOutcomeEvidence) ?? ""
		pendingRepairAttemptEvidence = try values.decodeIfPresent(String.self, forKey: .pendingRepairAttemptEvidence) ?? ""
		resolution = try values.decodeIfPresent(String.self, forKey: .resolution) ?? ""
		nextAction = try values.decodeIfPresent(MaatCaseNextAction.self, forKey: .nextAction)
		systemOne = try values.decodeIfPresent(MaatSystemOneVerdict.self, forKey: .systemOne)
		systemOneCalibration = try values.decodeIfPresent(MaatSystemOneCalibration.self, forKey: .systemOneCalibration)
        failureMemoryReceipt = try values.decodeIfPresent(MaatPreflightReceiptReference.self, forKey: .failureMemoryReceipt)
        failureMemoryIncidentKeys = try values.decodeIfPresent([String].self, forKey: .failureMemoryIncidentKeys) ?? []
    }

    var searchText: String {
        let caseFields = [
            time, kind, category, status, requester, resource, affected,
            determination, assessed, why, evidence, resolution, verifiedRepairEvidence, repairOutcomeEvidence, pendingRepairAttemptEvidence,
        ]
        let nextActionFields = [
            nextAction?.title ?? "",
            nextAction?.detail ?? "",
            nextAction?.steps.map { $0.title + " " + $0.detail }.joined(separator: " ") ?? "",
        ]
        let systemOneFields = [
            systemOne?.gate ?? "",
            systemOne?.subject.headSHA ?? "",
            systemOneCalibration?.screenEvidence ?? "",
            systemOneCalibration?.frontierEvidence ?? "",
        ]
        return (caseFields + nextActionFields + systemOneFields).joined(separator: " ")
    }
}

struct MaatSystemOneVerdict: Decodable {
    let contract: String
    let schemaVersion: Int
    let floorEvidence: String
    let featherWeight: Int
    let gate: String
    let confidence: Double
    let subject: MaatSystemOneSubject
    let floor: MaatSystemOneFloor
    let escalation: MaatSystemOneEscalation?
    let model: MaatSystemOneModel
    let findings: [MaatSystemOneFinding]

    enum CodingKeys: String, CodingKey {
        case contract, schemaVersion = "schema_version", floorEvidence = "floor_evidence"
        case featherWeight = "feather_weight", gate, confidence, subject, floor, escalation, model, findings
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        contract = try values.decodeIfPresent(String.self, forKey: .contract) ?? ""
        schemaVersion = try values.decodeIfPresent(Int.self, forKey: .schemaVersion) ?? 1
        floorEvidence = try values.decodeIfPresent(String.self, forKey: .floorEvidence) ?? ""
        featherWeight = try values.decode(Int.self, forKey: .featherWeight)
        gate = try values.decode(String.self, forKey: .gate)
        confidence = try values.decode(Double.self, forKey: .confidence)
        subject = try values.decode(MaatSystemOneSubject.self, forKey: .subject)
        floor = try values.decode(MaatSystemOneFloor.self, forKey: .floor)
        escalation = try values.decodeIfPresent(MaatSystemOneEscalation.self, forKey: .escalation)
        model = try values.decode(MaatSystemOneModel.self, forKey: .model)
        findings = try values.decodeIfPresent([MaatSystemOneFinding].self, forKey: .findings) ?? []
    }
}

// Both `sirsi maat triage --confirm --json` and `sirsi maat screen --confirm
// --json` intentionally project the closed System One verdict directly rather
// than wrapping it in the generic CommandResult envelope. Triage additionally
// binds the snapshot and Casebook decision evidence. Decode the shared verdict
// once so a successful local write can refresh the native Casebook instead of
// being displayed as a false failure.
struct MaatSystemOneCommandResult: Decodable {
    let verdict: MaatSystemOneVerdict
    let snapshotEvidence: String
    let decisionEvidence: String

    enum CodingKeys: String, CodingKey {
        case snapshotEvidence = "snapshot_evidence"
        case decisionEvidence = "decision_evidence"
    }

    init(from decoder: Decoder) throws {
        verdict = try MaatSystemOneVerdict(from: decoder)
        let values = try decoder.container(keyedBy: CodingKeys.self)
        snapshotEvidence = try values.decodeIfPresent(String.self, forKey: .snapshotEvidence) ?? ""
        decisionEvidence = try values.decodeIfPresent(String.self, forKey: .decisionEvidence) ?? ""
    }

    var summary: String {
        if verdict.contract == MaatSourceImportContract.id && verdict.schemaVersion == 2 {
            return "Ma'at recorded an unqualified source screen: \(verdict.gate). Floor evidence is declared-only; this is not binding approval."
        }
        switch verdict.gate {
        case "pass":
            return "Ma'at recorded this observation. No issue exceeded the configured threshold."
        case "changes":
            return "Ma'at recorded this observation with bounded changes to address."
        case "block":
            return "Ma'at recorded this observation and blocked it on deterministic or high-confidence evidence."
        case "escalate":
            return "Ma'at recorded this observation and requires an explicit owner review."
        default:
            return "Ma'at recorded this observation with an unrecognized System One gate."
        }
    }

    static func decode(_ raw: String) -> MaatSystemOneCommandResult? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatSystemOneCommandResult.self, from: Data(raw[start...].utf8))
    }
}

struct MaatSystemOneFinding: Decodable, Identifiable {
    let id: String
    let severity: String
    let category: String
    let file: String
    let line: Int
    let claim: String
    let evidence: String
    let fixHint: String
    let repairID: String

    enum CodingKeys: String, CodingKey {
        case id, severity, category, file, line, claim, evidence
        case fixHint = "fix_hint"
        case repairID = "repair_id"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        // System One's closed producer schema requires a stable finding ID;
        // do not mint a local random substitute that would make copied
        // evidence rows non-repeatable across refreshes.
        id = try values.decode(String.self, forKey: .id)
        severity = try values.decodeIfPresent(String.self, forKey: .severity) ?? "information"
        category = try values.decodeIfPresent(String.self, forKey: .category) ?? "finding"
        file = try values.decodeIfPresent(String.self, forKey: .file) ?? ""
        line = try values.decodeIfPresent(Int.self, forKey: .line) ?? 0
        claim = try values.decodeIfPresent(String.self, forKey: .claim) ?? "No claim was supplied."
        evidence = try values.decodeIfPresent(String.self, forKey: .evidence) ?? ""
        fixHint = try values.decodeIfPresent(String.self, forKey: .fixHint) ?? ""
        repairID = try values.decodeIfPresent(String.self, forKey: .repairID) ?? ""
    }

    var location: String {
        guard !file.isEmpty else { return "" }
        return line > 0 ? "\(file):\(line)" : file
    }
}

// Presentation only. Do not reorder the retained verdict or recompute its
// evidence: canonical wire order is by ID, while people see blockers first.
func maatSystemOneDisplayFindings(_ findings:[MaatSystemOneFinding])->[MaatSystemOneFinding]{
    func rank(_ severity:String)->Int{
        switch severity{
        case "block","critical","error":return 0
        case "major","warning":return 1
        case "minor":return 2
        default:return 3
        }
    }
    return findings.sorted{
        let left=rank($0.severity),right=rank($1.severity)
        return left==right ? $0.id<$1.id:left<right
    }
}

struct MaatSystemOneSubject: Decodable {
    let kind: String
    let ref: String
    let headSHA: String
    let repo: String?
    let hostBinding: String?
    enum CodingKeys: String, CodingKey { case kind, ref, repo; case hostBinding = "host_binding"; case headSHA = "head_sha" }
}

struct MaatSystemOneFloor: Decodable {
    let passed: Bool
    let checks: [MaatSystemOneFloorCheck]
}

struct MaatSystemOneFloorCheck: Decodable {
    let name: String
    let passed: Bool
    let detail: String

    enum CodingKeys: String, CodingKey { case name, passed, detail }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        name = try values.decode(String.self, forKey: .name)
        passed = try values.decode(Bool.self, forKey: .passed)
        detail = try values.decodeIfPresent(String.self, forKey: .detail) ?? ""
    }
}

struct MaatSystemOneModel: Decodable {
    let provider: String
    let version: String
    let local: Bool
    let latencyMS: Int
    let calibration: String
    let calibrationID: String
    let evidenceSetDigest: String

    enum CodingKeys: String, CodingKey {
        case provider, version, local
        case latencyMS = "latency_ms"
        case calibration, calibrationID = "calibration_id", evidenceSetDigest = "evidence_set_digest"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        provider = try values.decode(String.self, forKey: .provider)
        version = try values.decode(String.self, forKey: .version)
        local = try values.decode(Bool.self, forKey: .local)
        latencyMS = try values.decode(Int.self, forKey: .latencyMS)
        calibration = try values.decodeIfPresent(String.self, forKey: .calibration) ?? ""
        calibrationID = try values.decodeIfPresent(String.self, forKey: .calibrationID) ?? ""
        evidenceSetDigest = try values.decodeIfPresent(String.self, forKey: .evidenceSetDigest) ?? ""
    }

    var detail: String {
        "\(provider) \(version) · \(local ? "local" : "external") · \(latencyMS)ms"
    }
}

struct MaatSystemOneEscalation: Decodable {
    let reason: String
    let reviewTier: String
    enum CodingKeys: String, CodingKey { case reason; case reviewTier = "review_tier" }
}

struct MaatSystemOneCalibration: Decodable {
    let screenEvidence: String
    let frontierEvidence: String
    let screenGate: String
    let frontierGate: String

    enum CodingKeys: String, CodingKey {
        case screenEvidence = "screen_evidence"
        case frontierEvidence = "frontier_evidence"
        case screenGate = "screen_gate"
        case frontierGate = "frontier_gate"
    }
}

// MaatReleaseContractPreflight is the native projection of the typed local
// release source observation. It deliberately is not CommandResult: the
// preflight's file identities and deterministic floor are the evidence the
// operator must inspect before opting into one Casebook record.
struct MaatReleaseContractPreflight: Decodable {
    let root: String
    let fingerprint: String
    let verdict: MaatSystemOneVerdict
    let decisionEvidence: String

    enum CodingKeys: String, CodingKey {
        case root, fingerprint, verdict
        case decisionEvidence = "decision_evidence"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        root = try values.decode(String.self, forKey: .root)
        fingerprint = try values.decode(String.self, forKey: .fingerprint)
        verdict = try values.decode(MaatSystemOneVerdict.self, forKey: .verdict)
        decisionEvidence = try values.decodeIfPresent(String.self, forKey: .decisionEvidence) ?? ""
    }

    static func decode(_ raw: String) -> MaatReleaseContractPreflight? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatReleaseContractPreflight.self, from: Data(raw[start...].utf8))
    }
}

// MaatReleaseCredentialPreflight is intentionally narrower than a release
// receipt. It projects public local Developer ID certificate metadata and the
// explicit protected-workflow requirement for notarization; it never models a
// private key or secret as UI data.
struct MaatReleaseCredentialPreflight: Decodable {
    let teamID: String
    let fingerprint: String
    let developerIdentities: [MaatReleaseSigningIdentity]
    let observedNonDeveloperIdentityTypes: [String]
    let notarizationObserved: Bool
	let recoveryPlan: [String]
    let verdict: MaatSystemOneVerdict
    let decisionEvidence: String

    enum CodingKeys: String, CodingKey {
        case teamID = "team_id"
        case fingerprint
        case developerIdentities = "developer_identities"
        case observedNonDeveloperIdentityTypes = "observed_non_developer_identity_types"
        case notarizationObserved = "notarization_observed"
        case recoveryPlan = "recovery_plan"
        case verdict
        case decisionEvidence = "decision_evidence"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        teamID = try values.decode(String.self, forKey: .teamID)
        fingerprint = try values.decode(String.self, forKey: .fingerprint)
        developerIdentities = try values.decodeIfPresent([MaatReleaseSigningIdentity].self, forKey: .developerIdentities) ?? []
        observedNonDeveloperIdentityTypes = try values.decodeIfPresent([String].self, forKey: .observedNonDeveloperIdentityTypes) ?? []
        notarizationObserved = try values.decodeIfPresent(Bool.self, forKey: .notarizationObserved) ?? false
        recoveryPlan = try values.decodeIfPresent([String].self, forKey: .recoveryPlan) ?? []
        verdict = try values.decode(MaatSystemOneVerdict.self, forKey: .verdict)
        decisionEvidence = try values.decodeIfPresent(String.self, forKey: .decisionEvidence) ?? ""
    }

    static func decode(_ raw: String) -> MaatReleaseCredentialPreflight? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: Data(raw[start...].utf8))
    }
}

// protectedReleaseRecoverySteps gives the native surface a closed, honest
// recovery path for every credential outcome. It intentionally stops before
// secret/key access: only the protected release workflow may perform that work.
func protectedReleaseRecoverySteps(for preflight: MaatReleaseCredentialPreflight) -> [String] {
	if !preflight.recoveryPlan.isEmpty { return preflight.recoveryPlan }
	let identityObservationFailed = MaatCredentialPreflightProjection(preflight).identityObservationFailed
	let observedKinds = Set(preflight.developerIdentities.map(\.kind))
    var steps: [String] = []

    if !identityObservationFailed && !observedKinds.contains("application") {
        steps.append("Make one usable Team \(preflight.teamID) Developer ID Application identity available through the protected release workflow.")
    }
    if !identityObservationFailed && !observedKinds.contains("installer") {
        steps.append("Make one usable Team \(preflight.teamID) Developer ID Installer identity available through the protected release workflow.")
    }
    if !preflight.notarizationObserved {
        steps.append("Have the protected release workflow validate one complete notarization credential set without exposing its secrets to Ma'at.")
    }
    steps.append("Recheck readiness here, then run the signed, notarized DMG and PKG workflow only after every required proof is present.")
    return steps
}

struct MaatReleaseSigningIdentity: Decodable, Identifiable {
    let kind: String
    let name: String
    let fingerprint: String
    var id: String { "\(kind):\(fingerprint)" }
}

struct MaatCredentialPreflightProjection {
    let identityObservationFailed: Bool
    let requiredIdentityChecks: [MaatSystemOneFloorCheck]
    let observedRequiredKindCount: Int
    let additionalIdentityLabels: [String]
    let notarizationStatus: String

    init(_ preflight: MaatReleaseCredentialPreflight) {
        let requiredKinds: Set<String> = ["developer-id-application", "developer-id-installer"]
        identityObservationFailed = preflight.verdict.findings.contains { $0.id == "security-identity-observation" }
        requiredIdentityChecks = preflight.verdict.floor.checks.filter { requiredKinds.contains($0.name) }
        observedRequiredKindCount = Set(requiredIdentityChecks.filter(\.passed).map(\.name)).count
        additionalIdentityLabels = preflight.observedNonDeveloperIdentityTypes.map { "Additional local identity: \($0)" }
        notarizationStatus = preflight.notarizationObserved ? "Notarization was observed by the protected workflow." : "Notarization not inspected here."
    }

    var allRequiredKindsObserved: Bool { observedRequiredKindCount == 2 }
    var identityHeading: String { identityObservationFailed ? "Local identities unconfirmed" : "Local identities observed" }
    var requiredKindsSummary: String {
        identityObservationFailed ? "Required identity types could not be confirmed." : "Required identity types observed: \(observedRequiredKindCount) of 2."
    }
}

struct MaatCaseNextAction: Decodable {
    let kind: String
    let actionID: String
    let title: String
    let detail: String
    let evidence: String
    let requiresConfirmation: Bool
    let steps: [MaatResolutionStep]

    enum CodingKeys: String, CodingKey {
        case kind, title, detail, evidence, steps
        case actionID = "action_id"
        case requiresConfirmation = "requires_confirmation"
    }

    init(kind: String, actionID: String = "", title: String, detail: String, evidence: String, requiresConfirmation: Bool, steps: [MaatResolutionStep] = []) {
        self.kind = kind
        self.actionID = actionID
        self.title = title
        self.detail = detail
        self.evidence = evidence
        self.requiresConfirmation = requiresConfirmation
        self.steps = steps
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        kind = try values.decodeIfPresent(String.self, forKey: .kind) ?? "owner_review"
        actionID = try values.decodeIfPresent(String.self, forKey: .actionID) ?? ""
        title = try values.decodeIfPresent(String.self, forKey: .title) ?? "Review the retained evidence"
        detail = try values.decodeIfPresent(String.self, forKey: .detail) ?? "Record a new evidence-bound owner decision."
        evidence = try values.decodeIfPresent(String.self, forKey: .evidence) ?? ""
        requiresConfirmation = try values.decodeIfPresent(Bool.self, forKey: .requiresConfirmation) ?? false
        steps = try values.decodeIfPresent([MaatResolutionStep].self, forKey: .steps) ?? []
    }
}

struct MaatResolutionStep: Decodable, Identifiable {
    let level: Int
    let title: String
    let detail: String
    let evidence: String
    let requiresConfirmation: Bool

    var id: String { "\(level)-\(title)-\(evidence)" }

    enum CodingKeys: String, CodingKey {
        case level, title, detail, evidence
        case requiresConfirmation = "requires_confirmation"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        level = try values.decodeIfPresent(Int.self, forKey: .level) ?? 0
        title = try values.decodeIfPresent(String.self, forKey: .title) ?? "Recovery step"
        detail = try values.decodeIfPresent(String.self, forKey: .detail) ?? "Review the retained evidence."
        evidence = try values.decodeIfPresent(String.self, forKey: .evidence) ?? ""
        requiresConfirmation = try values.decodeIfPresent(Bool.self, forKey: .requiresConfirmation) ?? false
    }
}

enum MaatCasePriority: String, Decodable {
    case urgent, high, normal

    var tint: Color {
        switch self {
        case .urgent: return .red
        case .high: return .orange
        case .normal: return .blue
        }
    }

    var rank: Int {
        switch self {
        case .urgent: return 3
        case .high: return 2
        case .normal: return 1
        }
    }
}

struct MaatKnowledgeProjection: Decodable {
    let items: [MaatKnowledgeItem]
    let total: Int
    let withheld: Int
    let coverage: MaatKnowledgeCoverage?
    let nextCursor: String?
    let state: String?

    init(items: [MaatKnowledgeItem], total: Int, withheld: Int, coverage: MaatKnowledgeCoverage? = nil, nextCursor: String? = nil, state: String? = nil) {
        self.items = items
        self.total = total
        self.withheld = withheld
        self.coverage = coverage
        self.nextCursor = nextCursor
        self.state = state
    }

    private enum CodingKeys: String, CodingKey {
        case items, total, withheld, coverage, state
        case nextCursor = "next_cursor"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        items = try values.decode([MaatKnowledgeItem].self, forKey: .items)
        total = try values.decode(Int.self, forKey: .total)
        withheld = try values.decode(Int.self, forKey: .withheld)
        guard total >= 0 else {
            throw DecodingError.dataCorruptedError(forKey: .total, in: values, debugDescription: "total must be nonnegative")
        }
        guard withheld >= 0 else {
            throw DecodingError.dataCorruptedError(forKey: .withheld, in: values, debugDescription: "withheld must be nonnegative")
        }
        coverage = try values.decodeIfPresent(MaatKnowledgeCoverage.self, forKey: .coverage)
        nextCursor = try values.decodeIfPresent(String.self, forKey: .nextCursor)
        state = try values.decodeIfPresent(String.self, forKey: .state)
    }

    static let snapshotPreview = MaatKnowledgeProjection(
        items: [
            MaatKnowledgeItem(id: "knowledge-1", title: "Release evidence", summary: "A retained local note about the current package and review state.", references: [MaatKnowledgeReference(type: "source", value: "local archive")]),
            MaatKnowledgeItem(id: "knowledge-2", title: "Operator handback", summary: "A recorded outcome that can be connected to a Ma'at decision.", references: [MaatKnowledgeReference(type: "source", value: "activity ledger")]),
        ],
        total: 2,
        withheld: 0
    )
}

struct MaatKnowledgeProjectionReceipt: Equatable {
    let projected: Int
    let remaining: Int
    let isComplete: Bool
    var summary: String {
        if isComplete {
            return "Projected \(projected); no projection debt remains."
        }
        return "Projected \(projected); \(remaining) remain. Partial progress; reconciliation is not complete."
    }

    init(projected: Int, remaining: Int, isComplete: Bool) {
        self.projected = projected
        self.remaining = remaining
        self.isComplete = isComplete
    }

    init?(commandResult: CommandResult) {
        guard commandResult.command == "sirsi maat knowledge reconcile-projections",
              commandResult.evidence.count == 2,
              let projected = Self.count(label: "Projected", in: commandResult.evidence),
              let remaining = Self.count(label: "Remaining", in: commandResult.evidence) else { return nil }

        let complete: Bool
        switch commandResult.status {
        case .some("ok"):
            guard commandResult.errors.isEmpty, remaining == 0 else { return nil }
            complete = true
        case .some("partial"):
            guard commandResult.errors == ["projection_debt_remains"], remaining > 0 else { return nil }
            complete = false
        default:
            return nil
        }

        self.projected = projected
        self.remaining = remaining
        self.isComplete = complete
    }

    private static func count(label: String, in evidence: [CRFact]) -> Int? {
        let matches = evidence.filter { $0.label == label }
        guard matches.count == 1,
              let value = Int(matches[0].value), value >= 0,
              String(value) == matches[0].value else { return nil }
        return value
    }
}

struct MaatKnowledgeResolutionRequest: Equatable {
    let recordID: String
    let revision: Int
    let digest: String
    let state: String
    let note: String

    static func capture(recordID: String, revision: Int?, digest: String?, state: String, note: String) -> Self? {
        let approvedNote = note.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !recordID.isEmpty, let revision, revision > 0,
              let digest, !digest.isEmpty,
              ["reviewed", "accepted", "rejected"].contains(state),
              !approvedNote.isEmpty else { return nil }
        return Self(recordID: recordID, revision: revision, digest: digest, state: state, note: approvedNote)
    }

    var arguments: [String] {
        ["maat", "knowledge", "resolve", recordID, "--state", state,
         "--revision", String(revision), "--digest", digest, "--note-stdin"]
    }
}

struct MaatKnowledgeResolutionSubmission: Equatable {
    private(set) var inFlight = false
    private(set) var submissionStarted = false

    mutating func begin() -> Bool {
        guard !submissionStarted else { return false }
        inFlight = true
        submissionStarted = true
        return true
    }

    mutating func finish() { inFlight = false }
}

struct MaatKnowledgeCoverage: Decodable {
    let sources: [MaatKnowledgeSourceRun]
    let pendingProjectionDebt: Int

    private enum CodingKeys: String, CodingKey {
        case sources
        case pendingProjectionDebt = "pending_projection_debt"
    }
}

struct MaatKnowledgeSourceRun: Decodable, Identifiable {
    let id: Int64
    let source: String
    let state: String
    let errorCode: String?

    private enum CodingKeys: String, CodingKey {
        case id, source, state
        case errorCode = "error_code"
    }
}

struct MaatKnowledgeItem: Decodable, Identifiable {
    let id: String
    let title: String
    let summary: String
    let references: [MaatKnowledgeReference]
    let content: String?
    let classification: String?
    let resolution: String?
    let revision: Int?
    let contentDigest: String?
    let origins: [MaatKnowledgeOrigin]
    let withheldReason: String?
    let annotations: [MaatKnowledgeOwnerAnnotation]

    init(id: String, title: String, summary: String, references: [MaatKnowledgeReference]) {
        self.id = id
        self.title = title
        self.summary = summary
        self.references = references
        self.content = nil
        self.classification = nil
        self.resolution = nil
        self.revision = nil
        self.contentDigest = nil
        self.origins = []
        self.withheldReason = nil
        self.annotations = []
    }

    private enum CodingKeys: String, CodingKey {
        case id, title, summary, references, content, classification, resolution, revision, origins
        case contentDigest = "content_digest"
        case withheldReason = "withheld_reason"
        case annotations = "owner_annotations"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        title = try values.decodeIfPresent(String.self, forKey: .title) ?? "Untitled knowledge"
        summary = try values.decodeIfPresent(String.self, forKey: .summary) ?? ""
        references = try values.decodeIfPresent([MaatKnowledgeReference].self, forKey: .references) ?? []
        content = try values.decodeIfPresent(String.self, forKey: .content)
        classification = try values.decodeIfPresent(String.self, forKey: .classification)
        resolution = try values.decodeIfPresent(String.self, forKey: .resolution)
        revision = try values.decodeIfPresent(Int.self, forKey: .revision)
        contentDigest = try values.decodeIfPresent(String.self, forKey: .contentDigest)
        origins = try values.decodeIfPresent([MaatKnowledgeOrigin].self, forKey: .origins) ?? []
        withheldReason = try values.decodeIfPresent(String.self, forKey: .withheldReason)
        annotations = try values.decodeIfPresent([MaatKnowledgeOwnerAnnotation].self, forKey: .annotations) ?? []
        let sourceKey = ([title, summary] + references.map { $0.type + ":" + $0.value }).joined(separator: "\u{0}")
        id = try values.decodeIfPresent(String.self, forKey: .id) ?? sourceKey.data(using: .utf8)?.base64EncodedString() ?? UUID().uuidString
    }

    var source: String? {
        references.first(where: { $0.type == "source" })?.value
    }

    var searchText: String {
        ([title, summary] + references.map { $0.type + " " + $0.value }).joined(separator: " ")
    }
}

struct MaatKnowledgeOwnerAnnotation: Decodable, Identifiable {
    let id: Int64
    let state: String
    let text: String
}

struct MaatKnowledgeOrigin: Decodable, Identifiable {
    let recordID: String
    let source: String
    let instanceID: String
    let nativeItemID: String
    let reference: String
    var id: String { "\(recordID):\(source):\(instanceID):\(nativeItemID):\(reference)" }

    private enum CodingKeys: String, CodingKey {
        case source, reference
        case recordID = "record_id"
        case instanceID = "instance_id"
        case nativeItemID = "native_item_id"
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        recordID = try values.decodeIfPresent(String.self, forKey: .recordID) ?? ""
        source = try values.decodeIfPresent(String.self, forKey: .source) ?? "unknown source"
        instanceID = try values.decodeIfPresent(String.self, forKey: .instanceID) ?? ""
        nativeItemID = try values.decodeIfPresent(String.self, forKey: .nativeItemID) ?? ""
        reference = try values.decodeIfPresent(String.self, forKey: .reference) ?? ""
    }
}

struct MaatKnowledgeReference: Decodable, Identifiable {
    let type: String
    let value: String
    var id: String { type + ":" + value }
}
