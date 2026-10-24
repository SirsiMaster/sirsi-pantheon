import Foundation
import Combine
import Darwin

// Semantic evidence only; a renewed identical observation may remain valid,
// but unknown/stale/error observations never authorize execution.
struct FindingObservationSnapshot: Equatable {
    let current: Bool
    let check: String?
    let severity: Int?
    let message: String?
    let detail: String?
    let trend: Bool?
    let fix: String?
    let fixKind: String?
    let resolution: String?

    init(current: Bool, finding: DiagFinding?) {
        self.current = current
        check = finding?.check; severity = finding?.severity
        message = finding?.message; detail = finding?.detail; trend = finding?.trend
        fix = finding?.fix; fixKind = finding?.fixKind; resolution = finding?.resolution
    }
}

struct DiagnosticRepairApproval {
    let snapshot: FindingObservationSnapshot
    let args: [String]
    let directory: URL

    func authorizes(snapshot now: FindingObservationSnapshot, args nowArgs: [String], directory nowDirectory: URL) -> Bool {
        snapshot.current && now.current && snapshot.check != nil && (snapshot.severity ?? 0) > 1 && snapshot == now &&
            !args.isEmpty && args == nowArgs && directory == nowDirectory
    }
}

func remediationArguments(_ args: [String]) -> [String] {
    args.contains("--json") ? args : args + ["--json"]
}

enum OwnedCommandState: Equatable { case notStarted, exitConfirmed, exitUnknown }

struct ResultActionApproval {
    let evidence: CommandExecution
    let command: String
    let args: [String]
    let directory: URL

    // Bind offered actions to the report's producing scope BEFORE consent.
    // Selecting another project cannot launder an old report into new authority.
    static func prepare(evidence: CommandExecution?, producingDirectory: URL?,
                        command: String, currentDirectory: URL) -> ResultActionApproval? {
        guard let evidence, evidence.complete, let producingDirectory,
              producingDirectory == currentDirectory else { return nil }
        return ResultActionApproval(evidence: evidence, command: command,
            args: remediationArguments(sirsiArgs(command)), directory: producingDirectory)
    }

    func authorizes(evidence now: CommandExecution?, commands: [String], directory nowDirectory: URL) -> Bool {
        evidence.complete && now == evidence && commands.contains(command) &&
            args == remediationArguments(sirsiArgs(command)) && directory == nowDirectory
    }
}

struct CommandExecution: Equatable {
    let stdoutData: Data
    let stderrData: Data
    let exitCode: Int32?
    let launchError: String?
    let timedOut: Bool
    let cancelled: Bool
    let overflow: Bool
    let captureError: String?
    let childState: OwnedCommandState
    let termSent: Bool

    init(stdout: Data = Data(), stderr: Data = Data(), exitCode: Int32? = nil,
         launchError: String? = nil, timedOut: Bool = false, cancelled: Bool = false,
         overflow: Bool = false, captureError: String? = nil,
         childState: OwnedCommandState = .notStarted, termSent: Bool = false) {
        stdoutData = stdout; stderrData = stderr; self.exitCode = exitCode
        self.launchError = launchError; self.timedOut = timedOut; self.cancelled = cancelled
        self.overflow = overflow; self.captureError = captureError
        self.childState = childState; self.termSent = termSent
    }

    var stdout: String { String(decoding: stdoutData, as: UTF8.self) }
    var stderr: String { String(decoding: stderrData, as: UTF8.self) }
    var complete: Bool {
        childState == .exitConfirmed && exitCode != nil && launchError == nil &&
            !timedOut && !cancelled && !overflow && captureError == nil
    }
    // Complete nonzero typed reports are still reports, not decode failures.
    // Only the consumer can interpret their documented status semantics.
    var result: CommandResult? {
        guard complete, let text = String(data: stdoutData, encoding: .utf8),
              let start = text.firstIndex(of: "{") else { return nil }
        return try? JSONDecoder().decode(CommandResult.self, from: Data(text[start...].utf8))
    }
    var issue: String? {
        if let launchError { return launchError }
        if cancelled { return "Cancelled; execution outcome is not confirmed." }
        if overflow { return "Output exceeded the shared 4 MiB capture budget; execution outcome is not confirmed." }
        if timedOut { return "Capture deadline expired; execution outcome is not confirmed." }
        if let captureError { return captureError }
        if childState == .exitUnknown { return "Owned child exit is unknown." }
        return nil
    }
    var displayText: String {
        var text = issue.map { $0 + "\n\n" } ?? ""
        text += "STDOUT:\n\(stdout)\n\nSTDERR:\n\(stderr)\n\nExit status: \(exitCode.map(String.init) ?? "unavailable")"
        if childState == .exitUnknown {
            text += "\nOwned child exit remains unknown after bounded cleanup. Do not replay this operation; inspect current state."
        }
        return text
    }
    // Preserve JSONCommandRead's existing initializer/consumer contract. Partial
    // data stays in this receipt, never in a success-decodable typed read.
    var jsonRead: SirsiEngine.JSONCommandRead {
        SirsiEngine.JSONCommandRead(data: complete ? stdoutData : Data(),
            exitStatus: complete ? exitCode : nil, timedOut: timedOut,
            launchError: issue)
    }
}

// One object per ResultView identity. Set the latch before validation or await:
// .task, project changes and retry/refresh cannot silently replay the command.
final class RemediationInvocation {
    private var started = false
    private(set) var receipt: CommandExecution?

    @MainActor func executeOnce(args: [String], authorize: () -> Bool,
                               capture: ([String]) async -> CommandExecution) async -> CommandExecution? {
        guard !started else { return receipt }
        started = true
        guard authorize() else {
            let refused = CommandExecution(launchError: "The finding, command or project changed after approval. Recheck and approve again. No command launched.")
            receipt = refused
            return refused
        }
        let captured = await capture(args)
        receipt = captured
        return captured
    }
}

enum RemediationVerification: Equatable {
    case unverified
    case clear
    case present(Int)
}

struct RemediationVerificationState {
    private var generation = UUID()
    private(set) var verdict: RemediationVerification?

    // Starting an action or another read invalidates ALL older completions and
    // removes the previous verdict before the first suspension.
    mutating func begin() -> UUID {
        generation = UUID()
        verdict = nil
        return generation
    }

    mutating func publish(_ verdict: RemediationVerification, generation token: UUID) {
        guard token == generation else { return }
        self.verdict = verdict
    }
}

func remediationVerification(_ read: SirsiEngine.JSONCommandRead, check: String) -> RemediationVerification {
    guard read.exitStatus == 0, !read.timedOut, read.launchError == nil,
          let report = try? JSONDecoder().decode(DiagReport.self, from: read.data),
          let status = report.status, ["green", "amber", "red"].contains(status) else { return .unverified }
    guard let finding = report.findings.first(where: { $0.check == check }), finding.severity > 1 else { return .clear }
    return .present(finding.severity)
}

private final class CommandCaptureCancellation: @unchecked Sendable {
    private let lock = NSLock()
    private var value = false
    func cancel() { lock.lock(); value = true; lock.unlock() }
    var cancelled: Bool { lock.lock(); defer { lock.unlock() }; return value }
}

extension SirsiEngine {
    func diagnosticSnapshot(check: String) -> FindingObservationSnapshot {
        FindingObservationSnapshot(current: healthIsCurrent,
            finding: health.first(where: { $0.check == check }))
    }

    func readHealthForVerification() async -> JSONCommandRead {
        let read = await Self.runJSONRead(args: ["diagnose", "--json"])
        acceptHealthRead(read)
        return read // The caller verifies THIS read, not mutable shared health.
    }

    nonisolated static func runCaptured(args: [String], stdin: String? = nil, directory: URL? = nil,
                                       timeout: TimeInterval = 30,
                                       authorize: @escaping @MainActor () -> Bool = { true }) async -> CommandExecution {
        guard let binary = sirsiBinary() else {
            return CommandExecution(launchError: "Pantheon's bundled CLI is missing, linked, or not executable.")
        }
        return await captureProcess(executable: binary, args: args,
            directory: directory ?? workingDirectory(for: args), stdin: stdin, timeout: timeout, authorize: authorize)
    }

    // Shared aggregate output budget, not 4 MiB per stream. outputLimit exists
    // only for small inert fixtures; it cannot raise the production ceiling.
    nonisolated static func captureProcess(executable: String, args: [String], directory: URL? = nil,
        stdin: String? = nil, timeout: TimeInterval = 30, outputLimit: Int = 4 * 1024 * 1024,
        authorize: @escaping @MainActor () -> Bool = { true }) async -> CommandExecution {
        guard timeout.isFinite, timeout > 0, outputLimit > 0, outputLimit <= 4 * 1024 * 1024,
              (stdin?.utf8.count ?? 0) <= 64 * 1024 else {
            return CommandExecution(launchError: "Invalid bounded capture request.")
        }
        let cancellation = CommandCaptureCancellation()
        return await withTaskCancellationHandler(operation: {
            let process = Process()
            process.executableURL = URL(fileURLWithPath: executable)
            process.arguments = args
            process.currentDirectoryURL = directory
            let output = Pipe(), errors = Pipe(), input = Pipe()
            process.standardOutput = output; process.standardError = errors
            if stdin == nil { process.standardInput = FileHandle.nullDevice }
            else { process.standardInput = input }
            defer {
                for handle in [output.fileHandleForReading, output.fileHandleForWriting,
                               errors.fileHandleForReading, errors.fileHandleForWriting,
                               input.fileHandleForReading, input.fileHandleForWriting] { try? handle.close() }
            }
            for handle in [output.fileHandleForReading, errors.fileHandleForReading, input.fileHandleForWriting] {
                let fd = handle.fileDescriptor
                let flags = fcntl(fd, F_GETFL)
                guard flags != -1, fcntl(fd, F_SETFL, flags | O_NONBLOCK) != -1 else {
                    return CommandExecution(launchError: "Unable to configure nonblocking capture.")
                }
            }
            guard fcntl(input.fileHandleForWriting.fileDescriptor, F_SETNOSIGPIPE, 1) != -1 else {
                return CommandExecution(launchError: "Unable to configure bounded input.")
            }
            let deadline = ProcessInfo.processInfo.systemUptime + timeout
            do {
                // Validation and launch are one main-actor turn: finding/project
                // changes cannot interleave between this check and Process.run.
                try await MainActor.run {
                    guard !cancellation.cancelled, ProcessInfo.processInfo.systemUptime < deadline, authorize() else {
                        throw NSError(domain: "PantheonCapture", code: 1,
                            userInfo: [NSLocalizedDescriptionKey: "Execution cancelled or approval changed before launch."])
                    }
                    try process.run()
                }
            } catch {
                return CommandExecution(launchError: error.localizedDescription,
                    timedOut: ProcessInfo.processInfo.systemUptime >= deadline, cancelled: cancellation.cancelled)
            }
            try? output.fileHandleForWriting.close()
            try? errors.fileHandleForWriting.close()
            try? input.fileHandleForReading.close()
            return await withCheckedContinuation { continuation in
                DispatchQueue.global(qos: .userInitiated).async {
                    var stdout = Data(), stderr = Data()
                    var retainedBytes = 0
                    var outEOF = false, errEOF = false, inputClosed = stdin == nil
                    let inputData = Data((stdin ?? "").utf8)
                    var inputOffset = 0, timedOut = false, cancelled = false, overflow = false
                    var captureError: String?
                    func drain(_ handle: FileHandle, into data: inout Data, eof: inout Bool) {
                        guard !eof else { return }
                        var buffer = [UInt8](repeating: 0, count: 8192)
                        for _ in 0..<4 {
                            if ProcessInfo.processInfo.systemUptime >= deadline { return }
                            let count = buffer.withUnsafeMutableBytes { Darwin.read(handle.fileDescriptor, $0.baseAddress, $0.count) }
                            if count > 0 {
                                let remaining = outputLimit - retainedBytes
                                let accepted = min(count, remaining)
                                data.append(contentsOf: buffer.prefix(accepted))
                                retainedBytes += accepted
                                if count > remaining { overflow = true; return }
                            } else {
                                if count == 0 { eof = true }
                                else if errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR { captureError = "Output capture failed; outcome is unknown." }
                                return
                            }
                        }
                    }
                    while true {
                        if cancellation.cancelled { cancelled = true; break }
                        if ProcessInfo.processInfo.systemUptime >= deadline { timedOut = true; break }
                        drain(output.fileHandleForReading, into: &stdout, eof: &outEOF)
                        if !overflow && captureError == nil { drain(errors.fileHandleForReading, into: &stderr, eof: &errEOF) }
                        if overflow || captureError != nil { break }
                        if !inputClosed {
                            if inputOffset < inputData.count {
                                let count = inputData.withUnsafeBytes {
                                    Darwin.write(input.fileHandleForWriting.fileDescriptor,
                                        $0.baseAddress!.advanced(by: inputOffset), min(8192, inputData.count-inputOffset))
                                }
                                if count > 0 { inputOffset += count }
                                else if count < 0 && errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR {
                                    captureError = "Command input was not fully delivered; outcome is unknown."
                                    break
                                }
                            }
                            if inputOffset == inputData.count { try? input.fileHandleForWriting.close(); inputClosed = true }
                        }
                        if !process.isRunning && outEOF && errEOF { break }
                        Thread.sleep(forTimeInterval: min(0.005, max(0, deadline-ProcessInfo.processInfo.systemUptime)))
                    }
                    let incomplete = timedOut || cancelled || overflow || captureError != nil
                    var termSent = false
                    if incomplete && process.isRunning {
                        termSent = true
                        process.terminate() // Only Foundation's owned child, never a process group.
                        let cleanupDeadline = ProcessInfo.processInfo.systemUptime + 0.25
                        while process.isRunning && ProcessInfo.processInfo.systemUptime < cleanupDeadline {
                            Thread.sleep(forTimeInterval: 0.005)
                        }
                    }
                    let exited = !process.isRunning
                    continuation.resume(returning: CommandExecution(stdout: stdout, stderr: stderr,
                        exitCode: exited ? process.terminationStatus : nil,
                        timedOut: timedOut, cancelled: cancelled, overflow: overflow, captureError: captureError,
                        childState: exited ? .exitConfirmed : .exitUnknown, termSent: termSent))
                }
            }
        }, onCancel: { cancellation.cancel() })
    }
}
