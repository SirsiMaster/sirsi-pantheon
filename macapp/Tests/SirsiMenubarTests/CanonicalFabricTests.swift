import SwiftUI
import XCTest
@testable import SirsiMenubar

@MainActor final class CanonicalFabricTests: XCTestCase {
    private let at = "2026-10-25T12:00:00Z"
    private func fixture() -> [String:Any] {
        ["work":["done":0,"open":0,"in_progress":0,"assigned_not_started":0,"stalled":0,"blocked":0,"total":0],
         "messages":["open":0,"stranded":0,"oldest_age_seconds":0],"lanes":[],
         "health":["ram_pct":NSNull(),"swap":NSNull(),"total_ram":NSNull(),"cpu_cores":NSNull(),"git_dirty":NSNull(),"engine":"unknown","engine_state":"unavailable","issues":[]],
         "activity":[],"activity_seeded":false,"runtime_available":true,"build":"fixture","generated_at":at]
    }
    private func reply(_ object: [String:Any], exit: Int32 = 0) throws -> SirsiEngine.JSONCommandRead {
        .init(data:try JSONSerialization.data(withJSONObject:object),exitStatus:exit,timedOut:false,launchError:nil)
    }
    func testKnownEmptyAndUnavailableRetainDistinctTruthAndLastGood() async throws {
        let good = try reply(fixture()), failed = try reply(fixture(),exit:1)
        var calls = 0
        let store = FabricProjectionStore {args in
            XCTAssertEqual(args,["router","fabric","--json"]); calls += 1
            return calls == 1 ? good : failed
        }
        await store.load()
        XCTAssertEqual(store.board?.work.total,0); XCTAssertNil(store.board?.health.swap)
        XCTAssertNil(store.board?.health.ram_pct); XCTAssertNil(store.error)
        XCTAssertTrue(store.current(at:try XCTUnwrap(pantheonObservationDate(at)).addingTimeInterval(1)))
        XCTAssertFalse(store.board?.activity_seeded ?? true)
        await store.load()
        XCTAssertNotNil(store.error); XCTAssertEqual(store.board?.generated_at,at)
        XCTAssertFalse(store.current(at:try XCTUnwrap(pantheonObservationDate(at)).addingTimeInterval(1)))
        XCTAssertEqual(calls,2); XCTAssertFalse(store.loading)
    }
    func testMalformedOrIncompleteProjectionIsUnavailableNotEmpty() async throws {
        var object = fixture(); object.removeValue(forKey:"messages")
        let bad = try reply(object)
        let store = FabricProjectionStore {_ in bad}
        await store.load(); XCTAssertNil(store.board); XCTAssertNotNil(store.error)
        object = fixture(); object["work"] = ["total":-1]
        let malformed = try reply(object)
        let other = FabricProjectionStore {_ in malformed};await other.load()
        XCTAssertNil(other.board);XCTAssertNotNil(other.error)
    }
    func testCancelledReadCannotPublishLateFabricOrLeaveLoading() async throws {
        let good = try reply(fixture())
        var held: CheckedContinuation<SirsiEngine.JSONCommandRead,Never>?
        let store = FabricProjectionStore {_ in await withCheckedContinuation {held=$0}}
        let task = Task {@MainActor in await store.load()}
        for _ in 0..<1000 where held == nil {await Task.yield()}
        XCTAssertNotNil(held);task.cancel();held?.resume(returning:good);held=nil;await task.value
        XCTAssertNil(store.board);XCTAssertNil(store.error);XCTAssertFalse(store.loading)
    }
    func testEngineOwnsOneCanonicalProjectionForCompactAndFullNativeViews() throws {
        var reads=0
        let store=FabricProjectionStore {_ in reads+=1;return .init(data:Data(),exitStatus:1,timedOut:false,launchError:nil)}
        let engine=SirsiEngine(fabricProjection:store)
        let compact=CanonicalFabricView(store:engine.fabricProjection,compact:true)
        let full=CanonicalFabricView(store:engine.fabricProjection)
        let fleet=FleetView(engine:engine)
        XCTAssertTrue(compact.store === store);XCTAssertTrue(full.store === store)
        XCTAssertTrue(fleet.engine.fabricProjection === store);XCTAssertEqual(reads,0,"Constructing projections must not run the CLI")
    }
    func testActualGoEndpointFabricWireFeedsNativeWithoutReaggregation() async throws {
        guard let path=ProcessInfo.processInfo.environment["PANTHEON_TEST_FABRIC_WIRE_OUTPUT"] else {
            throw XCTSkip("Main must supply actual Go endpoint fixture output")
        }
        let data=try Data(contentsOf:URL(fileURLWithPath:path));XCTAssertLessThanOrEqual(data.count,8*1024*1024)
        let store=FabricProjectionStore {_ in .init(data:data,exitStatus:0,timedOut:false,launchError:nil)}
        await store.load();let board=try XCTUnwrap(store.board)
        XCTAssertEqual(board.work.total,2);XCTAssertEqual(board.messages.open,1)
        XCTAssertEqual(board.headlineLines,["Completed / in flight: 1 / 1 tasks","In progress / assigned: 1 / 0 tasks","Stalled / blocked: 0 / 0 tasks","Messages pending: 1 inbox messages · 0 stranded"])
        XCTAssertEqual(board.health.swap,0);XCTAssertEqual(board.health.engine,"inert-engine")
        XCTAssertEqual(board.activity.count,1);XCTAssertEqual(board.activity[0].from,"pending");XCTAssertEqual(board.activity[0].to,"in-progress")
        XCTAssertEqual(board.lanes.first?.state,"IDLE_WITH_WORK");XCTAssertEqual(board.lanes.first?.state_label,"IDLE with work")
        XCTAssertTrue(board.activity_seeded);XCTAssertTrue(board.runtime_available)
    }
    func testCompactUsesVisibleBoundedAttentionAndSameStoreDetail() async throws {
        var object = fixture()
        object["runtime_available"] = false
        var health = try XCTUnwrap(object["health"] as? [String:Any])
        health["issues"] = ["swap observation unavailable"]
        object["health"] = health
        let good = try reply(object), failed = try reply(object,exit:1)
        var calls = 0
        let store = FabricProjectionStore {_ in calls += 1;return calls == 1 ? good : failed}
        let now = try XCTUnwrap(pantheonObservationDate(at))
        await store.load()
        let compact = store.presentation(compact:true,at:now)
        XCTAssertTrue(compact.attention.contains("Runtime classification unavailable"))
        XCTAssertTrue(compact.attention.contains("swap observation unavailable"))
        XCTAssertFalse(compact.showsDetails);XCTAssertTrue(compact.offersDetail)
        XCTAssertTrue(store.presentation(compact:false,at:now).showsDetails)
        await store.load()
        let retained = store.presentation(compact:true,at:now)
        XCTAssertTrue(retained.attention.contains { $0.contains("refresh unavailable") })
        XCTAssertTrue(retained.attention.contains { $0.contains("Retained observation") })
        XCTAssertEqual(store.board?.generated_at,at)
        let emptyReply = try reply(fixture())
        let empty = FabricProjectionStore {_ in emptyReply};await empty.load()
        XCTAssertEqual(empty.presentation(compact:true,at:now).attention,["No attention entries supplied; absence is not a host-health verdict"])
        XCTAssertFalse(empty.presentation(compact:true,at:now).showsDetails)
    }

}
