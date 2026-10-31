//go:build darwin || linux

package selfupdate

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const maxExecutableCompareBytes int64 = 512 << 20

type executableIdentity struct {
	device, inode                  uint64
	mode, uid, gid                 uint32
	links                          uint64
	size                           int64
	mtimeSeconds, mtimeNanoseconds int64
	ctimeSeconds, ctimeNanoseconds int64
	regular                        bool
}

type executableCompareOps struct {
	open         func(string) (*os.File, error)
	fileIdentity func(*os.File) (executableIdentity, error)
	pathIdentity func(string) (executableIdentity, error)
	digest       func(context.Context, *os.File, int64) ([sha256.Size]byte, error)
}

func sameExecutable(left, right string) bool {
	return sameExecutableContext(context.Background(), left, right)
}

func sameExecutableContext(ctx context.Context, left, right string) bool {
	return sameExecutableWith(ctx, left, right, defaultExecutableCompareOps())
}

func defaultExecutableCompareOps() executableCompareOps {
	return executableCompareOps{
		open: openExecutableFile, fileIdentity: executableFileIdentity,
		pathIdentity: executablePathIdentity, digest: digestExecutable,
	}
}

func sameExecutableWith(ctx context.Context, left, right string, ops executableCompareOps) (same bool) {
	if filepath.Clean(left) == filepath.Clean(right) {
		return true
	}
	if ctx == nil || ctx.Err() != nil || ops.open == nil || ops.fileIdentity == nil || ops.pathIdentity == nil || ops.digest == nil {
		return false
	}
	leftFile, err := ops.open(left)
	if err != nil || leftFile == nil {
		return false
	}
	defer func() {
		if err := leftFile.Close(); err != nil {
			same = false
		}
	}()
	rightFile, err := ops.open(right)
	if err != nil || rightFile == nil {
		return false
	}
	defer func() {
		if err := rightFile.Close(); err != nil {
			same = false
		}
	}()

	leftBefore, err := ops.fileIdentity(leftFile)
	if err != nil || !leftBefore.regular {
		return false
	}
	rightBefore, err := ops.fileIdentity(rightFile)
	if err != nil || !rightBefore.regular {
		return false
	}
	if !executablePathStillNamesFile(left, leftBefore, ops.pathIdentity) ||
		!executablePathStillNamesFile(right, rightBefore, ops.pathIdentity) {
		return false
	}
	leftInfo, leftErr := leftFile.Stat()
	rightInfo, rightErr := rightFile.Stat()
	if leftErr != nil || rightErr != nil || leftInfo == nil || rightInfo == nil {
		return false
	}
	if os.SameFile(leftInfo, rightInfo) {
		return executableFilesUnchanged(leftFile, rightFile, left, right, leftBefore, rightBefore, ops)
	}
	if leftBefore.size != rightBefore.size || leftBefore.size < 0 || leftBefore.size > maxExecutableCompareBytes {
		return false
	}
	leftDigest, err := ops.digest(ctx, leftFile, leftBefore.size)
	if err != nil {
		return false
	}
	rightDigest, err := ops.digest(ctx, rightFile, rightBefore.size)
	if err != nil {
		return false
	}
	if !executableFilesUnchanged(leftFile, rightFile, left, right, leftBefore, rightBefore, ops) {
		return false
	}
	return leftDigest == rightDigest
}

func executableFilesUnchanged(leftFile, rightFile *os.File, leftPath, rightPath string,
	leftBefore, rightBefore executableIdentity, ops executableCompareOps) bool {
	leftAfter, leftErr := ops.fileIdentity(leftFile)
	rightAfter, rightErr := ops.fileIdentity(rightFile)
	return leftErr == nil && rightErr == nil && leftBefore == leftAfter && rightBefore == rightAfter &&
		executablePathStillNamesFile(leftPath, leftAfter, ops.pathIdentity) &&
		executablePathStillNamesFile(rightPath, rightAfter, ops.pathIdentity)
}

func executablePathStillNamesFile(path string, opened executableIdentity,
	pathIdentity func(string) (executableIdentity, error)) bool {
	current, err := pathIdentity(path)
	return err == nil && current.regular && current == opened
}

func openExecutableFile(path string) (*os.File, error) {
	// O_NONBLOCK prevents a raced FIFO/device substitution from hanging this
	// read-only health scan. Omitting O_NOFOLLOW intentionally allows PATH's
	// established symlink to resolve to its regular executable target.
	identity, err := executablePathIdentity(path)
	if err != nil || !identity.regular {
		return nil, os.ErrInvalid
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, os.ErrInvalid
	}
	return file, nil
}

func executableFileIdentity(file *os.File) (executableIdentity, error) {
	var stat unix.Stat_t
	if file == nil {
		return executableIdentity{}, os.ErrInvalid
	}
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return executableIdentity{}, err
	}
	return executableIdentityFromStat(stat), nil
}

func executablePathIdentity(path string) (executableIdentity, error) {
	var stat unix.Stat_t
	if err := unix.Stat(path, &stat); err != nil { // follows the intentional PATH symlink
		return executableIdentity{}, err
	}
	return executableIdentityFromStat(stat), nil
}

func executableIdentityFromStat(stat unix.Stat_t) executableIdentity {
	return executableIdentity{
		device: uint64(stat.Dev), inode: stat.Ino, mode: uint32(stat.Mode), uid: stat.Uid, gid: stat.Gid,
		links: uint64(stat.Nlink), size: stat.Size,
		mtimeSeconds: stat.Mtim.Sec, mtimeNanoseconds: stat.Mtim.Nsec,
		ctimeSeconds: stat.Ctim.Sec, ctimeNanoseconds: stat.Ctim.Nsec,
		regular: stat.Mode&unix.S_IFMT == unix.S_IFREG,
	}
}

func digestExecutable(ctx context.Context, file *os.File, size int64) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	if ctx == nil || file == nil || size < 0 || size > maxExecutableCompareBytes {
		return empty, errors.New("executable byte comparison exceeds its bounded regular-file limit")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return empty, err
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var read int64
	for read <= size {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		remaining := size + 1 - read
		chunk := int64(len(buffer))
		if remaining < chunk {
			chunk = remaining
		}
		n, err := file.Read(buffer[:int(chunk)])
		if n > 0 {
			read += int64(n)
			if _, hashErr := hash.Write(buffer[:n]); hashErr != nil {
				return empty, hashErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && read == size {
				break
			}
			return empty, err
		}
		if n == 0 {
			return empty, io.ErrNoProgress
		}
	}
	if read != size {
		return empty, errors.New("executable changed size or exceeded its bounded read")
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}
