import AppKit
import SwiftUI

// PantheonControlCenter is deliberately small. The status-item panel is where
// an operator learns what needs attention and takes the next action; it is not
// a dashboard trying to expose every Pantheon capability at once. The complete
// surface remains available through PantheonLibraryView.
struct PantheonControlCenterView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode

    private var hasAttention: Bool {
        !engine.ownerGatedItems.isEmpty ||
            engine.healthStatus != "green" ||
            engine.routerStatus != "green" ||
            engine.safeBytes >= SirsiEngine.wasteThreshold ||
            !engine.hasFDA
    }

    private var overallTitle: String { hasAttention ? "Needs attention" : "Ready" }
    private var overallDetail: String {
        if !engine.ownerGatedItems.isEmpty {
            return "\(engine.ownerGatedItems.count) decision\(engine.ownerGatedItems.count == 1 ? "" : "s") waiting for you"
        }
        if engine.healthStatus != "green" { return engine.healthLoading ? "Checking system health" : engine.healthSummary }
        if engine.routerStatus != "green" { return engine.routerSummary }
        if engine.safeBytes >= SirsiEngine.wasteThreshold { return "\(engine.safe.count) cleanup item\(engine.safe.count == 1 ? "" : "s") ready" }
        if !engine.hasFDA { return "Full Disk Access needs your approval" }
        return "Pantheon is monitoring this Mac"
    }

    private var overallSymbol: String {
        if !engine.ownerGatedItems.isEmpty { return "person.badge.key" }
        if engine.healthStatus != "green" { return "exclamationmark.triangle.fill" }
        if engine.routerStatus != "green" { return "point.3.connected.trianglepath.dotted" }
        if engine.safeBytes >= SirsiEngine.wasteThreshold { return "trash" }
        if !engine.hasFDA { return "lock.trianglebadge.exclamationmark" }
        return "checkmark.circle.fill"
    }

    private var overallTint: Color {
        if !engine.ownerGatedItems.isEmpty || engine.safeBytes >= SirsiEngine.wasteThreshold || !engine.hasFDA { return .orange }
        if engine.healthStatus != "green" { return statusColor(engine.healthStatus) }
        return statusColor(engine.routerStatus)
    }

    var body: some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 18) {
                    header
                    primaryAction
                    attention
                    controls
                    library
                }
                .padding(16)
            }

            Divider()
            HStack(spacing: 10) {
                Button {
                    Task { await engine.rescan() }
                } label: {
                    Label(engine.busy ? "Scanning" : "Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.bordered)
                .disabled(engine.busy)
                if engine.busy { ProgressView().controlSize(.small) }
                Spacer()
                Button("Quit") { NSApplication.shared.terminate(nil) }
                    .buttonStyle(.borderless)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
        }
        .task {
            engine.loadProjectRoot()
            engine.loadActivity()
            engine.loadRunReport()
            await engine.diagnose()
            await engine.loadRouterBoard()
            await engine.fetchVitals()
            await engine.fetchAutonomous()
        }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            VStack(alignment: .leading, spacing: 3) {
                Text("Pantheon")
                    .font(.title2.weight(.bold))
                Text(engine.projectName ?? "Local operator view")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer()
            Label(overallTitle, systemImage: overallSymbol)
                .font(.caption.weight(.semibold))
                .foregroundStyle(overallTint)
                .labelStyle(.titleAndIcon)
        }
    }

    private var primaryAction: some View {
        NavLink { AskSirsiView(engine: engine) } label: {
            HStack(spacing: 12) {
                Image(systemName: "sparkles")
                    .font(.title3.weight(.semibold))
                    .frame(width: 24)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Ask Sirsi")
                        .font(.headline)
                    Text(engine.localLLM?.healthy == true ? "Start with local intelligence" : "Check local intelligence")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Image(systemName: "arrow.right")
                    .font(.body.weight(.semibold))
            }
            .foregroundStyle(.primary)
            .padding(14)
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.accentColor.opacity(0.14)))
            .overlay(RoundedRectangle(cornerRadius: 10).stroke(Color.accentColor.opacity(0.35), lineWidth: 1))
            .contentShape(RoundedRectangle(cornerRadius: 10))
        }
        .accessibilityLabel("Ask Sirsi — open local intelligence")
    }

    private var attention: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("NOW")
                .font(.caption.weight(.bold))
                .foregroundStyle(.secondary)
            priorityLink
                .padding(12)
                .background(RoundedRectangle(cornerRadius: 10).fill(overallTint.opacity(0.10)))
        }
    }

    @ViewBuilder private var priorityLink: some View {
        if !engine.ownerGatedItems.isEmpty {
            NavLink { OwnerActionsListView(engine: engine) } label: { priorityRow }
        } else if engine.healthStatus != "green" {
            NavLink { HorusView(engine: engine) } label: { priorityRow }
        } else if engine.routerStatus != "green" {
            NavLink { RouterView(engine: engine) } label: { priorityRow }
        } else if engine.safeBytes >= SirsiEngine.wasteThreshold {
            NavLink { AnubisView(engine: engine) } label: { priorityRow }
        } else if !engine.hasFDA {
            NavLink { FDAGuideView() } label: { priorityRow }
        } else {
            NavLink { ActivityView(engine: engine) } label: { priorityRow }
        }
    }

    private var priorityRow: some View {
        ControlCenterRow(symbol: overallSymbol, title: overallTitle, detail: overallDetail, tint: overallTint)
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: 7) {
            Text("CONTROL")
                .font(.caption.weight(.bold))
                .foregroundStyle(.secondary)
            VStack(spacing: 1) {
                NavLink { RouterView(engine: engine) } label: {
                    ControlCenterRow(symbol: "point.3.connected.trianglepath.dotted", title: "Router", detail: engine.routerSummary, tint: statusColor(engine.routerStatus))
                }
                Divider().padding(.leading, 38)
                NavLink { HorusView(engine: engine) } label: {
                    ControlCenterRow(symbol: "waveform.path.ecg", title: "System health", detail: engine.healthLoading ? "Checking" : engine.healthSummary, tint: statusColor(engine.healthStatus))
                }
                Divider().padding(.leading, 38)
                NavLink { ThreadsView(engine: engine) } label: {
                    ControlCenterRow(symbol: "circle.dotted", title: "Active work", detail: engine.threadsTotal > 0 ? "\(engine.threadsTotal) live thread\(engine.threadsTotal == 1 ? "" : "s")" : "No live threads", tint: .secondary)
                }
                Divider().padding(.leading, 38)
                NavLink { ActivityView(engine: engine) } label: {
                    ControlCenterRow(symbol: "clock.arrow.circlepath", title: "Recent activity", detail: engine.lastRunSentence ?? "No recorded run", tint: .secondary)
                }
            }
            .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.045)))
        }
    }

    private var library: some View {
        NavLink { PantheonLibraryView(engine: engine) } label: {
            HStack {
                Label("All Pantheon tools", systemImage: "square.grid.2x2")
                    .font(.headline)
                Spacer()
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
            }
            .padding(.vertical, 10)
            .padding(.horizontal, 12)
            .contentShape(Rectangle())
        }
        .foregroundStyle(.primary)
        .accessibilityLabel("All Pantheon tools")
    }
}

private struct ControlCenterRow: View {
    let symbol: String
    let title: String
    let detail: String
    let tint: Color

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: symbol)
                .font(.body.weight(.semibold))
                .foregroundStyle(tint)
                .frame(width: 20)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(.primary)
                Text(detail)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            Spacer(minLength: 8)
            Image(systemName: "chevron.right")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.tertiary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .contentShape(Rectangle())
    }
}

struct PantheonLibraryView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Pantheon tools")
            MaybeScroll {
                VStack(alignment: .leading, spacing: 16) {
                    librarySection("WORK") {
                        libraryLink("Ask Sirsi", symbol: "sparkles") { AskSirsiView(engine: engine) }
                        libraryLink("Insight", symbol: "scope") { InsightView(engine: engine) }
                        libraryLink("Activity", symbol: "clock.arrow.circlepath") { ActivityView(engine: engine) }
                        libraryLink("Threads", symbol: "circle.dotted") { ThreadsView(engine: engine) }
                        libraryLink("Fleet", symbol: "rectangle.3.group") { FleetView(engine: engine) }
                    }
                    librarySection("OPERATIONS") {
                        libraryLink("Router", symbol: "point.3.connected.trianglepath.dotted") { RouterView(engine: engine) }
                        libraryLink("Horus", symbol: "waveform.path.ecg") { HorusView(engine: engine) }
                        libraryLink("Anubis", symbol: "trash") { AnubisView(engine: engine) }
                        libraryLink("Osiris", symbol: "shield") { RiskView(engine: engine) }
                    }
                    librarySection("INTELLIGENCE") {
                        libraryLink("Ma'at", symbol: "checkmark.seal") { MaatWorkspaceView(engine: engine) }
                        libraryLink("Thoth", symbol: "books.vertical") { ThothMemoryInfoView(engine: engine) }
                        libraryLink("Ra", symbol: "person.3") { ResultView(engine: engine, title: "Ra — Agent Fleet", args: ["ra", "status"]) }
                        libraryLink("Net", symbol: "arrow.triangle.branch") { ResultView(engine: engine, title: "Net — Plan", args: ["net", "status"]) }
                        libraryLink("Vault", symbol: "archivebox") { ResultView(engine: engine, title: "Vault — Context", args: ["vault", "stats"]) }
                        libraryLink("RTK", symbol: "line.3.horizontal.decrease.circle") { ResultView(engine: engine, title: "RTK — Output Filter", args: ["rtk", "stats"]) }
                    }
                }
                .padding(16)
            }
        }
    }

    private func librarySection<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 7) {
            Text(title)
                .font(.caption.weight(.bold))
                .foregroundStyle(.secondary)
            VStack(spacing: 1) { content() }
                .background(RoundedRectangle(cornerRadius: 10).fill(Color.primary.opacity(0.045)))
        }
    }

    private func libraryLink<Destination: View>(_ title: String, symbol: String, @ViewBuilder destination: @escaping () -> Destination) -> some View {
        NavLink(destination: destination) {
            HStack(spacing: 10) {
                Image(systemName: symbol)
                    .frame(width: 20)
                    .foregroundStyle(.secondary)
                Text(title)
                    .font(.body.weight(.medium))
                    .foregroundStyle(.primary)
                Spacer()
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.tertiary)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
    }
}
