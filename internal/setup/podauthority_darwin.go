//go:build darwin

package setup

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// InstallMaatPodServiceRecipe creates exactly one private dormant plist in an
// existing owner-private directory. It never creates parents, overwrites an
// occupied recipe, or asks launchd to load/start the job.
func InstallMaatPodServiceRecipe(plan MaatPodServicePlan) (MaatPodServicePlan, error) {
	return installMaatPodServiceRecipe(plan, nil)
}

func installMaatPodServiceRecipe(plan MaatPodServicePlan, afterCreate func(*os.File) error) (MaatPodServicePlan, error) {
	if err := validatePrivateRecipeDirectory(plan.RecipeDirectory); err != nil {
		return plan, fmt.Errorf("maat pod service: recipe parent: %w", err)
	}
	if err := validateCanonicalDirectory(plan.HomeDirectory); err != nil {
		return plan, fmt.Errorf("maat pod service: user home: %w", err)
	}
	if err := rejectAutoloadingPodRecipeDirectory(plan.RecipeDirectory, plan.HomeDirectory); err != nil {
		return plan, err
	}
	if err := validateCanonicalDirectory(plan.WorkingDirectory); err != nil {
		return plan, fmt.Errorf("maat pod service: working directory: %w", err)
	}
	pod := strings.TrimPrefix(plan.Label, maatPodServiceLabelPrefix)
	if !strings.HasPrefix(plan.Label, maatPodServiceLabelPrefix) || validatePodServiceSlug(pod) != nil {
		return plan, errors.New("maat pod service: invalid plan label")
	}
	wantPath := filepath.Join(plan.RecipeDirectory, plan.Label+".plist")
	wantLoginPath := filepath.Join(plan.HomeDirectory, "Library", "LaunchAgents", plan.Label+".plist")
	if plan.PlistPath != wantPath || plan.LoginOptInPath != wantLoginPath || !plan.EngineSignatureVerified ||
		plan.Status != "planned_activation_unobserved" || plan.GlobalState != "unknown" || len(plan.ActionsPerformed) != 1 || plan.ActionsPerformed[0] != "none" ||
		len(plan.ProgramArguments) < 3 || plan.ProgramArguments[1] != "maat" || plan.ProgramArguments[2] != "authority-serve" {
		return plan, errors.New("maat pod service: incomplete or inconsistent plan")
	}
	if !isKnownMaatPodEngine(plan.ProgramArguments[0]) {
		return plan, errors.New("maat pod service: plan does not target an installed sirsi engine")
	}
	if isUnderDirectory(plan.PlistPath, filepath.Join(plan.HomeDirectory, "Library", "LaunchAgents")) {
		return plan, errors.New("maat pod service: dormant recipe must remain outside LaunchAgents")
	}
	wantPlist, err := renderMaatPodServicePlist(plan.Label, plan.ProgramArguments, plan.WorkingDirectory)
	if err != nil || !bytes.Equal(wantPlist, plan.Plist) {
		return plan, errors.New("maat pod service: plist does not match the reviewed plan")
	}

	dir, dirInfo, err := openCanonicalPath(plan.RecipeDirectory, true, false)
	if err != nil {
		return plan, fmt.Errorf("maat pod service: open recipe parent: %w", err)
	}
	defer dir.Close()
	if dirInfo.Mode().Perm() != 0o700 || !ownedByCurrentUser(dirInfo) {
		return plan, errors.New("maat pod service: recipe parent must be owned by this user with mode 0700")
	}
	leaf := filepath.Base(plan.PlistPath)
	fd, err := unix.Openat(int(dir.Fd()), leaf, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return plan, fmt.Errorf("maat pod service: recipe already exists; preserved at %s", plan.PlistPath)
		}
		return plan, fmt.Errorf("maat pod service: create dormant recipe: %w", err)
	}
	file := os.NewFile(uintptr(fd), plan.PlistPath)
	createdInfo, statErr := file.Stat()
	if statErr != nil {
		return plan, podRecipeDebt(plan.PlistPath, nil, statErr, file)
	}
	if !createdInfo.Mode().IsRegular() || createdInfo.Mode().Perm() != 0o600 || !ownedByCurrentUser(createdInfo) || linkCount(createdInfo) != 1 {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, errors.New("created leaf is not an owned private regular file"), file)
	}
	if afterCreate != nil {
		if err := afterCreate(file); err != nil {
			return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
		}
	}
	if err := writeAndSync(file, plan.Plist); err != nil {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
	}
	readback, err := io.ReadAll(io.LimitReader(file, int64(len(plan.Plist))+1))
	wantHash, gotHash := sha256.Sum256(plan.Plist), sha256.Sum256(readback)
	if err != nil || len(readback) != len(plan.Plist) || !bytes.Equal(readback, plan.Plist) || wantHash != gotHash {
		if err == nil {
			err = errors.New("recipe readback did not match complete planned bytes")
		}
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
	}
	finalInfo, err := file.Stat()
	if err != nil || !os.SameFile(createdInfo, finalInfo) || finalInfo.Size() != int64(len(plan.Plist)) ||
		finalInfo.Mode().Perm() != 0o600 || !finalInfo.Mode().IsRegular() || !ownedByCurrentUser(finalInfo) || linkCount(finalInfo) != 1 {
		if err == nil {
			err = errors.New("recipe descriptor identity or metadata changed")
		}
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
	}
	var named unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), leaf, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil ||
		uint64(named.Dev) != fileDevice(finalInfo) || uint64(named.Ino) != fileInode(finalInfo) || named.Mode&unix.S_IFMT != unix.S_IFREG {
		if err == nil {
			err = errors.New("recipe path no longer names the created regular file")
		}
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
	}
	reopenedParent, reopenedInfo, err := openCanonicalPath(plan.RecipeDirectory, true, false)
	if err != nil {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, fmt.Errorf("recipe parent path continuity: %w", err), file)
	}
	parentSame := os.SameFile(dirInfo, reopenedInfo)
	_ = reopenedParent.Close()
	if !parentSame {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, errors.New("recipe parent path no longer names the opened directory"), file)
	}
	if err := dir.Sync(); err != nil {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, file)
	}
	if err := file.Close(); err != nil {
		return plan, podRecipeDebt(plan.PlistPath, createdInfo, err, nil)
	}
	plan.Status = maatPodServiceStatus
	plan.ActionsPerformed = []string{"created_private_recipe"}
	return plan, nil
}

func podRecipeDebt(path string, info os.FileInfo, cause error, file *os.File) error {
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			cause = errors.Join(cause, closeErr)
		}
	}
	identity := "identity unavailable"
	if info != nil {
		identity = fmt.Sprintf("dev=%d ino=%d", fileDevice(info), fileInode(info))
	}
	return fmt.Errorf("maat pod service: install failed (%v); cleanup debt retained at %s (%s); inspect manually", cause, path, identity)
}

func fileDevice(info os.FileInfo) uint64 {
	stat, _ := info.Sys().(*syscall.Stat_t)
	if stat == nil {
		return 0
	}
	return uint64(stat.Dev)
}

func fileInode(info os.FileInfo) uint64 {
	stat, _ := info.Sys().(*syscall.Stat_t)
	if stat == nil {
		return 0
	}
	return uint64(stat.Ino)
}

func isUnderDirectory(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func writeAndSync(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return errors.New("short recipe write")
		}
		data = data[n:]
	}
	return file.Sync()
}
