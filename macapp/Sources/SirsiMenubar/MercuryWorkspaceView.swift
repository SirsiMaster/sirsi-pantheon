import SwiftUI
import Foundation

// Mercury does not yet publish a dedicated `sirsi mercury status` contract.
// This workspace reads only existing, canonical projections: Ra's lane ping
// and admitted-machine registry, plus the host's shared transport receipt.
// It presents those sources as separate evidence and never infers a Mercury
// machine assignment, rail reservation, or negotiated rate.
struct MercuryWorkspaceView: View {
    @ObservedObject var engine: SirsiEngine
    @StateObject private var monitor = MercuryMonitor()
    @Environment(\.openPantheonWorkspace) private var openWorkspace
    @Environment(\.snapshotMode) private var snapshotMode
    @State private var lane: MercuryLanePing?
    @State private var machines: [MercuryMachine]?
    @State private var links: MercuryTransportReceipt?
    @State private var laneError: String?
    @State private var machinesError: String?
    @State private var linksError: String?
    @State private var laneEvidence: String?
    @State private var machinesEvidence: String?
    @State private var linksEvidence: String?
    @State private var refreshedAt: Date?
    @State private var laneReadAt: Date?
    @State private var loading = true

    private let laneArgs = ["router", "ping", "mercury", "--json"]
    private let machineArgs = ["router", "machines", "--json"]
    private let transportArgs = ["hardware", "links", "--json"]

    init(
        engine: SirsiEngine,
        preloadedLane: MercuryLanePing? = nil,
        preloadedMachines: [MercuryMachine]? = nil,
        preloadedLinks: MercuryTransportReceipt? = nil,
        preloadedMonitor: MercuryMonitorProjection? = nil
    ) {
        self.engine = engine
        _monitor = StateObject(wrappedValue: MercuryMonitor(preloaded: preloadedMonitor))
        _lane = State(initialValue: preloadedLane)
        _machines = State(initialValue: preloadedMachines)
        _links = State(initialValue: preloadedLinks)
        _loading = State(initialValue: preloadedLane == nil && preloadedMachines == nil && preloadedLinks == nil)
    }

    var body: some View {
        VStack(spacing: 0) {
            BackBar(title: "Mercury")
            HStack(alignment: .center, spacing: 10) {
                VStack(alignment: .leading, spacing: 3) {
                    Text("Mercury workspace")
                        .sirsiFont(.title3, weight: .bold)
                        .foregroundStyle(Color.white)
                    Text("Read-only view of published lane, admitted machines, and this Mac’s transport receipt.")
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                }
                Spacer(minLength: 6)
                Button { Task { await refresh() } } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
                .buttonStyle(.bordered)
                .tint(PantheonTheme.gold)
                .disabled(loading)
            }
            .padding(16)
            .background(PantheonTheme.sidebar)

            if loading && lane == nil && machines == nil && links == nil {
                VStack(spacing: 10) {
                    ProgressView()
                    Text("Reading canonical Ra and transport evidence…")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(PantheonTheme.mutedText)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                MaybeScroll {
                    VStack(alignment: .leading, spacing: 12) {
                        MercuryLiveMachinesView(monitor: monitor)
                        truthNote
                        TimelineView(.periodic(from: .now, by: 5)) { context in
                            laneSection(at: context.date)
                        }
                        machinesSection
                        transportSection
                        railSection
                        routesSection
                    }
                    .padding(16)
                }
            }

            Divider().overlay(PantheonTheme.gold.opacity(0.18))
            HStack(spacing: 8) {
                Text(refreshedAt.map { "Read \($0.formatted(date: .omitted, time: .shortened))" } ?? "No successful read yet")
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.mutedText)
                Spacer()
                Text("Canonical readers · read only")
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.mutedText.opacity(0.75))
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 9)
        }
        .background(PantheonTheme.canvas)
        .task(id: engine.workspaceVisible) {
            guard !snapshotMode, engine.workspaceVisible else { return }
            await refresh()
            while !Task.isCancelled {
                do { try await Task.sleep(nanoseconds: 5_000_000_000) }
                catch { return }
                guard !Task.isCancelled else { return }
                await monitor.refresh()
            }
        }
        .navigationTitle("Mercury")
    }

    private var truthNote: some View {
        HStack(alignment: .top, spacing: 9) {
            Image(systemName: "info.circle.fill")
                .sirsiFont(.subheadline)
                .foregroundStyle(PantheonTheme.gold)
            VStack(alignment: .leading, spacing: 4) {
                Text("What this view shows")
                    .sirsiFont(.subheadline, weight: .semibold)
                    .foregroundStyle(Color.white)
                Text("Mercury’s monitor supplies machine and interface measurements. Ra supplies admission and lane availability; hardware receipts describe this Mac’s available links.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(13)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(PantheonTheme.panelRaised))
        .overlay(RoundedRectangle(cornerRadius: 12).stroke(PantheonTheme.gold.opacity(0.25), lineWidth: 1))
    }

    private func laneSection(at now: Date) -> some View {
        let current = laneError == nil && pantheonObservationIsCurrent(laneReadAt, at: now, maxAge: 30)
        return sectionCard(title: "Mercury lane", subtitle: "Last Ra lane observation · refresh to revalidate") {
            if let lane {
                if let laneError { staleReadNotice(laneError) }
                Text(laneReadAt.map { "Observed \($0.formatted(date: .omitted, time: .standard))" } ?? "Observation time unavailable")
                    .sirsiFont(.caption).foregroundStyle(PantheonTheme.mutedText)
                if !current {
                    Text("Retained verdict · current lane availability is unknown. Refresh to revalidate.")
                        .sirsiFont(.caption).foregroundStyle(PantheonTheme.gold)
                }
                HStack(alignment: .top, spacing: 10) {
                    statusMark(current && (lane.verdict == "LIVE" || lane.verdict == "WAKEABLE") ? PantheonTheme.emerald : PantheonTheme.gold)
                    VStack(alignment: .leading, spacing: 4) {
                        HStack {
                            Text(current ? lane.verdict : "Last verdict: \(lane.verdict)")
                                .sirsiFont(.headline, weight: .semibold)
                                .foregroundStyle(current && (lane.verdict == "LIVE" || lane.verdict == "WAKEABLE") ? PantheonTheme.emerald : PantheonTheme.gold)
                            Spacer()
                            if !lane.agent.isEmpty {
                                Text(lane.agent)
                                    .sirsiFont(.caption, design: .monospaced)
                                    .foregroundStyle(PantheonTheme.mutedText)
                            }
                        }
                        Text(lane.detail.isEmpty ? "Ra returned no detail for this lane." : lane.detail)
                            .sirsiFont(.subheadline)
                            .foregroundStyle(PantheonTheme.mutedText)
                            .fixedSize(horizontal: false, vertical: true)
                        if let thread = lane.thread, !thread.isEmpty {
                            keyValue("Published thread", thread)
                        }
                        if let reportedAt = lane.reportedAt, !reportedAt.isEmpty {
                            keyValue("Worker report", reportedAt)
                        }
                        if let summary = lane.reportSummary, !summary.isEmpty {
                            keyValue("Last report", summary)
                        }
                    }
                }
            } else {
                emptyOrError(laneError, message: "No lane availability was returned. This does not establish that Mercury is stopped or healthy.")
            }
            evidenceDisclosure(
                "Lane ping evidence",
                command: laneArgs,
                sourceContract: "The JSON result is a list of lane verdicts and published worker details. This view accepts only the record whose agent is exactly `mercury`.",
                payload: laneEvidence,
                error: laneError
            )
        }
    }

    private var machinesSection: some View {
        sectionCard(title: "Admitted machines", subtitle: "Macs registered with the fabric") {
            if let machines {
                if let machinesError { staleReadNotice(machinesError) }
                if machines.isEmpty {
                    Text("The canonical registry returned no admitted machines.")
                        .sirsiFont(.subheadline)
                        .foregroundStyle(PantheonTheme.gold)
                } else {
                    VStack(spacing: 0) {
                        ForEach(machines.indices, id: \.self) { index in
                            let machine = machines[index]
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(alignment: .firstTextBaseline) {
                                    Text(machine.id)
                                        .sirsiFont(.subheadline, weight: .semibold)
                                        .foregroundStyle(Color.white)
                                    Spacer()
                                    Text(machine.model)
                                        .sirsiFont(.caption)
                                        .foregroundStyle(PantheonTheme.mutedText)
                                }
                                HStack(spacing: 12) {
                                    if !machine.chip.isEmpty { fact("Chip", machine.chip) }
                                    if machine.memoryGiB > 0 { fact("Memory", "\(machine.memoryGiB) GiB") }
                                    if machine.monitorPort > 0 { fact("Monitor", "Port \(machine.monitorPort)") }
                                }
                                if !machine.tailscale.isEmpty { keyValue("Tailscale", machine.tailscale) }
                                if !machine.lan.isEmpty { keyValue("LAN at admission", machine.lan) }
                                if !machine.registeredAt.isEmpty { keyValue("Admitted", machine.registeredAt) }
                                if !machine.admittedBy.isEmpty { keyValue("Admitted by", machine.admittedBy) }
                            }
                            .padding(.vertical, 9)
                            if index < machines.count - 1 { Divider().overlay(PantheonTheme.mutedText.opacity(0.15)) }
                        }
                    }
                    Text("Registry membership proves admission only. The current readers do not bind Mercury to one of these machines.")
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.top, 5)
                }
            } else {
                emptyOrError(machinesError, message: "Machine admission data is unavailable; no Mercury machine assignment is inferred.")
            }
            evidenceDisclosure(
                "Machine registry evidence",
                command: machineArgs,
                sourceContract: "The registry returns admitted Mac records. Admission does not identify a Mercury machine assignment.",
                payload: machinesEvidence,
                error: machinesError
            )
        }
    }

    private var transportSection: some View {
        sectionCard(title: "Local transport receipt", subtitle: "Thunderbolt and RDMA observations from this Mac") {
            if let links {
                if let linksError { staleReadNotice(linksError) }
                HStack(spacing: 7) {
                    statusMark(links.portCountKnown ? PantheonTheme.emerald : PantheonTheme.gold)
                    Text(links.portCountKnown ? "Port inventory observed" : "Port inventory unknown")
                        .sirsiFont(.subheadline, weight: .semibold)
                        .foregroundStyle(links.portCountKnown ? PantheonTheme.emerald : PantheonTheme.gold)
                    Spacer()
                    Text(links.host.isEmpty ? "Host unknown" : links.host)
                        .sirsiFont(.caption, design: .monospaced)
                        .foregroundStyle(PantheonTheme.mutedText)
                }
                if !links.ports.isEmpty {
                    VStack(spacing: 0) {
                        ForEach(links.ports.indices, id: \.self) { index in
                            let port = links.ports[index]
                            VStack(alignment: .leading, spacing: 5) {
                                HStack {
                                    Text(port.receptacleID.isEmpty ? port.bus : "Port \(port.receptacleID)")
                                        .sirsiFont(.subheadline, weight: .medium)
                                        .foregroundStyle(Color.white)
                                    Spacer()
                                    Text(port.connected.uppercased())
                                        .sirsiFont(.caption, weight: .semibold)
                                        .foregroundStyle(port.connected == "yes" ? PantheonTheme.emerald : PantheonTheme.mutedText)
                                }
                                HStack(spacing: 12) {
                                    fact("Bus", port.bus)
                                    fact("Advertised ceiling", port.advertisedMax.isEmpty ? "Unknown" : port.advertisedMax)
                                }
                                keyValue("Negotiated rate", port.negotiatedRate.isEmpty ? "Unknown" : port.negotiatedRate)
                                if !port.negotiatedWhy.isEmpty {
                                    Text(port.negotiatedWhy)
                                        .sirsiFont(.caption2)
                                        .foregroundStyle(PantheonTheme.mutedText)
                                        .fixedSize(horizontal: false, vertical: true)
                                }
                                if !port.peer.isEmpty { keyValue("Attached peer", port.peer) }
                            }
                            .padding(.vertical, 8)
                            if index < links.ports.count - 1 { Divider().overlay(PantheonTheme.mutedText.opacity(0.15)) }
                        }
                    }
                } else if links.portCountKnown {
                    Text("The receipt contains no port rows.")
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.mutedText)
                }
                HStack(spacing: 12) {
                    fact("RDMA family", links.rdma.familyLoaded.isEmpty ? "Unknown" : links.rdma.familyLoaded)
                    fact("RDMA NVRAM", links.rdma.nvramEnabled.isEmpty ? "Unknown" : links.rdma.nvramEnabled)
                }
                HStack(spacing: 12) {
                    if !links.macos.isEmpty { fact("macOS", links.macos) }
                    if !links.collectedAt.isEmpty { fact("Collected", links.collectedAt) }
                }
                if !links.thunderboltInterfaces.isEmpty {
                    keyValue("Thunderbolt interfaces", links.thunderboltInterfaces.joined(separator: ", "))
                }
                if !links.source.isEmpty {
                    keyValue("Receipt source", links.source)
                }
                if !links.rdma.reason.isEmpty {
                    Text(links.rdma.reason)
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.mutedText)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if !links.unknown.isEmpty {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Unobserved fields")
                            .sirsiFont(.caption, weight: .semibold)
                            .foregroundStyle(PantheonTheme.gold)
                        ForEach(links.unknown, id: \.self) { item in
                            Text(item)
                                .sirsiFont(.caption2)
                                .foregroundStyle(PantheonTheme.mutedText)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                }
                Text("This is a local host capability receipt. It does not prove that Mercury is using a listed link or that a specific end-to-end transport is active.")
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
                    .padding(.top, 4)
            } else {
                emptyOrError(linksError, message: "The local transport receipt is unavailable; link state remains unknown.")
            }
            evidenceDisclosure(
                "Transport receipt evidence",
                command: transportArgs,
                sourceContract: "This host receipt reports Thunderbolt ports and RDMA enablement. Advertised speed is a ceiling; this source reports negotiated rate as unknown and does not identify Mercury’s path.",
                payload: linksEvidence,
                error: linksError
            )
        }
    }

    private var railSection: some View {
        sectionCard(title: "Rail state", subtitle: "No Mercury rail reading is available") {
            HStack(alignment: .top, spacing: 9) {
                Image(systemName: "point.3.connected.trianglepath.dotted")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.gold)
                Text("These readings do not show which rail Mercury uses or whether Mercury has rail traffic.")
                    .sirsiFont(.subheadline)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var routesSection: some View {
        sectionCard(title: "Related workspaces", subtitle: "Open the native surfaces for the adjacent systems") {
            HStack(spacing: 8) {
                workspaceAction(.apollo, title: "Apollo run planner", symbol: "cpu", tint: PantheonTheme.emerald) {
                    ApolloRunPlannerView(engine: engine)
                }
                workspaceAction(.ra, title: "Ra fabric", symbol: "point.3.connected.trianglepath.dotted", tint: PantheonTheme.gold) {
                    RaFabricView(engine: engine)
                }
            }
        }
    }

    @ViewBuilder private func workspaceAction<Destination: View>(
        _ workspace: PantheonWorkspace,
        title: String,
        symbol: String,
        tint: Color,
        @ViewBuilder destination: @escaping () -> Destination
    ) -> some View {
        if let openWorkspace {
            Button { openWorkspace(workspace) } label: {
                Label(title, systemImage: symbol)
            }
            .buttonStyle(.bordered)
            .tint(tint)
        } else {
            NavLink(destination: destination) {
                Label(title, systemImage: symbol)
            }
            .buttonStyle(.bordered)
            .tint(tint)
        }
    }

    private func sectionCard<Content: View>(title: String, subtitle: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 9) {
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .sirsiFont(.headline, weight: .semibold)
                    .foregroundStyle(Color.white)
                Text(subtitle)
                    .sirsiFont(.caption2)
                    .foregroundStyle(PantheonTheme.mutedText.opacity(0.82))
                    .textSelection(.enabled)
            }
            content()
        }
        .padding(13)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: 12).fill(PantheonTheme.panel))
        .overlay(RoundedRectangle(cornerRadius: 12).stroke(PantheonTheme.mutedText.opacity(0.12), lineWidth: 1))
    }

    private func evidenceDisclosure(_ title: String, command: [String], sourceContract: String, payload: String?, error: String?) -> some View {
        DisclosureGroup(title) {
            VStack(alignment: .leading, spacing: 6) {
                Text(sourceContract)
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.mutedText)
                    .fixedSize(horizontal: false, vertical: true)
                Text("sirsi " + command.joined(separator: " "))
                    .sirsiFont(.caption, design: .monospaced)
                    .foregroundStyle(PantheonTheme.gold)
                    .textSelection(.enabled)
                if let error {
                    Text(error)
                        .sirsiFont(.caption)
                        .foregroundStyle(PantheonTheme.gold)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let payload, !payload.isEmpty {
                    Text(payload)
                        .sirsiFont(.caption2, design: .monospaced)
                        .foregroundStyle(PantheonTheme.mutedText)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(.top, 6)
        }
        .sirsiFont(.caption, weight: .semibold)
        .foregroundStyle(PantheonTheme.mutedText)
        .tint(PantheonTheme.gold)
        .padding(.top, 5)
    }

    @ViewBuilder private func emptyOrError(_ error: String?, message: String) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(message)
                .sirsiFont(.subheadline)
                .foregroundStyle(PantheonTheme.mutedText)
                .fixedSize(horizontal: false, vertical: true)
            if let error {
                Text(error)
                    .sirsiFont(.caption)
                    .foregroundStyle(PantheonTheme.gold)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private func staleReadNotice(_ error: String) -> some View {
        Label("Refresh failed · showing last successful read", systemImage: "exclamationmark.arrow.triangle.2.circlepath")
            .sirsiFont(.caption, weight: .semibold)
            .foregroundStyle(PantheonTheme.gold)
            .help(error)
    }

    private func keyValue(_ key: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Text(key)
                .sirsiFont(.caption)
                .foregroundStyle(PantheonTheme.mutedText.opacity(0.76))
            Text(value)
                .sirsiFont(.caption)
                .foregroundStyle(PantheonTheme.mutedText)
                .textSelection(.enabled)
        }
        .fixedSize(horizontal: false, vertical: true)
    }

    private func fact(_ key: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(key.uppercased())
                .sirsiFont(9, weight: .semibold)
                .foregroundStyle(PantheonTheme.mutedText.opacity(0.68))
            Text(value)
                .sirsiFont(.caption)
                .foregroundStyle(PantheonTheme.mutedText)
                .lineLimit(2)
        }
    }

    private func statusMark(_ color: Color) -> some View {
        Circle()
            .fill(color)
            .frame(width: 8, height: 8)
            .padding(.top, 5)
    }

    @MainActor private func refresh() async {
        loading = true
        async let monitorRead: Void = monitor.refresh()
        async let laneRead = SirsiEngine.runJSONRead(args: laneArgs, timeoutSeconds: 8)
        async let machineRead = SirsiEngine.runJSONRead(args: machineArgs, timeoutSeconds: 8)
        async let transportRead = SirsiEngine.runJSONRead(args: transportArgs, timeoutSeconds: 8)
        let (laneResult, machineResult, transportResult) = await (laneRead, machineRead, transportRead)
        _ = await monitorRead
        guard !Task.isCancelled else { loading = false; return }

        do {
            lane = try decodeMercuryLane(read: laneResult)
            laneError = nil
            laneReadAt = Date()
            laneEvidence = evidenceText(laneResult.data)
        } catch {
            laneError = readFailure(laneResult, label: "Ra lane ping")
            laneEvidence = evidenceText(laneResult.data)
        }

        if mercuryReadSucceeded(machineResult),
           let decoded = try? decodeMercuryMachines(machineResult.data) {
            machines = decoded.machines
            machinesError = nil
            machinesEvidence = evidenceText(machineResult.data)
        } else {
            machinesError = readFailure(machineResult, label: "Ra machine registry")
            machinesEvidence = evidenceText(machineResult.data)
        }

        if mercuryReadSucceeded(transportResult),
           let decoded = try? decodeMercuryTransportReceipt(transportResult.data) {
            links = decoded
            linksError = nil
            linksEvidence = evidenceText(transportResult.data)
        } else {
            linksError = readFailure(transportResult, label: "Host transport receipt")
            linksEvidence = evidenceText(transportResult.data)
        }

        if laneError == nil || machinesError == nil || linksError == nil {
            refreshedAt = Date()
        }
        loading = false
    }

    private func readFailure(_ read: SirsiEngine.JSONCommandRead, label: String) -> String {
        let detail: String
        if let launchError = read.launchError {
            detail = launchError
        } else if read.timedOut {
            detail = "The canonical read exceeded its 8-second limit."
        } else if read.data.isEmpty, let status = read.exitStatus {
            detail = "The command returned no JSON (exit status \(status)); CLI error text is not captured by this reader."
        } else if read.data.isEmpty {
            detail = "The command returned no JSON."
        } else if let status = read.exitStatus, status != 0 {
            detail = "The command returned JSON but exited with status \(status); the result was not accepted."
        } else if read.exitStatus == nil {
            detail = "The command did not report a successful exit status; the result was not accepted."
        } else {
            detail = "The command returned data that did not match the expected read model."
        }
        return "\(label): \(String(detail.prefix(260)))"
    }

    private func evidenceText(_ data: Data) -> String? {
        guard !data.isEmpty else { return nil }
        let maximumBytes = 12_000
        let clipped = data.count > maximumBytes ? Data(data.prefix(maximumBytes)) : data
        let rendered: String
        if let object = try? JSONSerialization.jsonObject(with: clipped),
           let pretty = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
           let string = String(data: pretty, encoding: .utf8) {
            rendered = string
        } else {
            rendered = String(data: clipped, encoding: .utf8) ?? "Evidence is not valid UTF-8."
        }
        if data.count > maximumBytes {
            return rendered + "\n… evidence clipped at 12,000 bytes."
        }
        return rendered
    }
}

enum MercuryWorkspaceDecodeError: Error, Equatable {
    case noMercuryLane
    case unsuccessfulRead
}

func mercuryReadSucceeded(_ read: SirsiEngine.JSONCommandRead) -> Bool {
    read.exitStatus == 0 && !read.timedOut && read.launchError == nil
}

func decodeMercuryLane(read: SirsiEngine.JSONCommandRead) throws -> MercuryLanePing {
    guard mercuryReadSucceeded(read) else { throw MercuryWorkspaceDecodeError.unsuccessfulRead }
    return try decodeMercuryLane(read.data)
}

func decodeMercuryLane(_ data: Data) throws -> MercuryLanePing {
    let records = try JSONDecoder().decode([MercuryLanePing].self, from: data)
    guard let lane = records.first(where: { $0.agent == "mercury" }) else {
        throw MercuryWorkspaceDecodeError.noMercuryLane
    }
    return lane
}

func decodeMercuryMachines(_ data: Data) throws -> MercuryMachinesReport {
    try JSONDecoder().decode(MercuryMachinesReport.self, from: data)
}

func decodeMercuryTransportReceipt(_ data: Data) throws -> MercuryTransportReceipt {
    try JSONDecoder().decode(MercuryTransportReceipt.self, from: data)
}

struct MercuryLanePing: Decodable {
    let agent: String
    let verdict: String
    let detail: String
    let workable: Bool?
    let thread: String?
    let reportedAt: String?
    let reportSummary: String?

    enum CodingKeys: String, CodingKey {
        case agent, verdict, detail, workable, thread
        case reportedAt = "reported_at"
        case reportSummary = "report_summary"
    }
}

struct MercuryMachinesReport: Decodable {
    let machines: [MercuryMachine]
}

struct MercuryMachine: Decodable {
    let id: String
    let model: String
    let chip: String
    let memoryGiB: Int
    let tailscale: String
    let lan: String
    let monitorPort: Int
    let registeredAt: String
    let admittedBy: String

    enum CodingKeys: String, CodingKey {
        case id, model, chip, tailscale, lan
        case memoryGiB = "memory_gib"
        case monitorPort = "monitor_port"
        case registeredAt = "registered_at"
        case admittedBy = "admitted_by"
    }

    init(
        id: String,
        model: String,
        chip: String = "",
        memoryGiB: Int = 0,
        tailscale: String = "",
        lan: String = "",
        monitorPort: Int = 0,
        registeredAt: String = "",
        admittedBy: String = ""
    ) {
        self.id = id
        self.model = model
        self.chip = chip
        self.memoryGiB = memoryGiB
        self.tailscale = tailscale
        self.lan = lan
        self.monitorPort = monitorPort
        self.registeredAt = registeredAt
        self.admittedBy = admittedBy
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        model = try container.decode(String.self, forKey: .model)
        chip = try container.decodeIfPresent(String.self, forKey: .chip) ?? ""
        memoryGiB = try container.decodeIfPresent(Int.self, forKey: .memoryGiB) ?? 0
        tailscale = try container.decodeIfPresent(String.self, forKey: .tailscale) ?? ""
        lan = try container.decodeIfPresent(String.self, forKey: .lan) ?? ""
        monitorPort = try container.decodeIfPresent(Int.self, forKey: .monitorPort) ?? 0
        registeredAt = try container.decodeIfPresent(String.self, forKey: .registeredAt) ?? ""
        admittedBy = try container.decodeIfPresent(String.self, forKey: .admittedBy) ?? ""
    }
}

struct MercuryTransportReceipt: Decodable {
    let collectedAt: String
    let host: String
    let macos: String
    let ports: [MercuryTransportPort]
    let portCountKnown: Bool
    let thunderboltInterfaces: [String]
    let rdma: MercuryRDMAState
    let unknown: [String]
    let source: String

    enum CodingKeys: String, CodingKey {
        case host, macos, ports, rdma, unknown, source
        case collectedAt = "collected_at"
        case portCountKnown = "port_count_known"
        case thunderboltInterfaces = "thunderbolt_interfaces"
    }

    init(
        collectedAt: String = "",
        host: String = "",
        macos: String = "",
        ports: [MercuryTransportPort] = [],
        portCountKnown: Bool = false,
        thunderboltInterfaces: [String] = [],
        rdma: MercuryRDMAState = MercuryRDMAState(familyLoaded: "unknown", nvramEnabled: "unknown", reason: ""),
        unknown: [String] = [],
        source: String = ""
    ) {
        self.collectedAt = collectedAt
        self.host = host
        self.macos = macos
        self.ports = ports
        self.portCountKnown = portCountKnown
        self.thunderboltInterfaces = thunderboltInterfaces
        self.rdma = rdma
        self.unknown = unknown
        self.source = source
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        collectedAt = try container.decodeIfPresent(String.self, forKey: .collectedAt) ?? ""
        host = try container.decodeIfPresent(String.self, forKey: .host) ?? ""
        macos = try container.decodeIfPresent(String.self, forKey: .macos) ?? ""
        ports = try container.decodeIfPresent([MercuryTransportPort].self, forKey: .ports) ?? []
        portCountKnown = try container.decodeIfPresent(Bool.self, forKey: .portCountKnown) ?? false
        thunderboltInterfaces = try container.decodeIfPresent([String].self, forKey: .thunderboltInterfaces) ?? []
        rdma = try container.decodeIfPresent(MercuryRDMAState.self, forKey: .rdma)
            ?? MercuryRDMAState(familyLoaded: "unknown", nvramEnabled: "unknown", reason: "")
        unknown = try container.decodeIfPresent([String].self, forKey: .unknown) ?? []
        source = try container.decodeIfPresent(String.self, forKey: .source) ?? ""
    }
}

struct MercuryTransportPort: Decodable {
    let bus: String
    let receptacleID: String
    let advertisedMax: String
    let connected: String
    let peer: String
    let negotiatedRate: String
    let negotiatedWhy: String

    enum CodingKeys: String, CodingKey {
        case bus, connected, peer
        case receptacleID = "receptacle_id"
        case advertisedMax = "advertised_max"
        case negotiatedRate = "negotiated_rate"
        case negotiatedWhy = "negotiated_rate_reason"
    }

    init(
        bus: String = "",
        receptacleID: String = "",
        advertisedMax: String = "",
        connected: String = "unknown",
        peer: String = "",
        negotiatedRate: String = "unknown",
        negotiatedWhy: String = ""
    ) {
        self.bus = bus
        self.receptacleID = receptacleID
        self.advertisedMax = advertisedMax
        self.connected = connected
        self.peer = peer
        self.negotiatedRate = negotiatedRate
        self.negotiatedWhy = negotiatedWhy
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        bus = try container.decodeIfPresent(String.self, forKey: .bus) ?? ""
        receptacleID = try container.decodeIfPresent(String.self, forKey: .receptacleID) ?? ""
        advertisedMax = try container.decodeIfPresent(String.self, forKey: .advertisedMax) ?? ""
        connected = try container.decodeIfPresent(String.self, forKey: .connected) ?? "unknown"
        peer = try container.decodeIfPresent(String.self, forKey: .peer) ?? ""
        negotiatedRate = try container.decodeIfPresent(String.self, forKey: .negotiatedRate) ?? "unknown"
        negotiatedWhy = try container.decodeIfPresent(String.self, forKey: .negotiatedWhy) ?? ""
    }
}

struct MercuryRDMAState: Decodable {
    let familyLoaded: String
    let nvramEnabled: String
    let reason: String

    enum CodingKeys: String, CodingKey {
        case reason
        case familyLoaded = "family_loaded"
        case nvramEnabled = "nvram_enabled"
    }

    init(familyLoaded: String, nvramEnabled: String, reason: String) {
        self.familyLoaded = familyLoaded
        self.nvramEnabled = nvramEnabled
        self.reason = reason
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        familyLoaded = try container.decodeIfPresent(String.self, forKey: .familyLoaded) ?? "unknown"
        nvramEnabled = try container.decodeIfPresent(String.self, forKey: .nvramEnabled) ?? "unknown"
        reason = try container.decodeIfPresent(String.self, forKey: .reason) ?? ""
    }
}
