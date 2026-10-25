import Foundation
import CryptoKit
import Darwin
import Security

// One public, closed request to one protected executable. The OS owns the
// password dialog; no app environment, actor, session or credential is forwarded.
@MainActor struct LegacyRelayPrivilegedBridge {
    static let engine = "/usr/local/libexec/sirsi-pantheon/sirsi"
    static let declined = "PANTHEON_MAINTENANCE_AUTH_DECLINED_V1"
    static let script = """
    on run argv
        if (count of argv) is not 1 then error "One bounded maintenance request required"
        set requestData to item 1 of argv
        set commandText to "cd / && exec /usr/bin/env -i HOME=/var/root PATH=/usr/bin:/bin:/usr/sbin:/sbin LC_ALL=C /usr/local/libexec/sirsi-pantheon/sirsi launchd legacy-maintenance-app " & quoted form of requestData
        try
            with timeout of 120 seconds
                return do shell script commandText with prompt "Pantheon: authorize only the reviewed legacy relay maintenance operation." with administrator privileges
            end timeout
        on error messageText number errorNumber
            if errorNumber is -128 then return "PANTHEON_MAINTENANCE_AUTH_DECLINED_V1"
            error messageText number errorNumber
        end try
    end run
    """
    typealias Invoke = @MainActor ([String], @escaping @MainActor () -> Bool) async -> CommandExecution
    private let qualify: @MainActor () throws -> String
    private let identity: @MainActor () -> String?
    private let invoke: Invoke

    init(qualify: @escaping @MainActor () throws -> String = { try protectedEngineHash() },
         identity: @escaping @MainActor () -> String? = { LegacyRelayAppHost.refusal() },
         invoke: Invoke? = nil) {
        self.qualify = qualify; self.identity = identity
        self.invoke = invoke ?? { args, authorize in
            await SirsiEngine.captureProcess(executable: "/usr/bin/osascript", args: args,
                directory: URL(fileURLWithPath: "/"), timeout: 125, outputLimit: 48 * 1024,
                environment: ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": NSHomeDirectory(), "LC_ALL": "C"],
                authorize: authorize)
        }
    }

    func capture(args: [String], stdin: String?, authorize: @escaping @MainActor () -> Bool) async -> CommandExecution {
        guard !Task.isCancelled, authorize() else { return CommandExecution(launchError: "Approval changed before authorization.", cancelled: Task.isCancelled) }
        if let reason = identity() { return CommandExecution(launchError: reason) }
        let hash: String, encoded: String
        do {
            hash = try qualify()
            encoded = try Self.request(args: args, stdin: stdin, engineHash: hash)
        } catch {
            return CommandExecution(launchError: "Maintenance is unavailable: the protected installed engine or request is unverified. Use Pantheon's normal signed PKG installation, then return here and review again. No app-bundle or PATH binary will run as root.")
        }
        let raw = await invoke(["-e", Self.script, encoded], {
            guard !Task.isCancelled, authorize(), (try? self.qualify()) == hash else { return false }
            return true
        })
        if raw.complete, raw.exitCode == 0, raw.stdout.trimmingCharacters(in: .whitespacesAndNewlines) == Self.declined {
            return CommandExecution(launchError: "Administrator authorization was declined. No maintenance operation was submitted.")
        }
        struct Reply: Decodable {
            let schema: Int; let operation: String; let ok: Bool; let bodyBase64: String; let error: String
            enum CodingKeys: String, CodingKey { case schema, operation, ok, error; case bodyBase64 = "body_base64" }
        }
        if raw.complete, raw.exitCode == 0, raw.stdoutData.count <= 48 * 1024,
           let frame = try? JSONDecoder().decode(Reply.self, from: raw.stdoutData),
           frame.schema == 1, frame.operation == args[2], frame.error.utf8.count <= 4800,
           let body = Data(base64Encoded: frame.bodyBase64), body.count <= 32 * 1024 {
            return CommandExecution(stdout: body, stderr: Data(frame.error.utf8), exitCode: frame.ok ? 0 : 1, childState: .exitConfirmed)
        }
        if raw.childState == .notStarted {
            return CommandExecution(launchError: raw.issue ?? "The authorization bridge did not start.")
        }
        // Terminating our osascript is not evidence that its privileged child
        // stopped. Retain unknown outcome and consume consent; never replay.
        return CommandExecution(stdout: raw.stdoutData, stderr: raw.stderrData, exitCode: raw.exitCode,
            timedOut: raw.timedOut, cancelled: raw.cancelled, overflow: raw.overflow,
            captureError: "Maintenance outcome is unknown. Read the current transaction, then explicitly resume, abort or restore that transaction. Do not confirm again.",
            childState: raw.childState, termSent: raw.termSent)
    }

    static func request(args: [String], stdin: String?, engineHash: String) throws -> String {
        func invalid() -> NSError { NSError(domain: "LegacyRelayBridge", code: 1) }
        guard args.count >= 3, Array(args.prefix(2)) == ["launchd", "legacy-maintenance"],
              engineHash.count == 64, engineHash.utf8.allSatisfy({ (48...57).contains($0) || (97...102).contains($0) }) else { throw invalid() }
        var value = ["profile": "root-relay-legacy-maintenance-v1", "operation": args[2], "engine_sha256": engineHash,
                     "legacy": "", "transaction": "", "plan_sha256": "", "artifact": "", "restore_json": ""]
        switch args[2] {
        case "preview":
            guard args.count == 5, args[3] == "--legacy-payload", stdin == nil else { throw invalid() }; value["legacy"] = args[4]
        case "confirm":
            guard args.count == 8, args[3] == "--legacy-payload", args[5] == "--expected-plan-sha256", args[7] == "--confirm-maintenance", stdin == nil else { throw invalid() }
            value["legacy"] = args[4]; value["plan_sha256"] = args[6]
        case "status":
            guard args.count == 3, stdin == nil else { throw invalid() }
        case "recover", "abort":
            guard args.count == 8, args[3] == "--transaction", args[5] == "--expected-plan-sha256", args[7] == "--confirm-maintenance", stdin == nil else { throw invalid() }
            value["transaction"] = args[4]; value["plan_sha256"] = args[6]
        case "restore-preview":
            guard args.count == 9, args[3] == "--transaction", args[5] == "--expected-plan-sha256", args[7] == "--artifact", stdin == nil else { throw invalid() }
            value["transaction"] = args[4]; value["plan_sha256"] = args[6]; value["artifact"] = args[8]
        case "restore-confirm":
            guard args.count == 6, args[3] == "--expected-restore-sha256", args[5] == "--confirm-maintenance",
                  let stdin, stdin.utf8.count <= 8192,
                  let data = stdin.data(using: .utf8), let plan = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let transaction = plan["transaction"] as? String, let original = plan["maintenance_sha256"] as? String,
                  plan["sha256"] as? String == args[4] else { throw invalid() }
            value["transaction"] = transaction; value["plan_sha256"] = original; value["restore_json"] = stdin
        default: throw invalid()
        }
        func hex(_ key: String, count: Int) -> Bool {
            guard let text=value[key] else {return false}
            return text.count == count && text.utf8.allSatisfy { (48...57).contains($0) || (97...102).contains($0) }
        }
        for key in ["legacy","artifact"] {
            if let path=value[key],!path.isEmpty {
                guard path.utf8.count <= 4096,path.hasPrefix("/"),URL(fileURLWithPath:path).standardizedFileURL.path == path,!path.utf8.contains(0) else {throw invalid()}
            }
        }
        if value["transaction"] != "" {guard hex("transaction",count:32) else {throw invalid()}}
        if value["plan_sha256"] != "" {guard hex("plan_sha256",count:64) else {throw invalid()}}
        let data = try JSONSerialization.data(withJSONObject: value, options: [.sortedKeys])
        guard data.count <= 24 * 1024 else { throw invalid() }
        return data.base64EncodedString()
    }

    // The native entry rechecks identity after OS authorization. This retained
    // descriptor probe prevents elevating a user-writable substitute beforehand.
    static func protectedEngineHash() throws -> String {
        func refused() -> NSError { NSError(domain: "ProtectedMaintenanceEngine", code: 1) }
        var parent = Darwin.open("/", O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard parent >= 0 else { throw refused() }
        defer { Darwin.close(parent) }
        for part in engine.split(separator: "/").dropLast() {
            let next = Darwin.openat(parent, String(part), O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
            guard next >= 0 else { throw refused() }
            Darwin.close(parent); parent = next
            var st = stat()
            guard fstat(parent, &st) == 0, st.st_uid == 0, st.st_mode & 0o022 == 0, noACL(parent) else { throw refused() }
        }
        let fd = Darwin.openat(parent, "sirsi", O_RDONLY | O_NONBLOCK | O_NOFOLLOW | O_CLOEXEC)
        guard fd >= 0 else { throw refused() }
        defer { Darwin.close(fd) }
        var before = stat()
        guard fstat(fd, &before) == 0, before.st_uid == 0, before.st_nlink == 1,
              before.st_mode & S_IFMT == S_IFREG, before.st_mode & 0o022 == 0, before.st_mode & 0o111 != 0,
              before.st_size > 0, before.st_size <= 256 * 1024 * 1024, noACL(fd) else { throw refused() }
        var code: SecStaticCode?, requirement: SecRequirement?
        let rule = "anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and identifier \"sirsi\""
        guard SecStaticCodeCreateWithPath(URL(fileURLWithPath: engine) as CFURL, SecCSFlags(rawValue: 0), &code) == errSecSuccess, let code,
              SecRequirementCreateWithString(rule as CFString, SecCSFlags(rawValue: 0), &requirement) == errSecSuccess, let requirement,
              SecStaticCodeCheckValidity(code, SecCSFlags(rawValue: kSecCSStrictValidate), requirement) == errSecSuccess else { throw refused() }
        var hasher = SHA256(), buffer = [UInt8](repeating: 0, count: 64 * 1024), total = 0
        while true {
            let count = buffer.withUnsafeMutableBytes { Darwin.read(fd, $0.baseAddress!, $0.count) }
            if count < 0 { if errno == EINTR { continue }; throw refused() }
            if count == 0 { break }
            total += count; guard total <= 256 * 1024 * 1024 else { throw refused() }
            hasher.update(data: Data(buffer.prefix(count)))
        }
        var after = stat(), named = stat()
        guard Int64(total) == before.st_size, fstat(fd, &after) == 0, fstatat(parent, "sirsi", &named, AT_SYMLINK_NOFOLLOW) == 0,
              before.st_dev == after.st_dev, before.st_ino == after.st_ino, before.st_size == after.st_size,
              before.st_mtimespec.tv_sec == after.st_mtimespec.tv_sec, before.st_mtimespec.tv_nsec == after.st_mtimespec.tv_nsec,
              before.st_mode == after.st_mode, before.st_uid == after.st_uid, after.st_nlink == 1,
              named.st_dev == after.st_dev, named.st_ino == after.st_ino else { throw refused() }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    private struct ACLResult { var length: UInt32 = 0; var reference = attrreference_t() }
    private static func noACL(_ fd: Int32) -> Bool {
        var attrs = attrlist(); attrs.bitmapcount = UInt16(ATTR_BIT_MAP_COUNT); attrs.commonattr = UInt32(ATTR_CMN_EXTENDED_SECURITY)
        var result = ACLResult()
        let status = withUnsafeMutablePointer(to: &attrs) { attributes in
            withUnsafeMutableBytes(of: &result) { bytes in Darwin.fgetattrlist(fd, attributes, bytes.baseAddress!, bytes.count, UInt32(FSOPT_REPORT_FULLSIZE)) }
        }
        return status == 0 && result.length == MemoryLayout<ACLResult>.size && result.reference.attr_length == 0
    }
}
