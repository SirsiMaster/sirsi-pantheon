import SwiftUI
import Foundation
import AppKit
import os

// applyLog routes one-click-remediation outcomes to the unified logging system
// (Console.app / `log show --predicate 'subsystem == "ai.sirsi.pantheon"'`) —
// structured, queryable, and rotated by the OS, rather than an ad-hoc write to a
// /tmp file. Used to diagnose a failed apply (FDA / cancel / 0-cleaned).
private let applyLog = Logger(subsystem: "ai.sirsi.pantheon", category: "apply")

private let gold = Color(red: 0.78, green: 0.66, blue: 0.32)

// openSystemURL opens a System Settings / file URL (e.g. the Full Disk Access
// pane). macOS cannot self-grant FDA — this is the one click that gets the user
// to the right pane so Sirsi can see and clean the whole workstation.
func openSystemURL(_ url: String) {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: "/usr/bin/open")
    p.arguments = [url]
    try? p.run()
}

// fullDiskAccessPaneURL is the System Settings deep link for the Full Disk
// Access list. macOS 13+ (Ventura → macOS 26) replaced the old System
// Preferences anchor `com.apple.preference.security` with the System Settings
// extension id below; the legacy id opens Settings but no longer resolves the
// `?Privacy_AllFiles` anchor, landing the user on a page with nothing to do.
let fullDiskAccessPaneURL =
    "x-apple.systempreferences:com.apple.settings.PrivacySecurity.extension?Privacy_AllFiles"

// revealAppInFinder opens Finder with this app bundle selected, so the user can
// drag it straight onto the Full Disk Access list. macOS does not let an app add
// itself to that list — the "+" button (or a drag) is the only sanctioned path —
// so we make the drag target one click away instead of pretending to self-grant.
func revealAppInFinder() {
    let url = URL(fileURLWithPath: Bundle.main.bundlePath)
    NSWorkspace.shared.activateFileViewerSelecting([url])
}

// sirsiArgs turns a CLI command string ("sirsi clean --confirm") into argv for
// SirsiEngine.run, dropping the leading binary name.
func sirsiArgs(_ command: String) -> [String] {
    var toks = command.split(separator: " ").map(String.init)
    if toks.first == "sirsi" { toks.removeFirst() }
    return toks
}

// RootView is the NavigationStack the popover hosts. Every screen pushes onto it
// and the native back button returns — the "persistent menubar that can go back"
// the user asked for. No screen ever kicks out to Terminal or a browser.
// ── Responsive type ──────────────────────────────────────────────────────────
//
// The owner's requirement is that the TEXT resizes with the window, not just
// the container. The first implementation did that with `.scaleEffect` on the
// whole view tree, which has two defects: `.scaleEffect` is a LAYER transform
// (SwiftUI rasterizes at the natural size, then CoreAnimation stretches the
// bitmap), so at non-integral factors every glyph lands off the pixel grid and
// goes soft; and because it paired the zoom with `.frame(height: h / scale)`,
// widening the window SHRANK the vertical content space.
//
// Passing the factor down as an environment value and multiplying it into the
// point size instead means the text engine rasterizes at the true size — sharp
// at every width — while layout, padding and dividers stay native, so nothing
// eats vertical space. Same requirement, without either defect.
private struct TypeScaleKey: EnvironmentKey { static let defaultValue: CGFloat = 1 }

extension EnvironmentValues {
    var sirsiTypeScale: CGFloat {
        get { self[TypeScaleKey.self] }
        set { self[TypeScaleKey.self] = newValue }
    }
}

// typeScale maps window width to a type multiplier. Deliberately damped and
// CAPPED: growing linearly to the 900pt maxSize would be 2.5×, which reads as a
// magnifying glass rather than a resize. 360pt (the window's minSize) is 1.0,
// 900pt (maxSize) is 1.6.
func typeScale(forWidth width: CGFloat) -> CGFloat {
    let t = min(max((width - 360) / (900 - 360), 0), 1)
    return 1 + 0.6 * t
}

private struct ScaledFont: ViewModifier {
    @Environment(\.sirsiTypeScale) private var scale
    let size: CGFloat
    let weight: Font.Weight
    let design: Font.Design
    func body(content: Content) -> some View {
        content.font(.system(size: size * scale, weight: weight, design: design)) // sirsi:scaling-primitive
    }
}

extension View {
    // sirsiFont replaces `.font(.system(size:weight:design:))` everywhere on this
    // surface. Same call shape, but the point size tracks the window.
    func sirsiFont(_ size: CGFloat, weight: Font.Weight = .regular,
                   design: Font.Design = .default) -> some View {
        modifier(ScaledFont(size: size, weight: weight, design: design))
    }
}

// SirsiTextStyle mirrors the SwiftUI semantic styles this surface uses. Base
// point sizes are the MEASURED macOS values (NSFont.preferredFont(forTextStyle:)
// on this OS), not guesses — caption/caption2/footnote really are all 10pt on
// macOS, unlike iOS.
//
// The scale-1.0 branch below returns the NATIVE semantic font rather than a
// system font of the same size. That is deliberate: at the owner's default
// window width the rendering is byte-identical to what shipped, so restoring
// responsive type cannot quietly restyle the panel at rest. Only widening the
// window switches to an explicitly-sized font.
enum SirsiTextStyle {
    case largeTitle, title, title2, title3, headline, body, callout, subheadline, footnote, caption, caption2

    var base: CGFloat {
        switch self {
        case .largeTitle: return 26
        case .title: return 22
        case .title2: return 17
        case .title3: return 15
        case .headline, .body: return 13
        case .callout: return 12
        case .subheadline: return 11
        case .footnote, .caption, .caption2: return 10
        }
    }

    // headline is the only style macOS renders bold by default.
    var defaultWeight: Font.Weight { self == .headline ? .bold : .regular }

    var native: Font {
        switch self {
        case .largeTitle: return .largeTitle
        case .title: return .title
        case .title2: return .title2
        case .title3: return .title3
        case .headline: return .headline
        case .body: return .body
        case .callout: return .callout
        case .subheadline: return .subheadline
        case .footnote: return .footnote
        case .caption: return .caption
        case .caption2: return .caption2
        }
    }
}

private struct ScaledSemanticFont: ViewModifier {
    @Environment(\.sirsiTypeScale) private var scale
    let style: SirsiTextStyle
    let weight: Font.Weight?
    let design: Font.Design

    @ViewBuilder func body(content: Content) -> some View {
        if scale == 1 && weight == nil && design == .default {
            content.font(style.native) // sirsi:scaling-primitive
        } else {
            content.font(.system(size: style.base * scale, // sirsi:scaling-primitive
                                 weight: weight ?? style.defaultWeight,
                                 design: design))
        }
    }
}

extension View {
    // sirsiFont(.caption) replaces .sirsiFont(.caption) and friends. Semantic styles
    // were the gap that made the owner report "text is super tiny": #320 scaled
    // the 73 explicit .system(size:) sites but left 135 semantic ones fixed, so
    // numerals grew while body copy in the SAME card did not.
    func sirsiFont(_ style: SirsiTextStyle, weight: Font.Weight? = nil,
                   design: Font.Design = .default) -> some View {
        modifier(ScaledSemanticFont(style: style, weight: weight, design: design))
    }
}

// ScaledFrame keeps a fixed-width element (glyph gutters, avatars) in step with
// the type it sits beside — without it, scaled text overflows a 28pt gutter.
private struct ScaledFrame: ViewModifier {
    @Environment(\.sirsiTypeScale) private var scale
    let width: CGFloat
    let height: CGFloat?
    func body(content: Content) -> some View {
        content.frame(width: width * scale, height: height.map { $0 * scale })
    }
}

extension View {
    func sirsiFrame(width: CGFloat, height: CGFloat? = nil) -> some View {
        modifier(ScaledFrame(width: width, height: height))
    }
}

// Nav is the explicit navigation stack. SwiftUI's NavigationStack, pushed inside
// this NSPanel, silently DROPPED the in-content BackBar of
// every drilled-in screen — the custom "‹ Back" row never rendered no matter
// what (proven exhaustively 2026-07-10: clean builds, insets, toolbar-hidden,
// fixed heights — none made it appear). So we drive navigation ourselves: a
// stack of destinations, a Back that pops it, and a top bar we fully control.
final class Nav: ObservableObject {
    struct Frame: Identifiable { let id = UUID(); let view: AnyView }
    @Published var stack: [Frame] = []
    func push<V: View>(_ v: V) { stack.append(Frame(view: AnyView(v))) }
    func pop() { if !stack.isEmpty { stack.removeLast() } }
    func popToRoot() { stack.removeAll() }
    var atRoot: Bool { stack.isEmpty }
}

// NavLink is a drop-in replacement for NavLink { destination } label: {…}
// that pushes onto our own Nav instead of a NavigationStack. Same call shape, so
// converting a screen is a rename. The pushed destination inherits `nav` via the
// environment object, so nested NavLinks keep working to any depth.

// MaybeList — a List that stays a real List in the live app but renders as a
// plain stack under snapshot QA: ImageRenderer cannot draw NSTableView-backed
// Lists (they rasterize as a giant prohibition glyph), so every List-based
// screen was invisible to the harness. .listStyle on the stack is a no-op.
struct MaybeList<Content: View>: View {
    @Environment(\.snapshotMode) private var snapshotMode
    @ViewBuilder let content: Content
    var body: some View {
        if snapshotMode {
            VStack(alignment: .leading, spacing: 10) { content }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .padding(12)
        } else {
            List { content }
        }
    }
}

// ScrollView has the same ImageRenderer limitation as List: its hosted content
// is blank in snapshot mode. This wrapper keeps the live app scrollable while
// rendering the identical stack directly for visual regression proof.
struct MaybeScroll<Content: View>: View {
    @Environment(\.snapshotMode) private var snapshotMode
    @ViewBuilder let content: Content
    var body: some View {
        if snapshotMode {
            content.frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        } else {
            ScrollView { content }
        }
    }
}

struct NavLink<Label: View, Destination: View>: View {
    @EnvironmentObject private var nav: Nav
    private let destination: () -> Destination
    private let label: () -> Label
    init(@ViewBuilder destination: @escaping () -> Destination, @ViewBuilder label: @escaping () -> Label) {
        self.destination = destination
        self.label = label
    }
    var body: some View {
        Button { nav.push(destination()) } label: { label() }
            .buttonStyle(.plain)
    }
}

struct RootView: View {
    @ObservedObject var engine: SirsiEngine
    @StateObject private var nav = Nav()

    var body: some View {
        // Wrapping the content in an explicit VStack (instead of a bare Group)
        // is what finally lets each pushed view's BackBar render — a bare Group
        // under the NSHostingView (sizingOptions: []) dropped the top row.
        //
        // No titlebar spacer here: the panel is a plain .titled window (NOT
        // fullSizeContentView, whatever the stale comment above Nav used to
        // claim), so AppKit already insets contentView by the 32pt titlebar —
        // NSWindow.contentRect(forFrameRect:) measures exactly that. The old
        // 50pt Color.clear stacked a SECOND clearance under the real one and
        // put ~82pt of dead space above every screen (owner, 2026-07-24:
        // "huge display gap ... on every window").
        //
        // The type DOES track the window (owner: "why doesnt the window and text
        // resize?"), but via the environment scale that `sirsiFont` multiplies
        // into each point size — not `.scaleEffect`. The geometry reader here
        // reads width and nothing else: layout stays native, so unlike the old
        // `design at 360pt, .scaleEffect(w/360), .frame(height: h/scale)` form,
        // widening the window no longer steals vertical space, and glyphs are
        // rasterized at their true size instead of being stretched as a bitmap.
        GeometryReader { geo in
            VStack(spacing: 0) {
                if let top = nav.stack.last {
                    top.view
                } else {
                    HomeView(engine: engine)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .environment(\.sirsiTypeScale, typeScale(forWidth: geo.size.width))
        }
        // Every panel (re)open starts at a fresh Home. Pushed screens load their
        // command output once (.task) and would otherwise show it forever — the
        // RTK screen kept rendering output from a since-replaced binary.
        .onChange(of: engine.reopenTick) { _ in nav.popToRoot() }
        // Toast deep-link: a clicked owner-gated notification lands directly on
        // that item's action screen (set by AppDelegate.openOwnerItem).
        .onChange(of: engine.pendingOwnerItemID) { id in
            guard let id else { return }
            nav.popToRoot()
            nav.push(OwnerActionView(engine: engine, itemID: id))
            engine.pendingOwnerItemID = nil
        }
        .environmentObject(nav)
        // Keep the entire popover legible, including drilled-in screens that
        // use semantic caption/callout styles rather than Home's explicit sizes.
        .dynamicTypeSize(.large)
    }
}

// ── Home ─────────────────────────────────────────────────────────────────────

struct HomeView: View {
    @ObservedObject var engine: SirsiEngine
    // Snapshot QA renders ScrollView viewports empty — swap for a plain stack.
    @Environment(\.snapshotMode) private var snapshotMode

    var body: some View {
        VStack(spacing: 0) {
            CommandDeckView(engine: engine)
                .padding(.horizontal, 12)
                .padding(.top, 12)
                .padding(.bottom, 8)
                .task { await engine.fetchVitals() }
                .task { await engine.fetchAutonomous() }

            Divider().padding(.horizontal, 12)

            // Home is two honest groups (owner, 2026-07-22 — "this is a mess"):
            // NEEDS ATTENTION = rows with a CURRENT non-green condition, shown
            // only while the condition holds; TOOLS = everything else, quiet,
            // no fake-state chips. Canon: surfaces are current + actionable.
            maybeScroll {
                VStack(spacing: 2) {
                    let attention = !engine.ownerGatedItems.isEmpty
                        || engine.healthStatus != "green"
                        || engine.routerStatus != "green"
                        || engine.safeBytes >= SirsiEngine.wasteThreshold
                    if attention {
                        SectionLabel("NEEDS ATTENTION")
                            .padding(.horizontal, 12).padding(.top, 8).padding(.bottom, 2)
                        if !engine.ownerGatedItems.isEmpty {
                            NavLink { OwnerActionsListView(engine: engine) } label: {
                                DeityRow(glyph: "🔑", title: "Needs you — owner actions",
                                         detail: "\(engine.ownerGatedItems.count) waiting", dot: .yellow)
                            }.buttonStyle(.plain)
                        }
                        if engine.healthStatus != "green" {
                            NavLink { HorusView(engine: engine) } label: {
                                DeityRow(glyph: "𓂀", title: "Horus — Ops",
                                         detail: engine.healthLoading ? "checking…" : engine.healthSummary,
                                         dot: statusColor(engine.healthStatus))
                            }.buttonStyle(.plain)
                        }
                        if engine.routerStatus != "green" {
                            NavLink { RouterView(engine: engine) } label: {
                                DeityRow(glyph: "🛰️", title: "Router — Fabric",
                                         detail: engine.routerSummary,
                                         dot: statusColor(engine.routerStatus))
                            }.buttonStyle(.plain)
                        }
                        if engine.safeBytes >= SirsiEngine.wasteThreshold {
                            NavLink { AnubisView(engine: engine) } label: {
                                DeityRow(glyph: "🐺", title: "Anubis — Hygiene",
                                         detail: "\(engine.safe.count) items ready", dot: .yellow)
                            }.buttonStyle(.plain)
                        }
                        SectionLabel("TOOLS")
                            .padding(.horizontal, 12).padding(.top, 8).padding(.bottom, 2)
                    }

                    NavLink { AskSirsiView(engine: engine) } label: {
                        DeityRow(glyph: "🗣️", title: "Ask Sirsi — Local AI",
                                 detail: engine.localLLM.map { $0.healthy == true ? "online" : "offline" },
                                 dot: engine.localLLM.map { $0.healthy == true ? .green : .red })
                    }.buttonStyle(.plain)

                    NavLink { SNEControlView() } label: {
                        DeityRow(glyph: "⚡", title: "SNE — Models & Engine",
                                 detail: "install, run, recover, and update local AI")
                    }.buttonStyle(.plain)

                    NavLink { InsightView(engine: engine) } label: {
                        DeityRow(glyph: "✨", title: "Insight — what to do next")
                    }.buttonStyle(.plain)

                    if engine.healthStatus == "green" {
                        NavLink { HorusView(engine: engine) } label: {
                            DeityRow(glyph: "𓂀", title: "Horus — Ops",
                                     detail: engine.healthLoading ? "checking…" : engine.healthSummary,
                                     dot: .green)
                        }.buttonStyle(.plain)
                    }

                    if engine.routerStatus == "green" {
                        NavLink { RouterView(engine: engine) } label: {
                            DeityRow(glyph: "🛰️", title: "Router — Fabric",
                                     detail: engine.routerSummary == "healthy" ? nil : engine.routerSummary)
                        }.buttonStyle(.plain)
                    }

                    // Fleet reads the shared producer, so this row and the Horus
                    // board cannot disagree. Detail stays nil until the board is
                    // loaded — a placeholder count would be a number the surface
                    // has not actually read.
                    NavLink { FleetView(engine: engine) } label: {
                        DeityRow(glyph: "⚑", title: "Fleet — every lane",
                                 detail: engine.fleetBoard.map { "\($0.summary.lanesWorking) of \($0.summary.lanesTotal) working · \($0.summary.pctDone)% done" })
                    }.buttonStyle(.plain)

                    if engine.safeBytes < SirsiEngine.wasteThreshold {
                        NavLink { AnubisView(engine: engine) } label: {
                            DeityRow(glyph: "🐺", title: "Anubis — Hygiene", detail: "clean")
                        }.buttonStyle(.plain)
                    }

                    NavLink { ThreadsView(engine: engine) } label: {
                        DeityRow(glyph: "💓", title: "Threads — Heartbeat",
                                 detail: engine.threadsTotal > 0 ? "\(engine.threadsTotal) live" : nil)
                    }.buttonStyle(.plain)

                    NavLink { ResultView(engine: engine, title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                        DeityRow(glyph: "𓆄", title: "Ma'at — Quality", detail: engine.projectName)
                    }.buttonStyle(.plain)

                    NavLink { ResultView(engine: engine, title: "Net — Plan", args: ["net", "status"]) } label: {
                        DeityRow(glyph: "𓁯", title: "Net — Plan", detail: engine.projectName)
                    }.buttonStyle(.plain)

                    NavLink { RiskView(engine: engine) } label: {
                        DeityRow(glyph: "𓁹", title: "Osiris — Checkpoints")
                    }.buttonStyle(.plain)

                    NavLink { ThothMemoryInfoView(engine: engine) } label: {
                        DeityRow(glyph: "𓁟", title: "Thoth — Memory")
                    }.buttonStyle(.plain)

                    NavLink { ResultView(engine: engine, title: "Ra — Agent Fleet", args: ["ra", "status"]) } label: {
                        DeityRow(glyph: "𓇶", title: "Ra — Agent Fleet")
                    }.buttonStyle(.plain)

                    NavLink { ResultView(engine: engine, title: "Seshat — Knowledge", args: ["seshat", "list"]) } label: {
                        DeityRow(glyph: "𓁆", title: "Seshat — Knowledge")
                    }.buttonStyle(.plain)

                    NavLink { ResultView(engine: engine, title: "Vault — Context", args: ["vault", "stats"]) } label: {
                        DeityRow(glyph: "🏛️", title: "Vault — Context")
                    }.buttonStyle(.plain)

                    NavLink { ResultView(engine: engine, title: "RTK — Output Filter", args: ["rtk", "stats"]) } label: {
                        DeityRow(glyph: "⚡", title: "RTK — Output Filter")
                    }.buttonStyle(.plain)

                    NavLink { ActivityView(engine: engine) } label: {
                        DeityRow(glyph: "𓆎", title: "Activity — what Pantheon did",
                                 detail: engine.activity.isEmpty ? nil : "\(engine.activity.count) logged")
                    }.buttonStyle(.plain)

                    // Only nag for Full Disk Access while we don't have it; once
                    // granted the row disappears entirely (a permanent "granted"
                    // confirmation row is exactly the noise this screen sheds).
                    if !engine.hasFDA {
                        NavLink { FDAGuideView() } label: {
                            DeityRow(glyph: "⚠️", title: "Grant Full Disk Access…",
                                     detail: "so Sirsi sees everything", dot: .yellow)
                        }.buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, 10).padding(.top, 6)
            }

            Divider()
            HStack {
                Button { Task { await engine.rescan() } } label: {
                    Label("Scan", systemImage: "arrow.clockwise")
                }.sirsiFont(14, weight: .semibold).disabled(engine.busy)
                if engine.busy { ProgressView().controlSize(.small).padding(.leading, 4) }
                Spacer()
                Button("Quit") { NSApplication.shared.terminate(nil) }
                    .sirsiFont(14, weight: .semibold)
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
        }
        .task { engine.loadProjectRoot(); engine.loadActivity(); engine.loadRunReport(); engine.refresh(); await engine.loadRouterBoard() }   // projection-only on open; diagnostics require the explicit Re-check action
    }

    @ViewBuilder private func maybeScroll<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        if snapshotMode {
            content().frame(maxHeight: .infinity, alignment: .top)
        } else {
            ScrollView { content() }
        }
    }
}

// CommandDeckView is the first screen: the operator should know, at a glance,
// whether local AI, compute, router, context, and risk are ready before opening
// a drill-down. It uses existing live engine state only; no decorative claims.
struct CommandDeckSignal {
    let title: String
    let detail: String
    let tint: Color
    var evidence: [String] = []
}

struct CommandDeckView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
    @Environment(\.colorScheme) private var colorScheme

    private var panelFill: Color {
        snapshotMode && colorScheme == .dark ? Color(red: 0.105, green: 0.105, blue: 0.105) : Color.primary.opacity(0.04)
    }

    private var tileFill: Color {
        snapshotMode && colorScheme == .dark ? Color(red: 0.145, green: 0.145, blue: 0.145) : Color.primary.opacity(0.035)
    }

    private var aiState: CommandDeckSignal {
        guard let llm = engine.localLLM else {
            return CommandDeckSignal(title: "Local AI", detail: "checking conduit", tint: .yellow)
        }
        if llm.healthy == true {
            let model = llm.model?.isEmpty == false ? llm.model! : "local model"
            let memory = llm.rssMB.map { " · \($0) MB" } ?? ""
            return CommandDeckSignal(title: "Local AI", detail: "\(model)\(memory)", tint: .green)
        }
        return CommandDeckSignal(title: "Local AI", detail: "offline or misregistered", tint: .red)
    }

    private var aiStatusLabel: String {
        guard let llm = engine.localLLM else { return "CHECK" }
        return llm.healthy == true ? "ONLINE" : "OFFLINE"
    }

    private var computeState: CommandDeckSignal {
        guard let v = engine.vitals else {
            return CommandDeckSignal(title: "Compute", detail: "reading pressure", tint: .yellow)
        }
        let evidence = [
            "swap \(SirsiEngine.human(v.swapUsedBytes))",
            v.top?.first.map { "top \($0.name) \(SirsiEngine.human($0.rssBytes))" },
        ].compactMap { $0 }
        switch v.pressure {
        case "critical":
            return CommandDeckSignal(title: "Compute", detail: "\(SirsiEngine.human(v.freeBytes)) free · critical", tint: .red, evidence: evidence)
        case "warn":
            return CommandDeckSignal(title: "Compute", detail: "\(SirsiEngine.human(v.freeBytes)) free · tight", tint: .orange, evidence: evidence)
        default:
            return CommandDeckSignal(title: "Compute", detail: "\(SirsiEngine.human(v.freeBytes)) free", tint: .green, evidence: evidence)
        }
    }

    private var routerState: CommandDeckSignal {
        CommandDeckSignal(title: "Router", detail: engine.routerSummary, tint: statusColor(engine.routerStatus))
    }

    private var contextState: CommandDeckSignal {
        let owner = engine.ownerGatedItems.count
        if owner > 0 {
            return CommandDeckSignal(title: "Context", detail: "\(owner) owner decision\(owner == 1 ? "" : "s")", tint: .yellow)
        }
        if engine.threadsTotal > 0 {
            return CommandDeckSignal(title: "Context", detail: "\(engine.threadsTotal) live threads", tint: .green)
        }
        return CommandDeckSignal(title: "Context", detail: "wake digest ready", tint: .secondary)
    }

    private var riskState: CommandDeckSignal {
        if engine.healthStatus != "green" {
            return CommandDeckSignal(title: "Risk", detail: engine.healthSummary, tint: statusColor(engine.healthStatus))
        }
        if engine.safeBytes >= SirsiEngine.wasteThreshold {
            return CommandDeckSignal(title: "Risk", detail: "\(SirsiEngine.human(engine.safeBytes)) reclaimable", tint: .yellow)
        }
        return CommandDeckSignal(title: "Risk", detail: "clean checkpoint", tint: .green)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Sirsi Command Deck")
                        .sirsiFont(18, weight: .bold)
                        .foregroundStyle(.primary)
                    Text("Local intelligence control plane")
                        .sirsiFont(12, weight: .semibold)
                        .foregroundStyle(gold)
                }
                Spacer()
                // The status capsule annotates the local AI — so it opens it.
                NavLink { AskSirsiView(engine: engine) } label: {
                    HStack(spacing: 6) {
                        Circle().fill(aiState.tint).frame(width: 8, height: 8)
                        Text(aiStatusLabel)
                            .sirsiFont(10, weight: .bold)
                            .foregroundStyle(.secondary)
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 5)
                    .background(Capsule().fill(tileFill))
                    .contentShape(Capsule())
                }
                .accessibilityLabel("\(aiStatusLabel) — open Ask Sirsi")
            }

            VStack(alignment: .leading, spacing: 8) {
                // The hero annotates the local AI; tapping it opens Ask Sirsi.
                NavLink { AskSirsiView(engine: engine) } label: {
                    HStack(alignment: .top, spacing: 10) {
                        RoundedRectangle(cornerRadius: 3)
                            .fill(aiState.tint)
                            .frame(width: 4)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(aiState.title)
                                .sirsiFont(24, weight: .bold)
                                .foregroundStyle(.primary)
                            Text(aiState.detail)
                                .sirsiFont(13, weight: .medium)
                                .foregroundStyle(.secondary)
                                .lineLimit(2)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        Spacer(minLength: 8)
                        Image(systemName: "chevron.right")
                            .sirsiFont(11, weight: .semibold)
                            .foregroundStyle(.tertiary)
                            .padding(.top, 6)
                    }
                    .contentShape(Rectangle())
                }
                .accessibilityLabel("\(aiState.title) — open Ask Sirsi")

                // Every chip is a drill-down into the surface it annotates (owner
                // gate): a chip that names a number the user cannot open teaches
                // them the panel is a poster, not an instrument. Context and Risk
                // pick their destination from the SAME condition that picked their
                // text, so the tap always lands on the thing the words describe.
                // Text and destination are derived ATOMICALLY: the route enum is
                // computed in the same body pass as the chip text and captured BY
                // VALUE in the destination closure. Re-reading engine state at tap
                // time could show "owner decisions" and open Threads if the engine
                // updated between render and tap (codex post-merge finding 3).
                let ctxRoute: DeckRoute = engine.ownerGatedItems.count > 0 ? .ownerActions : .threads
                let riskRoute: DeckRoute = engine.healthStatus != "green" ? .horus
                    : (engine.safeBytes >= SirsiEngine.wasteThreshold ? .anubis : .osiris)
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 132), spacing: 8)], spacing: 8) {
                    CommandDeckMetric(state: computeState, fill: tileFill, destinationName: DeckRoute.horus.surfaceName) { HorusView(engine: engine) }
                    CommandDeckMetric(state: routerState, fill: tileFill, destinationName: DeckRoute.routerFabric.surfaceName) { RouterView(engine: engine) }
                    CommandDeckMetric(state: contextState, fill: tileFill, destinationName: ctxRoute.surfaceName) {
                        DeckRouteView(route: ctxRoute, engine: engine)
                    }
                    CommandDeckMetric(state: riskState, fill: tileFill, destinationName: riskRoute.surfaceName) {
                        DeckRouteView(route: riskRoute, engine: engine)
                    }
                }

                if let sentence = engine.lastRunSentence {
                    NavLink { ActivityView(engine: engine) } label: {
                        HStack(spacing: 4) {
                            Text(sentence)
                                .sirsiFont(12, weight: .medium)
                                .foregroundStyle(engine.lastRun?.outcome == "degraded" ? Color.orange : Color.secondary)
                                .lineLimit(2)
                                .fixedSize(horizontal: false, vertical: true)
                            Image(systemName: "chevron.right")
                                .sirsiFont(9, weight: .semibold)
                                .foregroundStyle(.tertiary)
                        }
                        .contentShape(Rectangle())
                    }
                    .accessibilityLabel("Last run — open Activity")
                }
            }
            .padding(12)
            .background(
                RoundedRectangle(cornerRadius: 8)
                    .fill(panelFill)
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(gold.opacity(0.34), lineWidth: 1))
            )

            HStack(spacing: 8) {
                CommandDeckNav(title: "Ask", symbol: "sparkles", fill: panelFill) {
                    AskSirsiView(engine: engine)
                }
                CommandDeckNav(title: "Router", symbol: "point.3.connected.trianglepath.dotted", fill: panelFill) {
                    RouterView(engine: engine)
                }
                CommandDeckNav(title: "Ops", symbol: "waveform.path.ecg", fill: panelFill) {
                    HorusView(engine: engine)
                }
                CommandDeckNav(title: "Insight", symbol: "scope", fill: panelFill) {
                    InsightView(engine: engine)
                }
            }

            HStack(spacing: 8) {
                Image(systemName: engine.autonomousOn ? "bolt.shield.fill" : "eye")
                    .foregroundStyle(engine.autonomousOn ? gold : Color.secondary)
                    .sirsiFrame(width: 18)
                VStack(alignment: .leading, spacing: 1) {
                    Text("Autonomous fixes")
                        .sirsiFont(13, weight: .semibold)
                    Text(engine.autonomousOn ? "enabled for safe health work" : "review-first mode")
                        .sirsiFont(11, weight: .medium)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Toggle("", isOn: Binding(
                    get: { engine.autonomousOn },
                    set: { on in Task { await engine.setAutonomous(on) } }
                ))
                .labelsHidden()
                .toggleStyle(.switch)
                .controlSize(.small)
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(RoundedRectangle(cornerRadius: 7).fill(panelFill))
        }
    }
}

// DeckRoute is a VALUE captured at render time, so the surface a tap opens is
// the one the chip's text described — text and destination cannot diverge.
enum DeckRoute {
    case horus, routerFabric, ownerActions, threads, anubis, osiris

    var surfaceName: String {
        switch self {
        case .horus: return "Horus — Ops"
        case .routerFabric: return "Router — Fabric"
        case .ownerActions: return "Owner Actions"
        case .threads: return "Threads"
        case .anubis: return "Anubis — Hygiene"
        case .osiris: return "Osiris — Checkpoints"
        }
    }
}

struct DeckRouteView: View {
    let route: DeckRoute
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        switch route {
        case .horus: HorusView(engine: engine)
        case .routerFabric: RouterView(engine: engine)
        case .ownerActions: OwnerActionsListView(engine: engine)
        case .threads: ThreadsView(engine: engine)
        case .anubis: AnubisView(engine: engine)
        case .osiris: RiskView(engine: engine)
        }
    }
}

// CommandDeckMetric is a DRILL-DOWN, not a poster (owner gate 2026-07-30):
// every chip on the deck opens the surface whose state it annotates. The
// chevron and hover ring exist so it also LOOKS openable — an affordance the
// user cannot see is one they will never try.
struct CommandDeckMetric<Destination: View>: View {
    let state: CommandDeckSignal
    let fill: Color
    // Named so the accessibility label can say where the tap actually lands —
    // "open Context" names the chip, not the surface (codex post-merge finding 4).
    let destinationName: String
    @ViewBuilder let destination: () -> Destination
    @State private var hovering = false

    var body: some View {
        NavLink { destination() } label: {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Circle().fill(state.tint).frame(width: 7, height: 7)
                    Text(state.title.uppercased())
                        .sirsiFont(9, weight: .bold)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    Spacer(minLength: 2)
                    Image(systemName: "chevron.right")
                        .sirsiFont(8, weight: .semibold)
                        .foregroundStyle(hovering ? .secondary : .tertiary)
                }
                Text(state.detail)
                    .sirsiFont(12, weight: .semibold)
                    .foregroundStyle(.primary)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
                ForEach(state.evidence, id: \.self) { line in
                    Text(line)
                        .sirsiFont(10, weight: .medium)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .frame(maxWidth: .infinity, minHeight: state.evidence.isEmpty ? 46 : 70, alignment: .topLeading)
            .padding(8)
            .background(
                RoundedRectangle(cornerRadius: 7)
                    .fill(fill)
                    .overlay(RoundedRectangle(cornerRadius: 7)
                        .stroke(hovering ? Color.secondary.opacity(0.35) : .clear, lineWidth: 1))
            )
            .contentShape(Rectangle())
        }
        .onHover { hovering = $0 }
        .accessibilityLabel("\(state.title): \(state.detail) — open \(destinationName)")
    }
}

struct CommandDeckNav<Destination: View>: View {
    let title: String
    let symbol: String
    let fill: Color
    private let destination: () -> Destination

    init(title: String, symbol: String, fill: Color, @ViewBuilder destination: @escaping () -> Destination) {
        self.title = title
        self.symbol = symbol
        self.fill = fill
        self.destination = destination
    }

    var body: some View {
        NavLink { destination() } label: {
            HStack(spacing: 5) {
                Image(systemName: symbol).sirsiFont(11, weight: .semibold)
                Text(title).sirsiFont(12, weight: .semibold).lineLimit(1)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 8)
            .background(
                RoundedRectangle(cornerRadius: 7)
                    .fill(fill)
                    .overlay(RoundedRectangle(cornerRadius: 7).stroke(gold.opacity(0.42), lineWidth: 1))
            )
            .foregroundStyle(gold)
        }
    }
}

// statusColor maps the canonical green/amber/red roll-up to a dot/label colour.
func statusColor(_ status: String) -> Color {
    switch status {
    case "red": return .red
    case "amber": return .yellow
    default: return .green
    }
}

// findingColor colours a single finding's dot per the rubric (Go severity scale:
// 0 OK · 1 Info · 2 Warn · 3 Critical). A 7-day TREND critical is amber, not red —
// only a LIVE critical is act-now red, matching guard.HealthStatus.
func findingColor(_ f: DiagFinding) -> Color {
    switch f.severity {
    case 0, 1: return .green       // OK / Info
    case 2: return .yellow         // Warn
    default: return (f.trend ?? false) ? .yellow : .red  // Critical: trend → amber
    }
}

// insightSeverityColor maps an Insight signal's ALREADY-rolled-up severity
// (0 green · 1 amber · 2 red) to a dot. Distinct from findingColor's raw scale.
func insightSeverityColor(_ sev: Int) -> Color {
    switch sev {
    case 0: return .green
    case 1: return .yellow
    default: return .red
    }
}

// DeityRow: `detail` is LIVE STATE ONLY (a count, a verdict, a repo under
// audit) — never a static category label. A row with nothing current to say
// shows just its name; fake-state chips are what made Home unreadable
// (owner, 2026-07-22). Dot colors: red broken · yellow waiting-on-someone ·
// green good; a dot never accompanies neutral text.
struct DeityRow: View {
    let glyph: String; let title: String
    var detail: String? = nil
    var dot: Color? = nil
    @Environment(\.snapshotMode) private var snapshotMode
    @Environment(\.colorScheme) private var colorScheme
    private var panelFill: Color {
        snapshotMode && colorScheme == .dark ? Color(red: 0.105, green: 0.105, blue: 0.105) : Color.primary.opacity(0.04)
    }
    var body: some View {
        HStack(spacing: 10) {
            Text(glyph).sirsiFont(20).sirsiFrame(width: 28)
            Text(title).sirsiFont(15, weight: .semibold)
            Spacer()
            if let dot { Circle().fill(dot).frame(width: 7, height: 7) }
            if let detail {
                Text(detail).sirsiFont(13, weight: .medium).foregroundStyle(.secondary)
            }
            Image(systemName: "chevron.right").sirsiFont(13, weight: .medium).foregroundStyle(.tertiary)
        }
        .padding(.vertical, 8).padding(.horizontal, 10)
        .contentShape(Rectangle())
        .background(RoundedRectangle(cornerRadius: 7).fill(panelFill))
    }
}

// BackBar is the in-content "‹ Back" header every drilled-in screen needs. On
// macOS a NavigationStack draws its back button in the host window's toolbar —
// which an NSPopover does not have — so the native control is invisible and the
// user gets trapped. This calls @Environment(\.dismiss) to pop the stack
// regardless of toolbar chrome. Put it at the very top of each pushed view.
struct BackBar: View {
    @EnvironmentObject private var nav: Nav
    let title: String
    var body: some View {
        HStack(spacing: 6) {
            Button { nav.pop() } label: {
                // The LABEL is the hit area for a .plain button — the bare
                // chevron+text was a ~40×16pt target the owner had to "click
                // around a few times to actuate" (2026-07-09). Pad it to a
                // 44pt-class target and make the whole padded region tappable.
                HStack(spacing: 4) {
                    Image(systemName: "chevron.left").sirsiFont(12, weight: .semibold)
                    Text("Back").sirsiFont(12)
                }
                .padding(.vertical, 10)
                .padding(.leading, 12)
                .padding(.trailing, 24)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain).foregroundStyle(gold)
            Spacer()
            Text(title).sirsiFont(12, weight: .semibold).foregroundStyle(.secondary)
            Spacer()
            // invisible spacer mirroring the back button keeps the title centered
            HStack(spacing: 4) {
                Image(systemName: "chevron.left").sirsiFont(12)
                Text("Back").sirsiFont(12)
            }
            .padding(.leading, 12).padding(.trailing, 24)
            .opacity(0)
        }
        .padding(.vertical, 0)
        .contentShape(Rectangle())
        Divider()
    }
}

// ── Full Disk Access guide ───────────────────────────────────────────────────
// macOS has no API for an app to grant itself (or even reliably list itself in)
// Full Disk Access — the list is user-managed via "+". This screen is honest
// about that: it opens the right pane AND reveals the app for drag-to-add, with
// the exact steps, instead of a one-click button that silently does nothing.
struct FDAGuideView: View {
    @Environment(\.openURL) private var openURL

    private var steps: [(String, String)] {
        [
            ("1", "Tap “Open Full Disk Access” below — System Settings opens to the right list."),
            ("2", "Click the + button (or drag “Sirsi Menubar” in from the Finder window we reveal)."),
            ("3", "Toggle Sirsi Menubar on. You only do this once."),
        ]
    }

    var body: some View {
        VStack(spacing: 0) {
        BackBar(title: "Full Disk Access")
        VStack(alignment: .leading, spacing: 14) {
            Text("Grant Full Disk Access")
                .sirsiFont(.headline).foregroundStyle(gold)
            Text("macOS won’t let Sirsi grant itself disk access — you add it once, by hand. Here’s the quickest way:")
                .sirsiFont(.callout).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            VStack(alignment: .leading, spacing: 10) {
                ForEach(steps, id: \.0) { step in
                    HStack(alignment: .top, spacing: 10) {
                        Text(step.0)
                            .sirsiFont(12, weight: .bold)
                            .frame(width: 20, height: 20)
                            .background(Circle().fill(gold.opacity(0.20)))
                            .foregroundStyle(gold)
                        Text(step.1).sirsiFont(12)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }

            VStack(spacing: 8) {
                Button {
                    openSystemURL(fullDiskAccessPaneURL)
                    revealAppInFinder()
                } label: {
                    Label("Open Full Disk Access", systemImage: "lock.open")
                        .frame(maxWidth: .infinity)
                }.buttonStyle(.borderedProminent).tint(gold)

                Button {
                    revealAppInFinder()
                } label: {
                    Label("Reveal Sirsi Menubar in Finder", systemImage: "folder")
                        .frame(maxWidth: .infinity)
                }.buttonStyle(.bordered)
            }

            Spacer()
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle("Full Disk Access")
    }
}

// ── Horus — Ops (health → cause) ─────────────────────────────────────────────

struct HorusView: View {
    @ObservedObject var engine: SirsiEngine

    private static let memoryChecks: Set<String> = [
        "RAM Pressure", "Memory Death Spiral", "Swap", "Top Memory Consumers",
        "Process Footprint", "Duplicate Model Brokers",
    ]

    private var memoryFindings: [DiagFinding] {
        engine.health.filter { Self.memoryChecks.contains($0.check) }
    }
    private var memoryIssues: [DiagFinding] { memoryFindings.filter { $0.severity >= 2 } }
    private var otherIssues: [DiagFinding] {
        engine.health.filter { $0.severity >= 2 && !Self.memoryChecks.contains($0.check) }
    }
    private var quietFindings: [DiagFinding] { engine.health.filter { $0.severity < 2 } }

    private var statusTitle: String {
        switch engine.healthStatus {
        case "red": return "System needs attention"
        case "amber": return "Worth a look"
        default: return "Your Mac looks good"
        }
    }

    private var statusDetail: String {
        let n = engine.healthIssueCount
        if n == 0 { return "No active issues. Horus is watching quietly." }
        let count = "\(n) check\(n == 1 ? "" : "s") need attention."
        return memoryIssues.count > 1 ? count + " Related signals are grouped into one clear story." : count
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Horus — Ops")
            if engine.health.isEmpty {
                VStack(spacing: 10) {
                    if engine.healthLoading { ProgressView() }
                    Text(engine.healthLoading ? "Checking system health…" : "No health data")
                        .sirsiFont(.callout).foregroundStyle(.secondary)
                }.frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                MaybeScroll {
                    VStack(alignment: .leading, spacing: 14) {
                        HorusStatusCard(status: engine.healthStatus, title: statusTitle,
                                        detail: statusDetail, issueCount: engine.healthIssueCount)

                        if !memoryIssues.isEmpty {
                            SectionLabel("WHAT'S HAPPENING")
                            HorusMemoryStory(engine: engine, findings: memoryFindings,
                                             issueCount: memoryIssues.count)
                        }

                        if !otherIssues.isEmpty {
                            SectionLabel(memoryIssues.isEmpty ? "WHAT'S HAPPENING" : "ALSO NEEDS ATTENTION")
                            VStack(spacing: 0) {
                                ForEach(Array(otherIssues.enumerated()), id: \.element.id) { index, finding in
                                    HealthRow(engine: engine, finding: finding)
                                        .padding(.horizontal, 10).padding(.vertical, 5)
                                    if index != otherIssues.count - 1 { Divider().padding(.leading, 26) }
                                }
                            }
                            .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.045)))
                        }

                        if !quietFindings.isEmpty {
                            DisclosureGroup {
                                VStack(spacing: 0) {
                                    ForEach(Array(quietFindings.enumerated()), id: \.element.id) { index, finding in
                                        HealthRow(engine: engine, finding: finding)
                                            .padding(.horizontal, 8).padding(.vertical, 4)
                                        if index != quietFindings.count - 1 { Divider().padding(.leading, 24) }
                                    }
                                }.padding(.top, 6)
                            } label: {
                                HStack(spacing: 8) {
                                    Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                                    Text("Healthy checks").sirsiFont(12, weight: .semibold)
                                    Spacer()
                                    Text("\(quietFindings.count)").sirsiFont(.caption).foregroundStyle(.secondary)
                                }
                            }
                            .padding(12)
                            .background(RoundedRectangle(cornerRadius: 12).fill(Color.primary.opacity(0.035)))
                        }
                    }
                    .padding(14)
                }
            }
            Divider()
            HStack {
                Button { Task { await engine.diagnose(force: true) } } label: {
                    Label("Re-check", systemImage: "arrow.clockwise")
                }.disabled(engine.healthLoading)
                if engine.healthLoading { ProgressView().controlSize(.small).padding(.leading, 4) }
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
        }
        .navigationTitle("Horus — Ops")
    }
}

// The calm overview keeps the canonical roll-up exactly as reported by Go, but
// expresses it in human language. Colour is a compact accent, never a wall of
// alarm text; the explanation carries the meaning for accessibility.
private struct HorusStatusCard: View {
    let status: String
    let title: String
    let detail: String
    let issueCount: Int

    private var label: String {
        switch status { case "red": return "Critical"; case "amber": return "Attention"; default: return "Healthy" }
    }

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            ZStack {
                Circle().fill(statusColor(status).opacity(0.16)).frame(width: 38, height: 38)
                Image(systemName: status == "green" ? "checkmark" : "waveform.path.ecg")
                    .sirsiFont(15, weight: .semibold).foregroundStyle(statusColor(status))
            }
            VStack(alignment: .leading, spacing: 5) {
                HStack(alignment: .firstTextBaseline) {
                    Text(title).sirsiFont(16, weight: .semibold)
                    Spacer(minLength: 8)
                    Text(label.uppercased()).sirsiFont(.caption2, weight: .bold)
                        .foregroundStyle(statusColor(status))
                        .padding(.horizontal, 7).padding(.vertical, 3)
                        .background(Capsule().fill(statusColor(status).opacity(0.12)))
                }
                Text(detail).sirsiFont(.callout).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(14)
        .background(RoundedRectangle(cornerRadius: 14).fill(Color.primary.opacity(0.055)))
        .overlay(RoundedRectangle(cornerRadius: 14).stroke(Color.primary.opacity(0.07)))
        .accessibilityElement(children: .combine)
        .accessibilityLabel(issueCount == 0 ? "\(label). \(detail)" : "\(label). \(issueCount) checks need attention. \(detail)")
    }
}

// Memory findings are intentionally different measurements (live usage, peak
// footprint, system pressure, broker count). Presenting each as a peer row made
// one process look like several unrelated emergencies. This card preserves all
// measurements while telling one story and offering one path to the strongest
// actionable finding.
private struct HorusMemoryStory: View {
    @ObservedObject var engine: SirsiEngine
    let findings: [DiagFinding]
    let issueCount: Int

    private var actionable: DiagFinding {
        findings.sorted {
            if $0.severity != $1.severity { return $0.severity > $1.severity }
            return !($0.fix ?? "").isEmpty && ($1.fix ?? "").isEmpty
        }.first!
    }
    private var current: DiagFinding? { findings.first { $0.check == "Top Memory Consumers" } }
    private var peak: DiagFinding? { findings.first { $0.check == "Process Footprint" } }
    private var pressure: DiagFinding? {
        findings.first { $0.check == "RAM Pressure" || $0.check == "Memory Death Spiral" || $0.check == "Swap" }
    }
    private var broker: DiagFinding? { findings.first { $0.check == "Duplicate Model Brokers" } }

    var body: some View {
        NavLink { FindingView(engine: engine, finding: actionable) } label: {
            VStack(alignment: .leading, spacing: 11) {
                HStack(spacing: 9) {
                    ZStack {
                        RoundedRectangle(cornerRadius: 8).fill(findingColor(actionable).opacity(0.15))
                        Image(systemName: "memorychip").foregroundStyle(findingColor(actionable))
                    }.frame(width: 34, height: 34)
                    VStack(alignment: .leading, spacing: 1) {
                        Text("Memory load").sirsiFont(14, weight: .semibold)
                        Text("\(issueCount) related check\(issueCount == 1 ? "" : "s") · one system story")
                            .sirsiFont(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    Image(systemName: "chevron.right").sirsiFont(.caption).foregroundStyle(.tertiary)
                }

                if let current { HorusEvidence(label: "NOW", text: current.message) }
                if let peak { HorusEvidence(label: "PEAK", text: peak.message) }
                if current == nil, let pressure { HorusEvidence(label: "NOW", text: pressure.message) }

                if let broker, broker.severity < 2 {
                    HStack(spacing: 5) {
                        Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                        Text(broker.message).sirsiFont(.caption).foregroundStyle(.secondary)
                    }
                }
            }
            .padding(14)
            .background(RoundedRectangle(cornerRadius: 14).fill(Color.primary.opacity(0.055)))
            .overlay(RoundedRectangle(cornerRadius: 14).stroke(findingColor(actionable).opacity(0.24)))
            .contentShape(Rectangle())
        }.buttonStyle(.plain)
    }
}

private struct HorusEvidence: View {
    let label: String
    let text: String
    var body: some View {
        HStack(alignment: .top, spacing: 9) {
            Text(label).sirsiFont(.caption2, weight: .bold).foregroundStyle(.secondary)
                .sirsiFrame(width: 34)
                .frame(alignment: .leading)
            Text(text).sirsiFont(.callout).foregroundStyle(.primary.opacity(0.9))
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}

struct HealthRow: View {
    @ObservedObject var engine: SirsiEngine
    let finding: DiagFinding

    private var hasFix: Bool { !(finding.fix ?? "").isEmpty }
    private var navigable: Bool { hasFix || !(finding.detail ?? "").isEmpty }

    var body: some View {
        if navigable {
            NavLink { FindingView(engine: engine, finding: finding) } label: { row }
                .buttonStyle(.plain)
        } else {
            row
        }
    }

    private var row: some View {
        HStack(spacing: 8) {
            Circle().fill(findingColor(finding)).frame(width: 8, height: 8)
            VStack(alignment: .leading, spacing: 2) {
                Text(finding.check).sirsiFont(12, weight: .semibold)
                Text(finding.message).sirsiFont(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer()
            if hasFix {
                Image(systemName: "wrench.and.screwdriver.fill").sirsiFont(.caption2).foregroundStyle(gold)
            } else if navigable {
                Image(systemName: "chevron.right").sirsiFont(.caption2).foregroundStyle(.tertiary)
            }
        }
        .padding(.vertical, 3)
        .contentShape(Rectangle())
    }
}

// FindingView is the resolution surface for one health finding — "health → cause
// → fix" made real. It explains the finding and, when a safe remediation exists,
// offers a one-click "Fix it" that runs it. No finding dead-ends at "here's a
// problem" with no way to act.
// FindingDetailEntry is one parsed row of a pipe-separated finding detail —
// "Name (SIZE) | Name (SIZE) | …" (the Top Memory Consumers shape) or
// "name 45% | name 12%" (the Spotlight shape, no parenthesised value).
private struct FindingDetailEntry: Identifiable {
    let id: Int
    let name: String
    let value: String? // trailing "(…)" content, right-aligned when present
}

// parseFindingDetailList turns a pipe-separated detail string into rows so it
// renders as a legible list instead of one caption-monospaced blob (the owner's
// "tiny unreadable pipe-string" defect). Returns nil unless the string is
// genuinely a list (2+ pipe-separated items) so prose details keep wrapped text.
private func parseFindingDetailList(_ detail: String) -> [FindingDetailEntry]? {
    let parts = detail.components(separatedBy: "|")
        .map { $0.trimmingCharacters(in: .whitespaces) }
        .filter { !$0.isEmpty }
    guard parts.count >= 2 else { return nil }
    return parts.enumerated().map { i, part in
        // A trailing "(…)" is a value column (e.g. "Python (6.2 GB)").
        if part.hasSuffix(")"), let open = part.range(of: "(", options: .backwards) {
            let name = String(part[..<open.lowerBound]).trimmingCharacters(in: .whitespaces)
            let value = String(part[part.index(after: open.lowerBound)..<part.index(before: part.endIndex)])
            if !name.isEmpty && !value.isEmpty {
                return FindingDetailEntry(id: i, name: name, value: value)
            }
        }
        return FindingDetailEntry(id: i, name: part, value: nil)
    }
}

struct FindingView: View {
    @ObservedObject var engine: SirsiEngine
    let finding: DiagFinding

    // The honesty class drives EVERY label so a 7-day history never wears an
    // "instant fix" costume. See guard.FixKind (instant | relief | guidance).
    private var kind: String { finding.fixKind ?? "" }
    @State private var copied = false

    // recommendedCommand pulls a `sirsi …` command the finding names in its
    // message/detail (backtick-quoted) so guidance findings become actionable.
    private var recommendedCommand: String? {
        for text in [finding.message, finding.detail ?? ""] {
            // Prefer a backtick-quoted command.
            if let open = text.range(of: "`sirsi "), let close = text.range(of: "`", range: open.upperBound..<text.endIndex) {
                let cmd = String(text[open.upperBound..<close.lowerBound])
                if !cmd.isEmpty { return "sirsi " + cmd.replacingOccurrences(of: "sirsi ", with: "") }
            }
        }
        return nil
    }

    // Warn (2) and Critical (3) are alarms (guard.DiagnosticSeverity). An alarm
    // without a fix must say so honestly — never "Informational".
    private var isAlarm: Bool { finding.severity >= 2 }

    private var fixIcon: String {
        switch kind {
        case "relief": return "gauge.with.dots.needle.bottom.50percent"
        case "guidance": return "info.circle.fill"
        default: return "wrench.and.screwdriver.fill"
        }
    }
    private var fixSectionLabel: String {
        switch kind {
        case "relief": return "RELIEVE THE LIVE CAUSE"
        case "guidance": return "HOW TO ADDRESS"
        default: return "RESOLVE"
        }
    }
    private var fixButtonLabel: String {
        switch kind {
        case "relief": return "Relieve the live cause"
        case "guidance": return "Show how to address"
        default: return "Fix it"
        }
    }
    // The expectation set BEFORE the click — the heart of the honesty fix.
    private var fixExpectation: String? {
        switch kind {
        case "relief":
            return "This is a 7-day pattern — a record of what already happened. Relieving the live cause eases it now; the historical count decays as clean days pass, so it won't drop the instant you click."
        case "guidance":
            return "This acts only while the issue is happening live. If it isn't happening right now, you'll get steps to prevent it — the status won't change immediately."
        case "instant":
            if finding.check == "App Crashes (7d)" {
                return "These are the last 7 days of crash reports. Clearing the backlog drops the count now — it won't stop future crashes, but the signal clears."
            }
            return nil   // disk / binary-drift: a plain instant fix, no caveat needed
        default:
            return nil
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: finding.check)
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    HStack(alignment: .top, spacing: 8) {
                        Circle().fill(findingColor(finding)).frame(width: 10, height: 10).padding(.top, 4)
                        Text(finding.message).sirsiFont(14, weight: .semibold)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    if let d = finding.detail, !d.isEmpty {
                        // Live data is never tiny or greyed: a pipe-separated
                        // detail ("Python (6.2 GB) | node (1.1 GB) | …") becomes
                        // one readable row per item; prose wraps at .callout.
                        if let entries = parseFindingDetailList(d) {
                            VStack(spacing: 0) {
                                ForEach(entries) { e in
                                    HStack(alignment: .firstTextBaseline) {
                                        Text(e.name).sirsiFont(.callout)
                                            .lineLimit(1).truncationMode(.middle)
                                        Spacer(minLength: 12)
                                        if let v = e.value {
                                            Text(v).sirsiFont(.callout, design: .monospaced)
                                        }
                                    }
                                    .padding(.horizontal, 10).padding(.vertical, 5)
                                    if e.id != entries.count - 1 { Divider() }
                                }
                            }
                            .textSelection(.enabled)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        } else {
                            Text(d).sirsiFont(.callout)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .padding(10)
                                .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        }
                    }
                    if let fix = finding.fix, !fix.isEmpty {
                        // Honest framing BEFORE the click: a 7-day history must never
                        // look like an instant cure — that's the "I clicked Fix and
                        // nothing changed" trap. The banner sets the true expectation.
                        if let note = fixExpectation {
                            HStack(alignment: .top, spacing: 6) {
                                Image(systemName: fixIcon).sirsiFont(.caption)
                                    .foregroundStyle(.secondary).padding(.top, 1)
                                Text(note).sirsiFont(.caption).foregroundStyle(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        }
                        Text(fixSectionLabel).sirsiFont(.caption2, weight: .semibold).foregroundStyle(.secondary)
                        NavLink {
                            ResultView(engine: engine, title: finding.check, args: sirsiArgs(fix),
                                       reverifyCheck: finding.check, reverifyKind: finding.fixKind)
                        } label: {
                            HStack(spacing: 8) {
                                Image(systemName: fixIcon)
                                VStack(alignment: .leading, spacing: 1) {
                                    Text(fixButtonLabel).sirsiFont(12, weight: .semibold)
                                    Text(fix).sirsiFont(.caption2, design: .monospaced)
                                        .foregroundStyle(Color.white.opacity(0.85))
                                }
                                Spacer()
                            }.frame(maxWidth: .infinity).padding(.vertical, 2)
                        }.buttonStyle(.borderedProminent).tint(gold)
                    } else if isAlarm {
                        // An alarm without a lever must SAY so — calling a warn
                        // or critical finding "Informational" was the dead-end
                        // the owner flagged (ADR-033: alarm ⇒ way to act).
                        Text("This needs attention but has no one-click fix yet.")
                            .sirsiFont(.callout).foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    } else if let cmd = recommendedCommand {
                        // Guidance-tier (e.g. caution items cleared deliberately
                        // in Terminal): the command it names must be actionable,
                        // not buried in prose ending at "Informational."
                        Text("RECOMMENDED — RUN IN TERMINAL").sirsiFont(.caption2, weight: .semibold).foregroundStyle(.secondary)
                        Text(cmd).sirsiFont(.caption, design: .monospaced).foregroundStyle(gold)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading).padding(10)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.06)))
                        HStack(spacing: 8) {
                            Button {
                                copyToClipboard(cmd)
                                copied = true
                            } label: { Label(copied ? "Copied" : "Copy command", systemImage: copied ? "checkmark" : "doc.on.doc") }
                            Button { openTerminal() } label: { Label("Open Terminal", systemImage: "terminal") }
                        }.sirsiFont(.caption)
                    } else {
                        Text("Informational — nothing to act on.")
                            .sirsiFont(.callout).foregroundStyle(.secondary)
                    }
                    Spacer()
                }.padding(16)
            }
        }
    }
}

extension View {
    // Visually mark a not-yet-ported deity row without a dead-looking control.
    func disabledRow() -> some View { self.opacity(0.45) }
}

// ── Fleet (shared producer — matches the Horus board by construction) ────────
//
// Renders `sirsi router fleet --json`, the SAME computation Horus serves at
// /api/fleet. This surface deliberately does NO aggregation of its own: the
// menubar previously derived its fabric view from `router node-status` while
// Horus derived from ledger.Build, and two read models cannot agree. Matching
// is a property of sharing the producer, not of keeping two implementations in
// sync by hand.
//
// The activity feed is intentionally absent: it is a diff of consecutive reads
// and needs a long-lived process to hold the baseline. Horus has one; a panel
// that opens and closes does not. Showing an always-empty feed here would read
// as "nothing is happening".
struct FleetView: View {
    @ObservedObject var engine: SirsiEngine

    private func stateColor(_ st: String) -> Color {
        switch st {
        case "working": return .green
        case "blocked": return .orange
        default: return .secondary
        }
    }

    private func stateLabel(_ st: String) -> String {
        switch st {
        case "working": return "WORKING"
        case "blocked": return "blocked"
        default: return "stopped — no open work"
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            BackBar(title: "Fleet")
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if let err = engine.fleetError {
                        Text("Fleet read failed: \(err)")
                            .sirsiFont(12).foregroundColor(.orange)
                    }
                    if let b = engine.fleetBoard {
                        let s = b.summary
                        // Every percentage ships the counts it came from, so a
                        // number can never be read without its denominator.
                        VStack(alignment: .leading, spacing: 6) {
                            FleetTile(label: "COMPLETED / IN FLIGHT",
                                      value: "\(s.done) / \(s.total)",
                                      detail: "\(s.pctDone)% done · \(s.inFlight) still in flight")
                            FleetTile(label: "IN PROGRESS / ASSIGNED",
                                      value: "\(s.active) / \(s.active + s.assigned)",
                                      detail: "\(s.assigned) assigned but not started")
                            FleetTile(label: "STALLED / BLOCKED",
                                      value: "\(s.stalled + s.blocked)",
                                      detail: "\(s.stalled) stalled · \(s.blocked) blocked · \(s.idleLanes) idle lanes")
                        }
                        Text("\(s.lanesWorking) of \(s.lanesTotal) lanes actively working")
                            .sirsiFont(11).foregroundColor(.secondary)
                        ForEach(b.lanes) { l in
                            HStack(alignment: .firstTextBaseline, spacing: 8) {
                                Text(l.agent).sirsiFont(12, weight: .medium)
                                    .sirsiFrame(width: 150).frame(alignment: .leading)
                                    .lineLimit(1)
                                Text(stateLabel(l.state)).sirsiFont(11)
                                    .foregroundColor(stateColor(l.state))
                                    .sirsiFrame(width: 130).frame(alignment: .leading)
                                    .lineLimit(1)
                                Text(laneCounts(l)).sirsiFont(11)
                                    .foregroundColor(.secondary)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .lineLimit(1)
                            }
                        }
                    } else if engine.fleetLoading {
                        Text("reading fleet…").sirsiFont(12).foregroundColor(.secondary)
                    }
                }
                .padding(14)
            }
        }
        .task { await engine.loadFleetBoard() }
    }

    private func laneCounts(_ l: FleetLane) -> String {
        var parts = ["\(l.open) open"]
        if l.active > 0 { parts.append("\(l.active) active") }
        if l.stalled > 0 { parts.append("\(l.stalled) stalled") }
        if l.blocked > 0 { parts.append("\(l.blocked) blocked") }
        if let t = l.touchedAgo, !t.isEmpty { parts.append("last ledger update " + t) }
        return parts.joined(separator: " · ")
    }
}

struct FleetTile: View {
    let label: String, value: String, detail: String
    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).sirsiFont(10, weight: .semibold).foregroundColor(.secondary)
            Text(value).sirsiFont(20, weight: .semibold)
            Text(detail).sirsiFont(11).foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

// ── Router — Fabric (liveness + wake-enablement) ─────────────────────────────
//
// The Router view is the owner-actionable board: it leads with BLOCKERS (only
// current, fixable conditions — a real logout, a broken router daemon), then
// stranded inboxes (per-agent open-item counts, each with a one-click "Arm wake
// channel"). A degraded/inconclusive auth probe is shown as plain INFO, never an
// alarm — nothing the user clicks would clear it, so it must not read red
// (feedback_surfaces_current_actionable_only). A healthy fabric reads calm green.

// copyToClipboard puts a string on the general pasteboard (for the re-auth
// command — we never authenticate programmatically, we hand the operator the
// exact command to run themselves).
func copyToClipboard(_ s: String) {
    NSPasteboard.general.clearContents()
    NSPasteboard.general.setString(s, forType: .string)
}

// openTerminal launches Terminal.app so the operator can re-auth by hand. We open
// the app (not a command) — authentication is the user's action, never ours.
func openTerminal() {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: "/usr/bin/open")
    p.arguments = ["-a", "Terminal"]
    try? p.run()
}

struct RouterView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var resultLine: String?
    @Environment(\.snapshotMode) private var snapshotMode

    // ImageRenderer draws ScrollView viewports EMPTY — swap for a plain stack
    // under snapshot QA (same contract as HomeView.maybeScroll / ResultView).
    @ViewBuilder private func maybeScrollRouter<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        if snapshotMode {
            content().frame(maxHeight: .infinity, alignment: .top)
        } else {
            ScrollView { content() }
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Router — Fabric")
            maybeScrollRouter {
                VStack(alignment: .leading, spacing: 14) {

                    // ── Honest empty state: never a false "healthy" ─────────
                    if engine.routerBoard == nil {
                        HStack(spacing: 8) {
                            Circle().fill(.gray).frame(width: 8, height: 8)
                            Text(engine.routerLoading ? "Reading the fabric…" : "No fabric data yet — the board hasn't been generated on this machine.")
                                .sirsiFont(13, weight: .medium)
                                .fixedSize(horizontal: false, vertical: true)
                            Spacer()
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.05)))
                    }

                    // ── Fabric overview — the actual work map ───────────────
                    if let board = engine.routerBoard {
                        SectionLabel("THE FABRIC RIGHT NOW")
                        HStack(spacing: 16) {
                            VStack(alignment: .leading, spacing: 1) {
                                Text("\(board.liveThreadCount ?? 0)").sirsiFont(22, weight: .bold).foregroundStyle(.green)
                                Text("live threads").sirsiFont(.caption2).foregroundStyle(.secondary)
                            }
                            VStack(alignment: .leading, spacing: 1) {
                                Text("\(board.totalPending ?? 0)").sirsiFont(22, weight: .bold)
                                    .foregroundStyle((board.totalPending ?? 0) > 0 ? gold : .green)
                                Text("open items").sirsiFont(.caption2).foregroundStyle(.secondary)
                            }
                            Spacer()
                        }
                        let pending = (board.pendingByAgent ?? [:]).filter { !$0.value.isEmpty }
                        if !pending.isEmpty {
                            VStack(spacing: 0) {
                                ForEach(pending.keys.sorted(), id: \.self) { agent in
                                    HStack {
                                        Text(agent).sirsiFont(.caption)
                                        Spacer()
                                        Text("\(pending[agent]?.count ?? 0) open").sirsiFont(.caption, design: .monospaced).foregroundStyle(.secondary)
                                    }.padding(.vertical, 6)
                                    if agent != pending.keys.sorted().last { Divider() }
                                }
                            }
                            .padding(.horizontal, 12)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        }
                    }

                    // ── Blockers (current + fixable ONLY) ───────────────────
                    if engine.routerBoard != nil && engine.routerHasBlockers {
                        SectionLabel("BLOCKERS — FIX TO UNSTRAND WORK", tint: .red)

                        ForEach(engine.routerAuthBlockers) { h in
                            AuthBlockerCard(engine: engine, health: h)
                        }
                        if !engine.routerDaemonBlockers.isEmpty {
                            DaemonBlockerCard(engine: engine,
                                              broken: engine.routerDaemonBlockers,
                                              onResult: { resultLine = $0 })
                        }
                    } else if engine.routerBoard != nil {
                        HStack(spacing: 8) {
                            Circle().fill(.green).frame(width: 8, height: 8)
                            Text("No blockers — fabric is healthy")
                                .sirsiFont(13, weight: .semibold)
                            Spacer()
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(RoundedRectangle(cornerRadius: 9).fill(Color.green.opacity(0.10)))
                    }

                    // ── Stranded inboxes (work-to-do, not an alarm) ─────────
                    if !engine.routerStranded.isEmpty {
                        SectionLabel("STRANDED INBOXES — OPEN ITEMS, NO WATCHER")
                        Text("These agents have work waiting but no armed session watching. Arm a wake channel so their inbox is pulled automatically.")
                            .sirsiFont(.caption).foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                        ForEach(engine.routerStranded) { s in
                            NavLink {
                                StrandedAgentView(engine: engine, agent: s)
                            } label: {
                                HStack(spacing: 10) {
                                    Text("📥").sirsiFont(16).frame(width: 24)
                                    VStack(alignment: .leading, spacing: 1) {
                                        Text(s.agentId).sirsiFont(13, weight: .medium)
                                        Text("\(s.openItems) item\(s.openItems == 1 ? "" : "s") waiting")
                                            .sirsiFont(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    Image(systemName: "chevron.right").sirsiFont(.caption2).foregroundStyle(.tertiary)
                                }
                                .padding(.vertical, 8).padding(.horizontal, 10)
                                .contentShape(Rectangle())
                                .background(RoundedRectangle(cornerRadius: 7).fill(Color.primary.opacity(0.04)))
                            }.buttonStyle(.plain)
                        }
                    }

                    // ── Inconclusive probes (plain info, never an alarm) ────
                    if !engine.routerDegraded.isEmpty {
                        SectionLabel("PROBE INCONCLUSIVE — INFORMATIONAL")
                        ForEach(engine.routerDegraded) { h in
                            HStack(alignment: .top, spacing: 8) {
                                Text("🛈").sirsiFont(.callout)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text("\(h.agentType) — auth check inconclusive")
                                        .sirsiFont(12, weight: .medium)
                                    Text("The CLI didn't answer in time (a cold start), so we can't confirm login. This is not a logout — it clears on its own and blocks nothing.")
                                        .sirsiFont(.caption).foregroundStyle(.secondary)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        }
                    }

                    if let line = resultLine {
                        Text(line).sirsiFont(.caption, design: .monospaced).foregroundStyle(.secondary)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }

                    Spacer()
                }.padding(16)
            }
            if engine.busy {
                HStack { ProgressView().controlSize(.small); Text("Working…").sirsiFont(.caption).foregroundStyle(.secondary); Spacer() }
                    .padding(.horizontal, 16).padding(.bottom, 8)
            }
        }
        .task { await engine.loadRouterBoard() }
    }
}

// SectionLabel is a small caption header used across the Router view.
struct SectionLabel: View {
    let text: String
    var tint: Color = .secondary
    init(_ text: String, tint: Color = .secondary) { self.text = text; self.tint = tint }
    var body: some View {
        Text(text).sirsiFont(.caption2, weight: .semibold).foregroundStyle(tint)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

// AuthBlockerCard surfaces a REAL logout (needs_login) with a re-auth affordance.
// We never authenticate programmatically — we open Terminal and hand the operator
// the exact command to run, and offer to copy it.
struct AuthBlockerCard: View {
    @ObservedObject var engine: SirsiEngine
    let health: RBAgentHealth

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Text("🔑").sirsiFont(18)
                VStack(alignment: .leading, spacing: 1) {
                    Text("\(health.agentType) needs re-login")
                        .sirsiFont(13, weight: .semibold)
                    Text(blockedNote).sirsiFont(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
            Text("Sirsi never signs in for you. Open Terminal, run \(health.agentType), then /login.")
                .sirsiFont(.caption).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 8) {
                Button {
                    openTerminal()
                } label: {
                    Label("Open Terminal", systemImage: "terminal").frame(maxWidth: .infinity)
                }.buttonStyle(.borderedProminent).tint(gold)
                Button {
                    copyToClipboard(health.agentType)
                } label: {
                    Label("Copy command", systemImage: "doc.on.doc").frame(maxWidth: .infinity)
                }.buttonStyle(.bordered)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.red.opacity(0.10)))
    }

    private var blockedNote: String {
        let n = health.blockedItems ?? 0
        return n > 0 ? "blocking \(n) item\(n == 1 ? "" : "s")" : "some work can't dispatch"
    }
}

// DaemonBlockerCard surfaces missing/broken router LaunchAgents with a one-click
// `sirsi router install-daemons` repair.
struct DaemonBlockerCard: View {
    @ObservedObject var engine: SirsiEngine
    let broken: [RBLaunchAgent]
    let onResult: (String) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Text("⚙️").sirsiFont(18)
                VStack(alignment: .leading, spacing: 1) {
                    Text("\(broken.count) router daemon\(broken.count == 1 ? "" : "s") missing")
                        .sirsiFont(13, weight: .semibold)
                    Text("Work can't relay while a session is closed.")
                        .sirsiFont(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
            ForEach(broken) { d in
                Text("• \(friendlyDaemon(d.role)) (\(d.label))")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            Button {
                Task { onResult(await engine.installRouterDaemons()) }
            } label: {
                Label("Install router daemons", systemImage: "wrench.and.screwdriver.fill")
                    .frame(maxWidth: .infinity)
            }.buttonStyle(.borderedProminent).tint(gold).disabled(engine.busy)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.red.opacity(0.10)))
    }
}

// friendlyDaemon turns a router role into plain English.
func friendlyDaemon(_ role: String) -> String {
    switch role {
    case "router-supervisor": return "Background router supervisor"
    case "router-watchpaths": return "Live dispatch (on change)"
    case "router-sweep": return "Hourly queue sweep"
    case "registry-police": return "Thread cleanup"
    default: return role
    }
}

// StrandedAgentView drills into one stranded agent: the open-item count and the
// one-click "Arm wake channel" (sirsi router wake-install <agent>).
struct StrandedAgentView: View {
    @ObservedObject var engine: SirsiEngine
    let agent: RBStranded
    @State private var resultLine: String?

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: agent.agentId)
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    VStack(spacing: 4) {
                        Text("\(agent.openItems)").sirsiFont(34, weight: .bold).foregroundStyle(gold)
                        Text("item\(agent.openItems == 1 ? "" : "s") waiting").sirsiFont(.caption).foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity).padding(.vertical, 8)

                    Text("This agent has work in its inbox but no armed session watching. Arming a wake channel installs a pull-loop that checks its inbox automatically, so the work no longer waits for someone to open the session.")
                        .sirsiFont(.callout).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)

                    Button {
                        Task { resultLine = await engine.installWake(agent: agent.agentId) }
                    } label: {
                        Label("Arm wake channel", systemImage: "bolt.horizontal.circle")
                            .frame(maxWidth: .infinity)
                    }.buttonStyle(.borderedProminent).tint(gold).disabled(engine.busy)

                    Text("Runs: sirsi router wake-install \(agent.agentId)")
                        .sirsiFont(.caption2, design: .monospaced).foregroundStyle(.tertiary)

                    if let line = resultLine {
                        Text(line).sirsiFont(.caption, design: .monospaced).foregroundStyle(.secondary)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(10)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                    }
                    if engine.busy {
                        HStack { ProgressView().controlSize(.small); Text("Arming…").sirsiFont(.caption).foregroundStyle(.secondary) }
                    }
                    Spacer()
                }.padding(16)
            }
        }
    }
}

// ── Anubis ───────────────────────────────────────────────────────────────────

struct AnubisView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Anubis — Hygiene")
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    HStack {
                        VStack(alignment: .leading) {
                            Text("\(SirsiEngine.human(engine.safeBytes)) safe").sirsiFont(.title2, weight: .bold).foregroundStyle(gold)
                            Text("\(engine.safe.count) regenerable items · trash-first, recoverable")
                                .sirsiFont(.caption).foregroundStyle(.secondary)
                        }
                        Spacer()
                    }
                    .padding(.top, 6)

                    // ONE unified flow (was two split, confusing buttons): scan
                    // with visible progress → review every item → clean the ones
                    // you pick. ScanCleanView owns the whole workflow.
                    NavLink { ScanCleanView(engine: engine) } label: {
                        ActionCard(glyph: "🧹", title: "Scan & Clean Waste",
                                   sub: "Find waste, review every item, move what you choose to Trash")
                    }.buttonStyle(.plain)

                    // A real structured screen — the list of leftover apps and what
                    // to do — not a terminal transcript dumped into the popover.
                    NavLink { GhostsView(engine: engine) } label: {
                        ActionCard(glyph: "👻", title: "Find Leftover Apps",
                                   sub: "Remnants of apps you've uninstalled (Ka)")
                    }.buttonStyle(.plain)

                    // The Trash is where every OTHER clean path leaves things —
                    // recoverable by design. This is the one screen that can make
                    // that permanent, so it is its own drill-down with its own
                    // confirmation, never folded into the one-click clean.
                    NavLink { EmptyTrashView(engine: engine) } label: {
                        ActionCard(glyph: "🗑", title: "Empty Trash",
                                   sub: "Permanently delete what Sirsi (and you) moved to Trash — no undo")
                    }.buttonStyle(.plain)

                    // Legible, plain-English note about what's held back (was tiny).
                    if engine.cautionBytes > 0 {
                        ExclusionNote(bytes: engine.cautionBytes, count: engine.caution.count)
                    }
                }
                .padding(16)
            }
        }
        .navigationTitle("Anubis")
    }
}

// EmptyTrashView — the only surface in Sirsi that destroys something
// permanently. Everything else is trash-first and recoverable, so this screen
// is deliberately shaped against the rest of the app:
//
//   - it SHOWS the contents before offering the action (you cannot permanently
//     delete a list you have not seen);
//   - the destructive button is disabled until that list has loaded, so it can
//     never fire against unknown contents;
//   - it takes TWO taps, and the second one is the one that says "permanently";
//   - the result line reports what was actually freed, read back from the CLI,
//     not an optimistic "done".
struct EmptyTrashView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var count = 0
    @State private var sizeText = ""
    @State private var items: [String] = []
    @State private var loading = true
    @State private var arming = false
    @State private var result: String?

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Empty Trash")
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if loading {
                        HStack(spacing: 8) {
                            ProgressView().controlSize(.small)
                            Text("Reading Trash…").sirsiFont(12).foregroundStyle(.secondary)
                        }
                    } else if let r = result {
                        HStack(spacing: 8) {
                            Image(systemName: "checkmark.seal.fill").foregroundStyle(.green)
                            Text(r).sirsiFont(13, weight: .semibold)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    } else if count == 0 {
                        Text("Trash is empty — nothing to delete.")
                            .sirsiFont(13).foregroundStyle(.secondary)
                    } else {
                        Text("\(count) item\(count == 1 ? "" : "s") · \(sizeText)")
                            .sirsiFont(17, weight: .bold).foregroundStyle(gold)
                        Text("These are already in the Trash. Emptying it removes them for good — Sirsi cannot restore them, and neither can Finder.")
                            .sirsiFont(12).foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)

                        VStack(alignment: .leading, spacing: 3) {
                            ForEach(items.prefix(12), id: \.self) { line in
                                Text("· " + line).sirsiFont(11).foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                            if items.count > 12 {
                                Text("… and \(items.count - 12) more")
                                    .sirsiFont(11).foregroundStyle(.tertiary)
                            }
                        }

                        if arming {
                            Button(role: .destructive) {
                                Task {
                                    let out = await engine.emptyTrash()
                                    result = out
                                    arming = false
                                }
                            } label: {
                                Text("Permanently delete \(count) item\(count == 1 ? "" : "s")")
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(.red)
                            .disabled(engine.busy)
                            Button("Cancel") { arming = false }
                                .buttonStyle(.plain)
                                .sirsiFont(12)
                                .foregroundStyle(.secondary)
                        } else {
                            Button {
                                arming = true
                            } label: {
                                Text("Empty Trash…").frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.bordered)
                            .disabled(loading || count == 0)
                        }
                    }
                }
                .padding(16)
            }
        }
        .task {
            let r = await engine.trashList()
            count = r.count; sizeText = r.size; items = r.lines
            loading = false
        }
        .navigationTitle("Empty Trash")
    }
}

struct ActionCard: View {
    let glyph: String; let title: String; let sub: String
    var body: some View {
        HStack(spacing: 12) {
            Text(glyph).sirsiFont(22).sirsiFrame(width: 30)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).sirsiFont(14, weight: .semibold)
                Text(sub).sirsiFont(.caption).foregroundStyle(.secondary).multilineTextAlignment(.leading)
            }
            Spacer()
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.05)))
    }
}

// shortenPath renders a path with $HOME collapsed to ~ for legibility.
func shortenPath(_ p: String) -> String {
    let home = FileManager.default.homeDirectoryForCurrentUser.path
    return p.hasPrefix(home) ? "~" + p.dropFirst(home.count) : p
}

// prettyBundle turns a bundle id / folder name into a friendly app name:
// "com.google.Chrome" → "Chrome", "com.apple.Safari" → "Safari".
func prettyBundle(_ s: String) -> String {
    let parts = s.components(separatedBy: ".")
    if parts.count >= 2, ["com", "org", "io", "net", "app"].contains(parts[0]) {
        return (parts.last ?? s).replacingOccurrences(of: "-", with: " ")
    }
    return s
}

// owningEntity answers "whose is this?" for a finding — the app, project, or
// developer tool the file belongs to — derived from the path and rule. This is
// what makes each row drillable and honest ("Google Chrome", "FinalWishes
// (project)", "Go (developer tool)") instead of an opaque path.
func owningEntity(_ f: Finding) -> String {
    let p = f.path
    let comps = p.components(separatedBy: "/")
    // Project-scoped dev artifacts → the project folder name.
    for marker in ["node_modules", ".next", "target", "build", "dist", "DerivedData"] {
        if let i = comps.firstIndex(of: marker), i > 0 {
            return comps[i - 1] + " (project)"
        }
    }
    // App-scoped caches/support/containers → the owning app.
    for anchor in ["/Library/Caches/", "/Library/Application Support/", "/Library/Containers/", "/Library/Group Containers/"] {
        if let r = p.range(of: anchor) {
            let first = String(p[r.upperBound...]).components(separatedBy: "/").first ?? ""
            if !first.isEmpty { return prettyBundle(first) }
        }
    }
    // Developer-tool caches identified by rule/description.
    let d = (f.rule ?? "").lowercased() + " " + f.description.lowercased()
    if d.contains("npm") || d.contains("yarn") || d.contains("pnpm") || d.contains("node") { return "npm / Node (developer tool)" }
    if d.contains("go_mod") || d.contains("go module") || d.contains("golang") { return "Go (developer tool)" }
    if d.contains("cargo") || d.contains("rust") { return "Rust / Cargo (developer tool)" }
    if d.contains("pip") || d.contains("python") { return "Python (developer tool)" }
    if d.contains("docker") { return "Docker" }
    if d.contains("xcode") || d.contains("deriveddata") { return "Xcode" }
    if d.contains("brew") || d.contains("homebrew") { return "Homebrew" }
    return f.category?.capitalized ?? "System"
}

// ExclusionNote — legible, plain-English explanation of what's held back from
// one-click cleaning and why (was an unreadably tiny caption2 before).
struct ExclusionNote: View {
    let bytes: Int64; let count: Int
    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Text("🛈").sirsiFont(.callout)
            VStack(alignment: .leading, spacing: 2) {
                Text("\(SirsiEngine.human(bytes)) held back for now")
                    .sirsiFont(.callout, weight: .semibold)
                Text("\(count) caution-tier items (things like package caches and app remnants) aren't cleaned with one click, because they take longer to rebuild. Open Scan & Clean to review them.")
                    .sirsiFont(.footnote).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.05)))
    }
}

// ── Scan & Clean — the ONE unified flow: scan → review each item → clean ─────
//
// Replaces the old split "Scan for Waste" (silent, no screen) + "Review & Clean"
// (no per-item control) with a single drillable workflow: visible scan progress,
// a per-item checklist you curate, per-row drill-in, and a clean scoped to
// exactly what you picked (Go `--only`, intersection-only).
struct ScanCleanView: View {
    @ObservedObject var engine: SirsiEngine
    @EnvironmentObject private var nav: Nav
    @State private var selected: Set<String> = []
    @State private var resultLine: String?
    @State private var showCaution = false
    @State private var didInit = false

    private var selectedSafe: [Finding] { engine.safe.filter { selected.contains($0.path) } }
    private var selectedBytes: Int64 { selectedSafe.reduce(0) { $0 + $1.sizeBytes } }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Scan & Clean")
            content
        }
        .navigationTitle("Scan & Clean")
        .task { if !didInit { didInit = true; syncSelection() } }
    }

    @ViewBuilder private var content: some View {
        if let resultLine {
            resultState(resultLine)
        } else if engine.busy {
            progressState
        } else if engine.safe.isEmpty {
            emptyState
        } else {
            reviewList
            Divider()
            bottomBar
        }
    }

    // Progress — scanning OR cleaning; both read as visible work, never silent.
    private var progressState: some View {
        VStack(spacing: 12) {
            ProgressView()
            Text(engine.safe.isEmpty ? "Scanning your Mac for waste…" : "Moving selected items to Trash…")
                .sirsiFont(.callout).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity).padding(.top, 60)
    }

    // Empty — no scan yet, or nothing safe to clean. Always offers the next step.
    private var emptyState: some View {
        VStack(spacing: 14) {
            Text(engine.scannedAt.isEmpty ? "🔍" : "✓")
                .sirsiFont(40)
                .foregroundStyle(engine.scannedAt.isEmpty ? Color.secondary : .green)
            Text(engine.scannedAt.isEmpty
                 ? "Scan your Mac to find reclaimable waste."
                 : "Nothing safe to clean right now.")
                .sirsiFont(.callout).multilineTextAlignment(.center)
            Button { Task { await engine.rescan(); syncSelection() } } label: {
                Label("Scan now", systemImage: "magnifyingglass").frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent).tint(gold)
        }
        .frame(maxWidth: .infinity).padding(24)
    }

    private func resultState(_ line: String) -> some View {
        VStack(spacing: 14) {
            Text("✓").sirsiFont(40).foregroundStyle(.green)
            Text(line).sirsiFont(.callout).multilineTextAlignment(.center)
            Text("Moved to Trash — recoverable until you empty it.")
                .sirsiFont(.caption).foregroundStyle(.secondary)
            Button { nav.pop() } label: { Text("Done").frame(maxWidth: .infinity) }
                .buttonStyle(.borderedProminent).tint(gold).padding(.top, 4)
        }
        .frame(maxWidth: .infinity).padding(24)
    }

    private var reviewList: some View {
        MaybeList {
            Section {
                ForEach(engine.safe) { f in
                    itemRow(f, toggleable: true)
                }
            } header: {
                Text("REVIEW — \(selected.count) of \(engine.safe.count) selected · \(SirsiEngine.human(selectedBytes))")
            } footer: {
                Text("Regenerable caches, node_modules and build artifacts. Protected system paths are never touched. Tap a row for details.")
            }

            if !engine.caution.isEmpty {
                Section {
                    DisclosureGroup(isExpanded: $showCaution) {
                        ForEach(engine.caution) { f in itemRow(f, toggleable: false) }
                    } label: {
                        Text("Held back — \(engine.caution.count) caution items · \(SirsiEngine.human(engine.cautionBytes))")
                            .sirsiFont(.callout, weight: .semibold)
                    }
                } footer: {
                    Text("Not cleaned with one click — these take longer to rebuild. Tap any item to see what it is; clean deliberately in Terminal with `sirsi anubis clean --include-caution --confirm`.")
                }
            }
        }
        .listStyle(.inset)
    }

    // One row: an optional checkbox (safe items only) + a drillable label that
    // says what it is and whose it is, plus its size.
    private func itemRow(_ f: Finding, toggleable: Bool) -> some View {
        HStack(spacing: 8) {
            if toggleable {
                Button { toggle(f.path) } label: {
                    // 15pt glyph in a padded region — a bare icon toggle was a
                    // ~15pt target you had to poke at (same class as BackBar).
                    Image(systemName: selected.contains(f.path) ? "checkmark.circle.fill" : "circle")
                        .sirsiFont(15)
                        .foregroundStyle(selected.contains(f.path) ? gold : Color.secondary)
                        .sirsiFrame(width: 34, height: 34)
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
            NavLink { ItemDetailView(engine: engine, finding: f) } label: {
                HStack(spacing: 8) {
                    VStack(alignment: .leading, spacing: 1) {
                        Text(f.description).sirsiFont(.caption).lineLimit(1)
                        Text(owningEntity(f)).sirsiFont(.caption2).foregroundStyle(.secondary).lineLimit(1)
                    }
                    Spacer(minLength: 6)
                    Text(SirsiEngine.human(f.sizeBytes))
                        .sirsiFont(.caption, design: .monospaced)
                        .foregroundStyle(toggleable ? gold : .secondary)
                    Image(systemName: "chevron.right").sirsiFont(.caption2).foregroundStyle(.tertiary)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        }
    }

    private var bottomBar: some View {
        HStack(spacing: 10) {
            Button(selected.count == engine.safe.count ? "Select none" : "Select all") {
                if selected.count == engine.safe.count { selected.removeAll() }
                else { syncSelection() }
            }
            .sirsiFont(.caption).buttonStyle(.plain).foregroundStyle(gold)
            Spacer()
            Button {
                Task { resultLine = await engine.cleanSelected(paths: selectedSafe.map { $0.path }) }
            } label: {
                Text("Move \(selected.count) (\(SirsiEngine.human(selectedBytes))) to Trash")
            }
            .buttonStyle(.borderedProminent).tint(gold)
            .disabled(selected.isEmpty)
        }
        .padding(12)
    }

    private func toggle(_ path: String) {
        if selected.contains(path) { selected.remove(path) } else { selected.insert(path) }
    }

    // Default selection = every safe item (opt-out curation). Re-synced after a
    // fresh scan so newly-found items start selected.
    private func syncSelection() { selected = Set(engine.safe.map { $0.path }) }
}

// DetailRow — one labelled fact in the item drill-in.
struct DetailRow: View {
    let label: String; let value: String; var mono = false
    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            Text(label.uppercased()).sirsiFont(.caption2).foregroundStyle(.tertiary)
            Text(value)
                // Conditional style: the ternary picked between two SEMANTIC
                // fonts, so a plain textual sweep could not convert it. Split so
                // both arms scale.
                .sirsiFont(mono ? .caption : .callout, design: mono ? .monospaced : .default)
                .foregroundStyle(.primary)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

// ── Item detail — drill-in: what it is, where, whose, what happens if removed ─
struct ItemDetailView: View {
    @ObservedObject var engine: SirsiEngine
    let finding: Finding
    @EnvironmentObject private var nav: Nav
    @State private var resultLine: String?

    private var isSafe: Bool { finding.severity == "safe" }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Item")
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text(finding.description).sirsiFont(.headline)

                    DetailRow(label: "Size", value: SirsiEngine.human(finding.sizeBytes)
                              + (finding.fileCount.map { " · \($0) files" } ?? ""))
                    DetailRow(label: "Belongs to", value: owningEntity(finding))
                    DetailRow(label: "Where", value: shortenPath(finding.path), mono: true)
                    if let cat = finding.category, !cat.isEmpty {
                        DetailRow(label: "Kind", value: cat.capitalized)
                    }
                    if let adv = finding.advisory, !adv.isEmpty {
                        DetailRow(label: "What happens if removed", value: adv)
                    }

                    Button {
                        NSWorkspace.shared.selectFile(finding.path, inFileViewerRootedAtPath: "")
                    } label: {
                        Label("Reveal in Finder", systemImage: "folder").sirsiFont(.caption)
                    }
                    .buttonStyle(.plain).foregroundStyle(gold).padding(.top, 2)
                }
                .padding(16)
            }

            Divider()
            if let resultLine {
                HStack {
                    Text("✓ \(resultLine)").sirsiFont(.caption).foregroundStyle(.green)
                    Spacer()
                    Button("Done") { nav.pop() }.sirsiFont(.caption)
                }.padding(12)
            } else if isSafe {
                Button {
                    Task { resultLine = await engine.cleanSelected(paths: [finding.path]) }
                } label: {
                    if engine.busy {
                        HStack { ProgressView().controlSize(.small); Text("Moving to Trash…").sirsiFont(.caption) }
                            .frame(maxWidth: .infinity)
                    } else {
                        Text("Move this to Trash (\(SirsiEngine.human(finding.sizeBytes)))")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent).tint(gold).disabled(engine.busy).padding(12)
            } else {
                Text("Held back from one-click cleaning — rebuild it deliberately.")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity).padding(12)
            }
        }
        .navigationTitle("Item")
    }
}

// ── Ghosts (Ka) — structured leftover-app screen, not a CLI transcript ───────
//
// Checkpoint risk contract — mirrors `sirsi risk --json` (bespoke shape, kept
// stable for its TUI/script consumers; the popover decodes it natively rather
// than forcing the CLI onto CommandResult).
struct RiskReport: Decodable {
    let uncommittedFiles: Int
    let untrackedFiles: Int
    let modifiedFiles: Int
    let linesAdded: Int
    let linesDeleted: Int
    let lastCommitTime: String
    let lastCommitMessage: String
    let timeSinceCommit: Double // Go time.Duration NANOSECONDS (verified live: 1.79e15 ≈ 20.8d)
    let risk: String
    let warning: String?
    let branch: String
    let repoRoot: String
    enum CodingKeys: String, CodingKey {
        case risk, warning, branch
        case uncommittedFiles = "uncommitted_files"
        case untrackedFiles = "untracked_files"
        case modifiedFiles = "modified_files"
        case linesAdded = "lines_added"
        case linesDeleted = "lines_deleted"
        case lastCommitTime = "last_commit_time"
        case lastCommitMessage = "last_commit_message"
        case timeSinceCommit = "time_since_commit"
        case repoRoot = "repo_root"
    }
}

// Osiris — Checkpoints: uncommitted-work risk for the configured project.
// Honest states: no project configured → guidance; project configured → the
// real numbers (files at risk, line churn, time since the last checkpoint).
struct RiskView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var report: RiskReport?
    @State private var rawFallback: String?
    @State private var loading = true
    @State private var checkpointing = false
    @State private var actionResult: String?

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Osiris — Checkpoints")
            if loading {
                ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top).padding(.top, 60)
            } else if let r = report {
                MaybeList {
                    Section {
                        HStack {
                            Text(riskGlyph(r.risk)).sirsiFont(18)
                            VStack(alignment: .leading, spacing: 1) {
                                Text("\(r.uncommittedFiles) file\(r.uncommittedFiles == 1 ? "" : "s") not checkpointed")
                                    .sirsiFont(13, weight: .semibold)
                                Text("Last commit \(SirsiEngine.humanDuration(r.timeSinceCommit / 1_000_000_000)) ago — \(r.lastCommitMessage)")
                                    .sirsiFont(.caption2).foregroundStyle(.secondary).lineLimit(2)
                            }
                        }
                    } header: { Text("RISK: \(r.risk.uppercased())") }
                    Section {
                        LabeledContent("Repository") { Text(r.repoRoot).sirsiFont(.caption2, design: .monospaced).lineLimit(1).truncationMode(.middle) }
                        LabeledContent("Branch") { Text(r.branch).sirsiFont(.caption2, design: .monospaced) }
                        LabeledContent("Modified / untracked") { Text("\(r.modifiedFiles) / \(r.untrackedFiles)").sirsiFont(.caption2, design: .monospaced) }
                        LabeledContent("Line churn") { Text("+\(r.linesAdded) −\(r.linesDeleted)").sirsiFont(.caption2, design: .monospaced) }
                    } header: { Text("DETAILS") }
                    if let w = r.warning, !w.isEmpty {
                        Section { Text(w).sirsiFont(.caption).foregroundStyle(.secondary) } header: { Text("NOTE") }
                    }
                    if let line = actionResult {
                        Section {
                            HStack(alignment: .top, spacing: 8) {
                                Image(systemName: line.contains("Checkpointed") ? "checkmark.seal.fill" : "info.circle.fill")
                                    .foregroundStyle(line.contains("Checkpointed") ? .green : .secondary)
                                Text(line).sirsiFont(.caption).fixedSize(horizontal: false, vertical: true)
                            }
                        } header: { Text("RESULT") }
                    }
                    Section {
                        if r.uncommittedFiles == 0 {
                            // Resolved state — celebrate it, never a greyed dead-end.
                            HStack(spacing: 8) {
                                Image(systemName: "checkmark.circle.fill").foregroundStyle(.green)
                                Text("All work is checkpointed — nothing at risk right now.")
                                    .sirsiFont(.caption).fixedSize(horizontal: false, vertical: true)
                            }
                        } else {
                            Button {
                                Task { await checkpoint() }
                            } label: {
                                HStack {
                                    if checkpointing { ProgressView().controlSize(.small) }
                                    VStack(alignment: .leading, spacing: 1) {
                                        Text("Checkpoint now").sirsiFont(.caption, weight: .semibold)
                                        Text("Commit all changes locally — nothing is pushed, undo with git reset.")
                                            .sirsiFont(.caption2).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                }.contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                            .disabled(checkpointing)
                        }
                    } header: { Text("WHAT YOU CAN DO") }
                }
                .listStyle(.inset)
            } else {
                ScrollView {
                    let msg = (rawFallback?.isEmpty == false) ? rawFallback! : "Couldn't read checkpoint risk."
                    Text(msg)
                        .sirsiFont(.caption, design: .monospaced)
                        .frame(maxWidth: .infinity, alignment: .leading).padding(12)
                }
            }
            Divider()
            HStack {
                Button { Task { await load() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }.disabled(loading || checkpointing)
                if loading || checkpointing { ProgressView().controlSize(.small).padding(.leading, 4) }
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
        }
        .navigationTitle("Osiris — Checkpoints")
        .task { await load() }
    }

    private func riskGlyph(_ risk: String) -> String {
        switch risk.lowercased() {
        case "none", "clean", "": return "🟢" // resolved — never a red alarm
        case "low": return "🟢"
        case "medium", "moderate": return "🟡"
        case "high", "critical": return "🔴"
        default: return "🟢" // unknown/clean states must not fabricate an alarm
        }
    }

    // checkpoint pulls the lever (sirsi osiris checkpoint), reports the result
    // line, and re-measures — the screen RESOLVES what it found.
    private func checkpoint() async {
        checkpointing = true
        let data = await SirsiEngine.runJSON(args: ["osiris", "checkpoint", "--json"])
        let out = String(data: data, encoding: .utf8) ?? ""
        actionResult = SirsiEngine.summaryLine(out)
        engine.recordActivity(title: "Checkpoint commit", command: "osiris checkpoint", result: out)
        checkpointing = false
        await load()
    }

    private func load() async {
        loading = true
        report = nil
        rawFallback = nil
        let data = await SirsiEngine.runJSON(args: ["risk", "--json"])
        let text = String(data: data, encoding: .utf8) ?? ""
        if let i = text.firstIndex(of: "{"),
           let r = try? JSONDecoder().decode(RiskReport.self, from: Data(String(text[i...]).utf8)) {
            report = r
            loading = false
            return
        }
        // No JSON on stdout — risk prints its guidance ("needs a git
        // repository…") to STDERR, which runJSON drops. run() captures both, so
        // the screen shows the real message instead of a blank void (the bug:
        // an empty stdout became Text("") — transparent nothing, 2026-07-09).
        let combined = await SirsiEngine.run(args: ["risk"], stdin: nil)
        let cleaned = CommandView.stripBanner(combined).trimmingCharacters(in: .whitespacesAndNewlines)
        rawFallback = cleaned.isEmpty ? "Couldn't read checkpoint risk here. Set a project in a repo folder, or run `sirsi risk` in a terminal." : cleaned
        loading = false
    }
}

// Ghost report contract — mirrors cmd/sirsi/anubis.go `ghostReport`, the shape
// `sirsi ghosts --json` actually emits (pinned by ghosts_json_test.go). The
// previous view decoded CommandResult, a contract ghosts NEVER emitted — so
// every scan "failed" with a decode nil, and the owner saw "Couldn't scan"
// over a scan that had succeeded (2026-07-05 popover).
struct GhostReport: Decodable {
    let summary: String
    let ghostCount: Int
    let totalWaste: String
    let ghosts: [GhostApp]
    enum CodingKeys: String, CodingKey {
        case summary, ghosts
        case ghostCount = "ghost_count"
        case totalWaste = "total_waste"
    }
}

struct GhostApp: Decodable, Identifiable {
    var id: String { bundleID.isEmpty ? appName : bundleID }
    let appName: String
    let bundleID: String
    let totalSizeBytes: Int64
    let totalFiles: Int
    let residuals: [GhostResidual]
    enum CodingKeys: String, CodingKey {
        case residuals
        case appName = "app_name"
        case bundleID = "bundle_id"
        case totalSizeBytes = "total_size_bytes"
        case totalFiles = "total_files"
    }
}

struct GhostResidual: Decodable, Identifiable {
    var id: String { path }
    let path: String
    let type: String
    let sizeBytes: Int64
    enum CodingKeys: String, CodingKey {
        case path, type
        case sizeBytes = "size_bytes"
    }
}

// Renders `sirsi ghosts --json`: the summary, one section per leftover app
// (name, reclaimable size, residual count) with its leftover paths inline.
struct GhostsView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var report: GhostReport?
    @State private var loading = true
    @State private var cleaning = false
    @State private var confirmClean = false
    @State private var toast: String?
    @State private var toastOK = true

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Leftover Apps")
            if loading {
                VStack(spacing: 12) {
                    ProgressView()
                    Text("Scanning for remnants of uninstalled apps…")
                        .sirsiFont(.callout).foregroundStyle(.secondary)
                    Text("This can take a minute.").sirsiFont(.caption2).foregroundStyle(.tertiary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top).padding(.top, 60)
            } else if let r = report {
                MaybeList {
                    Section { Text(r.summary).sirsiFont(.callout) } header: { Text("RESULT") }
                    if r.ghosts.isEmpty {
                        Section {
                            Text("Nothing left behind — every uninstalled app cleaned up after itself.")
                                .sirsiFont(.caption).foregroundStyle(.secondary)
                        }
                    } else {
                        ForEach(r.ghosts) { g in
                            Section {
                                ForEach(g.residuals) { res in
                                    HStack {
                                        Text(res.path).sirsiFont(.caption2, design: .monospaced)
                                            .foregroundStyle(.secondary).lineLimit(1)
                                            .truncationMode(.middle)
                                        Spacer(minLength: 8)
                                        Text(SirsiEngine.human(res.sizeBytes))
                                            .sirsiFont(.caption2, design: .monospaced).foregroundStyle(.tertiary)
                                    }
                                }
                            } header: {
                                HStack {
                                    Text(g.appName)
                                    Spacer()
                                    Text("\(g.totalFiles) file\(g.totalFiles == 1 ? "" : "s") · \(SirsiEngine.human(g.totalSizeBytes))")
                                        .sirsiFont(.caption2)
                                }
                            }
                        }
                    }
                }
                .listStyle(.inset)
            } else {
                VStack(spacing: 8) {
                    Text("Couldn't scan for leftover apps.").foregroundStyle(.secondary)
                    Button("Try again") { Task { await load() } }.sirsiFont(.caption)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top).padding(.top, 60)
            }
            if let toast {
                Divider()
                HStack(spacing: 6) {
                    Image(systemName: toastOK ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                        .foregroundStyle(toastOK ? .green : .orange)
                    Text(toast).sirsiFont(.caption); Spacer()
                }.padding(.horizontal, 14).padding(.vertical, 8)
            }
            // The lever the audit asked for: a SAFE clean (Go `ghosts clean` —
            // dry-run/trash-first/protected-aware). Only shown when there's
            // something to reclaim; confirms first (trash is recoverable).
            if let r = report, !r.ghosts.isEmpty {
                Divider()
                HStack {
                    Button { confirmClean = true } label: {
                        Label("Move remnants to Trash", systemImage: "trash")
                    }.disabled(cleaning)
                    if cleaning { ProgressView().controlSize(.small) }
                    Spacer()
                    Text("recoverable").sirsiFont(.caption2).foregroundStyle(.tertiary)
                }.padding(.horizontal, 14).padding(.vertical, 10)
            }
        }
        .navigationTitle("Leftover Apps")
        .task { await load() }
        .confirmationDialog("Move ghost remnants to Trash?",
                            isPresented: $confirmClean, titleVisibility: .visible) {
            Button("Move to Trash", role: .destructive) { Task { await clean() } }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Residuals of uninstalled apps move to the Trash — recoverable until you empty it. Protected system paths and items needing admin rights are left alone.")
        }
    }

    private func clean() async {
        cleaning = true
        let out = String(data: await SirsiEngine.runJSON(args: ["ghosts", "clean", "--confirm", "--json"]), encoding: .utf8) ?? ""
        toastOK = SirsiEngine.resultOK(out)
        toast = SirsiEngine.summaryLine(out)
        engine.recordActivity(title: "Clean ghost remnants", command: "ghosts clean --confirm", result: toast ?? "")
        cleaning = false
        await load()
    }

    private func load() async {
        loading = true
        let data = await SirsiEngine.runJSON(args: ["ghosts", "--json"])
        if let s = String(data: data, encoding: .utf8), let i = s.firstIndex(of: "{") {
            report = try? JSONDecoder().decode(GhostReport.self, from: Data(String(s[i...]).utf8))
        }
        loading = false
    }
}

// ── CommandView — reusable inline deity surface ──────────────────────────────
//
// Shells `sirsi <args>` and renders the result INLINE in the popover (scrollable,
// monospaced, refreshable) — no kick-out to Terminal, native back button. This is
// how every remaining deity (Ma'at, Thoth, …) becomes a real, no-dead-end row
// without a bespoke parser each: Go owns the logic, the surface just renders it.
struct CommandView: View {
    let title: String
    let args: [String]
    @State private var output = ""
    @State private var loading = true

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: title)
            ScrollView {
                if loading {
                    HStack { Spacer(); ProgressView(); Spacer() }.padding(.top, 60)
                } else {
                    Text(output.isEmpty ? "No output." : output)
                        .sirsiFont(11.5, design: .monospaced)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(14)
                }
            }
            Divider()
            HStack {
                Button { Task { await load() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }.disabled(loading)
                if loading { ProgressView().controlSize(.small).padding(.leading, 4) }
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
        }
        .navigationTitle(title)
        .task { await load() }
    }

    private func load() async {
        loading = true
        let raw = await SirsiEngine.run(args: args, stdin: nil)
        output = Self.stripBanner(raw)
        loading = false
    }

    // stripBanner drops the PANTHEON splash + box-drawing lines the CLI prints, so
    // the popover shows just the deity's actual content.
    static func stripBanner(_ s: String) -> String {
        let drop = ["P A N T H E O N", "One Install", "Infrastructure Hygiene", "───", "═══"]
        let lines = s.split(separator: "\n", omittingEmptySubsequences: false).filter { line in
            let t = line.trimmingCharacters(in: .whitespaces)
            return !drop.contains { t.contains($0) }
        }
        return lines.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

// ── ProjectBar — which project a repo-scoped deity is weighing ────────────────
// Ma'at and Net measure a code project, but the app runs `sirsi` from the home
// folder, where they honestly say "unmeasured." This bar names the project being
// weighed and offers an in-popover picker (git projects one level under
// ~/Development). A modal file dialog would close the transient popover, so the
// picker is a Menu. "None" returns to the honest unmeasured default. The same
// setting is scriptable:
//   defaults write ai.sirsi.pantheon projectRoot -string ~/Development/<repo>
struct ProjectBar: View {
    @ObservedObject var engine: SirsiEngine
    var onChange: () -> Void   // re-runs the command after the project changes
    @State private var candidates: [String] = []

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "shippingbox")
                .sirsiFont(13).foregroundStyle(gold)
            VStack(alignment: .leading, spacing: 1) {
                if let name = engine.projectName {
                    Text("Weighing \(name)").sirsiFont(.caption, weight: .semibold)
                    Text(abbreviatedRoot).sirsiFont(.caption2).foregroundStyle(.secondary)
                } else {
                    Text("No project selected").sirsiFont(.caption, weight: .semibold)
                    Text("Pick a project to see its real score.")
                        .sirsiFont(.caption2).foregroundStyle(.secondary)
                }
            }
            Spacer()
            Menu {
                ForEach(candidates, id: \.self) { path in
                    Button {
                        engine.setProjectRoot(path)
                        onChange()
                    } label: {
                        let name = (path as NSString).lastPathComponent
                        if path == engine.projectRoot {
                            Label(name, systemImage: "checkmark")
                        } else {
                            Text(name)
                        }
                    }
                }
                if engine.projectRoot != nil {
                    Divider()
                    Button("None — stop weighing a project") {
                        engine.setProjectRoot(nil)
                        onChange()
                    }
                }
            } label: {
                Text(engine.projectRoot == nil ? "Choose…" : "Change…")
                    .sirsiFont(.caption)
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
        .padding(.horizontal, 12).padding(.vertical, 7)
        .background(Color.primary.opacity(0.03))
        .task {
            engine.loadProjectRoot()
            candidates = SirsiEngine.discoverProjectRoots()
            // Keep a valid root configured outside ~/Development choosable too.
            if let root = engine.projectRoot, !candidates.contains(root) {
                candidates.insert(root, at: 0)
            }
        }
        Divider()
    }

    private var abbreviatedRoot: String {
        guard let root = engine.projectRoot else { return "" }
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        return root.hasPrefix(home) ? "~" + root.dropFirst(home.count) : root
    }
}

// ── ResultView — the unified deity / action screen ───────────────────────────
// Runs a sirsi command. When the command emits the structured CommandResult, it
// renders summary + evidence + one-click next-action buttons — the `--confirm`
// applies confirm first and write to the provenance ledger. Commands without
// that shape fall back to clean text. Every path is navigable; nothing dead-ends.
struct ResultView: View {
    @ObservedObject var engine: SirsiEngine
    let title: String
    let args: [String]
    // When set, re-run diagnose after the fix ACTUALLY applies and report the real
    // post-fix status of this finding — the proof that kills "it says it's fixing
    // but the status stays the same." nil = a generic command run, no re-verify.
    var reverifyCheck: String? = nil
    var reverifyKind: String? = nil

    @State private var result: CommandResult?
    @State private var raw = ""
    @State private var loading = true
    @State private var applying = false
    @State private var pendingApply: CRAction?
    @State private var toast: String?
    @State private var toastOK = true      // did the toasted action succeed? drives icon/color
    @State private var postFix: String?   // honest verdict after re-verify
    @State private var didReverify = false // re-verify fires once (across load/apply paths)

    init(engine: SirsiEngine, title: String, args: [String],
         reverifyCheck: String? = nil, reverifyKind: String? = nil,
         preloaded: CommandResult? = nil) {
        self.engine = engine
        self.title = title
        self.args = args
        self.reverifyCheck = reverifyCheck
        self.reverifyKind = reverifyKind
        // Snapshot mode (Snapshot.swift) pre-fetches the CommandResult and
        // injects it: ImageRenderer draws the view synchronously and never runs
        // .task, so a self-loading view would render as an eternal spinner.
        _result = State(initialValue: preloaded)
        _loading = State(initialValue: preloaded == nil)
    }

    // Repo-scoped commands (maat, net) weigh a project — show WHICH one, and the
    // picker to change it. Workstation commands never show the bar.
    private var isRepoScoped: Bool {
        args.first.map { SirsiEngine.repoScopedVerbs.contains($0) } ?? false
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: title)
            if isRepoScoped {
                ProjectBar(engine: engine) { Task { await load() } }
            }
            Group {
                if loading && result == nil && raw.isEmpty {
                    VStack { Spacer(); ProgressView(); Spacer() }
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else if let r = result {
                    structuredScroll(r)
                } else {
                    rawScroll
                }
            }
            if applying {
                Divider()
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Working…").sirsiFont(.caption).foregroundStyle(.secondary)
                    Spacer()
                }.padding(.horizontal, 14).padding(.vertical, 8)
            }
            if let pf = postFix {
                Divider()
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: pf.hasPrefix("✓") ? "checkmark.seal.fill" : "info.circle.fill")
                        .foregroundStyle(pf.hasPrefix("✓") ? .green : .secondary).padding(.top, 1)
                    Text(pf).sirsiFont(.caption).foregroundStyle(.primary)
                        .fixedSize(horizontal: false, vertical: true)
                    Spacer(minLength: 0)
                }.padding(.horizontal, 14).padding(.vertical, 10)
                .background(Color.primary.opacity(0.03))
            }
            Divider()
            HStack {
                Button { Task { await load() } } label: { Label("Refresh", systemImage: "arrow.clockwise") }
                    .disabled(loading || applying)
                Spacer()
            }.padding(.horizontal, 14).padding(.vertical, 10)
        }
        .task { if result == nil && raw.isEmpty { await load() } }   // preloaded → already have it
        .confirmationDialog(
            "Apply this fix?",
            isPresented: Binding(get: { pendingApply != nil }, set: { if !$0 { pendingApply = nil } }),
            titleVisibility: .visible
        ) {
            Button("Apply", role: .destructive) {
                if let a = pendingApply { Task { await apply(a) } }
                pendingApply = nil
            }
            Button("Cancel", role: .cancel) { pendingApply = nil }
        } message: {
            if let a = pendingApply {
                Text("Runs `\(a.command)`.\nReversible — items move to Trash, and it's logged in Activity.")
            }
        }
    }

    // ImageRenderer (snapshot QA) draws ScrollView viewports empty, so snapshot
    // mode renders the same content in a plain stack. See Snapshot.swift.
    @Environment(\.snapshotMode) private var snapshotMode

    @ViewBuilder private func structuredScroll(_ r: CommandResult) -> some View {
        if snapshotMode {
            structuredBody(r).frame(maxHeight: .infinity, alignment: .top)
        } else {
            ScrollView { structuredBody(r) }
        }
    }

    @ViewBuilder private func structuredBody(_ r: CommandResult) -> some View {
            VStack(alignment: .leading, spacing: 14) {
                if let t = toast {
                    HStack(spacing: 6) {
                        Image(systemName: toastOK ? "checkmark.seal.fill" : "exclamationmark.triangle.fill")
                            .foregroundStyle(toastOK ? .green : .orange)
                        Text(t).sirsiFont(.caption)
                    }
                    .padding(8).frame(maxWidth: .infinity, alignment: .leading)
                    .background(RoundedRectangle(cornerRadius: 7).fill((toastOK ? Color.green : Color.orange).opacity(0.12)))
                }
                Text(r.summary).sirsiFont(14, weight: .semibold)
                    .fixedSize(horizontal: false, vertical: true)

                if !r.evidence.isEmpty {
                    VStack(spacing: 0) {
                        ForEach(r.evidence) { f in
                            // Full information, never a clipped tail (owner 2026-07-17:
                            // "drill-downs don't give enough information") — label and
                            // value WRAP to show everything, and both are selectable
                            // so evidence can be copied out of the popover.
                            HStack(alignment: .firstTextBaseline) {
                                Text(f.label).sirsiFont(.caption).foregroundStyle(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                                Spacer(minLength: 12)
                                Text(f.value).sirsiFont(.caption, design: .monospaced)
                                    .multilineTextAlignment(.trailing)
                                    .fixedSize(horizontal: false, vertical: true)
                                    .textSelection(.enabled)
                            }.padding(.vertical, 7)
                            if f.id != r.evidence.last?.id { Divider() }
                        }
                    }
                    .padding(.horizontal, 12)
                    .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                }

                if !r.nextActions.isEmpty {
                    Text("WHAT YOU CAN DO").sirsiFont(.caption2, weight: .semibold).foregroundStyle(.secondary)
                    VStack(spacing: 8) {
                        ForEach(r.nextActions) { actionButton($0) }
                    }
                }
            }.padding(16)
    }

    @ViewBuilder private func actionButton(_ a: CRAction) -> some View {
        if a.isApply {
            Button { pendingApply = a } label: { actionLabel(a, prominent: true) }
                .buttonStyle(.borderedProminent).tint(gold).disabled(applying)
        } else {
            Button { Task { await runFollow(a) } } label: { actionLabel(a, prominent: false) }
                .buttonStyle(.bordered).disabled(applying)
        }
    }

    @ViewBuilder private func actionLabel(_ a: CRAction, prominent: Bool) -> some View {
        HStack(spacing: 8) {
            Image(systemName: a.isApply ? "bolt.fill" : "arrow.right.circle")
            VStack(alignment: .leading, spacing: 1) {
                Text(a.label).sirsiFont(12, weight: .semibold)
                if let d = a.description, !d.isEmpty {
                    // Wrap, never clip mid-sentence — a half-shown consequence is
                    // worse than a taller button.
                    Text(d).sirsiFont(.caption2)
                        .foregroundStyle(prominent ? Color.white.opacity(0.85) : Color.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                        .multilineTextAlignment(.leading)
                }
            }
            Spacer()
        }.frame(maxWidth: .infinity).padding(.vertical, 2)
    }

    @ViewBuilder private var rawScroll: some View {
        if snapshotMode {
            rawBody.frame(maxHeight: .infinity, alignment: .top)
        } else {
            ScrollView { rawBody }
        }
    }

    private var rawBody: some View {
            Text(raw.isEmpty ? "No output." : raw)
                .sirsiFont(11.5, design: .monospaced)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(14)
    }

    private func load() async {
        loading = true
        if let r = await SirsiEngine.runResult(args: args) {
            result = r; raw = ""
        } else {
            let out = await SirsiEngine.run(args: args, stdin: nil)
            raw = CommandView.stripBanner(out); result = nil
        }
        loading = false
        // If the command completed in one step (no Apply to follow — e.g. a relief
        // or guidance command, not a clean preview), re-verify now. clean-family
        // results carry next_actions, so we wait for apply() to mutate first.
        if reverifyCheck != nil, (result?.nextActions.isEmpty ?? true) {
            await reverify()
        }
    }

    // reverify re-runs diagnose after a fix has actually run and reports the REAL
    // status of this finding — resolved, or an honest reason it persists (a 7-day
    // history cannot drop retroactively; guidance only bites a live issue).
    private func reverify() async {
        guard let check = reverifyCheck, !didReverify else { return }
        didReverify = true
        await engine.diagnose(force: true)
        let still = engine.health.first { $0.check == check }
        let now = Self.statusWord(engine.healthStatus)
        if still == nil || still!.severity <= 1 {
            postFix = "✓ Resolved — “\(check)” cleared. Overall health is now \(now)."
        } else {
            switch reverifyKind {
            case "relief":
                postFix = "Relief applied. “\(check)” is a 7-day history — it decays as clean days pass, so it won't clear the instant you click. Overall health: \(now)."
            case "guidance":
                postFix = "“\(check)” only clears when it's happening live. Nothing to undo this moment — the guidance above prevents it recurring. Overall health: \(now)."
            default:
                postFix = "Ran. “\(check)” is still present — overall health: \(now)."
            }
        }
    }

    private static func statusWord(_ s: String) -> String {
        switch s {
        case "red": return "🔴 red"
        case "amber": return "🟡 amber"
        case "green": return "🟢 green"
        default: return s
        }
    }

    // apply runs a --confirm action, feeding the CLI's [y/N] prompt, then RE-SCANS
    // so every surface (this screen, Refresh, the menubar title) reflects the new
    // reality — `clean` does not re-persist findings on its own. The toast and the
    // provenance ledger report what ACTUALLY happened, never an unconditional
    // "Applied" (the old lie when the clean had silently canceled).
    private func apply(_ a: CRAction) async {
        applying = true
        let out = await SirsiEngine.run(args: sirsiArgs(a.command), stdin: "y\n")
        // Diagnostic: the exact apply outcome goes to the unified log so a failed
        // apply (FDA, cancel, 0-cleaned) is diagnosable, not guessed. Public
        // privacy — this is local CLI output, no PII.
        applyLog.notice("apply [\(a.command, privacy: .public)] → \(out, privacy: .public)")
        let lc = out.lowercased()
        let canceled = lc.contains("cancel")
        let didApply = !canceled && (lc.contains("cleaned") || lc.contains("reclaimed")
                                     || lc.contains("healed") || lc.contains("applied"))
        let headline = Self.applyHeadline(out)
        engine.recordActivity(title: "\(title) — \(a.label)", command: a.command,
                              result: canceled ? "Canceled" : headline)
        // Refresh findings so stale pre-apply numbers and the tray title update.
        _ = await SirsiEngine.run(args: ["scan"], stdin: nil)
        toast = canceled ? "Canceled — nothing changed"
                         : (didApply ? "Done — \(headline)" : "Ran \(a.label)")
        applying = false
        await load()
        engine.refresh()
        // The mutation just landed — re-verify the finding's REAL status so the
        // user sees what actually changed, not an unconditional "fixed."
        if reverifyCheck != nil { await reverify() }
    }

    // applyHeadline pulls the human result line ("Cleaned 8 items. Reclaimed
    // 2.9 GB.") out of the CLI output for the toast + ledger.
    static func applyHeadline(_ out: String) -> String {
        for line in out.split(separator: "\n") {
            let t = line.trimmingCharacters(in: .whitespaces).lowercased()
            if t.contains("cleaned") || t.contains("reclaimed") || t.contains("healed") {
                return line.trimmingCharacters(in: .whitespaces)
            }
        }
        return "applied"
    }

    // runFollow runs a non-destructive next action (e.g. scan) and reloads.
    private func runFollow(_ a: CRAction) async {
        applying = true
        // The follow-up command's OUTPUT is the whole point — discarding it and
        // silently reloading made every "WHAT YOU CAN DO" button read as dead
        // (owner, 2026-07-09: "NONE are actually interactive"). Surface the
        // result summary as a toast, record it in the activity ledger, THEN
        // re-measure so the score reflects what just ran.
        // Run with --json so the toast shows the verb's real SUMMARY line, not
        // its styled banner/header (which firstMeaningful would otherwise grab).
        let jsonData = await SirsiEngine.runJSON(args: sirsiArgs(a.command) + ["--json"])
        let out = String(data: jsonData, encoding: .utf8) ?? ""
        engine.recordActivity(title: a.label, command: a.command, result: out)
        let summary = SirsiEngine.summaryLine(out)
        toastOK = SirsiEngine.resultOK(out)
        toast = summary.isEmpty ? (toastOK ? "\(a.label): done." : "\(a.label): failed.") : summary
        applying = false
        await load()
        engine.refresh()
    }
}

// ── Threads — the ambient CTR live-thread board + heartbeat ──────────────────
// Owner directive 20260709-182003: make the `sirsi thread list` CTR board an
// always-visible passive surface with a heartbeat graphic — see liveness WITHOUT
// running a query. One row per agent (the raw list is ~72 threads, mostly
// claude-home CCD sessions); each row shows a live/idle/stale roll-up, the
// freshest last-seen (the heartbeat), and a pulse bar. Surfaces-current+actionable:
// ⚠️ only for a genuinely stale agent; live data never greyed; plain English.
// ── Ask Sirsi — Local AI (board 1.2.0 local_llm feed) ────────────────────────
// Owner directive 2026-07-23: the on-device model's state must be visible
// inside Pantheon, and the owner must be able to query it from here. Copy is
// brand-level ("Ask Sirsi" / "Local AI") — model identifiers appear only as
// the small technical footnote. Offline is an honest message, never a greyed
// stale panel (live-data-never-greyed).
struct AskSirsiView: View {
    @ObservedObject var engine: SirsiEngine
    @Environment(\.snapshotMode) private var snapshotMode
    @State private var question = ""
    @State private var asking = false
    @State private var answer: String?
    private var panelFill: Color {
        snapshotMode ? Color(red: 0.13, green: 0.13, blue: 0.13) : Color.primary.opacity(0.04)
    }
    private var deepPanelFill: Color {
        snapshotMode ? Color(red: 0.095, green: 0.095, blue: 0.10) : Color.primary.opacity(0.065)
    }
    private var online: Bool { engine.localLLM?.healthy == true }
    private var liveStatus: String {
        if let llm = engine.localLLM {
            let memory = llm.rssMB.map { " · \(SirsiEngine.human(Int64($0) * 1_048_576))" } ?? ""
            let uptime = llm.uptime.map { " · up \($0)" } ?? ""
            return online ? "ONLINE ON THIS MAC\(memory)\(uptime)" : "OFFLINE - supervised restart path armed"
        }
        return "READING LOCAL CONDUIT"
    }
    private var managerTiles: [ManagerTileSpec] {
        let canon = engine.askSirsiCanonGroundingStatus()
        return [
            ManagerTileSpec(symbol: "point.3.connected.trianglepath.dotted",
                            title: "Router Fabric",
                            value: "\(engine.threadsTotal) live threads",
                            detail: engine.routerSummary,
                            tint: statusColor(engine.routerStatus)),
            ManagerTileSpec(symbol: "cpu",
                            title: "Compute",
                            value: engine.vitals.map { SirsiEngine.human($0.freeBytes) + " free" } ?? "sampling node",
                            detail: engine.vitals.map { "pressure \($0.pressure)" } ?? "ANE/MLX/Metal/CPU lanes",
                            tint: engine.vitals?.pressure == "critical" ? .red : (engine.vitals?.pressure == "warn" ? .orange : .green)),
            ManagerTileSpec(symbol: "books.vertical",
                            title: "Knowledge",
                            value: canon.value,
                            detail: canon.detail,
                            tint: canon.healthy ? .green : .orange),
            ManagerTileSpec(symbol: "lock.shield",
                            title: "Authority",
                            value: engine.ownerGatedItems.isEmpty ? "action-gated" : "\(engine.ownerGatedItems.count) owner items",
                            detail: "explains, routes, and keeps destructive work governed",
                            tint: engine.ownerGatedItems.isEmpty ? .green : .yellow),
        ]
    }

    init(engine: SirsiEngine, preloadedAnswer: String? = nil) {
        self.engine = engine
        _answer = State(initialValue: preloadedAnswer)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            BackBar(title: "Ask Sirsi")

            if snapshotMode {
                managerContent
                    .frame(maxHeight: .infinity, alignment: .top)
            } else {
                ScrollView { managerContent }
            }

            Spacer(minLength: 8)
        }
        .task { await engine.loadRouterBoard() }
        .navigationTitle("Ask Sirsi — Local AI")
    }

    private func ask() {
        let q = question.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty, !asking else { return }
        asking = true
        Task {
            answer = await engine.askLocalAI(q)
            asking = false
        }
    }

    private func askKnowledgeReport() {
        guard !asking else { return }
        asking = true
        Task {
            answer = await engine.askLocalAIKnowledgeReport()
            asking = false
        }
    }

    private var managerContent: some View {
        VStack(alignment: .leading, spacing: 12) {
            VStack(alignment: .leading, spacing: 10) {
                HStack(alignment: .top, spacing: 12) {
                    ZStack {
                        RoundedRectangle(cornerRadius: 8)
                            .fill(gold.opacity(0.18))
                            .frame(width: 44, height: 44)
                        Image(systemName: "terminal.fill")
                            .sirsiFont(20, weight: .bold)
                            .foregroundStyle(gold)
                    }
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Internal System Manager")
                            .sirsiFont(17, weight: .bold)
                            .foregroundStyle(.primary)
                        Text("Ask Sirsi knows Pantheon, the router, Hypergraph, Sirsi IO, portfolio apps, and this Mac's local operating state.")
                            .sirsiFont(11, weight: .medium)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Spacer(minLength: 8)
                }

                HStack(spacing: 7) {
                    ManagerPill(text: liveStatus, tint: online ? .green : .yellow)
                    ManagerPill(text: "LOCAL ONLY", tint: gold)
                }
            }
            .padding(14)
            .background(
                RoundedRectangle(cornerRadius: 8)
                    .fill(deepPanelFill)
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(gold.opacity(0.38), lineWidth: 1))
            )

            LazyVGrid(columns: [GridItem(.adaptive(minimum: 152), spacing: 8)], spacing: 8) {
                ForEach(managerTiles) { tile in
                    ManagerTile(tile: tile, fill: panelFill)
                }
            }

            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 6) {
                    Image(systemName: "sparkles").foregroundStyle(gold)
                    Text("Operator Query")
                        .sirsiFont(12, weight: .bold)
                        .foregroundStyle(.secondary)
                    Spacer()
                    if asking { ProgressView().controlSize(.small) }
                }

                if snapshotMode {
                    HStack(spacing: 6) {
                        Text("Ask Sirsi about router work, local health, or what changed.")
                            .sirsiFont(13)
                            .foregroundStyle(.secondary)
                        Spacer()
                        Image(systemName: "arrow.up.circle.fill").foregroundStyle(gold)
                    }
                    .padding(10)
                    .background(RoundedRectangle(cornerRadius: 8).fill(panelFill))
                } else {
                    HStack(spacing: 8) {
                        TextField("Ask about Sirsi, Pantheon, router work, local health, or what changed.", text: $question)
                            .textFieldStyle(.plain)
                            .sirsiFont(13)
                            .onSubmit { ask() }
                        Button { ask() } label: { Image(systemName: "arrow.up.circle.fill").sirsiFont(20) }
                            .buttonStyle(.plain)
                            .foregroundStyle(gold)
                            .disabled(question.trimmingCharacters(in: .whitespaces).isEmpty || !online || asking)
                    }
                    .padding(10)
                    .background(RoundedRectangle(cornerRadius: 8).fill(panelFill))
                }

                if snapshotMode {
                    Label("Report what Sirsi taught you", systemImage: "book.closed")
                        .frame(maxWidth: .infinity)
                        .sirsiFont(12, weight: .semibold)
                        .foregroundStyle(gold)
                        .padding(.vertical, 6)
                        .background(RoundedRectangle(cornerRadius: 8).fill(panelFill))
                } else {
                    Button { askKnowledgeReport() } label: {
                        Label("Report what Sirsi taught you", systemImage: "book.closed")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderless)
                    .sirsiFont(12, weight: .semibold)
                    .foregroundStyle(gold)
                    .padding(.vertical, 6)
                    .background(RoundedRectangle(cornerRadius: 8).fill(panelFill))
                    .disabled(asking || !online)
                }
            }
            .padding(12)
            .background(RoundedRectangle(cornerRadius: 8).fill(deepPanelFill))

            if let answer {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Sirsi Response")
                        .sirsiFont(12, weight: .bold)
                        .foregroundStyle(.secondary)
                    Text(answer)
                        .sirsiFont(13)
                        .lineLimit(snapshotMode ? 7 : nil)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                    Text("answered on-device by Sirsi - no cloud")
                        .sirsiFont(.caption2)
                        .foregroundStyle(.tertiary)
                }
                .padding(12)
                .background(RoundedRectangle(cornerRadius: 8).fill(panelFill))
            }
        }
        .padding(.horizontal, 12)
        .padding(.top, 8)
        .padding(.bottom, 12)
    }
}

struct ManagerTileSpec: Identifiable {
    var id: String { title }
    let symbol: String
    let title: String
    let value: String
    let detail: String
    let tint: Color
}

struct ManagerPill: View {
    let text: String
    let tint: Color

    var body: some View {
        HStack(spacing: 5) {
            Circle().fill(tint).frame(width: 6, height: 6)
            Text(text)
                .sirsiFont(9, weight: .bold)
                .lineLimit(1)
                .minimumScaleFactor(0.78)
        }
        .foregroundStyle(.primary)
        .padding(.horizontal, 8)
        .padding(.vertical, 5)
        .background(
            RoundedRectangle(cornerRadius: 7)
                .fill(tint.opacity(0.12))
                .overlay(RoundedRectangle(cornerRadius: 7).stroke(tint.opacity(0.35), lineWidth: 1))
        )
    }
}

struct ManagerTile: View {
    let tile: ManagerTileSpec
    let fill: Color

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 7) {
                Image(systemName: tile.symbol)
                    .sirsiFont(13, weight: .bold)
                    .foregroundStyle(tile.tint)
                    .sirsiFrame(width: 18)
                Text(tile.title.uppercased())
                    .sirsiFont(9, weight: .bold)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Spacer(minLength: 4)
            }
            Text(tile.value)
                .sirsiFont(13, weight: .bold)
                .foregroundStyle(.primary)
                .lineLimit(1)
                .minimumScaleFactor(0.75)
            Text(tile.detail)
                .sirsiFont(11, weight: .medium)
                .foregroundStyle(.secondary)
                .lineLimit(2)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity, minHeight: 86, alignment: .topLeading)
        .padding(10)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(fill)
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(tile.tint.opacity(0.26), lineWidth: 1))
        )
    }
}

struct ThreadsView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var question = ""
    @State private var answer: String?
    @State private var asking = false

    private func ago(_ s: Double) -> String {
        if s < 60 { return "\(Int(s))s ago" }
        if s < 3600 { return "\(Int(s / 60))m ago" }
        return "\(Int(s / 3600))h ago"
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Threads — Heartbeat")
            HStack(spacing: 6) {
                Text("\(engine.threadsTotal)").sirsiFont(22, weight: .bold).foregroundStyle(.green)
                VStack(alignment: .leading, spacing: 0) {
                    Text("live thread\(engine.threadsTotal == 1 ? "" : "s")").sirsiFont(.caption).foregroundStyle(.primary)
                    Text("\(engine.threadRoster.count) agent\(engine.threadRoster.count == 1 ? "" : "s") on the fabric").sirsiFont(.caption2).foregroundStyle(.secondary)
                }
                Spacer()
            }.padding(.horizontal, 16).padding(.vertical, 10)
            Divider()
            if engine.threadRoster.isEmpty {
                VStack(spacing: 8) {
                    if engine.threadsLoading { ProgressView() }
                    Text(engine.threadsLoading ? "Reading the fabric…" : "No live threads right now.")
                        .sirsiFont(.callout).foregroundStyle(.secondary)
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(28)
            } else {
                ScrollView {
                    VStack(spacing: 0) {
                        ForEach(engine.threadRoster) { a in
                            HStack(spacing: 10) {
                                Text(a.glyph).sirsiFont(15)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(a.agent).sirsiFont(12, weight: .semibold)
                                    Text(rollup(a)).sirsiFont(.caption2).foregroundStyle(.secondary)
                                }
                                Spacer()
                                VStack(alignment: .trailing, spacing: 3) {
                                    Text(ago(a.freshestIdle)).sirsiFont(.caption2, design: .monospaced).foregroundStyle(.secondary)
                                    PulseBar(pulse: a.pulse, stale: a.isStale)
                                }
                            }
                            .padding(.horizontal, 14).padding(.vertical, 9)
                            if a.id != engine.threadRoster.last?.id { Divider().padding(.leading, 40) }
                        }
                    }
                }
            }
            // Ask Sirsi in plain English — answered ON-DEVICE, never a cloud
            // model (owner directive: local-LLM every time). "Sirsi" is the
            // brand; the model under it (Gemma today) is a switchable fabric —
            // the user never sees the model name (owner, 2026-07-10).
            Divider()
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Image(systemName: "sparkles").sirsiFont(.caption).foregroundStyle(gold)
                    TextField("Ask about the fabric — e.g. \"what's stale?\"", text: $question)
                        .textFieldStyle(.plain).sirsiFont(12)
                        .onSubmit { ask() }
                    if asking {
                        ProgressView().controlSize(.small)
                    } else {
                        Button { ask() } label: { Image(systemName: "arrow.up.circle.fill") }
                            .buttonStyle(.plain).foregroundStyle(gold)
                            .disabled(question.trimmingCharacters(in: .whitespaces).isEmpty)
                    }
                }
                if let answer {
                    Text(answer).sirsiFont(.caption).foregroundStyle(.primary)
                        .fixedSize(horizontal: false, vertical: true)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    Text("answered on-device by Sirsi — no cloud").sirsiFont(.caption2).foregroundStyle(.tertiary)
                }
            }.padding(.horizontal, 14).padding(.vertical, 8)

            Divider()
            HStack {
                Button { Task { await engine.loadThreads() } } label: { Label("Refresh", systemImage: "arrow.clockwise") }
                    .disabled(engine.threadsLoading)
                Spacer()
                Text("updates every 60s").sirsiFont(.caption2).foregroundStyle(.tertiary)
            }.padding(.horizontal, 14).padding(.vertical, 10)
        }
        .task { await engine.loadThreads() }
    }

    private func ask() {
        let q = question.trimmingCharacters(in: .whitespaces)
        guard !q.isEmpty, !asking else { return }
        asking = true
        Task {
            let a = await engine.askAboutThreads(q)
            answer = a
            asking = false
        }
    }

    private func rollup(_ a: AgentHeartbeat) -> String {
        var parts: [String] = []
        if a.live > 0 { parts.append("\(a.live) live") }
        if a.idle > 0 { parts.append("\(a.idle) idle") }
        if a.staleN > 0 { parts.append("\(a.staleN) stale") }
        let counts = parts.joined(separator: " · ")
        let surf = a.surfaces.isEmpty ? "" : " — \(a.surfaces.joined(separator: "/"))"
        return counts + surf
    }
}

// PulseBar is the heartbeat graphic: a short bar that's full + green when the
// agent was just seen and shrinks/dims as it goes quiet; amber when stale.
struct PulseBar: View {
    let pulse: Double   // 1 = just seen → 0 = quiet (~10 min)
    let stale: Bool
    var body: some View {
        GeometryReader { geo in
            ZStack(alignment: .leading) {
                Capsule().fill(Color.primary.opacity(0.08))
                Capsule()
                    .fill(stale ? Color.orange : Color.green)
                    .frame(width: max(4, geo.size.width * CGFloat(stale ? 0.35 : pulse)))
                    .opacity(stale ? 0.7 : (0.4 + 0.6 * pulse))
            }
        }
        .frame(width: 48, height: 5)
    }
}

// ── ActivityView — the provenance ledger ─────────────────────────────────────
struct ActivityView: View {
    @ObservedObject var engine: SirsiEngine
    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Activity")
            if engine.activity.isEmpty {
                VStack(spacing: 8) {
                    Image(systemName: "clock.arrow.circlepath").sirsiFont(.title).foregroundStyle(.tertiary)
                    Text("No actions yet").sirsiFont(.callout).foregroundStyle(.secondary)
                    Text("Fixes you apply show here — what changed, when, and the command that ran. Everything is reversible.")
                        .sirsiFont(.caption2).foregroundStyle(.tertiary).multilineTextAlignment(.center)
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(28)
            } else {
                List(engine.activity) { e in
                    VStack(alignment: .leading, spacing: 2) {
                        HStack {
                            Text(e.title).sirsiFont(12, weight: .semibold)
                            Spacer()
                            Text(e.when).sirsiFont(.caption2).foregroundStyle(.tertiary)
                        }
                        Text(e.command).sirsiFont(.caption, design: .monospaced).foregroundStyle(gold)
                        if !e.result.isEmpty {
                            Text(e.result).sirsiFont(.caption2).foregroundStyle(.secondary).lineLimit(2)
                        }
                    }.padding(.vertical, 2)
                }.listStyle(.inset)
            }
        }.task { engine.loadActivity() }
    }
}

// ── Insight — the cross-deity "what to do next" (sirsi insight) ───────────────
//
// Renders `sirsi insight --json` inline: prioritized next actions (with the exact
// command), the per-deity platform signals, and — on demand — the optional local
// Gemma narration. Deterministic by default (instant); "Ask Gemma" adds the
// natural-language synthesis only if the local backend is installed (AI-optional).

struct InsightSignal: Decodable, Identifiable {
    let id = UUID()
    let deity: String
    let status: String
    let severity: Int
    enum CodingKeys: String, CodingKey { case deity, status, severity }
}

struct InsightAction: Decodable, Identifiable {
    let id = UUID()
    let title: String
    let why: String
    let command: String
    enum CodingKeys: String, CodingKey { case title, why, command }
}

struct InsightReport: Decodable {
    let signals: [InsightSignal]
    let actions: [InsightAction]
    let source: String
    let narrative: String?

    // Lenient decode: `sirsi insight --json` emits `"actions": null` (a nil Go
    // slice) whenever the platform is healthy — the common case. Swift's
    // synthesized Decodable treats null for a non-optional [T] as a hard error,
    // which surfaced as "Couldn't load insight." on a perfectly healthy Mac.
    // Treat null/missing arrays as empty, and default source, so a healthy
    // report always renders.
    enum CodingKeys: String, CodingKey { case signals, actions, source, narrative }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        signals = try c.decodeIfPresent([InsightSignal].self, forKey: .signals) ?? []
        actions = try c.decodeIfPresent([InsightAction].self, forKey: .actions) ?? []
        source = try c.decodeIfPresent(String.self, forKey: .source) ?? "rules"
        narrative = try c.decodeIfPresent(String.self, forKey: .narrative)
    }
}

struct InsightView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var report: InsightReport?
    @State private var loading = true
    @State private var askingGemma = false

    // Snapshot QA injection (same seam as ResultView): ImageRenderer never runs
    // .task, so without a preloaded report this screen renders an eternal
    // spinner and the harness can't judge it.
    init(engine: SirsiEngine, preloaded: InsightReport? = nil) {
        self.engine = engine
        _report = State(initialValue: preloaded)
        _loading = State(initialValue: preloaded == nil)
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Insight")
            if loading && report == nil {
                HStack { Spacer(); ProgressView(); Spacer() }.padding(.top, 60)
            } else if let r = report {
                MaybeList {
                    if let n = r.narrative, !n.isEmpty {
                        Section { Text(n).sirsiFont(.callout) } header: { Text("𓂀 LOCAL GEMMA") }
                    }
                    Section {
                        if r.actions.isEmpty {
                            Text("Everything healthy — nothing to do right now.")
                                .sirsiFont(.caption).foregroundStyle(.secondary)
                        }
                        ForEach(r.actions) { a in
                            // Each suggestion RUNS its command in-panel — tap pushes
                            // a CommandView that executes `sirsi <cmd>` and shows
                            // output (TUI-is-the-session: never a dead command label).
                            NavLink {
                                ResultView(engine: engine, title: a.title, args: Self.commandArgs(a.command))
                            } label: {
                                HStack(alignment: .top, spacing: 8) {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(a.title).sirsiFont(12, weight: .semibold)
                                        Text(a.why).sirsiFont(.caption2).foregroundStyle(.secondary)
                                            .fixedSize(horizontal: false, vertical: true)
                                        Text(a.command).sirsiFont(.caption, design: .monospaced).foregroundStyle(gold)
                                    }
                                    Spacer()
                                    Image(systemName: "play.circle.fill")
                                        .foregroundStyle(gold).sirsiFont(14)
                                }
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                            .padding(.vertical, 1)
                        }
                    } header: { Text("DO NEXT") }

                    Section {
                        ForEach(r.signals) { s in
                            // Platform rows drill into that deity's NATIVE view.
                            // Never a raw CLI run for the big three: `scan` from
                            // the app used to spin forever (full-disk walk) and
                            // `thoth status` was a jargon dump with wrong advice.
                            NavLink {
                                Self.deityDestination(engine: engine, deity: s.deity)
                            } label: {
                                HStack(spacing: 8) {
                                    Circle().fill(insightSeverityColor(min(s.severity, 2))).frame(width: 7, height: 7)
                                    Text(s.deity).sirsiFont(.caption)
                                    Spacer()
                                    Text(s.status).sirsiFont(.caption2).foregroundStyle(.secondary)
                                    Image(systemName: "chevron.right").sirsiFont(.caption2).foregroundStyle(.tertiary)
                                }
                                .contentShape(Rectangle())
                            }
                            .buttonStyle(.plain)
                        }
                    } header: { Text("PLATFORM") } footer: { Text("source: \(r.source)") }
                }
                .listStyle(.inset)
            } else {
                Text("Couldn't load insight.").foregroundStyle(.secondary).padding(40)
            }

            Divider()
            HStack {
                Button { Task { await load(ai: false) } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }.disabled(loading)
                Button { Task { await load(ai: true) } } label: {
                    Label("Ask Sirsi", systemImage: "sparkles")
                }.disabled(loading)
                if askingGemma { ProgressView().controlSize(.small) }
                Spacer()
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
        }
        .navigationTitle("Insight")
        .task { if report == nil { await load(ai: false) } }
    }

    private func load(ai: Bool) async {
        loading = true; askingGemma = ai
        var args = ["insight", "--json"]
        if !ai { args.append("--no-ai") }   // deterministic default = instant
        let raw = await SirsiEngine.run(args: args, stdin: nil)
        if let r = Self.decode(raw) { report = r }
        loading = false; askingGemma = false
    }

    static func decode(_ s: String) -> InsightReport? {
        guard let start = s.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(InsightReport.self, from: Data(String(s[start...]).utf8))
    }

    // commandArgs turns the action's exact command string ("sirsi self-update",
    // "sirsi clean") into argv for SirsiEngine.run, dropping the leading binary.
    // These are read/preview-safe (sirsi clean previews unless --confirm).
    static func commandArgs(_ command: String) -> [String] {
        var toks = command.split(whereSeparator: { $0 == " " }).map(String.init)
        if toks.first == "sirsi" { toks.removeFirst() }
        return toks.isEmpty ? ["status"] : toks
    }

    // deityDestination routes a PLATFORM signal to its NATIVE in-app view where
    // one exists — instant, actionable, no subprocess. Only deities without a
    // native view fall back to a fast read-only CLI render.
    @ViewBuilder
    static func deityDestination(engine: SirsiEngine, deity: String) -> some View {
        let d = deity.lowercased()
        if d.contains("anubis") {
            AnubisView(engine: engine)
        } else if d.contains("horus") {
            HorusView(engine: engine)
        } else if d.contains("thoth") {
            ThothMemoryInfoView(engine: engine)
        } else {
            ResultView(engine: engine, title: deity, args: Self.deityArgs(deity))
        }
    }

    // deityArgs maps the REMAINING deities to a fast, read-only command.
    static func deityArgs(_ deity: String) -> [String] {
        let d = deity.lowercased()
        if d.contains("ma") && d.contains("at") { return ["maat", "audit"] }
        if d.contains("ra") { return ["ra", "status"] }
        return ["status"]
    }
}

// ── Thoth (menubar) — plain-English explainer, not a CLI dump ─────────────────
//
// Thoth's memory lives inside each project folder; the menu bar app doesn't run
// inside a project, so a raw `thoth status` here said ".thoth/ not found — run
// sirsi thoth init", which is wrong advice for this surface (it would create
// /.thoth). Say what's true in plain English instead.
// ThothMemoryInfoView is project-aware (owner, 2026-07-10): like Ma'at/Net it
// weighs the ProjectBar-selected repo, showing that project's .thoth/memory.yaml
// — the compact "resume where the last session left off" state — with a Sync
// lever, instead of a generic "not in a project" dead-end.
struct ThothMemoryInfoView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var memory: String?
    @State private var lineCount = 0
    @State private var modified: String?
    @State private var busy = false
    @State private var toast: String?

    private var memoryPath: String? { engine.projectRoot.map { $0 + "/.thoth/memory.yaml" } }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Thoth — Memory")
            ProjectBar(engine: engine) { load() }
            Group {
                if engine.projectRoot == nil {
                    noProject
                } else if let mem = memory {
                    hasMemory(mem)
                } else {
                    noMemoryYet
                }
            }
            if let toast {
                Divider()
                HStack(spacing: 6) {
                    Image(systemName: "checkmark.seal.fill").foregroundStyle(.green)
                    Text(toast).sirsiFont(.caption); Spacer()
                }.padding(.horizontal, 14).padding(.vertical, 8)
            }
        }
        .task { load() }
        .navigationTitle("Thoth — Memory")
    }

    private var noProject: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Project memory lives with each project.").sirsiFont(13, weight: .semibold)
            Text("Thoth keeps a small memory file inside every project so a new session picks up where the last one left off — no re-reading the whole codebase.")
                .sirsiFont(.callout).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            Text("Pick a project above to see its memory.").sirsiFont(.callout).foregroundStyle(.secondary)
            Spacer()
        }.padding(16).frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    @ViewBuilder private func hasMemory(_ mem: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(spacing: 6) {
                Text("📖").sirsiFont(14)
                VStack(alignment: .leading, spacing: 1) {
                    Text("\(engine.projectName ?? "project") memory").sirsiFont(13, weight: .semibold)
                    Text("\(lineCount) lines\(modified.map { " · synced \($0)" } ?? "")").sirsiFont(.caption2).foregroundStyle(.secondary)
                }
                Spacer()
            }.padding(.horizontal, 14).padding(.vertical, 10)
            Divider()
            ScrollView {
                Text(mem).sirsiFont(.caption, design: .monospaced).foregroundStyle(.primary)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading).padding(12)
            }
            Divider()
            HStack {
                Button { runThoth(["thoth", "sync"], "Memory synced") } label: { Label("Sync memory", systemImage: "arrow.triangle.2.circlepath") }.disabled(busy)
                Button { reveal() } label: { Label("Reveal file", systemImage: "folder") }
                if busy { ProgressView().controlSize(.small) }
                Spacer()
            }.padding(.horizontal, 14).padding(.vertical, 10)
        }
    }

    private var noMemoryYet: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("No Thoth memory in \(engine.projectName ?? "this project") yet.").sirsiFont(13, weight: .semibold)
            Text("Initialize it so future sessions resume from a compact project state instead of re-reading everything.")
                .sirsiFont(.callout).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
            Button { runThoth(["thoth", "init"], "Memory initialized") } label: { Label("Initialize memory", systemImage: "sparkles") }.disabled(busy)
            if busy { ProgressView().controlSize(.small) }
            Spacer()
        }.padding(16).frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    private func load() {
        guard let path = memoryPath, let data = FileManager.default.contents(atPath: path),
              let s = String(data: data, encoding: .utf8) else { memory = nil; return }
        memory = s
        lineCount = s.split(separator: "\n", omittingEmptySubsequences: false).count
        if let attrs = try? FileManager.default.attributesOfItem(atPath: path),
           let d = attrs[.modificationDate] as? Date {
            let f = DateFormatter(); f.dateFormat = "MMM d, HH:mm"; modified = f.string(from: d)
        }
    }

    private func runThoth(_ args: [String], _ okMsg: String) {
        busy = true
        Task {
            let out = await SirsiEngine.run(args: args, stdin: nil)
            engine.recordActivity(title: okMsg, command: "sirsi " + args.joined(separator: " "), result: SirsiEngine.firstMeaningful(out))
            toast = okMsg
            busy = false
            load()
        }
    }

    private func reveal() {
        guard let path = memoryPath else { return }
        NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: path)])
    }
}

// ── Owner actions — items only the OWNER can move ────────────────────────────
// Open `to: user` router items (board owner_gated[], schema 1.1.0) get a toast
// (AppDelegate) and this pair of screens: a list, and a detail view with real
// levers — read the referenced docs, mark it handled, or reply to a decision.

struct OwnerActionsListView: View {
    @ObservedObject var engine: SirsiEngine
    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Needs you")
            if engine.ownerGatedItems.isEmpty {
                VStack(spacing: 8) {
                    Text("✓").sirsiFont(40).foregroundStyle(.green)
                    Text("Nothing is waiting on you.").sirsiFont(.callout).foregroundStyle(.secondary)
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(28)
            } else {
                MaybeList {
                    ForEach(engine.ownerGatedItems) { item in
                        NavLink { OwnerActionView(engine: engine, itemID: item.id) } label: {
                            HStack(alignment: .top, spacing: 10) {
                                Text(item.type == "decision" ? "❓" : "🔑").frame(width: 24)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(item.title).sirsiFont(12, weight: .semibold)
                                        .fixedSize(horizontal: false, vertical: true)
                                        .multilineTextAlignment(.leading)
                                    if let why = item.why, !why.isEmpty {
                                        Text(why).sirsiFont(.caption2).foregroundStyle(.secondary)
                                            .lineLimit(3).multilineTextAlignment(.leading)
                                    }
                                    Text("from \(item.from) · \(item.ageLabel)")
                                        .sirsiFont(.caption2).foregroundStyle(.tertiary)
                                }
                                Spacer()
                                Image(systemName: "chevron.right").sirsiFont(.caption2).foregroundStyle(.tertiary)
                            }
                            .contentShape(Rectangle())
                        }
                    }
                }
            }
        }
        .navigationTitle("Needs you")
        .task { await engine.loadRouterBoard() }
    }
}

struct OwnerActionView: View {
    @ObservedObject var engine: SirsiEngine
    let itemID: String
    var preloadedBody: String? = nil   // snapshot QA — ImageRenderer never runs .task
    @EnvironmentObject private var nav: Nav
    @Environment(\.snapshotMode) private var snapshotMode
    @State private var body_ = ""
    @State private var loading = true
    @State private var confirmClose = false
    @State private var decision = ""
    @State private var resultLine: String?

    private var meta: OwnerGated? { engine.ownerGatedItems.first { $0.id == itemID } }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Owner action")
            maybeScrollBody
            Divider()
            levers
        }
        .navigationTitle("Owner action")
        .task {
            if let pre = preloadedBody { body_ = pre; loading = false; return }
            body_ = await SirsiEngine.ownerItemBody(id: itemID)
            loading = false
        }
        .confirmationDialog("Mark this handled?", isPresented: $confirmClose, titleVisibility: .visible) {
            Button("Mark handled", role: .destructive) {
                Task { resultLine = await engine.closeOwnerItem(id: itemID, note: "") }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Closes the item in the router. Do this after you've actually done what it asks.")
        }
    }

    @ViewBuilder private var maybeScrollBody: some View {
        if snapshotMode { detail.frame(maxHeight: .infinity, alignment: .top) }
        else { ScrollView { detail } }
    }

    @ViewBuilder private var detail: some View {
        VStack(alignment: .leading, spacing: 12) {
            if let m = meta {
                Text(m.title).sirsiFont(14, weight: .semibold)
                    .fixedSize(horizontal: false, vertical: true)
                HStack(spacing: 8) {
                    Text(m.type).sirsiFont(.caption2, weight: .semibold)
                        .padding(.horizontal, 6).padding(.vertical, 2)
                        .background(Capsule().fill(m.type == "decision" ? Color.yellow.opacity(0.25) : Color.primary.opacity(0.08)))
                    Text("from \(m.from) · \(m.ageLabel)").sirsiFont(.caption2).foregroundStyle(.secondary)
                }
                if let refs = m.refs, !refs.isEmpty {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("REFERENCED FILES").sirsiFont(.caption2, weight: .semibold).foregroundStyle(.secondary)
                        ForEach(refs, id: \.self) { ref in
                            Button { openRef(ref) } label: {
                                Label(ref, systemImage: "doc.text").sirsiFont(.caption)
                            }.buttonStyle(.link)
                        }
                    }
                }
            }
            if let r = resultLine {
                HStack(spacing: 6) {
                    Image(systemName: "checkmark.seal.fill").foregroundStyle(.green)
                    Text(r).sirsiFont(.caption)
                }
                .padding(8).frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 7).fill(Color.green.opacity(0.12)))
            }
            if loading {
                HStack { Spacer(); ProgressView(); Spacer() }.padding(.top, 20)
            } else {
                Text(body_.isEmpty ? "No details recorded." : body_)
                    .sirsiFont(11, design: .monospaced)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }.padding(14)
    }

    @ViewBuilder private var levers: some View {
        VStack(spacing: 8) {
            if meta?.type == "decision" && resultLine == nil {
                HStack(spacing: 8) {
                    TextField("Your decision…", text: $decision)
                        .textFieldStyle(.roundedBorder).sirsiFont(.caption)
                    Button("Send") {
                        Task { resultLine = await engine.replyOwnerDecision(id: itemID, text: decision) }
                    }
                    .disabled(decision.trimmingCharacters(in: .whitespaces).isEmpty || engine.busy)
                }
            }
            HStack {
                if resultLine == nil {
                    Button { confirmClose = true } label: {
                        Label("Mark handled", systemImage: "checkmark.circle")
                    }.disabled(engine.busy)
                } else {
                    Button { nav.pop() } label: { Label("Done", systemImage: "chevron.left") }
                }
                if engine.busy { ProgressView().controlSize(.small).padding(.leading, 4) }
                Spacer()
            }
        }
        .padding(.horizontal, 14).padding(.vertical, 10)
    }

    // openRef reveals a repo-relative referenced path. Resolves against the
    // configured project root, then the canonical pantheon checkout, then $HOME.
    private func openRef(_ ref: String) {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        var roots = [home + "/Development/sirsi-pantheon", home]
        if let pr = engine.projectRoot { roots.insert(pr, at: 0) }
        for root in roots {
            let p = root + "/" + ref
            if FileManager.default.fileExists(atPath: p) {
                NSWorkspace.shared.activateFileViewerSelecting([URL(fileURLWithPath: p)])
                return
            }
        }
    }
}
