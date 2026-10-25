import XCTest
@testable import SirsiMenubar

@MainActor final class LegacyRelayPrivilegedBridgeTests: XCTestCase {
    private let engineDigest = String(repeating:"a",count:64)
    private var status: [String] { ["launchd","legacy-maintenance","status"] }
    private func exited(_ text: String) -> CommandExecution {
        CommandExecution(stdout:Data(text.utf8),exitCode:0,childState:.exitConfirmed)
    }
    func testOrdinaryAppTypedRequestIsDataNotShellSource() async throws {
        let path = "/owned/a '\"; $(never)\nnot-a-command"
        var invocations = 0
        let bridge = LegacyRelayPrivilegedBridge(qualify:{self.engineDigest},identity:{nil},compatibility:{}) { args, authorize in
            XCTAssertTrue(authorize()); invocations += 1
            XCTAssertEqual(args[0],"-e"); XCTAssertEqual(args[1],LegacyRelayPrivilegedBridge.script)
            XCTAssertFalse(args[1].contains(path))
            let data = try! XCTUnwrap(Data(base64Encoded:args[2]))
            let json = try! XCTUnwrap(JSONSerialization.jsonObject(with:data) as? [String:String])
            XCTAssertEqual(json["legacy"],path); XCTAssertEqual(json["engine_sha256"],self.engineDigest)
            XCTAssertEqual(Set(json.keys),Set(["profile","operation","engine_sha256","legacy","transaction","plan_sha256","artifact","restore_json","ticket_json","target_operation"]))
            return self.exited(LegacyRelayPrivilegedBridge.declined)
        }
        let result = await bridge.capture(args:["launchd","legacy-maintenance","preview","--legacy-payload",path],stdin:nil,authorize:{true})
        XCTAssertEqual(invocations,1); XCTAssertTrue(result.launchError?.contains("declined") == true)
        XCTAssertNil(result.captureError); XCTAssertEqual(result.childState,.notStarted)
    }
    func testMissingOrDriftingEngineNeverSubmitsRootRequest() async {
        for drift in [false,true] {
            var probes = 0, calls = 0
            let bridge = LegacyRelayPrivilegedBridge(qualify:{
                probes += 1
                if !drift { throw NSError(domain:"missing",code:1) }
                return probes == 1 ? self.engineDigest : String(repeating:"b",count:64)
            },identity:{nil},compatibility:{}) { _, authorize in
                if authorize() {calls += 1}
                return CommandExecution(launchError:"Approval changed before launch.")
            }
            let result = await bridge.capture(args:status,stdin:nil,authorize:{true})
            XCTAssertEqual(calls,0); XCTAssertNotNil(result.launchError)
        }
    }
    func testUnknownCancelAndMalformedRepliesNeverBecomeSuccess() async {
        for kind in ["lost","cancel","malformed","wrong-operation"] {
            let bridge = LegacyRelayPrivilegedBridge(qualify:{self.engineDigest},identity:{nil},compatibility:{}) { _, authorize in
                XCTAssertTrue(authorize())
                switch kind {
                case "lost":return CommandExecution(timedOut:true,childState:.exitUnknown)
                case "cancel":return CommandExecution(cancelled:true,childState:.exitUnknown)
                case "malformed":return self.exited("{}")
                default:return self.exited("{\"schema\":1,\"operation\":\"confirm\",\"ok\":true,\"body_base64\":\"e30=\",\"error\":\"\"}")
                }
            }
            let result = await bridge.capture(args:status,stdin:nil,authorize:{true})
            XCTAssertFalse(result.complete); XCTAssertTrue(result.captureError?.contains("unknown") == true)
        }
    }
    func testFramedOperationFailurePreservesExactControllerBytes() async throws {
        let bytes = Data("{\"retained\":true}\n".utf8)
        let json = try JSONSerialization.data(withJSONObject:["schema":1,"operation":"status","ok":false,"body_base64":bytes.base64EncodedString(),"error":"Retain the original transaction"])
        let bridge = LegacyRelayPrivilegedBridge(qualify:{self.engineDigest},identity:{nil},compatibility:{}) { _, _ in
            CommandExecution(stdout:json,exitCode:0,childState:.exitConfirmed)
        }
        let result = await bridge.capture(args:status,stdin:nil,authorize:{true})
        XCTAssertTrue(result.complete);XCTAssertEqual(result.exitCode,1);XCTAssertEqual(result.stdoutData,bytes)
        XCTAssertEqual(result.stderr,"Retain the original transaction")
    }
    func testUnrecognizedOperationsAndPrelaunchCancelNeverInvokeOS() async {
        var calls = 0
        let bridge = LegacyRelayPrivilegedBridge(qualify:{self.engineDigest},identity:{nil},compatibility:{}) { _,_ in calls += 1;return self.exited("{}") }
        _ = await bridge.capture(args:["launchd","restart","anything"],stdin:nil,authorize:{true})
        _ = await bridge.capture(args:status,stdin:nil,authorize:{false})
        XCTAssertEqual(calls,0)
    }
    func testFrameRetainsUnknownReceiptAndConsumesConsentAcrossRemount() async throws {
        let engine = SirsiEngine()
        XCTAssertTrue(engine.legacyRelayMaintenance === engine.legacyRelayMaintenance)
        let fixture = LegacyRelayFixture()
        let raw = try fixture.preview()
        var calls = 0
        let model = LegacyRelayMaintenanceModel(directory:fixture.scope, capture: { args,_,_,_ in
            calls += 1
            if args[2] == "preview" {return raw}
            return CommandExecution(cancelled:true,captureError:"Outcome unknown; read status",childState:.exitUnknown)
        }, storage: fixture.storage)
        model.select(fixture.artifact);await model.review()
        let approval = try XCTUnwrap(model.confirmation())
        let action = try XCTUnwrap(model.confirm(approval));await action.value
        XCTAssertTrue(model.outcomeUnknown);XCTAssertNotNil(model.receipt)
        XCTAssertNil(model.confirm(approval));await model.review()
        XCTAssertEqual(calls,2);XCTAssertEqual(model.preview?.sha256,fixture.digest)
        XCTAssertFalse(model.busy)
    }
}
