import SwiftUI

// PantheonDesktopView is the product's primary macOS workspace. The menu-bar
// Eye opens this window, but does not replace it. Every first-class operational
// surface is directly reachable here; the existing Nav stack is retained only
// for contextual drill-downs inside a surface.
struct PantheonDesktopView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var section: PantheonWorkspace = .command
    @StateObject private var nav = Nav()

    var body: some View {
        NavigationSplitView {
            VStack(spacing: 0) {
                sidebarIdentity
                Divider()
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 18) {
                        sidebarSection("Operate", [.command, .maat, .ra, .activity])
                        sidebarSection("Build", [.stackLab, .apollo, .release, .threads])
                        sidebarSection("Protect", [.horus, .anubis, .osiris])
                        sidebarSection("Explore", [.fleet, .library])
                    }
                    .padding(12)
                }
            }
            .background(PantheonTheme.sidebar)
            .frame(minWidth: 218)
        } detail: {
            VStack(spacing: 0) {
                workspaceHeader
                Divider()
                if let top = nav.stack.last {
                    top.view
                } else {
                    workspaceView(section)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(PantheonTheme.canvas)
            .environmentObject(nav)
        }
        .navigationSplitViewStyle(.balanced)
        .preferredColorScheme(.dark)
        .tint(emerald)
        .onChange(of: section) { _ in nav.popToRoot() }
        .onChange(of: engine.pendingOwnerItemID) { id in
            guard let id else { return }
            section = .command
            nav.popToRoot()
            nav.push(OwnerActionView(engine: engine, itemID: id))
            engine.pendingOwnerItemID = nil
        }
        .task {
            engine.loadProjectRoot()
            engine.loadActivity()
            engine.loadRunReport()
            await engine.diagnose()
            await engine.loadRouterBoard()
        }
        .dynamicTypeSize(.large)
    }

    private var workspaceHeader: some View {
        HStack(alignment: .firstTextBaseline, spacing: 14) {
            Circle()
                .fill(emerald)
                .frame(width: 8, height: 8)
                .accessibilityLabel("Pantheon is live")
            VStack(alignment: .leading, spacing: 3) {
                Text(section.title)
                    .font(.title2.weight(.bold))
                Text(section.detail)
                    .font(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .lineLimit(1)
            }
            Spacer()
            Button {
                Task { await engine.rescan() }
            } label: {
                Label(engine.busy ? "Refreshing" : "Refresh", systemImage: "arrow.clockwise")
            }
            .disabled(engine.busy)
            .buttonStyle(.bordered)
        }
        .padding(.horizontal, 26)
        .padding(.vertical, 18)
        .background(PantheonTheme.canvas)
    }

    private var sidebarIdentity: some View {
        HStack(spacing: 11) {
            PantheonBrandMark(size: 38)
            VStack(alignment: .leading, spacing: 1) {
                Text("SIRSI")
                    .sirsiFont(.caption, weight: .bold)
                    .tracking(1.2)
                    .foregroundStyle(gold)
                Text("Pantheon")
                    .sirsiFont(.headline, weight: .bold)
                    .foregroundStyle(Color.primary)
            }
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 17)
        .padding(.vertical, 16)
    }

    private func sidebarSection(_ title: String, _ workspaces: [PantheonWorkspace]) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(title.uppercased())
                .sirsiFont(10, weight: .bold)
                .tracking(0.9)
                .foregroundStyle(PantheonTheme.mutedText.opacity(0.78))
                .padding(.horizontal, 8)
            ForEach(workspaces) { workspace in
                workspaceButton(workspace)
            }
        }
    }

    private func workspaceButton(_ workspace: PantheonWorkspace) -> some View {
        let selected = workspace == section
        return Button {
            section = workspace
            nav.popToRoot()
        } label: {
            HStack(spacing: 10) {
                Image(systemName: workspace.symbol)
                    .sirsiFont(14, weight: .semibold)
                    .frame(width: 20)
                    .foregroundStyle(selected ? emerald : PantheonTheme.mutedText)
                Text(workspace.title)
                    .sirsiFont(13, weight: selected ? .semibold : .regular)
                    .foregroundStyle(selected ? Color.white : PantheonTheme.mutedText)
                Spacer(minLength: 0)
            }
            .padding(.horizontal, 9)
            .padding(.vertical, 8)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: 8)
                    .fill(selected ? Color.white.opacity(0.06) : Color.clear)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(selected ? PantheonTheme.gold.opacity(0.66) : Color.clear, lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
        .contentShape(Rectangle())
        .accessibilityLabel("Open \(workspace.title): \(workspace.detail)")
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    @ViewBuilder private func workspaceView(_ workspace: PantheonWorkspace) -> some View {
        switch workspace {
        case .command: PantheonControlCenterView(engine: engine)
        case .maat: MaatWorkspaceView(engine: engine)
        case .ra: RaFabricView(engine: engine)
        case .activity: ActivityView(engine: engine)
        case .stackLab: StackLabView(engine: engine)
        case .apollo: ApolloRunPlannerView(engine: engine)
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

private enum PantheonWorkspace: String, CaseIterable, Identifiable {
    case command, maat, ra, activity, stackLab, apollo, release, threads, horus, anubis, osiris, fleet, library

    var id: String { rawValue }

    var title: String {
        switch self {
        case .command: return "Command Center"
        case .maat: return "Ma’at"
        case .ra: return "Ra Fabric"
        case .activity: return "Activity"
        case .stackLab: return "Stack Lab"
        case .apollo: return "Apollo"
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
        // Command Center is the place an operator begins whether the Mac needs
        // recovery or is already healthy. Do not frame every opening as a
        // problem: the live state and its next in-app action say which it is.
        case .command: return "Live local state, guided control, and your next action"
        case .maat: return "Evidence, decisions, and guided resolution"
        case .ra: return "Fabric state, claims, handbacks, and messages"
        case .activity: return "Recent work and retained operational evidence"
        case .stackLab: return "Recipes, components, and release readiness"
        case .apollo: return "Inference routes, resource envelopes, and telemetry"
        case .release: return "Prepare a signed Pantheon delivery from this app"
        case .threads: return "Active work across Pantheon"
        case .horus: return "System health and capacity"
        case .anubis: return "Guided storage recovery and cleanup"
        case .osiris: return "Risk, checkpoints, and recovery posture"
        case .fleet: return "Connected Pantheon and Horus instances"
        case .library: return "Every Pantheon surface in one place"
        }
    }

    var symbol: String {
        switch self {
        case .command: return "rectangle.3.group.fill"
        case .maat: return "checkmark.seal.fill"
        case .ra: return "point.3.connected.trianglepath.dotted"
        case .activity: return "clock.arrow.circlepath"
        case .stackLab: return "square.3.layers.3d"
        case .apollo: return "cpu"
        case .release: return "shippingbox.fill"
        case .threads: return "circle.dotted"
        case .horus: return "waveform.path.ecg"
        case .anubis: return "trash"
        case .osiris: return "shield"
        case .fleet: return "rectangle.3.group"
        case .library: return "square.grid.2x2"
        }
    }
}
