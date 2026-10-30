import XCTest
@testable import SirsiMenubar

@MainActor final class MaatLifecycleTests: XCTestCase {
    private let head = String(repeating: "a", count: 40)
    private func response(recorded: Bool = false, duplicate: Bool = false) throws -> CommandExecution {
        let value: [String: Any] = ["schema":"maat.source-lifecycle-receipt.v1", "receipt_id":"review", "receipt_sha256":String(repeating:"b",count:64),
            "repo":"fixture/repo", "head_sha":head, "profile_id":String(repeating:"c",count:64), "acceptance":"canonical custody only; independent acceptance unknown",
            "recorded":recorded, "duplicate":duplicate,
            "state":["status":"unqualified", "reason":"Pinned policy/evaluation acceptance missing", "epoch":1, "declared_receipts":1, "verified_receipts":0, "missing_outcomes":0, "coverage_known":false]]
        return CommandExecution(stdout:try JSONSerialization.data(withJSONObject:value),exitCode:0,childState:.exitConfirmed)
    }
    private func configured() -> MaatLifecycleModel {
        let model=MaatLifecycleModel();model.receiptID="review";model.repo="fixture/repo";model.head=head;return model
    }
    private func adoptionResponse(recorded: Bool = false) -> CommandExecution {
        let profile:[String:Any] = ["producer":"fixture-producer","artifact_sha256":String(repeating:"a",count:64),
            "model_version":"fixture-v1","prompt_sha256":String(repeating:"b",count:64),
            "recipe_sha256":String(repeating:"c",count:64),"policy_sha256":String(repeating:"d",count:64)]
        let adoption:[String:Any] = ["review_id":"review","review_sha256":String(repeating:"e",count:64),
            "origin_receipt_id":"evaluation","origin_receipt_sha256":String(repeating:"f",count:64),
            "repo":"fixture/repo","head_sha":head,"profile":profile,"screen_sha256":String(repeating:"1",count:64),
            "candidate_gate":"pass","final_gate":"pass","producer":"fixture-producer",
            "reviewer_claim":"claimed-independent-reviewer","origin_issuer_claim":"fixture-producer"]
        let value:[String:Any] = ["schema":"maat.source-lifecycle-owner-adoption.v1","adoption":adoption,
            "adoption_sha256":String(repeating:"2",count:64),"recorded":recorded,"duplicate":false,
            "provenance":"Owner adoption of exact review bytes; canonical From is displayed custody attribution, not authenticated reviewer identity."]
        return CommandExecution(stdout:try! JSONSerialization.data(withJSONObject:value),exitCode:0,childState:.exitConfirmed)
    }

    func testActualPreviewAndExactConfirmationArgumentsOnce() async throws {
        let model=configured();let preview=try response();let done=try response(recorded:true);var calls:[[String]]=[]
        await model.preview { args in calls.append(args);return preview }
        XCTAssertEqual(model.projection?.state.status,"unqualified");XCTAssertNotNil(model.approval)
        XCTAssertEqual(calls,[ ["maat","lifecycle","--receipt-id","review","--expect-repo","fixture/repo","--expect-head",head,"--json","--preview"] ])
        await model.confirm { args in
            calls.append(args)
            XCTAssertEqual(Array(args.suffix(3)),["--expect-receipt-sha256",String(repeating:"b",count:64),"--confirm"])
            await model.confirm { _ in XCTFail("reentrant record");return done };return done
        }
        await model.confirm { _ in XCTFail("replayed record");return done }
        XCTAssertEqual(calls.count,2);XCTAssertNil(model.approval);XCTAssertEqual(model.receipt,done)
    }

    func testChangedScopeDismissAndRemountCannotRestoreApprovalOrReplay() async throws {
        let model=configured();let read=try response()
        await model.preview { _ in read };model.head=String(repeating:"d",count:40)
        await model.confirm { _ in XCTFail("changed head recorded");return read }
        XCTAssertNil(model.approval)
        model.head=head;await model.preview { _ in read };model.invalidate()
        await model.confirm { _ in XCTFail("dismissed review recorded");return read }
        let remounted=configured()
        await remounted.confirm { _ in XCTFail("remount replayed record");return read }
        XCTAssertNil(remounted.approval);XCTAssertNil(remounted.receipt)
    }

    func testMalformedIncompleteNonzeroNeverAutoRetriesAndReceiptSurvivesPreviewError() async throws {
        let good=try response()
        let failed=[CommandExecution(stdout:Data("not JSON".utf8),exitCode:0,childState:.exitConfirmed),
            CommandExecution(stdout:good.stdoutData,exitCode:1,childState:.exitConfirmed),
            CommandExecution(stdout:good.stdoutData,exitCode:0,timedOut:true,childState:.exitUnknown),
            CommandExecution(stdout:good.stdoutData,exitCode:0,overflow:true,childState:.exitConfirmed),
            CommandExecution(stdout:good.stdoutData,exitCode:0,cancelled:true,childState:.exitConfirmed)]
        for result in failed {
            let model=configured();var calls=0
            await model.preview { _ in calls+=1;return result }
            XCTAssertEqual(calls,1);XCTAssertNil(model.approval);XCTAssertNotNil(model.issue)
        }
        let model=configured();let done=try response(recorded:true)
        await model.preview { _ in good };await model.confirm { _ in done }
        await model.preview { _ in CommandExecution(launchError:"owned read refused") }
        XCTAssertEqual(model.receipt,done);XCTAssertNil(model.approval);XCTAssertNotNil(model.issue)
    }

    func testDelayedPreviewCannotOverwriteChangedSelection() async throws {
        let model=configured();let result=try response()
        var pending:AsyncStream<CommandExecution>.Continuation!
        let stream=AsyncStream<CommandExecution>{pending=$0};defer{pending.finish()}
        let entered=expectation(description:"preview held")
        let task=Task { @MainActor in await model.preview { _ in entered.fulfill();for await value in stream{return value};return CommandExecution(cancelled:true) } }
        await fulfillment(of:[entered],timeout:2)
        model.receiptID="different";pending.yield(result);await task.value
        XCTAssertNil(model.approval);XCTAssertNil(model.projection)
    }

    func testInvalidatedMaatApprovalIsRejectedAtOwnedProcessLaunch() async throws {
        let model=configured(), good=try response()
        await model.preview { _ in good }
        let approvedID=model.receiptID, approvedRepo=model.repo, approvedHead=model.head
        XCTAssertNotNil(model.approval)

        let directory=FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false)
        defer { try? FileManager.default.removeItem(at: directory) }
        let script=directory.appendingPathComponent("must-not-run.sh")
        try Data("printf 'child-launched'\n".utf8).write(to: script)

        let captureEntered=expectation(description:"confirmation reached the held native capture boundary")
        var releaseCapture: CheckedContinuation<Void,Never>?
        let confirmation=model.beginConfirm { args in
            XCTAssertEqual(Array(args.suffix(3)),["--expect-receipt-sha256",String(repeating:"b",count:64),"--confirm"])
            captureEntered.fulfill()
            await withCheckedContinuation { continuation in releaseCapture=continuation }
            // This uses the actual owned Process.run path. Its launch check is
            // bound to the Maat selection frozen by preview, not caller input.
            return await SirsiEngine.captureProcess(executable:"/bin/sh",args:[script.path],authorize:{
                model.receiptID==approvedID && model.repo==approvedRepo && model.head==approvedHead
            })
        }
        await fulfillment(of:[captureEntered],timeout:2)
        model.head=String(repeating:"d",count:40)
        releaseCapture?.resume()
        await confirmation?.value

        XCTAssertEqual(model.receipt?.childState,.notStarted)
        XCTAssertTrue(model.receipt?.stdoutData.isEmpty ?? false)
        XCTAssertNil(model.projection)

        // The injected test capture above is held to force the same gap the
        // production Process.run authorization must close; pin the production
        // wiring to the private generation/receipt approval predicate.
        let source=try String(contentsOf:URL(fileURLWithPath:#filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Sources/SirsiMenubar/MaatLifecycle.swift"),encoding:.utf8)
        XCTAssertTrue(source.contains("SirsiEngine.runCaptured(args: args, authorize: { self.authorizesLaunch(frozen) })"))
    }

    func testOwnerAdoptionPreviewAndExactConfirmationArgumentsOnce() async throws {
        let model=configured();model.originReceiptID="evaluation"
        let preview=adoptionResponse(),done=adoptionResponse(recorded:true);var calls=[[String]]()
        await model.previewAdoption { args in calls.append(args);return preview }
        XCTAssertEqual(calls,[ ["maat","lifecycle","--receipt-id","review","--origin-receipt-id","evaluation",
            "--expect-repo","fixture/repo","--expect-head",head,"--owner-adopt-review","--json","--preview"] ])
        XCTAssertNotNil(model.adoptionApproval)
        XCTAssertEqual(model.adoptionProjection?.adoption.reviewerClaim,"claimed-independent-reviewer")
        XCTAssertEqual(model.adoptionProjection?.adoption.repo,"fixture/repo")
        XCTAssertEqual(model.adoptionProjection?.adoption.headSHA,head)
        XCTAssertEqual(model.adoptionProjection?.adoption.profile.producer,"fixture-producer")
        XCTAssertEqual(model.adoptionProjection?.adoption.reviewSHA,String(repeating:"e",count:64))
        XCTAssertEqual(model.adoptionProjection?.adoption.originReceiptSHA,String(repeating:"f",count:64))
        XCTAssertTrue(model.adoptionProjection?.provenance.contains("not authenticated reviewer identity") ?? false)
        await model.confirmAdoption { args in
            calls.append(args)
            XCTAssertEqual(Array(args.suffix(7)),["--expect-review-sha256",String(repeating:"e",count:64),
                "--expect-origin-sha256",String(repeating:"f",count:64),"--expect-adoption-sha256",String(repeating:"2",count:64),"--confirm"])
            await model.confirmAdoption { _ in XCTFail("reentrant owner adoption");return done }
            return done
        }
        await model.confirmAdoption { _ in XCTFail("owner adoption replayed");return done }
        XCTAssertEqual(calls.count,2);XCTAssertNil(model.adoptionApproval)
        XCTAssertEqual(model.adoptionReceipt,done)
        XCTAssertEqual(model.adoptionProjection?.adoptionSHA,String(repeating:"2",count:64))
    }

    func testOriginChangeConsumesOwnerAdoptionApprovalBeforeLaunch() async throws {
        let model=configured();model.originReceiptID="evaluation"
        await model.previewAdoption { _ in adoptionResponse() }
        XCTAssertNotNil(model.adoptionApproval)
        var calls=0
        let task=model.beginAdoptionConfirm { _ in calls+=1;return self.adoptionResponse(recorded:true) }
        model.originReceiptID="different-evaluation"
        await task?.value
        XCTAssertEqual(calls,0);XCTAssertNil(model.adoptionApproval);XCTAssertNil(model.adoptionProjection)
    }

    func testOwnerAdoptionReviewScopeAndDismissalInvalidatePendingApproval() async throws {
        let changed: [(String, (MaatLifecycleModel) -> Void)] = [
            ("review", { $0.receiptID = "different-review" }),
            ("repository", { $0.repo = "different/repo" }),
            ("head", { $0.head = String(repeating: "d", count: 40) }),
            ("dismissal", { $0.invalidate() })
        ]
        for (name, invalidate) in changed {
            let model=configured();model.originReceiptID="evaluation"
            await model.previewAdoption { _ in adoptionResponse() }
            XCTAssertNotNil(model.adoptionApproval, name)
            var calls=0
            let task=model.beginAdoptionConfirm { _ in calls+=1;return self.adoptionResponse(recorded:true) }
            invalidate(model)
            await task?.value
            XCTAssertEqual(calls,0,name)
            XCTAssertNil(model.adoptionApproval,name)
            XCTAssertNil(model.adoptionProjection,name)
        }
    }

    func testOwnerAdoptionRetainsCancelledAndFailedProcessReceiptsWithoutRetry() async throws {
        let results = [
            CommandExecution(launchError:"cancelled at owned boundary",cancelled:true,childState:.notStarted),
            CommandExecution(stdout:Data("partial".utf8),exitCode:2,childState:.exitConfirmed)
        ]
        for failure in results {
            let model=configured();model.originReceiptID="evaluation"
            await model.previewAdoption { _ in adoptionResponse() }
            var calls=0
            await model.confirmAdoption { _ in calls+=1;return failure }
            XCTAssertEqual(calls,1)
            XCTAssertEqual(model.adoptionReceipt,failure)
            XCTAssertNil(model.adoptionApproval)
            XCTAssertNotNil(model.issue)
            await model.confirmAdoption { _ in XCTFail("failed owner adoption retried automatically");return self.adoptionResponse(recorded:true) }
            XCTAssertEqual(calls,1)
            XCTAssertEqual(model.adoptionReceipt,failure)
        }
    }

    func testOwnerAdoptionUsesActualProcessLaunchFenceAfterScopeInvalidation() async throws {
        let model=configured();model.originReceiptID="evaluation"
        await model.previewAdoption { _ in adoptionResponse() }
        let approvedID=model.receiptID,approvedOrigin=model.originReceiptID,approvedRepo=model.repo,approvedHead=model.head
        let directory=FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:false)
        defer { try? FileManager.default.removeItem(at:directory) }
        let script=directory.appendingPathComponent("must-not-adopt.sh")
        try Data("printf 'child-launched'\n".utf8).write(to:script)
        let captureEntered=expectation(description:"owner adoption reached held native capture")
        var releaseCapture:CheckedContinuation<Void,Never>?
        let confirmation=model.beginAdoptionConfirm { _ in
            captureEntered.fulfill()
            await withCheckedContinuation { (continuation:CheckedContinuation<Void,Never>) in releaseCapture=continuation }
            return await SirsiEngine.captureProcess(executable:"/bin/sh",args:[script.path],authorize:{
                model.receiptID==approvedID && model.originReceiptID==approvedOrigin &&
                    model.repo==approvedRepo && model.head==approvedHead
            })
        }
        await fulfillment(of:[captureEntered],timeout:2)
        model.originReceiptID="changed-origin"
        releaseCapture?.resume()
        await confirmation?.value
        XCTAssertEqual(model.adoptionReceipt?.childState,.notStarted)
        XCTAssertTrue(model.adoptionReceipt?.stdoutData.isEmpty ?? false)
        XCTAssertNil(model.adoptionProjection)

        let source=try String(contentsOf:URL(fileURLWithPath:#filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("Sources/SirsiMenubar/MaatLifecycle.swift"),encoding:.utf8)
        XCTAssertTrue(source.contains("SirsiEngine.runCaptured(args: args, authorize: { self.authorizesAdoptionLaunch(frozen) })"))
    }

    func testProjectionLabelsDistinguishLegacyComparisonsFromQualification() throws {
        let macapp=URL(fileURLWithPath:#filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let source=try String(contentsOf:macapp.appendingPathComponent("Sources/SirsiMenubar/MaatCasebookView.swift"),encoding:.utf8)
        XCTAssertTrue(source.contains("Legacy comparisons"));XCTAssertFalse(source.contains("systemMetric(\"Calibrated\""))
        XCTAssertTrue(source.contains("MaatLifecycleControl(engine: engine)"))
        let lifecycle=try String(contentsOf:macapp.appendingPathComponent("Sources/SirsiMenubar/MaatLifecycle.swift"),encoding:.utf8)
        XCTAssertTrue(lifecycle.contains("TextField(\"Originating evaluation receipt ID for owner adoption\""))
        XCTAssertTrue(lifecycle.contains("Button(\"Preview exact review owner adoption\""))
        XCTAssertTrue(lifecycle.contains("Button(\"Adopt this exact review locally\""))
        XCTAssertTrue(lifecycle.contains("Review SHA256 \\(adoption.reviewSHA)"))
        XCTAssertTrue(lifecycle.contains("Origin SHA256 \\(adoption.originSHA)"))
        XCTAssertTrue(lifecycle.contains("\\(adoption.adoption.repo) @ \\(adoption.adoption.headSHA)"))
    }
}
