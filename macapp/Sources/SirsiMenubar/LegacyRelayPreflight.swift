import Foundation

// Shared by the privileged bridge and the unprivileged Installer reader. An
// uncooperative read-only probe cannot extend the app's submission deadline.
// No structured task-group join waits for that probe after timeout/cancel.
@MainActor enum LegacyRelayPreflight {
    @MainActor private final class Latch {
        let deadline: TimeInterval
        var continuation: CheckedContinuation<String, Error>?
        var work: Task<Void, Never>?
        var timer: Task<Void, Never>?
        init(deadline: TimeInterval) { self.deadline = deadline }
        func finish(_ result: Result<String, Error>) {
            guard let saved = continuation else { return }
            continuation = nil; work?.cancel(); timer?.cancel()
            if ProcessInfo.processInfo.systemUptime >= deadline {
                saved.resume(throwing: NSError(domain: "MaintenancePreflightDeadline", code: 1))
            } else { saved.resume(with: result) }
        }
    }
    static func run(deadline: TimeInterval, operation: @escaping @MainActor () async throws -> String) async throws -> String {
        guard deadline.isFinite, !Task.isCancelled, ProcessInfo.processInfo.systemUptime < deadline else { throw CancellationError() }
        let latch = Latch(deadline: deadline)
        return try await withTaskCancellationHandler(operation: {
            try await withCheckedThrowingContinuation { continuation in
                latch.continuation = continuation
                latch.work = Task { @MainActor in
                    guard !Task.isCancelled, ProcessInfo.processInfo.systemUptime < deadline else {
                        latch.finish(.failure(CancellationError())); return
                    }
                    do { latch.finish(.success(try await operation())) }
                    catch { latch.finish(.failure(error)) }
                }
                latch.timer = Task { @MainActor in
                    do { try await Task.sleep(nanoseconds: UInt64(max(0, deadline - ProcessInfo.processInfo.systemUptime) * 1_000_000_000)) }
                    catch { return }
                    latch.finish(.failure(NSError(domain: "MaintenancePreflightDeadline", code: 1)))
                }
            }
        }, onCancel: { Task { @MainActor in latch.finish(.failure(CancellationError())) } })
    }
}

@MainActor struct LegacyRelayInstallerCapture {
    typealias Invoke = @MainActor (String, [String], TimeInterval, @escaping @MainActor () -> Bool) async -> CommandExecution
    private let qualify: @MainActor () async throws -> String
    private let invoke: Invoke
    private let identityTimeout: TimeInterval
    private let overallTimeout: TimeInterval

    init(identityTimeout: TimeInterval = 8, overallTimeout: TimeInterval = 130,
         qualify: (@MainActor () async throws -> String)? = nil, invoke: Invoke? = nil) {
        self.identityTimeout = identityTimeout; self.overallTimeout = overallTimeout
        self.qualify = qualify ?? {
            let worker = Task.detached(priority: .userInitiated) { () throws -> String in
                guard !Task.isCancelled else { throw CancellationError() }
                if let reason = LegacyRelayAppHost.refusal() { throw NSError(domain: "MaintenanceInstallerIdentity", code: 1, userInfo: [NSLocalizedDescriptionKey: reason]) }
                guard !Task.isCancelled, let executable = Bundle.main.executableURL,
                      let release = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String, !release.isEmpty else {
                    throw NSError(domain: "MaintenanceInstallerAppMetadata", code: 1)
                }
                let cli = executable.deletingLastPathComponent().appendingPathComponent("sirsi")
                guard !Task.isCancelled, SirsiEngine.isTrustedBundledCLI(cli) else {
                    throw NSError(domain: "MaintenanceInstallerAppEngine", code: 1)
                }
                return cli.path
            }
            return try await withTaskCancellationHandler(operation: { try await worker.value }, onCancel: { worker.cancel() })
        }
        self.invoke = invoke ?? { path, args, remaining, authorize in
            await SirsiEngine.captureProcess(executable: path, args: args, directory: URL(fileURLWithPath: NSHomeDirectory()),
                timeout: remaining, outputLimit: 16 * 1024,
                environment: ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": NSHomeDirectory(), "TMPDIR": NSTemporaryDirectory(), "LC_ALL": "C"], authorize: authorize)
        }
    }

    func capture(args: [String], authorize: @escaping @MainActor () -> Bool) async -> CommandExecution {
        let deadline = ProcessInfo.processInfo.systemUptime + overallTimeout
        let prepare = args == ["launchd", "maintenance-installer"]
        let verify = args.count == 4 && Array(args.prefix(3)) == ["launchd", "maintenance-installer", "--verify-selection"] &&
            !args[3].isEmpty && args[3].count <= 16384 && args[3].utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
        let rollback = args.count == 4 && args[0] == "update" && args[1] == "--rollback-version" &&
            AppRollbackSelection(release: args[2]) != nil && args[3] == "--confirm-rollback"
        let recheck = args == ["update", "--reconcile-app-attempt"]
        guard (prepare || verify || rollback || recheck), overallTimeout.isFinite, overallTimeout > 0, overallTimeout <= 130,
              identityTimeout.isFinite, identityTimeout > 0, identityTimeout <= 8,
              !Task.isCancelled, authorize() else { return CommandExecution(launchError: "Installer or rollback request canceled, expired or unrecognized before preflight.", cancelled: Task.isCancelled) }
        let path: String
        do {
            path = try await LegacyRelayPreflight.run(deadline: min(deadline, ProcessInfo.processInfo.systemUptime + identityTimeout), operation: qualify)
        } catch {
            return CommandExecution(launchError: "Signed app/CLI identity preflight canceled, expired or unverified; no command submitted.", cancelled: Task.isCancelled)
        }
        guard !Task.isCancelled, authorize(), ProcessInfo.processInfo.systemUptime < deadline else {
            return CommandExecution(launchError: "Installer approval changed or expired before submission.", cancelled: Task.isCancelled)
        }
        let result=await invoke(path, args, deadline - ProcessInfo.processInfo.systemUptime, {
            !Task.isCancelled && authorize() && ProcessInfo.processInfo.systemUptime < deadline
        })
        guard !Task.isCancelled,authorize(),ProcessInfo.processInfo.systemUptime<deadline else{
            return CommandExecution(stdout:result.stdoutData,stderr:result.stderrData,exitCode:result.exitCode,
                cancelled:Task.isCancelled,captureError:"Installer observation canceled, superseded or expired; no verified selection.",childState:result.childState)
        }
        return result
    }
}
