import XCTest
@testable import SirsiMenubar

final class TrashReceiptTests: XCTestCase {
    private func manifest(empty: Bool = false) -> TrashManifest {
        let root = TrashIdentity(device: 1, inode: 2, uid: 501, mode: 0o040700, size: 0,
                                 mtime_ns: 10, ctime_ns: 10, type: "directory")
        let file = TrashIdentity(device: 1, inode: 3, uid: 501, mode: 0o100600, size: 4,
                                 mtime_ns: 10, ctime_ns: 10, type: "file")
        return TrashManifest(schema: "pantheon.trash-manifest.v1", uid: 501, root: root,
                             containers: [], entries: empty ? [] : [TrashMember(path: "reviewed", identity: file)])
    }

    private func capture(_ report: TrashReport, exit: Int32 = 0) throws -> CommandExecution {
        CommandExecution(stdout: try JSONEncoder().encode(report), exitCode: exit, childState: .exitConfirmed)
    }

    private func preview(_ manifest: TrashManifest) throws -> CommandExecution {
        try capture(TrashReport(schema: "pantheon.trash-report.v1", status: manifest.entries.isEmpty ? "empty" : "ready",
                               preview: manifest, attempted: 0, deleted: [], logical_bytes: 0, errors: []))
    }

    func testCaptureFailureNeverBecomesSuccessOrEmpty() throws {
        let valid = try preview(manifest(empty: true))
        for execution in [CommandExecution(stdout: Data("not json".utf8), exitCode: 0, childState: .exitConfirmed),
                          CommandExecution(stdout: valid.stdoutData, exitCode: 1, childState: .exitConfirmed),
                          CommandExecution(stdout: valid.stdoutData, exitCode: 0, timedOut: true, childState: .exitConfirmed),
                          CommandExecution(stdout: valid.stdoutData, exitCode: 0, cancelled: true, childState: .exitConfirmed),
                          CommandExecution(stdout: valid.stdoutData, exitCode: 0, overflow: true, childState: .exitConfirmed),
                          CommandExecution(stdout: valid.stdoutData, exitCode: 0, childState: .exitUnknown),
                          CommandExecution(launchError: "owned inert refusal")] {
            let read = TrashCommandRead(execution, preview: true)
            XCTAssertNil(read.report); XCTAssertNotNil(read.issue)
        }
        XCTAssertEqual(TrashCommandRead(valid, preview: true).report?.status, "empty")
    }

    func testSchemaAndDeletionPrefixMustMatch() throws {
        let m = manifest()
        let good = TrashReport(schema: "pantheon.trash-report.v1", status: "completed", preview: nil,
                               attempted: 1, deleted: ["reviewed"], logical_bytes: 4, errors: [])
        XCTAssertTrue(good.validWireShape); XCTAssertTrue(good.matches(m))
        let wrong = TrashReport(schema: "wrong", status: "completed", preview: nil,
                                attempted: 1, deleted: ["unreviewed"], logical_bytes: 4, errors: [])
        XCTAssertFalse(wrong.validWireShape); XCTAssertFalse(wrong.matches(m))
        XCTAssertNil(TrashCommandRead(try capture(good, exit: 1), preview: false).report)
        let approval = try TrashApproval(m)
        XCTAssertEqual(try JSONDecoder().decode(TrashManifest.self, from: Data(approval.stdin.utf8)), m)
        XCTAssertEqual(approval.args, ["anubis", "empty-trash", "apply", "--confirm"])
    }

    @MainActor func testDoubleTapOnceAndRefreshNeverReplaysDeletion() async throws {
        let workflow = TrashWorkflow(); let m = manifest(); let ready = try preview(m)
        await workflow.loadPreview { ready }; workflow.prepareApproval()
        let frozen = try XCTUnwrap(workflow.approval)
        let done = try capture(TrashReport(schema: "pantheon.trash-report.v1", status: "completed", preview: nil,
                                         attempted: 1, deleted: ["reviewed"], logical_bytes: 4, errors: []))
        var calls = 0
        await workflow.apply { received in
            calls += 1; XCTAssertEqual(received, frozen); XCTAssertTrue(workflow.permitsLaunch(received))
            await workflow.apply { _ in calls += 1; return done }
            return done
        }
        await workflow.apply { _ in calls += 1; return done }
        XCTAssertEqual(calls, 1); XCTAssertTrue(workflow.confirmedSuccess)
        await workflow.loadPage { CommandExecution(launchError: "owned failed read") }
        XCTAssertEqual(workflow.phase, .error)
        XCTAssertEqual(workflow.execution, done); XCTAssertEqual(workflow.operation?.deleted, ["reviewed"])
        XCTAssertNil(workflow.approval)
        await workflow.apply { _ in calls += 1; return done }
        XCTAssertEqual(calls, 1)
    }

    @MainActor func testReadOnlyRefreshInvalidatesFrozenApproval() async throws {
        let workflow = TrashWorkflow(); let m = manifest(); let ready = try preview(m)
        await workflow.loadPreview { ready }; workflow.prepareApproval(); XCTAssertNotNil(workflow.approval)
        let empty = try preview(manifest(empty: true))
        await workflow.loadPreview { empty }; XCTAssertNil(workflow.approval)
        var calls = 0
        await workflow.apply { _ in calls += 1; return ready }
        XCTAssertEqual(calls, 0); XCTAssertEqual(workflow.phase, .empty)
    }

    @MainActor func testPartialAndUnknownRemainRetainedNotGreen() async throws {
        let m = manifest(); let ready = try preview(m)
        let failed = try capture(TrashReport(schema: "pantheon.trash-report.v1", status: "failed", preview: nil,
                                           attempted: 1, deleted: [], logical_bytes: 0, errors: ["owned unlink refused"]), exit: 1)
        for result in [failed, CommandExecution(stdout: failed.stdoutData, exitCode: 1, timedOut: true, childState: .exitUnknown)] {
            let workflow = TrashWorkflow(); await workflow.loadPreview { ready }; workflow.prepareApproval()
            await workflow.apply { _ in result }
            XCTAssertFalse(workflow.confirmedSuccess); XCTAssertEqual(workflow.phase, .result)
            XCTAssertEqual(workflow.execution, result); XCTAssertNotNil(workflow.message)
        }
    }

    @MainActor func testPartialDeletedPrefixSurvivesFailedRemainingReview() async throws {
        let base = manifest()
        let m = TrashManifest(schema: base.schema, uid: base.uid, root: base.root,
                              containers: [],
                              entries: [TrashMember(path: "folder", identity: base.root),
                                        TrashMember(path: "folder/reviewed", identity: base.entries[0].identity)])
        let ready = try preview(m)
        let partial = try capture(TrashReport(schema: "pantheon.trash-report.v1", status: "partial", preview: nil,
                                            attempted: 2, deleted: ["folder/reviewed"], logical_bytes: 4,
                                            errors: ["folder: not empty; late addition preserved"]), exit: 1)
        let workflow = TrashWorkflow(); await workflow.loadPreview { ready }; workflow.prepareApproval()
        await workflow.apply { _ in partial }
        XCTAssertEqual(workflow.operation?.status, "partial")
        XCTAssertEqual(workflow.operation?.deleted, ["folder/reviewed"])
        XCTAssertFalse(workflow.confirmedSuccess)
        await workflow.loadPage { CommandExecution(launchError: "owned read failure") }
        XCTAssertEqual(workflow.operation?.deleted, ["folder/reviewed"])
        XCTAssertEqual(workflow.execution, partial)
        XCTAssertNil(workflow.approval)
    }

    func testActualSwiftEncodedManifestCannotExceedRunner64KiB() throws {
        let small = manifest()
        XCTAssertLessThanOrEqual(try TrashApproval(small).stdin.utf8.count, 64 * 1024)
        let many = (0..<400).map { index in
            TrashMember(path: String(format: "%04d-", index) + String(repeating: "\\", count: 180),
                        identity: small.entries[0].identity)
        }
        let large = TrashManifest(schema: small.schema, uid: small.uid, root: small.root, containers: [], entries: many)
        XCTAssertTrue(large.validWireShape)
        XCTAssertGreaterThan(try JSONEncoder().encode(large).count, 64 * 1024)
        XCTAssertThrowsError(try TrashApproval(large))
    }

    @MainActor func testPagedBrowseSelectionAndDescendantContainers() async throws {
        let base = manifest()
        let entries = (0..<32).map { TrashMember(path: String(format: "entry-%03d", $0), identity: base.entries[0].identity) }
        let stamp = String(repeating: "a", count: 64)
        let next = TrashBrowseRequest(directory: ".", session: UUID(), sequence: 1, generation: stamp, root: base.root)
        let page = TrashPage(schema: "pantheon.trash-browse.v3", status: "page", uid: base.uid, root: base.root,
                             directory: ".", containers: [], entries: entries, blocked: [],
                             work: TrashBrowseWork(native_calls: 32, bytes: 4096, inspected: entries.count),
                             generation: stamp, more: true, next: next, errors: [])
        XCTAssertTrue(page.validWireShape)
        let rootInput = try trashWireInput(TrashBrowseRequest())
        let rootObject = try XCTUnwrap(JSONSerialization.jsonObject(with: Data(rootInput.utf8)) as? [String: Any])
        XCTAssertTrue(rootObject["root"] is NSNull)
        let workflow = TrashWorkflow()
        let read = CommandExecution(stdout: try JSONEncoder().encode(page), exitCode: 0, childState: .exitConfirmed)
        await workflow.loadInitial { read }
        XCTAssertEqual(workflow.phase, .browsing); XCTAssertNil(workflow.approval)
        workflow.toggle(entries[0])
        let chosen = TrashManifest(schema: base.schema, uid: base.uid, root: base.root, containers: [], entries: [entries[0]])
        let ready = try preview(chosen)
        var selectionReads = 0
        await workflow.reviewSelection { input in
            selectionReads += 1
            XCTAssertLessThanOrEqual(input.utf8.count, 64 * 1024)
            let request = try? JSONDecoder().decode(TrashSelectionRequest.self, from: Data(input.utf8))
            XCTAssertEqual(request?.entries, [entries[0]])
            return ready
        }
        XCTAssertEqual(selectionReads, 1); XCTAssertEqual(workflow.phase, .ready)
        workflow.prepareApproval(); XCTAssertNotNil(workflow.approval)
        // A cursor is browse metadata, not consent; page navigation invalidates
        // the reviewed manifest and never calls an apply closure.
        await workflow.loadPage { read }
        XCTAssertNil(workflow.approval); XCTAssertTrue(workflow.selected.isEmpty)
        let nested = TrashManifest(schema: base.schema, uid: base.uid, root: base.root,
                                   containers: [TrashMember(path: "large-build-folder", identity: base.root)],
                                   entries: [TrashMember(path: "large-build-folder/only-selected-child", identity: base.entries[0].identity)])
        XCTAssertTrue(nested.validWireShape)
        let frozen = try TrashApproval(nested)
        XCTAssertEqual(frozen.manifest.containers.count, 1)
        XCTAssertEqual(frozen.manifest.entries.map(\.path), ["large-build-folder/only-selected-child"])
    }

    @MainActor func testAllBlockedPageIsNotEmptyAndCanAdvanceWithoutApproval() async throws {
        let base = manifest()
        let stamp = String(repeating: "a", count: 64)
        let cursor = TrashBrowseRequest(session: UUID(), sequence: 1, generation: stamp, root: base.root)
        let blocked = TrashPage(schema: "pantheon.trash-browse.v3", status: "page", uid: base.uid, root: base.root,
                                directory: ".", containers: [], entries: [],
                                blocked: [TrashBlockedMember(path: "blocked", reason: "stat_failed", type: "unknown")],
                                work: TrashBrowseWork(native_calls: 1, bytes: 64, inspected: 1),
                                generation: stamp, more: true, next: cursor, errors: [])
        XCTAssertTrue(blocked.validWireShape)
        let workflow = TrashWorkflow()
        await workflow.loadInitial { CommandExecution(stdout: try! JSONEncoder().encode(blocked), exitCode: 0, childState: .exitConfirmed) }
        XCTAssertEqual(workflow.phase, .browsing)
        XCTAssertEqual(workflow.page?.next, cursor)
        workflow.toggle(base.entries[0]); XCTAssertTrue(workflow.selected.isEmpty)
        workflow.prepareApproval(); XCTAssertNil(workflow.approval)
        workflow.showPage(); XCTAssertEqual(workflow.phase, .browsing)
        let later = TrashPage(schema: blocked.schema, status: "page", uid: base.uid, root: base.root,
                              directory: ".", containers: [], entries: base.entries, blocked: [],
                              work: TrashBrowseWork(native_calls: 1, bytes: 64, inspected: base.entries.count),
                              generation: stamp, more: false, next: nil, errors: [])
        await workflow.loadPage { CommandExecution(stdout: try! JSONEncoder().encode(later), exitCode: 0, childState: .exitConfirmed) }
        XCTAssertEqual(workflow.phase, .browsing)
        workflow.toggle(base.entries[0]); XCTAssertEqual(workflow.selected.count, 1)
        XCTAssertNil(workflow.approval)
    }

    func testBrowseDirectoryRowsAreNotRetainedDeletionContainers() {
        let base = manifest()
        let directories = (0..<32).map { TrashMember(path: String(format: "folder-%03d", $0), identity: base.root) }
        let page = TrashPage(schema: "pantheon.trash-browse.v3", status: "page", uid: base.uid, root: base.root,
                             directory: ".", containers: [], entries: directories, blocked: [],
                             work: TrashBrowseWork(native_calls: 32, bytes: 4096, inspected: directories.count),
                             generation: String(repeating: "a", count: 64), more: false, next: nil, errors: [])
        XCTAssertTrue(page.validWireShape)
    }
}
