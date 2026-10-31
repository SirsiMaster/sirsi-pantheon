import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayPublicRecoveryTests: XCTestCase {
    // Shared wire fixture checked against the actual controller by Go tests.
    // Empty authority is not a never-started certificate.
    private func emptyStatus(_ _:LegacyRelayFixture)throws->CommandExecution {
        CommandExecution(stdout:try emptyWire(),exitCode:0,childState:.exitConfirmed)
    }
    private func emptyWire()throws->Data {
        let root=URL(fileURLWithPath:#filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        return try Data(contentsOf:root.appendingPathComponent("internal/setup/testdata/legacy-app-empty-status.json"))
    }
    private func emptyTransaction()throws->[String:Any] {
        let view=try XCTUnwrap(JSONSerialization.jsonObject(with:emptyWire()) as? [String:Any])
        return try XCTUnwrap(view["status"] as? [String:Any])
    }
    private func fence(_ f:LegacyRelayFixture)throws->CommandExecution {
        try f.result(["ticket":f.ticket(),"disposition":"future-execution-fenced","status":emptyTransaction(),
                      "safe_for_new_preview":true,"past_outcome":"never-admitted"])
    }

    func testUnreadablePublicStateAndEmptyStatusRequireConfirmedRebindFenceAndNewConsent()async throws {
        for kind in ["malformed","oversized","invalid-ticket"] {
            let f=LegacyRelayFixture()
            switch kind {
            case "oversized":f.saved=Data(repeating:65,count:256*1024+1)
            case "invalid-ticket":
                f.saved=Data(#"{"schema":1,"outstanding":{"operation":"confirm","ticketJSON":"{}","originalPreview":""},"lifecycleDebt":false}"#.utf8)
            default:f.saved=Data("unreadable public bytes".utf8)
            }
            let original=f.saved
            var operations=[String]()
            let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{args,_,_,_ in
                operations.append(args[2])
                if args[2]=="preview"{return try! f.preview()}
                if args[2]=="resolve-operation"{return try! self.fence(f)}
                return try! self.emptyStatus(f)
            },storage:f.storage)
            model.select(f.artifact)
            await model.readStatus();await model.review()
            XCTAssertTrue(model.publicStateUnreadable);XCTAssertTrue(model.outcomeUnknown)
            XCTAssertEqual(f.saved,original);XCTAssertEqual(operations,["status"])
            await model.inspectPublicBinding()
            let approval=try XCTUnwrap(model.publicBindingApproval)
            XCTAssertNil(approval.ticketJSON);XCTAssertEqual(f.saved,original)
            let reconcile=try XCTUnwrap(model.rebindPublicState(approval))
            XCTAssertNil(model.rebindPublicState(approval));await reconcile.value
            let retained=try XCTUnwrap(model.outstanding)
            XCTAssertFalse(model.publicStateUnreadable);XCTAssertTrue(model.outcomeUnknown)
            XCTAssertNil(model.confirmation());await model.review()
            XCTAssertEqual(operations,["status","status","preview"])
            try await XCTUnwrap(model.resolveOutstanding(retained)).value
            XCTAssertFalse(model.outcomeUnknown);XCTAssertFalse(model.lifecycleDebt)
            XCTAssertNotNil(model.unreadableReceipt)
            XCTAssertEqual(model.unreadableReceipt?.bytes,original?.count)
            let saved=try JSONDecoder().decode(LegacyRelaySavedOperationState.self,from:try XCTUnwrap(f.saved))
            XCTAssertEqual(saved.unreadableReceipt,model.unreadableReceipt)
            XCTAssertNotNil(saved.resolved?.receipt?.issue)
            await model.review()
            XCTAssertNotNil(model.confirmation(),"Only a verified fence followed by fresh preview allows NEW consent")
            XCTAssertFalse(operations.contains("confirm"));XCTAssertFalse(operations.contains("recover"))
        }
    }

    func testRecoveryAdoptsExactExistingControllerBindingOnlyAfterConfirmation()async throws {
        for state in ["reserved","running","terminal","fenced"] {
            let f=LegacyRelayFixture();f.saved=Data("bad".utf8)
            let status=try f.result(["status":f.transaction(),"admission":["schema":1,"high_water":state=="fenced" ? 2:1,"state":state],"ticket_json":f.ticketJSON()])
            let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{_,_,_,_ in status},storage:f.storage)
            await model.inspectPublicBinding()
            XCTAssertNil(model.outstanding);XCTAssertEqual(f.saved,Data("bad".utf8))
            let approved=try XCTUnwrap(model.publicBindingApproval)
            try await XCTUnwrap(model.rebindPublicState(approved)).value
            XCTAssertEqual(model.outstanding?.ticketJSON,try f.ticketJSON())
            XCTAssertTrue(model.outcomeUnknown);XCTAssertFalse(model.publicStateUnreadable)
            XCTAssertNil(model.confirmation())
        }
    }

    func testPublicWriteOrReadbackFailureNeverUnlocksConsentAndExactResolverRemainsUsable()async throws {
        for failure in ["write","readback"] {
            let f=LegacyRelayFixture();f.saved=Data("bad".utf8)
            var fail=true
            let storage=LegacyRelayOperationStorage(read:{f.saved},write:{data in
                if fail {if failure=="write"{throw NSError(domain:"FixtureWrite",code:1)};return}
                f.saved=data
            })
            let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{args,_,_,_ in
                if args[2]=="preview"{return try! f.preview()}
                if args[2]=="resolve-operation"{return try! self.fence(f)}
                return try! self.emptyStatus(f)
            },storage:storage)
            model.select(f.artifact);await model.inspectPublicBinding()
            try await XCTUnwrap(model.rebindPublicState(try XCTUnwrap(model.publicBindingApproval))).value
            let retained=try XCTUnwrap(model.outstanding)
            XCTAssertTrue(model.publicStateUnreadable);XCTAssertTrue(model.outcomeUnknown)
            XCTAssertNotNil(model.error);XCTAssertNil(model.confirmation())
            fail=false
            try await XCTUnwrap(model.resolveOutstanding(retained)).value
            XCTAssertFalse(model.outcomeUnknown);XCTAssertFalse(model.publicStateUnreadable)
            XCTAssertNotNil(model.lastResolved?.receipt)
        }
    }

    func testChangedOldStatusCannotReplaceNewerPersistedBinding()async throws {
        let f=LegacyRelayFixture();f.saved=Data("bad".utf8)
        var calls=0
        let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{_,_,_,_ in calls+=1;return try! f.status()},storage:f.storage)
        let b=LegacyRelayOutstandingOperation(operation:"confirm",ticketJSON:try f.ticketJSON(generation:2,id:String(repeating:"e",count:32)),originalPreview:Data())
        f.saved=try JSONEncoder().encode(LegacyRelaySavedOperationState(outstanding:b,resolved:nil,lifecycleDebt:true,lastTransaction:nil))
        await model.inspectPublicBinding()
        XCTAssertEqual(calls,0,"Recover valid stored B rather than inspecting/replacing it with status A")
        XCTAssertEqual(model.outstanding,b)
        await model.readStatus()
        XCTAssertEqual(model.outstanding,b);XCTAssertTrue(model.outcomeUnknown)
    }

    func testBusyCorruptOrChangedAuthorityNeverReplacesUnreadableState()async throws {
        for kind in ["busy","corrupt","changed"] {
            let f=LegacyRelayFixture();f.saved=Data("bad".utf8)
            var calls=0
            let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{_,_,_,_ in
                calls+=1
                if kind=="busy"{return CommandExecution(launchError:"selected service lock busy")}
                if kind=="corrupt"{return CommandExecution(stdout:Data("invalid authority".utf8),exitCode:0,childState:.exitConfirmed)}
                return try! f.result(["status":f.transaction(),"admission":["state":"reserved"],"ticket_json":f.ticketJSON(generation:calls)])
            },storage:f.storage)
            await model.inspectPublicBinding()
            if let approval=model.publicBindingApproval{try await XCTUnwrap(model.rebindPublicState(approval)).value}
            XCTAssertTrue(model.publicStateUnreadable);XCTAssertTrue(model.outcomeUnknown)
            XCTAssertNil(model.confirmation());XCTAssertEqual(f.saved,Data("bad".utf8))
        }
    }

    func testPublicBindingChangedAfterInspectionIsNotOverwrittenByOldStatus()async throws {
        let f=LegacyRelayFixture();f.saved=Data("bad".utf8)
        var reads=0
        let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{_,_,_,_ in reads+=1;return try! f.status()},storage:f.storage)
        await model.inspectPublicBinding()
        let old=try XCTUnwrap(model.publicBindingApproval)
        let b=LegacyRelayOutstandingOperation(operation:"confirm",ticketJSON:try f.ticketJSON(generation:2,id:String(repeating:"e",count:32)),originalPreview:Data())
        let savedB=try JSONEncoder().encode(LegacyRelaySavedOperationState(outstanding:b,resolved:nil,lifecycleDebt:true,lastTransaction:nil))
        f.saved=savedB
        try await XCTUnwrap(model.rebindPublicState(old)).value
        XCTAssertEqual(f.saved,savedB);XCTAssertTrue(model.outcomeUnknown)
        XCTAssertNil(model.confirmation());XCTAssertNotNil(model.error)
        await model.inspectPublicBinding()
        XCTAssertEqual(reads,2,"New inspection must preserve the already persisted B without replacing it from A")
        XCTAssertEqual(model.outstanding,b);XCTAssertEqual(f.saved,savedB)
    }

    func testEmptyInspectionOffersPayloadSelectionWithoutResettingUnknown()async throws {
        let f=LegacyRelayFixture();f.saved=Data("bad".utf8)
        let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{_,_,_,_ in try! self.emptyStatus(f)},storage:f.storage)
        await model.inspectPublicBinding()
        XCTAssertNil(model.publicBindingApproval);XCTAssertTrue(model.outcomeUnknown)
        XCTAssertTrue(model.statusNotice?.contains("Choose the retained legacy payload") == true)
        XCTAssertEqual(f.saved,Data("bad".utf8))
        model.select(f.artifact);await model.inspectPublicBinding()
        XCTAssertEqual(model.publicBindingApproval?.file,f.artifact)
        XCTAssertNil(model.confirmation());XCTAssertTrue(model.outcomeUnknown)
    }
}
