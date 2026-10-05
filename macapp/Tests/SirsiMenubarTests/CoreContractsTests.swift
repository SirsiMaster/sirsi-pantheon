import XCTest
import SwiftUI
@testable import SirsiMenubar

final class CoreContractsTests: XCTestCase {
    func testEveryDiagnosticHasAClosedNativeResolutionRoute() {
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 0, hasFix: false, hasRecommendedCommand: false),
            .accepted
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: "information", severity: 1, hasFix: false, hasRecommendedCommand: false),
            .accepted
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 2, hasFix: false, hasRecommendedCommand: false),
            .maatReview
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: "maat_review", severity: 3, hasFix: false, hasRecommendedCommand: false),
            .maatReview
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 3, hasFix: true, hasRecommendedCommand: false),
            .repair
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 1, hasFix: false, hasRecommendedCommand: true),
            .command
        )
        XCTAssertEqual(
            diagnosticResolutionRoute(resolution: nil, severity: 2, hasFix: true,
                                      hasRecommendedCommand: false, fixKind: "guidance"),
            .command
        )
    }

    func testResolutionOutcomesRequireCurrentEvidence() {
        XCTAssertEqual(resolutionOutcome(observation: .current, severity: nil), .verified)
        XCTAssertEqual(resolutionOutcome(observation: .current, severity: 1), .verified)
        XCTAssertEqual(resolutionOutcome(observation: .current, severity: 3), .partial)
        XCTAssertEqual(resolutionOutcome(observation: .stale, severity: nil), .unverified)
        XCTAssertEqual(resolutionOutcome(observation: .unknown, severity: 3), .unverified)
        XCTAssertEqual(resolutionOutcome(observation: .current, severity: nil, actionSucceeded: false), .failed)
        XCTAssertEqual(resolutionOutcome(observation: .current, severity: 3, cancelled: true), .cancelled)
    }

    func testDiagnosisDecodeFailurePreservesUnknownOrStaleState() {
        XCTAssertEqual(healthObservationAfterRead(status: nil, hasPreviousObservation: false), .unknown)
        XCTAssertEqual(healthObservationAfterRead(status: nil, hasPreviousObservation: true), .stale)
        XCTAssertEqual(healthObservationAfterRead(status: "invalid", hasPreviousObservation: false), .unknown)
        XCTAssertEqual(healthObservationAfterRead(status: "invalid", hasPreviousObservation: true), .stale)
        XCTAssertEqual(healthObservationAfterRead(status: "green", hasPreviousObservation: true), .current)
    }

    func testHealthResolutionRouteSurvivesAnEmptyFailedInitialRead() {
        let state = healthObservationAfterRead(status: nil, hasPreviousObservation: false)
        XCTAssertEqual(state, .unknown)
        XCTAssertTrue(shouldOfferHealthResolutionEntry(observation: state, isLoading: false))
        XCTAssertFalse(shouldOfferHealthResolutionEntry(observation: state, isLoading: true))
        XCTAssertTrue(shouldOfferHealthResolutionEntry(observation: .stale, isLoading: true))
        XCTAssertFalse(shouldOfferHealthResolutionEntry(observation: .current, isLoading: true))
        XCTAssertTrue(shouldOfferHealthResolutionEntry(observation: .current, isLoading: false))
    }

    func testGreenHealthIsReadyOnlyForCurrentObservation() {
        XCTAssertTrue(isConfirmedHealthy(status: "green", observation: .current))
        XCTAssertFalse(isConfirmedHealthy(status: "green", observation: .stale))
        XCTAssertFalse(isConfirmedHealthy(status: "green", observation: .unknown))
        XCTAssertFalse(isConfirmedHealthy(status: "amber", observation: .current))
        XCTAssertFalse(isConfirmedHealthy(status: "red", observation: .current))
    }

    func testFindingUsesLatestCurrentActionAndDisablesHistoricalActions() throws {
        let old = try decodeFinding(#"{"check":"Disk Pressure","severity":3,"message":"Old","fix":"sirsi old-fix","fixKind":"instant"}"#)
        let latest = try decodeFinding(#"{"check":"Disk Pressure","severity":2,"message":"Latest","fix":"sirsi new-fix","fixKind":"relief"}"#)
        let current = currentDiagnosticFinding(check: old.check, findings: [latest], observation: .current)

        XCTAssertEqual(current?.message, "Latest")
        XCTAssertEqual(current?.fix, "sirsi new-fix")
        XCTAssertTrue(canUseDiagnosticAction(observation: .current, findingIsCurrent: current != nil,
                                             hasAction: current?.fix != nil))
        XCTAssertNil(currentDiagnosticFinding(check: old.check, findings: [], observation: .current))
        XCTAssertFalse(canUseDiagnosticAction(observation: .current, findingIsCurrent: false, hasAction: true))
        XCTAssertNil(currentDiagnosticFinding(check: old.check, findings: [latest], observation: .stale))
        XCTAssertFalse(canUseDiagnosticAction(observation: .unknown, findingIsCurrent: true, hasAction: true))
    }

    func testFailedRefreshKeepsLastKnownFindingButDisablesItsAction() throws {
        let original = try decodeFinding(#"{"check":"Disk Pressure","severity":1,"message":"Original A"}"#)
        let latestKnown = try decodeFinding(#"{"check":"Disk Pressure","severity":3,"message":"Latest B","fix":"sirsi clean --confirm"}"#)
        let retained = latestKnownDiagnosticFinding(check: original.check, findings: [latestKnown])

        XCTAssertEqual(retained?.message, "Latest B")
        XCTAssertNil(currentDiagnosticFinding(check: original.check, findings: [latestKnown], observation: .stale))
        XCTAssertFalse(canUseDiagnosticAction(observation: .stale, findingIsCurrent: false,
                                              hasAction: retained?.fix != nil))
    }

    func testMaatPayloadCapturesLatestDisplayedFindingEvidence() throws {
        let original = try decodeFinding(#"{"check":"Disk Pressure","severity":1,"message":"Original A","detail":"old detail"}"#)
        let latest = try decodeFinding(#"{"check":"Disk Pressure","severity":3,"message":"Latest B","detail":"current detail","fix":"sirsi clean --confirm"}"#)

        XCTAssertNotEqual(maatReviewArguments(for: original), maatReviewArguments(for: latest))
        XCTAssertEqual(maatReviewArguments(for: latest), [
            "maat", "record-resolution", "--check", "Disk Pressure", "--message", "Latest B",
            "--detail", "current detail", "--confirm",
        ])
    }

    func testFindingMaAtCasebookRouteKeepsGuidedLabelAndFiltersCurrentFinding() throws {
        let displayed = try decodeFinding(#"{"check":"Current finding B","severity":3,"message":"Latest"}"#)
        let route = FindingCasebookRoute(finding: displayed)

        XCTAssertEqual(route.label, "Continue in Ma'at Casebook")
        XCTAssertEqual(route.contextCheck, "Current finding B")
    }

    func testRaFabricHandoffHeaderUsesSnapshotSafeActionButton() throws {
        let testFile = URL(fileURLWithPath: #filePath)
        let viewsURL = testFile.deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources/SirsiMenubar/Views.swift")
        let source = try String(contentsOf: viewsURL, encoding: .utf8)
        let viewStart = try XCTUnwrap(source.range(of: "struct RaFabricView: View {"))
        let headerStart = try XCTUnwrap(source.range(of: "HStack(spacing: 8) {",
                                                     range: viewStart.upperBound..<source.endIndex))
        let headerEnd = try XCTUnwrap(source.range(of: "Text(\"Ra keeps Horus instances",
                                                   range: headerStart.upperBound..<source.endIndex))
        let header = String(source[headerStart.lowerBound..<headerEnd.lowerBound])

        XCTAssertTrue(header.contains("SnapshotActionButton(disabled: engine.busy)"))
        XCTAssertTrue(header.contains("showingHandoff = true"))
        XCTAssertTrue(header.contains("Label(\"Hand off work\""))
        XCTAssertTrue(header.contains("Label(\"Live work\""))
        XCTAssertFalse(header.contains("Button {"))
        XCTAssertFalse(header.contains(".buttonStyle(.borderedProminent)"))
    }

    func testOutboxRecheckOutcomeRequiresFreshReadAndPreservesUnknownDepth() {
        let failed = outboxRecheckOutcome(readSucceeded: false,
                                          scopedAgents: ["claude-pantheon"],
                                          currentOutboxes: [])
        XCTAssertTrue(failed.message.contains("could not confirm a fresh live fabric read"))
        XCTAssertTrue(failed.message.contains("queue depth remains unknown"))
        XCTAssertTrue(failed.requiresRecovery)

        let unreadable = RBOutbox(agent: "claude-pantheon", observed: true, readable: false,
                                  queuedForRetry: 0, unreadable: true, error: "permission denied")
        let stillBlocked = outboxRecheckOutcome(readSucceeded: true,
                                                scopedAgents: ["claude-pantheon", "codex-pantheon"],
                                                currentOutboxes: [unreadable])
        XCTAssertTrue(stillBlocked.message.contains("still reports unreadable outboxes for claude-pantheon"))
        XCTAssertTrue(stillBlocked.message.contains("Queue depth remains unknown"))
        XCTAssertTrue(stillBlocked.requiresRecovery)

        let readable = RBOutbox(agent: "codex-pantheon", observed: true, readable: true,
                                 queuedForRetry: 0, unreadable: false, error: nil)
        let cleared = outboxRecheckOutcome(readSucceeded: true,
                                           scopedAgents: ["codex-pantheon"],
                                           currentOutboxes: [readable])
        XCTAssertTrue(cleared.message.contains("explicit readable records for each affected agent"))
        XCTAssertTrue(cleared.message.contains("Queue depth and delivery remain unverified"))
        XCTAssertFalse(cleared.requiresRecovery)
    }

    func testMissingOutboxTelemetryOrAffectedAgentRecordCannotConfirmClearance() {
        let scope: Set<String> = ["claude-pantheon", "codex-pantheon"]
        let missingTelemetry = outboxRecheckOutcome(readSucceeded: true, scopedAgents: scope, currentOutboxes: nil)
        XCTAssertTrue(missingTelemetry.message.contains("did not include outbox telemetry"))
        XCTAssertTrue(missingTelemetry.message.localizedCaseInsensitiveContains("blocker clearance is unverified"))
        XCTAssertTrue(missingTelemetry.requiresRecovery)

        let onlyOneReadable = RBOutbox(agent: "codex-pantheon", observed: true, readable: true,
                                       queuedForRetry: 1, unreadable: false, error: nil)
        let missingEntry = outboxRecheckOutcome(readSucceeded: true, scopedAgents: scope,
                                                currentOutboxes: [onlyOneReadable])
        XCTAssertTrue(missingEntry.message.contains("No explicit readable outbox record exists for claude-pantheon"))
        XCTAssertTrue(missingEntry.message.contains("blocker clearance is unverified"))
        XCTAssertTrue(missingEntry.requiresRecovery)

        let legacyUnknown = RBOutbox(agent: "claude-pantheon", observed: nil, readable: nil,
                                     queuedForRetry: 0, unreadable: nil, error: nil)
        let unknown = outboxRecheckOutcome(readSucceeded: true, scopedAgents: ["claude-pantheon"],
                                           currentOutboxes: [legacyUnknown])
        XCTAssertTrue(unknown.message.contains("blocker clearance is unverified"))
        XCTAssertTrue(unknown.requiresRecovery)
    }

    func testUnverifiedRefreshRetainsPreRefreshOutboxScope() {
        let captured = [RBOutbox(agent: "codex-pantheon", observed: true, readable: false,
                                 queuedForRetry: 0, unreadable: true, error: "permission denied")]
        let unresolved = OutboxRecheckOutcome(message: "still unknown", requiresRecovery: true)
        XCTAssertEqual(retainedOutboxScope(outcome: unresolved, capturedScope: captured).map(\.agent),
                       ["codex-pantheon"])

        let cleared = OutboxRecheckOutcome(message: "readable", requiresRecovery: false)
        XCTAssertTrue(retainedOutboxScope(outcome: cleared, capturedScope: captured).isEmpty)
    }

    @MainActor
    func testUnresolvedOutboxScopeSurvivesNavigationThroughSharedEngineStatus() {
        let engine = SirsiEngine()
        engine.routerBoard = try! JSONDecoder().decode(RouterBoard.self, from: Data("{}".utf8))
        let captured = [RBOutbox(agent: "codex-pantheon", observed: true, readable: false,
                                 queuedForRetry: 0, unreadable: true, error: "permission denied")]
        engine.recordOutboxRecheck(requiresRecovery: true, capturedScope: captured)
        XCTAssertEqual(engine.routerVisibleOutboxBlockers.map(\.agent), ["codex-pantheon"])
        XCTAssertTrue(engine.routerHasBlockers)
        XCTAssertEqual(engine.routerStatus, "red")
        XCTAssertTrue(engine.routerSummary.contains("1 blocker"))

        engine.recordOutboxRecheck(requiresRecovery: false, capturedScope: captured)
        XCTAssertTrue(engine.routerVisibleOutboxBlockers.isEmpty)
        XCTAssertFalse(engine.routerHasBlockers)
    }

    func testRouterBoardRecheckCanBypassFreshCache() throws {
        XCTAssertTrue(SirsiEngine.routerBoardCacheIsUsable(forceLive: false, isFresh: true, hasDecodedBoard: true))
        XCTAssertFalse(SirsiEngine.routerBoardCacheIsUsable(forceLive: true, isFresh: true, hasDecodedBoard: true))
        XCTAssertFalse(SirsiEngine.routerBoardCacheIsUsable(forceLive: false, isFresh: false, hasDecodedBoard: true))
        XCTAssertFalse(SirsiEngine.routerBoardCacheIsUsable(forceLive: false, isFresh: true, hasDecodedBoard: false))

        let testFile = URL(fileURLWithPath: #filePath)
        let engineURL = testFile.deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources/SirsiMenubar/SirsiEngine.swift")
        let source = try String(contentsOf: engineURL, encoding: .utf8)
        XCTAssertTrue(source.contains("func loadRouterBoard(forceLive: Bool = false) async -> Bool"))
        XCTAssertTrue(source.contains("Self.routerBoardCacheIsUsable(forceLive: forceLive"))
    }

    func testUnreadableOutboxCardHasThreeSafeResolutionRoutes() throws {
        let testFile = URL(fileURLWithPath: #filePath)
        let viewsURL = testFile.deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources/SirsiMenubar/Views.swift")
        let source = try String(contentsOf: viewsURL, encoding: .utf8)
        let cardStart = try XCTUnwrap(source.range(of: "struct OutboxBlockerCard: View {"))
        let cardEnd = try XCTUnwrap(source.range(of: "struct OutboxRetryCard: View {", range: cardStart.upperBound..<source.endIndex))
        let card = String(source[cardStart.lowerBound..<cardEnd.lowerBound])

        XCTAssertTrue(card.contains("Queue depth is unknown, not zero."))
        XCTAssertFalse(card.contains("check the directory by hand"))
        XCTAssertTrue(card.contains("await engine.loadRouterBoard(forceLive: true)"))
        XCTAssertTrue(card.contains("onHandoff"))
        XCTAssertTrue(card.contains("resolutionLevel(1, \"Recheck the fabric\""))
        XCTAssertTrue(card.contains("resolutionLevel(2, \"Guided host recovery\""))
        XCTAssertTrue(card.contains("resolutionLevel(3, \"Ma’at Casebook review\""))
        XCTAssertTrue(card.contains("MaatCasebookView(engine: engine)"))
        XCTAssertFalse(card.contains("contextCheck"))
        XCTAssertTrue(card.contains("filesystem permissions or ownership"))
        XCTAssertTrue(source.contains("onHandoff: { showingHandoff = true }"))
        XCTAssertTrue(source.contains("RaHandoffView(engine: engine, isPresented: $showingHandoff)"))
        XCTAssertTrue(source.contains("engine.routerVisibleOutboxBlockers"))
        XCTAssertTrue(source.contains("engine.routerHasBlockers || !visibleOutboxBlockers.isEmpty"))
        XCTAssertTrue(card.contains("let capturedScope = blocked"))
        XCTAssertTrue(card.contains("onRecheckResult(outcome, capturedScope)"))
        XCTAssertTrue(source.contains("engine.recordOutboxRecheck(requiresRecovery: outcome.requiresRecovery"))
    }

    func testCasebookRouteDoesNotSeedItsLiteralSearchFilter() throws {
        let testFile = URL(fileURLWithPath: #filePath)
        let casebookURL = testFile.deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appendingPathComponent("Sources/SirsiMenubar/MaatCasebookView.swift")
        let source = try String(contentsOf: casebookURL, encoding: .utf8)
        XCTAssertTrue(source.contains("_query = State(initialValue: contextCheck ?? \"\")"))
        XCTAssertTrue(source.contains("$0.searchText.localizedCaseInsensitiveContains(term)"))
    }

    func testVerifiedOutcomeIsHiddenAfterFindingOrObservationChanges() throws {
        let verified = try decodeFinding(#"{"check":"Disk Pressure","severity":0,"message":"Clear"}"#)
        let changed = try decodeFinding(#"{"check":"Disk Pressure","severity":3,"message":"Critical again","fix":"sirsi clean --confirm"}"#)
        let verifiedSnapshot = findingObservationSnapshot(observation: .current, finding: verified,
                                                         healthStatus: "green")
        XCTAssertTrue(shouldShowResolutionOutcome(.verified, outcomeSnapshot: verifiedSnapshot,
                                                  currentSnapshot: verifiedSnapshot))
        XCTAssertFalse(shouldShowResolutionOutcome(.verified, outcomeSnapshot: verifiedSnapshot,
                                                   currentSnapshot: findingObservationSnapshot(observation: .current,
                                                                                             finding: changed)))
        XCTAssertFalse(shouldShowResolutionOutcome(.verified, outcomeSnapshot: verifiedSnapshot,
                                                   currentSnapshot: findingObservationSnapshot(observation: .current,
                                                                                             finding: verified,
                                                                                             healthStatus: "red")))
        XCTAssertFalse(shouldShowResolutionOutcome(.verified, outcomeSnapshot: verifiedSnapshot,
                                                   currentSnapshot: findingObservationSnapshot(observation: .stale,
                                                                                             finding: changed)))
        XCTAssertFalse(shouldShowResolutionOutcome(.verified, outcomeSnapshot: verifiedSnapshot,
                                                   currentSnapshot: findingObservationSnapshot(observation: .current,
                                                                                             finding: nil)))
    }

    func testPublishedRecheckOutcomeSurvivesItsMatchingSnapshotCallback() throws {
        let issue = try decodeFinding(#"{"check":"Disk Pressure","severity":3,"message":"Needs attention"}"#)
        let cleared = findingObservationSnapshot(observation: .current, finding: nil, healthStatus: "green")
        var outcome: ResolutionOutcome? = .verified
        var outcomeSnapshot: FindingObservationSnapshot? = cleared

        if shouldInvalidateResolutionOutcome(outcomeSnapshot: outcomeSnapshot, newSnapshot: cleared) {
            outcome = nil
            outcomeSnapshot = nil
        }
        XCTAssertEqual(outcome, .verified)
        XCTAssertTrue(shouldShowResolutionOutcome(outcome, outcomeSnapshot: outcomeSnapshot,
                                                  currentSnapshot: cleared))

        let laterCritical = findingObservationSnapshot(observation: .current, finding: issue,
                                                      healthStatus: "red")
        if shouldInvalidateResolutionOutcome(outcomeSnapshot: outcomeSnapshot, newSnapshot: laterCritical) {
            outcome = nil
            outcomeSnapshot = nil
        }
        XCTAssertNil(outcome)
        XCTAssertFalse(shouldShowResolutionOutcome(outcome, outcomeSnapshot: outcomeSnapshot,
                                                   currentSnapshot: laterCritical))
    }

    func testSpotlightGuidanceUsesNativePrivacyRecoveryRoute() {
        XCTAssertEqual(diagnosticRecoveryGuidance(check: "Spotlight Storm"), .spotlightPrivacy)
        XCTAssertEqual(diagnosticRecoveryGuidance(check: "RAM Pressure"), .command)
    }

    func testCommandExecutionPreservesStreamsAndDoesNotInferSuccessFromEmptyOutput() {
        let captured = CommandExecution(stdout: "partial output", stderr: "permission denied", exitCode: 2,
                                       launchError: nil, timedOut: false)
        XCTAssertTrue(captured.displayText.contains("STDOUT:\npartial output"))
        XCTAssertTrue(captured.displayText.contains("STDERR:\npermission denied"))
        XCTAssertTrue(captured.displayText.contains("Exit status: 2"))
        XCTAssertEqual(captured.outcome, .failed)
        let stderrOnly = CommandExecution(stdout: "", stderr: "failed to run", exitCode: 1,
                                          launchError: nil, timedOut: false)
        XCTAssertTrue(stderrOnly.displayText.contains("STDERR:\nfailed to run"))
        XCTAssertEqual(stderrOnly.outcome, .failed)
        let launchFailure = CommandExecution(stdout: "", stderr: "", exitCode: nil,
                                             launchError: "permission denied", timedOut: false)
        XCTAssertTrue(launchFailure.displayText.contains("Launch failure: permission denied"))
        XCTAssertEqual(launchFailure.outcome, .launchFailed)
        let empty = CommandExecution(stdout: "", stderr: "", exitCode: 0,
                                     launchError: nil, timedOut: false)
        XCTAssertTrue(empty.displayText.contains("STDOUT:\n(empty)"))
        XCTAssertEqual(empty.outcome, .unverified)
        XCTAssertEqual(commandExecutionOutcome(exitCode: nil, launchError: nil, timedOut: false,
                                               hasOutput: false), .unverified)
        XCTAssertEqual(commandExecutionOutcome(exitCode: 0, launchError: "not executable",
                                               timedOut: false, hasOutput: true), .launchFailed)
        XCTAssertEqual(commandExecutionOutcome(exitCode: 143, launchError: nil,
                                               timedOut: true, hasOutput: true), .timedOut)
        XCTAssertEqual(commandExecutionOutcome(exitCode: 1, launchError: nil,
                                               timedOut: false, hasOutput: true), .failed)
        let silentNonzero = CommandExecution(stdout: "", stderr: "", exitCode: 1,
                                             launchError: nil, timedOut: false)
        XCTAssertEqual(silentNonzero.outcome, .failed)
        XCTAssertTrue(silentNonzero.displayText.contains("Exit status: 1"))
        XCTAssertEqual(commandExecutionOutcome(exitCode: 0, launchError: nil,
                                               timedOut: false, hasOutput: true), .succeeded)
    }

    private func decodeFinding(_ json: String) throws -> DiagFinding {
        try JSONDecoder().decode(DiagFinding.self, from: Data(json.utf8))
    }

    func testNavigationStackSurvivesWindowReopen() {
        let navigation = Nav()
        navigation.push(Text("Evidence-backed resolution"))

        navigation.windowDidReopen()

        XCTAssertEqual(navigation.stack.count, 1)
        XCTAssertEqual(navigation.reopenCount, 1)
        XCTAssertFalse(navigation.atRoot)
    }

    func testDiagnosticFindingDecodesExplicitMaatReviewRoute() throws {
        let raw = #"""
        {
          "check": "Kernel Panics (7d)",
          "severity": 3,
          "message": "Two recent kernel panics need review",
          "resolution": "maat_review"
        }
        """#.data(using: .utf8)!

        let finding = try JSONDecoder().decode(DiagFinding.self, from: raw)

        XCTAssertEqual(finding.check, "Kernel Panics (7d)")
        XCTAssertEqual(finding.resolution, "maat_review")
        XCTAssertNil(finding.fix)
    }

    func testApolloSnapshotCatalogMakesAllDetectedEstatesVisible() {
        let catalog = ApolloCatalog.snapshotPreview

        XCTAssertEqual(catalog.machine.id, "this-mac")
        XCTAssertGreaterThan(catalog.machine.cpuCores, 0)
        XCTAssertGreaterThan(catalog.machine.memoryBytes, 0)
        XCTAssertEqual(Set(catalog.machine.chipEstates ?? []), Set(catalog.estates.map(\.id)))
        XCTAssertTrue(catalog.estates.contains { $0.id == "neural-engine" && !$0.available })
    }

    func testSnapshotCoverageIncludesStackLabPrimaryWorkflow() throws {
        let source = try String(contentsOfFile: "Sources/SirsiMenubar/Snapshot.swift", encoding: .utf8)
        XCTAssertTrue(source.contains("(\"stack-lab\", AnyView(StackLabView"),
                      "The visual gate must render Stack Lab itself, not only its Apollo drill-in.")
        XCTAssertTrue(source.contains("StackLabView.fetchReport()"),
                      "Stack Lab must receive the same typed report in the visual gate rather than an inert loading shell.")
    }

    func testApolloCatalogKeepsResidentModelChoicesBoundToTheirEngineAndMachine() throws {
        let raw = #"""
        {
          "machine": {"id":"m1","name":"M1","cpu_cores":8,"memory_bytes":17179869184},
          "machines": [
            {"id":"m1","name":"M1","cpu_cores":8,"memory_bytes":17179869184,"estates":[{"id":"cpu","name":"M1 CPU","available":true,"description":"8 cores"}]},
            {"id":"m2","name":"M2","cpu_cores":12,"memory_bytes":34359738368,"estates":[{"id":"gpu","name":"M2 GPU","available":true,"description":"12 cores"}]}
          ],
          "engines": [
            {"id":"apollo-m1","machine_id":"m1","name":"Apollo MLX","provider":"SNE","resident_model":"Apollo Plain","state":"configured"},
            {"id":"apollo-m2","machine_id":"m2","name":"Apollo Flash","provider":"SNE","resident_model":"Apollo Flash","state":"configured"}
          ],
          "chip_estates": [{"id":"cpu","name":"Local CPU","available":false,"description":"local only"}]
        }
        """#.data(using: .utf8)!

        let catalog = try JSONDecoder().decode(ApolloCatalog.self, from: raw)

        XCTAssertEqual(catalog.residentModelOptions(for: "m1").map(\.id), ["apollo-m1"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m1").map(\.residentModel), ["Apollo Plain"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m2").map(\.id), ["apollo-m2"])
        XCTAssertEqual(catalog.residentModelOptions(for: "m2").map(\.residentModel), ["Apollo Flash"])
        XCTAssertEqual(catalog.route(machineID: "m1", engineID: "apollo-m1")?.residentModel, "Apollo Plain")
        XCTAssertNil(catalog.route(machineID: "m1", engineID: "apollo-m2"))
        XCTAssertEqual(catalog.estateOptions(for: "m1").map(\.name), ["M1 CPU"])
        XCTAssertEqual(catalog.estateOptions(for: "m2").map(\.name), ["M2 GPU"])
    }

    func testApolloPlanPreservesRequestedUnqualifiedEstate() throws {
        let raw = #"""
        {
          "machine_id": "this-mac",
          "engine_id": "apollo-local",
          "resident_model": "Apollo Plain",
          "cpu_cores": 4,
          "memory_bytes": 8589934592,
          "swap_bytes": 0,
          "chip_estates": ["cpu", "neural-engine"],
          "unavailable_chip_estates": ["neural-engine"]
        }
        """#.data(using: .utf8)!

        let plan = try JSONDecoder().decode(ApolloPlan.self, from: raw)

        XCTAssertEqual(plan.chipEstates, ["cpu", "neural-engine"])
        XCTAssertEqual(plan.unqualifiedEstates, ["neural-engine"])
        XCTAssertEqual(plan.residentModel, "Apollo Plain")
    }

    func testApolloTelemetryDecodesSelectedAndAdditionalReportedEstates() throws {
        let raw = #"""
        {
          "state": "active",
          "telemetry": {
            "engine_id": "apollo-local",
            "tokens_per_second": 31.5,
            "bandwidth_bytes_per_second": 1048576,
            "memory_bytes": 8589934592,
            "network_saturation_percent": 12.5,
            "cpu_residency_percent": 43.0,
            "gpu_residency_percent": 66.0,
            "chip_estates": [
              {"id": "gpu", "residency_percent": 66.0, "utilization_percent": 51.0, "memory_bytes": 4294967296},
              {"id": "neural-engine", "residency_percent": 8.0, "utilization_percent": 5.0, "memory_bytes": 0}
            ]
          }
        }
        """#.data(using: .utf8)!

        let read = try JSONDecoder().decode(ApolloTelemetryRead.self, from: raw)

        XCTAssertEqual(read.state, "active")
        XCTAssertEqual(read.telemetry?.tokensPerSec, 31.5)
        XCTAssertEqual(read.telemetry?.estates.map(\.id), ["gpu", "neural-engine"])
    }

    func testApolloTelemetryRequiresMachineMatchForASelectedPeer() throws {
        let plan = try JSONDecoder().decode(ApolloPlan.self, from: #"""
        {"machine_id":"horus-m1","engine_id":"apollo-m1","cpu_cores":4,"memory_bytes":8589934592,"swap_bytes":0,"chip_estates":["cpu"]}
        """#.data(using: .utf8)!)
        let matching = try JSONDecoder().decode(ApolloSessionTelemetry.self, from: #"""
        {"engine_id":"apollo-m1","machine_id":"horus-m1","chip_estates":[]}
        """#.data(using: .utf8)!)
        let ambiguousLegacy = try JSONDecoder().decode(ApolloSessionTelemetry.self, from: #"""
        {"engine_id":"apollo-m1","chip_estates":[]}
        """#.data(using: .utf8)!)

        XCTAssertTrue(matching.matches(plan: plan))
        XCTAssertFalse(ambiguousLegacy.matches(plan: plan))
    }

    func testMaatCredentialPreflightDecodesObservedNonDeveloperIdentityTypes() throws {
        let raw = #"""
        {
          "team_id": "9D382WV988",
          "fingerprint": "sha256=fixture",
          "developer_identities": [],
          "observed_non_developer_identity_types": ["Apple Distribution"],
          "notarization_observed": false,
          "verdict": {
            "schema_version": "maat-system-one/v1",
            "feather_weight": 0,
            "gate": "block",
            "confidence": 1,
            "subject": {"kind": "host", "ref": "release-credentials", "head_sha": "fixture"},
            "floor": {"passed": false, "checks": []},
            "model": {"provider": "local:test", "version": "v1", "local": true, "latency_ms": 0},
            "findings": []
          }
        }
        """#.data(using: .utf8)!

        let preflight = try JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: raw)

        XCTAssertEqual(preflight.observedNonDeveloperIdentityTypes, ["Apple Distribution"])
    }

    func testCredentialPreflightAlwaysProvidesAProtectedRecoveryPlan() throws {
        let raw = #"""
        {
          "team_id": "9D382WV988",
          "fingerprint": "sha256=fixture",
          "developer_identities": [],
          "notarization_observed": false,
          "verdict": {
            "schema_version": "maat-system-one/v1",
            "feather_weight": 0,
            "gate": "block",
            "confidence": 1,
            "subject": {"kind": "host", "ref": "release-credentials", "head_sha": "fixture"},
            "floor": {"passed": false, "checks": []},
            "model": {"provider": "local:test", "version": "v1", "local": true, "latency_ms": 0},
            "findings": []
          }
        }
        """#.data(using: .utf8)!

        let preflight = try JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: raw)
        let steps = protectedReleaseRecoverySteps(for: preflight)

        XCTAssertTrue(steps.contains { $0.contains("Developer ID Application") && $0.contains("9D382WV988") })
        XCTAssertTrue(steps.contains { $0.contains("Developer ID Installer") && $0.contains("9D382WV988") })
        XCTAssertTrue(steps.contains { $0.contains("notarization credential") })
        XCTAssertTrue(steps.last?.contains("Recheck readiness") == true)
    }

    func testCredentialPreflightPrefersCanonicalRecoveryPlan() throws {
        let raw = #"""
        {
          "team_id": "9D382WV988",
          "fingerprint": "sha256=fixture",
          "developer_identities": [],
          "notarization_observed": false,
          "recovery_plan": ["Use the protected workflow.", "Recheck readiness."],
          "verdict": {
            "schema_version": "maat-system-one/v1",
            "feather_weight": 0,
            "gate": "block",
            "confidence": 1,
            "subject": {"kind": "host", "ref": "release-credentials", "head_sha": "fixture"},
            "floor": {"passed": false, "checks": []},
            "model": {"provider": "local:test", "version": "v1", "local": true, "latency_ms": 0},
            "findings": []
          }
        }
        """#.data(using: .utf8)!

        let preflight = try JSONDecoder().decode(MaatReleaseCredentialPreflight.self, from: raw)

        XCTAssertEqual(protectedReleaseRecoverySteps(for: preflight), ["Use the protected workflow.", "Recheck readiness."])
    }

    func testMaatClosedRepairActionDecodesWithoutAcceptingACommand() throws {
        let raw = #"""
        {
          "kind": "maat_repair",
          "action_id": "launchd-disabled",
          "title": "Restore the managed launchd labels",
          "detail": "bounded repair",
          "evidence": "maat-system-one:sha256=fixture",
          "requires_confirmation": true
        }
        """#.data(using: .utf8)!

        let action = try JSONDecoder().decode(MaatCaseNextAction.self, from: raw)

        XCTAssertEqual(action.kind, "maat_repair")
        XCTAssertEqual(action.actionID, "launchd-disabled")
        XCTAssertFalse(action.detail.contains("launchctl"))
    }

    func testMenubarLeaseAllowsOneLocalProcessAtATime() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("sirsi-menubar-lease-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let path = root.appendingPathComponent("menubar.instance.lock").path

        var first: MenubarInstanceLease? = MenubarInstanceLease.acquire(at: path)
        XCTAssertNotNil(first)
        XCTAssertNil(MenubarInstanceLease.acquire(at: path))
        first = nil
        XCTAssertNotNil(MenubarInstanceLease.acquire(at: path))
    }

    func testProjectRootAdmissionAcceptsGitWorktreesAndRejectsPlainFolders() throws {
        let root = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("sirsi-project-root-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)

        XCTAssertNil(SirsiEngine.projectRootPath(root.path))

        // A linked worktree represents .git as a file rather than a directory.
        try "gitdir: /tmp/fixture".write(to: root.appendingPathComponent(".git"), atomically: true, encoding: .utf8)
        XCTAssertEqual(SirsiEngine.projectRootPath(root.path), root.path)
    }
}
