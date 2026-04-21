import SwiftUI

/// 𓁢 Anubis — Infrastructure scanner view.
/// Scan the device sandbox for waste, caches, and artifacts.
struct AnubisView: View {
    @EnvironmentObject var appState: AppState
    @State private var scanResult: ScanResult?
    @State private var isScanning = false
    @State private var selectedCategories: Set<String> = []
    @State private var errorMessage: String?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // Header
                DeityHeader(
                    glyph: "𓁢",
                    name: "Anubis",
                    subtitle: "Weigh. Judge. Purge.",
                    description: "Scan for infrastructure waste — caches, build artifacts, orphaned data."
                )

                // Category filter chips
                CategoryFilterView(selectedCategories: $selectedCategories)

                // Scan button
                Button {
                    Task { await runScan() }
                } label: {
                    HStack {
                        Image(systemName: isScanning ? "progress.indicator" : "magnifyingglass")
                        Text(isScanning ? "Scanning..." : "Run Scan")
                    }
                    .frame(maxWidth: .infinity)
                    .padding()
                    .background(PantheonTheme.gold)
                    .foregroundStyle(.black)
                    .clipShape(RoundedRectangle(cornerRadius: 12))
                    .font(.headline)
                }
                .disabled(isScanning)

                // Error with retry
                if let errorMessage {
                    ErrorRetryView(message: errorMessage) { await runScan() }
                }

                // Loading skeleton
                if isScanning {
                    ScanResultSkeleton()
                }

                // Results
                if let result = scanResult, !isScanning {
                    ScanSummaryCard(result: result)

                    ForEach(result.findings) { finding in
                        FindingRow(finding: finding)
                    }
                }
            }
            .padding()
        }
        .background(PantheonTheme.background)
    }

    private func runScan() async {
        isScanning = true
        errorMessage = nil
        defer { isScanning = false }

        do {
            let rootPath = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first?.path ?? "/"
            let result = try await appState.bridge.anubisScan(
                rootPath: rootPath,
                categories: Array(selectedCategories)
            )
            scanResult = result

            // Write results to App Group for widgets
            let sorted = result.findings.sorted { $0.sizeBytes > $1.sizeBytes }
            let topFindings = sorted.prefix(3).map { (name: $0.description, size: $0.sizeBytes) }
            SharedDataManager.saveScanResults(
                findingCount: result.findings.count,
                totalSize: result.totalSize,
                topFindings: topFindings,
                rulesRan: result.rulesRan
            )
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

// MARK: - Subviews

struct CategoryFilterView: View {
    @Binding var selectedCategories: Set<String>

    private let categories = [
        ("general", "General"), ("dev", "Dev"), ("ai", "AI/ML"),
        ("ides", "IDEs"), ("cloud", "Cloud"), ("storage", "Storage"),
    ]

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(categories, id: \.0) { id, label in
                    let isSelected = selectedCategories.contains(id)
                    Button {
                        if isSelected { selectedCategories.remove(id) }
                        else { selectedCategories.insert(id) }
                    } label: {
                        Text(label)
                            .font(.caption)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 6)
                            .background(isSelected ? PantheonTheme.gold : PantheonTheme.surfaceElevated)
                            .foregroundStyle(isSelected ? .black : PantheonTheme.textSecondary)
                            .clipShape(Capsule())
                    }
                }
            }
        }
    }
}

struct ScanSummaryCard: View {
    let result: ScanResult

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Scan Complete")
                .font(.headline)
                .foregroundStyle(PantheonTheme.gold)

            HStack(spacing: 24) {
                StatPill(label: "Findings", value: "\(result.findings.count)")
                StatPill(label: "Reclaimable", value: ByteCountFormatter.string(
                    fromByteCount: result.totalSize, countStyle: .file))
                StatPill(label: "Rules", value: "\(result.rulesRan)")
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

struct FindingRow: View {
    let finding: Finding

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 4) {
                Text(finding.description)
                    .font(.subheadline)
                    .foregroundStyle(PantheonTheme.textPrimary)
                Text(finding.path)
                    .font(.caption)
                    .foregroundStyle(PantheonTheme.textSecondary)
                    .lineLimit(1)
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 4) {
                Text(finding.formattedSize)
                    .font(.subheadline.bold())
                    .foregroundStyle(PantheonTheme.gold)
                Text(finding.severity)
                    .font(.caption2)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(PantheonTheme.severityColor(finding.severity).opacity(0.2))
                    .foregroundStyle(PantheonTheme.severityColor(finding.severity))
                    .clipShape(Capsule())
            }
        }
        .padding()
        .background(PantheonTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: 8))
    }
}
