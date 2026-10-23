import SwiftUI
import Combine

// UI identity only: no hidden workspace views, polling tasks or persisted
// operations. Adapted from the separately preserved worker navigation patch.
final class PantheonWorkspaceSession: ObservableObject {
    static let selectionKey = "pantheon.workspace"
    private let defaults: UserDefaults
    private let stacks: [PantheonWorkspace: Nav]
    private var observations: [AnyCancellable] = []
    @Published private(set) var section: PantheonWorkspace

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        let stored = defaults.string(forKey: Self.selectionKey)
            ?? defaults.string(forKey: "PantheonWorkspaceSelection")
        section = stored.flatMap(PantheonWorkspace.init(rawValue:)) ?? .command
        stacks = Dictionary(uniqueKeysWithValues: PantheonWorkspace.allCases.map { ($0, Nav()) })
        // The active header and detail both depend on navigation state.
        // Forward mutations without keeping any inactive view tree mounted.
        observations = stacks.values.map { nav in
            nav.objectWillChange.sink { [weak self] _ in self?.objectWillChange.send() }
        }
    }

    var navigation: Nav { navigation(for: section) }
    func navigation(for workspace: PantheonWorkspace) -> Nav { stacks[workspace]! }

    func select(_ workspace: PantheonWorkspace) {
        guard section != workspace else { return }
        section = workspace
        defaults.set(workspace.rawValue, forKey: Self.selectionKey)
    }

    func open<Detail: View>(_ workspace: PantheonWorkspace, detail: Detail) {
        select(workspace)
        navigation(for: workspace).push(detail)
    }
}
