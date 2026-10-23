import Foundation
import SwiftUI

// Consume the existing Mercury monitor, as Apollo does. No sampler, model or
// service is started by this client. Only this fixed local GET is permitted.
@MainActor final class MercuryMonitor: ObservableObject {
    @Published var projection: MercuryMonitorProjection?
    @Published var error: String?
    @Published var reading = false
    static let endpoint = URL(string: "http://127.0.0.1:8099/api")!
    static let byteLimit = 2 * 1024 * 1024

    init(preloaded: MercuryMonitorProjection? = nil) { projection = preloaded }

    func refresh() async {
        guard !reading else { return }
        reading = true
        defer { reading = false }
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 4
        config.timeoutIntervalForResource = 6
        config.httpMaximumConnectionsPerHost = 1
        let session = URLSession(configuration: config, delegate: MercuryNoRedirect(), delegateQueue: nil)
        defer { session.invalidateAndCancel() }
        do {
            var request = URLRequest(url: Self.endpoint)
            request.cachePolicy = .reloadIgnoringLocalCacheData
            let (bytes, response) = try await session.bytes(for: request)
            guard let response = response as? HTTPURLResponse, response.statusCode == 200,
                  response.url == Self.endpoint else { throw MercuryMonitorFailure.invalidResponse }
            guard response.expectedContentLength <= Int64(Self.byteLimit) else { throw MercuryMonitorFailure.oversize }
            var data = Data()
            data.reserveCapacity(64 * 1024)
            for try await byte in bytes {
                guard data.count < Self.byteLimit else { throw MercuryMonitorFailure.oversize }
                data.append(byte)
            }
            let decoded = try JSONDecoder().decode(MercuryMonitorProjection.self, from: data)
            guard !Task.isCancelled else { return }
            projection = decoded
            error = nil
        } catch {
            guard !Task.isCancelled else { return }
            self.error = "Mercury’s local monitor could not be read. Refresh to retry; inspect Ra for machine connectivity. \(error.localizedDescription)"
        }
    }
}

private final class MercuryNoRedirect: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}

private enum MercuryMonitorFailure: LocalizedError {
    case invalidResponse, oversize
    var errorDescription: String? {
        switch self {
        case .invalidResponse: return "The fixed local monitor did not return a successful response."
        case .oversize: return "The monitor response exceeded the 2 MiB reading budget."
        }
    }
}

struct MercuryMonitorProjection: Decodable {
    let now: Double
    let local: String?
    let machines: [String: MercuryMonitorMachine]
}

struct MercuryMonitorMachine: Decodable {
    let state: String?
    let profile: MercuryMonitorProfile?
    let samples: [MercuryMonitorSample]?
    var latest: MercuryMonitorSample? { samples?.filter { $0.t.isFinite }.max { $0.t < $1.t } }

    func hasCurrentObservation(at now: Date, readSucceeded: Bool) -> Bool {
        readSucceeded && state == "live" && latest?.isFresh(at: now) == true
    }
}

struct MercuryMonitorProfile: Decodable {
    let chip: String?
    let model: String?
    let mem_gb: Double?
}

struct MercuryMonitorSample: Decodable {
    let t: Double
    let cores: [Double]?
    let chip: MercuryMonitorChip?
    let ifstat: [String: MercuryMonitorInterface]?

    func isFresh(at now: Date) -> Bool {
        let age = now.timeIntervalSince1970 - t
        return t.isFinite && age >= -2 && age <= 10
    }
    var meanCPU: Double? {
        guard let cores, !cores.isEmpty, cores.allSatisfy({ $0.isFinite && (0...100).contains($0) }) else { return nil }
        return cores.reduce(0, +) / Double(cores.count)
    }
}

struct MercuryMonitorChip: Decodable {
    let gpu_busy: Double?
    let cpu_mw: Double?
    let gpu_mw: Double?
}

struct MercuryMonitorInterface: Decodable {
    let rx_bps: Double?
    let tx_bps: Double?
}

struct MercuryLiveMachinesView: View {
    @ObservedObject var monitor: MercuryMonitor
    var isVisible = true
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Live machine telemetry").sirsiFont(15, weight: .semibold)
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
                if projection.machines.isEmpty {
                    Text("Mercury returned no machine observations. Inspect the fabric or refresh the monitor.")
                        .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                } else if isVisible {
                    // Age retained samples even when an in-flight read has not
                    // published a new model. This clock performs no network work.
                    TimelineView(.periodic(from: .now, by: 1)) { context in
                        machineRows(projection, at: context.date)
                    }
                } else {
                    machineRows(projection, at: Date())
                }
                Text("Source: Mercury local monitor · read every 5 seconds while open")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            } else if monitor.error == nil {
                Text("Reading Mercury’s existing local monitor…").sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
            }
        }
        .padding(16).frame(maxWidth: .infinity, alignment: .leading)
        .background(PantheonTheme.panel, in: RoundedRectangle(cornerRadius: 10))
    }

    private func machineRows(_ projection: MercuryMonitorProjection, at now: Date) -> some View {
        ForEach(projection.machines.keys.sorted(), id: \.self) { name in
            if let machine = projection.machines[name] { machineRow(name, machine, at: now) }
        }
    }

    private func machineRow(_ name: String, _ machine: MercuryMonitorMachine, at now: Date) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            HStack {
                Text(name).sirsiFont(14, weight: .semibold)
                Text(machine.profile?.chip ?? "Chip not reported").sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
                Spacer()
                Text(machine.latest.map { "Sample \(Date(timeIntervalSince1970: $0.t).formatted(date: .omitted, time: .standard))" } ?? "No sample")
                    .sirsiFont(11).foregroundStyle(PantheonTheme.mutedText)
            }
            if let sample = machine.latest {
                let current = machine.hasCurrentObservation(at: now, readSucceeded: monitor.error == nil)
                HStack(spacing: 24) {
                    measurement("CPU utilization", percent(sample.meanCPU))
                    measurement("GPU residency", percent(sample.chip?.gpu_busy))
                    measurement("CPU power", watts(sample.chip?.cpu_mw))
                    measurement("GPU power", watts(sample.chip?.gpu_mw))
                }
                .opacity(current ? 1 : 0.7)
                if !current {
                    Text("Retained sample · current utilization is unknown. Refresh or inspect connectivity in Ra.")
                        .sirsiFont(11).foregroundStyle(gold)
                }
                if let interfaces = sample.ifstat, !interfaces.isEmpty {
                    ForEach(interfaces.keys.sorted(), id: \.self) { name in
                        if let counters = interfaces[name] {
                            HStack {
                                Text(name).sirsiFont(11, design: .monospaced).frame(width: 70, alignment: .leading)
                                Text("↓ \(rate(counters.rx_bps))    ↑ \(rate(counters.tx_bps))")
                                    .sirsiFont(12).monospacedDigit()
                                Spacer()
                            }.foregroundStyle(PantheonTheme.mutedText)
                        }
                    }
                }
            } else {
                Text("No telemetry sample has been published for this machine.")
                    .sirsiFont(12).foregroundStyle(PantheonTheme.mutedText)
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
    private func watts(_ value: Double?) -> String {
        guard let value, value.isFinite, value >= 0 else { return "Unknown" }
        return String(format: "%.2f W", value / 1000)
    }
    private func rate(_ value: Double?) -> String {
        guard let value, value.isFinite, value >= 0, value < Double(Int64.max) else { return "Unknown" }
        return SirsiEngine.human(Int64(value)) + "/s"
    }
}
