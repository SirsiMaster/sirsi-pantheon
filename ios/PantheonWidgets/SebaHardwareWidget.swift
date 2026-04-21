import WidgetKit
import SwiftUI

// MARK: - Timeline Entry

struct SebaEntry: TimelineEntry {
    let date: Date
    let cpuModel: String
    let cpuCores: Int
    let cpuArch: String
    let ram: String
    let hasNeuralEngine: Bool
    let hasMetal: Bool
    let gpuName: String?

    static let placeholder = SebaEntry(
        date: .now,
        cpuModel: "Apple A17 Pro",
        cpuCores: 6,
        cpuArch: "arm64",
        ram: "8 GB",
        hasNeuralEngine: true,
        hasMetal: true,
        gpuName: "Apple GPU"
    )

    static let empty = SebaEntry(
        date: .now,
        cpuModel: "---",
        cpuCores: 0,
        cpuArch: "---",
        ram: "---",
        hasNeuralEngine: false,
        hasMetal: false,
        gpuName: nil
    )
}

// MARK: - Timeline Provider

struct SebaTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> SebaEntry {
        .placeholder
    }

    func getSnapshot(in context: Context, completion: @escaping (SebaEntry) -> Void) {
        completion(fetchFromSharedStorage() ?? .placeholder)
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<SebaEntry>) -> Void) {
        let entry = fetchFromSharedStorage() ?? .empty
        // Hardware doesn't change -- refresh once per day.
        let nextUpdate = Calendar.current.date(byAdding: .hour, value: 24, to: entry.date)!
        completion(Timeline(entries: [entry], policy: .after(nextUpdate)))
    }

    /// Read cached hardware data from the App Group shared container.
    private func fetchFromSharedStorage() -> SebaEntry? {
        guard let json = SharedDataManager.loadHardwareJSON(),
              let data = json.data(using: .utf8),
              let response = try? JSONDecoder().decode(BridgeEnvelope<HWData>.self, from: data),
              response.ok, let hw = response.data else {
            return nil
        }
        let ram = ByteCountFormatter.string(fromByteCount: hw.totalRam, countStyle: .memory)
        return SebaEntry(
            date: .now,
            cpuModel: hw.cpuModel,
            cpuCores: hw.cpuCores,
            cpuArch: hw.cpuArch,
            ram: ram,
            hasNeuralEngine: hw.neuralEngine,
            hasMetal: true,
            gpuName: hw.gpu?.name
        )
    }
}

// MARK: - Widget View

struct SebaWidgetView: View {
    var entry: SebaEntry
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
        .widgetURL(URL(string: "sirsi://seba"))
    }

    // MARK: - Lock Screen Widgets

    private var circularView: some View {
        ZStack {
            AccessoryWidgetBackground()
            VStack(spacing: 2) {
                Image(systemName: "cpu.fill")
                    .font(.title3)
                Text("\(entry.cpuCores)")
                    .font(.caption.bold())
            }
        }
        .containerBackground(.clear, for: .widget)
    }

    private var rectangularView: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Image(systemName: "cpu.fill")
                    .font(.caption)
                Text("Seba")
                    .font(.caption.bold())
            }
            Text(entry.cpuModel)
                .font(.caption)
                .lineLimit(1)
            HStack(spacing: 8) {
                Text("\(entry.cpuCores) cores")
                Text(entry.ram)
            }
            .font(.caption2)
            .foregroundStyle(.secondary)
        }
        .containerBackground(.clear, for: .widget)
    }

    // MARK: - Home Screen Widgets

    private var smallView: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 4) {
                Text("\u{131FD}")
                    .font(.caption)
                Text("Seba")
                    .font(.caption.bold())
                    .foregroundStyle(gold)
            }

            Text(entry.cpuModel)
                .font(.system(size: 11, weight: .medium))
                .lineLimit(2)

            Spacer()

            HStack(spacing: 8) {
                Label("\(entry.cpuCores)", systemImage: "cpu")
                    .font(.caption2)
                Label(entry.ram, systemImage: "memorychip")
                    .font(.caption2)
            }
            .foregroundStyle(.secondary)

            HStack(spacing: 6) {
                if entry.hasNeuralEngine {
                    SebaBadge(text: "ANE", color: green)
                }
                if entry.hasMetal {
                    SebaBadge(text: "Metal", color: gold)
                }
            }
        }
        .padding()
        .containerBackground(darkBg, for: .widget)
    }

    private var mediumView: some View {
        HStack(spacing: 16) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 4) {
                    Text("\u{131FD}")
                    Text("Seba \u{2014} Hardware Profile")
                        .font(.caption.bold())
                        .foregroundStyle(gold)
                }

                Text(entry.cpuModel)
                    .font(.subheadline.bold())

                if let gpu = entry.gpuName {
                    Text(gpu)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            Spacer()

            VStack(alignment: .trailing, spacing: 6) {
                SebaStatRow(icon: "cpu", label: "Cores", value: "\(entry.cpuCores)")
                SebaStatRow(icon: "memorychip", label: "RAM", value: entry.ram)
                SebaStatRow(icon: "brain", label: "ANE", value: entry.hasNeuralEngine ? "Yes" : "No")
            }
        }
        .padding()
        .containerBackground(darkBg, for: .widget)
    }

    private var gold: Color { Color(red: 0.78, green: 0.66, blue: 0.32) }
    private var green: Color { Color(red: 0.30, green: 0.69, blue: 0.31) }
    private var darkBg: Color { Color(red: 0.06, green: 0.06, blue: 0.06) }
}

// MARK: - Helper Views

struct SebaBadge: View {
    let text: String
    let color: Color

    var body: some View {
        Text(text)
            .font(.system(size: 9, weight: .bold))
            .padding(.horizontal, 5)
            .padding(.vertical, 2)
            .background(color.opacity(0.2))
            .foregroundStyle(color)
            .clipShape(Capsule())
    }
}

struct SebaStatRow: View {
    let icon: String
    let label: String
    let value: String

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: icon)
                .font(.caption2)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.caption.bold())
        }
    }
}

// MARK: - Widget Definition

struct SebaHardwareWidget: Widget {
    let kind = "ai.sirsi.pantheon.seba"

    var body: some WidgetConfiguration {
        StaticConfiguration(kind: kind, provider: SebaTimelineProvider()) { entry in
            SebaWidgetView(entry: entry)
        }
        .configurationDisplayName("\u{131FD} Seba Hardware")
        .description("Device hardware profile \u{2014} CPU, GPU, Neural Engine.")
        .supportedFamilies([
            .systemSmall, .systemMedium,
            .accessoryCircular, .accessoryRectangular
        ])
    }
}

// MARK: - Decodable helpers (widget can't import main app target)

private struct BridgeEnvelope<T: Decodable>: Decodable {
    let ok: Bool
    let data: T?
    let error: String?
}

private struct HWData: Decodable {
    let cpuCores: Int
    let cpuModel: String
    let cpuArch: String
    let totalRam: Int64
    let neuralEngine: Bool
    let gpu: GPUData?

    enum CodingKeys: String, CodingKey {
        case cpuCores = "cpu_cores"
        case cpuModel = "cpu_model"
        case cpuArch = "cpu_arch"
        case totalRam = "total_ram"
        case neuralEngine = "neural_engine"
        case gpu
    }
}

private struct GPUData: Decodable {
    let name: String
    let type: String
}
