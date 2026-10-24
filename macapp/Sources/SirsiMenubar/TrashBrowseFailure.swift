import Foundation
import Darwin

// Native read-only failure metadata, never a Go selection/deletion protocol.
struct TrashBrowseFailure: Codable, Equatable, Error, Sendable {
    enum Stage: String, Codable, Sendable { case ownerHome, trashRoot, ancestor, directoryStream, metadata }
    enum Code: String, Codable, Sendable {
        case permissionDenied, noFollowRefused, unavailable, notDirectory, descriptorLimit, syscall, safetyRefusal
    }
    let schema: String
    let code: Code
    let posixErrno: Int32?
    let stage: Stage
    let message: String

    init(errno: Int32, stage: Stage) {
        schema = "pantheon.trash-browse-error.v1"
        code = Self.code(for: errno); posixErrno = errno; self.stage = stage
        message = Self.message(for: code)
    }

    init(safety message: String, stage: Stage = .metadata) {
        schema = "pantheon.trash-browse-error.v1"; code = .safetyRefusal
        posixErrno = nil; self.stage = stage; self.message = message
    }

    init(message: String) { self.init(safety: message) }

    private static func code(for number: Int32) -> Code {
        switch number {
        case EACCES, EPERM: return .permissionDenied
        case ELOOP: return .noFollowRefused
        case ENOENT: return .unavailable
        case ENOTDIR: return .notDirectory
        case EMFILE, ENFILE: return .descriptorLimit
        default: return .syscall
        }
    }

    private static func message(for code: Code) -> String {
        switch code {
        case .permissionDenied: return "macOS denied opening this folder. Permissions or privacy restrictions may be involved; Full Disk Access is a possible cause, not a confirmed diagnosis."
        case .noFollowRefused: return "A no-follow open was refused. The path may involve a link or loop. Start a fresh browse; no links will be followed."
        case .unavailable: return "The folder or path no longer exists or is unavailable. Its contents are unknown, not empty. Start a fresh browse; Pantheon will not create or replace it."
        case .notDirectory: return "A required component is not an admitted directory. A no-follow directory open can also fail this way. Start a fresh browse."
        case .descriptorLimit: return "The process or system has reached its open-file limit. Retry explicitly after resources become available; no process will be stopped."
        case .syscall: return "A native folder operation failed. Contents are unknown. Retry a fresh browse."
        case .safetyRefusal: return "Folder safety checks refused this browse. Start a fresh browse."
        }
    }

    var offersAccessGuide: Bool { code == .permissionDenied && [EACCES, EPERM].contains(posixErrno ?? 0) }
    var valid: Bool {
        guard schema == "pantheon.trash-browse-error.v1", !message.isEmpty, message.utf8.count <= 1024 else { return false }
        if code == .safetyRefusal { return posixErrno == nil }
        guard let posixErrno, posixErrno > 0 else { return false }
        return code == Self.code(for: posixErrno) && message == Self.message(for: code)
    }

    // Keep native failure incomplete for existing browse callers, while carrying
    // a bounded separately versioned envelope for this workflow's error branch.
    var execution: CommandExecution {
        CommandExecution(stdout: (try? JSONEncoder().encode(self)) ?? Data(), exitCode: 1,
                         launchError: message, childState: .exitConfirmed)
    }

    static func decode(_ capture: CommandExecution) -> TrashBrowseFailure? {
        guard capture.exitCode == 1, capture.childState == .exitConfirmed,
              !capture.cancelled, !capture.timedOut, !capture.overflow, capture.captureError == nil,
              capture.stdoutData.count <= 4096,
              let failure = try? JSONDecoder().decode(Self.self, from: capture.stdoutData), failure.valid,
              capture.launchError == failure.message else { return nil }
        return failure
    }
}

// Per-instance seams capture errno before cleanup/formatting; no global hooks.
struct TrashBrowseSyscalls: Sendable {
    struct OpenResult { let fd: Int32; let error: Int32? }
    struct StreamResult { let stream: UnsafeMutablePointer<DIR>?; let error: Int32? }
    var open: @Sendable (Int32?, String, Int32) -> OpenResult
    var stream: @Sendable (Int32) -> StreamResult
    var close: @Sendable (Int32) -> Void

    static let live = TrashBrowseSyscalls(open: { parent, name, flags in
        name.withCString { path in
            let fd: Int32
            if let parent { fd = openat(parent, path, flags) }
            else { fd = Darwin.open(path, flags) }
            let captured = fd < 0 ? errno : nil
            return OpenResult(fd: fd, error: captured)
        }
    }, stream: { fd in
        let pointer = fdopendir(fd)
        let captured = pointer == nil ? errno : nil
        return StreamResult(stream: pointer, error: captured)
    }, close: { fd in _ = Darwin.close(fd) })

    static func admitted(_ result: OpenResult, stage: TrashBrowseFailure.Stage) throws -> Int32 {
        guard result.fd >= 0 else {
            if let captured = result.error { throw TrashBrowseFailure(errno: captured, stage: stage) }
            throw TrashBrowseFailure(safety: "Native open failed without an available error code. Start a fresh browse.", stage: stage)
        }
        return result.fd
    }
}

struct PantheonAccessGuide {
    let bundleURL: URL
    init(bundleURL: URL = Bundle.main.bundleURL) { self.bundleURL = bundleURL }
    var appName: String { bundleURL.pathExtension == "app" ? bundleURL.deletingPathExtension().lastPathComponent : "Pantheon" }
    func openSettings(open: (URL) -> Void, reveal: (URL) -> Void) {
        // Only the owner's explicit button invokes these existing native actions.
        if let url = URL(string: fullDiskAccessPaneURL) { open(url) }
        reveal(bundleURL)
    }
}
