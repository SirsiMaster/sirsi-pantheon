import XCTest
import Darwin
import SwiftUI
@testable import SirsiMenubar

final class TrashBrowseRecoveryTests: XCTestCase {
    // Test-only recorder: the injected I/O actor owns the syscall sequence;
    // assertions snapshot it under this lock after the read has returned.
    private final class Probe: @unchecked Sendable {
        private let lock = NSLock()
        private var opens: [(String, Int32)] = []
        private var closed: [Int32] = []
        func opened(_ name: String, _ flags: Int32) { lock.lock(); defer { lock.unlock() }; opens.append((name, flags)) }
        func didClose(_ fd: Int32) { lock.lock(); defer { lock.unlock() }; closed.append(fd) }
        var snapshot: ([(String, Int32)], [Int32]) { lock.lock(); defer { lock.unlock() }; return (opens, closed) }
    }

    private func syscalls(_ target: String, _ number: Int32, _ probe: Probe) -> TrashBrowseSyscalls {
        TrashBrowseSyscalls(open: { parent, name, flags in
            probe.opened(name, flags)
            if (target == "home" && parent == nil) || (target != "home" && name == target) {
                return .init(fd: -1, error: number)
            }
            return TrashBrowseSyscalls.live.open(parent, name, flags)
        }, stream: { fd in
            if target == "fdopendir" { return .init(stream: nil, error: number) }
            return TrashBrowseSyscalls.live.stream(fd)
        }, close: { fd in
            TrashBrowseSyscalls.live.close(fd); probe.didClose(fd)
            errno = ENOSPC // cleanup must not replace the captured cause
        })
    }

    private func ownedHome() throws -> URL {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent("pantheon-trash-recovery-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: home.appendingPathComponent(".Trash"), withIntermediateDirectories: true,
                                                attributes: [.posixPermissions: 0o700])
        addTeardownBlock { try FileManager.default.removeItem(at: home) }
        return home
    }

    @MainActor func testRootErrnoMatrixIsTypedNeverEmptyAndOnlyPermissionOffersGuide() async throws {
        let numbers: [Int32] = [EPERM, EACCES, ELOOP, ENOENT, ENOTDIR, EMFILE, ENFILE, EIO]
        let codes: [TrashBrowseFailure.Code] = [.permissionDenied, .permissionDenied, .noFollowRefused,
                                               .unavailable, .notDirectory, .descriptorLimit, .descriptorLimit, .syscall]
        let home = try ownedHome()
        for (index, number) in numbers.enumerated() {
            let probe = Probe()
            let io = TrashBrowseIO(syscalls: syscalls(".Trash", number, probe))
            let workflow = TrashWorkflow()
            await workflow.loadPage { await io.read(TrashBrowseRequest(), home: home, fence: TrashBrowseReadFence()) }
            XCTAssertEqual(workflow.phase, .error); XCTAssertNil(workflow.page)
            XCTAssertEqual(workflow.browseFailure?.posixErrno, number)
            XCTAssertEqual(workflow.browseFailure?.code, codes[index])
            XCTAssertEqual(workflow.browseFailure?.stage, .trashRoot)
            XCTAssertEqual(workflow.canReviewAccess, number == EPERM || number == EACCES)
            var mutationCalls = 0
            await workflow.reviewSelection { _ in mutationCalls += 1; return CommandExecution() }
            workflow.prepareApproval()
            await workflow.apply { _ in mutationCalls += 1; return CommandExecution() }
            XCTAssertEqual(mutationCalls, 0); XCTAssertNil(workflow.approval)
            let (opens, closed) = probe.snapshot
            XCTAssertEqual(opens.map(\.0), [home.path, ".Trash"])
            XCTAssertTrue(opens.allSatisfy { $0.1 == TrashDirectorySession.flags })
            XCTAssertEqual(closed.count, 1)
            for fd in closed { XCTAssertEqual(fcntl(fd, F_GETFD), -1) }
            await io.close(); await workflow.browser.close()
        }
    }

    func testEveryOpenAndStreamFailureRetainsStageAndClosesOwnedParents() throws {
        let home = try ownedHome()
        try FileManager.default.createDirectory(at: home.appendingPathComponent(".Trash/parent"), withIntermediateDirectories: false)
        let cases: [(String, TrashBrowseFailure.Stage, Int)] = [
            ("home", .ownerHome, 0), (".Trash", .trashRoot, 1), ("parent", .ancestor, 2),
            (".", .directoryStream, 2), ("fdopendir", .directoryStream, 3)]
        for (target, stage, closedCount) in cases {
            let probe = Probe()
            let request = TrashBrowseRequest(directory: target == "parent" ? "parent" : ".")
            XCTAssertThrowsError(try TrashDirectorySession(home: home, request: request, syscalls: syscalls(target, EACCES, probe))) { error in
                let failure = error as? TrashBrowseFailure
                XCTAssertEqual(failure?.stage, stage); XCTAssertEqual(failure?.posixErrno, EACCES)
                XCTAssertEqual(failure?.code, .permissionDenied)
            }
            let (opens, closed) = probe.snapshot
            XCTAssertEqual(closed.count, closedCount)
            XCTAssertTrue(opens.allSatisfy { $0.1 == TrashDirectorySession.flags })
            for fd in closed { XCTAssertEqual(fcntl(fd, F_GETFD), -1) }
        }
    }

    func testRealOwnedAbsentNonDirectorySymlinkAndIdentityRefusalsStaySafe() throws {
        for kind in ["absent", "file", "symlink", "identity"] {
            let home = try ownedHome(); let root = home.appendingPathComponent(".Trash")
            if kind == "identity" {
                let session = try TrashDirectorySession(home: home, request: TrashBrowseRequest())
                let r = session.root; session.close()
                let changed = TrashIdentity(device: r.device, inode: r.inode + 1, uid: r.uid, mode: r.mode,
                                            size: r.size, mtime_ns: r.mtime_ns, ctime_ns: r.ctime_ns, type: r.type)
                XCTAssertThrowsError(try TrashDirectorySession(home: home, request: TrashBrowseRequest(root: changed))) { error in
                    let failure = error as? TrashBrowseFailure
                    XCTAssertEqual(failure?.code, .safetyRefusal); XCTAssertNil(failure?.posixErrno)
                    XCTAssertEqual(failure?.offersAccessGuide, false)
                }
                continue
            }
            try FileManager.default.removeItem(at: root)
            if kind == "file" { try Data().write(to: root) }
            if kind == "symlink" {
                let target = home.appendingPathComponent("owned-target")
                try FileManager.default.createDirectory(at: target, withIntermediateDirectories: false)
                try FileManager.default.createSymbolicLink(at: root, withDestinationURL: target)
            }
            XCTAssertThrowsError(try TrashDirectorySession(home: home, request: TrashBrowseRequest())) { error in
                let failure = error as? TrashBrowseFailure
                XCTAssertEqual(failure?.stage, .trashRoot)
                XCTAssertNotNil(failure?.posixErrno); XCTAssertEqual(failure?.offersAccessGuide, false)
                if kind == "absent" { XCTAssertEqual(failure?.code, .unavailable) }
                // Darwin may return ENOTDIR or ELOOP for no-follow directories;
                // do not assert a stronger filesystem-specific causal diagnosis.
            }
        }
    }

    @MainActor func testUnknownMalformedAndIncompleteEnvelopesNeverOfferAccess() async throws {
        let failure = TrashBrowseFailure(errno: EPERM, stage: .trashRoot)
        let valid = failure.execution
        var unknown = try XCTUnwrap(JSONSerialization.jsonObject(with: valid.stdoutData) as? [String: Any])
        unknown["schema"] = "unknown"
        var mismatched = try XCTUnwrap(JSONSerialization.jsonObject(with: valid.stdoutData) as? [String: Any])
        mismatched["posixErrno"] = ENOENT
        let invalidData = [Data("not JSON".utf8), try JSONSerialization.data(withJSONObject: unknown),
                           try JSONSerialization.data(withJSONObject: mismatched)]
        var captures = invalidData.map { CommandExecution(stdout: $0, exitCode: 1, launchError: failure.message, childState: .exitConfirmed) }
        captures += [CommandExecution(stdout: valid.stdoutData, exitCode: 0, childState: .exitConfirmed),
                     CommandExecution(stdout: valid.stdoutData, exitCode: 1, launchError: failure.message, cancelled: true, childState: .exitConfirmed),
                     CommandExecution(stdout: valid.stdoutData, exitCode: 1, launchError: failure.message, timedOut: true, childState: .exitConfirmed),
                     CommandExecution(stdout: valid.stdoutData, exitCode: 1, launchError: failure.message, overflow: true, childState: .exitConfirmed)]
        for capture in captures {
            let workflow = TrashWorkflow(); await workflow.loadPage { capture }
            XCTAssertEqual(workflow.phase, .error); XCTAssertNil(workflow.browseFailure)
            XCTAssertFalse(workflow.canReviewAccess); XCTAssertNil(workflow.page)
            await workflow.browser.close()
        }
    }

    @MainActor func testDelayedOldPermissionFailureCannotOverwriteFreshPage() async throws {
        let home = try ownedHome(); let io = TrashBrowseIO()
        let fresh = await io.read(TrashBrowseRequest(), home: home, fence: TrashBrowseReadFence())
        let workflow = TrashWorkflow()
        var pending: AsyncStream<CommandExecution>.Continuation!
        let stream = AsyncStream<CommandExecution> { pending = $0 }
        defer { pending.finish() }
        let entered = expectation(description: "old read held")
        let old = Task { @MainActor in
            await workflow.loadPage {
                entered.fulfill()
                for await capture in stream { return capture }
                return CommandExecution(cancelled: true)
            }
        }
        await fulfillment(of: [entered], timeout: 2)
        await workflow.loadPage { fresh }
        pending.yield(TrashBrowseFailure(errno: EPERM, stage: .trashRoot).execution)
        await old.value
        XCTAssertEqual(workflow.phase, .empty); XCTAssertNil(workflow.browseFailure)
        XCTAssertFalse(workflow.canReviewAccess)
        await io.close(); await workflow.browser.close()
    }

    @MainActor func testGuideReturnRequiresExplicitFreshBrowseAndNewConsent() async throws {
        let home = try ownedHome()
        try Data("inert".utf8).write(to: home.appendingPathComponent(".Trash/owned"))
        let io = TrashBrowseIO()
        let fresh = await io.read(TrashBrowseRequest(), home: home, fence: TrashBrowseReadFence())
        let page = try JSONDecoder().decode(TrashPage.self, from: fresh.stdoutData)
        let member = try XCTUnwrap(page.entries.first)
        let workflow = TrashWorkflow(); await workflow.loadPage { fresh }; workflow.toggle(member)
        let manifest = TrashManifest(schema: "pantheon.trash-manifest.v1", uid: page.uid, root: page.root,
                                     containers: page.containers, entries: [member])
        let ready = TrashReport(schema: "pantheon.trash-report.v1", status: "ready", preview: manifest,
                                attempted: 0, deleted: [], logical_bytes: 0, errors: [])
        await workflow.reviewSelection { _ in CommandExecution(stdout: try! JSONEncoder().encode(ready), exitCode: 0, childState: .exitConfirmed) }
        workflow.prepareApproval(); XCTAssertNotNil(workflow.approval)
        await workflow.loadPage { TrashBrowseFailure(errno: EACCES, stage: .trashRoot).execution }
        workflow.beginAccessReview(); XCTAssertNil(workflow.approval); XCTAssertNil(workflow.preview)
        XCTAssertTrue(workflow.selected.isEmpty); XCTAssertNil(workflow.page)
        workflow.activateBrowse()
        var reads = 0; var mutations = 0
        await workflow.loadInitial { reads += 1; return fresh }
        XCTAssertEqual(reads, 0, "returning from Settings must not probe or retry automatically")
        await workflow.apply { _ in mutations += 1; return CommandExecution() }
        XCTAssertEqual(mutations, 0)
        await workflow.loadPage { reads += 1; return fresh }
        XCTAssertEqual(reads, 1); XCTAssertEqual(workflow.phase, .browsing)
        XCTAssertNil(workflow.browseFailure); XCTAssertFalse(workflow.canReviewAccess)
        XCTAssertTrue(workflow.selected.isEmpty); XCTAssertNil(workflow.approval)
        await io.close(); await workflow.browser.close()
    }

    @MainActor func testCancelledPermissionReadCannotInstallRecovery() async {
        let workflow = TrashWorkflow()
        var pending: AsyncStream<CommandExecution>.Continuation!
        let stream = AsyncStream<CommandExecution> { pending = $0 }
        defer { pending.finish() }
        let entered = expectation(description: "cancelled read held")
        let reading = Task { @MainActor in
            await workflow.loadPage {
                entered.fulfill()
                for await capture in stream { return capture }
                return CommandExecution(cancelled: true)
            }
        }
        await fulfillment(of: [entered], timeout: 2)
        reading.cancel()
        pending.yield(TrashBrowseFailure(errno: EPERM, stage: .trashRoot).execution)
        await reading.value
        XCTAssertEqual(workflow.phase, .error); XCTAssertNil(workflow.browseFailure)
        XCTAssertFalse(workflow.canReviewAccess); XCTAssertNil(workflow.approval)
        await workflow.browser.close()
    }

    @MainActor func testAccessRetryFailurePreservesPriorOperationReceipt() async throws {
        let home = try ownedHome()
        try Data("inert".utf8).write(to: home.appendingPathComponent(".Trash/owned"))
        let io = TrashBrowseIO()
        let raw = await io.read(TrashBrowseRequest(), home: home, fence: TrashBrowseReadFence())
        let page = try JSONDecoder().decode(TrashPage.self, from: raw.stdoutData)
        let member = try XCTUnwrap(page.entries.first)
        let manifest = TrashManifest(schema: "pantheon.trash-manifest.v1", uid: page.uid, root: page.root,
                                     containers: [], entries: [member])
        let ready = TrashReport(schema: "pantheon.trash-report.v1", status: "ready", preview: manifest,
                                attempted: 0, deleted: [], logical_bytes: 0, errors: [])
        let workflow = TrashWorkflow()
        await workflow.loadPreview { CommandExecution(stdout: try! JSONEncoder().encode(ready), exitCode: 0, childState: .exitConfirmed) }
        workflow.prepareApproval()
        let report = TrashReport(schema: "pantheon.trash-report.v1", status: "completed", preview: nil,
                                 attempted: 1, deleted: [member.path], logical_bytes: member.identity.size, errors: [])
        let receipt = CommandExecution(stdout: try JSONEncoder().encode(report), exitCode: 0, childState: .exitConfirmed)
        var mutations = 0
        await workflow.apply { _ in mutations += 1; return receipt } // inert report, no deletion
        let failure = TrashBrowseFailure(errno: EACCES, stage: .trashRoot).execution
        await workflow.loadPage { failure }
        workflow.beginAccessReview(); workflow.activateBrowse()
        await workflow.loadPage { failure }
        XCTAssertEqual(workflow.execution, receipt); XCTAssertEqual(workflow.operation, report)
        XCTAssertEqual(workflow.phase, .error); XCTAssertTrue(workflow.canReviewAccess)
        XCTAssertNil(workflow.approval); XCTAssertTrue(workflow.selected.isEmpty)
        await workflow.apply { _ in mutations += 1; return receipt }
        XCTAssertEqual(mutations, 1)
        await io.close(); await workflow.browser.close()
    }

    func testGuideIdentifiesCurrentBundleAndOnlyExplicitActionOpensSettings() throws {
        let bundle = URL(fileURLWithPath: "/Applications/Pantheon.app")
        let guide = PantheonAccessGuide(bundleURL: bundle)
        XCTAssertEqual(guide.appName, "Pantheon"); XCTAssertEqual(PantheonAccessGuide().bundleURL, Bundle.main.bundleURL)
        var opened: [URL] = []; var revealed: [URL] = []
        XCTAssertTrue(opened.isEmpty); XCTAssertTrue(revealed.isEmpty)
        guide.openSettings(open: { opened.append($0) }, reveal: { revealed.append($0) })
        XCTAssertEqual(opened.map(\.absoluteString), [fullDiskAccessPaneURL]); XCTAssertEqual(revealed, [bundle])
        let macapp = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let views = try String(contentsOf: macapp.appendingPathComponent("Sources/SirsiMenubar/Views.swift"), encoding: .utf8)
        let body = try XCTUnwrap(views.components(separatedBy: "struct FDAGuideView: View {").last?.components(separatedBy: "// ── Horus").first)
        XCTAssertFalse(body.contains("registerForFullDiskAccess()"))
        XCTAssertFalse(body.contains("Sirsi Menubar")); XCTAssertTrue(body.contains("Back to Trash"))
        XCTAssertTrue(views.contains("if workflow.canReviewAccess"))
        XCTAssertTrue(views.contains(".onAppear { workflow.beginAccessReview() }"))
    }

    @MainActor func testActualGuideReturnPopsOwnedStackAndRetainsFreshIntentFence() async throws {
        let workflow = TrashWorkflow()
        let home = try ownedHome()
        try Data("inert".utf8).write(to: home.appendingPathComponent(".Trash/owned"))
        let io = TrashBrowseIO()
        let fresh = await io.read(TrashBrowseRequest(), home: home, fence: TrashBrowseReadFence())
        let page = try JSONDecoder().decode(TrashPage.self, from: fresh.stdoutData)
        let member = try XCTUnwrap(page.entries.first)
        await workflow.loadPage { fresh }; workflow.toggle(member)
        let manifest = TrashManifest(schema: "pantheon.trash-manifest.v1", uid: page.uid, root: page.root,
                                     containers: [], entries: [member])
        let ready = TrashReport(schema: "pantheon.trash-report.v1", status: "ready", preview: manifest,
                                attempted: 0, deleted: [], logical_bytes: 0, errors: [])
        await workflow.reviewSelection { _ in CommandExecution(stdout: try! JSONEncoder().encode(ready), exitCode: 0, childState: .exitConfirmed) }
        workflow.prepareApproval(); XCTAssertNotNil(workflow.approval)
        let origin = EmptyTrashView(engine: SirsiEngine(), workflow: workflow)
        let nav = Nav()
        nav.push(Text("Anubis"))
        let parentID = try XCTUnwrap(nav.stack.last?.id)
        nav.push(origin)
        let originID = try XCTUnwrap(nav.stack.last?.id)
        await workflow.loadPage { TrashBrowseFailure(errno: EPERM, stage: .trashRoot).execution }
        XCTAssertTrue(workflow.canReviewAccess)
        // This is the exact guide-entry hook and exact return action wired into
        // the production view, without rendering UI or triggering permissions.
        workflow.beginAccessReview()
        let guide = FDAGuideView(returnToTrash: true)
        nav.push(guide)
        XCTAssertEqual(nav.stack.count, 3)
        XCTAssertNotEqual(nav.stack.last?.id, originID)
        guide.returnToOrigin(using: nav)
        XCTAssertEqual(nav.stack.count, 2)
        XCTAssertEqual(nav.stack.last?.id, originID)
        XCTAssertEqual(nav.stack.first?.id, parentID)
        XCTAssertTrue(origin.workflow === workflow, "the stored origin owns the same model across guide remount")
        origin.workflow.activateBrowse()
        var browseCalls = 0; var selectionCalls = 0; var applyCalls = 0
        await origin.workflow.loadInitial { browseCalls += 1; return CommandExecution() }
        await origin.workflow.reviewSelection { _ in selectionCalls += 1; return CommandExecution() }
        origin.workflow.prepareApproval()
        await origin.workflow.apply { _ in applyCalls += 1; return CommandExecution() }
        XCTAssertEqual(browseCalls, 0); XCTAssertEqual(selectionCalls, 0); XCTAssertEqual(applyCalls, 0)
        XCTAssertTrue(workflow.selected.isEmpty); XCTAssertNil(workflow.page)
        XCTAssertNil(workflow.preview); XCTAssertNil(workflow.approval)
        XCTAssertEqual(workflow.phase, .error)
        // The unrelated/default guide retains its existing BackBar behavior;
        // it does not gain a forced Trash destination or a second pushed frame.
        XCTAssertFalse(FDAGuideView().returnToTrash)
        let macapp = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let views = try String(contentsOf: macapp.appendingPathComponent("Sources/SirsiMenubar/Views.swift"), encoding: .utf8)
        XCTAssertTrue(views.contains("Button(\"Back to Trash\") { returnToOrigin(using: nav) }"))
        await io.close(); await workflow.browser.close()
    }
}
