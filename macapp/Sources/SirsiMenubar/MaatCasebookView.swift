import SwiftUI
import UniformTypeIdentifiers

// MaatWorkspaceView joins decision quality and retained local knowledge under
// one operator authority. Seshat's ingestion compatibility commands are not a
// second Pantheon surface: Ma'at is where a person inspects the resulting
// evidence, classifications, and decisions.
struct MaatWorkspaceView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var section: MaatWorkspaceSection = .systemOne
    let preloadedCasebook: MaatCasebookProjection?
    let preloadedKnowledge: MaatKnowledgeProjection?

    init(engine: SirsiEngine, preloadedCasebook: MaatCasebookProjection? = nil,
         preloadedKnowledge: MaatKnowledgeProjection? = nil) {
        self.engine = engine
        self.preloadedCasebook = preloadedCasebook
        self.preloadedKnowledge = preloadedKnowledge
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Ma'at")
            Picker("Ma'at workspace", selection: $section) {
                ForEach(MaatWorkspaceSection.allCases) { option in
                    Label(option.title, systemImage: option.symbol).tag(option)
                }
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)

            switch section {
            case .systemOne:
                MaatSystemOneView(engine: engine, section: $section, preloaded: preloadedCasebook)
            case .decisions:
                MaatCasebookView(engine: engine, preloaded: preloadedCasebook, showsBackBar: false)
            case .knowledge:
                MaatKnowledgeView(engine: engine, preloaded: preloadedKnowledge, showsBackBar: false)
            }
        }
    }
}

private enum MaatWorkspaceSection: String, CaseIterable, Identifiable {
    case systemOne, decisions, knowledge

    var id: String { rawValue }
    var title: String {
        switch self {
        case .systemOne: return "System One"
        case .decisions: return "Decisions"
        case .knowledge: return "Knowledge"
        }
    }
    var symbol: String {
        switch self {
        case .systemOne: return "scalemass"
        case .decisions: return "checkmark.seal"
        case .knowledge: return "books.vertical"
        }
    }
}

// MaatSystemOneView makes the local JEV-like system visible as an operational
// surface, not a hidden field in an individual case. It projects only the
// canonical Casebook: no ambient state is classified here and no local screen
// becomes mutation or release authority merely because it is rendered.
private struct MaatSystemOneView: View {
    @ObservedObject var engine: SirsiEngine
    @Binding var section: MaatWorkspaceSection
    @State private var casebook: MaatCasebookProjection?
    @State private var loading: Bool
    @State private var loadError: String?
    @State private var showScreenPicker = false
    @State private var selectedScreenURL: URL?
    @State private var confirmScreenImport = false
    @State private var screenImportInFlight = false
    @State private var screenImportResult: CommandResult?
    @State private var screenImportError: String?

    init(engine: SirsiEngine, section: Binding<MaatWorkspaceSection>, preloaded: MaatCasebookProjection? = nil) {
        self.engine = engine
        _section = section
        _casebook = State(initialValue: preloaded)
        _loading = State(initialValue: preloaded == nil)
    }

    var body: some View {
        Group {
            if loading {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("Opening retained System One evidence…")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let loadError {
                unavailableState(loadError)
            } else if let casebook {
                systemOneBody(casebook)
            } else {
                unavailableState("Pantheon did not receive a Ma'at Casebook projection. No System One result was inferred.")
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
                selectedScreenURL = url
                screenImportError = nil
                confirmScreenImport = true
            case .failure(let error):
                screenImportError = "Pantheon did not open the selected evidence file: \(error.localizedDescription)"
            }
        }
        .confirmationDialog("Record this System One screen?", isPresented: $confirmScreenImport, titleVisibility: .visible) {
            Button("Validate and record") { Task { await importScreen() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Ma'at will validate the exact selected JSON and record an evidence-bound local gate. It will not execute the assessed payload, authorize work, or treat this as a release decision.")
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
            Button("Try again") { Task { await load() } }
                .buttonStyle(.borderedProminent)
                .tint(gold)
            Button("Open decisions") { section = .decisions }
                .buttonStyle(.bordered)
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab authority", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func systemOneBody(_ casebook: MaatCasebookProjection) -> some View {
        let screens = casebook.cases.filter { $0.systemOne != nil }
        let calibrations = casebook.cases.filter { $0.systemOneCalibration != nil }
        return VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    summary(screens: screens, calibrations: calibrations)
                    resolutionLane(screens)
                    screenImportControl
                    if screens.isEmpty {
                        emptyState
                    } else {
                        screenList(screens)
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
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 11)
        }
    }

    private func summary(screens: [MaatCase], calibrations: [MaatCase]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Ma'at System One")
                .sirsiFont(.title3, weight: .bold)
            Text("Local deterministic screens are retained, evidence-bound, and calibrated only against distinct independent review outcomes.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
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
        let calibratedMetric = systemMetric("Calibrated", calibrations.count, .green)

        // A single metric row becomes unreadable at accessibility text sizes.
        // Keep the same facts, but let SwiftUI select a two-line layout when
        // the operator's window or text size needs it.
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 8) {
                screensMetric
                changesMetric
                blockedMetric
                escalatedMetric
                calibratedMetric
            }
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    screensMetric
                    changesMetric
                    blockedMetric
                }
                HStack(spacing: 8) {
                    escalatedMetric
                    calibratedMetric
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
        }.sorted { lhs, rhs in
            let left = gateRank(lhs.systemOne?.gate ?? "")
            let right = gateRank(rhs.systemOne?.gate ?? "")
            if left != right { return left < right }
            return lhs.priority.rank > rhs.priority.rank
        }

        if let next = unresolved.first, let verdict = next.systemOne {
            VStack(alignment: .leading, spacing: 8) {
                Label(resolutionTitle(for: verdict.gate), systemImage: resolutionSymbol(for: verdict.gate))
                    .sirsiFont(.headline)
                    .foregroundStyle(gateTint(verdict.gate))
                Text(resolutionDetail(for: next, gate: verdict.gate))
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                NavLink { MaatCaseDetailView(engine: engine, entry: next) } label: {
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
            Text("This is an empty evidence history, not a pass or a failure. System One does not invent a screen from ambient state: a qualified Pantheon producer supplies a closed, typed evidence packet, then Ma'at records the deterministic gate here.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Open decisions") { section = .decisions }
                .buttonStyle(.bordered)
            NavLink { StackLabView(engine: engine) } label: {
                Label("Inspect Stack Lab recipes", systemImage: "cube.transparent")
            }
            .buttonStyle(.bordered)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private var screenImportControl: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Record a closed screen")
                .sirsiFont(.headline)
            Text("Choose a qualified producer's System One JSON. Ma'at validates the closed schema and deterministic floor before it writes one evidence-bound Casebook record.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button {
                showScreenPicker = true
            } label: {
                Label("Choose System One JSON", systemImage: "doc.badge.plus")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
            .disabled(screenImportInFlight)
            if let selectedScreenURL {
                Text("Selected: \(selectedScreenURL.lastPathComponent)")
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            if screenImportInFlight {
                ProgressView("Validating and recording…")
                    .sirsiFont(.caption)
            }
            if let result = screenImportResult {
                Label(result.summary, systemImage: result.ok ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(result.ok ? .green : .orange)
                if result.ok {
                    Text("The Casebook was refreshed from the recorded evidence.")
                        .sirsiFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            if let screenImportError {
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

    @ViewBuilder private func screenList(_ screens: [MaatCase]) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Recorded screens")
                .sirsiFont(.headline)
            ForEach(screens) { entry in
                NavLink { MaatCaseDetailView(engine: engine, entry: entry) } label: {
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
        guard let selectedScreenURL else { return }
        screenImportInFlight = true
        screenImportError = nil
        screenImportResult = await SirsiEngine.runResult(args: ["maat", "screen", "--input", selectedScreenURL.path, "--confirm"])
        if screenImportResult == nil {
            screenImportError = "Ma'at could not validate or record this screen. The input remains unchanged and no System One result was inferred. Choose a valid closed evidence JSON or inspect the producer's receipt."
        } else if screenImportResult?.ok == true {
            await load()
        }
        screenImportInFlight = false
    }

    @MainActor private func load() async {
        loading = true
        loadError = nil
        let result = await MaatCasebookView.fetch()
        if let result {
            casebook = result
        } else {
            loadError = "Pantheon could not read the local Ma'at decision journal. No System One gate was inferred. Retry the exact read, inspect decisions, or inspect Stack Lab authority."
        }
        loading = false
    }
}

// MaatCasebookView is the native operator surface for Ma'at's recorded
// decisions. It reads the same local casebook projection exposed by the CLI
// and dashboard; it does not rescore, alter, or authorize a decision.
struct MaatCasebookView: View {
    @ObservedObject var engine: SirsiEngine
    let showsBackBar: Bool
    @State private var casebook: MaatCasebookProjection?
    @State private var query = ""
    @State private var loading = true
    @State private var loadError: String?

    init(engine: SirsiEngine, preloaded: MaatCasebookProjection? = nil, showsBackBar: Bool = true) {
        self.engine = engine
        self.showsBackBar = showsBackBar
        _casebook = State(initialValue: preloaded)
        _loading = State(initialValue: preloaded == nil)
    }

    var body: some View {
        VStack(spacing: 0) {
            if showsBackBar { BackBar(title: "Ma'at casebook") }
            if loading {
                loadingState
            } else if let loadError {
                failureState(loadError)
            } else if let casebook {
                casebookBody(casebook)
            } else {
                emptyState
            }
        }
        .task {
            guard casebook == nil else { return }
            await load()
        }
    }

    private var loadingState: some View {
        VStack(spacing: 10) {
            ProgressView()
            Text("Opening the local casebook…")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
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
            Button("Try again") { Task { await load() } }
                .buttonStyle(.borderedProminent)
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
                    summary(casebook.summary)
                    searchField
                    caseList(for: filtered(casebook.cases))
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
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 11)
        }
    }

    private func summary(_ summary: MaatCasebookSummary) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Decision casebook")
                .sirsiFont(.title3, weight: .bold)
            Text(summaryDetail(summary))
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
            HStack(spacing: 8) {
                caseMetric("Open", value: summary.open, tint: .orange)
                caseMetric("Urgent", value: summary.urgent, tint: .red)
                caseMetric("Resolved", value: summary.resolved, tint: .green)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private func summaryDetail(_ summary: MaatCasebookSummary) -> String {
        guard summary.total > 0 else { return "No decisions match this view" }
        if summary.urgent > 0 { return "Start with the decisions that need a response." }
        if summary.open > 0 { return "\(summary.open) decision\(summary.open == 1 ? "" : "s") still needs attention." }
        return "All recorded decisions are resolved."
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
            if !query.isEmpty {
                Button { query = "" } label: {
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
                    NavLink { MaatCaseDetailView(engine: engine, entry: entry) } label: {
                        MaatCaseRow(entry: entry)
                    }
                    if entry.id != cases.last?.id { Divider().padding(.leading, 14) }
                }
            }
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.045)))
        }
    }

    private func filtered(_ cases: [MaatCase]) -> [MaatCase] {
        let term = query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !term.isEmpty else { return cases }
        return cases.filter { $0.searchText.localizedCaseInsensitiveContains(term) }
    }

    @MainActor private func load() async {
        loading = true
        loadError = nil
        let result = await Self.fetch()
        if let result {
            casebook = result
        } else {
            loadError = "Pantheon could not read the local Ma'at decision journal. Check the project and try again."
        }
        loading = false
    }

    nonisolated static func fetch() async -> MaatCasebookProjection? {
        let raw = await SirsiEngine.run(args: ["maat", "casebook", "--json", "--limit", "50"], stdin: nil)
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
    @ObservedObject var engine: SirsiEngine
    let showsBackBar: Bool
    @State private var knowledge: MaatKnowledgeProjection?
    @State private var query = ""
    @State private var loading = true
    @State private var loadError: String?

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
            } else if let loadError {
                failureState(loadError)
            } else if let knowledge {
                knowledgeBody(knowledge)
            }
        }
        .task {
            guard knowledge == nil else { return }
            await load()
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
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func knowledgeBody(_ knowledge: MaatKnowledgeProjection) -> some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
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
                    searchField
                    knowledgeList(filtered(knowledge.items))
                }
                .padding(16)
            }
            Divider()
            HStack {
                Text("Local knowledge cache · read only")
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
            if !query.isEmpty {
                Button { query = "" } label: {
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

    @ViewBuilder private func knowledgeList(_ items: [MaatKnowledgeItem]) -> some View {
        if items.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                Text(query.isEmpty ? "Nothing in the local library" : "No matching knowledge")
                    .sirsiFont(.headline)
                Text(query.isEmpty ? "Ingestion remains available through the compatibility tools; Ma'at is the place to inspect what is retained." : "Try a broader term or clear the filter.")
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

    private func filtered(_ items: [MaatKnowledgeItem]) -> [MaatKnowledgeItem] {
        let term = query.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !term.isEmpty else { return items }
        return items.filter { $0.searchText.localizedCaseInsensitiveContains(term) }
    }

    @MainActor private func load() async {
        loading = true
        loadError = nil
        let result = await Self.fetch()
        if let result {
            knowledge = result
        } else {
            loadError = "Pantheon could not read Ma'at's local knowledge projection. Check the project and try again."
        }
        loading = false
    }

    nonisolated static func fetch() async -> MaatKnowledgeProjection? {
        let raw = await SirsiEngine.run(args: ["maat", "knowledge", "--json"], stdin: nil)
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatKnowledgeProjection.self, from: Data(raw[start...].utf8))
    }
}

private struct MaatKnowledgeDetailView: View {
    let item: MaatKnowledgeItem

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Ma'at knowledge")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    Text(item.title).sirsiFont(.title3, weight: .bold)
                    if !item.summary.isEmpty {
                        Text(item.summary)
                            .sirsiFont(.body)
                            .fixedSize(horizontal: false, vertical: true)
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
                }
                .padding(16)
            }
        }
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

private struct MaatCaseDetailView: View {
    @ObservedObject var engine: SirsiEngine
    let entry: MaatCase
    @State private var conclusion = ""
    @State private var actionResult: CommandResult?
    @State private var actionError: String?
    @State private var actionInFlight = false
    @State private var confirmAction = false
    @State private var evidenceCopied = false

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
                        if action.requiresConfirmation {
                            Label("Requires explicit confirmation", systemImage: "checkmark.shield")
                                .sirsiFont(.caption, weight: .semibold)
                                .foregroundStyle(sirsiGold)
                        }
                    }
                    if !entry.resolution.isEmpty {
                        detailSection("Owner acceptance", entry.resolution)
                        Text("This records an owner conclusion. It does not claim that Pantheon repaired the diagnosed system.")
                            .sirsiFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                    if let action = entry.nextAction, entry.status != "resolved" { resolutionAction(action) }
                }
                .padding(16)
            }
        }
        .confirmationDialog(confirmationTitle, isPresented: $confirmAction, titleVisibility: .visible) {
            Button(confirmationButton) { Task { await performResolution() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text(confirmationMessage)
        }
    }

    private var confirmationTitle: String {
        entry.nextAction?.kind == "owner_acceptance" ? "Record this owner acceptance?" : "Create an owner review?"
    }

    private var confirmationButton: String {
        entry.nextAction?.kind == "owner_acceptance" ? "Record owner acceptance" : "Create owner review"
    }

    private var confirmationMessage: String {
        entry.nextAction?.kind == "owner_acceptance"
            ? "This records an owner conclusion for the exact retained evidence. It does not change or claim to repair the system."
            : "This records an evidence-bound owner review for this exact case. It does not change the system. A conclusion must be accepted explicitly later."
    }

    @ViewBuilder private func resolutionAction(_ action: MaatCaseNextAction) -> some View {
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
            Button {
                confirmAction = true
            } label: {
                Label(confirmationButton, systemImage: action.kind == "owner_acceptance" ? "checkmark.circle.fill" : "arrow.triangle.branch")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(sirsiGold)
            .disabled(actionInFlight || (action.kind == "owner_acceptance" && conclusion.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
            .accessibilityHint("Records an evidence-bound owner decision; it does not repair the system.")
            if let actionResult {
                Label(actionResult.summary, systemImage: actionResult.ok ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(actionResult.ok ? .green : .orange)
                if actionResult.ok {
                    Text("Refresh the casebook to read the updated evidence projection.")
                        .sirsiFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            if let actionError {
                Label(actionError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
            }
        }
        .padding(12)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    @MainActor private func performResolution() async {
        guard let action = entry.nextAction else { return }
        actionInFlight = true
        actionError = nil
        if action.kind == "owner_acceptance" {
            actionResult = await SirsiEngine.runResult(args: ["maat", "accept-resolution", "--evidence", action.evidence, "--note", conclusion.trimmingCharacters(in: .whitespacesAndNewlines), "--confirm"])
        } else {
            let check = entry.resource.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? entry.kind : entry.resource
            actionResult = await SirsiEngine.runResult(args: ["maat", "record-resolution", "--check", check, "--message", entry.why.isEmpty ? entry.determination : entry.why, "--origin-evidence", entry.evidence, "--confirm"])
        }
        if actionResult == nil {
            actionError = "Ma'at could not record the decision. The original case remains open and no system state changed."
        }
        actionInFlight = false
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
}

struct MaatCasebookProjection: Decodable {
    let cases: [MaatCase]
    let summary: MaatCasebookSummary

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
	let resolution: String
	let nextAction: MaatCaseNextAction?
	let systemOne: MaatSystemOneVerdict?
	let systemOneCalibration: MaatSystemOneCalibration?

    init(id: String, time: String, kind: String, category: String, status: String,
         priority: MaatCasePriority, requester: String, resource: String,
         affected: String, determination: String, assessed: String, why: String,
         evidence: String, resolution: String = "", nextAction: MaatCaseNextAction? = nil, systemOne: MaatSystemOneVerdict? = nil, systemOneCalibration: MaatSystemOneCalibration? = nil) {
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
		self.resolution = resolution
		self.nextAction = nextAction
		self.systemOne = systemOne
		self.systemOneCalibration = systemOneCalibration
    }

    private enum CodingKeys: String, CodingKey {
        case id, time, kind, category, status, priority, requester, resource
        case affected, determination, assessed, why, evidence, resolution
        case nextAction = "next_action"
		case systemOne = "system_one"
		case systemOneCalibration = "system_one_calibration"
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
		resolution = try values.decodeIfPresent(String.self, forKey: .resolution) ?? ""
		nextAction = try values.decodeIfPresent(MaatCaseNextAction.self, forKey: .nextAction)
		systemOne = try values.decodeIfPresent(MaatSystemOneVerdict.self, forKey: .systemOne)
		systemOneCalibration = try values.decodeIfPresent(MaatSystemOneCalibration.self, forKey: .systemOneCalibration)
    }

    var searchText: String {
        [time, kind, category, status, requester, resource, affected,
         determination, assessed, why, evidence, resolution, nextAction?.title ?? "", nextAction?.detail ?? "", systemOne?.gate ?? "", systemOne?.subject.headSHA ?? "", systemOneCalibration?.screenEvidence ?? "", systemOneCalibration?.frontierEvidence ?? ""].joined(separator: " ")
    }
}

struct MaatSystemOneVerdict: Decodable {
    let featherWeight: Int
    let gate: String
    let confidence: Double
    let subject: MaatSystemOneSubject
    let floor: MaatSystemOneFloor
    let escalation: MaatSystemOneEscalation?

    enum CodingKeys: String, CodingKey {
        case featherWeight = "feather_weight", gate, confidence, subject, floor, escalation
    }
}

struct MaatSystemOneSubject: Decodable {
    let kind: String
    let ref: String
    let headSHA: String
    enum CodingKeys: String, CodingKey { case kind, ref; case headSHA = "head_sha" }
}

struct MaatSystemOneFloor: Decodable {
    let passed: Bool
    let checks: [MaatSystemOneFloorCheck]
}

struct MaatSystemOneFloorCheck: Decodable {
    let name: String
    let passed: Bool
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

struct MaatCaseNextAction: Decodable {
    let kind: String
    let title: String
    let detail: String
    let evidence: String
    let requiresConfirmation: Bool

    enum CodingKeys: String, CodingKey {
        case kind, title, detail, evidence
        case requiresConfirmation = "requires_confirmation"
    }

    init(kind: String, title: String, detail: String, evidence: String, requiresConfirmation: Bool) {
        self.kind = kind
        self.title = title
        self.detail = detail
        self.evidence = evidence
        self.requiresConfirmation = requiresConfirmation
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        kind = try values.decodeIfPresent(String.self, forKey: .kind) ?? "owner_review"
        title = try values.decodeIfPresent(String.self, forKey: .title) ?? "Review the retained evidence"
        detail = try values.decodeIfPresent(String.self, forKey: .detail) ?? "Record a new evidence-bound owner decision."
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
}

struct MaatKnowledgeProjection: Decodable {
    let items: [MaatKnowledgeItem]
    let total: Int
    let withheld: Int

    static let snapshotPreview = MaatKnowledgeProjection(
        items: [
            MaatKnowledgeItem(id: "knowledge-1", title: "Release evidence", summary: "A retained local note about the current package and review state.", references: [MaatKnowledgeReference(type: "source", value: "local archive")]),
            MaatKnowledgeItem(id: "knowledge-2", title: "Operator handback", summary: "A recorded outcome that can be connected to a Ma'at decision.", references: [MaatKnowledgeReference(type: "source", value: "activity ledger")]),
        ],
        total: 2,
        withheld: 0
    )
}

struct MaatKnowledgeItem: Decodable, Identifiable {
    let id: String
    let title: String
    let summary: String
    let references: [MaatKnowledgeReference]

    init(id: String, title: String, summary: String, references: [MaatKnowledgeReference]) {
        self.id = id
        self.title = title
        self.summary = summary
        self.references = references
    }

    private enum CodingKeys: String, CodingKey { case title, summary, references }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        title = try values.decodeIfPresent(String.self, forKey: .title) ?? "Untitled knowledge"
        summary = try values.decodeIfPresent(String.self, forKey: .summary) ?? ""
        references = try values.decodeIfPresent([MaatKnowledgeReference].self, forKey: .references) ?? []
        let sourceKey = ([title, summary] + references.map { $0.type + ":" + $0.value }).joined(separator: "\u{0}")
        id = sourceKey.data(using: .utf8)?.base64EncodedString() ?? UUID().uuidString
    }

    var source: String? {
        references.first(where: { $0.type == "source" })?.value
    }

    var searchText: String {
        ([title, summary] + references.map { $0.type + " " + $0.value }).joined(separator: " ")
    }
}

struct MaatKnowledgeReference: Decodable, Identifiable {
    let type: String
    let value: String
    var id: String { type + ":" + value }
}
