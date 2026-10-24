import XCTest
import Darwin
@testable import SirsiMenubar

final class TrashBrowseSessionTests: XCTestCase {
    private func ownedHome(_ count: Int = 0) throws -> URL {
        let home = FileManager.default.temporaryDirectory.appendingPathComponent("pantheon-trash-session-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: home.appendingPathComponent(".Trash"), withIntermediateDirectories: true,
                                                attributes: [.posixPermissions: 0o700])
        addTeardownBlock { try FileManager.default.removeItem(at: home) }
        for index in 0..<count {
            try Data("owned inert fixture".utf8).write(to: home.appendingPathComponent(String(format: ".Trash/owned-%04d", index)))
        }
        return home
    }

    func testRetainedStreamExactCoverageAndRealWorkBounds() throws {
        let total = 1003
        let home = try ownedHome(total)
        let session = try TrashDirectorySession(home: home, request: TrashBrowseRequest())
        defer { session.close() }
        var request = TrashBrowseRequest()
        var seen: Set<String> = []; var calls = 0; var bytes = 0
        for _ in 0...total {
            let page = try session.page(request)
            XCTAssertTrue(page.validWireShape)
            XCTAssertTrue(page.blocked.isEmpty)
            XCTAssertLessThanOrEqual(page.work.inspected, 32)
            XCTAssertLessThanOrEqual(page.work.native_calls, 34)
            XCTAssertLessThanOrEqual(page.work.bytes, 34 * 1024)
            calls += page.work.native_calls; bytes += page.work.bytes
            for row in page.entries { XCTAssertTrue(seen.insert(row.path).inserted, "replayed \(row.path)") }
            guard let next = page.next else {
                XCTAssertEqual(seen, Set((0..<total).map { String(format: "owned-%04d", $0) }))
                XCTAssertGreaterThanOrEqual(calls, total + 1) // rows plus EOF, optionally dot entries
                XCTAssertLessThanOrEqual(calls, total + 3)
                let expectedBytes = seen.reduce(0) { $0 + $1.utf8.count + 1 }
                XCTAssertGreaterThanOrEqual(bytes, expectedBytes)
                XCTAssertLessThanOrEqual(bytes, expectedBytes + 5)
                XCTAssertTrue(session.isClosed); XCTAssertEqual(session.ownedDescriptorCount, 0)
                return
            }
            XCTAssertEqual(next.session, session.id)
            XCTAssertEqual(next.sequence, request.sequence + 1)
            request = next
        }
        XCTFail("retained stream did not reach EOF")
    }

    func testAllBlockedPageAdvancesToLaterEligibleWithoutReplay() throws {
        let home = try ownedHome(100)
        var refuse = true
        let session = try TrashDirectorySession(home: home, request: TrashBrowseRequest(), leafStat: { fd, name in
            if refuse { throw TrashDirectorySession.Refusal(message: "owned stat fixture") }
            return try TrashDirectorySession.statAt(fd, name)
        })
        defer { session.close() }
        let first = try session.page(TrashBrowseRequest())
        XCTAssertTrue(first.entries.isEmpty); XCTAssertEqual(first.blocked.count, 32)
        XCTAssertEqual(Set(first.blocked.map(\.reason)), ["stat_failed"])
        var request = try XCTUnwrap(first.next)
        var seen = Set(first.blocked.map(\.path)); var eligible = 0
        refuse = false
        for _ in 0..<100 {
            let later = try session.page(request)
            for entry in later.entries { XCTAssertTrue(seen.insert(entry.path).inserted); eligible += 1 }
            if let next = later.next { request = next }
            else { XCTAssertEqual(seen.count, 100); XCTAssertEqual(eligible, 68); return }
        }
        XCTFail("blocked page prevented reaching later eligible names")
    }

    func testStatSpecialAndInvalidNamesAreBlockedNotWholePageFailure() throws {
        let home = try ownedHome()
        let root = home.appendingPathComponent(".Trash")
        for name in ["good", "stat", "owner", "device"] { try Data().write(to: root.appendingPathComponent(name)) }
        XCTAssertEqual(root.appendingPathComponent("fifo").path.withCString { mkfifo($0, 0o600) }, 0)
        let session = try TrashDirectorySession(home: home, request: TrashBrowseRequest(), leafStat: { fd, name in
            if name == "stat" { throw TrashDirectorySession.Refusal(message: "owned stat failure") }
            let value = try TrashDirectorySession.statAt(fd, name)
            return TrashIdentity(device: name == "device" ? value.device + 1 : value.device, inode: value.inode,
                                 uid: name == "owner" ? value.uid + 1 : value.uid, mode: value.mode, size: value.size,
                                 mtime_ns: value.mtime_ns, ctime_ns: value.ctime_ns, type: value.type)
        })
        defer { session.close() }
        let page = try session.page(TrashBrowseRequest())
        XCTAssertEqual(page.entries.map(\.path), ["good"])
        XCTAssertEqual(Set(page.blocked.map(\.reason)), ["stat_failed", "unsupported_type", "unowned", "cross_device"])
        XCTAssertEqual(page.work.inspected, 5)

        // APFS refused creation of 0xff in Main's actual 070 qualification.
        // Inject inert raw dirent-name bytes into the SAME production decoder;
        // do not claim the kernel yielded an invalid leaf or count it as real I/O.
        let invalidRecords: [[UInt8]] = [[0xff], [0xc3, 0x28], [0x61, 0, 0x62], Array("bad/name".utf8)]
        for bytes in invalidRecords {
            let decoded = try TrashDirectorySession.decodeName(Data(bytes), directory: ".")
            XCTAssertEqual(decoded, .blocked(TrashBlockedMember(path: "(invalid name)", reason: "invalid_path", type: "unknown")))
        }
        XCTAssertEqual(try TrashDirectorySession.decodeName(Data("good".utf8), directory: "."),
                       .eligible(name: "good", relative: "good"))
        XCTAssertEqual(try TrashDirectorySession.decodeName(Data("good".utf8), directory: "parent"),
                       .eligible(name: "good", relative: "parent/good"))
        for dot in [".", ".."] { XCTAssertEqual(try TrashDirectorySession.decodeName(Data(dot.utf8), directory: "."), .dot) }
        XCTAssertThrowsError(try TrashDirectorySession.decodeName(Data(), directory: "."))
        XCTAssertThrowsError(try TrashDirectorySession.decodeName(Data(repeating: 0x61, count: 1024), directory: "."))
    }

    func testGenerationSequenceAndSessionReplayRefuseAndClose() throws {
        for defect in ["sequence", "session", "generation", "changed-directory"] {
            let home = try ownedHome(100)
            let session = try TrashDirectorySession(home: home, request: TrashBrowseRequest())
            let first = try session.page(TrashBrowseRequest())
            let next = try XCTUnwrap(first.next)
            var request = next
            switch defect {
            case "sequence": _ = try session.page(next) // replay the consumed Next
            case "session": request = TrashBrowseRequest(session: UUID(), sequence: next.sequence, generation: next.generation, root: next.root)
            case "generation": request = TrashBrowseRequest(session: next.session, sequence: next.sequence, generation: String(repeating: "f", count: 64), root: next.root)
            default: try Data().write(to: home.appendingPathComponent(".Trash/new-generation"))
            }
            XCTAssertThrowsError(try session.page(request), defect)
            XCTAssertTrue(session.isClosed); XCTAssertEqual(session.ownedDescriptorCount, 0)
        }
    }

    func testRootAndParentSubstitutionRefuseAndClose() throws {
        for changed in ["root", "parent"] {
            let home = try ownedHome()
            let root = home.appendingPathComponent(".Trash")
            let folder = root.appendingPathComponent("parent")
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: false)
            for i in 0..<100 { try Data().write(to: folder.appendingPathComponent("child-\(i)")) }
            let initial = TrashBrowseRequest(directory: "parent")
            let session = try TrashDirectorySession(home: home, request: initial)
            let next = try XCTUnwrap(session.page(initial).next)
            let replaced = changed == "root" ? root : folder
            try FileManager.default.moveItem(at: replaced, to: home.appendingPathComponent("retained-original"))
            try FileManager.default.createDirectory(at: replaced, withIntermediateDirectories: false)
            XCTAssertThrowsError(try session.page(next))
            XCTAssertTrue(session.isClosed)
        }
    }

    func testLeafSubstitutionDuringReadRefusesGeneration() throws {
        let home = try ownedHome(1)
        var replaced = false
        let session = try TrashDirectorySession(home: home, request: TrashBrowseRequest(), leafStat: { fd, name in
            let before = try TrashDirectorySession.statAt(fd, name)
            if !replaced {
                replaced = true
                let leaf = home.appendingPathComponent(".Trash/" + name)
                try FileManager.default.moveItem(at: leaf, to: home.appendingPathComponent("retained-leaf"))
                try Data("replacement".utf8).write(to: leaf)
            }
            return before
        })
        XCTAssertThrowsError(try session.page(TrashBrowseRequest()))
        XCTAssertTrue(session.isClosed)
    }

    func testCloseAndDeinitReleaseExactlyOwnedDescriptors() throws {
        let home = try ownedHome(100)
        var session: TrashDirectorySession? = try TrashDirectorySession(home: home, request: TrashBrowseRequest())
        let descriptors = try XCTUnwrap(session).descriptorSnapshot
        XCTAssertEqual(descriptors.count, 3)
        session?.close(); session?.close()
        XCTAssertEqual(session?.ownedDescriptorCount, 0)
        for fd in descriptors { errno = 0; XCTAssertEqual(fcntl(fd, F_GETFD), -1); XCTAssertEqual(errno, EBADF) }
        session = try TrashDirectorySession(home: home, request: TrashBrowseRequest())
        let finalDescriptors = try XCTUnwrap(session).descriptorSnapshot
        session = nil
        for fd in finalDescriptors { errno = 0; XCTAssertEqual(fcntl(fd, F_GETFD), -1); XCTAssertEqual(errno, EBADF) }
    }

    @MainActor func testRefreshExpiryCancelAndWorkflowDismissInvalidateSession() async throws {
        let home = try ownedHome(100)
        let browser = TrashBrowseController()
        func read(_ request: TrashBrowseRequest = TrashBrowseRequest()) async throws -> TrashPage {
            let capture = await browser.read(request, home: home)
            XCTAssertTrue(capture.complete)
            return try JSONDecoder().decode(TrashPage.self, from: capture.stdoutData)
        }
        let oldPage = try await read()
        let old = try XCTUnwrap(oldPage.next)
        let freshPage = try await read()
        let fresh = try XCTUnwrap(freshPage.next)
        XCTAssertNotEqual(old.session, fresh.session)
        browser.expire(try XCTUnwrap(old.session)); XCTAssertTrue(browser.hasOpenSession)
        browser.expire(try XCTUnwrap(fresh.session)); XCTAssertFalse(browser.hasOpenSession)
        await browser.close()
        let expired = await browser.read(fresh, home: home)
        XCTAssertFalse(expired.complete)
        _ = try await read()
        let replay = await browser.read(old, home: home)
        XCTAssertFalse(replay.complete)
        XCTAssertFalse(browser.hasOpenSession)
        let cancelled = Task { @MainActor in await browser.read(home: home) }
        cancelled.cancel()
        let cancelledResult = await cancelled.value
        XCTAssertTrue(cancelledResult.cancelled)
        XCTAssertFalse(browser.hasOpenSession)
        let workflow = TrashWorkflow()
        _ = await workflow.browser.read(home: home)
        XCTAssertTrue(workflow.browser.hasOpenSession)
        workflow.cancelBrowse()
        XCTAssertFalse(workflow.browser.hasOpenSession)
        await workflow.browser.close()
        XCTAssertEqual(workflow.browser.lifetime, .closed)
        var lateReads = 0
        await workflow.loadPage { lateReads += 1; return CommandExecution() }
        XCTAssertEqual(lateReads, 0, "dismissed queued work must not reopen a session")
        await browser.close()
    }

    @MainActor func testBrowseClosedBeforeAuthorizedMutationCallback() async throws {
        let home = try ownedHome(100)
        let workflow = TrashWorkflow()
        let read = await workflow.browser.read(home: home)
        let page = try JSONDecoder().decode(TrashPage.self, from: read.stdoutData)
        let manifest = TrashManifest(schema: "pantheon.trash-manifest.v1", uid: page.uid, root: page.root,
                                     containers: [], entries: Array(page.entries.prefix(1)))
        let ready = TrashReport(schema: "pantheon.trash-report.v1", status: "ready", preview: manifest,
                                attempted: 0, deleted: [], logical_bytes: 0, errors: [])
        let capture = CommandExecution(stdout: try JSONEncoder().encode(ready), exitCode: 0, childState: .exitConfirmed)
        await workflow.loadPreview { capture }
        workflow.prepareApproval(); XCTAssertNotNil(workflow.approval)
        XCTAssertTrue(workflow.browser.hasOpenSession)
        var callbacks = 0
        await workflow.apply { _ in
            callbacks += 1; XCTAssertFalse(workflow.browser.hasOpenSession)
            return CommandExecution(launchError: "inert fixture: no mutator executed")
        }
        XCTAssertEqual(callbacks, 1)
    }

    @MainActor func testSlowIOOwnerDoesNotBlockMainActorAndDismissFencesPublication() async throws {
        let home = try ownedHome(100)
        let entered = expectation(description: "serial I/O owner entered")
        let release = DispatchSemaphore(value: 0)
        let io = TrashBrowseIO(beforeRead: {
            entered.fulfill()
            XCTAssertEqual(release.wait(timeout: .now() + 5), .success,
                           "MainActor could not release the held I/O owner")
        })
        let browser = TrashBrowseController(io: io)
        let reading = Task { @MainActor in await browser.read(home: home) }
        await fulfillment(of: [entered], timeout: 2)
        let duplicate = await browser.read(TrashBrowseRequest(session: UUID(), sequence: 1), home: home)
        XCTAssertFalse(duplicate.complete)
        XCTAssertEqual(duplicate.launchError, "A browse page is already being read.")
        // This runs while the I/O owner is deliberately held, proving the GUI
        // actor can fence and enqueue close without waiting on the directory.
        browser.invalidate()
        XCTAssertEqual(browser.lifetime, .closing)
        release.signal()
        let result = await reading.value
        XCTAssertFalse(result.complete); XCTAssertTrue(result.cancelled)
        await browser.close()
        XCTAssertEqual(browser.lifetime, .closed)
    }

    @MainActor func testCancellationDuringIOFencesResultAndAcknowledgesClose() async throws {
        let home = try ownedHome(100)
        let entered = expectation(description: "owned read started")
        let release = DispatchSemaphore(value: 0)
        let browser = TrashBrowseController(io: TrashBrowseIO(beforeRead: {
            entered.fulfill(); _ = release.wait(timeout: .now() + 5)
        }))
        let reading = Task { @MainActor in await browser.read(home: home) }
        await fulfillment(of: [entered], timeout: 2)
        reading.cancel()
        release.signal()
        let result = await reading.value
        XCTAssertFalse(result.complete); XCTAssertTrue(result.cancelled)
        await browser.close()
        XCTAssertEqual(browser.lifetime, .closed)
    }

    func testCancelledQueuedReadCannotReopenOrCloseNewerSession() async throws {
        let home = try ownedHome(100)
        let owner = TrashBrowseIO()
        let current = await owner.read(TrashBrowseRequest(), home: home, fence: TrashBrowseReadFence())
        let page = try JSONDecoder().decode(TrashPage.self, from: current.stdoutData)
        let next = try XCTUnwrap(page.next)
        let stale = TrashBrowseReadFence(); stale.cancel()
        let refused = await owner.read(TrashBrowseRequest(), home: home, fence: stale)
        XCTAssertTrue(refused.cancelled); XCTAssertFalse(refused.complete)
        let resumed = await owner.read(next, home: home, fence: TrashBrowseReadFence())
        XCTAssertTrue(resumed.complete, "stale message closed the newer retained stream")
        let later = try JSONDecoder().decode(TrashPage.self, from: resumed.stdoutData)
        XCTAssertTrue(Set(page.entries.map(\.path)).isDisjoint(with: Set(later.entries.map(\.path))))
        await owner.close()
    }
}
