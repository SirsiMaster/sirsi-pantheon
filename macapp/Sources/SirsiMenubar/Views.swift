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

            // Status card — only meaningful reclaim (≥ threshold) reads as waste;
            // trivial caches read "Clean" so the surface never alarms on 230 KB.
            VStack(spacing: 4) {
                let hasWaste = engine.safeBytes >= SirsiEngine.wasteThreshold
                Text(hasWaste ? SirsiEngine.human(engine.safeBytes) : "Clean")
                    .font(.system(size: 30, weight: .bold))
                    .foregroundStyle(hasWaste ? gold : .green)
                Text(hasWaste ? "safe to reclaim" : "nothing significant to clean")
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
                    NavigationLink { InsightView(engine: engine) } label: {
                        DeityRow(glyph: "✨", title: "Insight — what to do next",
                                 detail: "across the platform")
                    }.buttonStyle(.plain)

                    NavigationLink { AnubisView(engine: engine) } label: {
                        DeityRow(glyph: "🐺", title: "Anubis — Hygiene",
                                 detail: engine.safeBytes >= SirsiEngine.wasteThreshold ? "\(engine.safe.count) items ready" : "clean")
                    }.buttonStyle(.plain)

                    NavigationLink { HorusView(engine: engine) } label: {
                        DeityRow(glyph: "𓂀", title: "Horus — Ops",
                                 detail: engine.healthLoading ? "checking…" : engine.healthSummary,
                                 dot: statusColor(engine.healthStatus))
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "Ma'at — Quality", args: ["maat", "audit"]) } label: {
                        DeityRow(glyph: "𓆄", title: "Ma'at — Quality", detail: "governance")
                    }.buttonStyle(.plain)

                    NavigationLink { ThothMemoryInfoView() } label: {
                        DeityRow(glyph: "𓁟", title: "Thoth — Memory", detail: "memory")
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "Ra — Agent Fleet", args: ["ra", "status"]) } label: {
                        DeityRow(glyph: "𓇶", title: "Ra — Agent Fleet", detail: "orchestration")
                    }.buttonStyle(.plain)

                    NavigationLink { RouterView(engine: engine) } label: {
                        DeityRow(glyph: "🛰️", title: "Router — Fabric",
                                 detail: engine.routerSummary,
                                 dot: statusColor(engine.routerStatus))
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "Osiris — Checkpoints", args: ["osiris", "risk"]) } label: {
                        DeityRow(glyph: "𓁹", title: "Osiris — Checkpoints", detail: "uncommitted risk")
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "Seshat — Knowledge", args: ["seshat", "list"]) } label: {
                        DeityRow(glyph: "𓁆", title: "Seshat — Knowledge", detail: "ingestion")
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "Net — Plan", args: ["net", "status"]) } label: {
                        DeityRow(glyph: "𓁯", title: "Net — Plan", detail: "alignment")
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "Vault — Context", args: ["vault", "stats"]) } label: {
                        DeityRow(glyph: "🏛️", title: "Vault — Context", detail: "code search")
                    }.buttonStyle(.plain)

                    NavigationLink { ResultView(engine: engine, title: "RTK — Output Filter", args: ["rtk", "stats"]) } label: {
                        DeityRow(glyph: "⚡", title: "RTK — Output Filter", detail: "noise sieve")
                    }.buttonStyle(.plain)

                    NavigationLink { ActivityView(engine: engine) } label: {
                        DeityRow(glyph: "𓆎", title: "Activity — what Pantheon did",
                                 detail: engine.activity.isEmpty ? "ledger" : "\(engine.activity.count) logged")
                    }.buttonStyle(.plain)

                    // Only nag for Full Disk Access while we don't have it. Once
                    // granted, the row disappears and a quiet confirmation shows.
                    if !engine.hasFDA {
                        NavigationLink { FDAGuideView() } label: {
                            DeityRow(glyph: "⚠️", title: "Grant Full Disk Access…",
                                     detail: "so Sirsi sees everything")
                        }.buttonStyle(.plain)
                    } else {
                        DeityRow(glyph: "✅", title: "Full Disk Access",
                                 detail: "granted")
                    }
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
        .task { engine.loadActivity(); await engine.diagnose(); await engine.loadRouterBoard() }   // health + ledger + fabric on open
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

// BackBar is the in-content "‹ Back" header every drilled-in screen needs. On
// macOS a NavigationStack draws its back button in the host window's toolbar —
// which an NSPopover does not have — so the native control is invisible and the
// user gets trapped. This calls @Environment(\.dismiss) to pop the stack
// regardless of toolbar chrome. Put it at the very top of each pushed view.
struct BackBar: View {
    @Environment(\.dismiss) private var dismiss
    let title: String
    var body: some View {
        HStack(spacing: 6) {
            Button { dismiss() } label: {
                Image(systemName: "chevron.left").font(.system(size: 12, weight: .semibold))
                Text("Back").font(.system(size: 12))
            }
            .buttonStyle(.plain).foregroundStyle(gold)
            Spacer()
            Text(title).font(.system(size: 12, weight: .semibold)).foregroundStyle(.secondary)
            Spacer()
            // invisible spacer mirroring the back button keeps the title centered
            Image(systemName: "chevron.left").font(.system(size: 12)).opacity(0)
            Text("Back").font(.system(size: 12)).opacity(0)
        }
        .padding(.horizontal, 12).padding(.vertical, 8)
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
        }
        .navigationTitle("Full Disk Access")
    }
}

// ── Horus — Ops (health → cause) ─────────────────────────────────────────────

struct HorusView: View {
    @ObservedObject var engine: SirsiEngine

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Horus — Ops")
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
                            HealthRow(engine: engine, finding: f)
                        }
                    } header: {
                        // Canonical green/amber/red roll-up — NOT raw worst-severity
                        // (which read CRITICAL on historical 7-day trends).
                        let n = engine.healthIssueCount
                        Text(engine.healthStatus == "green" ? "ALL SYSTEMS HEALTHY"
                             : (engine.healthStatus == "amber" ? "ATTENTION — \(n) item(s)"
                                : "CRITICAL — \(n) item(s)"))
                            .foregroundStyle(statusColor(engine.healthStatus))
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
    @ObservedObject var engine: SirsiEngine
    let finding: DiagFinding

    private var hasFix: Bool { !(finding.fix ?? "").isEmpty }
    private var navigable: Bool { hasFix || !(finding.detail ?? "").isEmpty }

    var body: some View {
        if navigable {
            NavigationLink { FindingView(engine: engine, finding: finding) } label: { row }
                .buttonStyle(.plain)
        } else {
            row
        }
    }

    private var row: some View {
        HStack(spacing: 8) {
            Circle().fill(findingColor(finding)).frame(width: 8, height: 8)
            VStack(alignment: .leading, spacing: 2) {
                Text(finding.check).font(.system(size: 12, weight: .semibold))
                Text(finding.message).font(.caption).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer()
            if hasFix {
                Image(systemName: "wrench.and.screwdriver.fill").font(.caption2).foregroundStyle(gold)
            } else if navigable {
                Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
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
                        Text(finding.message).font(.system(size: 14, weight: .semibold))
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
                                        Text(e.name).font(.callout)
                                            .lineLimit(1).truncationMode(.middle)
                                        Spacer(minLength: 12)
                                        if let v = e.value {
                                            Text(v).font(.callout.monospaced())
                                        }
                                    }
                                    .padding(.horizontal, 10).padding(.vertical, 5)
                                    if e.id != entries.count - 1 { Divider() }
                                }
                            }
                            .textSelection(.enabled)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        } else {
                            Text(d).font(.callout)
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
                                Image(systemName: fixIcon).font(.caption)
                                    .foregroundStyle(.secondary).padding(.top, 1)
                                Text(note).font(.caption).foregroundStyle(.secondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        }
                        Text(fixSectionLabel).font(.caption2.weight(.semibold)).foregroundStyle(.secondary)
                        NavigationLink {
                            ResultView(engine: engine, title: finding.check, args: sirsiArgs(fix),
                                       reverifyCheck: finding.check, reverifyKind: finding.fixKind)
                        } label: {
                            HStack(spacing: 8) {
                                Image(systemName: fixIcon)
                                VStack(alignment: .leading, spacing: 1) {
                                    Text(fixButtonLabel).font(.system(size: 12, weight: .semibold))
                                    Text(fix).font(.caption2.monospaced())
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
                            .font(.callout).foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    } else {
                        Text("Informational.")
                            .font(.callout).foregroundStyle(.secondary)
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

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Router — Fabric")
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {

                    // ── Blockers (current + fixable ONLY) ───────────────────
                    if engine.routerHasBlockers {
                        SectionLabel("BLOCKERS — FIX TO UNSTRAND WORK", tint: .red)

                        ForEach(engine.routerAuthBlockers) { h in
                            AuthBlockerCard(engine: engine, health: h)
                        }
                        if !engine.routerDaemonBlockers.isEmpty {
                            DaemonBlockerCard(engine: engine,
                                              broken: engine.routerDaemonBlockers,
                                              onResult: { resultLine = $0 })
                        }
                    } else {
                        HStack(spacing: 8) {
                            Circle().fill(.green).frame(width: 8, height: 8)
                            Text("Fabric healthy — no blockers")
                                .font(.system(size: 13, weight: .semibold))
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
                            .font(.caption).foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                        ForEach(engine.routerStranded) { s in
                            NavigationLink {
                                StrandedAgentView(engine: engine, agent: s)
                            } label: {
                                HStack(spacing: 10) {
                                    Text("📥").font(.system(size: 16)).frame(width: 24)
                                    VStack(alignment: .leading, spacing: 1) {
                                        Text(s.agentId).font(.system(size: 13, weight: .medium))
                                        Text("\(s.openItems) item\(s.openItems == 1 ? "" : "s") waiting")
                                            .font(.caption).foregroundStyle(.secondary)
                                    }
                                    Spacer()
                                    Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
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
                                Text("🛈").font(.callout)
                                VStack(alignment: .leading, spacing: 2) {
                                    Text("\(h.agentType) — auth check inconclusive")
                                        .font(.system(size: 12, weight: .medium))
                                    Text("The CLI didn't answer in time (a cold start), so we can't confirm login. This is not a logout — it clears on its own and blocks nothing.")
                                        .font(.caption).foregroundStyle(.secondary)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                        }
                    }

                    if let line = resultLine {
                        Text(line).font(.caption.monospaced()).foregroundStyle(.secondary)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }

                    Spacer()
                }.padding(16)
            }
            if engine.busy {
                HStack { ProgressView().controlSize(.small); Text("Working…").font(.caption).foregroundStyle(.secondary); Spacer() }
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
        Text(text).font(.caption2.weight(.semibold)).foregroundStyle(tint)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

// AuthBlockerCard surfaces a REAL logout (needs_login) with a re-auth affordance.
// We never authenticate programmatically — we open Terminal and hand the operator
// the exact command to run, and offer to copy it.
struct AuthBlockerCard: View {
    @ObservedObject var engine: SirsiEngine
    let health: RBAgentHealth
    private var reauthCmd: String { "\(health.agentType)  # then run /login inside it" }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Text("🔑").font(.system(size: 18))
                VStack(alignment: .leading, spacing: 1) {
                    Text("\(health.agentType) needs re-login")
                        .font(.system(size: 13, weight: .semibold))
                    Text(blockedNote).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
            Text("Sirsi never signs in for you. Open Terminal, run \(health.agentType), then /login.")
                .font(.caption).foregroundStyle(.secondary)
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
                Text("⚙️").font(.system(size: 18))
                VStack(alignment: .leading, spacing: 1) {
                    Text("\(broken.count) router daemon\(broken.count == 1 ? "" : "s") missing")
                        .font(.system(size: 13, weight: .semibold))
                    Text("Work can't relay while a session is closed.")
                        .font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
            }
            ForEach(broken) { d in
                Text("• \(friendlyDaemon(d.role)) (\(d.label))")
                    .font(.caption).foregroundStyle(.secondary)
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
                        Text("\(agent.openItems)").font(.system(size: 34, weight: .bold)).foregroundStyle(gold)
                        Text("item\(agent.openItems == 1 ? "" : "s") waiting").font(.caption).foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity).padding(.vertical, 8)

                    Text("This agent has work in its inbox but no armed session watching. Arming a wake channel installs a pull-loop that checks its inbox automatically, so the work no longer waits for someone to open the session.")
                        .font(.callout).foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)

                    Button {
                        Task { resultLine = await engine.installWake(agent: agent.agentId) }
                    } label: {
                        Label("Arm wake channel", systemImage: "bolt.horizontal.circle")
                            .frame(maxWidth: .infinity)
                    }.buttonStyle(.borderedProminent).tint(gold).disabled(engine.busy)

                    Text("Runs: sirsi router wake-install \(agent.agentId)")
                        .font(.caption2.monospaced()).foregroundStyle(.tertiary)

                    if let line = resultLine {
                        Text(line).font(.caption.monospaced()).foregroundStyle(.secondary)
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(10)
                            .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                    }
                    if engine.busy {
                        HStack { ProgressView().controlSize(.small); Text("Arming…").font(.caption).foregroundStyle(.secondary) }
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
                            Text("\(SirsiEngine.human(engine.safeBytes)) safe").font(.title2.bold()).foregroundStyle(gold)
                            Text("\(engine.safe.count) regenerable items · trash-first, recoverable")
                                .font(.caption).foregroundStyle(.secondary)
                        }
                        Spacer()
                    }
                    .padding(.top, 6)

                    // ONE unified flow (was two split, confusing buttons): scan
                    // with visible progress → review every item → clean the ones
                    // you pick. ScanCleanView owns the whole workflow.
                    NavigationLink { ScanCleanView(engine: engine) } label: {
                        ActionCard(glyph: "🧹", title: "Scan & Clean Waste",
                                   sub: "Find waste, review every item, move what you choose to Trash")
                    }.buttonStyle(.plain)

                    // A real structured screen — the list of leftover apps and what
                    // to do — not a terminal transcript dumped into the popover.
                    NavigationLink { GhostsView(engine: engine) } label: {
                        ActionCard(glyph: "👻", title: "Find Leftover Apps",
                                   sub: "Remnants of apps you've uninstalled (Ka)")
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
            Text("🛈").font(.callout)
            VStack(alignment: .leading, spacing: 2) {
                Text("\(SirsiEngine.human(bytes)) held back for now")
                    .font(.callout.weight(.semibold))
                Text("\(count) caution-tier items (things like package caches and app remnants) aren't cleaned with one click, because they take longer to rebuild. Open Scan & Clean to review them.")
                    .font(.footnote).foregroundStyle(.secondary)
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
    @Environment(\.dismiss) private var dismiss
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
                .font(.callout).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity).padding(.top, 60)
    }

    // Empty — no scan yet, or nothing safe to clean. Always offers the next step.
    private var emptyState: some View {
        VStack(spacing: 14) {
            Text(engine.scannedAt.isEmpty ? "🔍" : "✓")
                .font(.system(size: 40))
                .foregroundStyle(engine.scannedAt.isEmpty ? Color.secondary : .green)
            Text(engine.scannedAt.isEmpty
                 ? "Scan your Mac to find reclaimable waste."
                 : "Nothing safe to clean right now.")
                .font(.callout).multilineTextAlignment(.center)
            Button { Task { await engine.rescan(); syncSelection() } } label: {
                Label("Scan now", systemImage: "magnifyingglass").frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent).tint(gold)
        }
        .frame(maxWidth: .infinity).padding(24)
    }

    private func resultState(_ line: String) -> some View {
        VStack(spacing: 14) {
            Text("✓").font(.system(size: 40)).foregroundStyle(.green)
            Text(line).font(.callout).multilineTextAlignment(.center)
            Text("Moved to Trash — recoverable until you empty it.")
                .font(.caption).foregroundStyle(.secondary)
            Button { dismiss() } label: { Text("Done").frame(maxWidth: .infinity) }
                .buttonStyle(.borderedProminent).tint(gold).padding(.top, 4)
        }
        .frame(maxWidth: .infinity).padding(24)
    }

    private var reviewList: some View {
        List {
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
                            .font(.callout.weight(.semibold))
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
                    Image(systemName: selected.contains(f.path) ? "checkmark.circle.fill" : "circle")
                        .font(.system(size: 15))
                        .foregroundStyle(selected.contains(f.path) ? gold : Color.secondary)
                }
                .buttonStyle(.plain)
            }
            NavigationLink { ItemDetailView(engine: engine, finding: f) } label: {
                HStack(spacing: 8) {
                    VStack(alignment: .leading, spacing: 1) {
                        Text(f.description).font(.caption).lineLimit(1)
                        Text(owningEntity(f)).font(.caption2).foregroundStyle(.secondary).lineLimit(1)
                    }
                    Spacer(minLength: 6)
                    Text(SirsiEngine.human(f.sizeBytes))
                        .font(.caption.monospaced())
                        .foregroundStyle(toggleable ? gold : .secondary)
                    Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
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
            .font(.caption).buttonStyle(.plain).foregroundStyle(gold)
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
            Text(label.uppercased()).font(.caption2).foregroundStyle(.tertiary)
            Text(value)
                .font(mono ? .caption.monospaced() : .callout)
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
    @Environment(\.dismiss) private var dismiss
    @State private var resultLine: String?

    private var isSafe: Bool { finding.severity == "safe" }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Item")
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text(finding.description).font(.headline)

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
                        Label("Reveal in Finder", systemImage: "folder").font(.caption)
                    }
                    .buttonStyle(.plain).foregroundStyle(gold).padding(.top, 2)
                }
                .padding(16)
            }

            Divider()
            if let resultLine {
                HStack {
                    Text("✓ \(resultLine)").font(.caption).foregroundStyle(.green)
                    Spacer()
                    Button("Done") { dismiss() }.font(.caption)
                }.padding(12)
            } else if isSafe {
                Button {
                    Task { resultLine = await engine.cleanSelected(paths: [finding.path]) }
                } label: {
                    if engine.busy {
                        HStack { ProgressView().controlSize(.small); Text("Moving to Trash…").font(.caption) }
                            .frame(maxWidth: .infinity)
                    } else {
                        Text("Move this to Trash (\(SirsiEngine.human(finding.sizeBytes)))")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.borderedProminent).tint(gold).disabled(engine.busy).padding(12)
            } else {
                Text("Held back from one-click cleaning — rebuild it deliberately.")
                    .font(.caption).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity).padding(12)
            }
        }
        .navigationTitle("Item")
    }
}

// ── Ghosts (Ka) — structured leftover-app screen, not a CLI transcript ───────
//
// Renders `sirsi ghosts --json` via the shared CommandResult contract: a summary,
// the leftover apps found (each with its residual count + reclaimable size), and
// the real next steps as buttons — so the user always has a way forward.
struct GhostsView: View {
    @ObservedObject var engine: SirsiEngine
    @State private var result: CommandResult?
    @State private var loading = true

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Leftover Apps")
            if loading {
                VStack(spacing: 12) {
                    ProgressView()
                    Text("Scanning for remnants of uninstalled apps…")
                        .font(.callout).foregroundStyle(.secondary)
                    Text("This can take a minute.").font(.caption2).foregroundStyle(.tertiary)
                }
                .frame(maxWidth: .infinity).padding(.top, 60)
            } else if let r = result {
                List {
                    Section { Text(r.summary).font(.callout) } header: { Text("RESULT") }
                    if !r.evidence.isEmpty {
                        Section {
                            ForEach(r.evidence) { e in
                                HStack {
                                    Text(e.label).font(.caption).lineLimit(1)
                                    Spacer(minLength: 8)
                                    Text(e.value).font(.caption2.monospaced())
                                        .foregroundStyle(.secondary).multilineTextAlignment(.trailing)
                                }
                            }
                        } header: { Text("DETAILS") }
                    }
                    if !r.nextActions.isEmpty {
                        Section {
                            ForEach(r.nextActions) { a in
                                NavigationLink {
                                    ResultView(engine: engine, title: a.label,
                                               args: InsightView.commandArgs(a.command))
                                } label: {
                                    HStack {
                                        VStack(alignment: .leading, spacing: 1) {
                                            Text(a.label).font(.caption.weight(.semibold))
                                            if let d = a.description {
                                                Text(d).font(.caption2).foregroundStyle(.secondary).lineLimit(2)
                                            }
                                        }
                                        Spacer()
                                        Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
                                    }
                                    .contentShape(Rectangle())
                                }
                                .buttonStyle(.plain)
                            }
                        } header: { Text("WHAT YOU CAN DO") }
                    }
                }
                .listStyle(.inset)
            } else {
                VStack(spacing: 8) {
                    Text("Couldn't scan for leftover apps.").foregroundStyle(.secondary)
                    Button("Try again") { Task { await load() } }.font(.caption)
                }
                .frame(maxWidth: .infinity).padding(40)
            }
        }
        .navigationTitle("Leftover Apps")
        .task { await load() }
    }

    private func load() async {
        loading = true
        result = await SirsiEngine.runResult(args: ["ghosts"])
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
    @State private var postFix: String?   // honest verdict after re-verify
    @State private var didReverify = false // re-verify fires once (across load/apply paths)

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: title)
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
                    Text("Working…").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                }.padding(.horizontal, 14).padding(.vertical, 8)
            }
            if let pf = postFix {
                Divider()
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: pf.hasPrefix("✓") ? "checkmark.seal.fill" : "info.circle.fill")
                        .foregroundStyle(pf.hasPrefix("✓") ? .green : .secondary).padding(.top, 1)
                    Text(pf).font(.caption).foregroundStyle(.primary)
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
        .task { await load() }
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

    @ViewBuilder private func structuredScroll(_ r: CommandResult) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                if let t = toast {
                    HStack(spacing: 6) {
                        Image(systemName: "checkmark.seal.fill").foregroundStyle(.green)
                        Text(t).font(.caption)
                    }
                    .padding(8).frame(maxWidth: .infinity, alignment: .leading)
                    .background(RoundedRectangle(cornerRadius: 7).fill(Color.green.opacity(0.12)))
                }
                Text(r.summary).font(.system(size: 14, weight: .semibold))
                    .fixedSize(horizontal: false, vertical: true)

                if !r.evidence.isEmpty {
                    VStack(spacing: 0) {
                        ForEach(r.evidence) { f in
                            HStack {
                                Text(f.label).font(.caption).foregroundStyle(.secondary)
                                Spacer()
                                Text(f.value).font(.caption.monospaced())
                            }.padding(.vertical, 7)
                            if f.id != r.evidence.last?.id { Divider() }
                        }
                    }
                    .padding(.horizontal, 12)
                    .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.04)))
                }

                if !r.nextActions.isEmpty {
                    Text("WHAT YOU CAN DO").font(.caption2.weight(.semibold)).foregroundStyle(.secondary)
                    VStack(spacing: 8) {
                        ForEach(r.nextActions) { actionButton($0) }
                    }
                }
            }.padding(16)
        }
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
                Text(a.label).font(.system(size: 12, weight: .semibold))
                if let d = a.description, !d.isEmpty {
                    Text(d).font(.caption2)
                        .foregroundStyle(prominent ? Color.white.opacity(0.85) : Color.secondary)
                }
            }
            Spacer()
        }.frame(maxWidth: .infinity).padding(.vertical, 2)
    }

    private var rawScroll: some View {
        ScrollView {
            Text(raw.isEmpty ? "No output." : raw)
                .font(.system(size: 11.5, design: .monospaced))
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(14)
        }
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
        await engine.diagnose()
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
        _ = await SirsiEngine.run(args: sirsiArgs(a.command), stdin: nil)
        applying = false
        await load()
        engine.refresh()
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
                    Image(systemName: "clock.arrow.circlepath").font(.title).foregroundStyle(.tertiary)
                    Text("No actions yet").font(.callout).foregroundStyle(.secondary)
                    Text("Fixes you apply show here — what changed, when, and the command that ran. Everything is reversible.")
                        .font(.caption2).foregroundStyle(.tertiary).multilineTextAlignment(.center)
                }.frame(maxWidth: .infinity, maxHeight: .infinity).padding(28)
            } else {
                List(engine.activity) { e in
                    VStack(alignment: .leading, spacing: 2) {
                        HStack {
                            Text(e.title).font(.system(size: 12, weight: .semibold))
                            Spacer()
                            Text(e.when).font(.caption2).foregroundStyle(.tertiary)
                        }
                        Text(e.command).font(.caption.monospaced()).foregroundStyle(gold)
                        if !e.result.isEmpty {
                            Text(e.result).font(.caption2).foregroundStyle(.secondary).lineLimit(2)
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

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Insight")
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
                            // Each suggestion RUNS its command in-panel — tap pushes
                            // a CommandView that executes `sirsi <cmd>` and shows
                            // output (TUI-is-the-session: never a dead command label).
                            NavigationLink {
                                ResultView(engine: engine, title: a.title, args: Self.commandArgs(a.command))
                            } label: {
                                HStack(alignment: .top, spacing: 8) {
                                    VStack(alignment: .leading, spacing: 2) {
                                        Text(a.title).font(.system(size: 12, weight: .semibold))
                                        Text(a.why).font(.caption2).foregroundStyle(.secondary)
                                            .fixedSize(horizontal: false, vertical: true)
                                        Text(a.command).font(.caption.monospaced()).foregroundStyle(gold)
                                    }
                                    Spacer()
                                    Image(systemName: "play.circle.fill")
                                        .foregroundStyle(gold).font(.system(size: 14))
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
                            NavigationLink {
                                Self.deityDestination(engine: engine, deity: s.deity)
                            } label: {
                                HStack(spacing: 8) {
                                    Circle().fill(insightSeverityColor(min(s.severity, 2))).frame(width: 7, height: 7)
                                    Text(s.deity).font(.caption)
                                    Spacer()
                                    Text(s.status).font(.caption2).foregroundStyle(.secondary)
                                    Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
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
            ThothMemoryInfoView()
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
struct ThothMemoryInfoView: View {
    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Thoth — Memory")
            VStack(alignment: .leading, spacing: 12) {
                Text("Project memory lives with each project.")
                    .font(.system(size: 13, weight: .semibold))
                Text("Thoth keeps a small memory file inside every project folder so AI sessions can pick up exactly where the last one left off — no re-reading the whole codebase.")
                    .font(.callout).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                Text("The menu bar isn't inside a project, so there's nothing to show here. In a project folder, use:")
                    .font(.callout).foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                VStack(alignment: .leading, spacing: 4) {
                    Text("sirsi thoth init").font(.caption.monospaced()).foregroundStyle(gold)
                    Text("sirsi thoth sync").font(.caption.monospaced()).foregroundStyle(gold)
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(RoundedRectangle(cornerRadius: 8).fill(Color.primary.opacity(0.05)))
                Spacer()
            }
            .padding(16)
        }
        .navigationTitle("Thoth — Memory")
    }
}
