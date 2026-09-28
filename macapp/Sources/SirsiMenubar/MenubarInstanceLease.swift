import Foundation
import Darwin

// Darwin imports the C `struct flock` under the same name as the libc locking
// function. Bind the latter explicitly so this stays an FD-held lock rather
// than falling back to a stale path/PID marker.
@_silgen_name("flock")
private func lockFileDescriptor(_ descriptor: Int32, _ operation: Int32) -> Int32

// MenubarInstanceLease keeps exactly one modern Pantheon menubar process alive
// for this macOS user. It is intentionally local to the user environment: a
// Horus client on another Mac never contends for this lease. The advisory lock
// is held by an open descriptor, so the OS releases it on a crash or normal
// exit without relying on a stale PID file.
final class MenubarInstanceLease {
    private let descriptor: Int32

    private init(descriptor: Int32) {
        self.descriptor = descriptor
    }

    deinit {
        _ = lockFileDescriptor(descriptor, LOCK_UN)
        _ = Darwin.close(descriptor)
    }

    static func acquire() -> MenubarInstanceLease? {
        let manager = FileManager.default
        guard let support = try? manager.url(for: .applicationSupportDirectory,
                                             in: .userDomainMask,
                                             appropriateFor: nil,
                                             create: true) else {
            return nil
        }
        let directory = support
            .appendingPathComponent("Sirsi", isDirectory: true)
            .appendingPathComponent("Pantheon", isDirectory: true)
        do {
            try manager.createDirectory(at: directory, withIntermediateDirectories: true)
        } catch {
            return nil
        }

        return acquire(at: directory.appendingPathComponent("menubar.instance.lock", isDirectory: false).path)
    }

    // Internal for the native contract test. Production always derives the
    // path above; no caller can choose a cross-user or cross-machine lease.
    static func acquire(at path: String) -> MenubarInstanceLease? {
        let fd = Darwin.open(path, O_RDWR | O_CREAT | O_CLOEXEC, S_IRUSR | S_IWUSR)
        guard fd >= 0 else { return nil }
        guard lockFileDescriptor(fd, LOCK_EX | LOCK_NB) == 0 else {
            _ = Darwin.close(fd)
            return nil
        }
        return MenubarInstanceLease(descriptor: fd)
    }
}
