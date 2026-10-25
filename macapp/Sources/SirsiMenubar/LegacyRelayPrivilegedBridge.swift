import Foundation
import CryptoKit
import Darwin
import Security

// One public, closed request to one protected executable. The OS owns the
// password dialog; no app environment, actor, session or credential is forwarded.
@MainActor struct LegacyRelayPrivilegedBridge {
    nonisolated static let engine = "/usr/local/libexec/sirsi-pantheon/sirsi"
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
    struct Capability: Decodable, Equatable {
        let schema: Int; let proto: String; let release: String; let implementation: String;let native:Bool
        enum CodingKeys:String,CodingKey {case schema,release,implementation,native;case proto="protocol"}
        var supported:Bool {schema==1 && native && proto=="root-relay-local-app-v1" && implementation.count==64 &&
            implementation.utf8.allSatisfy{(48...57).contains($0)||(97...102).contains($0)}}
    }
    private let qualify: @MainActor () async throws -> String
    private let compatibility: @MainActor () async throws -> Void
    private let identity: @MainActor () async -> String?
    private let invoke: Invoke
    private let preflightTimeout:TimeInterval

    init(qualify: (@MainActor () async throws -> String)? = nil,
         identity: (@MainActor () async -> String?)? = nil,
         compatibility: (@MainActor () async throws -> Void)? = nil,
         preflightTimeout:TimeInterval = 30,
         invoke: Invoke? = nil) {
        self.qualify = qualify ?? {try await Self.defaultQualification()}
        self.identity = identity ?? {await Task.detached(priority:.userInitiated){LegacyRelayAppHost.refusal()}.value}
        self.compatibility = compatibility ?? {try await Self.requireCompatibleInstalledPair()}
        self.preflightTimeout=preflightTimeout
        self.invoke = invoke ?? { args, authorize in
            await SirsiEngine.captureProcess(executable: "/usr/bin/osascript", args: args,
                directory: URL(fileURLWithPath: "/"), timeout: 125, outputLimit: 48 * 1024,
                environment: ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": NSHomeDirectory(), "LC_ALL": "C"],
                authorize: authorize)
        }
    }

    func capture(args: [String], stdin: String?, authorize: @escaping @MainActor () -> Bool) async -> CommandExecution {
        let deadline=ProcessInfo.processInfo.systemUptime+preflightTimeout
        guard preflightTimeout.isFinite,preflightTimeout>0,preflightTimeout<=30 else{return CommandExecution(launchError:"Invalid preflight bound")}
        guard !Task.isCancelled, authorize() else { return CommandExecution(launchError: "Approval changed before authorization.", cancelled: Task.isCancelled) }
        do {
            _ = try await boundedProbe(deadline:deadline){
                if let reason=await self.identity(){throw NSError(domain:"MaintenanceAppIdentity",code:1,userInfo:[NSLocalizedDescriptionKey:reason])}
                return "signed-app"
            }
        }catch{return CommandExecution(launchError:"The app identity preflight was canceled, expired or unverified. No authorization request was submitted.")}
        let hash: String, encoded: String
        do {
            hash = try await boundedProbe(deadline:deadline){try await self.qualify()}
            _ = try await boundedProbe(deadline:deadline){try await self.compatibility();return "compatible"}
            encoded = try Self.request(args: args, stdin: stdin, engineHash: hash)
        } catch {
            return CommandExecution(launchError: "Maintenance is unavailable: the protected installed engine or request is unverified. Use Pantheon's normal signed PKG installation, then return here and review again. No app-bundle or PATH binary will run as root.")
        }
        guard (try? await boundedProbe(deadline:deadline){try await self.qualify()}) == hash else {return CommandExecution(launchError:"Protected engine changed or qualification expired. Recheck before another review.")}
        guard !Task.isCancelled,authorize(),ProcessInfo.processInfo.systemUptime<deadline else{return CommandExecution(launchError:"Approval canceled, consumed or expired before submission",cancelled:Task.isCancelled)}
        let raw = await invoke(["-e", Self.script, encoded], {
            guard !Task.isCancelled, authorize(),ProcessInfo.processInfo.systemUptime<deadline else { return false }
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

    private func boundedProbe(deadline:TimeInterval,operation:@escaping @MainActor()async throws->String)async throws->String{
        try await LegacyRelayPreflight.run(deadline:deadline,operation:operation)
    }
    private static func defaultQualification()async throws->String{
        func hash()async throws->String{
            let worker=Task.detached(priority:.userInitiated){try Self.protectedEngineHash(deadline:ProcessInfo.processInfo.systemUptime+8)}
            return try await withTaskCancellationHandler(operation:{try await worker.value},onCancel:{worker.cancel()})
        }
        let before=try await hash()
        let signature=await SirsiEngine.captureProcess(executable:"/usr/bin/codesign",
            args:["--verify","--strict","-R","=anchor apple generic and certificate leaf[subject.OU] = \"9D382WV988\" and identifier \"sirsi\"",engine],
            directory:URL(fileURLWithPath:"/"),timeout:8,outputLimit:8192,
            environment:["PATH":"/usr/bin:/bin:/usr/sbin:/sbin","LC_ALL":"C"])
        guard signature.complete,signature.exitCode==0,!Task.isCancelled else{throw NSError(domain:"MaintenanceEngineSignature",code:1)}
        let after=try await hash()
        guard before==after,!Task.isCancelled else{throw NSError(domain:"MaintenanceEngineDrift",code:1)}
        return before
    }

    static func requireCompatibleInstalledPair() async throws {
        guard let bundled=Bundle.main.executableURL?.deletingLastPathComponent().appendingPathComponent("sirsi"),
              SirsiEngine.isTrustedBundledCLI(bundled),
              let release=Bundle.main.object(forInfoDictionaryKey:"CFBundleShortVersionString") as? String else {
            throw NSError(domain:"MaintenanceCompatibleInstallation",code:1)
        }
        let a=try await capability(at:bundled.path),b=try await capability(at:engine)
        guard a.supported,a==b,a.release==release else {throw NSError(domain:"MaintenanceCompatibleInstallation",code:2)}
    }
    static func capability(at path:String)async throws->Capability{
        let result=await SirsiEngine.captureProcess(executable:path,
            args:["launchd","legacy-maintenance-app-capabilities"],directory:URL(fileURLWithPath:"/"),
            timeout:8,outputLimit:8192,environment:["PATH":"/usr/bin:/bin:/usr/sbin:/sbin","HOME":NSHomeDirectory(),"LC_ALL":"C"])
        guard result.complete,result.exitCode==0,let capability=try?JSONDecoder().decode(Capability.self,from:result.stdoutData),capability.supported else {
            throw NSError(domain:"MaintenanceHelperProtocolUnavailable",code:1)
        }
        return capability
    }

    static func request(args original: [String], stdin: String?, engineHash: String) throws -> String {
        func invalid() -> NSError { NSError(domain: "LegacyRelayBridge", code: 1) }
        var args=original
        var ticketJSON=""
        if args.count>=5,args[args.count-2]=="--app-ticket" {
            guard let bytes=Data(base64Encoded:args.last!),bytes.count<=8192,
                  let raw=String(data:bytes,encoding:.utf8),
                  let ticket=try?JSONDecoder().decode(LegacyRelayAppTicket.self,from:bytes),ticket.valid else{throw invalid()}
            ticketJSON=raw;args.removeLast(2)
        }
        guard args.count >= 3, Array(args.prefix(2)) == ["launchd", "legacy-maintenance"],
              engineHash.count == 64, engineHash.utf8.allSatisfy({ (48...57).contains($0) || (97...102).contains($0) }) else { throw invalid() }
        var value = ["profile": "root-relay-legacy-maintenance-v1", "operation": args[2], "engine_sha256": engineHash,
                     "legacy": "", "transaction": "", "plan_sha256": "", "artifact": "", "restore_json": "",
                     "ticket_json":ticketJSON,"target_operation":""]
        switch args[2] {
        case "preview":
            guard args.count == 5, args[3] == "--legacy-payload", stdin == nil else { throw invalid() }; value["legacy"] = args[4]
        case "confirm":
            guard args.count == 8, args[3] == "--legacy-payload", args[5] == "--expected-plan-sha256", args[7] == "--confirm-maintenance", stdin == nil else { throw invalid() }
            value["legacy"] = args[4]; value["plan_sha256"] = args[6]
        case "status":
            guard args.count == 3, stdin == nil,ticketJSON.isEmpty else { throw invalid() }
        case "prepare-operation":
            guard args.count==9,args[3]=="--operation",["recover","abort","restore-confirm"].contains(args[4]),
                  args[5]=="--transaction",args[7]=="--expected-plan-sha256",ticketJSON.isEmpty else{throw invalid()}
            value["target_operation"]=args[4];value["transaction"]=args[6];value["plan_sha256"]=args[8]
            if args[4]=="restore-confirm"{guard let stdin,stdin.utf8.count<=8192 else{throw invalid()};value["restore_json"]=stdin}
            else if stdin != nil{throw invalid()}
        case "resolve-operation":
            guard args.count==3,!ticketJSON.isEmpty,stdin==nil else{throw invalid()}
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
        if ["confirm","recover","abort","restore-confirm"].contains(args[2]),ticketJSON.isEmpty{throw invalid()}
        if ["preview","restore-preview"].contains(args[2]),!ticketJSON.isEmpty{throw invalid()}
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
    nonisolated static func protectedEngineHash(deadline:TimeInterval) throws -> String {
        func refused() -> NSError { NSError(domain: "ProtectedMaintenanceEngine", code: 1) }
        func current()throws{if Task.isCancelled||ProcessInfo.processInfo.systemUptime>=deadline{throw refused()}}
        try current()
        var parent = Darwin.open("/", O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard parent >= 0 else { throw refused() }
        defer { Darwin.close(parent) }
        for part in engine.split(separator: "/").dropLast() {
            try current()
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
        var hasher = SHA256(), buffer = [UInt8](repeating: 0, count: 64 * 1024), total = 0
        while true {
            try current()
            let count = buffer.withUnsafeMutableBytes { Darwin.read(fd, $0.baseAddress!, $0.count) }
            if count < 0 { if errno == EINTR { continue }; throw refused() }
            if count == 0 { break }
            total += count; guard total <= 256 * 1024 * 1024 else { throw refused() }
            hasher.update(data: Data(buffer.prefix(count)))
        }
        var after = stat(), named = stat()
        try current()
        guard Int64(total) == before.st_size, fstat(fd, &after) == 0, fstatat(parent, "sirsi", &named, AT_SYMLINK_NOFOLLOW) == 0,
              before.st_dev == after.st_dev, before.st_ino == after.st_ino, before.st_size == after.st_size,
              before.st_mtimespec.tv_sec == after.st_mtimespec.tv_sec, before.st_mtimespec.tv_nsec == after.st_mtimespec.tv_nsec,
              before.st_mode == after.st_mode, before.st_uid == after.st_uid, after.st_nlink == 1,
              named.st_dev == after.st_dev, named.st_ino == after.st_ino else { throw refused() }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    private struct ACLResult { var length: UInt32 = 0; var reference = attrreference_t() }
    nonisolated private static func noACL(_ fd: Int32) -> Bool {
        var attrs = attrlist(); attrs.bitmapcount = UInt16(ATTR_BIT_MAP_COUNT); attrs.commonattr = UInt32(ATTR_CMN_EXTENDED_SECURITY)
        var result = ACLResult()
        let status = withUnsafeMutablePointer(to: &attrs) { attributes in
            withUnsafeMutableBytes(of: &result) { bytes in Darwin.fgetattrlist(fd, attributes, bytes.baseAddress!, bytes.count, UInt32(FSOPT_REPORT_FULLSIZE)) }
        }
        return status == 0 && result.length == MemoryLayout<ACLResult>.size && result.reference.attr_length == 0
    }
}
