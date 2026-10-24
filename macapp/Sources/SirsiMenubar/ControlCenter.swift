import AppKit
import SwiftUI

// Overview is a directory into working controls. Its status comes from the
// same engine as the detail views, and an unread source never becomes green.
struct PantheonControlCenterView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
    @Environment(\.openPantheonWorkspace) private var openWorkspace
    @Environment(\.openPantheonOwnerActions) private var openOwnerActions
    @EnvironmentObject private var nav: Nav

    private var healthKnown: Bool { engine.healthIsCurrent && !engine.healthLoading }
    private var needsAttention: Bool {
        !engine.ownerGatedItems.isEmpty || (healthKnown && engine.healthIssueCount > 0)
    }

    var body: some View {
        MaybeScroll {
            VStack(alignment: .leading, spacing: 26) {
                introduction
                attention
                workspaceDirectory
                machineSummary
                recentWork
            }
            .padding(28)
            .frame(maxWidth: 1120, alignment: .leading)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .task {
            guard !snapshotMode else { return }
            engine.loadProjectRoot()
            engine.loadActivity()
            engine.loadRunReport()
            await refreshObservations(force: false)
        }
    }

    private var introduction: some View {
        HStack(alignment: .top) {
            VStack(alignment: .leading, spacing: 8) {
                Text("One place to run your estate.")
                    .sirsiFont(27, weight: .semibold)
                    .fixedSize(horizontal: false, vertical: true)
                Text("Choose intelligence, connect your machines, coordinate work, and resolve what needs attention.")
                    .sirsiFont(13).foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: 610, alignment: .leading)
            }
            Spacer(minLength: 12)
            Button { nav.push(AskSirsiView(engine: engine)) } label: {
                Label("Ask Sirsi", systemImage: "sparkles")
            }
            .buttonStyle(.bordered).tint(gold)
            .help("Ask a question using the configured local intelligence")
        }
    }

    private var attention: some View {
        HStack(alignment: .top, spacing: 14) {
            Image(systemName: needsAttention ? "exclamationmark.circle" : "waveform.path.ecg")
                .sirsiFont(22, weight: .medium).foregroundStyle(needsAttention ? gold : PantheonTheme.mutedText)
            VStack(alignment: .leading, spacing: 6) {
                Text(attentionTitle).sirsiFont(14, weight: .semibold)
                Text(attentionDetail).sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 8)
            Button {
                if !engine.ownerGatedItems.isEmpty {
                    if let openOwnerActions { openOwnerActions() }
                    else { nav.push(OwnerActionsListView(engine: engine)) }
                } else {
                    navigate(attentionWorkspace)
                }
            } label: {
                Label(attentionAction, systemImage: "arrow.right")
            }
            .buttonStyle(.bordered).tint(gold)
        }
        .padding(18)
        .background(PantheonTheme.panelRaised, in: RoundedRectangle(cornerRadius: 10))
    }

    private var attentionTitle: String {
        if !engine.ownerGatedItems.isEmpty { return "\(engine.ownerGatedItems.count) decisions ready for review" }
        if !healthKnown { return engine.healthLoading ? "Checking this Mac" : "Read your current system state" }
        if engine.healthIssueCount > 0 { return "\(engine.healthIssueCount) system findings to resolve" }
        return "Latest health check: no actionable findings"
    }

    private var attentionDetail: String {
        if !engine.ownerGatedItems.isEmpty { return "Review the evidence and choose the next action. Pantheon keeps the resulting outcome with the work." }
        if !healthKnown { return "Open Horus to read current health and capacity. An unavailable observation stays unknown until a read succeeds." }
        if engine.healthIssueCount > 0 { return "Inspect the affected resources in Horus, then follow the scoped repair or Ma’at resolution path." }
        return "Explore your available routes below. Your next run, fabric connection, or release starts from the same workspace."
    }

    private var attentionAction: String {
        if !engine.ownerGatedItems.isEmpty { return "Resolve with Ma'at" }
        return healthKnown && engine.healthIssueCount == 0 ? "Open Ma’at" : "Inspect system"
    }
    private var attentionWorkspace: PantheonWorkspace {
        !engine.ownerGatedItems.isEmpty || (healthKnown && engine.healthIssueCount == 0) ? .maat : .horus
    }

    private var workspaceDirectory: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Intelligence & fabric").sirsiFont(15, weight: .semibold)
            Text("Four connected responsibilities. One local workspace.")
                .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText).padding(.bottom, 6)
            VStack(spacing: 0) {
                workspaceRow(.apollo, role: "Inference", action: "Choose model & machine",
                             detail: "Select a resident model, set cores and memory, then follow the run into measured telemetry.")
                rule
                workspaceRow(.mercury, role: "Connections", action: "Inspect transport",
                             detail: "See admitted machines and the published transport state behind their connections.")
                rule
                workspaceRow(.ra, role: "Coordination", action: "Open work & workers",
                             detail: "Inspect queues, messages, claims, and handbacks across your fabric.")
                rule
                workspaceRow(.maat, role: "Resolution", action: "Open Ma'at evidence",
                             detail: "Classify evidence, inspect local knowledge, and complete guided decisions.")
            }
            .background(PantheonTheme.panel, in: RoundedRectangle(cornerRadius: 10))
        }
    }

    private func workspaceRow(_ workspace: PantheonWorkspace, role: String, action: String, detail: String) -> some View {
        Button { navigate(workspace) } label: {
            HStack(alignment: .center, spacing: 16) {
                Image(systemName: workspace.symbol).sirsiFont(20, weight: .regular)
                    .foregroundStyle(gold).frame(width: 30)
                VStack(alignment: .leading, spacing: 5) {
                    HStack(spacing: 10) {
                        Text(workspace.title).sirsiFont(15, weight: .semibold).foregroundStyle(PantheonTheme.text)
                        Text(role).sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
                    }
                    Text(detail).sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Spacer(minLength: 14)
                Image(systemName: "arrow.up.right").sirsiFont(12, weight: .medium)
                    .foregroundStyle(PantheonTheme.mutedText)
            }
            .padding(.horizontal, 18).padding(.vertical, 18)
            .frame(maxWidth: .infinity, alignment: .leading).contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("\(workspace.title). \(action). \(detail)")
        .help(action)
    }

    private var machineSummary: some View {
        VStack(alignment: .leading, spacing: 13) {
            HStack {
                Text("This Mac").sirsiFont(15, weight: .semibold)
                Spacer()
                Button { navigate(.horus) } label: { Label("Inspect", systemImage: "arrow.up.right") }
                    .buttonStyle(.plain).sirsiFont(12).foregroundStyle(gold)
            }
            HStack(alignment: .top, spacing: 24) {
                observation("Health", healthKnown ? engine.healthSummary : "Not yet observed",
                            tint: healthKnown && engine.healthIssueCount == 0 ? emerald : PantheonTheme.mutedText)
                observation("Fabric", engine.routerReadError != nil ? "Refresh unavailable" : (engine.routerBoard == nil ? "Not yet observed" : engine.routerSummary))
                observation("Cleanup", engine.scannedAt.isEmpty ? "Scan to inspect" : "\(SirsiEngine.human(engine.safeBytes)) reviewable")
            }
            HStack(spacing: 18) {
                Button { navigate(.anubis) } label: { Label("Review cleanup", systemImage: "externaldrive") }
                Button { navigate(.stackLab) } label: { Label("Explore recipes", systemImage: "square.3.layers.3d") }
                Button { navigate(.release) } label: { Label("Prepare a release", systemImage: "shippingbox") }
            }
            .buttonStyle(.plain).sirsiFont(12).foregroundStyle(gold)
            if let date = engine.healthReadAt {
                Text("Health observed \(date.formatted(date: .omitted, time: .shortened))\(engine.healthReadError == nil ? "" : " · refresh failed")")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            }
            if let date = engine.routerReadAt {
                Text("Ra board observed \(date.formatted(date: .omitted, time: .shortened))\(engine.routerReadError == nil ? "" : " · refresh failed")")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            }
            if let error = engine.healthReadError { Text(error).sirsiFont(12).foregroundStyle(gold) }
            if let error = engine.routerReadError { Text(error).sirsiFont(12).foregroundStyle(gold) }
            Button { Task { await refreshObservations(force: true) } } label: {
                Label("Refresh observations", systemImage: "arrow.clockwise")
            }
            .keyboardShortcut("r", modifiers: .command).buttonStyle(.bordered)
            .disabled(engine.healthLoading || engine.routerLoading)
        }
        .padding(18).background(PantheonTheme.panel, in: RoundedRectangle(cornerRadius: 10))
    }

    private func observation(_ title: String, _ value: String, tint: Color = PantheonTheme.text) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(title).sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            Text(value).sirsiFont(13, weight: .medium).foregroundStyle(tint).monospacedDigit()
                .fixedSize(horizontal: false, vertical: true)
        }.frame(maxWidth: .infinity, alignment: .leading)
    }

    private var recentWork: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Recent activity").sirsiFont(15, weight: .semibold)
                Spacer()
                Button("View all") { navigate(.activity) }.buttonStyle(.plain).sirsiFont(12).foregroundStyle(gold)
            }
            if engine.activity.isEmpty {
                Text("Completed actions will appear here with their retained outcomes.")
                    .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
            } else {
                ForEach(Array(engine.activity.prefix(3))) { entry in
                    Button { navigate(.activity) } label: {
                        HStack(spacing: 10) {
                            Image(systemName: "doc.text").foregroundStyle(PantheonTheme.mutedText)
                            Text(entry.title).sirsiFont(12).lineLimit(1)
                            Spacer()
                            Image(systemName: "chevron.right").sirsiFont(10).foregroundStyle(PantheonTheme.mutedText)
                        }.contentShape(Rectangle()).padding(.vertical, 5)
                    }.buttonStyle(.plain)
                }
            }
        }
    }

    private var rule: some View { Rectangle().fill(PantheonTheme.separator.opacity(0.6)).frame(height: 1).padding(.leading, 64) }

    @MainActor private func refreshObservations(force: Bool) async {
        async let healthRead: Void = engine.diagnose(force: force)
        async let routerRead: Bool = engine.loadRouterBoard(force: force)
        _ = await (healthRead, routerRead)
    }

    private func navigate(_ workspace: PantheonWorkspace) {
        if let openWorkspace { openWorkspace(workspace); return }
        switch workspace {
        case .apollo: nav.push(ApolloRunPlannerView(engine: engine))
        case .mercury: nav.push(MercuryWorkspaceView(engine: engine))
        case .ra: nav.push(RaFabricView(engine: engine))
        case .maat: nav.push(MaatWorkspaceView(engine: engine))
        case .horus: nav.push(HorusView(engine: engine))
        case .anubis: nav.push(AnubisView(engine: engine))
        case .stackLab: nav.push(StackLabView(engine: engine))
        case .release: nav.push(PantheonReleaseWorkspaceView(engine: engine))
        case .activity: nav.push(ActivityView(engine: engine))
        default: nav.push(PantheonLibraryView(engine: engine))
        }
    }
}

struct PantheonLibraryView: View {
    @ObservedObject var engine: SirsiEngine
    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Pantheon tools")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 18) {
                    librarySection("Intelligence & fabric") {
                        libraryLink("Apollo", symbol: "cpu") { ApolloRunPlannerView(engine: engine) }
                        libraryLink("Mercury", symbol: "cable.connector.horizontal") { MercuryWorkspaceView(engine: engine) }
                        libraryLink("Ra Fabric", symbol: "point.3.connected.trianglepath.dotted") { RaFabricView(engine: engine) }
                        libraryLink("Ma’at", symbol: "scalemass") { MaatWorkspaceView(engine: engine) }
                        libraryLink("Ask Sirsi", symbol: "sparkles") { AskSirsiView(engine: engine) }
                    }
                    librarySection("Build & work") {
                        libraryLink("Stack Lab", symbol: "square.3.layers.3d") { StackLabView(engine: engine) }
                        libraryLink("Release", symbol: "shippingbox") { PantheonReleaseWorkspaceView(engine: engine) }
                        libraryLink("Activity", symbol: "clock.arrow.circlepath") { ActivityView(engine: engine) }
                        libraryLink("Work", symbol: "person.2") { ThreadsView(engine: engine) }
                        libraryLink("Thoth", symbol: "books.vertical") { ThothMemoryInfoView(engine: engine) }
                    }
                    librarySection("System & tools") {
                        libraryLink("Horus", symbol: "waveform.path.ecg") { HorusView(engine: engine) }
                        libraryLink("Anubis", symbol: "externaldrive") { AnubisView(engine: engine) }
                        libraryLink("Osiris", symbol: "shield") { RiskView(engine: engine) }
                        libraryLink("Fleet", symbol: "desktopcomputer") { FleetView(engine: engine) }
                        libraryLink("Insight", symbol: "scope") { InsightView(engine: engine) }
                        libraryLink("Net", symbol: "arrow.triangle.branch") { ResultView(engine: engine, title: "Net — Plan", args: ["net", "status"]) }
                        libraryLink("Vault", symbol: "archivebox") { ResultView(engine: engine, title: "Vault — Context", args: ["vault", "stats"]) }
                        libraryLink("RTK", symbol: "line.3.horizontal.decrease.circle") { ResultView(engine: engine, title: "RTK — Output Filter", args: ["rtk", "stats"]) }
                    }
                }.padding(24)
            }
        }
    }
    private func librarySection<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            Text(title).sirsiFont(14, weight: .semibold)
            VStack(spacing: 1) { content() }
                .background(PantheonTheme.panel, in: RoundedRectangle(cornerRadius: 10))
        }
    }
    private func libraryLink<Destination: View>(_ title: String, symbol: String, @ViewBuilder destination: @escaping () -> Destination) -> some View {
        NavLink(destination: destination) {
            HStack(spacing: 10) {
                Image(systemName: symbol).frame(width: 20).foregroundStyle(gold)
                Text(title).sirsiFont(13, weight: .medium)
                Spacer()
                Image(systemName: "chevron.right").sirsiFont(10).foregroundStyle(PantheonTheme.mutedText)
            }
            .padding(.horizontal, 14).padding(.vertical, 12).contentShape(Rectangle())
        }
    }
}
