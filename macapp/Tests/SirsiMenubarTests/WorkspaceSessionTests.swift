import XCTest
import SwiftUI
@testable import SirsiMenubar

final class WorkspaceSessionTests: XCTestCase {
    private func defaults() -> UserDefaults {
        let name = "WorkspaceSessionTests." + UUID().uuidString
        let store = UserDefaults(suiteName: name)!
        addTeardownBlock { UserDefaults(suiteName: name)?.removePersistentDomain(forName: name) }
        return store
    }

    @MainActor
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

    func testBackForwardPreservesFrameIdentityAndNewPushDropsForwardBranch() throws {
        let nav = Nav()
        nav.push(Text("First"))
        nav.push(Text("Second"))
        nav.push(Text("Third"))
        let firstID = nav.stack[0].id
        let secondID = nav.stack[1].id
        let thirdID = nav.stack[2].id

        nav.pop()
        nav.pop()
        XCTAssertEqual(nav.stack.map(\.id), [firstID])
        XCTAssertTrue(nav.canGoForward)
        nav.forward()
        XCTAssertEqual(nav.stack.map(\.id), [firstID, secondID])
        XCTAssertNotEqual(nav.stack.last?.id, thirdID)

        nav.push(Text("Branch"))
        let branchID = try XCTUnwrap(nav.stack.last?.id)
        XCTAssertEqual(nav.stack.map(\.id), [firstID, secondID, branchID])
        XCTAssertFalse(nav.canGoForward)
        nav.forward()
        XCTAssertEqual(nav.stack.map(\.id), [firstID, secondID, branchID])
        XCTAssertEqual(Set(nav.stack.map(\.id)).count, nav.stack.count)
    }

    @MainActor
    func testForwardHistoryAndHomeResetStayWithinEachWorkspace() {
        let session = PantheonWorkspaceSession(defaults: defaults())
        let ra = session.navigation(for: .ra)
        let maat = session.navigation(for: .maat)
        ra.push(Text("Ra root detail"))
        ra.push(Text("Ra evidence"))
        let raEvidenceID = ra.stack.last!.id
        ra.pop()
        maat.push(Text("Ma'at root detail"))
        maat.push(Text("Ma'at case"))
        let maatCaseID = maat.stack.last!.id
        maat.pop()

        session.select(.ra)
        XCTAssertTrue(ra.canGoForward)
        XCTAssertTrue(maat.canGoForward)
        ra.forward()
        XCTAssertEqual(ra.stack.last?.id, raEvidenceID)
        XCTAssertEqual(maat.stack.count, 1)
        XCTAssertTrue(maat.canGoForward)

        session.select(.maat)
        maat.forward()
        XCTAssertEqual(session.navigation.stack.last?.id, maatCaseID)
        maat.popToRoot() // the workspace Home operation clears both directions
        XCTAssertTrue(maat.atRoot)
        XCTAssertFalse(maat.canGoBack)
        XCTAssertFalse(maat.canGoForward)
        XCTAssertEqual(ra.stack.last?.id, raEvidenceID)
        XCTAssertFalse(ra.atRoot)
    }

    @MainActor
    func testMaatPrimaryViewportIsRootOnlyAndRetainsItsIdentityAcrossDetails() throws {
        let store = defaults()
        let session = PantheonWorkspaceSession(defaults: store)
        let maat = session.navigation(for: .maat)
        session.select(.maat)
        let rootPosition = try XCTUnwrap(session.maatPrimaryRootScrollPosition(for: maat))
        rootPosition.wrappedValue = 684.5

        session.select(.horus)
        XCTAssertNil(session.maatPrimaryRootScrollPosition(for: session.navigation(for: .horus)))
        session.select(.maat)
        XCTAssertEqual(session.maatPrimaryRootScrollPosition(for: maat)?.wrappedValue, 684.5)

        maat.push(Text("Ma'at detail"))
        XCTAssertNil(session.maatPrimaryRootScrollPosition(for: maat),
                     "detail navigation frames do not inherit the root viewport owner")
        maat.pop()
        XCTAssertEqual(session.maatPrimaryRootScrollPosition(for: maat)?.wrappedValue, 684.5)

        let restored = PantheonWorkspaceSession(defaults: store)
        XCTAssertEqual(restored.section, .maat)
        XCTAssertEqual(restored.maatPrimaryRootScrollPosition(for: restored.navigation(for: .maat))?.wrappedValue, 0,
                       "root scroll offset stays in the window session rather than persisting across launches")
    }

    func testMaatPrimaryScrollRestoresClampedOffsetOnceAfterReadyLayout() {
        var restoration = MaatRootScrollRestoration(savedOffset: 600)
        XCTAssertNil(restoration.layoutChanged(maximumOffset: 300, ready: false))
        XCTAssertEqual(restoration.layoutChanged(maximumOffset: 300, ready: true), 300)
        XCTAssertNil(restoration.layoutChanged(maximumOffset: 700, ready: true))
    }

    func testMaatPrimaryBoundsCallbackTracksFractionalMovementBeforeDeferredRestore() {
        var restoration = MaatRootScrollRestoration(savedOffset: 600)
        restoration.recordOffset(0)
        XCTAssertNil(restoration.layoutChanged(maximumOffset: 300, ready: false))
        XCTAssertEqual(restoration.boundsChanged(to: 0.5), 0.5)
        XCTAssertEqual(restoration.boundsChanged(to: 1), 1)
        XCTAssertEqual(restoration.boundsChanged(to: 1.5), 1.5)
        XCTAssertNil(restoration.boundsChanged(to: 1.5), "unchanged callbacks do not alter the accepted baseline")
        XCTAssertTrue(restoration.resolved)
        XCTAssertNil(restoration.layoutChanged(maximumOffset: 300, ready: true))
    }

    @MainActor
    func testRestoreKeepsOnlySectionIdentityWithoutReplayingDrilldowns() {
        let store = defaults()
        let first = PantheonWorkspaceSession(defaults: store)
        first.select(.apollo)
        first.navigation.push(Text("Unpersisted operation"))
        let restored = PantheonWorkspaceSession(defaults: store)
        XCTAssertEqual(restored.section, .apollo)
        XCTAssertTrue(restored.navigation.atRoot)
    }

    @MainActor
    func testUnknownStoredWorkspaceRecoversToOverview() {
        let store = defaults()
        store.set("removed-future-workspace", forKey: PantheonWorkspaceSession.selectionKey)
        let session = PantheonWorkspaceSession(defaults: store)
        XCTAssertEqual(session.section, .command)
        XCTAssertTrue(session.navigation.atRoot)
        XCTAssertEqual(Set(PantheonWorkspace.allCases.map { ObjectIdentifier(session.navigation(for: $0)) }).count,
                       PantheonWorkspace.allCases.count)
    }

    @MainActor
    func testLegacyWorkerSelectionIsReadableWithoutOverridingCurrentSelection() {
        let store = defaults()
        store.set("maat", forKey: "PantheonWorkspaceSelection")
        XCTAssertEqual(PantheonWorkspaceSession(defaults: store).section, .maat)
        store.set("mercury", forKey: PantheonWorkspaceSession.selectionKey)
        XCTAssertEqual(PantheonWorkspaceSession(defaults: store).section, .mercury)
    }

    @MainActor
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

    @MainActor
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
