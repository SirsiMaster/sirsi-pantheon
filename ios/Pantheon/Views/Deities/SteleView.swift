import SwiftUI

/// The Stele — Event Ledger dashboard.
/// Shows real-time event bus activity across all deities with hash chain verification.
struct SteleView: View {
    @EnvironmentObject var appState: AppState
    @State private var entries: [SteleEntry] = []
    @State private var stats: SteleStats?
    @State private var verifyResult: SteleVerifyResult?
    @State private var isLoading = false
    @State private var isVerifying = false
    @State private var errorMessage: String?
    @State private var selectedDeity: String?
    @State private var expandedEntrySeq: Int64?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // Header
                DeityHeader(
                    glyph: "\u{130BD}",
                    name: "Stele",
                    subtitle: "Event Ledger",
                    description: "Append-only hash-chained event bus for all Pantheon inter-deity communication."
                )

                // A. Stats Card
                if let stats {
                    SteleStatsCard(
                        stats: stats,
                        verifyResult: verifyResult,
                        isVerifying: isVerifying
                    )
                }

                // B. Deity Activity Grid
                if let stats {
                    DeityActivityGrid(
                        deityCounts: stats.deityCounts,
                        selectedDeity: $selectedDeity
                    )
                }

                // Filter indicator
                if let selectedDeity {
                    HStack {
                        Text("Filtered: \(selectedDeity)")
                            .font(.caption)
                            .foregroundStyle(PantheonTheme.gold)
                        Spacer()
                        Button("Clear") {
                            self.selectedDeity = nil
                        }
                        .font(.caption.bold())
                        .foregroundStyle(PantheonTheme.gold)
                    }
                    .padding(.horizontal, 8)
                }

                // Error
                if let errorMessage {
                    ErrorBanner(message: errorMessage)
                }

                // Loading
                if isLoading {
                    ScanResultSkeleton()
                }

                // C. Event Timeline
                if !isLoading {
                    let filtered = filteredEntries
                    if filtered.isEmpty && !entries.isEmpty {
                        Text("No events for this deity.")
                            .font(.subheadline)
                            .foregroundStyle(PantheonTheme.textSecondary)
                            .padding()
                    } else {
                        ForEach(filtered) { entry in
                            SteleEntryRow(
                                entry: entry,
                                isExpanded: expandedEntrySeq == entry.seq,
                                onTap: {
                                    withAnimation(.easeInOut(duration: 0.2)) {
                                        expandedEntrySeq = expandedEntrySeq == entry.seq ? nil : entry.seq
                                    }
                                }
                            )
                        }
                    }
                }

                // D. Hash Chain Status
                if let vr = verifyResult {
                    HashChainStatusCard(result: vr)
                }
            }
            .padding()
        }
        .background(PantheonTheme.background)
        .task { await loadData() }
        .refreshable { await loadData() }
    }

    private var filteredEntries: [SteleEntry] {
        guard let deity = selectedDeity else { return entries }
        return entries.filter { $0.deity.lowercased() == deity.lowercased() }
    }

    private func loadData() async {
        isLoading = true
        errorMessage = nil
        defer { isLoading = false }

        do {
            stats = try appState.bridge.steleStats()
        } catch {
            // Non-fatal.
        }

        do {
            entries = try await appState.bridge.steleReadRecent(count: 100)
        } catch {
            errorMessage = error.localizedDescription
        }

        // Verify chain in background.
        isVerifying = true
        do {
            verifyResult = try await appState.bridge.steleVerify()
        } catch {
            // Non-fatal.
        }
        isVerifying = false
    }
}

// MARK: - Stats Card

private struct SteleStatsCard: View {
    let stats: SteleStats
    let verifyResult: SteleVerifyResult?
    let isVerifying: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Ledger Overview")
                .font(.headline)
                .foregroundStyle(PantheonTheme.gold)

            HStack(spacing: 24) {
                StatPill(label: "Events", value: "\(stats.totalEntries)")
                StatPill(
                    label: "Chain",
                    value: isVerifying ? "..." : (verifyResult?.isVerified == true ? "Verified" : (verifyResult != nil ? "Broken" : "?"))
                )
                StatPill(label: "Span", value: stats.timeSpan)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

// MARK: - Deity Activity Grid

private struct DeityActivityGrid: View {
    let deityCounts: [String: Int]
    @Binding var selectedDeity: String?

    private let columns = Array(repeating: GridItem(.flexible(), spacing: 12), count: 4)

    private let allDeities: [(key: String, glyph: String)] = [
        ("anubis", "\u{130E3}"),
        ("maat", "\u{13184}"),
        ("thoth", "\u{1305F}"),
        ("ra", "\u{131F6}"),
        ("neith", "\u{1306F}"),
        ("seba", "\u{131FD}"),
        ("seshat", "\u{13046}"),
        ("isis", "\u{13050}"),
        ("osiris", "\u{13079}"),
        ("horus", "\u{13080}"),
        ("rtk", "\u{26A1}"),
        ("vault", "\u{1F3DB}\u{FE0F}"),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Deity Activity")
                .font(.headline)
                .foregroundStyle(PantheonTheme.gold)

            LazyVGrid(columns: columns, spacing: 12) {
                ForEach(allDeities, id: \.key) { deity in
                    let count = deityCounts[deity.key] ?? 0
                    let isSelected = selectedDeity == deity.key
                    Button {
                        selectedDeity = isSelected ? nil : deity.key
                    } label: {
                        VStack(spacing: 4) {
                            Text(deity.glyph)
                                .font(.title2)
                            Text("\(count)")
                                .font(.caption.bold())
                                .foregroundStyle(count > 0 ? PantheonTheme.gold : PantheonTheme.textSecondary)
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 8)
                        .background(isSelected ? PantheonTheme.gold.opacity(0.2) : PantheonTheme.surfaceElevated)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                        .overlay(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(isSelected ? PantheonTheme.gold : .clear, lineWidth: 1)
                        )
                    }
                    .opacity(count > 0 ? 1.0 : 0.4)
                }
            }
        }
        .padding()
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

// MARK: - Event Entry Row

private struct SteleEntryRow: View {
    let entry: SteleEntry
    let isExpanded: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 8) {
                    // Deity glyph + name
                    Text(entry.deityGlyph)
                        .font(.title3)
                    Text(entry.deityName)
                        .font(.subheadline.bold())
                        .foregroundStyle(PantheonTheme.textPrimary)

                    // Type badge
                    Text(entry.type)
                        .font(.caption2.bold())
                        .padding(.horizontal, 8)
                        .padding(.vertical, 3)
                        .background(typeColor(for: entry.typeColorName).opacity(0.2))
                        .foregroundStyle(typeColor(for: entry.typeColorName))
                        .clipShape(Capsule())

                    Spacer()

                    // Relative timestamp
                    Text(entry.relativeTime)
                        .font(.caption)
                        .foregroundStyle(PantheonTheme.textSecondary)
                }

                // Scope (if present)
                if !entry.scope.isEmpty {
                    Text(entry.scope)
                        .font(.caption)
                        .foregroundStyle(PantheonTheme.textSecondary)
                }

                // Expanded data payload
                if isExpanded, let data = entry.data, !data.isEmpty {
                    VStack(alignment: .leading, spacing: 2) {
                        ForEach(data.sorted(by: { $0.key < $1.key }), id: \.key) { key, value in
                            HStack(spacing: 6) {
                                Text(key)
                                    .font(.caption2.bold())
                                    .foregroundStyle(PantheonTheme.gold)
                                Text(value)
                                    .font(.caption2)
                                    .foregroundStyle(PantheonTheme.textSecondary)
                                    .lineLimit(3)
                            }
                        }
                    }
                    .padding(.top, 4)
                    .padding(.horizontal, 4)
                }

                // Expanded hash info
                if isExpanded {
                    HStack(spacing: 4) {
                        Text("seq:\(entry.seq)")
                        Text("hash:\(String(entry.hash.prefix(12)))...")
                    }
                    .font(.system(.caption2, design: .monospaced))
                    .foregroundStyle(PantheonTheme.textSecondary.opacity(0.6))
                    .padding(.top, 2)
                }
            }
            .padding()
            .background(PantheonTheme.surface)
            .clipShape(RoundedRectangle(cornerRadius: 8))
        }
        .buttonStyle(.plain)
    }

    private func typeColor(for name: String) -> Color {
        switch name {
        case "governance": return Color(hex: 0xC8A951)
        case "commit":     return Color(hex: 0x4CAF50)
        case "toolUse":    return Color(hex: 0x42A5F5)
        case "deploy":     return Color(hex: 0xAB47BC)
        case "driftCheck": return Color(hex: 0xFFA726)
        case "error":      return Color(hex: 0xEF5350)
        default:           return PantheonTheme.textSecondary
        }
    }
}

// MARK: - Hash Chain Status Card

private struct HashChainStatusCard: View {
    let result: SteleVerifyResult

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Image(systemName: result.isVerified ? "checkmark.shield.fill" : "exclamationmark.shield.fill")
                    .foregroundStyle(result.isVerified ? PantheonTheme.success : PantheonTheme.error)
                    .font(.title2)
                VStack(alignment: .leading, spacing: 2) {
                    Text("Hash Chain: \(result.status.capitalized)")
                        .font(.headline)
                        .foregroundStyle(result.isVerified ? PantheonTheme.success : PantheonTheme.error)
                    Text("\(result.chainLength) of \(result.totalCount) entries verified")
                        .font(.caption)
                        .foregroundStyle(PantheonTheme.textSecondary)
                }
            }

            if !result.breaks.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Chain Breaks:")
                        .font(.caption.bold())
                        .foregroundStyle(PantheonTheme.error)
                    ForEach(result.breaks.prefix(5), id: \.self) { breakMsg in
                        Text(breakMsg)
                            .font(.system(.caption2, design: .monospaced))
                            .foregroundStyle(PantheonTheme.textSecondary)
                            .lineLimit(2)
                    }
                    if result.breaks.count > 5 {
                        Text("... and \(result.breaks.count - 5) more")
                            .font(.caption2)
                            .foregroundStyle(PantheonTheme.textSecondary)
                    }
                }
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}
