import XCTest
@testable import SirsiMenubar

final class MaatOperationStateTests: XCTestCase {
    func testPreflightProjectionRequiresExplicitJournalIntegrityCount() {
        let complete = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"invalid_count":0,"issues":[]}}"#.utf8)
        let missingIntegrity = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0}}"#.utf8)
        let missingCount = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"issues":[]}}"#.utf8)

        XCTAssertNotNil(MaatOperationPreflight.decodeProjection(from: complete))
        XCTAssertNil(MaatOperationPreflight.decodeProjection(from: missingIntegrity))
        XCTAssertNil(MaatOperationPreflight.decodeProjection(from: missingCount))
    }

    func testPreflightRequiresSuccessfulCleanExactCaseAndAction() {
        let original = caseEntry()
        XCTAssertEqual(evaluate(original: original, latest: original), .ready)

        XCTAssertRejected(evaluate(original: original, latest: original, exitStatus: 1))
        XCTAssertRejected(evaluate(original: original, latest: original, timedOut: true))
        XCTAssertRejected(evaluate(original: original, latest: original, launchError: "unavailable"))
        XCTAssertRejected(evaluate(original: original, latest: original, projection: false))
        XCTAssertRejected(evaluate(original: original, latest: original, invalidCount: 1))
        XCTAssertRejected(evaluate(original: original, latest: nil))
        XCTAssertRejected(evaluate(original: original, latest: original, duplicateCaseID: true))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(status: "resolved")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(evidence: "evidence-2")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(kind: "other-kind")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(resource: "other-resource")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(why: "changed reason")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(determination: "changed determination")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(actionKind: "other-action")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(actionID: "other-id")))
        XCTAssertRejected(evaluate(original: original, latest: caseEntry(actionEvidence: "other-action-evidence")))
    }

    @MainActor
    func testStoreClaimsOnceAndCompletedReceiptNeedsExplicitFreshReviewToRetry() {
        let key = MaatOperationKey(case: caseEntry())
        let store = MaatCaseOperationStore()

        XCTAssertTrue(store.begin(key))
        XCTAssertFalse(store.begin(key))
        store.markRunning(key)
        store.complete(key, receipt: MaatOperationReceipt(summary: "Recorded", succeeded: true))
        XCTAssertFalse(store.begin(key))
        XCTAssertEqual(store.record(for: key)?.receipt?.summary, "Recorded")

        let reviewedGeneration = store.record(for: key)!.generation
        store.authorizeReviewedRetry(key, reviewedGeneration: reviewedGeneration)
        XCTAssertEqual(store.record(for: key)?.phase, .retryReady)
        XCTAssertTrue(store.begin(key))
        XCTAssertFalse(store.begin(key))
        store.complete(key, receipt: MaatOperationReceipt(summary: "Recorded", succeeded: true))
        store.authorizeReviewedRetry(key, reviewedGeneration: reviewedGeneration)
        XCTAssertEqual(store.record(for: key)?.phase, .completed,
                       "A delayed prior review must not re-arm a later completed invocation")
    }

    func testOperationKeyTracksRetainedCaseAndActionBindingExactly() {
        let original = MaatOperationKey(case: caseEntry())
        XCTAssertEqual(original, MaatOperationKey(case: caseEntry()))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(status: "resolved")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(evidence: "evidence-2")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(kind: "other-kind")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(resource: "other-resource")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(why: "changed reason")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(determination: "changed determination")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(actionKind: "other-action")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(actionID: "other-id")))
        XCTAssertNotEqual(original, MaatOperationKey(case: caseEntry(actionEvidence: "other-action-evidence")))
    }

    private func evaluate(
        original: MaatCase,
        latest: MaatCase?,
        exitStatus: Int32? = 0,
        timedOut: Bool = false,
        launchError: String? = nil,
        projection: Bool = true,
        invalidCount: Int = 0,
        duplicateCaseID: Bool = false
    ) -> MaatCasePreflightDecision {
        var cases = latest.map { [$0] } ?? []
        if duplicateCaseID, let latest { cases.append(latest) }
        let readProjection = projection
            ? MaatCasebookProjection(
                cases: cases,
                summary: MaatCasebookSummary(total: cases.count, open: cases.count, urgent: 0, high: 0, resolved: 0),
                journalIntegrity: MaatJournalIntegrity(invalidCount: invalidCount, issues: [])
            )
            : nil
        return MaatOperationPreflight.evaluate(
            original: original,
            exitStatus: exitStatus,
            timedOut: timedOut,
            launchError: launchError,
            projection: readProjection
        )
    }

    private func XCTAssertRejected(_ decision: MaatCasePreflightDecision, file: StaticString = #filePath, line: UInt = #line) {
        guard case .rejected = decision else {
            return XCTFail("Expected preflight to reject this case binding", file: file, line: line)
        }
    }

    private func caseEntry(
        status: String = "open",
        kind: String = "diagnostic",
        evidence: String = "evidence-1",
        resource: String = "launch-agent",
        actionKind: String = "maat_repair",
        actionID: String = "repair-1",
        actionEvidence: String = "action-evidence-1",
        why: String = "retained reason",
        determination: String = "needs repair"
    ) -> MaatCase {
        MaatCase(
            id: "case-1",
            time: "",
            kind: kind,
            category: "system",
            status: status,
            priority: .normal,
            requester: "test",
            resource: resource,
            affected: "",
            determination: determination,
            assessed: "",
            why: why,
            evidence: evidence,
            nextAction: MaatCaseNextAction(
                kind: actionKind,
                actionID: actionID,
                title: "Repair",
                detail: "Bounded recovery",
                evidence: actionEvidence,
                requiresConfirmation: true
            )
        )
    }
}
