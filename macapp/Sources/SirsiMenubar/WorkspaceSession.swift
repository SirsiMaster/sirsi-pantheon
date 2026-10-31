import SwiftUI
import Combine

// UI identity only: no hidden workspace views, polling tasks or persisted
// operations. Adapted from the separately preserved worker navigation patch.
@MainActor final class PantheonWorkspaceSession: ObservableObject {
    static let selectionKey = "pantheon.workspace"
    private let defaults: UserDefaults
    private let stacks: [PantheonWorkspace: Nav]
    // Only Ma'at's root System One pane retains a viewport; details and the
    // separate recovery pane do not share this owner.
    private var maatPrimaryRootScrollOffset: CGFloat = 0
    private var maatPrimaryScrollOwnerID = UUID()
    private var observations: [AnyCancellable] = []
    private let apolloRootDraft = ApolloPlannerDraft()
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
            nav.objectWillChange.sink { [weak self] _ in
                guard let self else { return }
                if nav === self.stacks[.maat] {
                    self.maatPrimaryScrollOwnerID = UUID()
                }
                self.objectWillChange.send()
            }
        }
    }

    @MainActor func apolloPlanner(engine: SirsiEngine) -> ApolloRunPlannerView {
        apolloRootDraft.enterProject(engine.projectRoot)
        return ApolloRunPlannerView(engine: engine, draft: apolloRootDraft)
    }

    var navigation: Nav { navigation(for: section) }
    func navigation(for workspace: PantheonWorkspace) -> Nav { stacks[workspace]! }

    func maatPrimaryRootScrollPosition(for navigation: Nav) -> Binding<CGFloat>? {
        let navigationID = ObjectIdentifier(navigation)
        guard section == .maat, navigationID == ObjectIdentifier(stacks[.maat]!), navigation.atRoot else { return nil }
        let ownerID = maatPrimaryScrollOwnerID
        return Binding(
            get: { self.maatPrimaryRootScrollOffset },
            set: { value in
                // A native bounds callback may already hold this Binding while
                // SwiftUI tears down the old section. Revalidate ownership at
                // write time so that late unmount offsets cannot replace the
                // retained root position or write from a pushed detail.
                guard self.section == .maat,
                      ObjectIdentifier(self.stacks[.maat]!) == navigationID,
                      self.stacks[.maat]!.atRoot,
                      self.maatPrimaryScrollOwnerID == ownerID,
                      value.isFinite else { return }
                self.maatPrimaryRootScrollOffset = max(0, value)
            }
        )
    }

    func select(_ workspace: PantheonWorkspace) {
        guard section != workspace else { return }
        maatPrimaryScrollOwnerID = UUID()
        section = workspace
        defaults.set(workspace.rawValue, forKey: Self.selectionKey)
    }

    func open<Detail: View>(_ workspace: PantheonWorkspace, detail: Detail) {
        select(workspace)
        navigation(for: workspace).push(detail)
    }
}
