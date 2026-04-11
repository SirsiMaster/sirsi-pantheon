import WidgetKit
import SwiftUI
import PantheonCore

// MARK: - Timeline Entry

struct AnubisEntry: TimelineEntry {
    let date: Date
    let findingCount: Int
    let totalSize: String
    let topFindings: [(name: String, size: String)]
    let rulesRan: Int
    let isStale: Bool

    static let placeholder = AnubisEntry(
        date: .now,
        findingCount: 12,
        totalSize: "3.4 GB",
        topFindings: [
            ("Xcode DerivedData", "1.2 GB"),
            ("npm caches", "890 MB"),
            ("Docker images", "1.3 GB"),
        ],
        rulesRan: 58,
        isStale: false
    )
}

// MARK: - Timeline Provider

struct AnubisTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> AnubisEntry {
        .placeholder
    }

    func getSnapshot(in context: Context, completion: @escaping (AnubisEntry) -> Void) {
        completion(.placeholder)
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<AnubisEntry>) -> Void) {
        let entry = fetchScanEntry()
        // Refresh every 30 minutes.
        let nextUpdate = Calendar.current.date(byAdding: .minute, value: 30, to: entry.date)!
        completion(Timeline(entries: [entry], policy: .after(nextUpdate)))
    }

    private func fetchScanEntry() -> AnubisEntry {
        let json = MobileAnubisScan("", "")
        guard let data = json.data(using: .utf8),
              let response = try? JSONDecoder().decode(BridgeEnvelope<ScanData>.self, from: data),
              response.ok, let scan = response.data else {
            return AnubisEntry(
                date: .now, findingCount: 0, totalSize: "—",
                topFindings: [], rulesRan: 0, isStale: true
            )
        }

        let totalSize = ByteCountFormatter.string(fromByteCount: scan.totalSize, countStyle: .file)

        let sorted = scan.findings.sorted { $0.sizeBytes > $1.sizeBytes }
        let top = sorted.prefix(3).map { f in
            (f.description, ByteCountFormatter.string(fromByteCount: f.sizeBytes, countStyle: .file))
        }

        return AnubisEntry(
            date: .now,
            findingCount: scan.findings.count,
            totalSize: totalSize,
            topFindings: top,
            rulesRan: scan.rulesRan,
            isStale: false
        )
    }
}

// MARK: - Widget View

struct AnubisWidgetView: View {
    var entry: AnubisEntry
    @Environment(\.widgetFamily) var family

    var body: some View {
        switch family {
        case .systemSmall:
            smallView
        default:
            mediumView
        }
    }

    private var smallView: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 4) {
                Text("𓁢")
                    .font(.caption)
                Text("Anubis")
                    .font(.caption.bold())
                    .foregroundStyle(gold)
            }

            Spacer()

            Text(entry.totalSize)
                .font(.title2.bold())
                .foregroundStyle(gold)

            Text("reclaimable")
                .font(.caption2)
                .foregroundStyle(.secondary)

            Text("\(entry.findingCount) findings")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .padding()
        .containerBackground(darkBg, for: .widget)
    }

    private var mediumView: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 4) {
                    Text("𓁢")
                    Text("Anubis Scan")
                        .font(.caption.bold())
                        .foregroundStyle(gold)
                }

                Text(entry.totalSize)
                    .font(.title.bold())
                    .foregroundStyle(gold)

                Text("\(entry.findingCount) findings · \(entry.rulesRan) rules")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                if entry.isStale {
                    Text("Tap to refresh")
                        .font(.caption2)
                        .foregroundStyle(.orange)
                }
            }

            if !entry.topFindings.isEmpty {
                Divider()
                    .background(gold.opacity(0.3))

                VStack(alignment: .leading, spacing: 4) {
                    Text("Top waste")
                        .font(.caption2)
                        .foregroundStyle(.secondary)

                    ForEach(entry.topFindings.indices, id: \.self) { i in
                        HStack {
                            Text(entry.topFindings[i].name)
                                .font(.caption)
                                .lineLimit(1)
                            Spacer()
                            Text(entry.topFindings[i].size)
                                .font(.caption.bold())
                                .foregroundStyle(gold)
                        }
                    }
                }
            }
        }
        .padding()
        .containerBackground(darkBg, for: .widget)
    }

    private var gold: Color { Color(red: 0.78, green: 0.66, blue: 0.32) }
    private var darkBg: Color { Color(red: 0.06, green: 0.06, blue: 0.06) }
}

// MARK: - Widget Definition

struct AnubisScanWidget: Widget {
    let kind = "ai.sirsi.pantheon.anubis"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: AnubisTimelineProvider()) { entry in
            AnubisWidgetView(entry: entry)
        }
        .configurationDisplayName("𓁢 Anubis Scan")
        .description("Infrastructure waste summary — reclaimable storage.")
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}

// MARK: - Decodable helpers

private struct BridgeEnvelope<T: Decodable>: Decodable {
    let ok: Bool
    let data: T?
    let error: String?
}

private struct ScanData: Decodable {
    let findings: [FindingData]
    let totalSize: Int64
    let rulesRan: Int

    enum CodingKeys: String, CodingKey {
        case findings
        case totalSize = "TotalSize"
        case rulesRan = "RulesRan"
    }
}

private struct FindingData: Decodable {
    let description: String
    let sizeBytes: Int64

    enum CodingKeys: String, CodingKey {
        case description = "Description"
        case sizeBytes = "SizeBytes"
    }
}
