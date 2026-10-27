import XCTest
@testable import SirsiMenubar

final class MaatOperationStateTests: XCTestCase {
    @MainActor
    func testUpdaterHistoryLinksSurviveLoaderSearchAndExactActionPreflight() async throws {
        for mode in ["binary-only", "other-finding", "failed-floor", "failed", "pending"] {
            let resolved = mode == "binary-only"
            let verified = mode != "failed" && mode != "pending"
            let evidence = "screen:binary-history"
            let receipt = "receipt:\(mode)"
            let actionID = mode == "pending" ? "inspect_binary_drift_attempt" : "rescreen_system_one"
            var row: [String: Any] = [
                "id": "binary-case", "kind": "system one screen", "status": resolved ? "resolved" : "open",
                "evidence": evidence, "determination": "block", "resource": "", "why": ""
            ]
            if mode == "pending" {
                row["pending_repair_attempt_evidence"] = receipt
            } else {
                row["repair_outcome_evidence"] = receipt
                if verified { row["verified_repair_evidence"] = receipt }
            }
            if !resolved {
                row["next_action"] = ["kind": "system_one_review", "action_id": actionID,
                    "title": "Review retained outcome", "detail": "Fresh factual observation",
                    "evidence": evidence, "requires_confirmation": true]
            }
            let envelope: [String: Any] = [
                "cases": [row], "graph": ["nodes": [], "edges": []],
                "summary": ["total": 1, "open": resolved ? 0 : 1, "resolved": resolved ? 1 : 0, "urgent": 0, "high": 0],
                "journal_integrity": ["invalid_count": 0, "issues": []],
                "history": ["scope": "all", "query_id": "query:repair", "snapshot": "sha256:repair",
                    "returned": 1, "candidate_matches": 1, "count_scope": "query", "evidence_matches": 1,
                    "evidence_matches_complete": true, "candidate_matches_complete": true, "page_size": 50,
                    "coverage": "complete", "has_more": false, "has_more_known": true, "total_complete": true, "total": 1]
            ]
            let raw = try JSONSerialization.data(withJSONObject: envelope)
            let recorder = HistoryReadRecorder()
            await recorder.enqueue(SirsiEngine.JSONCommandRead(data: raw, exitStatus: 0, timedOut: false, launchError: nil))
            let store = MaatHistoryRefreshStore()
            await store.load(scope: "all", query: receipt) { await recorder.next(arguments: $0) }
            let projection = try XCTUnwrap(store.state.projection)
            let entry = try XCTUnwrap(projection.cases.first)
            XCTAssertNil(store.state.error)
            XCTAssertTrue(entry.searchText.contains(receipt))
            XCTAssertEqual(entry.pendingRepairAttemptEvidence, mode == "pending" ? receipt : "")
            XCTAssertEqual(entry.verifiedRepairEvidence, verified ? receipt : "")
            XCTAssertEqual(entry.repairOutcomeEvidence, mode == "pending" ? "" : receipt)
            if mode == "pending" {
                XCTAssertNil(maatCasebookRepairOutcomePresentation(entry))
            } else {
                let presentation = try XCTUnwrap(maatCasebookRepairOutcomePresentation(entry))
                XCTAssertEqual(presentation.title, verified ? "Verified binary repair receipt" : "Unverified repair outcome evidence")
            }
            let original = caseEntry(id: "binary-case", kind: "system one screen", evidence: evidence,
                resource: "", actionID: "binary-drift", actionEvidence: evidence, why: "", determination: "block")
            XCTAssertRejected(MaatOperationPreflight.evaluate(original: original, exitStatus: 0, timedOut: false, launchError: nil, projection: projection))
            if !resolved {
                XCTAssertEqual(MaatOperationPreflight.evaluate(original: entry, exitStatus: 0, timedOut: false, launchError: nil, projection: projection), .ready)
                XCTAssertEqual(maatCasebookSystemOneReviewArguments(kind: entry.nextAction?.kind ?? "", actionID: entry.nextAction?.actionID ?? "", evidence: evidence),
                    mode == "pending" ? ["maat", "repair", "binary-drift", "--inspect-attempt", "--case-evidence", evidence, "--json"] : ["maat", "triage", "--confirm", "--json"])
            }
            let arguments = await recorder.arguments()
            XCTAssertEqual(arguments, [maatCasebookHistoryArguments(scope: "all", query: receipt)])
        }
    }

    func testCasebookPreflightReadsExactEvidenceFromHistorySnapshot() {
        XCTAssertEqual(
            maatExactCasebookReadArguments(evidence: "diagnostic:exact"),
            ["maat", "casebook", "--json", "--scope", "all", "--evidence", "diagnostic:exact", "--limit", "2"]
        )
        XCTAssertEqual(
            maatExactCasebookReadArguments(evidence: "diagnostic:exact", cursor: "bounded-continuation"),
            ["maat", "casebook", "--json", "--scope", "all", "--evidence", "diagnostic:exact", "--limit", "2", "--cursor", "bounded-continuation"]
        )
        XCTAssertEqual(
            maatCasebookHistoryArguments(scope: "system-one"),
            ["maat", "casebook", "--json", "--scope", "system-one", "--limit", "50"]
        )
        XCTAssertEqual(
            maatCasebookHistoryArguments(scope: "all", cursor: "snapshot-cursor"),
            ["maat", "casebook", "--json", "--scope", "all", "--limit", "50", "--cursor", "snapshot-cursor"]
        )
        XCTAssertEqual(
            maatCasebookHistoryArguments(scope: "all", query: "exact older evidence", cursor: "query-bound-cursor"),
            ["maat", "casebook", "--json", "--scope", "all", "--limit", "50", "--cursor", "query-bound-cursor", "exact older evidence"]
        )
    }

    @MainActor
    func testCasebookSummaryDetailUsesOptionalHistoryArgument() {
        let summary = MaatCasebookSummary(total: 0, open: 0, urgent: 0, high: 0, resolved: 0)
        XCTAssertEqual(
            MaatCasebookView.summaryDetail(summary, history: nil),
            "No decisions match this page"
        )

        let partialHistory = MaatHistoryWindow(
            scope: "all", queryID: "query:bounded", snapshot: "sha256:bounded", returned: 0,
            candidateMatches: 0, countScope: "query", evidenceMatches: 0,
            evidenceMatchesComplete: true, candidateMatchesComplete: false, pageSize: 50,
            coverage: "partial", hasMore: true, hasMoreKnown: true, nextCursor: "cursor:next",
            totalComplete: false, total: nil, scannedRecords: 8, scannedBytes: 4096
        )
        XCTAssertEqual(
            MaatCasebookView.summaryDetail(summary, history: partialHistory),
            "No matches in this bounded scan chunk; history remains incomplete."
        )
    }

    func testLivenessFailureMemoryControlsUseOnlyClosedConfirmedMaAtActions() {
        XCTAssertEqual(
            maatCasebookFailureMemoryArguments(.initialize),
            ["maat", "repair", "liveness-watch", "--initialize-failure-memory", "--confirm", "--json"]
        )
        XCTAssertEqual(
            maatCasebookFailureMemoryArguments(.reobserve),
            ["maat", "repair", "liveness-watch", "--reobserve-incomplete", "--confirm", "--json"]
        )
        XCTAssertFalse(maatCasebookFailureMemoryArguments(.initialize).contains { $0.hasPrefix("/") })
    }

    func testLivenessFailureMemoryStatusRequiresStateAndDefaultsNoPendingAction() throws {
        let status = try JSONDecoder().decode(
            MaatFailureMemoryStatus.self,
            from: Data(#"{"state":"ready","store_uuid":"b3c515a2-4773-4653-86d1-8a5734c2ee45"}"#.utf8)
        )
        XCTAssertEqual(status.state, "ready")
        XCTAssertEqual(status.storeUUID, "b3c515a2-4773-4653-86d1-8a5734c2ee45")
        XCTAssertFalse(status.incompleteAttempt)
        XCTAssertFalse(status.projectionDebt)

        let firstUse = try JSONDecoder().decode(
            MaatFailureMemoryStatus.self,
            from: Data(#"{"state":"uninitialized"}"#.utf8)
        )
        XCTAssertEqual(firstUse.state, "uninitialized")

        let pending = try JSONDecoder().decode(
            MaatFailureMemoryStatus.self,
            from: Data(#"{"state":"ready","incomplete_attempt":true}"#.utf8)
        )
        XCTAssertTrue(pending.incompleteAttempt)

        let projectionDebt = try JSONDecoder().decode(
            MaatFailureMemoryStatus.self,
            from: Data(#"{"state":"ready","projection_debt":true}"#.utf8)
        )
        XCTAssertTrue(projectionDebt.projectionDebt)
    }

    func testPreflightProjectionRequiresExplicitJournalIntegrityCount() {
        let complete = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"invalid_count":0,"issues":[]},"history":{"scope":"all","query_id":"query:test","snapshot":"sha256:test","returned":0,"candidate_matches":0,"count_scope":"query","candidate_matches_complete":true,"page_size":100,"coverage":"complete","has_more":false,"has_more_known":true,"total_complete":true,"total":0}}"#.utf8)
        let missingIntegrity = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0}}"#.utf8)
        let missingCount = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"issues":[]}}"#.utf8)

        XCTAssertNotNil(MaatOperationPreflight.decodeProjection(from: complete))
        XCTAssertNil(MaatOperationPreflight.decodeProjection(from: missingIntegrity))
        XCTAssertNil(MaatOperationPreflight.decodeProjection(from: missingCount))
        let nullCases = Data(#"{"cases":null,"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"invalid_count":0,"issues":[]}}"#.utf8)
        XCTAssertNil(MaatOperationPreflight.decodeProjection(from: nullCases))
        let unqualifiedHistory = Data(#"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"invalid_count":0,"issues":[]},"history":{"scope":"all","query_id":"query:test","snapshot":"sha256:test","returned":0,"candidate_matches":0,"count_scope":"scanned-so-far","candidate_matches_complete":false,"page_size":100,"coverage":"partial","has_more":false,"has_more_known":false,"total_complete":false}}"#.utf8)
        XCTAssertEqual(
            MaatOperationPreflight.evaluate(original: caseEntry(), exitStatus: 0, timedOut: false, launchError: nil,
                projection: MaatOperationPreflight.decodeProjection(from: unqualifiedHistory)),
            .rejected("The exact case history is incomplete or changed. Refresh and review the case before trying again.")
        )
    }

    func testActualHistoryLoaderRequiresZeroExitAndRetainsPriorPageOnFailure() throws {
        let emptyJSON = #"{"cases":[],"summary":{"total":0,"open":0,"urgent":0,"high":0,"resolved":0},"journal_integrity":{"invalid_count":0,"issues":[]},"history":{"scope":"system-one","query_id":"query:empty","snapshot":"sha256:empty","returned":0,"candidate_matches":0,"count_scope":"query","candidate_matches_complete":true,"page_size":50,"coverage":"complete","has_more":false,"has_more_known":true,"total_complete":true,"total":0}}"#
        let bytes = Data(emptyJSON.utf8)
        let prior = try XCTUnwrap(MaatCasebookView.decode(SirsiEngine.JSONCommandRead(data: bytes, exitStatus: 0, timedOut: false, launchError: nil)))
        XCTAssertTrue(prior.cases.isEmpty)
        XCTAssertEqual(prior.historyEmptyState, .empty)

        let rejectedReads = [
            SirsiEngine.JSONCommandRead(data: bytes, exitStatus: 9, timedOut: false, launchError: nil),
            SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil, timedOut: true, launchError: "timeout"),
            SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil, timedOut: false, launchError: "cancelled"),
            SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil, timedOut: false, launchError: "output limit exceeded"),
            SirsiEngine.JSONCommandRead(data: Data("snapshot changed; refresh".utf8), exitStatus: 1, timedOut: false, launchError: nil),
            SirsiEngine.JSONCommandRead(data: Data("{".utf8), exitStatus: 0, timedOut: false, launchError: nil),
        ]
        for read in rejectedReads {
            var state = MaatHistoryRefreshState(projection: prior, error: nil)
            receiveHistory(read, state: &state, scope: "system-one")
            XCTAssertEqual(state.projection?.history?.queryID, "query:empty")
            XCTAssertNotNil(state.error)
        }

        let partial = SirsiEngine.JSONCommandRead(data: historyEnvelope(
            coverage: "partial", cursor: "bounded-next", countScope: "scanned-so-far"),
            exitStatus: 0, timedOut: false, launchError: nil)
        var continued = MaatHistoryRefreshState(projection: prior, error: nil)
        receiveHistory(partial, state: &continued, scope: "all")
        XCTAssertEqual(continued.projection?.history?.coverage, "partial")
        XCTAssertEqual(continued.projection?.history?.nextCursor, "bounded-next")
        XCTAssertNil(continued.error)
        XCTAssertEqual(continued.projection?.historyEmptyState, .incomplete)
        receiveHistory(SirsiEngine.JSONCommandRead(data: bytes, exitStatus: 0, timedOut: false, launchError: nil), state: &continued, scope: "system-one")
        XCTAssertEqual(continued.projection?.historyEmptyState, .empty)
        XCTAssertNil(continued.error)

        var tail = try XCTUnwrap(JSONSerialization.jsonObject(with: bytes) as? [String: Any])
        var tailHistory = try XCTUnwrap(tail["history"] as? [String: Any])
        tailHistory["count_scope"] = "after-cursor"
        tailHistory["total_complete"] = false
        tailHistory.removeValue(forKey: "total")
        tail["history"] = tailHistory
        receiveHistory(SirsiEngine.JSONCommandRead(data: try JSONSerialization.data(withJSONObject: tail), exitStatus: 0, timedOut: false, launchError: nil), state: &continued, scope: "system-one")
        XCTAssertEqual(continued.projection?.historyEmptyState, .afterCursor)
        tail["journal_integrity"] = ["invalid_count": 1, "issues": []]
        receiveHistory(SirsiEngine.JSONCommandRead(data: try JSONSerialization.data(withJSONObject: tail), exitStatus: 0, timedOut: false, launchError: nil), state: &continued, scope: "system-one")
        XCTAssertEqual(continued.projection?.historyEmptyState, .invalidHistory)
    }

    func testHistoryRefreshClassifiesExactSnapshotRefusalWithoutExposingStderr() throws {
        let previousPage = try XCTUnwrap(MaatCasebookView.decode(
            browseRead(scope: "system-one", queryID: "query:old", snapshot: "sha256:old", cursor: "stale-cursor")))
        let canonical = "maat decision history: snapshot changed; refresh the query"
        let actualANSISurface = "  \u{001B}[1;38;2;242;84;91m𓁵\u{001B}[m \u{001B}[38;2;230;239;233m\(canonical)\u{001B}[m\n"
        let captured = CommandExecution(stderr: Data(actualANSISurface.utf8), exitCode: 1,
            childState: .exitConfirmed).jsonRead
        XCTAssertEqual(captured.stderrData, Data(actualANSISurface.utf8))
        let staleRead = captured

        func classify(_ read: SirsiEngine.JSONCommandRead) -> MaatHistoryRefreshState {
            var state = MaatHistoryRefreshState(projection: previousPage)
            guard let request = state.begin(scope: "system-one", cursor: "stale-cursor") else {
                XCTFail("Retained cursor should belong to the previous page")
                return state
            }
            state.complete(read, for: request, cancelled: false)
            return state
        }

        let stale = classify(staleRead)
        XCTAssertEqual(stale.failure, .staleSnapshot)
        XCTAssertEqual(stale.projection?.history?.snapshot, "sha256:old")
        XCTAssertTrue(stale.error?.contains("restart from the latest snapshot") == true)
        XCTAssertFalse(stale.error?.contains(canonical) == true)

        let nearMatch = classify(SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false,
            launchError: nil, stderrData: Data("\(canonical); private=do-not-display\n".utf8)))
        XCTAssertEqual(nearMatch.failure, .commandFailed)
        XCTAssertFalse(nearMatch.error?.contains("do-not-display") == true)
        XCTAssertEqual(classify(SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false,
            launchError: nil, stderrData: Data("unrecognized refusal\n".utf8))).failure, .commandFailed)
        XCTAssertEqual(classify(SirsiEngine.JSONCommandRead(data: Data("{".utf8), exitStatus: 0,
            timedOut: false, launchError: nil)).failure, .malformedResponse)
        XCTAssertEqual(classify(SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil,
            timedOut: true, launchError: "deadline expired")).failure, .timedOut)

        let partial = MaatCasebookView.decode(SirsiEngine.JSONCommandRead(
            data: historyEnvelope(coverage: "partial", cursor: "next", countScope: "scanned-so-far"),
            exitStatus: 0, timedOut: false, launchError: nil))
        XCTAssertEqual(partial?.history?.scannedRecords, 1)
        XCTAssertEqual(partial?.history?.scannedBytes, 10)
        XCTAssertEqual(partial?.history?.boundedProgressText,
            "This bounded read scanned 1 journal rows (10 bytes). Results remain incomplete.")
    }

    @MainActor
    func testStaleCursorRestartRetainsPageAndRetriesWithoutCursor() async throws {
        let responses = ControlledHistoryResponses()
        let store = MaatHistoryRefreshStore()
        let staleRead = SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false,
            launchError: nil, stderrData: Data("𓁵 maat decision history: snapshot changed; refresh the query\n".utf8))

        let initial = Task {
            await store.load(scope: "all", query: "needle") { arguments in
                await responses.read(arguments)
            }
        }
        await responses.waitForCount(1)
        let firstArguments = await responses.arguments()
        XCTAssertEqual(firstArguments, [maatCasebookHistoryArguments(scope: "all", query: "needle")])
        let oldPage = try browseRead(scope: "all", queryID: "query:old", snapshot: "sha256:old", cursor: "stale-cursor")
        await responses.finish(0, with: oldPage)
        await initial.value
        XCTAssertEqual(store.state.query, "needle")

        let continuation = Task {
            await store.load(scope: "all", cursor: "stale-cursor") { arguments in
                await responses.read(arguments)
            }
        }
        await responses.waitForCount(2)
        let continuationArguments = await responses.arguments()
        XCTAssertEqual(continuationArguments[1], maatCasebookHistoryArguments(scope: "all", query: "needle", cursor: "stale-cursor"))
        await responses.finish(1, with: staleRead)
        await continuation.value
        XCTAssertEqual(store.state.failure, .staleSnapshot)
        XCTAssertEqual(store.state.projection?.history?.snapshot, "sha256:old")
        XCTAssertEqual(store.state.query, "needle")
        XCTAssertEqual(store.state.projection?.history?.nextCursor, "stale-cursor")

        let restart = Task {
            await store.load(scope: "all") { arguments in
                await responses.read(arguments)
            }
        }
        await responses.waitForCount(3)
        XCTAssertTrue(store.state.loading)
        XCTAssertEqual(store.state.projection?.history?.snapshot, "sha256:old",
            "The retained page stays available while the fresh snapshot read is pending")
        XCTAssertEqual(store.state.failure, .staleSnapshot)
        XCTAssertEqual(store.state.query, "needle")
        let allArguments = await responses.arguments()
        XCTAssertEqual(allArguments[2], maatCasebookHistoryArguments(scope: "all", query: "needle"),
            "Restart must preserve the query and omit the stale cursor")

        let fresh = try browseRead(scope: "all", queryID: "query:fresh", snapshot: "sha256:fresh")
        await responses.finish(2, with: fresh)
        await restart.value
        XCTAssertFalse(store.state.loading)
        XCTAssertNil(store.state.error)
        XCTAssertNil(store.state.failure)
        XCTAssertEqual(store.state.projection?.history?.snapshot, "sha256:fresh")
        XCTAssertEqual(store.state.projection?.history?.queryID, "query:fresh")
    }

    func testExactActionPreflightConsumesOnlyBoundedSameSnapshotScanContinuations() async throws {
        let recorder = HistoryReadRecorder()
        let partial = SirsiEngine.JSONCommandRead(data: historyEnvelope(
            coverage: "partial", cursor: "continue-1", countScope: "scanned-so-far"),
            exitStatus: 0, timedOut: false, launchError: nil)
        let complete = SirsiEngine.JSONCommandRead(data: try selectedHistoryEnvelope(),
            exitStatus: 0, timedOut: false, launchError: nil)
        await recorder.enqueue(partial)
        await recorder.enqueue(complete)
        let read = await MaatOperationPreflight.readCompleteExactHistory(evidence: "evidence-1") { args, _ in
            await recorder.next(arguments: args)
        }
        XCTAssertEqual(read.exitStatus, 0)
        let callCount = await recorder.callCount()
        XCTAssertEqual(callCount, 2)
        let calls = await recorder.arguments()
        XCTAssertFalse(calls[0].contains("--cursor"))
        XCTAssertEqual(Array(calls[1].suffix(2)), ["--cursor", "continue-1"])
        XCTAssertEqual(MaatOperationPreflight.evaluate(original: caseEntry(), exitStatus: read.exitStatus,
            timedOut: read.timedOut, launchError: read.launchError,
            projection: MaatOperationPreflight.decodeProjection(from: read.data)), .ready)
    }

    @MainActor
    func testHistoryRefreshIgnoresDelayedFailureAndSuccessAfterNewerCommit() async throws {
        for scope in ["all", "system-one"] {
            let oldRead = try browseRead(scope: scope, queryID: "query:old", snapshot: "snapshot:old")
            let newRead = try browseRead(scope: scope, queryID: "query:new", snapshot: "snapshot:new")
            for stale in [SirsiEngine.JSONCommandRead(data: oldRead.data, exitStatus: 7, timedOut: false, launchError: nil), oldRead] {
                let responses = ControlledHistoryResponses()
                let store = MaatHistoryRefreshStore(projection: MaatCasebookView.decode(oldRead))
                let a = Task { await store.load(scope: scope, query: "old") { await responses.read($0) } }
                await responses.waitForCount(1)
                let b = Task { await store.load(scope: scope, query: "new") { await responses.read($0) } }
                await responses.waitForCount(2)
                await responses.finish(1, with: newRead)
                await b.value
                XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new")
                XCTAssertEqual(store.state.query, "new")
                await responses.finish(0, with: stale)
                await a.value
                XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new", scope)
                XCTAssertEqual(store.state.query, "new")
                XCTAssertNil(store.state.error)
                XCTAssertFalse(store.state.loading)

                // A current failure retains B's committed projection, even
                // though A began with an older projection before B existed.
                let c = Task { await store.load(scope: scope) { await responses.read($0) } }
                await responses.waitForCount(3)
                await responses.finish(2, with: stale.exitStatus == 0
                    ? SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 8, timedOut: false, launchError: nil) : stale)
                await c.value
                XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new")
                XCTAssertEqual(store.state.query, "new")
                XCTAssertNotNil(store.state.error)
                XCTAssertFalse(store.state.loading)
            }
        }
    }

    @MainActor
    func testHistoryRefreshCancellationCannotReplaceNewSnapshotOrClearCurrentLoading() async throws {
        for scope in ["all", "system-one"] {
            let oldRead = try browseRead(scope: scope, queryID: "query:old", snapshot: "snapshot:old")
            let newRead = try browseRead(scope: scope, queryID: "query:new", snapshot: "snapshot:new")
            let newerRead = try browseRead(scope: scope, queryID: "query:newer", snapshot: "snapshot:newer")
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore(projection: MaatCasebookView.decode(oldRead))
            let a = Task { await store.load(scope: scope, query: "old") { await responses.read($0) } }
            await responses.waitForCount(1)
            a.cancel()
            let b = Task { await store.load(scope: scope, query: "new") { await responses.read($0) } }
            await responses.waitForCount(2)
            await responses.finish(0, with: oldRead)
            await a.value
            XCTAssertTrue(store.state.loading, "A's cancellation must not finish B's load")
            XCTAssertNil(store.state.error)
            await responses.finish(1, with: newRead)
            await b.value
            XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new")
            XCTAssertEqual(store.state.query, "new")
            XCTAssertNil(store.state.error)
            XCTAssertFalse(store.state.loading)

            let c = Task { await store.load(scope: scope, query: "cancelled") { await responses.read($0) } }
            await responses.waitForCount(3)
            c.cancel()
            await responses.finish(2, with: oldRead)
            await c.value
            XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new")
            XCTAssertEqual(store.state.query, "new")
            XCTAssertTrue(store.state.error?.contains("cancelled") == true)
            XCTAssertFalse(store.state.loading)

            let d = Task { await store.load(scope: scope, query: "old") { await responses.read($0) } }
            await responses.waitForCount(4)
            d.cancel()
            let e = Task { await store.load(scope: scope, query: "newer") { await responses.read($0) } }
            await responses.waitForCount(5)
            await responses.finish(4, with: newerRead)
            await e.value
            await responses.finish(3, with: oldRead)
            await d.value
            XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:newer")
            XCTAssertEqual(store.state.query, "newer")
            XCTAssertNil(store.state.error)
            XCTAssertFalse(store.state.loading)
        }
    }

    @MainActor
    func testHistoryContinuationCommitsQueryAndCursorTogetherAndRejectsMismatchedPair() async throws {
        let initial = try browseRead(scope: "all", queryID: "query:needle", snapshot: "snapshot:stable", cursor: "cursor:one")
        let tail = try browseRead(scope: "all", queryID: "query:needle", snapshot: "snapshot:stable", cursor: "cursor:two")
        let changed = try browseRead(scope: "all", queryID: "query:other", snapshot: "snapshot:replacement")
        let responses = ControlledHistoryResponses()
        let store = MaatHistoryRefreshStore()
        let first = Task { await store.load(scope: "all", query: "  needle  ") { await responses.read($0) } }
        await responses.waitForCount(1)
        await responses.finish(0, with: initial)
        await first.value
        XCTAssertEqual(store.state.query, "needle")
        XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:one")
        let next = Task { await store.load(scope: "all", cursor: "cursor:one") { await responses.read($0) } }
        await responses.waitForCount(2)
        let arguments = await responses.arguments()
        XCTAssertEqual(arguments[1], maatCasebookHistoryArguments(scope: "all", query: "needle", cursor: "cursor:one"))
        await responses.finish(1, with: tail)
        await next.value
        XCTAssertEqual(store.state.query, "needle")
        XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:two")
        await store.load(scope: "all", query: "different", cursor: "cursor:two") { await responses.read($0) }
        let calls = await responses.arguments()
        XCTAssertEqual(calls.count, 2, "A mismatched query/cursor must not launch a read")
        XCTAssertEqual(store.state.query, "needle")
        XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:two")
        XCTAssertNotNil(store.state.error)
        XCTAssertFalse(store.state.loading)
        let drift = Task { await store.load(scope: "all", cursor: "cursor:two") { await responses.read($0) } }
        await responses.waitForCount(3)
        await responses.finish(2, with: changed)
        await drift.value
        XCTAssertEqual(store.state.query, "needle")
        XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:stable")
        XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:two")
        XCTAssertNotNil(store.state.error)
        XCTAssertFalse(store.state.loading)
        try await assertOptionalGoProducedHistoryFixture()
    }

    @MainActor
    func testHistoryRefreshAutomaticallyContinuesBothScopesAndStopsAtCompleteOlderCursor() async throws {
        for scope in ["all", "system-one"] {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore(maximumBurstReads: 8)
            let task = Task {
                await store.load(scope: scope, query: "needle") { await responses.read($0) }
            }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: scope, queryID: "query:stable",
                snapshot: "snapshot:stable", coverage: "partial", cursor: "scan:one",
                countScope: "scanned-so-far"))
            await responses.waitForCount(2)
            XCTAssertTrue(store.state.loading, "The burst remains busy between subprocess reads")
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "scan:one")
            await responses.finish(1, with: try historyRead(scope: scope, queryID: "query:stable",
                snapshot: "snapshot:stable", coverage: "partial", cursor: "scan:two",
                countScope: "after-cursor-scanned-so-far"))
            await responses.waitForCount(3)
            XCTAssertTrue(store.state.loading)
            await responses.finish(2, with: try historyRead(scope: scope, queryID: "query:stable",
                snapshot: "snapshot:stable", coverage: "complete", cursor: "older:page",
                countScope: "query", hasMore: true))
            await task.value

            XCTAssertFalse(store.state.loading)
            XCTAssertNil(store.state.error)
            XCTAssertEqual(store.state.projection?.history?.coverage, "complete")
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "older:page")
            let arguments = await responses.arguments()
            XCTAssertEqual(arguments, [
                maatCasebookHistoryArguments(scope: scope, query: "needle"),
                maatCasebookHistoryArguments(scope: scope, query: "needle", cursor: "scan:one"),
                maatCasebookHistoryArguments(scope: scope, query: "needle", cursor: "scan:two"),
            ])
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 3, "A complete page's older cursor is user driven")
        }
    }

    @MainActor
    func testHistoryRefreshExplicitOlderPageMayAutoContinueItsPartialScan() async throws {
        let older = try historyRead(scope: "all", queryID: "query:older", snapshot: "snapshot:stable",
            coverage: "complete", cursor: "older:one", countScope: "query", hasMore: true)
        let store = MaatHistoryRefreshStore(projection: MaatCasebookView.decode(older))
        let responses = ControlledHistoryResponses()
        let task = Task {
            await store.load(scope: "all", cursor: "older:one") { await responses.read($0) }
        }
        await responses.waitForCount(1)
        await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:older",
            snapshot: "snapshot:stable", coverage: "partial", cursor: "older:scan",
            countScope: "after-cursor-scanned-so-far"))
        await responses.waitForCount(2)
        await responses.finish(1, with: try historyRead(scope: "all", queryID: "query:older",
            snapshot: "snapshot:stable", coverage: "complete", cursor: "older:two",
            countScope: "after-cursor", hasMore: true))
        await task.value

        XCTAssertEqual(store.state.projection?.history?.coverage, "complete")
        XCTAssertEqual(store.state.projection?.history?.nextCursor, "older:two")
        let arguments = await responses.arguments()
        XCTAssertEqual(arguments, [
            maatCasebookHistoryArguments(scope: "all", cursor: "older:one"),
            maatCasebookHistoryArguments(scope: "all", cursor: "older:scan"),
        ])
    }

    @MainActor
    func testHistoryRefreshCallAndClockBudgetsPauseWithAcceptedCursorForExplicitContinue() async throws {
        do {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore(maximumBurstReads: 1)
            let first = Task { await store.load(scope: "all") { await responses.read($0) } }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:budget",
                snapshot: "snapshot:budget", coverage: "partial", cursor: "cursor:accepted",
                countScope: "scanned-so-far"))
            await first.value
            XCTAssertFalse(store.state.loading)
            XCTAssertTrue(store.state.continuationPaused)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:accepted")

            let continued = Task {
                await store.load(scope: "all", cursor: "cursor:accepted") { await responses.read($0) }
            }
            await responses.waitForCount(2)
            await responses.finish(1, with: try historyRead(scope: "all", queryID: "query:budget",
                snapshot: "snapshot:budget", coverage: "complete", cursor: nil,
                countScope: "query"))
            await continued.value
            XCTAssertNil(store.state.error)
            XCTAssertFalse(store.state.continuationPaused)
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 2)
        }

        do {
            let responses = ControlledHistoryResponses()
            let clock = TestHistoryMonotonicClock([0, 0, 0, 8])
            let store = MaatHistoryRefreshStore(monotonicNow: { clock.now() })
            let task = Task { await store.load(scope: "system-one") { await responses.read($0) } }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "system-one", queryID: "query:clock",
                snapshot: "snapshot:clock", coverage: "partial", cursor: "cursor:clock",
                countScope: "scanned-so-far"))
            await task.value
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 1, "The monotonic deadline prevents another dispatch")
            XCTAssertTrue(store.state.continuationPaused)
            XCTAssertNil(store.state.failure)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:clock")
        }
    }

    @MainActor
    func testHistoryRefreshDeadlineAndUserCancelKeepCursorAndDoNotDispatchAgain() async throws {
        do {
            let responses = ControlledHistoryResponses()
            let deadline = ControlledHistoryDeadline()
            let store = MaatHistoryRefreshStore(deadlineSleep: { _ in await deadline.sleep() },
                deadlineHandled: { await deadline.markHandled() })
            let task = Task { await store.load(scope: "all") { await responses.read($0) } }
            await deadline.waitForCount(1)
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:deadline",
                snapshot: "snapshot:deadline", coverage: "partial", cursor: "cursor:kept",
                countScope: "scanned-so-far"))
            await responses.waitForCount(2)
            await deadline.releaseAll()
            await deadline.waitForHandled(1)
            XCTAssertFalse(store.state.loading)
            XCTAssertTrue(store.state.continuationPaused)
            XCTAssertNil(store.state.failure)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:kept")
            await responses.finish(1, with: try historyRead(scope: "all", queryID: "query:deadline",
                snapshot: "snapshot:deadline", coverage: "partial", cursor: "cursor:late",
                countScope: "after-cursor-scanned-so-far"))
            await task.value
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 2)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:kept")
            await deadline.releaseAll()
        }

        do {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore()
            let task = Task { await store.load(scope: "system-one") { await responses.read($0) } }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "system-one", queryID: "query:cancel",
                snapshot: "snapshot:cancel", coverage: "partial", cursor: "cursor:kept",
                countScope: "scanned-so-far"))
            await responses.waitForCount(2)
            store.cancel()
            XCTAssertFalse(store.state.loading)
            XCTAssertTrue(store.state.continuationPaused)
            XCTAssertNil(store.state.error)
            await responses.finish(1, with: try historyRead(scope: "system-one", queryID: "query:cancel",
                snapshot: "snapshot:cancel", coverage: "partial", cursor: "cursor:late",
                countScope: "after-cursor-scanned-so-far"))
            await task.value
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 2)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:kept")
        }
    }

    @MainActor
    func testSupersedingHistoryQueryRejectsOldSuccessFailureCancellationAndDeadline() async throws {
        let staleResults = [
            SirsiEngine.JSONCommandRead(data: try historyRead(scope: "all", queryID: "query:old",
                snapshot: "snapshot:old", coverage: "complete", cursor: nil, countScope: "query").data,
                exitStatus: 0, timedOut: false, launchError: nil),
            SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 9, timedOut: false, launchError: nil),
            SirsiEngine.JSONCommandRead(data: Data(), exitStatus: nil, timedOut: false, launchError: "cancelled"),
        ]
        for stale in staleResults {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore()
            let old = Task { await store.load(scope: "all", query: "old") { await responses.read($0) } }
            await responses.waitForCount(1)
            let fresh = Task { await store.load(scope: "all", query: "new") { await responses.read($0) } }
            await responses.waitForCount(2)
            await responses.finish(1, with: try historyRead(scope: "all", queryID: "query:new",
                snapshot: "snapshot:new", coverage: "complete", cursor: nil, countScope: "query"))
            await fresh.value
            await responses.finish(0, with: stale)
            await old.value
            XCTAssertEqual(store.state.query, "new")
            XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new")
            XCTAssertNil(store.state.error)
            XCTAssertFalse(store.state.loading)
        }

        let responses = ControlledHistoryResponses()
        let deadline = ControlledHistoryDeadline()
        let store = MaatHistoryRefreshStore(deadlineSleep: { _ in await deadline.sleep() },
            deadlineHandled: { await deadline.markHandled() })
        let old = Task { await store.load(scope: "all", query: "old") { await responses.read($0) } }
        await deadline.waitForCount(1)
        await responses.waitForCount(1)
        let fresh = Task { await store.load(scope: "all", query: "new") { await responses.read($0) } }
        await deadline.waitForCount(2)
        await responses.waitForCount(2)
        await responses.finish(1, with: try historyRead(scope: "all", queryID: "query:new",
            snapshot: "snapshot:new", coverage: "complete", cursor: nil, countScope: "query"))
        await fresh.value
        await deadline.releaseAll()
        await deadline.waitForHandled(2)
        await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:old",
            snapshot: "snapshot:old", coverage: "complete", cursor: nil, countScope: "query"))
        await old.value
        XCTAssertEqual(store.state.query, "new")
        XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:new")
        XCTAssertNil(store.state.error)
        XCTAssertFalse(store.state.loading)
    }

    @MainActor
    func testHistoryRefreshRejectsMissingRepeatedCyclicAndDriftedContinuations() async throws {
        do {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore()
            let task = Task { await store.load(scope: "all") { await responses.read($0) } }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:missing",
                snapshot: "snapshot:missing", coverage: "partial", cursor: nil,
                countScope: "scanned-so-far"))
            await task.value
            XCTAssertTrue(store.state.restartRequired)
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 1)
        }

        for cursors in [["repeat", "repeat"], ["cycle:a", "cycle:b", "cycle:a"]] {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore()
            let task = Task { await store.load(scope: "all") { await responses.read($0) } }
            for (index, cursor) in cursors.enumerated() {
                await responses.waitForCount(index + 1)
                await responses.finish(index, with: try historyRead(scope: "all", queryID: "query:loop",
                    snapshot: "snapshot:loop", coverage: "partial", cursor: cursor,
                    countScope: index == 0 ? "scanned-so-far" : "after-cursor-scanned-so-far"))
            }
            await task.value
            XCTAssertTrue(store.state.restartRequired)
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, cursors.count)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, cursors[cursors.count - 2],
                "The last accepted cursor remains visible while restart is required")
        }

        for drift in ["query", "snapshot"] {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore()
            let task = Task { await store.load(scope: "all") { await responses.read($0) } }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:stable",
                snapshot: "snapshot:stable", coverage: "partial", cursor: "cursor:stable",
                countScope: "scanned-so-far"))
            await responses.waitForCount(2)
            await responses.finish(1, with: try historyRead(scope: "all",
                queryID: drift == "query" ? "query:changed" : "query:stable",
                snapshot: drift == "snapshot" ? "snapshot:changed" : "snapshot:stable",
                coverage: "partial", cursor: "cursor:drifted", countScope: "after-cursor-scanned-so-far"))
            await task.value
            XCTAssertEqual(store.state.failure, .staleSnapshot)
            XCTAssertTrue(store.state.restartRequired)
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:stable")
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 2)
        }
    }

    @MainActor
    func testHistoryRefreshMidBurstCommandAndDecodeFailuresRetainAcceptedProgress() async throws {
        let failures = [
            SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 7, timedOut: false, launchError: nil),
            SirsiEngine.JSONCommandRead(data: Data("{".utf8), exitStatus: 0, timedOut: false, launchError: nil),
        ]
        for failure in failures {
            let responses = ControlledHistoryResponses()
            let store = MaatHistoryRefreshStore()
            let task = Task { await store.load(scope: "all") { await responses.read($0) } }
            await responses.waitForCount(1)
            await responses.finish(0, with: try historyRead(scope: "all", queryID: "query:retained",
                snapshot: "snapshot:retained", coverage: "partial", cursor: "cursor:retained",
                countScope: "scanned-so-far"))
            await responses.waitForCount(2)
            await responses.finish(1, with: failure)
            await task.value
            XCTAssertEqual(store.state.projection?.history?.nextCursor, "cursor:retained")
            XCTAssertEqual(store.state.projection?.history?.snapshot, "snapshot:retained")
            XCTAssertNotNil(store.state.error)
            let callCount = await responses.callCount()
            XCTAssertEqual(callCount, 2)
        }
    }

    private func historyRead(
        scope: String,
        queryID: String,
        snapshot: String,
        coverage: String,
        cursor: String?,
        countScope: String,
        hasMore: Bool = false
    ) throws -> SirsiEngine.JSONCommandRead {
        var root = try XCTUnwrap(JSONSerialization.jsonObject(with: historyEnvelope(
            coverage: coverage, cursor: cursor, countScope: countScope)) as? [String: Any])
        var history = try XCTUnwrap(root["history"] as? [String: Any])
        history["scope"] = scope
        history["query_id"] = queryID
        history["snapshot"] = snapshot
        history["coverage"] = coverage
        history["count_scope"] = countScope
        history["next_cursor"] = cursor
        history["has_more"] = hasMore
        history["has_more_known"] = coverage == "complete"
        history["candidate_matches_complete"] = coverage == "complete"
        history["evidence_matches_complete"] = coverage == "complete"
        history["total_complete"] = coverage == "complete"
        if coverage == "complete" {
            history["total"] = hasMore ? 2 : 0
        } else {
            history.removeValue(forKey: "total")
        }
        root["history"] = history
        return SirsiEngine.JSONCommandRead(data: try JSONSerialization.data(withJSONObject: root),
            exitStatus: 0, timedOut: false, launchError: nil)
    }

    func testExactActionPreflightRefusesChangedSnapshotAndBoundedExhaustion() async throws {
        let changed = HistoryReadRecorder()
        await changed.enqueue(SirsiEngine.JSONCommandRead(data: historyEnvelope(
            coverage: "partial", cursor: "continue-1", countScope: "scanned-so-far"),
            exitStatus: 0, timedOut: false, launchError: nil))
        var changedEnvelope = try XCTUnwrap(JSONSerialization.jsonObject(with: historyEnvelope(
            coverage: "complete", cursor: nil, countScope: "query")) as? [String: Any])
        var changedHistory = try XCTUnwrap(changedEnvelope["history"] as? [String: Any])
        changedHistory["snapshot"] = "sha256:replacement"
        changedEnvelope["history"] = changedHistory
        await changed.enqueue(SirsiEngine.JSONCommandRead(data: try JSONSerialization.data(withJSONObject: changedEnvelope),
            exitStatus: 0, timedOut: false, launchError: nil))
        let changedRead = await MaatOperationPreflight.readCompleteExactHistory(evidence: "diagnostic:old") { args, _ in
            await changed.next(arguments: args)
        }
        XCTAssertNil(changedRead.exitStatus)
        XCTAssertTrue(changedRead.launchError?.contains("snapshot changed") == true)

        let exhausted = HistoryReadRecorder()
        await exhausted.enqueue(SirsiEngine.JSONCommandRead(data: historyEnvelope(
            coverage: "partial", cursor: "continue-1", countScope: "scanned-so-far"),
            exitStatus: 0, timedOut: false, launchError: nil))
        let boundedRead = await MaatOperationPreflight.readCompleteExactHistory(evidence: "diagnostic:old", maximumReads: 1) { args, _ in
            await exhausted.next(arguments: args)
        }
        XCTAssertEqual(boundedRead.exitStatus, 0)
        let progress = try XCTUnwrap(boundedRead.continuation)
        XCTAssertEqual(progress.cursor, "continue-1")
        XCTAssertRejected(MaatOperationPreflight.evaluate(original: caseEntry(), exitStatus: boundedRead.exitStatus,
            timedOut: boundedRead.timedOut, launchError: boundedRead.launchError,
            projection: MaatOperationPreflight.decodeProjection(from: boundedRead.data)))
        let callCount = await exhausted.callCount()
        XCTAssertEqual(callCount, 1)
        await exhausted.enqueue(SirsiEngine.JSONCommandRead(data: historyEnvelope(
            coverage: "complete", cursor: nil, countScope: "query"), exitStatus: 0, timedOut: false, launchError: nil))
        let resumed = await MaatOperationPreflight.readCompleteExactHistory(evidence: "diagnostic:old", continuing: progress) { args, _ in
            await exhausted.next(arguments: args)
        }
        XCTAssertNil(resumed.continuation)
        XCTAssertEqual(resumed.exitStatus, 0)
        let calls = await exhausted.arguments()
        XCTAssertEqual(Array(calls[1].suffix(2)), ["--cursor", "continue-1"])
    }

    func testExactHistoryCancellationNeverConsumesOrRetainsActionProgress() async {
        let recorder = HistoryReadRecorder()
        let task = Task {
            withUnsafeCurrentTask { $0?.cancel() }
            return await MaatOperationPreflight.readCompleteExactHistory(evidence: "diagnostic:old") { args, _ in
                await recorder.next(arguments: args)
            }
        }
        let read = await task.value
        XCTAssertNil(read.continuation)
        XCTAssertNil(read.exitStatus)
        XCTAssertTrue(read.launchError?.contains("cancelled") == true)
        let count = await recorder.callCount()
        XCTAssertEqual(count, 0)
    }

    @MainActor
    func testExactHistoryProgressIsBoundToOperationGenerationAndNeverAuthorizesAction() throws {
        let key = MaatOperationKey(case: caseEntry())
        let store = MaatCaseOperationStore()
        XCTAssertTrue(store.begin(key))
        let generation = try XCTUnwrap(store.record(for: key)?.generation)
        let continuation = MaatExactHistoryContinuation(evidence: key.evidence, queryID: "query:exact", snapshot: "sha256:stable", cursor: "bounded-progress")
        store.pauseHistory(key, generation: generation, continuation: continuation)
        XCTAssertEqual(store.record(for: key)?.phase, .historyIncomplete)
        store.markRunning(key)
        XCTAssertEqual(store.record(for: key)?.phase, .historyIncomplete)
        XCTAssertNil(store.record(for: key)?.receipt)
        XCTAssertNil(store.historyContinuation(for: key, generation: UUID()))
        XCTAssertTrue(store.begin(key))
        XCTAssertEqual(store.record(for: key)?.generation, generation)
        XCTAssertEqual(store.historyContinuation(for: key, generation: generation)?.cursor, "bounded-progress")
        store.pauseHistory(key, generation: generation, continuation: continuation)
        store.restartHistory(key)
        XCTAssertNotEqual(store.record(for: key)?.generation, generation)
        XCTAssertFalse(store.hasHistoryContinuation(for: key))
        store.pauseHistory(key, generation: generation, continuation: continuation)
        XCTAssertFalse(store.hasHistoryContinuation(for: key), "A delayed read must not attach to a new generation")
    }

    private func historyEnvelope(coverage: String, cursor: String?, countScope: String) -> Data {
        var history: [String: Any] = [
            "scope": "all", "query_id": "query:exact", "snapshot": "sha256:stable",
            "returned": 0, "candidate_matches": 0, "count_scope": countScope,
            "candidate_matches_complete": coverage == "complete", "page_size": 2,
            "evidence_matches": 0, "evidence_matches_complete": coverage == "complete",
            "coverage": coverage, "has_more": false, "has_more_known": coverage == "complete",
            "total_complete": coverage == "complete", "scanned_bytes": 10, "scanned_records": 1,
        ]
        history["next_cursor"] = cursor
        if coverage == "complete" { history["total"] = 0 }
        return try! JSONSerialization.data(withJSONObject: [
            "cases": [], "summary": ["total": 0, "open": 0, "urgent": 0, "high": 0, "resolved": 0],
            "journal_integrity": ["invalid_count": 0, "issues": []], "history": history,
        ])
    }

    private func receiveHistory(_ read: SirsiEngine.JSONCommandRead, state: inout MaatHistoryRefreshState, scope: String) {
        guard let request = state.begin(scope: scope) else { return XCTFail("Fresh history request was refused") }
        state.complete(read, for: request, cancelled: false)
    }

    /// HW08 opt-in only: Main supplies unchanged stdout from canonical Go CLI
    /// `maat casebook --json --scope system-one --limit 50` over its owned
    /// older-screen fixture (evidence maat-system-one:older-screen, head 40*a).
    /// No default fixture read, engine override, or CLI launch occurs here.
    @MainActor
    private func assertOptionalGoProducedHistoryFixture() async throws {
        guard let path = ProcessInfo.processInfo.environment["SIRSI_TEST_MAAT_HISTORY_JSON_FIXTURE"] else { return }
        guard path.hasPrefix("/") else { return XCTFail("History wire fixture must have an absolute path") }
        let file = try FileHandle(forReadingFrom: URL(fileURLWithPath: path))
        defer { try? file.close() }
        let data = try XCTUnwrap(try file.read(upToCount: (1 << 20) + 1))
        guard !data.isEmpty, data.count <= (1 << 20) else { return XCTFail("History wire fixture must contain 1...1048576 bytes") }
        let recorder = HistoryReadRecorder()
        await recorder.enqueue(SirsiEngine.JSONCommandRead(data: data, exitStatus: 0, timedOut: false, launchError: nil))
        let store = MaatHistoryRefreshStore()
        await store.load(scope: "system-one") { await recorder.next(arguments: $0) }
        XCTAssertNil(store.state.error)
        XCTAssertFalse(store.state.loading)
        XCTAssertEqual(store.state.query, "")
        let calls = await recorder.arguments()
        XCTAssertEqual(calls, [["maat", "casebook", "--json", "--scope", "system-one", "--limit", "50"]])
        let projection = try XCTUnwrap(store.state.projection)
        XCTAssertEqual(projection.journalIntegrity.invalidCount, 0)
        XCTAssertEqual(projection.historyEmptyState, .records)
        XCTAssertEqual(projection.cases.count, 1)
        XCTAssertEqual(projection.summary.total, 1)
        let entry = try XCTUnwrap(projection.cases.first)
        XCTAssertTrue(entry.id.hasPrefix("maat-case-"))
        XCTAssertEqual(entry.evidence, "maat-system-one:older-screen")
        XCTAssertEqual(entry.kind, "system one screen")
        let verdict = try XCTUnwrap(entry.systemOne)
        XCTAssertEqual(verdict.subject.headSHA, String(repeating: "a", count: 40))
        let window = try XCTUnwrap(projection.history)
        XCTAssertEqual(window.scope, "system-one")
        XCTAssertTrue(window.queryID.hasPrefix("sha256:"))
        XCTAssertTrue(window.snapshot.hasPrefix("sha256:"))
        XCTAssertEqual(window.returned, 1)
        XCTAssertEqual(window.candidateMatches, 1)
        XCTAssertEqual(window.countScope, "query")
        XCTAssertTrue(window.candidateMatchesComplete)
        XCTAssertEqual(window.pageSize, 50)
        XCTAssertEqual(window.coverage, "complete")
        XCTAssertFalse(window.hasMore)
        XCTAssertTrue(window.hasMoreKnown)
        XCTAssertNil(window.nextCursor)
        XCTAssertTrue(window.totalComplete)
        XCTAssertEqual(window.total, 1)
    }

    private func selectedHistoryEnvelope() throws -> Data {
        var root = try XCTUnwrap(JSONSerialization.jsonObject(with: historyEnvelope(
            coverage: "complete", cursor: nil, countScope: "query")) as? [String: Any])
        var history = try XCTUnwrap(root["history"] as? [String: Any])
        for key in ["returned", "candidate_matches", "evidence_matches", "total"] { history[key] = 1 }
        root["history"] = history
        root["summary"] = ["total": 1, "open": 1, "urgent": 0, "high": 0, "resolved": 0]
        let action: [String: Any] = ["kind": "maat_repair", "action_id": "repair-1", "title": "Repair",
            "detail": "Bounded recovery", "evidence": "action-evidence-1", "requires_confirmation": true]
        let selected: [String: Any] = [
            "id": "case-1", "status": "open", "kind": "diagnostic", "resource": "launch-agent",
            "why": "retained reason", "determination": "needs repair", "evidence": "evidence-1",
            "next_action": action,
        ]
        root["cases"] = [selected]
        return try JSONSerialization.data(withJSONObject: root)
    }

    private func browseRead(scope: String, queryID: String, snapshot: String, cursor: String? = nil) throws -> SirsiEngine.JSONCommandRead {
        var root = try XCTUnwrap(JSONSerialization.jsonObject(with: selectedHistoryEnvelope()) as? [String: Any])
        var history = try XCTUnwrap(root["history"] as? [String: Any])
        history["scope"] = scope
        history["query_id"] = queryID
        history["snapshot"] = snapshot
        if scope == "system-one" {
            root["cases"] = []
            root["summary"] = ["total": 0, "open": 0, "urgent": 0, "high": 0, "resolved": 0]
            for field in ["returned", "candidate_matches", "evidence_matches", "total"] { history[field] = 0 }
        }
        if let cursor {
            history["next_cursor"] = cursor
            history["has_more"] = true
            history["candidate_matches"] = 2
            history["total"] = 2
        }
        root["history"] = history
        return SirsiEngine.JSONCommandRead(data: try JSONSerialization.data(withJSONObject: root),
            exitStatus: 0, timedOut: false, launchError: nil)
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
        XCTAssertRejected(evaluate(original: original, latest: original, duplicateEvidence: true))
        XCTAssertRejected(evaluate(original: original, latest: original, rawEvidenceCount: 2))
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
        XCTAssertFalse(store.begin(key), "The same confirmed repair remains non-replayable while its updater is running")
        XCTAssertEqual(store.record(for: key)?.phase, .running)
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
        duplicateCaseID: Bool = false,
        duplicateEvidence: Bool = false,
        rawEvidenceCount: Int? = nil
    ) -> MaatCasePreflightDecision {
        var cases = latest.map { [$0] } ?? []
        if duplicateCaseID, let latest { cases.append(latest) }
        if duplicateEvidence, let latest {
            cases.append(caseEntry(id: latest.id + "-duplicate", evidence: latest.evidence))
        }
        let readProjection = projection
            ? MaatCasebookProjection(
                cases: cases,
                summary: MaatCasebookSummary(total: cases.count, open: cases.count, urgent: 0, high: 0, resolved: 0),
                journalIntegrity: MaatJournalIntegrity(invalidCount: invalidCount, issues: []),
                history: MaatHistoryWindow(scope: "all", queryID: "query:test", snapshot: "sha256:test", returned: cases.count,
                    candidateMatches: cases.count, countScope: "query", evidenceMatches: rawEvidenceCount ?? cases.count, evidenceMatchesComplete: true, candidateMatchesComplete: true,
                    pageSize: 100, coverage: "complete", hasMore: false, hasMoreKnown: true,
                    nextCursor: nil, totalComplete: true, total: cases.count,
                    scannedRecords: nil, scannedBytes: nil)
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
        id: String = "case-1",
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
            id: id,
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

private actor HistoryReadRecorder {
    private var pending: [SirsiEngine.JSONCommandRead] = []
    private var recordedArguments: [[String]] = []

    func enqueue(_ read: SirsiEngine.JSONCommandRead) { pending.append(read) }
    func next(arguments: [String]) -> SirsiEngine.JSONCommandRead {
        recordedArguments.append(arguments)
        return pending.removeFirst()
    }
    func callCount() -> Int { recordedArguments.count }
    func arguments() -> [[String]] { recordedArguments }
}

private actor ControlledHistoryResponses {
    private var recorded: [[String]] = []
    private var pending: [Int: CheckedContinuation<SirsiEngine.JSONCommandRead, Never>] = [:]
    private var waiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []

    func read(_ arguments: [String]) async -> SirsiEngine.JSONCommandRead {
        let index = recorded.count
        recorded.append(arguments)
        return await withCheckedContinuation { continuation in
            pending[index] = continuation
            let ready = waiters.filter { $0.count <= recorded.count }
            waiters.removeAll { $0.count <= recorded.count }
            for waiter in ready { waiter.continuation.resume() }
        }
    }

    func waitForCount(_ count: Int) async {
        if recorded.count >= count { return }
        await withCheckedContinuation { waiters.append((count, $0)) }
    }

    func finish(_ index: Int, with read: SirsiEngine.JSONCommandRead) {
        guard let continuation = pending.removeValue(forKey: index) else {
            preconditionFailure("Missing controlled history request")
        }
        continuation.resume(returning: read)
    }

    func arguments() -> [[String]] { recorded }
}

private actor ControlledHistoryDeadline {
    private var continuations: [CheckedContinuation<Void, Never>] = []
    private var countWaiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []
    private var handledCount = 0
    private var handledWaiters: [(count: Int, continuation: CheckedContinuation<Void, Never>)] = []

    func sleep() async {
        await withCheckedContinuation { continuation in
            continuations.append(continuation)
            resumeReady(&countWaiters, observed: continuations.count)
        }
    }

    func waitForCount(_ count: Int) async {
        if continuations.count >= count { return }
        await withCheckedContinuation { countWaiters.append((count, $0)) }
    }

    func markHandled() {
        handledCount += 1
        resumeReady(&handledWaiters, observed: handledCount)
    }

    func waitForHandled(_ count: Int) async {
        if handledCount >= count { return }
        await withCheckedContinuation { handledWaiters.append((count, $0)) }
    }

    func releaseAll() {
        let waiting = continuations
        continuations.removeAll()
        waiting.forEach { $0.resume() }
    }

    private func resumeReady(
        _ waiters: inout [(count: Int, continuation: CheckedContinuation<Void, Never>)],
        observed: Int
    ) {
        let ready = waiters.filter { $0.count <= observed }
        waiters.removeAll { $0.count <= observed }
        ready.forEach { $0.continuation.resume() }
    }
}

private final class TestHistoryMonotonicClock: @unchecked Sendable {
    private let lock = NSLock()
    private var values: [TimeInterval]

    init(_ values: [TimeInterval]) { self.values = values }

    func now() -> TimeInterval {
        lock.lock()
        defer { lock.unlock() }
        if values.count > 1 { return values.removeFirst() }
        return values.first ?? 0
    }
}
