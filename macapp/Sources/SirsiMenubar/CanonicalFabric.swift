import SwiftUI

// Decodes the ONE Go FabricBoard, not node-status or per-view arithmetic.
struct CanonicalFabricBoard: Decodable {
    struct Work: Decodable { let done, open, in_progress, assigned_not_started, stalled, blocked, total: Int }
    struct Messages: Decodable { let open, stranded: Int; let oldest_age_seconds: Double }
    struct Lane: Decodable, Identifiable {
        let agent, state, state_label: String
        let open, active, stalled, blocked: Int
        var id: String { agent }
    }
    struct Health: Decodable {
        let ram_pct: Double?; let swap, total_ram: Int64?; let cpu_cores: Int?; let git_dirty: Bool?
        let engine, engine_state: String; let issues: [String]
    }
    struct Activity: Decodable { let at, agent, task_id, from, to: String }
    let work: Work; let messages: Messages; let lanes: [Lane]; let health: Health
    let activity: [Activity]; let activity_seeded, runtime_available: Bool
    let build, generated_at: String
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
    var compact = false
    @Environment(\.snapshotMode) private var snapshotMode
    var body: some View {
        TimelineView(.periodic(from:.now,by:5)) {context in content(at:context.date)}
            .task {if !snapshotMode {await store.load()}}
    }
    private func content(at now: Date) -> some View {
        VStack(alignment:.leading,spacing:8) {
            Text("Tasks · messages · host health").sirsiFont(.caption,weight:.semibold)
            if let error = store.error { Text(error).sirsiFont(.caption).foregroundStyle(.orange) }
            if let board = store.board {
                Text(store.current(at:now) ? "Current fabric observation" : "Retained fabric observation · current state unavailable")
                    .sirsiFont(.caption).foregroundStyle(.secondary)
                ForEach(board.headlineLines,id:\.self) {Text($0).sirsiFont(.caption)}
                if !compact {
                    Text("Needs attention").sirsiFont(.caption,weight:.semibold)
                    if !board.runtime_available {Text("Runtime classification unavailable").sirsiFont(.caption)}
                    ForEach(board.health.issues,id:\.self) {Text($0).sirsiFont(.caption)}
                    ForEach(board.lanes) {lane in Text("\(lane.agent) · \(lane.state_label) · \(lane.open) open tasks").sirsiFont(.caption)}
                    Text("Host health — separate from tasks/messages").sirsiFont(.caption,weight:.semibold)
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
            } else {Text("Fabric unavailable — no task or message counts observed.").sirsiFont(.caption)}
            Button(store.loading ? "Reading fabric…" : "Refresh tasks and messages") {Task {await store.load()}}
                .disabled(store.loading).buttonStyle(.bordered)
        }
        .frame(maxWidth:.infinity,alignment:.leading)
    }
}
