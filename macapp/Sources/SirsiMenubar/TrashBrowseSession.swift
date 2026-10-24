import Foundation
import Darwin
import CryptoKit

// Read-only metadata, never deletion authority. The I/O actor alone owns and
// uses this stream; no FD/cookie is exported across processes or actor owners.
final class TrashDirectorySession {
    static let flags = O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_NONBLOCK | O_CLOEXEC
    let id = UUID()
    let directory: String
    let root: TrashIdentity
    let containers: [TrashMember]
    let generation: String
    private var descriptors: [Int32] = [] // owner home, Trash, then ancestors
    private var stream: UnsafeMutablePointer<DIR>?
    private var identities: [TrashIdentity] = []
    private let leafStat: (Int32, String) throws -> TrashIdentity
    private let syscalls: TrashBrowseSyscalls
    private var sequence: UInt64 = 0
    private var exhausted = false
    var isClosed: Bool { stream == nil }
    var ownedDescriptorCount: Int { descriptors.count + (stream == nil ? 0 : 1) }
    var descriptorSnapshot: [Int32] { descriptors + (stream.map { [dirfd($0)] } ?? []) }

    typealias Refusal = TrashBrowseFailure

    enum RawNameRow: Equatable {
        case dot
        case eligible(name: String, relative: String)
        case blocked(TrashBlockedMember)
    }

    // The exact production raw-name boundary, independently testable without
    // asking a filesystem to create a name its encoding rules reject. Input is
    // d_namlen bytes, excluding the terminator, never a caller-selected path.
    static func decodeName(_ data: Data, directory: String) throws -> RawNameRow {
        guard !data.isEmpty, data.count < 1024 else {
            throw Refusal(message: "Directory returned an invalid name length.")
        }
        let name = String(data: data, encoding: .utf8)
        if name == "." || name == ".." { return .dot }
        guard let name, !name.contains("/"), !name.contains("\0") else {
            return .blocked(TrashBlockedMember(path: "(invalid name)", reason: "invalid_path", type: "unknown"))
        }
        let relative = directory == "." ? name : directory + "/" + name
        guard relative.utf8.count <= 8192, relative.split(separator: "/").count <= 24 else {
            return .blocked(TrashBlockedMember(path: name, reason: "invalid_path", type: "unknown"))
        }
        return .eligible(name: name, relative: relative)
    }

    static func identity(_ s: stat) -> TrashIdentity {
        let kind: String
        switch UInt32(s.st_mode) & 0o170000 {
        case 0o040000: kind = "directory"
        case 0o100000: kind = "file"
        case 0o120000: kind = "symlink"
        default: kind = "unsupported"
        }
        return TrashIdentity(device: UInt64(bitPattern: Int64(s.st_dev)), inode: UInt64(s.st_ino),
                             uid: s.st_uid, mode: UInt32(s.st_mode), size: s.st_size,
                             mtime_ns: Int64(s.st_mtimespec.tv_sec) &* 1_000_000_000 &+ Int64(s.st_mtimespec.tv_nsec),
                             ctime_ns: Int64(s.st_ctimespec.tv_sec) &* 1_000_000_000 &+ Int64(s.st_ctimespec.tv_nsec), type: kind)
    }

    static func statFD(_ fd: Int32) throws -> TrashIdentity {
        var s = stat()
        guard fstat(fd, &s) == 0 else {
            let captured = errno
            throw Refusal(errno: captured, stage: .metadata)
        }
        return identity(s)
    }

    static func statAt(_ fd: Int32, _ name: String) throws -> TrashIdentity {
        var s = stat()
        let result = name.withCString { path -> Int32? in
            let code = fstatat(fd, path, &s, AT_SYMLINK_NOFOLLOW)
            return code < 0 ? errno : nil
        }
        if let captured = result { throw Refusal(errno: captured, stage: .metadata) }
        return identity(s)
    }

    static func stable(_ a: TrashIdentity, _ b: TrashIdentity) -> Bool {
        a.device == b.device && a.inode == b.inode && a.uid == b.uid && a.mode == b.mode && a.type == b.type
    }

    // home is an internal owned-fixture seam. Production passes only the
    // invoking user's fixed home, never a request-supplied root.
    init(home: URL, request: TrashBrowseRequest,
         syscalls: TrashBrowseSyscalls = .live,
         leafStat: @escaping (Int32, String) throws -> TrashIdentity = TrashDirectorySession.statAt) throws {
        directory = request.directory
        self.leafStat = leafStat
        self.syscalls = syscalls
        let parts = directory == "." ? [] : directory.split(separator: "/", omittingEmptySubsequences: false).map(String.init)
        guard request.schema == "pantheon.trash-browse.v3", request.session == nil, request.sequence == 0,
              request.generation.isEmpty, parts.count <= 24, directory.utf8.count <= 8192,
              !parts.contains(where: { $0.isEmpty || $0 == "." || $0 == ".." || $0.contains("\0") }) else {
            throw Refusal(message: "Invalid folder request. Start a fresh browse.")
        }
        var opened: [Int32] = []
        var retained = false
        defer { if !retained { for fd in opened.reversed() { syscalls.close(fd) } } }
        let homeOpen = syscalls.open(nil, home.path, Self.flags)
        let homeFD = try TrashBrowseSyscalls.admitted(homeOpen, stage: .ownerHome)
        opened.append(homeFD)
        let rootOpen = syscalls.open(homeFD, ".Trash", Self.flags)
        let rootFD = try TrashBrowseSyscalls.admitted(rootOpen, stage: .trashRoot)
        opened.append(rootFD)
        let observedRoot = try Self.statFD(rootFD)
        guard observedRoot.type == "directory", observedRoot.uid == geteuid(),
              request.root.map({ Self.stable($0, observedRoot) }) ?? true else {
            throw Refusal(message: "Trash root changed or is not owned by the current user.")
        }
        root = observedRoot
        var observed = [observedRoot]
        var ancestors: [TrashMember] = []
        var relative = ""
        for part in parts {
            let parent = opened.last!
            let before = try Self.statAt(parent, part)
            guard before.type == "directory", before.uid == observedRoot.uid, before.device == observedRoot.device else {
                throw Refusal(message: "Folder ancestry is untrusted. Browse again.")
            }
            let ancestorOpen = syscalls.open(parent, part, Self.flags)
            let fd = try TrashBrowseSyscalls.admitted(ancestorOpen, stage: .ancestor)
            opened.append(fd)
            let after = try Self.statFD(fd)
            guard before == after else { throw Refusal(message: "Folder changed while opening.") }
            relative = relative.isEmpty ? part : relative + "/" + part
            ancestors.append(TrashMember(path: relative, identity: after)); observed.append(after)
        }
        containers = ancestors
        let stamp = try JSONEncoder().encode(observed.last!)
        generation = SHA256.hash(data: stamp).map { String(format: "%02x", $0) }.joined()
        let streamOpen = syscalls.open(opened.last!, ".", Self.flags)
        let readFD = try TrashBrowseSyscalls.admitted(streamOpen, stage: .directoryStream)
        // libc may materialize a union directory to suppress duplicates. Do not
        // admit that unbounded read-all mode into a bounded native browser.
        var filesystem = statfs()
        guard fstatfs(readFD, &filesystem) == 0 else {
            let captured = errno
            syscalls.close(readFD); throw Refusal(errno: captured, stage: .directoryStream)
        }
        guard UInt32(filesystem.f_flags) & UInt32(MNT_UNION) == 0 else {
            syscalls.close(readFD); throw Refusal(safety: "This directory filesystem cannot support bounded native browsing.", stage: .directoryStream)
        }
        let streamOpenResult = syscalls.stream(readFD)
        guard let handle = streamOpenResult.stream else {
            let captured = streamOpenResult.error
            syscalls.close(readFD)
            if let captured { throw Refusal(errno: captured, stage: .directoryStream) }
            throw Refusal(safety: "Directory stream failed without an available error code.", stage: .directoryStream)
        }
        stream = handle; descriptors = opened; identities = observed; retained = true
    }

    deinit { close() }

    func close() {
        if let stream { closedir(stream); self.stream = nil }
        for fd in descriptors.reversed() { syscalls.close(fd) }
        descriptors.removeAll()
    }

    private func verifyBindings() throws {
        guard descriptors.count == identities.count + 1, let last = descriptors.last else {
            throw Refusal(message: "Browse session is closed. Start a fresh browse.")
        }
        let names = [".Trash"] + containers.map { String($0.path.split(separator: "/").last!) }
        for index in names.indices {
            let linked = try Self.statAt(descriptors[index], names[index])
            let opened = try Self.statFD(descriptors[index + 1])
            guard Self.stable(linked, identities[index]), Self.stable(opened, identities[index]) else {
                throw Refusal(message: "Trash root or parent changed. Start a fresh browse.")
            }
        }
        guard try Self.statFD(last) == identities.last!,
              let stream, try Self.statFD(dirfd(stream)) == identities.last! else {
            throw Refusal(message: "Folder generation changed. Start a fresh browse.")
        }
    }

    func page(_ request: TrashBrowseRequest) throws -> TrashPage {
        do {
            guard let stream, !exhausted, request.schema == "pantheon.trash-browse.v3", request.directory == directory,
                  request.sequence == sequence,
                  (sequence == 0 ? request.session == nil && request.generation.isEmpty :
                    request.session == id && request.generation == generation && request.root == root) else {
                throw Refusal(message: "Browse sequence is stale or closed. Start a fresh browse.")
            }
            try verifyBindings()
            var entries: [TrashMember] = []; var blocked: [TrashBlockedMember] = []
            var calls = 0; var bytes = 0; var rows = 0
            while rows < 32 && calls < 34 {
                errno = 0
                let pointer = readdir(stream); calls += 1
                guard let pointer else {
                    let captured = errno
                    guard captured == 0 else { throw Refusal(errno: captured, stage: .directoryStream) }
                    exhausted = true; break
                }
                var record = pointer.pointee
                let length = Int(record.d_namlen)
                guard length > 0, length < 1024 else { throw Refusal(message: "Directory returned an invalid name length.") }
                let data = withUnsafeBytes(of: &record.d_name) { Data($0.prefix(length)) }
                bytes += length + 1
                let name: String
                let relative: String
                switch try Self.decodeName(data, directory: directory) {
                case .dot: continue
                case .blocked(let row):
                    rows += 1; blocked.append(row); continue
                case .eligible(let decoded, let path):
                    rows += 1; name = decoded; relative = path
                }
                let item: TrashIdentity
                do { item = try leafStat(dirfd(stream), name) }
                catch { blocked.append(TrashBlockedMember(path: relative, reason: "stat_failed", type: "unknown")); continue }
                let reason: String?
                if !["file", "directory", "symlink"].contains(item.type) || item.size < 0 || item.inode == 0 { reason = "unsupported_type" }
                else if item.uid != root.uid { reason = "unowned" }
                else if item.device != root.device { reason = "cross_device" }
                else { reason = nil }
                if let reason { blocked.append(TrashBlockedMember(path: relative, reason: reason, type: item.type)) }
                else { entries.append(TrashMember(path: relative, identity: item)) }
            }
            try verifyBindings()
            sequence += 1
            let more = !exhausted
            let next = more ? TrashBrowseRequest(directory: directory, session: id, sequence: sequence, generation: generation, root: root) : nil
            let result = TrashPage(schema: "pantheon.trash-browse.v3", status: "page", uid: root.uid, root: root,
                                   directory: directory, containers: containers,
                                   entries: entries.sorted { $0.path.utf8.lexicographicallyPrecedes($1.path.utf8) },
                                   blocked: blocked, work: TrashBrowseWork(native_calls: calls, bytes: bytes, inspected: rows),
                                   generation: generation, more: more, next: next, errors: [])
            guard result.validWireShape else { throw Refusal(message: "Native browse metadata is invalid. Browse again.") }
            if exhausted { close() }
            return result
        } catch { close(); throw error }
    }
}

// One per pending read, not a token registry. This fence also protects against
// actor message reordering: a stale queued read cannot reopen after close ACK.
final class TrashBrowseReadFence: @unchecked Sendable {
    private let lock = NSLock()
    private var cancelled = false
    func cancel() { lock.lock(); cancelled = true; lock.unlock() }
    var isCancelled: Bool { lock.lock(); defer { lock.unlock() }; return cancelled }
}

actor TrashBrowseIO {
    private var session: TrashDirectorySession?
    private let beforeRead: @Sendable () -> Void
    private let syscalls: TrashBrowseSyscalls
    init(syscalls: TrashBrowseSyscalls = .live, beforeRead: @escaping @Sendable () -> Void = {}) {
        self.syscalls = syscalls; self.beforeRead = beforeRead
    }
    deinit { session?.close() }
    func close() { session?.close(); session = nil }

    // No await inside directory use: actor reentrancy cannot interleave DIR reads
    // or close/reuse an FD while a synchronous native operation is still active.
    func read(_ request: TrashBrowseRequest, home: URL, fence: TrashBrowseReadFence) -> CommandExecution {
        do {
            // No stream was acquired by this invocation yet; a reordered stale
            // message must not close a newer invocation's already-open stream.
            if Task.isCancelled || fence.isCancelled { return CommandExecution(cancelled: true) }
            beforeRead() // inert per-instance fixture seam; production is empty
            if Task.isCancelled || fence.isCancelled { close(); return CommandExecution(cancelled: true) }
            if request.session == nil { close(); session = try TrashDirectorySession(home: home, request: request, syscalls: syscalls) }
            guard let session else { throw TrashDirectorySession.Refusal(message: "Browse session expired. Start a fresh browse.") }
            let page = try session.page(request)
            if Task.isCancelled || fence.isCancelled { close(); return CommandExecution(cancelled: true) }
            if session.isClosed { close() }
            return CommandExecution(stdout: try JSONEncoder().encode(page), exitCode: 0, childState: .exitConfirmed)
        } catch {
            close()
            let failure = error as? TrashBrowseFailure ?? TrashBrowseFailure(safety: "Native Trash browse failed. Start a fresh browse.")
            return failure.execution
        }
    }
}

@MainActor final class TrashBrowseController {
    enum Lifetime: Equatable { case closed, reading, open, closing }
    private let io: TrashBrowseIO
    private var expiry: Task<Void, Never>?
    private var closeTask: Task<Void, Never>?
    private var generation = UUID()
    private var inflight: UUID?
    private var currentReadFence: TrashBrowseReadFence?
    private var openID: UUID?
    private(set) var lifetime: Lifetime = .closed
    var hasOpenSession: Bool { lifetime == .open }
    init(io: TrashBrowseIO = TrashBrowseIO()) { self.io = io }
    deinit {
        expiry?.cancel()
        currentReadFence?.cancel()
        // One final owned close, not a new runner or a foreign signal. If native
        // I/O is stuck this remains queued; it is never claimed already closed.
        if closeTask == nil { let owner = io; Task { await owner.close() } }
    }

    // Fence immediately on MainActor, coalesce at most one pending owned close.
    // A kernel stall can delay its acknowledgement, never block the GUI actor.
    func invalidate() {
        currentReadFence?.cancel(); currentReadFence = nil
        generation = UUID(); inflight = nil; openID = nil
        expiry?.cancel(); expiry = nil; lifetime = .closing
        guard closeTask == nil else { return }
        let owner = io
        closeTask = Task { [weak self] in
            await owner.close()
            guard let self else { return }
            self.closeTask = nil
            if self.inflight == nil { self.lifetime = .closed }
        }
    }

    func close() async { invalidate(); await closeTask?.value }
    func expire(_ id: UUID) { if openID == id { invalidate() } }

    func read(_ request: TrashBrowseRequest = TrashBrowseRequest(), home: URL = FileManager.default.homeDirectoryForCurrentUser) async -> CommandExecution {
        if request.session == nil { invalidate() }
        else if inflight != nil { return CommandExecution(launchError: "A browse page is already being read.") }
        let token = generation
        let fence = TrashBrowseReadFence(); currentReadFence = fence
        inflight = token; lifetime = .reading
        expiry?.cancel()
        expiry = Task { [weak self] in
            do { try await Task.sleep(nanoseconds: 60_000_000_000) } catch { return }
            if self?.generation == token { self?.invalidate() }
        }
        defer {
            if inflight == token { inflight = nil }
            if currentReadFence === fence { currentReadFence = nil }
        }
        return await withTaskCancellationHandler(operation: {
            await closeTask?.value
            guard generation == token, !Task.isCancelled else { return CommandExecution(cancelled: true) }
            let capture = await io.read(request, home: home, fence: fence)
            guard generation == token, !Task.isCancelled else {
                if generation == token { invalidate() }
                return CommandExecution(cancelled: true)
            }
            guard capture.complete,
                  let page = try? JSONDecoder().decode(TrashPage.self, from: capture.stdoutData), page.validWireShape else {
                invalidate(); return capture
            }
            expiry?.cancel()
            if let id = page.next?.session {
                openID = id; lifetime = .open
                expiry = Task { [weak self] in
                    do { try await Task.sleep(nanoseconds: 60_000_000_000) } catch { return }
                    self?.expire(id)
                }
            } else { openID = nil; lifetime = .closed }
            return capture
        }, onCancel: { [weak self] in
            fence.cancel()
            guard let owner = self else { return }
            Task { @MainActor in
                guard owner.generation == token else { return }
                owner.invalidate()
            }
        })
    }
}
