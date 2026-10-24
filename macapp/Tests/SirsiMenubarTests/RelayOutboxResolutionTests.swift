import XCTest
@testable import SirsiMenubar

@MainActor
final class RelayOutboxResolutionTests: XCTestCase {
    private func board(access: String = "unavailable", eligible: Bool = true,
                       inode: UInt64 = 10, age: TimeInterval = -0.01) throws -> RouterBoard {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let stamp = formatter.string(from: Date().addingTimeInterval(-age))
        let readable = access == "observed"
        let known = readable || access == "service-private"
        let row: [String: Any] = ["agent": "codex-pantheon", "unreadable": !readable,
            "observer_access": access, "repair_eligible": eligible, "repair_spool": "/owned/relay",
            "scope": ["canonical_spool": "/owned/relay", "root_device": 9, "root_inode": inode],
            "retry_source": access == "service-private" ? "relay-producer" : "direct-metadata",
            "retry_state": known ? "observed" : "unavailable", "retry_count": 0,
            "retry_started_at": stamp, "retry_observed_at": stamp]
        return try JSONDecoder().decode(RouterBoard.self,
            from: JSONSerialization.data(withJSONObject: ["outbox": [row]]))
    }

    private func fixture() throws -> (SirsiEngine, RelayOutboxResolutionModel) {
        let engine = SirsiEngine()
        engine.routerBoard = try board()
        engine.routerReadAt = Date()
        let context = RelayOutboxResolutionContext(outbox: try XCTUnwrap(engine.routerBoard?.outbox?.first))
        return (engine, RelayOutboxResolutionModel(context: context))
    }

    // Even a guard-removal negative control must never fall through to a live
    // CLI, board read or activity writer.
    private func noInvocation(_ model: RelayOutboxResolutionModel, engine: SirsiEngine) -> Task<Void, Never>? {
        model.confirm(engine: engine, execute: { _ in
            XCTFail("no approved invocation expected")
            return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        }, readback: { false }, record: { _ in })
    }

    func testActualRaDestinationRetainsSelectedContextAndReturn() throws {
        let (engine, model) = try fixture()
        let card = OutboxBlockerCard(engine: engine, blocked: [model.context.outbox])
        let destination = card.guidedResolution(for: model.context.outbox)
        let context = try XCTUnwrap(destination.relayOutboxContext)
        XCTAssertEqual(context.outbox.agent, "codex-pantheon")
        XCTAssertEqual(context.outbox.scope, model.context.outbox.scope)
        XCTAssertEqual(context.location, "/owned/relay/codex-pantheon/outbox")
        XCTAssertEqual(context.sourceTitle, "Ra Fabric")
        XCTAssertEqual(context.returnTitle, "Back to Ra Fabric")
        XCTAssertNil(MaatWorkspaceView(engine: engine).relayOutboxContext)
        let nav = Nav()
        nav.push(destination)
        XCTAssertEqual(nav.stack.count, 1)
        nav.pop()
        XCTAssertTrue(nav.atRoot)
    }

    func testAmbientInventoryAloneCannotOfferOrExecuteRepair() throws {
        let (engine, model) = try fixture()
        XCTAssertEqual(RelayOutboxResolutionProjection(context: model.context, engine: engine,
                                                      freshPreflight: model.freshPreflight).state, .refreshRequired)
        XCTAssertNil(model.approval)
        XCTAssertNil(noInvocation(model, engine: engine))
    }

    func testFreshPrivateServiceNeverOffersPermissionRepair() async throws {
        let engine = SirsiEngine()
        engine.routerBoard = try board(access: "service-private", eligible: true)
        let model = RelayOutboxResolutionModel(context: RelayOutboxResolutionContext(outbox: engine.routerBoard!.outbox![0]))
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
        XCTAssertNil(model.approval, "even a forged eligible flag cannot authorize service-owned repair")
        let fresh = RelayOutboxResolutionProjection(context: model.context, engine: engine, freshPreflight: model.freshPreflight)
        XCTAssertEqual(fresh.state, .serviceManaged)
        XCTAssertTrue(fresh.guidance.contains("payload access remains private"))
        engine.routerBoard = try board(access: "service-private", eligible: true, age: 16)
        let stale = RelayOutboxResolutionProjection(context: model.context, engine: engine, freshPreflight: true)
        XCTAssertTrue(stale.guidance.contains("relay owner"))
        XCTAssertFalse(stale.guidance.contains("Current relay-producer metadata is available"))
    }

    func testFailedReadAndScopeReplacementRefuseBeforeConfirmation() async throws {
        let (engine, model) = try fixture()
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return false })
        XCTAssertFalse(model.freshPreflight)
        XCTAssertNil(model.approval)
        engine.routerBoard = try board(inode: 99)
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
        XCTAssertNil(model.approval)
        XCTAssertEqual(RelayOutboxResolutionProjection(context: model.context, engine: engine,
                                                      freshPreflight: model.freshPreflight).state, .scopeChanged)
        engine.routerBoard = try JSONDecoder().decode(RouterBoard.self, from: Data("{}".utf8))
        XCTAssertEqual(RelayOutboxResolutionProjection(context: model.context, engine: engine,
                                                      freshPreflight: true).state, .missing)
    }

    func testCancelOrNavigationDuringReadCannotReinstateApproval() async throws {
        let (engine, model) = try fixture()
        await model.refresh(engine: engine, review: true, read: {
            model.cancelReview()
            engine.routerReadAt = Date()
            return true
        })
        XCTAssertNil(model.approval)
        XCTAssertFalse(model.freshPreflight)
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
        XCTAssertNotNil(model.approval)
        model.cancelReview()
        XCTAssertNil(noInvocation(model, engine: engine))
    }

    func testApprovedScopeDriftRefusesAtExistingEngineBoundary() async throws {
        let (engine, model) = try fixture()
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
        XCTAssertNotNil(model.approval)
        engine.routerBoard = try board(inode: 99)
        let task = try XCTUnwrap(model.confirm(engine: engine, execute: { _ in
            XCTFail("changed root must not launch")
            return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        }, readback: { XCTFail("refusal is before readback"); return true }, record: { _ in
            XCTFail("no operation was performed")
        }))
        await task.value
        XCTAssertTrue(model.operationResult?.contains("Repair unavailable") == true)
    }

    func testConfirmationIsOnceOnlyAndFreshReadableRecoveryIsSeparate() async throws {
        let (engine, model) = try fixture()
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
        var launches = 0
        let task = try XCTUnwrap(model.confirm(engine: engine, execute: { args in
            launches += 1
            XCTAssertEqual(args, ["router", "relay", "repair-outbox", "--agent", "codex-pantheon", "--spool", "/owned/relay",
                                  "--expected-root-device", "9", "--expected-root-inode", "10"])
            return CommandExecution(exitCode: 0, childState: .exitConfirmed)
        }, readback: {
            engine.routerBoard = try! self.board(access: "observed", eligible: false)
            engine.routerReadAt = Date()
            return true
        }, record: { _ in }))
        XCTAssertTrue(model.applying, "latch must be set in the synchronous button action")
        XCTAssertNil(noInvocation(model, engine: engine))
        model.cancelReview() // dialog dismissal does not discard confirmed intent
        await task.value
        XCTAssertEqual(launches, 1)
        XCTAssertFalse(model.applying)
        XCTAssertTrue(model.operationResult?.contains("fresh direct outbox metadata is readable") == true)
        let receipt = model.operationResult
        await model.refresh(engine: engine, read: { engine.routerReadAt = Date(); return true })
        XCTAssertEqual(RelayOutboxResolutionProjection(context: model.context, engine: engine,
                                                      freshPreflight: model.freshPreflight).state, .observed)
        XCTAssertEqual(model.operationResult, receipt)
        XCTAssertNil(noInvocation(model, engine: engine), "refresh is never repeat consent")
        XCTAssertEqual(launches, 1)
    }

    func testFailedOrPartialCaptureNeverBecomesSuccessAndRefreshRetainsReceipt() async throws {
        let outcomes = [CommandExecution(stderr: Data("inert failure".utf8), exitCode: 4, childState: .exitConfirmed),
                        CommandExecution(exitCode: 0, timedOut: true, childState: .exitConfirmed)]
        for outcome in outcomes {
            let (engine, model) = try fixture()
            await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
            let task = try XCTUnwrap(model.confirm(engine: engine, execute: { _ in outcome },
                                                   readback: { false }, record: { _ in }))
            await task.value
            let receipt = try XCTUnwrap(model.operationResult)
            XCTAssertTrue(receipt.contains("did not complete successfully"))
            XCTAssertFalse(receipt.contains("fresh direct outbox metadata is readable"))
            await model.refresh(engine: engine, read: { false })
            XCTAssertEqual(model.operationResult, receipt)
            XCTAssertNil(model.approval)
        }
        let (engine, model) = try fixture()
        await model.refresh(engine: engine, review: true, read: { engine.routerReadAt = Date(); return true })
        let task = try XCTUnwrap(model.confirm(engine: engine, execute: { _ in
            CommandExecution(exitCode: 0, childState: .exitConfirmed)
        }, readback: { false }, record: { _ in }))
        await task.value
        XCTAssertTrue(model.operationResult?.contains("Verification unavailable") == true)
    }
}
