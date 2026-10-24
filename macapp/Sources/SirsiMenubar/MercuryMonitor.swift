import Foundation
import SwiftUI

// Consume Pantheon's existing Mercury v1 reader. Its CLI command owns the
// bounded private GET, schema validation, units, and freshness projection.
@MainActor final class MercuryMonitor: ObservableObject {
    @Published var projection: MercuryMonitorProjection?
    @Published var error: String?
    @Published var reading = false

    private var readCompletedMonotonic: ContinuousClock.Instant?
    private(set) var readCompletedAt: Double?

    init(preloaded: MercuryMonitorProjection? = nil) {
        projection = preloaded
        if preloaded != nil {
            readCompletedMonotonic = ContinuousClock().now
            readCompletedAt = Date().timeIntervalSince1970
        }
    }

    func elapsedSinceRead(at now: ContinuousClock.Instant = ContinuousClock().now) -> Double? {
        guard let readCompletedMonotonic else { return nil }
        let elapsed = readCompletedMonotonic.duration(to: now).components
        return max(0, Double(elapsed.seconds) + Double(elapsed.attoseconds) / 1_000_000_000_000_000_000)
    }

    func refresh() async {
        guard !reading else { return }
        reading = true
        defer { reading = false }
        do {
            let read = await SirsiEngine.runJSONRead(args: ["mercury", "--json"], timeoutSeconds: 8)
            let completedAt = Date().timeIntervalSince1970
            let completedMonotonic = ContinuousClock().now
            guard !Task.isCancelled else { return }
            projection = try decodeMercuryMonitor(read: read)
            readCompletedAt = completedAt
            readCompletedMonotonic = completedMonotonic
            error = nil
        } catch {
            guard !Task.isCancelled else { return }
            self.error = "The canonical Mercury dashboard read did not complete successfully. Refresh to retry."
        }
    }
}

func decodeMercuryMonitor(read: SirsiEngine.JSONCommandRead) throws -> MercuryMonitorProjection {
    guard mercuryReadSucceeded(read) else { throw MercuryWorkspaceDecodeError.unsuccessfulRead }
    return try JSONDecoder().decode(MercuryMonitorProjection.self, from: read.data)
}

struct MercuryMonitorProjection: Decodable {
    let state: String
    let message: String
    let snapshot: MercuryMonitorDashboard?
    let scene: MercuryMonitorScene?

    func gpuWork(for node: MercuryMonitorNode, readSucceeded: Bool, readCompletedAt: Double?, elapsed: Double?) -> Double? {
        guard readSucceeded, isCurrent(node, readCompletedAt: readCompletedAt, elapsed: elapsed) else { return nil }
        return validPercent(node.gpu.work_pct)
    }

    func cpuBusy(for node: MercuryMonitorNode, readSucceeded: Bool, readCompletedAt: Double?, elapsed: Double?) -> Double? {
        guard readSucceeded, isCurrent(node, readCompletedAt: readCompletedAt, elapsed: elapsed) else { return nil }
        return validPercent(node.cpu.busy_pct)
    }

    func isCurrent(_ node: MercuryMonitorNode, readCompletedAt: Double?, elapsed: Double?) -> Bool {
        guard state == "ok", node.state == "live",
              let completedAt = finite(readCompletedAt),
              let elapsed = finite(elapsed), elapsed >= 0,
              let dashboard = snapshot,
              let generatedAt = finite(dashboard.generated_at),
              let ageAtGeneration = finite(node.age_s), ageAtGeneration >= -2,
              let observedAt = finite(node.observed_at),
              let freshness = finite(node.freshness_s), freshness > 0 else { return false }

        let envelopeAge = completedAt - generatedAt
        let envelopeWindow = dashboard.envelopeFreshnessWindow
        guard envelopeAge >= -2, envelopeAge <= envelopeWindow,
              envelopeAge + elapsed <= envelopeWindow else { return false }

        let producerAge = ageAtGeneration + max(0, envelopeAge)
        let timestampAge = completedAt - observedAt
        return timestampAge >= -2 && max(producerAge, timestampAge) + elapsed <= freshness
    }

    func currentRate(for sceneDirection: MercuryMonitorDirection, cable: MercuryMonitorCable, readSucceeded: Bool,
                     readCompletedAt: Double?, elapsed: Double?) -> String {
        guard readSucceeded, state == "ok", cable.state == "up", let dashboard = snapshot,
              let sourceLink = dashboard.links.first(where: { $0.id == cable.id && $0.state == "up" }),
              let rawDirection = sourceLink.directions.first(where: {
                  $0.from == sceneDirection.from && $0.to == sceneDirection.to
              }),
              let observedAt = finite(rawDirection.observed_at),
              let completedAt = finite(readCompletedAt),
              let elapsed = finite(elapsed), elapsed >= 0,
              let wireRate = finite(rawDirection.wire_gbs),
              isCurrentNode(id: sceneDirection.from, completedAt: completedAt, elapsed: elapsed),
              isCurrentNode(id: sceneDirection.to, completedAt: completedAt, elapsed: elapsed) else { return "Unknown" }

        let age = completedAt - observedAt
        guard wireRate >= 0, age >= -2, age + elapsed <= 6,
              sceneDirection.rate != "— MB/s" else { return "Unknown" }
        return sceneDirection.rate
    }

    func envelopeIsCurrent(readCompletedAt: Double?, elapsed: Double?) -> Bool {
        guard state == "ok", let dashboard = snapshot,
              let completedAt = finite(readCompletedAt),
              let elapsed = finite(elapsed), elapsed >= 0,
              let generatedAt = finite(dashboard.generated_at) else { return false }
        let age = completedAt - generatedAt
        let window = dashboard.envelopeFreshnessWindow
        return age >= -2 && age <= window && age + elapsed <= window
    }

    private func isCurrentNode(id: String, completedAt: Double, elapsed: Double) -> Bool {
        guard let node = snapshot?.nodes.first(where: { $0.id == id }) else { return false }
        return isCurrent(node, readCompletedAt: completedAt, elapsed: elapsed)
    }

    private func finite(_ value: Double?) -> Double? {
        guard let value, value.isFinite else { return nil }
        return value
    }

    private func validPercent(_ value: Double?) -> Double? {
        guard let value, value.isFinite, (0...100).contains(value) else { return nil }
        return value
    }
}

struct MercuryMonitorDashboard: Decodable {
    let generated_at: Double?
    let nodes: [MercuryMonitorNode]
    let links: [MercuryMonitorLink]

    private enum CodingKeys: String, CodingKey { case generated_at, nodes, links }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        generated_at = try container.decodeIfPresent(Double.self, forKey: .generated_at)
        nodes = try container.decode([MercuryMonitorNode].self, forKey: .nodes)
        links = try container.decodeIfPresent([MercuryMonitorLink].self, forKey: .links) ?? []
    }

    var envelopeFreshnessWindow: Double {
        nodes.lazy.filter { $0.state == "live" }.map(\.freshness_s).compactMap { $0 }
            .filter { $0.isFinite && $0 > 0 && $0 < 6 }
            .min() ?? 6
    }
}

struct MercuryMonitorNode: Decodable {
    let id: String
    let name: String
    let chip: String?
    let state: String
    let observed_at: Double?
    let age_s: Double?
    let freshness_s: Double?
    let gpu: MercuryMonitorGPU
    let cpu: MercuryMonitorCPU
}

struct MercuryMonitorLink: Decodable {
    let id: String
    let state: String
    let directions: [MercuryMonitorRawDirection]
}

struct MercuryMonitorRawDirection: Decodable {
    let from: String
    let to: String
    let wire_gbs: Double?
    let observed_at: Double?
}

struct MercuryMonitorGPU: Decodable {
    let work_pct: Double?
}

struct MercuryMonitorCPU: Decodable {
    let busy_pct: Double?
}

struct MercuryMonitorScene: Decodable {
    let cables: [MercuryMonitorCable]
}

struct MercuryMonitorCable: Decodable {
    let id: String
    let state: String
    let iface: String
    let directions: [MercuryMonitorDirection]
}

struct MercuryMonitorDirection: Decodable {
    let from: String
    let to: String
    let rate: String
}

struct MercuryLiveMachinesView: View {
    @ObservedObject var monitor: MercuryMonitor
    var body: some View {
        TimelineView(.periodic(from: .now, by: 1)) { _ in
          VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Mercury dashboard").sirsiFont(15, weight: .semibold)
                Spacer()
                if monitor.reading { ProgressView().controlSize(.small) }
            }
            if let error = monitor.error {
                Text(error).sirsiFont(12).foregroundStyle(gold).fixedSize(horizontal: false, vertical: true)
                if monitor.projection != nil {
                    Text("Showing the last retained response below.").sirsiFont(11).foregroundStyle(gold)
                }
            }
            if let projection = monitor.projection {
                let elapsed = monitor.elapsedSinceRead()
                let current = monitor.error == nil && projection.envelopeIsCurrent(readCompletedAt: monitor.readCompletedAt, elapsed: elapsed)
                Text("\(current ? projection.state.uppercased() : "STALE") · \(projection.message)")
                    .sirsiFont(11).foregroundStyle(current ? PantheonTheme.mutedText : gold)
                if let snapshot = projection.snapshot, snapshot.nodes.isEmpty {
                    Text("Mercury returned no node observations. Refresh or inspect monitor connectivity.")
                        .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                } else {
                    dashboardRows(projection, elapsed: elapsed)
                }
                Text("Source: Mercury dashboard v1 · read every 5 seconds while open")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            } else if monitor.error == nil {
                Text("Reading Mercury’s existing local monitor…").sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
            }
          }
        }
        .padding(16).frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.panel, in: RoundedRectangle(cornerRadius: 10))
    }

    @ViewBuilder
    private func dashboardRows(_ projection: MercuryMonitorProjection, elapsed: Double?) -> some View {
        if let dashboard = projection.snapshot {
            ForEach(dashboard.nodes.sorted { $0.id < $1.id }, id: \.id) { node in
                nodeRow(node, projection: projection, elapsed: elapsed)
            }
            if let scene = projection.scene, !scene.cables.isEmpty {
                VStack(alignment: .leading, spacing: 7) {
                    Text("Observed link directions").sirsiFont(12, weight: .semibold)
                    ForEach(scene.cables, id: \.id) { cable in
                        ForEach(cable.directions.indices, id: \.self) { index in
                            let direction = cable.directions[index]
                            HStack {
                                Text("\(direction.from) → \(direction.to)")
                                    .sirsiFont(11, design: .monospaced)
                                Spacer()
                                Text(projection.currentRate(for: direction, cable: cable,
                                                            readSucceeded: monitor.error == nil,
                                                            readCompletedAt: monitor.readCompletedAt, elapsed: elapsed))
                                    .sirsiFont(12).monospacedDigit()
                            }
                            .foregroundStyle(cable.state == "up" ? PantheonTheme.mutedText : gold)
                        }
                    }
                }.padding(.top, 8)
            }
        } else {
            Text(projection.message).sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
        }
    }

    @ViewBuilder
    private func nodeRow(_ node: MercuryMonitorNode, projection: MercuryMonitorProjection, elapsed: Double?) -> some View {
        let current = monitor.error == nil && projection.isCurrent(node, readCompletedAt: monitor.readCompletedAt, elapsed: elapsed)
        VStack(alignment: .leading, spacing: 9) {
            HStack {
                Text(node.name).sirsiFont(14, weight: .semibold)
                Text(node.chip ?? "Chip not reported").sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                Spacer()
                Text(node.observed_at.map { "Sample \(Date(timeIntervalSince1970: $0).formatted(date: .omitted, time: .standard))" } ?? "No sample")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            }
            HStack(spacing: 24) {
                measurement("CPU busy", percent(projection.cpuBusy(for: node, readSucceeded: monitor.error == nil,
                                                                    readCompletedAt: monitor.readCompletedAt, elapsed: elapsed)))
                measurement("GPU work", percent(projection.gpuWork(for: node, readSucceeded: monitor.error == nil,
                                                                    readCompletedAt: monitor.readCompletedAt, elapsed: elapsed)))
                measurement("Power", "Unknown")
                measurement("Node state", current ? node.state : "stale locally")
            }
            if !current || monitor.error != nil {
                Text("Current measurements are unknown until the canonical reader reports a fresh live observation.")
                    .sirsiFont(11).foregroundStyle(gold)
            }
        }.padding(.vertical, 10)
    }

    private func measurement(_ name: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(name).sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            Text(value).sirsiFont(16, weight: .medium).monospacedDigit()
        }.frame(maxWidth: .infinity, alignment: .leading)
    }
    private func percent(_ value: Double?) -> String {
        guard let value, value.isFinite, (0...100).contains(value) else { return "Unknown" }
        return String(format: "%.1f%%", value)
    }
}
