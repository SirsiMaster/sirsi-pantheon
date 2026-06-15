import SwiftUI
import Foundation
import AppKit

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

// registerForFullDiskAccess forces this app into the Full Disk Access list so
// there is actually a row to toggle. macOS never *prompts* for FDA: an app only
// appears in the list after it attempts a TCC-guarded `open()` and is denied —
// that denial is what registers it. Without this the FDA pane shows no
// "Sirsi Menubar" row at all, so the button looked like it pointed at nothing.
//
// We use the raw POSIX open(2) syscall, NOT Data(contentsOf:): Foundation tends
// to do an access(R_OK) preflight, take the TCC denial as EACCES, and bail
// before the real open() — but it is open() that TCC intercepts and registers.
// Fired both at launch (so the row exists before the user ever opens the pane)
// and on the button. The opens are expected to fail; failure is the point.
func registerForFullDiskAccess() {
    let home = FileManager.default.homeDirectoryForCurrentUser.path
    let protectedPaths = [
        home + "/Library/Application Support/com.apple.TCC/TCC.db",
        home + "/Library/Mail",
        home + "/Library/Messages/chat.db",
        home + "/Library/Safari/Bookmarks.plist",
    ]
    for path in protectedPaths {
        let fd = open(path, O_RDONLY)   // TCC intercepts; EPERM here registers us
        if fd >= 0 { close(fd) }
    }
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

// RootView is the NavigationStack the popover hosts. Every screen pushes onto it
// and the native back button returns — the "persistent menubar that can go back"
// the user asked for. No screen ever kicks out to Terminal or a browser.
struct RootView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        NavigationStack {
            HomeView(engine: engine)
        }
        .frame(width: 380, height: 520)
    }
}

// ── Home ─────────────────────────────────────────────────────────────────────

struct HomeView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("𓁢 Sirsi Pantheon").font(.headline).foregroundStyle(gold)
                Spacer()
            }
            .padding(.horizontal, 16).padding(.top, 14).padding(.bottom, 8)

            // Status card
            VStack(spacing: 4) {
                Text(engine.safeBytes > 0 ? SirsiEngine.human(engine.safeBytes) : "Clean")
                    .font(.system(size: 30, weight: .bold))
                    .foregroundStyle(engine.safeBytes > 0 ? gold : .green)
                Text(engine.safeBytes > 0 ? "safe to reclaim" : "nothing to clean")
                    .font(.caption).foregroundStyle(.secondary)
                if !engine.scannedAt.isEmpty {
                    Text("scanned \(engine.scannedAt)").font(.caption2).foregroundStyle(.tertiary)
                }
            }
            .frame(maxWidth: .infinity).padding(.vertical, 12)

            Divider().padding(.horizontal, 12)

            // Deity rows
            ScrollView {
                VStack(spacing: 2) {
                    NavigationLink { InsightView() } label: {
                        DeityRow(glyph: "✨", title: "Insight — what to do next",
                                 detail: "across the platform")
                    }.buttonStyle(.plain)

                    NavigationLink { AnubisView(engine: engine) } label: {
                        DeityRow(glyph: "🐺", title: "Anubis — Hygiene",
                                 detail: engine.safeBytes > 0 ? "\(engine.safe.count) items ready" : "clean")
                    }.buttonStyle(.plain)

                    NavigationLink { HorusView(engine: engine) } label: {
                        DeityRow(glyph: "𓂀", title: "Horus — Ops",
                                 detail: engine.healthLoading ? "checking…" : engine.healthSummary,
                                 dot: severityColor(engine.healthWorst))
                    }.buttonStyle(.plain)

                    NavigationLink { CommandView(title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                        DeityRow(glyph: "𓆄", title: "Ma'at — Quality", detail: "governance")
                    }.buttonStyle(.plain)

                    NavigationLink { CommandView(title: "Thoth — Memory", args: ["thoth", "status"]) } label: {
                        DeityRow(glyph: "𓁟", title: "Thoth — Memory", detail: "memory")
                    }.buttonStyle(.plain)

                    NavigationLink { CommandView(title: "Ra — Agent Fleet", args: ["ra", "status"]) } label: {
                        DeityRow(glyph: "𓇶", title: "Ra — Agent Fleet", detail: "orchestration")
                    }.buttonStyle(.plain)

                    NavigationLink { FDAGuideView() } label: {
                        DeityRow(glyph: "⚠️", title: "Grant Full Disk Access…",
                                 detail: "so Sirsi sees everything")
                    }.buttonStyle(.plain)
                }
                .padding(.horizontal, 10).padding(.top, 6)
            }

            Divider()
            HStack {
                Button { Task { await engine.rescan() } } label: {
                    Label("Scan", systemImage: "arrow.clockwise")
                }.disabled(engine.busy)
                if engine.busy { ProgressView().controlSize(.small).padding(.leading, 4) }
                Spacer()
                Button("Quit") { NSApplication.shared.terminate(nil) }
            }
            .padding(.horizontal, 14).padding(.vertical, 10)
        }
        .task { await engine.diagnose() }   // load health when the popover opens
    }
}

// severityColor maps a diagnose severity (0 OK / 1 Warn / 2 Critical) to a dot.
func severityColor(_ sev: Int) -> Color {
    switch sev {
    case 0: return .green
    case 1: return .yellow
    default: return .red
    }
}

struct DeityRow: View {
    let glyph: String; let title: String; let detail: String
    var dot: Color? = nil
    var body: some View {
        HStack(spacing: 10) {
            Text(glyph).font(.system(size: 18)).frame(width: 26)
            Text(title).font(.system(size: 13, weight: .medium))
            Spacer()
            if let dot { Circle().fill(dot).frame(width: 7, height: 7) }
            Text(detail).font(.caption).foregroundStyle(.secondary)
            Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
        }
        .padding(.vertical, 8).padding(.horizontal, 10)
        .contentShape(Rectangle())
        .background(RoundedRectangle(cornerRadius: 7).fill(Color.primary.opacity(0.04)))
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
        VStack(alignment: .leading, spacing: 14) {
            Text("Grant Full Disk Access")
                .font(.headline).foregroundStyle(gold)
            Text("macOS won’t let Sirsi grant itself disk access — you add it once, by hand. Here’s the quickest way:")
                .font(.callout).foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            VStack(alignment: .leading, spacing: 10) {
                ForEach(steps, id: \.0) { step in
                    HStack(alignment: .top, spacing: 10) {
                        Text(step.0)
                            .font(.system(size: 12, weight: .bold))
                            .frame(width: 20, height: 20)
                            .background(Circle().fill(gold.opacity(0.20)))
                            .foregroundStyle(gold)
                        Text(step.1).font(.system(size: 12))
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }

            VStack(spacing: 8) {
                Button {
                    registerForFullDiskAccess()
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
        .navigationTitle("Full Disk Access")
    }
}

// ── Horus — Ops (health → cause) ─────────────────────────────────────────────

struct HorusView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        VStack(spacing: 0) {
            if engine.health.isEmpty {
                VStack(spacing: 10) {
                    if engine.healthLoading { ProgressView() }
                    Text(engine.healthLoading ? "Checking system health…" : "No health data")
                        .font(.callout).foregroundStyle(.secondary)
                }.frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List {
                    Section {
                        ForEach(engine.health) { f in
                            HealthRow(finding: f)
                        }
                    } header: {
                        let worst = engine.healthWorst
                        Text(worst == 0 ? "ALL SYSTEMS HEALTHY"
                             : (worst == 1 ? "ATTENTION — \(engine.health.filter{$0.severity>=1}.count) item(s)"
                                : "CRITICAL — \(engine.health.filter{$0.severity>=2}.count) item(s)"))
                            .foregroundStyle(severityColor(engine.healthWorst))
                    }
                }
                .listStyle(.inset)
            }
            Divider()
            HStack {
                Button { Task { await engine.diagnose() } } label: {
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

struct HealthRow: View {
    let finding: DiagFinding
    @State private var expanded = false
    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 8) {
                Circle().fill(severityColor(finding.severity)).frame(width: 8, height: 8)
                Text(finding.check).font(.system(size: 12, weight: .semibold))
                Spacer()
                if let d = finding.detail, !d.isEmpty {
                    Image(systemName: expanded ? "chevron.down" : "chevron.right")
                        .font(.caption2).foregroundStyle(.tertiary)
                }
            }
            Text(finding.message).font(.caption).foregroundStyle(.secondary)
            if expanded, let d = finding.detail, !d.isEmpty {
                Text(d).font(.caption2.monospaced()).foregroundStyle(.tertiary)
                    .padding(.top, 2).textSelection(.enabled)
            }
        }
        .padding(.vertical, 2)
        .contentShape(Rectangle())
        .onTapGesture { if finding.detail?.isEmpty == false { expanded.toggle() } }
    }
}

extension View {
    // Visually mark a not-yet-ported deity row without a dead-looking control.
    func disabledRow() -> some View { self.opacity(0.45) }
}

// ── Anubis ───────────────────────────────────────────────────────────────────

struct AnubisView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        VStack(spacing: 0) {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    HStack {
                        VStack(alignment: .leading) {
                            Text("\(SirsiEngine.human(engine.safeBytes)) safe").font(.title2.bold()).foregroundStyle(gold)
                            Text("\(engine.safe.count) regenerable items · trash-first, recoverable")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        Spacer()
                    }
                    .padding(.top, 6)

                    NavigationLink { CleanReviewView(engine: engine) } label: {
                        ActionCard(glyph: "🧹", title: "Review & Clean Waste",
                                   sub: "See every file, then move safe items to Trash")
                    }.buttonStyle(.plain).disabled(engine.safe.isEmpty)

                    Button { Task { await engine.rescan() } } label: {
                        ActionCard(glyph: "🔍", title: "Scan for Waste",
                                   sub: engine.busy ? "scanning…" : "re-scan the workstation now")
                    }.buttonStyle(.plain).disabled(engine.busy)

                    NavigationLink { CommandView(title: "Leftover Apps", args: ["ghosts"]) } label: {
                        ActionCard(glyph: "👻", title: "Find Leftover Apps",
                                   sub: "detect remnants of uninstalled apps (Ka)")
                    }.buttonStyle(.plain)

                    if engine.cautionBytes > 0 {
                        Text("\(SirsiEngine.human(engine.cautionBytes)) of caution-tier items (app remnants) are excluded from one-click cleaning — review them in a terminal with `sirsi anubis clean --include-caution`.")
                            .font(.caption2).foregroundStyle(.secondary)
                            .padding(.top, 4)
                    }
                }
                .padding(16)
            }
        }
        .navigationTitle("Anubis")
    }
}

struct ActionCard: View {
    let glyph: String; let title: String; let sub: String
    var body: some View {
        HStack(spacing: 12) {
            Text(glyph).font(.system(size: 22)).frame(width: 30)
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(.system(size: 14, weight: .semibold))
                Text(sub).font(.caption).foregroundStyle(.secondary).multilineTextAlignment(.leading)
            }
            Spacer()
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 9).fill(Color.primary.opacity(0.05)))
    }
}

// ── Clean Review — the inline manifest + confirm + result ────────────────────

struct CleanReviewView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var resultLine: String?
    @State private var showCaution = false

    var body: some View {
        VStack(spacing: 0) {
            if let resultLine {
                // Result state — inline, no kick-out.
                VStack(spacing: 10) {
                    Text("✓").font(.system(size: 40)).foregroundStyle(.green)
                    Text(resultLine).font(.callout).multilineTextAlignment(.center)
                    Text("Moved to Trash — recoverable until you empty it.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                .frame(maxWidth: .infinity).padding(24)
            } else {
                List {
                    Section {
                        ForEach(engine.safe) { f in
                            HStack {
                                Text(SirsiEngine.human(f.sizeBytes))
                                    .font(.caption.monospaced()).foregroundStyle(gold)
                                    .frame(width: 64, alignment: .trailing)
                                Text(shorten(f.path)).font(.caption).lineLimit(1).truncationMode(.middle)
                                Spacer()
                            }
                        }
                    } header: {
                        Text("WILL MOVE TO TRASH — \(engine.safe.count) items · \(SirsiEngine.human(engine.safeBytes))")
                    } footer: {
                        Text("Regenerable caches, node_modules, build artifacts. Protected system paths are never touched.")
                    }

                    if !engine.caution.isEmpty {
                        Section {
                            DisclosureGroup("Excluded — \(engine.caution.count) caution items · \(SirsiEngine.human(engine.cautionBytes))", isExpanded: $showCaution) {
                                ForEach(engine.caution) { f in
                                    HStack {
                                        Text(SirsiEngine.human(f.sizeBytes))
                                            .font(.caption.monospaced()).foregroundStyle(.secondary)
                                            .frame(width: 64, alignment: .trailing)
                                        Text(shorten(f.path)).font(.caption).lineLimit(1).truncationMode(.middle)
                                        Spacer()
                                    }
                                }
                            }.font(.caption)
                        } footer: {
                            Text("Not cleaned here. Review deliberately: sirsi anubis clean --include-caution --confirm")
                        }
                    }
                }
                .listStyle(.inset)

                Divider()
                HStack(spacing: 10) {
                    if engine.busy {
                        ProgressView().controlSize(.small)
                        Text("Moving to Trash…").font(.caption).foregroundStyle(.secondary)
                    } else {
                        Button {
                            Task { resultLine = await engine.cleanSafe() }
                        } label: {
                            Text("Move \(engine.safe.count) items (\(SirsiEngine.human(engine.safeBytes))) to Trash")
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.borderedProminent).tint(gold)
                        .disabled(engine.safe.isEmpty)
                    }
                }
                .padding(12)
            }
        }
        .navigationTitle("Review & Clean")
    }

    private func shorten(_ p: String) -> String {
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        return p.hasPrefix(home) ? "~" + p.dropFirst(home.count) : p
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
            ScrollView {
                if loading {
                    HStack { Spacer(); ProgressView(); Spacer() }.padding(.top, 60)
                } else {
                    Text(output.isEmpty ? "No output." : output)
                        .font(.system(size: 11.5, design: .monospaced))
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
}

struct InsightView: View {
    @State private var report: InsightReport?
    @State private var loading = true
    @State private var askingGemma = false

    var body: some View {
        VStack(spacing: 0) {
            if loading && report == nil {
                HStack { Spacer(); ProgressView(); Spacer() }.padding(.top, 60)
            } else if let r = report {
                List {
                    if let n = r.narrative, !n.isEmpty {
                        Section { Text(n).font(.callout) } header: { Text("𓂀 LOCAL GEMMA") }
                    }
                    Section {
                        if r.actions.isEmpty {
                            Text("Everything healthy — nothing to do right now.")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        ForEach(r.actions) { a in
                            VStack(alignment: .leading, spacing: 2) {
                                Text(a.title).font(.system(size: 12, weight: .semibold))
                                Text(a.why).font(.caption2).foregroundStyle(.secondary)
                                Text(a.command).font(.caption.monospaced()).foregroundStyle(gold).textSelection(.enabled)
                            }.padding(.vertical, 1)
                        }
                    } header: { Text("DO NEXT") }

                    Section {
                        ForEach(r.signals) { s in
                            HStack(spacing: 8) {
                                Circle().fill(severityColor(min(s.severity, 2))).frame(width: 7, height: 7)
                                Text(s.deity).font(.caption)
                                Spacer()
                                Text(s.status).font(.caption2).foregroundStyle(.secondary)
                            }
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
                    Label("Ask Gemma", systemImage: "sparkles")
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
}
