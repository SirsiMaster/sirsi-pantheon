import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayInstallerPreflightTests:XCTestCase {
    @MainActor private final class HeldIdentity {
        let entered:XCTestExpectation
        private var continuation:CheckedContinuation<String,Never>?
        private var released=false
        init(_ entered:XCTestExpectation){self.entered=entered}
        func read()async->String{
            entered.fulfill()
            return await withCheckedContinuation{c in
                if released{c.resume(returning:"/owned/inert/sirsi")}else{continuation=c}
            }
        }
        func release(){released=true;let pending=continuation;continuation=nil;pending?.resume(returning:"/owned/inert/sirsi")}
        var pending:Bool{continuation != nil}
    }
    private func selection(_ f:LegacyRelayFixture)throws->CommandExecution{
        try f.result(["schema":1,"release":"0.24.200","package":"/private/var/tmp/sirsi-maintenance-installer-fixture/SirsiPantheon-0.24.200-arm64.pkg",
                      "sha256":f.digest,"device":1,"inode":2,"bytes":20,"implementation":String(repeating:"d",count:64)])
    }

    func testStalledInstallerIdentityReturnsBeforeReleaseWithoutSubmission()async{
        let entered=expectation(description:"identity entered"),returned=expectation(description:"bounded return")
        let held=HeldIdentity(entered)
        var submissions=0
        let reader=LegacyRelayInstallerCapture(identityTimeout:0.05,overallTimeout:1,qualify:{await held.read()},invoke:{_,_,_,_ in
            submissions+=1;return CommandExecution(exitCode:0,childState:.exitConfirmed)
        })
        let task=Task{@MainActor in
            let result=await reader.capture(args:["launchd","maintenance-installer"],authorize:{true})
            returned.fulfill();return result
        }
        await fulfillment(of:[entered,returned],timeout:2)
        XCTAssertTrue(held.pending,"Deadline return must not depend on releasing the identity worker")
        held.release()
        let result=await task.value
        XCTAssertEqual(result.childState,.notStarted);XCTAssertNotNil(result.launchError)
        for _ in 0..<8{await Task.yield()}
        XCTAssertEqual(submissions,0)
    }

    func testFinalIdentityCancelExpiryOrGenerationChangeNeverSubmitsOrOpensLate()async throws{
        for kind in ["cancel","expiry","generation"] {
            let f=LegacyRelayFixture(),raw=try selection(f)
            let entered=expectation(description:"final identity \(kind)"),returned=expectation(description:"final return \(kind)")
            let held=HeldIdentity(entered)
            var probes=0,submissions=0,opens=0
            let reader=LegacyRelayInstallerCapture(identityTimeout:kind=="expiry" ? 0.05:1,overallTimeout:2,qualify:{
                probes+=1
                if probes==2{return await held.read()}
                return "/owned/inert/sirsi"
            },invoke:{_,_,_,authorize in
                XCTAssertTrue(authorize());submissions+=1;return raw
            })
            let model=LegacyRelayMaintenanceModel(directory:f.scope,capture:{_,_,_,_ in
                XCTFail("Pending local status must not submit another privileged command")
                return CommandExecution(launchError:"Inert unexpected status")
            },installerCapture:{args,_,_,authorize in
                await reader.capture(args:args,authorize:authorize)
            },openInstaller:{_ in opens+=1;return true},storage:f.storage)
            await model.prepareInstaller()
            let approval=try XCTUnwrap(model.installer)
            let action=try XCTUnwrap(model.install(approval))
            let joined=Task{@MainActor in await action.value;returned.fulfill()}
            await fulfillment(of:[entered],timeout:2)
            if kind != "expiry" {
                await model.readStatus() // Responsive pending status, no root callback.
                XCTAssertTrue(model.busy);XCTAssertNotNil(model.statusNotice)
            }
            if kind=="cancel"{model.cancelInvocation()}
            if kind=="generation"{model.changeDirectory(URL(fileURLWithPath:"/owned/other"));held.release()}
            await fulfillment(of:[returned],timeout:3)
            if kind != "generation"{XCTAssertTrue(held.pending,"Cancel/deadline must return before late completion")}
            held.release();await joined.value
            for _ in 0..<8{await Task.yield()}
            XCTAssertEqual(submissions,1,"Only initial preparation may submit; final identity must not submit")
            XCTAssertEqual(opens,0);XCTAssertFalse(model.busy)
            XCTAssertNil(model.install(approval),"Consumed Installer consent cannot revive")
        }
    }

    func testConsumedApprovalAndIdentityRefusalNeverSubmitInstallerCommand()async{
        for kind in ["consumed","refused"] {
            var approved=true,submissions=0
            let reader=LegacyRelayInstallerCapture(qualify:{
                if kind=="refused"{throw NSError(domain:"InertSignatureRefusal",code:1)}
                approved=false;return "/owned/inert/sirsi"
            },invoke:{_,_,_,_ in submissions+=1;return CommandExecution(exitCode:0,childState:.exitConfirmed)})
            let result=await reader.capture(args:["launchd","maintenance-installer","--verify-selection","00"],authorize:{approved})
            XCTAssertEqual(submissions,0);XCTAssertEqual(result.childState,.notStarted)
            XCTAssertNotNil(result.launchError)
        }
    }

    func testRollbackBridgeAcceptsOnlyTypedExactVersionCommand()async{
        var submitted=[[String]]()
        let reader=LegacyRelayInstallerCapture(qualify:{"/owned/inert/sirsi"},invoke:{_,args,_,authorize in
            XCTAssertTrue(authorize());submitted.append(args)
            return CommandExecution(stdout:Data("retained attempt".utf8),exitCode:0,childState:.exitConfirmed)
        })
        let exact=await reader.capture(args:["update","--rollback-version","0.24.137","--confirm-rollback"],authorize:{true})
        XCTAssertTrue(exact.complete)
        let malformed=await reader.capture(args:["update","--rollback-version","0.24.137","--confirm-rollback","--app"],authorize:{true})
        XCTAssertEqual(malformed.childState,.notStarted)
        let belowReaderFloor=await reader.capture(args:["update","--rollback-version","0.24.136","--confirm-rollback"],authorize:{true})
        XCTAssertEqual(belowReaderFloor.childState,.notStarted)
        XCTAssertEqual(submitted,[["update","--rollback-version","0.24.137","--confirm-rollback"]])
    }

    func testModelRechecksDeadlineImmediatelyBeforeOpeningInstaller()async throws{
        let f=LegacyRelayFixture(),raw=try selection(f)
        var calls=0,opens=0
        let model=LegacyRelayMaintenanceModel(directory:f.scope,installerCapture:{_,_,_,_ in
            calls+=1
            if calls==2{try? await Task.sleep(nanoseconds:150_000_000)}
            return raw
        },openInstaller:{_ in opens+=1;return true},storage:f.storage,installerTimeout:0.1)
        await model.prepareInstaller()
        let approved=try XCTUnwrap(model.installer)
        try await XCTUnwrap(model.install(approved)).value
        XCTAssertEqual(calls,2);XCTAssertEqual(opens,0)
        XCTAssertNil(model.install(approved));XCTAssertNotNil(model.error)
    }
}
