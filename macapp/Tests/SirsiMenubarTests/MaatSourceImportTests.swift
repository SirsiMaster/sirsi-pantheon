import XCTest
@testable import SirsiMenubar

final class MaatSourceImportTests: XCTestCase {
    private let head = String(repeating: "a", count: 40)
    private let digest = String(repeating: "d", count: 64)
    private let selected = URL(fileURLWithPath: "/owned/inert-source.json")

    private func sharedFixture() throws -> Data {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        return try Data(contentsOf: root.appendingPathComponent("contracts/maat/source-screen-v2-fixture.json"))
    }

    private func response(change: (inout [String: Any]) -> Void = { _ in }) throws -> CommandExecution {
        var object = try XCTUnwrap(JSONSerialization.jsonObject(with: sharedFixture()) as? [String: Any])
        object["gate"] = "escalate"
        object["floor_evidence"] = "declared-only; checks not executed or authenticated by import"
        object["escalation"] = ["reason": "unqualified", "missed_conf": 0, "review_tier": "frontier"]
        object["input_sha256"] = digest
        change(&object)
        return CommandExecution(stdout: try JSONSerialization.data(withJSONObject: object), exitCode: 0, childState: .exitConfirmed)
    }

    @MainActor private func model() -> MaatSourceImportModel {
        let model = MaatSourceImportModel()
        model.select(selected); model.expectedHead = head
        return model
    }

    func testNativeExampleMatchesSharedGoContractFixture() throws {
        let fixture = try JSONSerialization.jsonObject(with: sharedFixture()) as? NSDictionary
        let example = try JSONSerialization.jsonObject(with: Data(MaatSourceImportContract.example.utf8)) as? NSDictionary
        XCTAssertEqual(example, fixture)
        XCTAssertTrue(MaatSourceImportContract.guidance.contains("unqualified"))
        XCTAssertTrue(MaatSourceImportContract.guidance.contains("evidence_id"))
    }

    @MainActor func testPreviewThenExactHeadAndDigestConfirmationOnce() async throws {
        let model = model(), good = try response()
        var calls = [[String]]()
        await model.review { args in XCTAssertFalse(model.recording); calls.append(args); return good }
        XCTAssertEqual(calls, [["maat", "screen", "--input", selected.path, "--expect-head", head, "--preview", "--json"]])
        XCTAssertNil(model.result)
        let approval = try XCTUnwrap(model.approval)
        XCTAssertEqual(approval.expectedHead, head)
        XCTAssertEqual(approval.inputSHA256, digest)
        let first = model.confirm { args in calls.append(args); return good }
        XCTAssertTrue(model.recording)
        XCTAssertNil(model.confirm { args in calls.append(args); return good })
        await first?.value
        XCTAssertEqual(calls.count, 2)
        XCTAssertEqual(calls.last, approval.arguments)
        XCTAssertNil(model.error)
        XCTAssertEqual(model.result?.verdict.contract, MaatSourceImportContract.id)
        XCTAssertNil(model.confirm { args in calls.append(args); return good })
        XCTAssertEqual(calls.count, 2)
    }

    @MainActor func testMissingHeadAndChangedDraftNeverLaunchRecording() async throws {
        let model = model(), good = try response()
        var calls = 0
        model.expectedHead = ""
        await model.review { _ in calls += 1; return good }
        XCTAssertEqual(calls, 0)
        XCTAssertNil(model.approval)
        XCTAssertNotNil(model.error)
        model.expectedHead = head
        await model.review { _ in good }
        let started = model.confirm { _ in calls += 1; return good }
        // Change while the confirmed task is queued, before its first launch.
        model.expectedHead = String(repeating: "b", count: 40)
        await started?.value
        XCTAssertEqual(calls, 0)
        XCTAssertNotNil(model.error)
        XCTAssertNil(model.result)
        XCTAssertEqual(model.receipt?.childState, .notStarted)
    }

    @MainActor func testExactSourceContractNotProviderStringControlsPreview() async throws {
        let wrongVersion = try response { $0["schema_version"] = 1 }
        let wrongContract = try response { $0["contract"] = "other" }
        let wrongHead = try response { object in
            var subject = object["subject"] as! [String: Any]; subject["head_sha"] = "other"; object["subject"] = subject
        }
        let qualified = try response { object in
            var model = object["model"] as! [String: Any]; model["calibration"] = "qualified"; object["model"] = model
        }
        for bad in [wrongVersion, wrongContract, wrongHead, qualified] {
            let model = model()
            await model.review { _ in bad }
            XCTAssertNil(model.approval)
            XCTAssertNotNil(model.error)
        }
        let model = model()
        let otherProvider = try response { object in
            var stamp = object["model"] as! [String: Any]; stamp["provider"] = "fixture:not-a-qualification-authority"; object["model"] = stamp
        }
        await model.review { _ in otherProvider }
        XCTAssertNotNil(model.approval, "Provider names must not stand in for source contract/qualification")
    }

    @MainActor func testValidationErrorAllowsCorrectedHeadOrDifferentFile() async throws {
        let model = model(), good = try response()
        let failure = CommandExecution(stderr: Data("expected head mismatch; correct --expect-head".utf8), exitCode: 1, childState: .exitConfirmed)
        await model.review { _ in failure }
        XCTAssertTrue(model.error?.contains("expected head mismatch") == true)
        XCTAssertEqual(model.expectedHead, head)
        XCTAssertEqual(model.selectedFile, selected)
        model.select(URL(fileURLWithPath: "/owned/corrected.json"))
        await model.review { _ in good }
        XCTAssertNil(model.error)
        XCTAssertNotNil(model.approval)
        model.cancelReview()
        XCTAssertNil(model.approval)
        XCTAssertNil(model.confirm { _ in XCTFail("cancelled review launched"); return good })
    }

    @MainActor func testDismissalOrSelectionChangeDuringPreviewCannotRestoreApproval() async throws {
        let model = model(), good = try response()
        await model.review { _ in model.cancelReview(); return good }
        XCTAssertNil(model.approval)
        await model.review { _ in model.select(URL(fileURLWithPath: "/owned/new.json")); return good }
        XCTAssertNil(model.approval)
    }

    @MainActor func testIncompleteNonzeroMalformedOutputNeverRecordsOrRetries() async throws {
        let good = try response()
        let wrongContract = try response { $0["contract"] = "mismatched-response" }
        let invalid = [wrongContract, CommandExecution(stdout: good.stdoutData, exitCode: 1, childState: .exitConfirmed),
                       CommandExecution(stdout: good.stdoutData, exitCode: 0, timedOut: true, childState: .exitUnknown),
                       CommandExecution(stdout: good.stdoutData, exitCode: 0, overflow: true, childState: .exitConfirmed),
                       CommandExecution(stdout: good.stdoutData, exitCode: 0, cancelled: true, childState: .exitConfirmed),
                       CommandExecution(stdout: Data("not JSON".utf8), exitCode: 0, childState: .exitConfirmed)]
        for bad in invalid {
            let model = model()
            await model.review { _ in bad }
            XCTAssertNil(model.approval)
            await model.review { _ in good }
            var records = 0
            let task = model.confirm { _ in records += 1; return bad }
            await task?.value
            XCTAssertEqual(records, 1)
            XCTAssertNil(model.result)
            XCTAssertNotNil(model.error)
            XCTAssertNil(model.confirm { _ in records += 1; return good })
            XCTAssertEqual(records, 1)
        }
    }

    @MainActor func testReadbackFailureAndNewReviewPreserveOriginalOperationReceipt() async throws {
        let model = model(), good = try response()
        await model.review { _ in good }
        let recorded = model.confirm { _ in good }
        await recorded?.value
        XCTAssertEqual(model.receipt, good)
        model.cancelReview() // refresh/navigation may revoke consent, not erase the receipt
        await model.review { _ in CommandExecution(launchError: "fixture read failed") }
        XCTAssertEqual(model.receipt, good)
        XCTAssertNotNil(model.result)
        XCTAssertNotNil(model.error)
        XCTAssertNil(model.approval)
    }
}
