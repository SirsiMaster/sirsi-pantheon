import SwiftUI

struct PantheonDesktopView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
    @StateObject private var session = PantheonWorkspaceSession()
    @State private var search = ""
    @FocusState private var searchFocused: Bool
    private let previewWorkspace: PantheonWorkspace?
    private let previewContent: AnyView?

    init(engine: SirsiEngine, previewWorkspace: PantheonWorkspace? = nil, previewContent: AnyView? = nil) {
        self.engine = engine
        self.previewWorkspace = previewWorkspace
        self.previewContent = previewContent
    }

    private var section: PantheonWorkspace {
        previewWorkspace ?? session.section
    }

    private var nav: Nav { session.navigation(for: section) }

    var body: some View {
        HStack(spacing: 0) {
            sidebar.frame(width: 218)
            Rectangle().fill(PantheonTheme.separator).frame(width: 1)
            VStack(spacing: 0) {
                workspaceHeader
                Rectangle().fill(PantheonTheme.separator).frame(height: 1)
                Group {
                    if snapshotMode, let previewContent { previewContent }
                    else if let top = nav.stack.last { top.view.id(top.id) }
                    else { workspaceView(section) }
                }
                .id(section)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .environmentObject(nav)
                .environment(\.openPantheonWorkspace, select)
                .environment(\.openPantheonOwnerActions, {
                    session.open(.maat, detail: OwnerActionsListView(engine: engine))
                })
            }
            .background(PantheonTheme.canvas)
        }
        .foregroundStyle(PantheonTheme.text)
        .preferredColorScheme(.dark)
        .tint(gold)
        .background(PantheonTheme.canvas)
        .onChange(of: engine.pendingOwnerItemID) { id in
            guard let id else { return }
            session.open(.maat, detail: OwnerActionView(engine: engine, itemID: id))
            engine.pendingOwnerItemID = nil
        }
        .task {
            guard !snapshotMode else { return }
            engine.loadProjectRoot()
            engine.loadActivity()
            engine.loadRunReport()
        }
    }

    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 11) {
                PantheonBrandMark(size: 36)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Pantheon").sirsiFont(18, weight: .semibold)
                    Text("Sirsi workspace").sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
                }
            }
            .padding(.horizontal, 18).padding(.top, 23).padding(.bottom, 23)
            HStack(spacing: 7) {
                Image(systemName: "magnifyingglass").foregroundStyle(PantheonTheme.mutedText)
                if snapshotMode {
                    Text("Find a workspace").foregroundStyle(PantheonTheme.mutedText)
                        .frame(maxWidth: .infinity, alignment: .leading)
                } else {
                    TextField("Find a workspace", text: $search)
                    .textFieldStyle(.plain).focused($searchFocused)
                    .onSubmit { if let first = PantheonWorkspace.matching(search).first { select(first) } }
                    .accessibilityLabel("Find a Pantheon workspace")
                }
                if !search.isEmpty {
                    Button { search = "" } label: { Image(systemName: "xmark.circle.fill") }
                        .buttonStyle(.plain).accessibilityLabel("Clear workspace search")
                }
            }
            .sirsiFont(12).padding(9)
            .background(PantheonTheme.panelRaised, in: RoundedRectangle(cornerRadius: 7))
            .padding(.horizontal, 12).padding(.bottom, 18)
            MaybeScroll {
                VStack(alignment: .leading, spacing: 20) {
                    sidebarSection("Workspace", [.command, .activity])
                    sidebarSection("Intelligence & fabric", [.apollo, .mercury, .ra, .maat])
                    sidebarSection("Build & release", [.stackLab, .release])
                    sidebarSection("This Mac", [.horus, .anubis, .osiris, .threads, .fleet, .library])
                    if PantheonWorkspace.matching(search).isEmpty {
                        Text("No matching workspace. Clear the search to see all tools.")
                            .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText).padding(8)
                    }
                }
                .padding(.horizontal, 10).padding(.bottom, 16)
            }
            Spacer(minLength: 8)
            Rectangle().fill(PantheonTheme.separator).frame(height: 1)
            VStack(alignment: .leading, spacing: 5) {
                Label(engine.projectName ?? "This Mac", systemImage: "folder")
                    .sirsiFont(12, weight: .medium).lineLimit(1)
                    .help(engine.projectRoot ?? "Select a project inside a workspace")
                Text("Local control · shared fabric")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            }.padding(18)
        }
        .background(PantheonTheme.sidebar)
        .background {
            Button { searchFocused = true } label: { EmptyView() }
                .keyboardShortcut("k", modifiers: .command).hidden()
        }
    }

    private var workspaceHeader: some View {
        HStack(spacing: 12) {
            Image(systemName: section.symbol).sirsiFont(17, weight: .medium)
                .foregroundStyle(gold).frame(width: 25)
            VStack(alignment: .leading, spacing: 4) {
                Text(section.title).sirsiFont(19, weight: .semibold)
                Text(section.detail).sirsiFont(12).foregroundStyle(PantheonTheme.mutedText).lineLimit(2)
            }
            Spacer(minLength: 12)
            if !nav.atRoot {
                Button { nav.popToRoot() } label: {
                    Label("\(section.title) home", systemImage: "arrow.uturn.backward")
                }.buttonStyle(.borderless).sirsiFont(12)
            }
            // Refresh belongs to the active workflow, whose inputs it retains.
            // Replacing the root here would destroy edited plans and case context.
        }.padding(.horizontal, 26).padding(.vertical, 20)
    }

    @ViewBuilder private func sidebarSection(_ title: String, _ destinations: [PantheonWorkspace]) -> some View {
        let visible = destinations.filter { $0.matches(search) }
        if !visible.isEmpty {
            VStack(alignment: .leading, spacing: 3) {
                Text(title).sirsiFont(11, weight: .medium).foregroundStyle(PantheonTheme.mutedText)
                    .padding(.horizontal, 10).padding(.bottom, 5)
                ForEach(visible) { workspace in
                    Button { select(workspace) } label: {
                        HStack(spacing: 10) {
                            Image(systemName: workspace.symbol).frame(width: 20)
                                .foregroundStyle(section == workspace ? gold : PantheonTheme.mutedText)
                            Text(workspace.title).fontWeight(section == workspace ? .semibold : .regular)
                            Spacer(minLength: 0)
                            if section == workspace { Circle().fill(gold).frame(width: 4, height: 4) }
                        }
                        .sirsiFont(13).padding(.horizontal, 10).padding(.vertical, 9)
                        .contentShape(Rectangle())
                        .background(section == workspace ? PantheonTheme.panelRaised : .clear,
                                    in: RoundedRectangle(cornerRadius: 7))
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Open \(workspace.title): \(workspace.detail)")
                    .accessibilityAddTraits(section == workspace ? .isSelected : [])
                }
            }
        }
    }

    private func select(_ workspace: PantheonWorkspace) {
        session.select(workspace)
    }

    @ViewBuilder private func workspaceView(_ workspace: PantheonWorkspace) -> some View {
        switch workspace {
        case .command: PantheonControlCenterView(engine: engine)
        case .apollo: ApolloRunPlannerView(engine: engine)
        case .mercury: MercuryWorkspaceView(engine: engine)
        case .maat: MaatWorkspaceView(engine: engine)
        case .ra: RaFabricView(engine: engine)
        case .activity: ActivityView(engine: engine)
        case .stackLab: StackLabView(engine: engine)
        case .release: PantheonReleaseWorkspaceView(engine: engine)
        case .threads: ThreadsView(engine: engine)
        case .horus: HorusView(engine: engine)
        case .anubis: AnubisView(engine: engine)
        case .osiris: RiskView(engine: engine)
        case .fleet: FleetView(engine: engine)
        case .library: PantheonLibraryView(engine: engine)
        }
    }
}

enum PantheonWorkspace: String, CaseIterable, Identifiable {
    case command, apollo, mercury, ra, maat, activity, stackLab, release, threads, horus, anubis, osiris, fleet, library
    var id: String { rawValue }
    var title: String {
        switch self {
        case .command: return "Overview"
        case .apollo: return "Apollo"
        case .mercury: return "Mercury"
        case .ra: return "Ra Fabric"
        case .maat: return "Ma’at"
        case .activity: return "Activity"
        case .stackLab: return "Stack Lab"
        case .release: return "Release"
        case .threads: return "Work"
        case .horus: return "Horus"
        case .anubis: return "Anubis"
        case .osiris: return "Osiris"
        case .fleet: return "Fleet"
        case .library: return "All Tools"
        }
    }
    var detail: String {
        switch self {
        case .command: return "Your Mac, intelligence, and connected work"
        case .apollo: return "Choose a model and machine, plan a run, inspect telemetry"
        case .mercury: return "Inspect transport, machines, and the connections between them"
        case .ra: return "Coordinate workers, messages, claims, and handbacks"
        case .maat: return "Understand findings, resolve issues, and retain the evidence"
        case .activity: return "Completed actions and their retained evidence"
        case .stackLab: return "Recipes, components, candidates, and releases"
        case .release: return "Prepare and inspect Pantheon delivery"
        case .threads: return "Active sessions and work across Pantheon"
        case .horus: return "Inspect this Mac’s health and capacity"
        case .anubis: return "Review and recover storage with scoped cleanup"
        case .osiris: return "Risk, checkpoints, and recovery options"
        case .fleet: return "Worker lanes, task progress, and supervision states"
        case .library: return "Find every Pantheon tool and workflow"
        }
    }
    var symbol: String {
        switch self {
        case .command: return "rectangle.3.group"
        case .apollo: return "cpu"
        case .mercury: return "cable.connector.horizontal"
        case .ra: return "point.3.connected.trianglepath.dotted"
        case .maat: return "scalemass"
        case .activity: return "clock.arrow.circlepath"
        case .stackLab: return "square.3.layers.3d"
        case .release: return "shippingbox"
        case .threads: return "person.2"
        case .horus: return "waveform.path.ecg"
        case .anubis: return "externaldrive"
        case .osiris: return "shield"
        case .fleet: return "desktopcomputer"
        case .library: return "square.grid.2x2"
        }
    }
    func matches(_ search: String) -> Bool {
        let query = search.trimmingCharacters(in: .whitespacesAndNewlines)
        return query.isEmpty || (title + " " + detail).localizedStandardContains(query)
    }
    static func matching(_ search: String) -> [PantheonWorkspace] { allCases.filter { $0.matches(search) } }
}

private struct OpenPantheonWorkspaceKey: EnvironmentKey {
    static let defaultValue: ((PantheonWorkspace) -> Void)? = nil
}

private struct OpenPantheonOwnerActionsKey: EnvironmentKey {
    static let defaultValue: (() -> Void)? = nil
}
extension EnvironmentValues {
    var openPantheonOwnerActions: (() -> Void)? {
        get { self[OpenPantheonOwnerActionsKey.self] }
        set { self[OpenPantheonOwnerActionsKey.self] = newValue }
    }
    var openPantheonWorkspace: ((PantheonWorkspace) -> Void)? {
        get { self[OpenPantheonWorkspaceKey.self] }
        set { self[OpenPantheonWorkspaceKey.self] = newValue }
    }
}
