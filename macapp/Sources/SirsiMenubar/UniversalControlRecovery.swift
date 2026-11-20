import Foundation

// The adapter must identify only real, selectable Universal Control peers.
// A peer name alone is not sufficient unless it is unique in the current menu.
struct UniversalControlPeer: Equatable {
    let id: String
    // nil means macOS did not provide a reliable connection state.
    let linked: Bool?
    let selectable: Bool
}

@MainActor
protocol UniversalControlPeerAdapter {
    func peers() async throws -> [UniversalControlPeer]
    // Re-resolve one exact selectable peer and check this state immediately
    // before selecting it. Never fall back to a similarly named peer.
    func selectPeer(id: String, expectedLinked: Bool) async throws
}

enum UniversalControlRecoveryResult: Equatable {
    case linkedNeedsInputVerification
    case targetUnavailable
    case stateChangedAfterPossibleDisconnect
    case stateChangedAfterPossibleConnection
    case alreadyRunning
    case cancelled
    case cancelledAfterPossibleDisconnect
    case cancelledAfterPossibleConnection
    case failed
    case failedAfterPossibleDisconnect
    case failedAfterPossibleConnection
}

// No global process restart. Always re-read the exact peer before the next
// mutation and report an observed link, not verified pointer/keyboard input.
@MainActor
final class UniversalControlRecovery {
    private static var running = false

    func reconnect(id: String, adapter: UniversalControlPeerAdapter) async -> UniversalControlRecoveryResult {
        guard !Self.running else { return .alreadyRunning }
        guard !id.isEmpty else { return .targetUnavailable }
        Self.running = true
        defer { Self.running = false }
        var mayHaveDisconnected = false
        var mayHaveConnected = false
        do {
            try Task.checkCancellation()
            guard let original = try await uniquePeer(id: id, adapter: adapter),
                  original.selectable, let wasLinked = original.linked else {
                return .targetUnavailable
            }
            if wasLinked {
                try Task.checkCancellation()
                // An adapter error may arrive after the selection took effect.
                mayHaveDisconnected = true
                try await adapter.selectPeer(id: id, expectedLinked: true)
                guard let disconnected = try await uniquePeer(id: id, adapter: adapter),
                      disconnected.selectable, disconnected.linked == false else { return .stateChangedAfterPossibleDisconnect }
            }
            try Task.checkCancellation()
            // The action can succeed even if the adapter then reports an error.
            mayHaveConnected = true
            try await adapter.selectPeer(id: id, expectedLinked: false)
            guard let linked = try await uniquePeer(id: id, adapter: adapter), linked.linked == true else {
                return mayHaveDisconnected ? .stateChangedAfterPossibleDisconnect : .stateChangedAfterPossibleConnection
            }
            try Task.checkCancellation()
            return .linkedNeedsInputVerification
        } catch is CancellationError {
            if mayHaveDisconnected { return .cancelledAfterPossibleDisconnect }
            return mayHaveConnected ? .cancelledAfterPossibleConnection : .cancelled
        } catch {
            if mayHaveDisconnected { return .failedAfterPossibleDisconnect }
            return mayHaveConnected ? .failedAfterPossibleConnection : .failed
        }
    }

    private func uniquePeer(id: String, adapter: UniversalControlPeerAdapter) async throws -> UniversalControlPeer? {
        let matches = try await adapter.peers().filter { $0.id == id }
        return matches.count == 1 ? matches[0] : nil
    }
}
