import XCTest
@testable import SirsiMenubar

final class MaatKnowledgeArchiveTests: XCTestCase {
    func testGoShapedKnowledgeProjectionRetainsCoverageDebtCursorDigestAndOptionalOrigin() throws {
        let raw = #"""
        {
          "items": [{
            "id": "stable-record", "title": "Retained", "summary": "Bounded summary",
            "references": [{"type":"file","value":"source.txt"}],
            "content": "full admitted content", "classification": "reference",
            "resolution": "open", "revision": 3,
            "content_digest": "digest-v3",
            "origins": [{"record_id":"stable-record","source":"claude","instance_id":"profile-a","native_item_id":"item-a","observed_at":"2026-10-24T12:00:00Z"}],
            "owner_annotations": []
          }],
          "total": 1, "withheld": 2,
          "coverage": {"sources":[{"id":7,"source":"claude","state":"partial","error_code":"source_unavailable"}],"pending_projection_debt":4},
          "next_cursor": "cursor-next", "state": "partial"
        }
        """#
        let page = try JSONDecoder().decode(MaatKnowledgeProjection.self, from: Data(raw.utf8))
        XCTAssertEqual(page.coverage?.pendingProjectionDebt, 4)
        XCTAssertEqual(page.coverage?.sources.first?.state, "partial")
        XCTAssertEqual(page.nextCursor, "cursor-next")
        XCTAssertEqual(page.state, "partial")
        XCTAssertEqual(page.items.first?.revision, 3)
        XCTAssertEqual(page.items.first?.contentDigest, "digest-v3")
        XCTAssertEqual(page.items.first?.origins.first?.reference, "")
    }

    func testKnowledgeProjectionRejectsMalformedOrFailedReadsWithoutInventingEmptyLibrary() throws {
        func read(_ json: String, exitStatus: Int32? = 0, timedOut: Bool = false, launchError: String? = nil) -> SirsiEngine.JSONCommandRead {
            SirsiEngine.JSONCommandRead(data: Data(json.utf8), exitStatus: exitStatus, timedOut: timedOut, launchError: launchError)
        }

        let validEmpty = try XCTUnwrap(MaatKnowledgeView.decodeProjection(read(#"{"items":[],"total":0,"withheld":0}"#)))
        XCTAssertTrue(validEmpty.items.isEmpty)
        XCTAssertEqual(validEmpty.total, 0)
        XCTAssertEqual(validEmpty.withheld, 0)
        XCTAssertNil(validEmpty.coverage)
        XCTAssertNil(validEmpty.nextCursor)
        XCTAssertNil(validEmpty.state)

        let errorEnvelope = #"{"command":"sirsi maat knowledge","summary":"read failed","status":"error","errors":["archive_unavailable"]}"#
        let malformed = [
            "{}",
            errorEnvelope,
            #"{"total":0,"withheld":0}"#,
            #"{"items":[],"withheld":0}"#,
            #"{"items":[],"total":0}"#,
            #"{"items":null,"total":0,"withheld":0}"#,
            #"{"items":{},"total":0,"withheld":0}"#,
            #"{"items":[],"total":null,"withheld":0}"#,
            #"{"items":[],"total":"0","withheld":0}"#,
            #"{"items":[],"total":0,"withheld":null}"#,
            #"{"items":[],"total":0,"withheld":"0"}"#,
            #"{"items":[],"total":-1,"withheld":0}"#,
            #"{"items":[],"total":0,"withheld":-1}"#,
        ]
        for json in malformed {
            XCTAssertNil(MaatKnowledgeView.decodeProjection(read(json)), "must reject malformed projection: \(json)")
        }

        XCTAssertNil(MaatKnowledgeView.decodeProjection(read(#"{"items":[],"total":0,"withheld":0}"#, exitStatus: 1)))
        XCTAssertNil(MaatKnowledgeView.decodeProjection(read(#"{"items":[],"total":0,"withheld":0}"#, timedOut: true)))
        XCTAssertNil(MaatKnowledgeView.decodeProjection(read(#"{"items":[],"total":0,"withheld":0}"#, launchError: "launch failed")))
    }

    func testOwnerReviewCapturesExactVersionAndOperandsOnce() throws {
        var pickerState = "accepted"
        var editableNote = "  Reviewed the displayed source.  "
        let approval = try XCTUnwrap(MaatKnowledgeResolutionRequest.capture(
            recordID: "record-7", revision: 4, digest: "digest-4", state: pickerState, note: editableNote
        ))
        pickerState = "rejected"
        editableNote = "edited after confirmation"
        XCTAssertEqual(approval.arguments, [
            "maat", "knowledge", "resolve", "record-7", "--state", "accepted",
            "--revision", "4", "--digest", "digest-4", "--note-stdin"
        ])
        XCTAssertEqual(approval.note, "Reviewed the displayed source.")

        var submission = MaatKnowledgeResolutionSubmission()
        XCTAssertTrue(submission.begin())
        XCTAssertFalse(submission.begin(), "a second confirmation must not launch another write")
        submission.finish()
        XCTAssertFalse(submission.begin(), "an uncertain outcome must not be replayed from the same reviewed view")
        var freshReview = MaatKnowledgeResolutionSubmission()
        XCTAssertTrue(freshReview.begin(), "a deliberately reopened review has a new once-only submission gate")
    }

    func testOwnerReviewRequiresFreshVersionBindingAndNonemptyConsent() {
        XCTAssertNil(MaatKnowledgeResolutionRequest.capture(recordID: "record", revision: nil, digest: "digest", state: "accepted", note: "why"))
        XCTAssertNil(MaatKnowledgeResolutionRequest.capture(recordID: "record", revision: 2, digest: nil, state: "accepted", note: "why"))
        XCTAssertNil(MaatKnowledgeResolutionRequest.capture(recordID: "record", revision: 2, digest: "digest", state: "accepted", note: "  \n"))
    }

    func testNativeProjectionReconciliationDistinguishesCompleteAndPartialReceipts() throws {
        let complete = #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Reconciled","status":"ok","evidence":[{"label":"Projected","value":"3"},{"label":"Remaining","value":"0"}]}"#
        let zero = #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Already current","status":"ok","evidence":[{"label":"Projected","value":"0"},{"label":"Remaining","value":"0"}]}"#
        let partial = #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Incomplete","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"}]}"#
        let malformed = #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Incomplete","status":"ok","evidence":[{"label":"Projected","value":"1"}]}"#
        func receipt(_ json: String) throws -> MaatKnowledgeProjectionReceipt? {
            let result = try JSONDecoder().decode(CommandResult.self, from: Data(json.utf8))
            return MaatKnowledgeProjectionReceipt(commandResult: result)
        }
        let completeReceipt = try XCTUnwrap(receipt(complete))
        XCTAssertEqual(completeReceipt, MaatKnowledgeProjectionReceipt(projected: 3, remaining: 0, isComplete: true))
        XCTAssertTrue(completeReceipt.isComplete)
        XCTAssertEqual(try receipt(zero), MaatKnowledgeProjectionReceipt(projected: 0, remaining: 0, isComplete: true))

        let partialReceipt = try XCTUnwrap(receipt(partial))
        XCTAssertEqual(partialReceipt, MaatKnowledgeProjectionReceipt(projected: 1, remaining: 2, isComplete: false))
        XCTAssertFalse(partialReceipt.isComplete, "remaining projection debt must not be rendered as completion")
        XCTAssertEqual(partialReceipt.summary, "Projected 1; 2 remain. Partial progress; reconciliation is not complete.")

        let contradictoryOrMalformed = [
            malformed,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Wrong error","status":"partial","errors":["projection_failed"],"evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Missing error","status":"partial","evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"No debt","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"0"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Unreported debt","status":"ok","evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Contradictory error","status":"ok","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"0"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Negative","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"-1"},{"label":"Remaining","value":"2"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Noncanonical count","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"01"},{"label":"Remaining","value":"2"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Duplicate count","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"1"},{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"}]}"#,
            #"{"command":"sirsi maat knowledge reconcile-projections","summary":"Unexpected metric","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"},{"label":"Other","value":"0"}]}"#,
            #"{"command":"sirsi maat knowledge other-command","summary":"Wrong command","status":"partial","errors":["projection_debt_remains"],"evidence":[{"label":"Projected","value":"1"},{"label":"Remaining","value":"2"}]}"#,
        ]
        for json in contradictoryOrMalformed {
            XCTAssertNil(try receipt(json), "must reject contradictory or malformed receipt: \(json)")
        }
    }

    func testKnowledgeRefreshFailureDoesNotClaimUnavailableArchiveCacheRemainsAvailable() {
        let unavailable = MaatKnowledgeView.refreshFailureMessage(retainedViewAvailable: false, detail: "archive_unavailable")
        XCTAssertTrue(unavailable.contains("archive could not be read"))
        XCTAssertTrue(unavailable.contains("contents are unverified"))
        XCTAssertFalse(unavailable.contains("cache remains available"))

        let retained = MaatKnowledgeView.refreshFailureMessage(retainedViewAvailable: true, detail: "source_unavailable")
        XCTAssertTrue(retained.contains("displayed retained view remains available and unchanged"))
        XCTAssertFalse(retained.contains("contents are unverified"))
    }
}
