import WidgetKit
import SwiftUI

// MARK: - Timeline Entry

struct GuardEntry: TimelineEntry {
    let date: Date
    let totalRAM: String
    let usedRAM: String
    let ramPercent: Double
    let processCount: Int
    let topProcesses: [(name: String, ram: String)]
    let isStale: Bool

    static let placeholder = GuardEntry(
        date: .now,
        totalRAM: "8 GB",
        usedRAM: "5.2 GB",
        ramPercent: 0.65,
        processCount: 142,
        topProcesses: [
            ("Xcode", "1.8 GB"),
            ("Chrome", "1.2 GB"),
            ("Pantheon", "340 MB"),
        ],
        isStale: false
    )

    static let empty = GuardEntry(
        date: .now,
        totalRAM: "---",
        usedRAM: "---",
        ramPercent: 0,
        processCount: 0,
        topProcesses: [],
        isStale: true
    )
}

// MARK: - Timeline Provider

struct GuardTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> GuardEntry {
        .placeholder
    }

    func getSnapshot(in context: Context, completion: @escaping (GuardEntry) -> Void) {
        completion(fetchFromSharedStorage() ?? .placeholder)
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<GuardEntry>) -> Void) {
        let entry = fetchFromSharedStorage() ?? .empty
        // Refresh every 30 minutes -- main app updates the data.
        let nextUpdate = Calendar.current.date(byAdding: .minute, value: 30, to: entry.date)!
        completion(Timeline(entries: [entry], policy: .after(nextUpdate)))
    }

    private func fetchFromSharedStorage() -> GuardEntry? {
        guard let snapshot = SharedDataManager.loadGuardSnapshot() else { return nil }

        let totalRAM = ByteCountFormatter.string(fromByteCount: snapshot.totalRAM, countStyle: .memory)
        let usedRAM = ByteCountFormatter.string(fromByteCount: snapshot.usedRAM, countStyle: .memory)
        let top = snapshot.topProcesses.map { ($0.name, $0.formattedRAM) }

        return GuardEntry(
            date: snapshot.lastUpdate ?? .now,
            totalRAM: totalRAM,
            usedRAM: usedRAM,
            ramPercent: snapshot.ramUsagePercent,
            processCount: snapshot.processCount,
            topProcesses: top,
            isStale: snapshot.isStale
        )
    }
}

// MARK: - Widget View

struct GuardWidgetView: View {
    var entry: GuardEntry
    @Environment(\.widgetFamily) var family

    var body: some View {
        Group {
            switch family {
            case .systemSmall:
                smallView
            case .systemMedium:
                mediumView
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

    // MARK: - Lock Screen: Circular (RAM Gauge)

    private var circularView: some View {
        ZStack {
            AccessoryWidgetBackground()
            Gauge(value: entry.ramPercent, in: 0...1) {
                Image(systemName: "memorychip")
            } currentValueLabel: {
                Text("\(Int(entry.ramPercent * 100))")
                    .font(.system(size: 12, weight: .bold))
            }
            .gaugeStyle(.accessoryCircular)
        }
        .containerBackground(.clear, for: .widget)
    }

    // MARK: - Lock Screen: Rectangular

    private var rectangularView: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Image(systemName: "shield.checkered")
                    .font(.caption)
                Text("Guard")
                    .font(.caption.bold())
            }
            Text(entry.usedRAM)
                .font(.headline)
            Text("\(entry.processCount) active")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .containerBackground(.clear, for: .widget)
    }

    // MARK: - Home Screen: Small

    private var smallView: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 4) {
                Image(systemName: "shield.checkered")
                    .font(.caption)
                    .foregroundStyle(gold)
                Text("Guard")
                    .font(.caption.bold())
                    .foregroundStyle(gold)
            }

            Spacer()

            // RAM gauge ring
            ZStack {
                Circle()
                    .stroke(gold.opacity(0.2), lineWidth: 5)
                    .frame(width: 48, height: 48)
                Circle()
                    .trim(from: 0, to: entry.ramPercent)
                    .stroke(ramColor, style: StrokeStyle(lineWidth: 5, lineCap: .round))
                    .frame(width: 48, height: 48)
                    .rotationEffect(.degrees(-90))
                Text("\(Int(entry.ramPercent * 100))%")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundStyle(ramColor)
            }
            .frame(maxWidth: .infinity)

            Spacer()

            Text(entry.usedRAM)
                .font(.caption.bold())
                .foregroundStyle(.white)
            Text("of \(entry.totalRAM)")
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .padding()
        .containerBackground(darkBg, for: .widget)
    }

    // MARK: - Home Screen: Medium

    private var mediumView: some View {
        HStack(spacing: 16) {
            // Left: summary
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 4) {
                    Image(systemName: "shield.checkered")
                        .foregroundStyle(gold)
                    Text("Guard \u{2014} Process Watchdog")
                        .font(.caption.bold())
                        .foregroundStyle(gold)
                }

                HStack(alignment: .firstTextBaseline, spacing: 4) {
                    Text(entry.usedRAM)
                        .font(.title2.bold())
                        .foregroundStyle(ramColor)
                    Text("/ \(entry.totalRAM)")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }

                Text("\(entry.processCount) active")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                if entry.isStale {
                    HStack(spacing: 4) {
                        Image(systemName: "arrow.clockwise")
                            .font(.caption2)
                        Text("Open app to refresh")
                            .font(.caption2)
                    }
                    .foregroundStyle(gold.opacity(0.7))
                }
            }

            if !entry.topProcesses.isEmpty {
                Divider()
                    .background(gold.opacity(0.3))

                // Right: top processes
                VStack(alignment: .leading, spacing: 4) {
                    Text("Top by RAM")
                        .font(.caption2)
                        .foregroundStyle(.secondary)

                    ForEach(entry.topProcesses.prefix(3).indices, id: \.self) { i in
                        HStack {
                            Text(entry.topProcesses[i].name)
                                .font(.caption)
                                .lineLimit(1)
                            Spacer()
                            Text(entry.topProcesses[i].ram)
                                .font(.caption.bold())
                                .foregroundStyle(gold)
                        }
                    }

                    if entry.topProcesses.isEmpty {
                        Text("No data yet")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
        .padding()
        .containerBackground(darkBg, for: .widget)
    }

    // MARK: - Theme

    private var gold: Color { Color(red: 0.78, green: 0.66, blue: 0.32) }
    private var darkBg: Color { Color(red: 0.06, green: 0.06, blue: 0.06) }

    /// RAM color shifts from green to gold to red based on pressure.
    private var ramColor: Color {
        if entry.ramPercent < 0.6 {
            return Color(red: 0.30, green: 0.69, blue: 0.31) // green
        } else if entry.ramPercent < 0.85 {
            return gold
        } else {
            return Color(red: 0.94, green: 0.33, blue: 0.31) // red
        }
    }
}

// MARK: - Widget Definition

struct GuardWidget: Widget {
    let kind = "ai.sirsi.pantheon.guard"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: GuardTimelineProvider()) { entry in
            GuardWidgetView(entry: entry)
        }
        .configurationDisplayName("Guard \u{2014} Process Watchdog")
        .description("RAM usage and top processes at a glance.")
        .supportedFamilies([
            .systemSmall, .systemMedium,
            .accessoryCircular, .accessoryRectangular
        ])
    }
}
