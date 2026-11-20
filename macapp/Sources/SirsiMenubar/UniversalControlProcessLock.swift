import Darwin
import Foundation

enum UniversalControlLockError: Error {
    case busy
    case unavailable
}

// A short-lived advisory lock for cooperating Pantheon processes of one user
// on this host. It is not a security boundary against that user's other code.
final class UniversalControlProcessLock {
    static var defaultPath: String { "/private/tmp/ai.sirsi.universal-control.\(getuid()).lock" }
    private let descriptor: Int32
    private let path: String

    init(path: String) throws {
        let fd = open(path, O_RDWR | O_CREAT | O_NOFOLLOW | O_CLOEXEC | O_NONBLOCK, 0o600)
        guard fd >= 0 else { throw UniversalControlLockError.unavailable }
        var file = stat()
        var named = stat()
        guard fstat(fd, &file) == 0,
              (file.st_mode & S_IFMT) == S_IFREG,
              file.st_uid == getuid(), file.st_nlink == 1,
              (file.st_mode & 0o077) == 0,
              lstat(path, &named) == 0,
              file.st_dev == named.st_dev, file.st_ino == named.st_ino else {
            close(fd)
            throw UniversalControlLockError.unavailable
        }
        guard flock(fd, LOCK_EX | LOCK_NB) == 0 else {
            let lockErrno = errno
            close(fd)
            throw lockErrno == EWOULDBLOCK ? UniversalControlLockError.busy : UniversalControlLockError.unavailable
        }
        guard lstat(path, &named) == 0,
              file.st_dev == named.st_dev, file.st_ino == named.st_ino else {
            close(fd)
            throw UniversalControlLockError.unavailable
        }
        descriptor = fd
        self.path = path
    }

    func isStillValid() -> Bool {
        var file = stat()
        var named = stat()
        return fstat(descriptor, &file) == 0 && lstat(path, &named) == 0 &&
            (file.st_mode & S_IFMT) == S_IFREG && file.st_uid == getuid() &&
            file.st_nlink == 1 && (file.st_mode & 0o077) == 0 &&
            file.st_dev == named.st_dev && file.st_ino == named.st_ino
    }

    deinit { close(descriptor) }
}
