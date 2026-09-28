import SwiftUI

// StackLabCatalogView renders the local recipe and wing contracts that make a
// Stack Lab workspace independently upgradeable. It reads only the canonical
// Go projection; this view does not enumerate the filesystem or infer remote
// registry status. The doctor remains the authority for remote canonicality.
struct StackLabCatalogView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var catalog: StackLabCatalog?
    @State private var loadError: String?
    @State private var loading = true

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Stack Lab recipes")
            ProjectBar(engine: engine) { Task { await load() } }
            Group {
                if engine.projectRoot == nil {
                    noProjectState
                } else if loading {
                    ProgressView("Reading local Stack Lab contracts…")
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let loadError {
                    failureState(loadError)
                } else if let catalog {
                    catalogBody(catalog)
                } else {
                    failureState("Pantheon did not receive a local Stack Lab catalog. No contract inventory was inferred.")
                }
            }
        }
        .task {
            engine.loadProjectRoot()
            await load()
        }
    }

    private var noProjectState: some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "square.3.layers.3d")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(gold)
            Text("Choose the Pantheon project")
                .sirsiFont(.headline)
            Text("Choose the repository above to browse its actual Stack Lab recipes, components, tests, and upgrade steps. Pantheon does not scan an arbitrary home directory for a catalog.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func failureState(_ message: String) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .sirsiFont(24, weight: .semibold)
                .foregroundStyle(.orange)
            Text("Stack Lab needs your next step")
                .sirsiFont(.headline)
            Text(message)
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            VStack(alignment: .leading, spacing: 6) {
                recoveryStep("1", "Refresh this selected project if its files were just updated.")
                recoveryStep("2", "Use Change above if this is not the repository you meant to inspect.")
                recoveryStep("3", "Run Stack Lab Doctor to identify the exact record and continue its Ma'at evidence route when source repair needs review.")
            }
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.orange.opacity(0.08)))
            HStack(spacing: 10) {
                Button("Refresh selected project") { Task { await load() } }
                    .buttonStyle(.borderedProminent)
                    .tint(gold)
                    .accessibilityHint("Reads the currently selected project's Stack Lab catalog again without changing it.")
                NavLink { StackLabView(engine: engine) } label: {
                    Label("Open Stack Lab Doctor", systemImage: "stethoscope")
                }
                .buttonStyle(.bordered)
                .accessibilityHint("Checks the selected project's canonical Stack Lab records and routes unresolved findings to Ma'at evidence.")
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        .padding(20)
    }

    private func recoveryStep(_ number: String, _ detail: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Text(number)
                .sirsiFont(.caption, weight: .bold)
                .foregroundStyle(gold)
                .frame(width: 16, height: 16)
                .background(Circle().fill(gold.opacity(0.14)))
            Text(detail)
                .sirsiFont(.subheadline)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private func catalogBody(_ catalog: StackLabCatalog) -> some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 14) {
                    summary(catalog)
                    ForEach(catalog.entries) { entry in
                        NavLink { StackLabContractDetail(engine: engine, entry: entry) } label: {
                            StackLabCatalogRow(entry: entry)
                        }
                        .buttonStyle(.plain)
                    }
                    if !catalog.unknown.isEmpty {
                        unknownContracts(catalog.unknown)
                    }
                }
                .padding(16)
            }
            Divider()
            HStack {
                Text("Local source contracts · remote authority remains in Doctor")
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

    private func summary(_ catalog: StackLabCatalog) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Replaceable product recipes")
                .sirsiFont(.title3, weight: .bold)
            Text(catalog.complete
                 ? "Every local Stack Lab contract was read and structurally projected. This is not a remote pin, build, or release verdict."
                 : "One or more local contract records could not be read or validated. The workspace is not complete until each item is resolved.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                catalogMetric("Contracts", catalog.entries.count, .secondary)
                catalogMetric("Unreadable", catalog.unknown.count, catalog.unknown.isEmpty ? .green : .orange)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private func catalogMetric(_ label: String, _ value: Int, _ tint: Color) -> some View {
        Text("\(value) \(label)")
            .sirsiFont(.caption, weight: .semibold)
            .padding(.horizontal, 8)
            .padding(.vertical, 5)
            .background(Capsule().fill(tint.opacity(0.12)))
            .foregroundStyle(tint)
    }

    private func unknownContracts(_ unknown: [String]) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Label("Unreadable or malformed contracts", systemImage: "exclamationmark.triangle.fill")
                .sirsiFont(.headline)
                .foregroundStyle(.orange)
            Text("Repair or restore the named source record, then refresh. Pantheon will not hide it or call the workspace complete.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
            ForEach(unknown, id: \.self) { item in
                Text(item)
                    .sirsiFont(.caption, design: .monospaced)
                    .textSelection(.enabled)
            }
        }
        .padding(13)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.orange.opacity(0.08)))
    }

    @MainActor private func load() async {
        guard engine.projectRoot != nil else {
            catalog = nil
            loadError = nil
            loading = false
            return
        }
        loading = true
        loadError = nil
        let data = await SirsiEngine.runJSON(args: ["stacklab", "catalog", "--json"])
        if let decoded = try? JSONDecoder().decode(StackLabCatalog.self, from: data) {
            catalog = decoded
        } else {
            catalog = nil
            loadError = "Pantheon could not decode the canonical local Stack Lab catalog. No component or recipe was inferred; retry after repairing the selected repository."
        }
        loading = false
    }
}

private struct StackLabCatalogRow: View {
    let entry: StackLabCatalogEntry

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: entry.kind == "recipe" ? "cube.transparent" : "flag.checkered")
                .foregroundStyle(gold)
            VStack(alignment: .leading, spacing: 4) {
                Text(entry.id)
                    .sirsiFont(.headline)
                Text(entry.purpose.isEmpty ? entry.sourcePath : entry.purpose)
                    .sirsiFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(3)
                Text(entry.sourcePath)
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(.secondary)
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
}

private struct StackLabContractDetail: View {
    @ObservedObject var engine: SirsiEngine
    let entry: StackLabCatalogEntry

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: entry.kind == "recipe" ? "Recipe" : "Wing")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    Text(entry.id)
                        .sirsiFont(.title3, weight: .bold)
                    Text(entry.purpose)
                        .sirsiFont(.subheadline)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    contractFact("Source contract", entry.sourcePath)
                    if !entry.wing.isEmpty { contractFact("Wing", entry.wing) }
                    if !entry.product.isEmpty { contractFact("Product", entry.product) }
                    if entry.version > 0 { contractFact("Version", String(entry.version)) }
                    if !entry.nextAction.isEmpty { contractFact("Next action", entry.nextAction) }
                    if entry.components.contains(where: { $0.id == "stacklab-apollo-run-planner" }) {
                        apolloRecipeHandoff
                    }
                    if entry.components.contains(where: { $0.id == "maat-host-health-screen" }) {
                        maatHealthRecipeHandoff
                    }
                    ForEach(entry.components) { component in
                        StackLabComponentCard(component: component)
                    }
                }
                .padding(16)
            }
        }
    }

    private var apolloRecipeHandoff: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Fill the Apollo resource recipe", systemImage: "slider.horizontal.3")
                .sirsiFont(.headline)
            Text("Choose the resident inference route, the measured machine, explicit CPU cores, unified memory, swap ceiling, and chip estates. Creating this plan does not start inference; Apollo opens the live telemetry surface after SNE admits a session.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            NavLink { ApolloRunPlannerView(engine: engine) } label: {
                Label("Open Apollo recipe", systemImage: "arrow.right.circle.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    // Stack Lab records component metadata, but it is not a shell launcher.
    // Each runnable product route is deliberately named here and routes into
    // the native owner surface. That keeps a recipe's explanatory strings from
    // becoming executable input while still giving an operator a complete path.
    private var maatHealthRecipeHandoff: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("Observe this Mac with Ma'at", systemImage: "waveform.path.ecg")
                .sirsiFont(.headline)
            Text("Run one local System One health observation, inspect every finding, then choose its bounded repair, review, or owner-resolution route. Recording the evidence requires confirmation; observation never repairs or changes this Mac.")
                .sirsiFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            NavLink { MaatWorkspaceView(engine: engine) } label: {
                Label("Open Ma'at System One", systemImage: "arrow.right.circle.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(gold)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private func contractFact(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(label).sirsiFont(.caption, weight: .semibold).foregroundStyle(gold)
            Text(value).sirsiFont(.caption, design: .monospaced).textSelection(.enabled)
        }
    }
}

private struct StackLabComponentCard: View {
    let component: StackLabRecipeComponent

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            Text(component.id).sirsiFont(.headline)
            componentList("Source", component.source)
            componentList("Tests", component.tests)
            componentList("Inputs", component.inputs)
            componentList("Outputs", component.outputs)
            if !component.writes.isEmpty { componentList("Writes", component.writes) }
            componentList("Upgrade recipe", component.upgradeRecipe)
        }
        .padding(13)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.05)))
    }

    private func componentList(_ title: String, _ values: [String]) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title).sirsiFont(.caption, weight: .semibold).foregroundStyle(gold)
            ForEach(values, id: \.self) { value in
                Text(value).sirsiFont(.caption).fixedSize(horizontal: false, vertical: true)
            }
        }
    }
}

private struct StackLabCatalog: Decodable {
    let entries: [StackLabCatalogEntry]
    let unknown: [String]

    enum CodingKeys: String, CodingKey { case entries, unknown }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        entries = try values.decodeIfPresent([StackLabCatalogEntry].self, forKey: .entries) ?? []
        unknown = try values.decodeIfPresent([String].self, forKey: .unknown) ?? []
    }
    var complete: Bool { unknown.isEmpty }
}

private struct StackLabCatalogEntry: Decodable, Identifiable {
    let id: String
    let kind: String
    let sourcePath: String
    let wing: String
    let product: String
    let version: Int
    let purpose: String
    let components: [StackLabRecipeComponent]
    let nextAction: String

    enum CodingKeys: String, CodingKey {
        case id, kind, wing, product, version, purpose, components
        case sourcePath = "source_path"
        case nextAction = "next_action"
    }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        id = try values.decode(String.self, forKey: .id)
        kind = try values.decode(String.self, forKey: .kind)
        sourcePath = try values.decode(String.self, forKey: .sourcePath)
        wing = try values.decodeIfPresent(String.self, forKey: .wing) ?? ""
        product = try values.decodeIfPresent(String.self, forKey: .product) ?? ""
        version = try values.decodeIfPresent(Int.self, forKey: .version) ?? 0
        purpose = try values.decodeIfPresent(String.self, forKey: .purpose) ?? ""
        components = try values.decodeIfPresent([StackLabRecipeComponent].self, forKey: .components) ?? []
        nextAction = try values.decodeIfPresent(String.self, forKey: .nextAction) ?? ""
    }
}

private struct StackLabRecipeComponent: Decodable, Identifiable {
    let id: String
    let source: [String]
    let tests: [String]
    let inputs: [String]
    let outputs: [String]
    let writes: [String]
    let upgradeRecipe: [String]

    enum CodingKeys: String, CodingKey {
        case id, source, tests, inputs, outputs, writes
        case upgradeRecipe = "upgrade_recipe"
    }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        id = try values.decode(String.self, forKey: .id)
        source = try values.decodeIfPresent([String].self, forKey: .source) ?? []
        tests = try values.decodeIfPresent([String].self, forKey: .tests) ?? []
        inputs = try values.decodeIfPresent([String].self, forKey: .inputs) ?? []
        outputs = try values.decodeIfPresent([String].self, forKey: .outputs) ?? []
        writes = try values.decodeIfPresent([String].self, forKey: .writes) ?? []
        upgradeRecipe = try values.decodeIfPresent([String].self, forKey: .upgradeRecipe) ?? []
    }
}
