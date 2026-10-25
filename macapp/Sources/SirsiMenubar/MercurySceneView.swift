import SwiftUI

// Exact card-edge geometry, labels and motion speeds come from Go. Missing
// geometry falls back to labelled rows; no native topology/family guessing.
struct MercurySceneGeometry: Decodable {
    let width: Double; let height: Double; let cards: [Card]; let cables: [Cable]
    struct Card: Decodable, Identifiable {
        let id: String; let family: String; let x: Double; let y: Double; let width: Double; let height: Double
    }
    struct Cable: Decodable, Identifiable {
        let id: String; let letter: String; let state: String; let iface: String
        let x1: Double; let y1: Double; let x2: Double; let y2: Double; let lane: Double
        let rate: MercuryMonitorCell; let directions: [Flow]
    }
    struct Flow: Decodable {
        let from: String; let to: String; let rate: String; let color: String; let speed: Double; let forward: Bool
    }
    var isUsable: Bool {
        width.isFinite && height.isFinite && width > 0 && height > 0 &&
        Set(cards.map(\.id)).count == cards.count && Set(cables.map(\.id)).count == cables.count &&
        cards.allSatisfy { [$0.x, $0.y, $0.width, $0.height].allSatisfy(\.isFinite) && $0.width > 0 && $0.height > 0 } &&
        cables.allSatisfy { [$0.x1, $0.y1, $0.x2, $0.y2, $0.lane].allSatisfy(\.isFinite) &&
            $0.directions.allSatisfy { $0.speed.isFinite && $0.speed >= 0 && $0.speed <= 600 } }
    }
}

// Native vector presentation of Go's shared scene. No case ports or line-rate
// dials: those facts are not carried by dashboard v1.
struct MercuryHardwareScene: View {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.snapshotMode) private var snapshotMode
    let scene: MercurySceneGeometry
    let nodes: [MercuryMonitorNode]
    let projection: MercuryMonitorProjection
    let readSucceeded: Bool
    let readCompletedAt: Double?
    let elapsed: Double?
    private func flowIsCurrent(_ cable: MercurySceneGeometry.Cable, _ flow: MercurySceneGeometry.Flow) -> Bool {
        guard let raw = projection.scene?.cables.first(where: { $0.id == cable.id }),
              let direction = raw.directions.first(where: { $0.from == flow.from && $0.to == flow.to }) else { return false }
        return projection.currentRate(for: direction, cable: raw, readSucceeded: readSucceeded,
                                      readCompletedAt: readCompletedAt, elapsed: elapsed) != "Unknown"
    }
    private var hasCurrentMotion: Bool {
        for cable in scene.cables {
            for flow in cable.directions where flow.speed > 0 && flowIsCurrent(cable, flow) { return true }
        }
        return false
    }
    private func track(_ c: MercurySceneGeometry.Cable) -> Path {
        var p = Path(); p.move(to: CGPoint(x: c.x1, y: c.y1))
        if c.state == "missing" {
            p.addLine(to: CGPoint(x: c.x2, y: c.y2))
        } else {
            let bend = min(36, (c.x2 - c.x1) / 3)
            p.addCurve(to: CGPoint(x: c.x1 + bend, y: c.lane), control1: CGPoint(x: c.x1 + bend, y: c.y1), control2: CGPoint(x: c.x1 + bend, y: c.lane))
            p.addLine(to: CGPoint(x: c.x2 - bend, y: c.lane))
            p.addCurve(to: CGPoint(x: c.x2, y: c.y2), control1: CGPoint(x: c.x2 - bend, y: c.lane), control2: CGPoint(x: c.x2 - bend, y: c.y2))
        }
        return p
    }
    var body: some View {
        GeometryReader { geometry in
            let scale = geometry.size.width / scene.width
            ZStack(alignment: .topLeading) {
                TimelineView(.animation(minimumInterval: 1.0 / 30, paused: reduceMotion || snapshotMode || !hasCurrentMotion)) { timeline in
                    Canvas { context, _ in
                        context.scaleBy(x: scale, y: scale)
                        for cable in scene.cables {
                            let path = track(cable), down = cable.state != "up"
                            context.stroke(path, with: .color(emerald.opacity(down ? 0.12 : 0.2)), style: StrokeStyle(lineWidth: 10, lineCap: .round, dash: down ? [5, 6] : []))
                            context.stroke(path, with: .color(down ? Color.secondary : emerald.opacity(0.6)), style: StrokeStyle(lineWidth: 1.5, dash: down ? [5, 6] : []))
                            if !reduceMotion && !snapshotMode {
                                for flow in cable.directions where flow.speed > 0 && flowIsCurrent(cable, flow) {
                                    let phase = (timeline.date.timeIntervalSinceReferenceDate * flow.speed / 1000).truncatingRemainder(dividingBy: 1)
                                    for i in 0..<12 {
                                        let t = (Double(i) / 12 + phase).truncatingRemainder(dividingBy: 1)
                                        let from = flow.forward ? t : 1 - t
                                        context.stroke(path.trimmedPath(from: from, to: min(1, from + 0.013)), with: .color(flow.color == "out" ? emerald : flow.color == "in" ? gold : .secondary), style: StrokeStyle(lineWidth: 3, lineCap: .round))
                                    }
                                }
                            }
                        }
                    }
                }.frame(width: scene.width * scale, height: scene.height * scale)
                ForEach(scene.cables) { cable in
                    Text(cable.letter).sirsiFont(14, weight: .semibold).foregroundStyle(gold)
                        .padding(5).background(.black).clipShape(RoundedRectangle(cornerRadius: 7))
                        .overlay(RoundedRectangle(cornerRadius: 7).stroke(gold.opacity(0.8)))
                        .overlay(alignment: .bottom) {
                            if cable.state == "missing" { Text(cable.iface).sirsiFont(10).foregroundStyle(.secondary).offset(y: 16) }
                        }
                        .position(x: (cable.x1 + cable.x2) / 2 * scale, y: cable.lane * scale)
                        .accessibilityLabel("Cable " + cable.letter + ", " + cable.state + (cable.state == "missing" ? ", " + cable.iface : ""))
                }
                ForEach(scene.cards) { card in
                    if let node = nodes.first(where: { $0.id == card.id }) {
                        hardware(node, card: card)
                            .frame(width: card.width, height: card.height)
                            .scaleEffect(scale, anchor: .topLeading)
                            .offset(x: card.x * scale, y: card.y * scale)
                    }
                }
            }
        }.accessibilityElement(children: .contain)
    }
    private func hardware(_ node: MercuryMonitorNode, card: MercurySceneGeometry.Card) -> some View {
        let ghost = node.state == "unreachable" || node.state == "expired"
        let muted = !readSucceeded || !projection.isCurrent(node, readCompletedAt: readCompletedAt, elapsed: elapsed)
        let cpu = projection.cpuBusy(for: node, readSucceeded: readSucceeded, readCompletedAt: readCompletedAt, elapsed: elapsed)
        let gpu = projection.gpuWork(for: node, readSucceeded: readSucceeded, readCompletedAt: readCompletedAt, elapsed: elapsed)
        let calibration = node.gpu.calibration?.status ?? "unknown"
        let cpuText = cpu.map { String(format: "%.0f%%", $0) } ?? "—"
        let gpuText = calibration == "calibrated" ? gpu.map { String(format: "%.0f%%", $0) } ?? "—" : "—"
        return ZStack(alignment: .top) {
            MercuryHardwareOutline(family: card.family)
                .fill(LinearGradient(colors: [Color(red: 0.12, green: 0.25, blue: 0.21), Color(red: 0.045, green: 0.11, blue: 0.09)], startPoint: .topLeading, endPoint: .bottomTrailing))
            MercuryHardwareOutline(family: card.family)
                .stroke(ghost ? Color.red.opacity(0.6) : emerald.opacity(0.45), style: StrokeStyle(lineWidth: 1.3, dash: ghost || card.family == "generic" ? [6, 5] : []))
            VStack(spacing: 4) {
                Text("CPU").sirsiFont(12, weight: .semibold).tracking(2).foregroundStyle(.secondary)
                Text(cpuText).font(.system(size: 44, weight: .light))
                Spacer(minLength: 30)
                VStack(spacing: 4) {
                    Text(calibration == "calibrated" ? "GPU" : "GPU · " + calibration).sirsiFont(12, weight: .semibold).tracking(1).foregroundStyle(.secondary)
                    Text(gpuText).font(.system(size: 44, weight: .light))
                }.padding(.horizontal, 14).padding(.vertical, 4)
                    .background(card.family == "laptop" ? Color.black.opacity(0.7) : .clear)
                    .clipShape(RoundedRectangle(cornerRadius: 6))
            }.monospacedDigit().padding(.top, 35).padding(.bottom, card.family == "laptop" ? 75 : 45)
                .opacity(ghost ? 0.25 : 1)
            VStack(spacing: 3) {
                Text(node.id).sirsiFont(22, weight: .semibold)
                Text((muted ? "Last reported · " : "") + node.state).sirsiFont(.caption2).foregroundStyle(.secondary)
            }.offset(y: card.height + 8)
        }.saturation(muted ? 0.25 : 1).opacity(muted ? 0.85 : 1)
            .accessibilityElement(children: .combine)
    }
}

private struct MercuryHardwareOutline: Shape {
    var family: String
    func path(in rect: CGRect) -> Path {
        var path = Path()
        if family == "laptop" {
            path.addRoundedRect(in: CGRect(x: 0, y: 0, width: rect.width, height: rect.height * 0.49), cornerSize: CGSize(width: 15, height: 15))
            path.addRect(CGRect(x: 0, y: rect.height * 0.5, width: rect.width, height: rect.height * 0.48))
            // A keyboard and trackpad identify the family, without implying ports.
            for row in 0..<4 { for col in 0..<13 {
                path.addRoundedRect(in: CGRect(x: 20 + Double(col) * (rect.width - 40) / 13, y: rect.height * 0.54 + Double(row) * 10, width: (rect.width - 40) / 13 - 2, height: 8), cornerSize: CGSize(width: 1, height: 1))
            } }
            path.addRoundedRect(in: CGRect(x: rect.width * 0.28, y: rect.height * 0.77, width: rect.width * 0.44, height: rect.height * 0.18), cornerSize: CGSize(width: 4, height: 4))
        } else if family == "imac" {
            path.addRoundedRect(in: CGRect(x: 0, y: 0, width: rect.width, height: rect.height * 0.85), cornerSize: CGSize(width: 15, height: 15))
            path.addRect(CGRect(x: rect.width * 0.43, y: rect.height * 0.85, width: rect.width * 0.14, height: rect.height * 0.13))
        } else {
            path.addRoundedRect(in: rect, cornerSize: CGSize(width: family == "studio" || family == "mini" ? 24 : 12, height: family == "studio" || family == "mini" ? 24 : 12))
            if family == "studio" || family == "mini" { path.move(to: CGPoint(x: 0, y: rect.height * 0.87)); path.addLine(to: CGPoint(x: rect.width, y: rect.height * 0.87)) }
        }
        return path
    }
}
