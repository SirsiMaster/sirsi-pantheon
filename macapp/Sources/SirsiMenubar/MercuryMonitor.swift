import Foundation
import SwiftUI

// Consume Pantheon's existing Mercury v1 reader. Its CLI command owns the
// bounded private GET, schema validation, units, and freshness projection.
@MainActor final class MercuryMonitor: ObservableObject {
    @Published var projection: MercuryMonitorProjection?
    @Published var error: String?
    @Published var reading = false

    init(preloaded: MercuryMonitorProjection? = nil) { projection = preloaded }

    func refresh() async {
        guard !reading else { return }
        reading = true
        defer { reading = false }
        do {
            let read = await SirsiEngine.runJSONRead(args: ["mercury", "--json"], timeoutSeconds: 8)
            guard !Task.isCancelled else { return }
            projection = try decodeMercuryMonitor(read: read)
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

    func gpuWork(for node: MercuryMonitorNode, readSucceeded: Bool) -> Double? {
        guard readSucceeded, state == "ok", node.state == "live" else { return nil }
        return validPercent(node.gpu.work_pct)
    }

    func cpuBusy(for node: MercuryMonitorNode, readSucceeded: Bool) -> Double? {
        guard readSucceeded, state == "ok", node.state == "live" else { return nil }
        return validPercent(node.cpu.busy_pct)
    }

    private func validPercent(_ value: Double?) -> Double? {
        guard let value, value.isFinite, (0...100).contains(value) else { return nil }
        return value
    }
}

struct MercuryMonitorDashboard: Decodable {
    let nodes: [MercuryMonitorNode]
}

struct MercuryMonitorNode: Decodable {
    let id: String
    let name: String
    let chip: String?
    let state: String
    let observed_at: Double?
    let gpu: MercuryMonitorGPU
    let cpu: MercuryMonitorCPU
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
                Text("\(projection.state.uppercased()) · \(projection.message)")
                    .sirsiFont(11).foregroundStyle(projection.state == "ok" ? PantheonTheme.mutedText : gold)
                if let snapshot = projection.snapshot, snapshot.nodes.isEmpty {
                    Text("Mercury returned no node observations. Refresh or inspect monitor connectivity.")
                        .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                } else {
                    dashboardRows(projection)
                }
                Text("Source: Mercury dashboard v1 · read every 5 seconds while open")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            } else if monitor.error == nil {
                Text("Reading Mercury’s existing local monitor…").sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
            }
        }
        .padding(16).frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.panel, in: RoundedRectangle(cornerRadius: 10))
    }

    private func dashboardRows(_ projection: MercuryMonitorProjection) -> some View {
        if let dashboard = projection.snapshot {
            ForEach(dashboard.nodes.sorted { $0.id < $1.id }, id: \.id) { node in
                nodeRow(node, projection: projection)
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
                                Text(direction.rate).sirsiFont(12).monospacedDigit()
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

    private func nodeRow(_ node: MercuryMonitorNode, projection: MercuryMonitorProjection) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            HStack {
                Text(node.name).sirsiFont(14, weight: .semibold)
                Text(node.chip ?? "Chip not reported").sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                Spacer()
                Text(node.observed_at.map { "Sample \(Date(timeIntervalSince1970: $0).formatted(date: .omitted, time: .standard))" } ?? "No sample")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            }
            HStack(spacing: 24) {
                measurement("CPU busy", percent(projection.cpuBusy(for: node, readSucceeded: monitor.error == nil)))
                measurement("GPU work", percent(projection.gpuWork(for: node, readSucceeded: monitor.error == nil)))
                measurement("Node state", node.state)
            }
            if projection.state != "ok" || monitor.error != nil || node.state != "live" {
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
