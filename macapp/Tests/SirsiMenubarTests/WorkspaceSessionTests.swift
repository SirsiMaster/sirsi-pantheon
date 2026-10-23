import XCTest
import SwiftUI
@testable import SirsiMenubar

final class WorkspaceSessionTests: XCTestCase {
    private func defaults() -> UserDefaults {
        let name = "WorkspaceSessionTests." + UUID().uuidString
        let store = UserDefaults(suiteName: name)!
        addTeardownBlock { store.removePersistentDomain(forName: name) }
        return store
    }

    func testSwitchingSectionsPreservesIndependentDrilldowns() {
        let session = PantheonWorkspaceSession(defaults: defaults())
        session.select(.ra)
        let ra = session.navigation
        ra.push(Text("Selected Ra task"))
        let task = ra.stack.last!.id
        session.select(.maat)
        XCTAssertTrue(session.navigation.atRoot)
        session.navigation.push(Text("Selected evidence"))
        session.select(.ra)
        XCTAssertTrue(session.navigation === ra)
        XCTAssertEqual(session.navigation.stack.last?.id, task)
        session.select(.ra)
        XCTAssertEqual(session.navigation.stack.last?.id, task)
        ra.popToRoot()
        XCTAssertFalse(session.navigation(for: .maat).atRoot)
    }

    func testRestoreKeepsOnlySectionIdentityWithoutReplayingDrilldowns() {
        let store = defaults()
        let first = PantheonWorkspaceSession(defaults: store)
        first.select(.apollo)
        first.navigation.push(Text("Unpersisted operation"))
        let restored = PantheonWorkspaceSession(defaults: store)
        XCTAssertEqual(restored.section, .apollo)
        XCTAssertTrue(restored.navigation.atRoot)
    }

    func testUnknownStoredWorkspaceRecoversToOverview() {
        let store = defaults()
        store.set("removed-future-workspace", forKey: PantheonWorkspaceSession.selectionKey)
        let session = PantheonWorkspaceSession(defaults: store)
        XCTAssertEqual(session.section, .command)
        XCTAssertTrue(session.navigation.atRoot)
        XCTAssertEqual(Set(PantheonWorkspace.allCases.map { ObjectIdentifier(session.navigation(for: $0)) }).count,
                       PantheonWorkspace.allCases.count)
    }

    func testLegacyWorkerSelectionIsReadableWithoutOverridingCurrentSelection() {
        let store = defaults()
        store.set("maat", forKey: "PantheonWorkspaceSelection")
        XCTAssertEqual(PantheonWorkspaceSession(defaults: store).section, .maat)
        store.set("mercury", forKey: PantheonWorkspaceSession.selectionKey)
        XCTAssertEqual(PantheonWorkspaceSession(defaults: store).section, .mercury)
    }

    func testNavigationMutationPublishesToHeaderAndDetail() {
        let session = PantheonWorkspaceSession(defaults: defaults())
        var mutations = 0
        let observation = session.objectWillChange.sink { mutations += 1 }
        session.navigation.push(Text("Detail"))
        XCTAssertEqual(mutations, 1)
        session.navigation.popToRoot()
        XCTAssertEqual(mutations, 2)
        withExtendedLifetime(observation) {}
    }

    func testCrossWorkspaceDetailBelongsToDestinationNotPreviousStack() {
        let session = PantheonWorkspaceSession(defaults: defaults())
        let overview = session.navigation
        session.open(.maat, detail: Text("Owner decisions"))
        XCTAssertEqual(session.section, .maat)
        XCTAssertTrue(overview.atRoot)
        XCTAssertFalse(session.navigation.atRoot)
        session.select(.command)
        XCTAssertTrue(session.navigation.atRoot)
        XCTAssertFalse(session.navigation(for: .maat).atRoot)
    }
}
