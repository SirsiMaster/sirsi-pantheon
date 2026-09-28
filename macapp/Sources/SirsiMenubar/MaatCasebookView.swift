import SwiftUI
import UniformTypeIdentifiers

// MaatWorkspaceView joins decision quality and retained local knowledge under
// one operator authority. Seshat's ingestion compatibility commands are not a
// second Pantheon surface: Ma'at is where a person inspects the resulting
// evidence, classifications, and decisions.
struct MaatWorkspaceView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
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
    @State private var confirmHostTriage = false
    @State private var hostTriageInFlight = false
    @State private var hostTriageResult: CommandResult?
    @State private var hostTriageError: String?

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
        .confirmationDialog("Observe this Mac for Ma'at System One?", isPresented: $confirmHostTriage, titleVisibility: .visible) {
            Button("Observe and record") { Task { await recordHostTriage() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Ma'at will run one local health observation, hash that exact report, and record its deterministic result in Casebook. It will not repair services, kill processes, install software, or authorize other work.")
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
            NavLink { ResultView(engine: engine, title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                Label("Run a fresh Ma'at audit", systemImage: "checkmark.seal")
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
                    hostTriageControl
                    releasePreflightControl
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
            Text("This is an empty evidence history, not a pass or a failure. Choose Observe this Mac to create one qualified local health screen, or record a closed typed screen from another qualified Pantheon producer. Ma'at always shows the deterministic gate before retaining it here.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Text("System One does not invent a screen: until a bounded observation is hashed and validated, Ma'at shows no result and grants no authority.")
                .sirsiFont(.caption)
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

    // This is the local System One entry point: Ma'at owns the observation,
    // normalizes and hashes it, then records the exact evidence only after an
    // explicit confirmation. It replaces the previous "bring your own JSON"
    // dead-end for ordinary workstation health without making a health screen
    // an implicit repair or execution authority.
    private var hostTriageControl: some View {
        VStack(alignment: .leading, spacing: 9) {
            Text("Observe this Mac")
                .sirsiFont(.headline)
            Text("Create one local, evidence-bound System One health screen. Ma'at will show every active finding with its bounded repair, review, and owner-resolution route in Casebook.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            SnapshotActionButton(disabled: hostTriageInFlight) {
                confirmHostTriage = true
            } label: {
                Label("Observe and record", systemImage: "waveform.path.ecg")
                    .frame(maxWidth: .infinity)
            }
            .accessibilityHint("Runs one local diagnostic and records its exact hashed System One result after confirmation. It does not repair the Mac.")
            if hostTriageInFlight {
                ProgressView("Observing this Mac…")
                    .sirsiFont(.caption)
            }
            if let result = hostTriageResult {
                let gate = hostTriageGate(result)
                Label(result.summary, systemImage: hostTriageSymbol(gate: gate, result: result))
                    .sirsiFont(.caption)
                    .foregroundStyle(hostTriageTint(gate: gate, result: result))
                    .fixedSize(horizontal: false, vertical: true)
                if result.ok, gate == "pass" {
                    Text("The Casebook was refreshed from this exact local observation.")
                        .sirsiFont(.caption)
                        .foregroundStyle(.secondary)
                } else if result.ok {
                    Text("The observation was retained. Open the Casebook to follow its repair, review, or owner-resolution route.")
                        .sirsiFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            if let hostTriageError {
                Label(hostTriageError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
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
            SnapshotActionButton(disabled: screenImportInFlight) {
                showScreenPicker = true
            } label: {
                Label("Choose System One JSON", systemImage: "doc.badge.plus")
                    .frame(maxWidth: .infinity)
            }
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
                Button {
                    Task { await inspectReleasePreflight() }
                } label: {
                    Label("Inspect release contract", systemImage: "checklist")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(gold)
                .disabled(releasePreflightInFlight)
            }
            credentialPreflightControl
            if releasePreflightInFlight {
                ProgressView("Inspecting source contract…")
                    .sirsiFont(.caption)
            }
            if let releasePreflight {
                releasePreflightSummary(releasePreflight)
                Button {
                    confirmReleasePreflight = true
                } label: {
                    Label(releasePreflightRecorded ? "Recorded in Ma'at Casebook" : "Record in Ma'at Casebook", systemImage: releasePreflightRecorded ? "checkmark.seal.fill" : "checkmark.shield")
                }
                .buttonStyle(.bordered)
                .disabled(releasePreflightInFlight || releasePreflightRecorded)
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
                Button("Try inspection again") { Task { await inspectReleasePreflight() } }
                    .buttonStyle(.bordered)
                    .disabled(releasePreflightInFlight || engine.projectRoot == nil)
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
            Button {
                Task { await inspectCredentialPreflight() }
            } label: {
                Label("Check local Developer ID readiness", systemImage: "checkmark.shield")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .disabled(credentialPreflightInFlight)
            if credentialPreflightInFlight {
                ProgressView("Checking public certificate metadata…")
                    .sirsiFont(.caption)
            }
            if let credentialPreflight {
                credentialPreflightSummary(credentialPreflight)
                Button {
                    confirmCredentialPreflight = true
                } label: {
                    Label(credentialPreflightRecorded ? "Recorded in Ma'at Casebook" : "Record readiness evidence", systemImage: credentialPreflightRecorded ? "checkmark.seal.fill" : "checkmark.shield")
                }
                .buttonStyle(.bordered)
                .disabled(credentialPreflightInFlight || credentialPreflightRecorded)
            }
            if let credentialPreflightError {
                Label(credentialPreflightError, systemImage: "exclamationmark.triangle.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(.orange)
                    .fixedSize(horizontal: false, vertical: true)
                Button("Try readiness check again") { Task { await inspectCredentialPreflight() } }
                    .buttonStyle(.bordered)
                    .disabled(credentialPreflightInFlight)
            }
        }
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
        VStack(alignment: .leading, spacing: 7) {
            Label("Release credentials still need a protected proof", systemImage: "key.horizontal")
                .sirsiFont(.subheadline, weight: .semibold)
                .foregroundStyle(.orange)
            Text("Team \(preflight.teamID) · \(preflight.developerIdentities.count) required Developer ID identities observed")
                .sirsiFont(.caption)
                .foregroundStyle(.secondary)
            ForEach(preflight.verdict.floor.checks, id: \.name) { check in
                Label(check.detail, systemImage: check.passed ? "checkmark.circle.fill" : "xmark.octagon.fill")
                    .sirsiFont(.caption)
                    .foregroundStyle(check.passed ? Color.secondary : Color.orange)
                    .fixedSize(horizontal: false, vertical: true)
            }
            ForEach(preflight.verdict.findings) { finding in
                if !finding.fixHint.isEmpty {
                    Text("Next: \(finding.fixHint)")
                        .sirsiFont(.caption)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            Text("Fingerprint: \(preflight.fingerprint)")
                .sirsiFont(.caption2, design: .monospaced)
                .foregroundStyle(.secondary)
                .lineLimit(1)
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(Color.orange.opacity(0.09)))
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

    @MainActor private func recordHostTriage() async {
        guard !hostTriageInFlight else { return }
        hostTriageInFlight = true
        hostTriageError = nil
        hostTriageResult = await SirsiEngine.runResult(args: ["maat", "triage", "--confirm"])
        if hostTriageResult == nil {
            hostTriageError = "Ma'at could not record the local observation. No System One outcome was inferred or accepted. Try again, then inspect the local Casebook if the problem persists."
        } else if hostTriageResult?.ok == true {
            await load()
        }
        hostTriageInFlight = false
    }

    // A Casebook append succeeding is not synonymous with a healthy result.
    // The command carries the deterministic System One gate as evidence, and
    // this native projection preserves it instead of showing a green success
    // treatment for a recorded block or escalation.
    private func hostTriageGate(_ result: CommandResult) -> String {
        result.evidence.first(where: { $0.label == "Deterministic gate" })?.value.lowercased() ?? ""
    }

    private func hostTriageSymbol(gate: String, result: CommandResult) -> String {
        guard result.ok else { return "exclamationmark.triangle.fill" }
        switch gate {
        case "pass": return "checkmark.seal.fill"
        case "changes": return "exclamationmark.circle.fill"
        case "block": return "hand.raised.fill"
        case "escalate": return "arrow.triangle.branch"
        default: return "checkmark.circle.fill"
        }
    }

    private func hostTriageTint(gate: String, result: CommandResult) -> Color {
        guard result.ok else { return .orange }
        switch gate {
        case "pass": return .green
        case "changes": return .orange
        case "block": return .red
        case "escalate": return gold
        default: return .secondary
        }
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
    @State private var confirmKnowledgeRefresh = false
    @State private var knowledgeRefreshInFlight = false
    @State private var knowledgeRefreshResult: String?
    @State private var knowledgeRefreshError: String?

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
    @State private var confirmKnowledgeRefresh = false
    @State private var knowledgeRefreshInFlight = false
    @State private var knowledgeRefreshResult: String?
    @State private var knowledgeRefreshError: String?

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
                    knowledgeRefreshControl
                    searchField
                    knowledgeList(filtered(knowledge.items))
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

    private var knowledgeRefreshControl: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Keep local knowledge current")
                .sirsiFont(.headline)
            Text("Refresh reads the configured local sources into Ma'at's cache. Review the scope before confirming; Ma'at does not export, open a browser, or make a remote decision from this screen.")
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
            knowledgeRefreshError = "Ma'at could not refresh local knowledge. The current cache remains available. Check the reported source issue, then retry this confirmed refresh. \(detail)"
        }
        knowledgeRefreshInFlight = false
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
    @State private var copiedFindingID: String?

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
						systemOneFloor(systemOne.floor)
						systemOneFindings(systemOne.findings)
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
        switch entry.nextAction?.kind {
        case "maat_repair": return "Apply Ma'at's bounded recovery?"
        case "owner_acceptance": return "Record this owner acceptance?"
        case "system_one_floor_recovery": return "Record this recovery review?"
        default: return "Create an owner review?"
        }
    }

    private var confirmationButton: String {
        switch entry.nextAction?.kind {
        case "maat_repair": return "Apply bounded recovery"
        case "owner_acceptance": return "Record owner acceptance"
        case "system_one_floor_recovery": return "Record recovery review"
        default: return "Create owner review"
        }
    }

    private var confirmationMessage: String {
        entry.nextAction?.kind == "maat_repair"
            ? "Ma'at will re-check the exact managed disabled labels, change only the bounded verified set, re-check the same diagnostic, and retain the outcome. It will not touch unrelated services."
            : entry.nextAction?.kind == "owner_acceptance"
            ? "This records an owner conclusion for the exact retained evidence. It does not change or claim to repair the system."
            : entry.nextAction?.kind == "system_one_floor_recovery"
                ? "This records the evidence-bound recovery review after you complete the stated correction. It does not run a repair; the case remains open until the new evidence is accepted."
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
                Label(confirmationButton, systemImage: action.kind == "maat_repair" ? "wrench.and.screwdriver.fill" : action.kind == "owner_acceptance" ? "checkmark.circle.fill" : "arrow.triangle.branch")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
            .disabled(actionInFlight || (action.kind == "owner_acceptance" && conclusion.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
            .accessibilityHint(action.kind == "maat_repair" ? "Runs only Ma'at's named bounded recovery after confirmation, then retains its post-repair evidence." : "Records an evidence-bound owner decision; it does not repair the system.")
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

    @MainActor private func performResolution() async {
        guard let action = entry.nextAction else { return }
        actionInFlight = true
        actionError = nil
        if action.kind == "maat_repair" {
            guard action.actionID == "launchd-disabled" else {
                actionError = "Ma'at refused an unknown repair reference. No system state changed."
                actionInFlight = false
                return
            }
            actionResult = await SirsiEngine.runResult(args: ["maat", "repair", "launchd-disabled", "--confirm"])
        } else if action.kind == "owner_acceptance" {
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

    // A qualified producer may describe a bounded remediation in a System One
    // finding. Treat it as evidence and an operator aid, never as executable
    // input: the app shows and copies it, while the exact owner-review path
    // remains responsible for acknowledgement or a separately safe repair.
    @ViewBuilder private func systemOneFindings(_ findings: [MaatSystemOneFinding]) -> some View {
        if !findings.isEmpty {
            VStack(alignment: .leading, spacing: 9) {
                Text(findings.count == 1 ? "Screen finding" : "Screen findings")
                    .sirsiFont(.caption, weight: .bold)
                    .foregroundStyle(.secondary)
                ForEach(findings) { finding in
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
                        if !finding.fixHint.isEmpty {
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
                        }
                        if !finding.evidence.isEmpty {
                            Text("Evidence: \(finding.evidence)")
                                .sirsiFont(.caption2, design: .monospaced)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
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
        let caseFields = [
            time, kind, category, status, requester, resource, affected,
            determination, assessed, why, evidence, resolution,
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
    let featherWeight: Int
    let gate: String
    let confidence: Double
    let subject: MaatSystemOneSubject
    let floor: MaatSystemOneFloor
    let escalation: MaatSystemOneEscalation?
    let model: MaatSystemOneModel
    let findings: [MaatSystemOneFinding]

    enum CodingKeys: String, CodingKey {
        case featherWeight = "feather_weight", gate, confidence, subject, floor, escalation, model, findings
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
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

struct MaatSystemOneFinding: Decodable, Identifiable {
    let id: String
    let severity: String
    let category: String
    let file: String
    let line: Int
    let claim: String
    let evidence: String
    let fixHint: String

    enum CodingKeys: String, CodingKey {
        case id, severity, category, file, line, claim, evidence
        case fixHint = "fix_hint"
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
    }

    var location: String {
        guard !file.isEmpty else { return "" }
        return line > 0 ? "\(file):\(line)" : file
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

    enum CodingKeys: String, CodingKey {
        case provider, version, local
        case latencyMS = "latency_ms"
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
private struct MaatReleaseContractPreflight: Decodable {
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
private struct MaatReleaseCredentialPreflight: Decodable {
    let teamID: String
    let fingerprint: String
    let developerIdentities: [MaatReleaseSigningIdentity]
    let notarizationObserved: Bool
    let verdict: MaatSystemOneVerdict
    let decisionEvidence: String

    enum CodingKeys: String, CodingKey {
        case teamID = "team_id"
        case fingerprint
        case developerIdentities = "developer_identities"
        case notarizationObserved = "notarization_observed"
        case verdict
        case decisionEvidence = "decision_evidence"
    }

    static func decode(_ raw: String) -> MaatReleaseCredentialPreflight? {
        guard let start = raw.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: Data(raw[start...].utf8))
    }
}

private struct MaatReleaseSigningIdentity: Decodable, Identifiable {
    let kind: String
    let name: String
    let fingerprint: String
    var id: String { "\(kind):\(fingerprint)" }
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
