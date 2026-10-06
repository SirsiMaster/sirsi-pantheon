import AppKit
import SwiftUI

// PantheonControlCenter is the application's front door. It is intentionally
// not a status-card dashboard: the first screen names the current condition,
// offers one complete in-app route through it, then makes the two creative
// operator routes (Stack Lab and Apollo) immediately available. The complete
// library remains available, but discovery never competes with the next action.
struct PantheonControlCenterView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode

    private var hasAttention: Bool {
        !engine.ownerGatedItems.isEmpty ||
            engine.healthStatus != "green" ||
            engine.routerStatus != "green" ||
            engine.safeBytes >= SirsiEngine.wasteThreshold
    }

    private var overallTitle: String { hasAttention ? "Guidance is ready" : "Ready" }
    private var overallDetail: String {
        if !engine.ownerGatedItems.isEmpty {
            return "\(engine.ownerGatedItems.count) decision\(engine.ownerGatedItems.count == 1 ? "" : "s") waiting for you"
        }
        if engine.healthStatus != "green" { return engine.healthLoading ? "Checking system health" : engine.healthSummary }
        if engine.routerStatus != "green" { return engine.routerSummary }
        if engine.safeBytes >= SirsiEngine.wasteThreshold { return "\(engine.safe.count) cleanup item\(engine.safe.count == 1 ? "" : "s") ready" }
        // Full Disk Access expands optional observability; Pantheon can still
        // diagnose, guide repairs, and operate its managed surfaces without
        // it. Never turn a capability upgrade into a false blocking incident.
        if !engine.hasFDA { return "Ready — Full Disk Access is optional for broader disk visibility" }
        return "Pantheon is monitoring this Mac"
    }

    private var overallSymbol: String {
        if !engine.ownerGatedItems.isEmpty { return "person.badge.key" }
        if engine.healthStatus != "green" { return "exclamationmark.triangle.fill" }
        if engine.routerStatus != "green" { return "point.3.connected.trianglepath.dotted" }
        if engine.safeBytes >= SirsiEngine.wasteThreshold { return "trash" }
        return "checkmark.circle.fill"
    }

    private var overallTint: Color {
        if hasAttention { return gold }
        return emerald
    }

    private func controlledStateTint(_ state: String) -> Color {
        state == "green" ? emerald : gold
    }

    private var routerDetail: String {
        engine.routerStatus == "green" ? engine.routerSummary : "Guidance available in Ra"
    }

    private var healthDetail: String {
        if engine.healthLoading { return "Checking local health" }
        return engine.healthStatus == "green" ? engine.healthSummary : "Guidance available in Horus"
    }

    private var heroTitle: String {
        hasAttention ? "Your next step is ready." : "Your local estate is ready."
    }

    private var heroDetail: String {
        if !engine.ownerGatedItems.isEmpty {
            return "Ma’at has an evidence-bound decision for you. Review it, take the guided action, and keep the resulting proof with this Mac."
        }
        if engine.healthStatus != "green" || engine.routerStatus != "green" {
            return "Pantheon has isolated the current condition and can take you to the right local control surface without sending you to a terminal."
        }
        if engine.safeBytes >= SirsiEngine.wasteThreshold {
            return "There is verified reclaimable space. Review the exact items and complete the cleanup from Pantheon."
        }
        return "Choose a recipe, prepare an Apollo run, inspect the fabric, or open a retained result. Every route stays in the application."
    }

    private var nextActionTitle: String {
        if !engine.ownerGatedItems.isEmpty { return "Review Ma’at’s decision" }
        if engine.healthStatus != "green" { return "Open system recovery" }
        if engine.routerStatus != "green" { return "Inspect the Ra fabric" }
        if engine.safeBytes >= SirsiEngine.wasteThreshold { return "Review verified cleanup" }
        return "Open today’s activity"
    }

    private var nextActionDetail: String {
        if !engine.ownerGatedItems.isEmpty { return "See the reason, the safe choices, and the evidence before you commit." }
        if engine.healthStatus != "green" { return "Understand the current health signal and follow the guided local resolution." }
        if engine.routerStatus != "green" { return "Review the route, claim, or handback that needs attention." }
        if engine.safeBytes >= SirsiEngine.wasteThreshold { return "Inspect exact reclaimable items before anything is removed." }
        return "See the latest completed work and retained evidence."
    }

    var body: some View {
        VStack(spacing: 0) {
            MaybeScroll {
                VStack(alignment: .leading, spacing: 28) {
                    commandHero
                    nextAction
                    operationalContext
                    buildRoute
                    workspaceLibrary
                }
                .padding(.horizontal, 34)
                .padding(.vertical, 30)
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
            // Keep the panel's first actionable state lean. These two probes
            // each start a CLI child, but neither value appears on Home: Apollo
            // owns live estate telemetry and the autonomous control owns its
            // own current mode. Starting them on every panel open made a simple
            // click queue unrelated work behind health/router evidence and
            // created needless process and memory churn. Their destination
            // screens load the same canonical values when the operator opens
            // them, so no state is hidden or silently assumed here.
        }
    }

    private var commandHero: some View {
        HStack(alignment: .top, spacing: 26) {
            VStack(alignment: .leading, spacing: 13) {
                HStack(spacing: 8) {
                    Circle()
                        .fill(overallTint)
                        .frame(width: 8, height: 8)
                        .accessibilityHidden(true)
                    Text(overallTitle.uppercased())
                        .sirsiFont(.caption, weight: .bold)
                        .tracking(1.1)
                        .foregroundStyle(overallTint)
                }
                Text(heroTitle)
                    .sirsiFont(.largeTitle, weight: .bold)
                    .foregroundStyle(Color.white)
                    .fixedSize(horizontal: false, vertical: true)
                Text(heroDetail)
                    .sirsiFont(.body)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: 620, alignment: .leading)
            }
            Spacer(minLength: 24)
            VStack(alignment: .trailing, spacing: 10) {
                PantheonBrandMark(size: 62)
                    .accessibilityHidden(true)
                Text(engine.projectName ?? "Local Pantheon")
                    .sirsiFont(.subheadline, weight: .semibold)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .lineLimit(1)
                Text("Native workspace")
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.mutedText.opacity(0.72))
            }
        }
        .padding(26)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.panel)
        .overlay(alignment: .top) {
            Rectangle()
                .fill(gold.opacity(0.82))
                .frame(height: 1)
        }
        .clipShape(RoundedRectangle(cornerRadius: 16))
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Pantheon status: \(overallTitle). \(heroTitle). \(heroDetail)")
    }

    private var nextAction: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionLabel("NEXT")
            priorityLink
                .padding(18)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(PantheonTheme.panelRaised)
                .overlay(alignment: .leading) {
                    Rectangle().fill(overallTint).frame(width: 3)
                }
                .clipShape(RoundedRectangle(cornerRadius: 12))
        }
    }

    @ViewBuilder private var priorityLink: some View {
        if !engine.ownerGatedItems.isEmpty {
            NavLink { OwnerActionsListView(engine: engine) } label: { priorityRow }
        } else if engine.healthStatus != "green" {
            NavLink { HorusView(engine: engine) } label: { priorityRow }
        } else if engine.routerStatus != "green" {
            NavLink { RaFabricView(engine: engine) } label: { priorityRow }
        } else if engine.safeBytes >= SirsiEngine.wasteThreshold {
            NavLink { AnubisView(engine: engine) } label: { priorityRow }
        } else {
            NavLink { ActivityView(engine: engine) } label: { priorityRow }
        }
    }

    private var priorityRow: some View {
        HStack(spacing: 16) {
            Image(systemName: overallSymbol)
                .sirsiFont(.title3, weight: .semibold)
                .foregroundStyle(overallTint)
                .frame(width: 26)
            VStack(alignment: .leading, spacing: 4) {
                Text(nextActionTitle)
                    .sirsiFont(.headline)
                    .foregroundStyle(Color.white)
                Text(nextActionDetail)
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 10)
            Image(systemName: "arrow.right")
                .sirsiFont(.body, weight: .semibold)
                .foregroundStyle(gold)
        }
        .contentShape(Rectangle())
        .accessibilityLabel("\(nextActionTitle). \(nextActionDetail)")
    }

    private var operationalContext: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionLabel("AT A GLANCE")
            HStack(alignment: .top, spacing: 0) {
                operationalLink(symbol: "point.3.connected.trianglepath.dotted", title: "Ra fabric", detail: routerDetail, tint: controlledStateTint(engine.routerStatus)) {
                    RaFabricView(engine: engine)
                }
                Divider().frame(height: 62)
                operationalLink(symbol: "waveform.path.ecg", title: "System health", detail: healthDetail, tint: controlledStateTint(engine.healthStatus)) {
                    HorusView(engine: engine)
                }
                Divider().frame(height: 62)
                operationalLink(symbol: "circle.dotted", title: "Active work", detail: engine.threadsTotal > 0 ? "\(engine.threadsTotal) live thread\(engine.threadsTotal == 1 ? "" : "s")" : "No live threads", tint: PantheonTheme.mutedText) {
                    ThreadsView(engine: engine)
                }
            }
            .background(PantheonTheme.panel)
            .clipShape(RoundedRectangle(cornerRadius: 12))
        }
    }

    // Stack Lab and Apollo are the product's creative work route. Neither
    // starts inference from Home: Stack Lab assembles a bounded recipe, then
    // Apollo shows the measured local envelope and live telemetry.
    private var buildRoute: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionLabel("BUILD WITH INTENT")
            HStack(spacing: 14) {
                workspaceRoute(
                    symbol: "square.3.layers.3d",
                    title: "Stack Lab",
                    detail: "Choose a recipe, inspect every component, and retain the evidence behind a candidate.",
                    action: "Open recipes"
                ) { StackLabView(engine: engine) }
                workspaceRoute(
                    symbol: "cpu",
                    title: "Apollo",
                    detail: "Select a qualified machine, engine, model, and resource envelope—then read live telemetry.",
                    action: "Plan a run"
                ) { ApolloRunPlannerView(engine: engine) }
            }
        }
    }

    private var workspaceLibrary: some View {
        NavLink { PantheonLibraryView(engine: engine) } label: {
            HStack(spacing: 10) {
                Image(systemName: "square.grid.2x2")
                    .foregroundStyle(PantheonTheme.mutedText)
                Text("Browse every Pantheon surface")
                    .sirsiFont(.body, weight: .semibold)
                    .foregroundStyle(Color.white)
                Spacer()
                Text("All tools")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                Image(systemName: "arrow.right")
                    .sirsiFont(.caption, weight: .semibold)
                    .foregroundStyle(gold)
            }
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Browse every Pantheon surface")
    }

    private func sectionLabel(_ title: String) -> some View {
        Text(title)
            .sirsiFont(.caption, weight: .bold)
            .tracking(1.0)
            .foregroundStyle(PantheonTheme.mutedText.opacity(0.78))
    }

    private func operationalLink<Destination: View>(symbol: String, title: String, detail: String, tint: Color, @ViewBuilder destination: @escaping () -> Destination) -> some View {
        NavLink(destination: destination) {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: symbol)
                    .sirsiFont(.body, weight: .semibold)
                    .foregroundStyle(tint)
                    .frame(width: 20)
                VStack(alignment: .leading, spacing: 4) {
                    Text(title)
                        .sirsiFont(.subheadline, weight: .semibold)
                        .foregroundStyle(Color.white)
                    Text(detail)
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.mutedText)
                        .lineLimit(2)
                }
                Spacer(minLength: 0)
            }
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Open \(title): \(detail)")
    }

    private func workspaceRoute<Destination: View>(symbol: String, title: String, detail: String, action: String, @ViewBuilder destination: @escaping () -> Destination) -> some View {
        NavLink(destination: destination) {
            VStack(alignment: .leading, spacing: 16) {
                Image(systemName: symbol)
                    .sirsiFont(.title2, weight: .semibold)
                    .foregroundStyle(gold)
                Text(title)
                    .sirsiFont(.title3, weight: .bold)
                    .foregroundStyle(Color.white)
                Text(detail)
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
                Spacer(minLength: 2)
                HStack(spacing: 6) {
                    Text(action)
                        .sirsiFont(.subheadline, weight: .semibold)
                    Image(systemName: "arrow.right")
                        .sirsiFont(.caption, weight: .semibold)
                }
                .foregroundStyle(gold)
            }
            .padding(20)
            .frame(maxWidth: .infinity, minHeight: 210, alignment: .leading)
            .background(PantheonTheme.panel)
            .overlay(RoundedRectangle(cornerRadius: 14).stroke(Color.white.opacity(0.08), lineWidth: 1))
            .clipShape(RoundedRectangle(cornerRadius: 14))
        }
        .buttonStyle(.plain)
        .accessibilityLabel("\(title). \(detail). \(action)")
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
                        libraryLink("Ra fabric", symbol: "point.3.connected.trianglepath.dotted") { RaFabricView(engine: engine) }
                        libraryLink("Horus", symbol: "waveform.path.ecg") { HorusView(engine: engine) }
                        libraryLink("Anubis", symbol: "trash") { AnubisView(engine: engine) }
                        libraryLink("Osiris", symbol: "shield") { RiskView(engine: engine) }
                    }
                    librarySection("INTELLIGENCE") {
                        libraryLink("Ma'at", symbol: "checkmark.seal") { MaatWorkspaceView(engine: engine) }
                        libraryLink("Stack Lab", symbol: "square.3.layers.3d") { StackLabView(engine: engine) }
                        libraryLink("Thoth", symbol: "books.vertical") { ThothMemoryInfoView(engine: engine) }
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
                .sirsiFont(.caption, weight: .bold)
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
                    .sirsiFont(.body, weight: .medium)
                    .foregroundStyle(.primary)
                Spacer()
                Image(systemName: "chevron.right")
                    .sirsiFont(.caption, weight: .semibold)
                    .foregroundStyle(.tertiary)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .contentShape(Rectangle())
        }
    }
}
