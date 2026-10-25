import SwiftUI

// Present the retained Go scene and mapping in the existing native workspace.
// No run controls, second reader, calibration, discovery, or physical ports.
struct MercurySnapshotPresentation: View {
    let projection: MercuryMonitorProjection
    let readSucceeded: Bool
    let readCompletedAt: Double?
    let elapsed: Double?

    private var currentAggregate: Bool {
        readSucceeded && projection.aggregateIsCurrent(readCompletedAt: readCompletedAt, elapsed: elapsed)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            if let h = projection.headline {
                HStack(alignment: .top, spacing: 40) {
                    headlineCell("WIRE", h.wire, color: .primary)
                    headlineCell("COMPRESSED", h.compressed, color: h.compressed.number == "—" ? .primary : gold)
                    headlineCell("GAIN", h.gain, color: .primary)
                }.frame(maxWidth: .infinity).monospacedDigit().accessibilityElement(children: .combine)
                Text(currentAggregate ? h.context : "Current aggregate unavailable · incomplete or expired observations")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                if let lamps = h.lamps {
                    HStack(spacing: 12) {
                        ForEach(lamps.indices, id: \.self) { i in
                            Label(lamps[i].label, systemImage: currentAggregate && lamps[i].on ? "circle.fill" : "circle")
                                .sirsiFont(.caption2).foregroundStyle(gold)
                        }
                    }
                }
                if let cables = h.cables {
                    Text(currentAggregate ? "\(cables) active cables" : "Active cable aggregate unknown")
                        .sirsiFont(.caption2).foregroundStyle(.secondary)
                }
                if let run = h.run_status, !run.isEmpty {
                    Text("Source run at snapshot: \(run)").sirsiFont(.callout)
                }
            }
            if let dashboard = projection.snapshot {
                if let geometry = projection.scene?.geometry, geometry.isUsable,
                   Set(geometry.cards.map(\.id)) == Set(dashboard.nodes.map(\.id)) {
                    ScrollView(.horizontal) {
                        MercuryHardwareScene(scene: geometry, nodes: dashboard.nodes, projection: projection,
                                             readSucceeded: readSucceeded, readCompletedAt: readCompletedAt, elapsed: elapsed)
                            .frame(width: max(geometry.width, 1000), height: geometry.height)
                            .background(.black)
                    }
                    Text("Cable endpoints are diagram card edges, not physical ports. The retained wide scene scrolls on narrow windows.")
                        .sirsiFont(.caption2).foregroundStyle(.secondary)
                    ForEach(geometry.cables) { cable in
                        cableStrip(cable)
                    }
                } else if !dashboard.nodes.isEmpty {
                    Text("Scene geometry is unavailable. The labelled observations remain below; no hardware layout is inferred.")
                        .sirsiFont(.caption).foregroundStyle(gold)
                }
                DisclosureGroup("Retained measurement details · source observations") {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("Source \(dashboard.source?.host ?? "Unknown") · build \(dashboard.source?.build ?? "Unknown") · generation \(dashboard.generation.map(String.init) ?? "Unknown")")
                        Text("Generated \(timestamp(dashboard.generated_at)) · decimal GB/s. These retained facts are not a current-health or throughput certificate.")
                        ForEach(dashboard.nodes, id: \.id) { node in
                            VStack(alignment: .leading, spacing: 3) {
                                Text("\(node.name) · \(node.model ?? "Unknown model") · \(node.chip ?? "Unknown chip")")
                                Text("Calibration \(node.gpu.calibration?.status ?? "unknown") · \(node.gpu.calibration?.source ?? "Unknown source") · ID \(node.gpu.calibration?.id ?? "Unknown")")
                                Text("Decode \(node.decode?.label ?? node.decode?.mode ?? "Unknown") · seal \(node.seal?.label ?? node.seal?.state ?? "Unknown")")
                                Text("Node \(node.state) · source \(node.source ?? "Unknown") · observed \(timestamp(node.observed_at)) · age at snapshot \(node.age_s.map { String(format: "%.1f s", $0) } ?? "Unknown")")
                            }
                        }
                        ForEach(dashboard.links, id: \.id) { link in
                            VStack(alignment: .leading, spacing: 3) {
                                Text("\(link.id) · \(link.state) · MTU \(link.mtu.map(String.init) ?? "Unknown")")
                                if let ends = link.ends {
                                    Text(ends.map { $0.map { "\($0.node) · \($0.iface)" } ?? "Missing peer" }.joined(separator: " ↔ "))
                                }
                                ForEach(link.directions.indices, id: \.self) { i in
                                    let direction = link.directions[i]
                                    Text("\(direction.from) → \(direction.to) · \(rate(direction.wire_gbs)) wire · \(rate(direction.payload_gbs)) payload · \(rate(direction.compressed_gbs)) compressed")
                                    Text("\(direction.provenance ?? "Unknown provenance") · \(direction.interval_s.map { String(format: "%.2f s sample", $0) } ?? "Unknown interval") · observed \(timestamp(direction.observed_at))")
                                }
                            }
                        }
                    }.sirsiFont(.caption).textSelection(.enabled).padding(.top, 8)
                }
            }
        }
        .padding(14)
        .background(.black, in: RoundedRectangle(cornerRadius: 10))
        .environment(\.colorScheme, .dark)
    }

    private func headlineCell(_ title: String, _ cell: MercuryMonitorCell, color: Color) -> some View {
        let number = currentAggregate ? cell.number : "—"
        let unit = currentAggregate ? cell.unit : ""
        let note = currentAggregate ? cell.note ?? "" : ""
        return VStack(alignment: .center, spacing: 10) {
            Text(title).sirsiFont(12, weight: .semibold).tracking(2).foregroundStyle(color)
            HStack(alignment: .firstTextBaseline, spacing: 3) {
                Text(number).font(.system(size: 60, weight: .light)).minimumScaleFactor(0.4).lineLimit(1)
                Text(unit).sirsiFont(18)
            }.foregroundStyle(number == "—" ? Color.secondary : color)
            Text(note.uppercased()).sirsiFont(12, weight: .semibold).tracking(1)
                .foregroundStyle(.secondary).multilineTextAlignment(.center).frame(minHeight: 36, alignment: .top)
        }.frame(maxWidth: 300, alignment: .center)
    }

    private func cableStrip(_ cable: MercurySceneGeometry.Cable) -> some View {
        let complete = cableIsCurrent(cable.id)
        return VStack(alignment: .leading, spacing: 4) {
            Text("\(cable.letter) · \(complete ? cable.rate.number + " " + cable.rate.unit : "Unknown") · \(cable.state)")
                .sirsiFont(.callout, weight: .semibold)
            if cable.state == "missing" { Text("\(cable.iface) · Missing peer").sirsiFont(.caption).foregroundStyle(gold) }
            ForEach(cable.directions.indices, id: \.self) { i in
                let flow = cable.directions[i]
                Text("\(flow.from) → \(flow.to) · \(directionRate(cable.id, from: flow.from, to: flow.to))")
                    .sirsiFont(.caption).foregroundStyle(flow.color == "out" ? emerald : gold)
            }
        }
    }

    private func cableIsCurrent(_ id: String) -> Bool {
        guard let cable = projection.scene?.cables.first(where: { $0.id == id }),
              cable.state == "up", cable.directions.count == 2 else { return false }
        return cable.directions.allSatisfy {
            projection.currentRate(for: $0, cable: cable, readSucceeded: readSucceeded,
                                   readCompletedAt: readCompletedAt, elapsed: elapsed) != "Unknown"
        }
    }
    private func directionRate(_ id: String, from: String, to: String) -> String {
        guard let cable = projection.scene?.cables.first(where: { $0.id == id }),
              let direction = cable.directions.first(where: { $0.from == from && $0.to == to }) else { return "Unknown" }
        return projection.currentRate(for: direction, cable: cable, readSucceeded: readSucceeded,
                                      readCompletedAt: readCompletedAt, elapsed: elapsed)
    }

    private func timestamp(_ value: Double?) -> String {
        mercuryObservationTime(value)
    }
    private func rate(_ value: Double?) -> String {
        guard let value, value.isFinite, value >= 0 else { return "Unknown" }
        return String(format: "%.3f GB/s", value)
    }
}
