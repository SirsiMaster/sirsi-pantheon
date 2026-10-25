import Foundation
import Combine

// Workspace state shares the same invocation lifetime as its structured child
// reads. The monitor separately guards its own canonical projection. An older
// canceled invocation may still be joining children when a new one starts.
@MainActor final class MercuryWorkspaceRefresh: ObservableObject {
    @Published private(set) var loading: Bool
    private var generation = UUID()

    init(loading: Bool = true) { self.loading = loading }

    func run<Value>(read: @MainActor () async -> Value, publish: @MainActor (Value) -> Void) async {
        guard !Task.isCancelled else { return }
        let current = UUID()
        generation = current
        loading = true
        defer { if generation == current { loading = false } }
        let value = await read()
        guard generation == current, !Task.isCancelled else { return }
        publish(value)
    }
}
