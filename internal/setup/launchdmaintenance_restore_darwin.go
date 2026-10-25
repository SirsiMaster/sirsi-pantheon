//go:build darwin

package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

const maintenanceProgramLimit = 256 << 20

type maintenanceProgramState struct {
	Exists         bool
	Device, Inode  uint64
	Size           int64
	Mode, UID, GID uint32
	SHA256         string
}

func maintenanceProgramParent(path string, owner uint32) (*os.File, unix.Stat_t, error) {
	var st unix.Stat_t
	fd, err := maintenanceOpenDirectory(path)
	if err != nil {
		return nil, st, err
	}
	f := os.NewFile(uintptr(fd), path)
	if unix.Fstat(fd, &st) != nil || st.Uid != owner || st.Mode&0022 != 0 {
		_ = f.Close()
		return nil, st, errors.New("legacy maintenance: protected program parent required")
	}
	return f, st, nil
}

func maintenanceFileBytes(f *os.File, owner uint32) (maintenanceProgramState, []byte, error) {
	var result maintenanceProgramState
	var before, after unix.Stat_t
	if unix.Fstat(int(f.Fd()), &before) != nil || before.Uid != owner || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Mode&0022 != 0 || before.Nlink != 1 || before.Size < 0 || before.Size > maintenanceProgramLimit || !routerstore.MaintenanceMetadataNoACL(int(f.Fd())) {
		return result, nil, errors.New("legacy maintenance: bounded protected regular program required")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return result, nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maintenanceProgramLimit+1))
	if err != nil {
		return result, nil, err
	}
	if int64(len(data)) != before.Size || unix.Fstat(int(f.Fd()), &after) != nil || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || before.Mtim != after.Mtim || before.Mode != after.Mode || before.Uid != after.Uid || before.Gid != after.Gid || after.Nlink != 1 {
		return result, nil, errors.New("legacy maintenance: retained program changed while reading")
	}
	h := sha256.Sum256(data)
	return maintenanceProgramState{true, uint64(before.Dev), before.Ino, before.Size, uint32(before.Mode & 07777), before.Uid, before.Gid, hex.EncodeToString(h[:])}, data, nil
}

func maintenanceFileAt(parent *os.File, name string, owner uint32) (maintenanceProgramState, []byte, error) {
	var result maintenanceProgramState
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return result, nil, nil
	}
	if err != nil {
		return result, nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	result, data, err := maintenanceFileBytes(f, owner)
	if err != nil {
		return result, nil, err
	}
	var named unix.Stat_t
	if unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(named.Dev) != result.Device || named.Ino != result.Inode {
		return result, nil, errors.New("legacy maintenance: program name substituted")
	}
	return result, data, nil
}

func restoreTarget(p MaintenanceRestorePlan) maintenanceProgramState {
	return maintenanceProgramState{p.TargetExists, p.TargetDevice, p.TargetInode, p.TargetSize, p.TargetMode, p.TargetUID, p.TargetGID, p.TargetSHA256}
}

func (b nativeServiceBackend) maintenanceRestorePreview(ctx context.Context, s MaintenanceStatus, artifact, nonce string) (MaintenanceRestorePlan, error) {
	var p MaintenanceRestorePlan
	if err := rootRelayOwner(os.Getuid(), os.Geteuid()); err != nil {
		return p, err
	}
	if err := maintenanceEnabled(ctx); err != nil {
		return p, err
	}
	d := b.relayDependencies()
	raw, hash, err := d.artifact(rootRelayPlist, true)
	if err != nil {
		return p, err
	}
	if hash != s.Plan.Original.PlistSHA256 {
		return p, errors.New("legacy maintenance: selected configuration changed; no program restoration")
	}
	decoded, err := d.read(ctx, raw, "/usr/bin/plutil", "-convert", "json", "-o", "-", "-")
	if err != nil {
		return p, err
	}
	var config rootRelayConfiguration
	if json.Unmarshal(decoded, &config) != nil || validateRootRelayConfiguration(config) != nil {
		return p, errors.New("legacy maintenance: exact selected service configuration required")
	}
	root, _, err := maintenanceSpool(s.Plan.RootDevice, s.Plan.RootInode)
	if err != nil {
		return p, err
	}
	if err := root.Close(); err != nil {
		return p, err
	}
	artifactHash, err := maintenancePayload(artifact)
	if err != nil || artifactHash != s.Plan.Original.ProgramSHA256 {
		return p, errors.New("legacy maintenance: artifact is not the originally approved replacement")
	}
	cdhash, err := maintenanceImage(ctx, artifact, 0)
	if err != nil || cdhash != s.Plan.ReplacementCDHash {
		return p, errors.New("legacy maintenance: replacement signature/code identity does not match approved plan")
	}
	parent, st, err := maintenanceProgramParent(filepath.Dir(rootRelayProgram), 0)
	if err != nil {
		return p, err
	}
	defer parent.Close()
	before, _, err := maintenanceFileAt(parent, filepath.Base(rootRelayProgram), 0)
	if err != nil {
		return p, err
	}
	p = MaintenanceRestorePlan{Transaction: s.ID, MaintenanceSHA256: s.Plan.SHA256, Revision: s.Revision, Artifact: artifact, ArtifactSHA256: artifactHash, ArtifactCDHash: cdhash, Nonce: nonce, ParentDevice: uint64(st.Dev), ParentInode: st.Ino, TargetExists: before.Exists, TargetDevice: before.Device, TargetInode: before.Inode, TargetSize: before.Size, TargetMode: before.Mode, TargetUID: before.UID, TargetGID: before.GID, TargetSHA256: before.SHA256}
	p.SHA256 = restoreDigest(p)
	return p, nil
}

func (b nativeServiceBackend) maintenanceRestorePrepare(ctx context.Context, s MaintenanceStatus, p MaintenanceRestorePlan) (maintenanceArtifactPublication, error) {
	parent, _, err := maintenanceProgramParent(filepath.Dir(p.Artifact), 0)
	if err != nil {
		return nil, err
	}
	artifact, data, err := maintenanceFileAt(parent, filepath.Base(p.Artifact), 0)
	closeErr := parent.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	if !artifact.Exists || artifact.SHA256 != p.ArtifactSHA256 {
		return nil, errors.New("legacy maintenance: approved artifact changed before staging")
	}
	return prepareMaintenanceArtifactAt(ctx, filepath.Dir(rootRelayProgram), 0, 0, p, data, func(path string) error {
		hash, err := maintenanceImage(ctx, path, 0)
		if err != nil || hash != p.ArtifactCDHash {
			return errors.New("legacy maintenance: retained stage signature/code identity unverified")
		}
		return nil
	}, nil)
}

type maintenanceProgramPublication struct {
	parent, stage         *os.File
	parentPath, stageLeaf string
	parentStat            unix.Stat_t
	owner                 uint32
	plan                  MaintenanceRestorePlan
	prepared              MaintenanceRestoration
	verify                func(string) error
	beforePublish         func() error
}

func (p *maintenanceProgramPublication) Prepared() MaintenanceRestoration { return p.prepared }
func (p *maintenanceProgramPublication) Close() error {
	return errors.Join(p.stage.Close(), p.parent.Close())
}

func prepareMaintenanceArtifactAt(ctx context.Context, path string, owner, gid uint32, p MaintenanceRestorePlan, data []byte, verify func(string) error, beforePublish func() error) (publication maintenanceArtifactPublication, err error) {
	parent, st, err := maintenanceProgramParent(path, owner)
	if err != nil {
		return nil, err
	}
	defer func() {
		if publication == nil {
			_ = parent.Close()
		}
	}()
	if uint64(st.Dev) != p.ParentDevice || st.Ino != p.ParentInode {
		return nil, errors.New("legacy maintenance: restore parent identity changed")
	}
	current, old, err := maintenanceFileAt(parent, "sirsi", owner)
	if err != nil || current != restoreTarget(p) {
		return nil, errors.Join(errors.New("legacy maintenance: restore target changed before staging"), err)
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != p.ArtifactSHA256 {
		return nil, errors.New("legacy maintenance: restoration bytes differ from approved artifact")
	}
	stagePath, backupPath := maintenanceRestoreNames(p)
	stageLeaf, backupLeaf := filepath.Base(stagePath), filepath.Base(backupPath)
	create := func(name string, body []byte, mode os.FileMode) (*os.File, error) {
		fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err != nil {
			return nil, err
		}
		f := os.NewFile(uintptr(fd), name)
		fail := func(err error) (*os.File, error) { _ = f.Close(); return nil, err }
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if err := f.Chown(int(owner), int(gid)); err != nil {
			return fail(err)
		}
		if err := f.Chmod(mode); err != nil {
			return fail(err)
		}
		if n, err := f.Write(body); err != nil || n != len(body) {
			return fail(errors.Join(io.ErrShortWrite, err))
		}
		if err := f.Sync(); err != nil {
			return fail(err)
		}
		state, readback, err := maintenanceFileBytes(f, owner)
		digest := sha256.Sum256(body)
		if err != nil || int64(len(readback)) != state.Size || state.SHA256 != hex.EncodeToString(digest[:]) {
			return fail(errors.Join(errors.New("legacy maintenance: staged bytes/EOF readback unverified; retained leaf"), err))
		}
		var named unix.Stat_t
		if unix.Fstatat(int(parent.Fd()), name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(named.Dev) != state.Device || named.Ino != state.Inode {
			return fail(errors.New("legacy maintenance: newly created staging name changed"))
		}
		return f, nil
	}
	if current.Exists {
		backup, err := create(backupLeaf, old, 0600)
		if err != nil {
			return nil, err
		}
		if err := backup.Close(); err != nil {
			return nil, err
		}
	} else {
		backupPath = ""
	}
	stage, err := create(stageLeaf, data, 0755)
	if err != nil {
		return nil, err
	}
	defer func() {
		if publication == nil {
			_ = stage.Close()
		}
	}()
	actualStage := filepath.Join(path, stageLeaf)
	if verify == nil {
		return nil, errors.New("legacy maintenance: independent signed-stage verification required")
	}
	if err := verify(actualStage); err != nil {
		return nil, err
	}
	if err := parent.Sync(); err != nil {
		return nil, err
	}
	var ss unix.Stat_t
	if err := unix.Fstat(int(stage.Fd()), &ss); err != nil {
		return nil, err
	}
	return &maintenanceProgramPublication{parent: parent, stage: stage, parentPath: path, stageLeaf: stageLeaf, parentStat: st, owner: owner, plan: p, prepared: MaintenanceRestoration{Plan: p, Phase: "prepared", StageDevice: uint64(ss.Dev), StageInode: ss.Ino, StagePath: actualStage, BackupPath: func() string {
		if backupPath == "" {
			return ""
		}
		return filepath.Join(path, backupLeaf)
	}()}, verify: verify, beforePublish: beforePublish}, nil
}

func (p *maintenanceProgramPublication) Publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.beforePublish != nil {
		if err := p.beforePublish(); err != nil {
			return err
		}
	}
	parent, st, err := maintenanceProgramParent(p.parentPath, p.owner)
	if err != nil {
		return err
	}
	_ = parent.Close()
	if st.Dev != p.parentStat.Dev || st.Ino != p.parentStat.Ino {
		return errors.New("legacy maintenance: restore parent path changed before publication")
	}
	current, _, err := maintenanceFileAt(p.parent, "sirsi", p.owner)
	if err != nil || current != restoreTarget(p.plan) {
		return errors.Join(errors.New("legacy maintenance: restore target changed before publication; no overwrite"), err)
	}
	stage, _, err := maintenanceFileBytes(p.stage, p.owner)
	if err != nil || stage.SHA256 != p.plan.ArtifactSHA256 || stage.Device != p.prepared.StageDevice || stage.Inode != p.prepared.StageInode {
		return errors.Join(errors.New("legacy maintenance: stage continuity changed"), err)
	}
	var named unix.Stat_t
	if unix.Fstatat(int(p.parent.Fd()), p.stageLeaf, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || uint64(named.Dev) != stage.Device || named.Ino != stage.Inode {
		return errors.New("legacy maintenance: unknown stage replacement retained")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Serialized, immediate identity reinspection protects cooperative ordinary
	// replacement. No hostile-root atomic rename-CAS theorem is asserted.
	if p.plan.TargetExists {
		err = unix.Renameat(int(p.parent.Fd()), p.stageLeaf, int(p.parent.Fd()), "sirsi")
	} else {
		err = unix.RenameatxNp(int(p.parent.Fd()), p.stageLeaf, int(p.parent.Fd()), "sirsi", unix.RENAME_EXCL)
	}
	if err != nil {
		return err
	}
	if err := p.parent.Sync(); err != nil {
		return err
	}
	result, _, err := maintenanceFileAt(p.parent, "sirsi", p.owner)
	if err != nil || result.Device != stage.Device || result.Inode != stage.Inode || result.SHA256 != p.plan.ArtifactSHA256 || result.Mode != 0755 {
		return errors.Join(errors.New("legacy maintenance: publication outcome unverified; preserve backup/stage history"), err)
	}
	return p.verify(filepath.Join(p.parentPath, "sirsi"))
}
