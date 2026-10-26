import SwiftUI
import Foundation

// Decodes the ONE Go FabricBoard, not node-status or per-view arithmetic.
struct CanonicalFabricBoard: Decodable {
    struct Work: Decodable { let done, open, in_progress, assigned_not_started, stalled, blocked, total: Int }
    struct Messages: Decodable { let open, stranded: Int; let oldest_age_seconds: Double }
    struct Lane: Decodable, Identifiable {
        let agent, state, state_label: String
        let open, active, stalled, blocked: Int
        let role: String?
        var id: String { agent }
    }
    struct Health: Decodable {
        let ram_pct: Double?; let swap, total_ram: Int64?; let cpu_cores: Int?; let git_dirty: Bool?
        let engine, engine_state: String; let issues: [String]
    }
    struct Activity: Decodable { let at, agent, task_id, from, to: String }
    struct Presentation: Decodable {
        struct Swatch: Decodable { let dark, light: String }
        let headlines: [String:String]
        let palette: [String:Swatch]
    }
    let work: Work; let messages: Messages; let lanes: [Lane]; let health: Health
    let activity: [Activity]; let activity_seeded, runtime_available: Bool
    let build, generated_at: String
    let presentation: Presentation?
    var headlineRoleTokens: [String?] {
        ["completed","progress","stalled","messages"].map { presentation?.headlines[$0] }
    }
    // The Go brand owner supplies the palette; this surface invents neither a
    // runtime classification nor independent hexadecimal state colors.
    func roleHex(_ role: String?, dark: Bool) -> String? {
        guard let role, ["ok","gold","warn","ink2","dim"].contains(role),
              let swatch = presentation?.palette[role] else { return nil }
        let hex = dark ? swatch.dark : swatch.light
        guard hex.count == 7, hex.first == "#", hex.dropFirst().allSatisfy({$0.isHexDigit}) else { return nil }
        return hex
    }
    func roleColor(_ role: String?, dark: Bool) -> Color {
        guard let hex = roleHex(role,dark:dark) else { return .secondary }
        var value: UInt64 = 0
        guard Scanner(string:String(hex.dropFirst())).scanHexInt64(&value) else { return .secondary }
        return Color(.sRGB,red:Double((value >> 16) & 255)/255,green:Double((value >> 8) & 255)/255,blue:Double(value & 255)/255,opacity:1)
    }
    var valid: Bool {
        [work.done,work.open,work.in_progress,work.assigned_not_started,work.stalled,work.blocked,work.total,messages.open,messages.stranded].allSatisfy{$0 >= 0} &&
        pantheonObservationDate(generated_at) != nil && messages.oldest_age_seconds.isFinite && messages.oldest_age_seconds >= 0 &&
        (health.ram_pct.map{$0.isFinite && (0...100).contains($0)} ?? true) &&
        (health.swap.map{$0 >= 0} ?? true) && (health.total_ram.map{$0 > 0} ?? true) &&
        (health.cpu_cores.map{$0 > 0} ?? true) && !health.engine.isEmpty && !health.engine_state.isEmpty &&
        lanes.allSatisfy{!$0.agent.isEmpty && !$0.state_label.isEmpty}
    }
    var headlineLines: [String] {
        ["Completed / in flight: \(work.done) / \(work.open) tasks",
         "In progress / assigned: \(work.in_progress) / \(work.assigned_not_started) tasks",
         "Stalled / blocked: \(work.stalled) / \(work.blocked) tasks",
         "Messages pending: \(messages.open) inbox messages · \(messages.stranded) stranded"]
    }
}

// The compact and full views use this same bounded attention projection.
struct FabricContentProjection {
    let attention: [String]
    let showsDetails: Bool
    let offersDetail: Bool
}

@MainActor final class FabricProjectionStore: ObservableObject {
    typealias Reader = @MainActor ([String]) async -> SirsiEngine.JSONCommandRead
    static let arguments = ["router","fabric","--json"]
    @Published private(set) var board: CanonicalFabricBoard?
    @Published private(set) var error: String?
    @Published private(set) var loading = false
    private let read: Reader
    init(read: Reader? = nil) { self.read = read ?? { await SirsiEngine.runJSONRead(args:$0,timeoutSeconds:20) } }
    func current(at now: Date) -> Bool {
        error == nil && pantheonObservationIsCurrent(pantheonObservationDate(board?.generated_at),at:now,maxAge:30)
    }
    func presentation(compact: Bool, at now: Date) -> FabricContentProjection {
        var lines: [String] = []
        if let error { lines.append(error) }
        if let board {
            if !current(at:now) {lines.append("Retained observation — current Fabric state unavailable")}
            if !board.runtime_available {lines.append("Runtime classification unavailable")}
            lines.append(contentsOf:board.health.issues.prefix(8))
            if board.health.issues.count > 8 {lines.append("More host issues in the full Fabric detail")}
            if lines.isEmpty {lines.append("No attention entries supplied; absence is not a host-health verdict")}
        } else {lines.append("Fabric unavailable — no observation received")}
        return FabricContentProjection(attention:lines,showsDetails:!compact,offersDetail:compact)
    }
    func load() async {
        guard !loading, !Task.isCancelled else {return}
        loading = true; defer {loading = false}
        let result = await read(Self.arguments)
        guard !Task.isCancelled else {return}
        guard !result.timedOut, result.launchError == nil, result.exitStatus == 0,
              result.data.count <= 8*1024*1024,
              let value = try? JSONDecoder().decode(CanonicalFabricBoard.self,from:result.data), value.valid else {
            error = "Fabric refresh unavailable. Any retained task/message/runtime/host facts belong to the previous observation; refresh to retry."
            return
        }
        board = value; error = nil
    }
}

@MainActor struct CanonicalFabricView: View {
    @ObservedObject var store: FabricProjectionStore
    @EnvironmentObject private var nav: Nav
    var compact = false
    @Environment(\.snapshotMode) private var snapshotMode
    @Environment(\.colorScheme) private var colorScheme
    var body: some View {
        TimelineView(.periodic(from:.now,by:5)) {context in content(at:context.date)}
            .task {if !snapshotMode {await store.load()}}
    }
    // The compact action and regression use the same destination/store path.
    @discardableResult func openDetail(in navigation: Nav) -> CanonicalFabricDetailView {
        let detail = CanonicalFabricDetailView(store:store)
        navigation.push(detail)
        return detail
    }
    private func content(at now: Date) -> some View {
        let projection = store.presentation(compact:compact,at:now)
        return VStack(alignment:.leading,spacing:8) {
            Text("Tasks · messages · host health").sirsiFont(.caption,weight:.semibold)
            if let board = store.board {
                Text(store.current(at:now) ? "Current fabric observation" : "Retained fabric observation · current state unavailable")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                ForEach(Array(board.headlineLines.enumerated()),id:\.offset) {index,line in
                    Text(line).sirsiFont(.caption).foregroundStyle(board.roleColor(board.headlineRoleTokens[index],dark:colorScheme == .dark))
                }
                Text("Needs attention").sirsiFont(.caption,weight:.semibold)
                ForEach(Array(projection.attention.enumerated()),id:\.offset) {_,line in
                    Text(line).sirsiFont(.caption).foregroundStyle(.secondary)
                }
                if projection.offersDetail {
                    Button {openDetail(in:nav)} label: {Text("Open full Fabric detail").sirsiFont(.caption)}
                        .buttonStyle(.plain)
                }
                if projection.showsDetails {
                    ForEach(board.lanes) {lane in Text("\(lane.agent) · \(lane.state_label) · \(lane.open) open tasks").sirsiFont(.caption).foregroundStyle(board.roleColor(lane.role,dark:colorScheme == .dark))}
                    Text("Host health — separate from tasks/messages").sirsiFont(.caption,weight:.semibold)
                    ForEach(board.health.issues,id:\.self) {Text($0).sirsiFont(.caption)}
                    Text("RAM availability complement: \(board.health.ram_pct.map{String($0)+"%"} ?? "Unavailable") · Swap: \(board.health.swap.map{String($0)+" bytes"} ?? "Unavailable")").sirsiFont(.caption)
                    Text("Capacity: \(board.health.total_ram.map{String($0)+" bytes"} ?? "Unavailable") · CPU cores: \(board.health.cpu_cores.map{String($0)} ?? "Unavailable") · Repository dirty: \(board.health.git_dirty.map{String($0)} ?? "Unavailable")").sirsiFont(.caption)
                    Text("Engine: \(board.health.engine) · \(board.health.engine_state) (session observation, not readiness proof)").sirsiFont(.caption)
                    Text(board.activity_seeded ? "Observed task transitions" : "Baseline only — transitions not observed by this one-shot producer")
                        .sirsiFont(.caption,weight:.semibold)
                    ForEach(Array(board.activity.enumerated()),id:\.offset) {_,event in
                        Text("\(event.at) · \(event.agent) · \(event.task_id) · \(event.from) → \(event.to)").sirsiFont(.caption)
                    }
                }
                Text("\(board.generated_at) · \(board.build)").sirsiFont(.caption).foregroundStyle(.secondary)
            } else {
                Text("Fabric unavailable — no task or message counts observed.").sirsiFont(.caption)
                ForEach(Array(projection.attention.enumerated()),id:\.offset) {_,line in Text(line).sirsiFont(.caption)}
            }
            Button(store.loading ? "Reading fabric…" : "Refresh tasks and messages") {Task {await store.load()}}
                .disabled(store.loading).buttonStyle(.bordered)
        }
        .frame(maxWidth:.infinity,alignment:.leading)
    }
}

// Custom Nav has no automatic toolbar/back or scrolling. Keep Back outside the
// scroll region; long canonical content, bottom refresh and provenance stay in it.
@MainActor struct CanonicalFabricDetailView: View {
    @ObservedObject var store: FabricProjectionStore
    var content: CanonicalFabricView {CanonicalFabricView(store:store)}
    var body: some View {
        VStack(alignment:.leading,spacing:0) {
            BackBar(title:"Fabric")
            ScrollView {content.padding(14)}
        }
    }
}
