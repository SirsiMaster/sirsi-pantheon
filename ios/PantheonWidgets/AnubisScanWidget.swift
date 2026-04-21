import WidgetKit
import SwiftUI

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

    static let empty = AnubisEntry(
        date: .now,
        findingCount: 0,
        totalSize: "---",
        topFindings: [],
        rulesRan: 0,
        isStale: true
    )
}

// MARK: - Timeline Provider

struct AnubisTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> AnubisEntry {
        .placeholder
    }

    func getSnapshot(in context: Context, completion: @escaping (AnubisEntry) -> Void) {
        completion(fetchFromSharedStorage() ?? .placeholder)
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<AnubisEntry>) -> Void) {
        let entry = fetchFromSharedStorage() ?? .empty
        let nextUpdate = Calendar.current.date(byAdding: .minute, value: 30, to: entry.date)!
        completion(Timeline(entries: [entry], policy: .after(nextUpdate)))
    }

    /// Read cached scan results from the App Group shared container.
    private func fetchFromSharedStorage() -> AnubisEntry? {
        guard let snapshot = SharedDataManager.loadScanSnapshot() else { return nil }
        let totalSize = ByteCountFormatter.string(fromByteCount: snapshot.totalSize, countStyle: .file)
        let top = snapshot.topFindings.map { f in
            (f.name, ByteCountFormatter.string(fromByteCount: f.size, countStyle: .file))
        }
        return AnubisEntry(
            date: snapshot.date ?? .now,
            findingCount: snapshot.findingCount,
            totalSize: totalSize,
            topFindings: top,
            rulesRan: snapshot.rulesRan,
            isStale: snapshot.isStale
        )
    }
}

// MARK: - Widget View

struct AnubisWidgetView: View {
    var entry: AnubisEntry
    @Environment(\.widgetFamily) var family

    var body: some View {
        Group {
            switch family {
            case .systemSmall:
                smallView
            case .accessoryCircular:
                circularView
            case .accessoryRectangular:
                rectangularView
            default:
                mediumView
            }
        }
        .widgetURL(URL(string: "sirsi://anubis"))
    }

    // MARK: - Lock Screen Widgets

    private var circularView: some View {
        ZStack {
            AccessoryWidgetBackground()
            VStack(spacing: 2) {
                Image(systemName: "magnifyingglass.circle.fill")
                    .font(.title3)
                Text("\(entry.findingCount)")
                    .font(.caption.bold())
            }
        }
        .containerBackground(.clear, for: .widget)
    }

    private var rectangularView: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Image(systemName: "magnifyingglass.circle.fill")
                    .font(.caption)
                Text("Anubis")
                    .font(.caption.bold())
            }
            Text(entry.totalSize)
                .font(.headline)
            Text("\(entry.findingCount) findings")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .containerBackground(.clear, for: .widget)
    }

    // MARK: - Home Screen Widgets

    private var smallView: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 4) {
                Text("\u{13062}")
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
                    Text("\u{13062}")
                    Text("Anubis Scan")
                        .font(.caption.bold())
                        .foregroundStyle(gold)
                }

                Text(entry.totalSize)
                    .font(.title.bold())
                    .foregroundStyle(gold)

                Text("\(entry.findingCount) findings \u{00B7} \(entry.rulesRan) rules")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                if entry.isStale {
                    HStack(spacing: 4) {
                        Image(systemName: "arrow.clockwise")
                            .font(.caption2)
                        Text("Open app to rescan")
                            .font(.caption2)
                    }
                    .foregroundStyle(gold.opacity(0.7))
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
        .configurationDisplayName("\u{13062} Anubis Scan")
        .description("Infrastructure waste summary \u{2014} reclaimable storage.")
        .supportedFamilies([
            .systemSmall, .systemMedium,
            .accessoryCircular, .accessoryRectangular
        ])
    }
}
