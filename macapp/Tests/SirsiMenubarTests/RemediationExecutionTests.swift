import XCTest
@testable import SirsiMenubar

final class RemediationExecutionTests: XCTestCase {
    private func finding(_ message: String = "original", fix: String = "sirsi scan") throws -> DiagFinding {
        let object: [String: Any] = ["check": "fixture", "severity": 2, "message": message,
                                     "detail": "owned evidence", "fix": fix, "fixKind": "instant"]
        return try JSONDecoder().decode(DiagFinding.self, from: JSONSerialization.data(withJSONObject: object))
    }

    func testApprovalBindsSemanticFindingExactArgumentsAndRepository() throws {
        let evidence = FindingObservationSnapshot(current: true, finding: try finding())
        let args = remediationArguments(["scan"])
        let repo = URL(fileURLWithPath: "/owned/original")
        let approval = DiagnosticRepairApproval(snapshot: evidence, args: args, directory: repo)
        XCTAssertTrue(approval.authorizes(snapshot: evidence, args: args, directory: repo))
        XCTAssertFalse(approval.authorizes(snapshot: .init(current: true, finding: try finding("changed")), args: args, directory: repo))
        XCTAssertFalse(approval.authorizes(snapshot: .init(current: true, finding: nil), args: args, directory: repo))
        XCTAssertFalse(approval.authorizes(snapshot: .init(current: false, finding: try finding()), args: args, directory: repo))
        XCTAssertFalse(approval.authorizes(snapshot: evidence, args: args + ["--confirm"], directory: repo))
        XCTAssertFalse(approval.authorizes(snapshot: evidence, args: args, directory: URL(fileURLWithPath: "/owned/other")))
        // Legacy registry approvals use this same evidence/argv/repo contract.
        let legacy = DiagnosticRepairApproval(snapshot: evidence, args: ["maat", "repair", "liveness-watch", "--confirm", "--json"], directory: repo)
        XCTAssertFalse(legacy.authorizes(snapshot: .init(current: true, finding: try finding(fix: "sirsi changed")), args: legacy.args, directory: repo))
    }

    @MainActor func testOnceLatchPrecedesAwaitAndNonJSONOrErrorNeverReruns() async {
        for receipt in [CommandExecution(stdout: Data("not JSON".utf8), exitCode: 0, childState: .exitConfirmed),
                        CommandExecution(stdout: Data("error output".utf8), exitCode: 7, childState: .exitConfirmed)] {
            let invocation = RemediationInvocation()
            var calls = 0
            let first = await invocation.executeOnce(args: ["owned"], authorize: { true }) { _ in
                calls += 1
                let nested = await invocation.executeOnce(args: ["owned"], authorize: { true }) { _ in
                    calls += 1
                    return receipt
                }
                XCTAssertNil(nested)
                return receipt
            }
            let again = await invocation.executeOnce(args: ["changed"], authorize: { true }) { _ in
                calls += 1
                return receipt
            }
            XCTAssertEqual(first, receipt)
            XCTAssertEqual(again, receipt)
            XCTAssertEqual(calls, 1)
            XCTAssertNil(first?.result)
        }
    }

    @MainActor func testRefusedApprovalDoesNotCaptureOrRetry() async {
        let invocation = RemediationInvocation()
        var calls = 0
        for _ in 0..<2 {
            let receipt = await invocation.executeOnce(args: ["owned"], authorize: { false }) { _ in
                calls += 1
                return CommandExecution()
            }
            XCTAssertEqual(receipt?.childState, .notStarted)
        }
        XCTAssertEqual(calls, 0)
    }

    func testVerificationRequiresThatSpecificSuccessfulRead() {
        let clear = Data(#"{"status":"green","findings":[]}"#.utf8)
        let failed = SirsiEngine.JSONCommandRead(data: clear, exitStatus: 1, timedOut: false, launchError: nil)
        XCTAssertEqual(remediationVerification(failed, check: "fixture"), .unverified)
        XCTAssertEqual(remediationVerification(.init(data: clear, exitStatus: nil, timedOut: true, launchError: nil), check: "fixture"), .unverified)
        XCTAssertEqual(remediationVerification(.init(data: clear, exitStatus: 0, timedOut: false, launchError: "failed"), check: "fixture"), .unverified)
        XCTAssertEqual(remediationVerification(.init(data: Data("malformed".utf8), exitStatus: 0, timedOut: false, launchError: nil), check: "fixture"), .unverified)
        XCTAssertEqual(remediationVerification(.init(data: clear, exitStatus: 0, timedOut: false, launchError: nil), check: "fixture"), .clear)
    }

    func testCompleteNonzeroTypedReportRemainsAvailableButPartialNeverDecodes() {
        let json = Data(#"{"summary":"findings","status":"error","errors":["finding"]}"#.utf8)
        let complete = CommandExecution(stdout: json, exitCode: 3, childState: .exitConfirmed)
        XCTAssertEqual(complete.result?.summary, "findings")
        XCTAssertEqual(complete.jsonRead.exitStatus, 3)
        XCTAssertEqual(complete.jsonRead.data, json)
        for partial in [CommandExecution(stdout: json, exitCode: 0, timedOut: true, childState: .exitConfirmed),
                        CommandExecution(stdout: json, exitCode: 0, overflow: true, childState: .exitConfirmed),
                        CommandExecution(stdout: json, cancelled: true, childState: .exitUnknown)] {
            XCTAssertNil(partial.result)
            XCTAssertTrue(partial.jsonRead.data.isEmpty)
            XCTAssertNil(partial.jsonRead.exitStatus)
        }
    }

    func testFollowupApprovalBindsReceiptCommandAndProject() {
        let receipt = CommandExecution(stdout: Data("evidence".utf8), exitCode: 0, childState: .exitConfirmed)
        let command = "sirsi clean --confirm"
        let args = remediationArguments(sirsiArgs(command))
        let directory = URL(fileURLWithPath: "/owned/repo")
        let approval = ResultActionApproval(evidence: receipt, command: command, args: args, directory: directory)
        XCTAssertTrue(approval.authorizes(evidence: receipt, commands: [command], directory: directory))
        XCTAssertFalse(approval.authorizes(evidence: CommandExecution(stdout: Data("changed".utf8), exitCode: 0, childState: .exitConfirmed), commands: [command], directory: directory))
        XCTAssertFalse(approval.authorizes(evidence: receipt, commands: [command + " changed"], directory: directory))
        XCTAssertFalse(approval.authorizes(evidence: receipt, commands: [command], directory: URL(fileURLWithPath: "/owned/other")))
    }

    func testReportFromRepositoryACannotOfferActionAfterSelectingB() {
        let report = CommandExecution(stdout: Data("A report".utf8), exitCode: 0, childState: .exitConfirmed)
        let repoA = URL(fileURLWithPath: "/owned/repo-A")
        let repoB = URL(fileURLWithPath: "/owned/repo-B")
        let command = "sirsi maat repair liveness-watch --confirm"
        // The report already exists BEFORE selection changes. No approval has
        // been formed yet, and the generic ResultView authorizer may be true.
        XCTAssertNotNil(ResultActionApproval.prepare(evidence: report, producingDirectory: repoA,
            command: command, currentDirectory: repoA))
        let offeredAfterSelectionChanged = ResultActionApproval.prepare(evidence: report,
            producingDirectory: repoA, command: command, currentDirectory: repoB)
        XCTAssertNil(offeredAfterSelectionChanged)
        XCTAssertNil(ResultActionApproval.prepare(evidence: report, producingDirectory: nil,
            command: command, currentDirectory: repoB))
    }

    func testDelayedOldClearCannotOverwriteNewActionFailedVerification() {
        var state = RemediationVerificationState()
        let oldRead = state.begin() // Old read is suspended, eventually returns clear.
        _ = state.begin() // New action starts before the old read returns.
        XCTAssertNil(state.verdict)
        let newRead = state.begin()
        let failed = SirsiEngine.JSONCommandRead(data: Data(), exitStatus: 1, timedOut: false, launchError: nil)
        state.publish(remediationVerification(failed, check: "fixture"), generation: newRead)
        XCTAssertEqual(state.verdict, .unverified)
        let oldClear = SirsiEngine.JSONCommandRead(data: Data(#"{"status":"green","findings":[]}"#.utf8),
            exitStatus: 0, timedOut: false, launchError: nil)
        // Deterministic completion ordering, without timers, processes or services.
        state.publish(remediationVerification(oldClear, check: "fixture"), generation: oldRead)
        XCTAssertEqual(state.verdict, .unverified)
        let latest = state.begin()
        state.publish(.clear, generation: latest)
        XCTAssertEqual(state.verdict, .clear)
        _ = state.begin() // Prior verdict clears synchronously at next action start.
        XCTAssertNil(state.verdict)
    }

    func testResultViewWiringCannotReplayMutatorOnRefreshOrDecodeFailure() throws {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let source = try String(contentsOf: root.appendingPathComponent("Sources/SirsiMenubar/Views.swift"), encoding: .utf8)
        let section = try XCTUnwrap(source.components(separatedBy: "struct ResultView: View {").last)
            .components(separatedBy: "// ── Threads")[0]
        XCTAssertFalse(section.contains("SirsiEngine.runResult("))
        XCTAssertFalse(section.contains("SirsiEngine.run(args:"))
        XCTAssertTrue(section.contains("guard !didLoad else { return }"))
        XCTAssertTrue(section.contains("invocation.executeOnce"))
        XCTAssertTrue(section.contains("Label(\"Recheck evidence\""))
        XCTAssertTrue(section.contains("let read = await engine.readHealthForVerification()"))
        XCTAssertFalse(section.contains("engine.health.first"))
        XCTAssertFalse(section.contains("await load()\n"))
    }

    func testOwnedCaptureCompleteNonzeroAndDeliveredStdin() async throws {
        try await withScript("IFS= read -r answer\nprintf '{\"summary\":\"%s\"}' \"$answer\"\nprintf 'owned stderr' >&2\nexit 3\n") { script in
            let receipt = await SirsiEngine.captureProcess(executable: "/bin/sh", args: [script.path], stdin: "inert\n", timeout: 2)
            XCTAssertTrue(receipt.complete)
            XCTAssertEqual(receipt.exitCode, 3)
            XCTAssertEqual(receipt.result?.summary, "inert")
            XCTAssertEqual(receipt.stderr, "owned stderr")
            XCTAssertEqual(receipt.jsonRead.exitStatus, 3)
        }
    }

    func testOwnedCaptureCancellationRetainsUnknownOutcome() async throws {
        try await withScript("trap '' TERM\n/bin/sleep 0.8\n") { script in
            let task = Task { await SirsiEngine.captureProcess(executable: "/bin/sh", args: [script.path], timeout: 2) }
            try await Task.sleep(nanoseconds: 50_000_000)
            task.cancel()
            let receipt = await task.value
            XCTAssertTrue(receipt.cancelled)
            XCTAssertFalse(receipt.complete)
            XCTAssertNil(receipt.result)
        }
    }

    // These are owned inert shell fixtures, never a bundled CLI, model or
    // service. All delayed descendants self-expire within one second. No
    // fixture signals a foreign PID, process group, or live application.
    private func withScript(_ body: String, run: (URL) async throws -> Void) async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: false)
        defer { try? FileManager.default.removeItem(at: dir) }
        let script = dir.appendingPathComponent("owned-inert.sh")
        try Data(body.utf8).write(to: script)
        do { try await run(script) }
        catch {
            try? await Task.sleep(nanoseconds: 1_100_000_000)
            throw error
        }
        // Retained writers / TERM-ignoring fixtures have a finite self-exit.
        // This wait is fixture cleanup, not a production process-tree claim.
        try? await Task.sleep(nanoseconds: 1_100_000_000)
    }

    func testOwnedCaptureSharedOverflowNeverDecodesValidJSONPrefix() async throws {
        try await withScript("printf '{\"summary\":\"prefix\"}'; printf 'abcdefghijklmnopqrstuvwxyz' >&2\n") { script in
            let receipt = await SirsiEngine.captureProcess(executable: "/bin/sh", args: [script.path], timeout: 2, outputLimit: 32)
            XCTAssertTrue(receipt.overflow)
            XCTAssertLessThanOrEqual(receipt.stdoutData.count + receipt.stderrData.count, 32)
            XCTAssertNil(receipt.result)
            XCTAssertTrue(receipt.jsonRead.data.isEmpty)
        }
    }

    func testOwnedCaptureUnconsumedStdinAndIgnoringTERMAreBounded() async throws {
        try await withScript("trap '' TERM\n/bin/sleep 0.8\n") { script in
            let start = ProcessInfo.processInfo.systemUptime
            let receipt = await SirsiEngine.captureProcess(executable: "/bin/sh", args: [script.path],
                stdin: String(repeating: "x", count: 64 * 1024), timeout: 0.1)
            XCTAssertLessThan(ProcessInfo.processInfo.systemUptime - start, 2)
            XCTAssertTrue(receipt.timedOut)
            XCTAssertFalse(receipt.complete)
            XCTAssertNil(receipt.result)
            XCTAssertTrue(receipt.jsonRead.data.isEmpty)
        }
    }

    func testOwnedCaptureRetainedPipeWriterCannotHoldEOFIndefinitely() async throws {
        try await withScript("(/bin/sleep 0.8) &\nprintf '{\"summary\":\"prefix\"}'\nexit 0\n") { script in
            let start = ProcessInfo.processInfo.systemUptime
            let receipt = await SirsiEngine.captureProcess(executable: "/bin/sh", args: [script.path], timeout: 0.1)
            XCTAssertLessThan(ProcessInfo.processInfo.systemUptime - start, 2)
            XCTAssertTrue(receipt.timedOut)
            XCTAssertFalse(receipt.complete)
            XCTAssertNil(receipt.result)
        }
    }

    func testOwnedCaptureApprovalIsRecheckedAtLaunch() async throws {
        try await withScript("printf 'must-not-launch'\n") { script in
            let receipt = await SirsiEngine.captureProcess(executable: "/bin/sh", args: [script.path], authorize: { false })
            XCTAssertEqual(receipt.childState, .notStarted)
            XCTAssertTrue(receipt.stdoutData.isEmpty)
        }
    }
}
