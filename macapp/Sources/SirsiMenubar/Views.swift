import SwiftUI

private let gold = Color(red: 0.78, green: 0.66, blue: 0.32)

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
                    NavigationLink { AnubisView(engine: engine) } label: {
                        DeityRow(glyph: "🐺", title: "Anubis — Hygiene",
                                 detail: engine.safeBytes > 0 ? "\(engine.safe.count) items ready" : "clean")
                    }.buttonStyle(.plain)

                    DeityRow(glyph: "𓂀", title: "Horus — Ops", detail: "dashboard").disabledRow()
                    DeityRow(glyph: "𓆄", title: "Ma'at — Quality", detail: "gate").disabledRow()
                    DeityRow(glyph: "𓁟", title: "Thoth — Memory", detail: "sync").disabledRow()
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
    }
}

struct DeityRow: View {
    let glyph: String; let title: String; let detail: String
    var body: some View {
        HStack(spacing: 10) {
            Text(glyph).font(.system(size: 18)).frame(width: 26)
            Text(title).font(.system(size: 13, weight: .medium))
            Spacer()
            Text(detail).font(.caption).foregroundStyle(.secondary)
            Image(systemName: "chevron.right").font(.caption2).foregroundStyle(.tertiary)
        }
        .padding(.vertical, 8).padding(.horizontal, 10)
        .contentShape(Rectangle())
        .background(RoundedRectangle(cornerRadius: 7).fill(Color.primary.opacity(0.04)))
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
