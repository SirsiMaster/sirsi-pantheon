import SwiftUI

// MaatWorkspaceView joins decision quality and retained local knowledge under
// one operator authority. Seshat's ingestion compatibility commands are not a
// second Pantheon surface: Ma'at is where a person inspects the resulting
// evidence, classifications, and decisions.
struct MaatWorkspaceView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var section: MaatWorkspaceSection = .decisions
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
            case .decisions:
                MaatCasebookView(engine: engine, preloaded: preloadedCasebook, showsBackBar: false)
            case .knowledge:
                MaatKnowledgeView(engine: engine, preloaded: preloadedKnowledge, showsBackBar: false)
            }
        }
    }
}

private enum MaatWorkspaceSection: String, CaseIterable, Identifiable {
    case decisions, knowledge

    var id: String { rawValue }
    var title: String { self == .decisions ? "Decisions" : "Knowledge" }
    var symbol: String { self == .decisions ? "checkmark.seal" : "books.vertical" }
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
        if summary.open > 0 { return "(summary.open) decision\(summary.open == 1 ? "" : "s") still needs attention." }
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
                    NavLink { MaatCaseDetailView(entry: entry) } label: {
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
    let entry: MaatCase

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
                }
                .padding(16)
            }
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

    init(id: String, time: String, kind: String, category: String, status: String,
         priority: MaatCasePriority, requester: String, resource: String,
         affected: String, determination: String, assessed: String, why: String,
         evidence: String) {
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
    }

    private enum CodingKeys: String, CodingKey {
        case id, time, kind, category, status, priority, requester, resource
        case affected, determination, assessed, why, evidence
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
    }

    var searchText: String {
        [time, kind, category, status, requester, resource, affected,
         determination, assessed, why, evidence].joined(separator: " ")
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
