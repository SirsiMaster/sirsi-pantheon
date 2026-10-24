//go:build darwin

package setup

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// RouterMCPMigration has instance-local probes for inert fixture qualification.
// Production callers cannot supply another home, engine or filesystem policy.
type RouterMCPMigration struct {
	home          string
	engine        func() (RouterMCPEngine, error)
	legacy        func(string) (RouterMCPEngine, error)
	writePrivate  func(*os.File, []byte) error
	beforePublish func() error // test seam; runs before the final retained checks
	afterPublish  func() error // test seam; failure never triggers blind restoration
}

func NewRouterMCPMigration() (*RouterMCPMigration, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("router MCP: owner home unavailable")
	}
	if !filepath.IsAbs(home) || filepath.Clean(home) != home {
		return nil, errors.New("router MCP: canonical owner home required")
	}
	return &RouterMCPMigration{home: home, engine: managedRouterMCPEngine, legacy: legacyRouterMCPEngine}, nil
}

func sameMCPPayloadInfo(a, b os.FileInfo) bool {
	if a == nil || b == nil || !os.SameFile(a, b) {
		return false
	}
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Mode == y.Mode && x.Uid == y.Uid && x.Gid == y.Gid && x.Nlink == y.Nlink && x.Size == y.Size && x.Mtimespec == y.Mtimespec && x.Ctimespec == y.Ctimespec
}

func managedRouterMCPEngine() (RouterMCPEngine, error) {
	path, err := CurrentPackagedMaatEngine()
	if err != nil {
		return RouterMCPEngine{}, err
	}
	if err := VerifyPackagedMaatEngine(path); err != nil {
		return RouterMCPEngine{}, err
	}
	f, info, err := openCanonicalPath(path, false, false)
	if err != nil {
		return RouterMCPEngine{}, errors.New("router MCP: installed engine unavailable")
	}
	defer func() { _ = f.Close() }()
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 || info.Size() <= 0 || info.Size() > 512<<20 {
		return RouterMCPEngine{}, errors.New("router MCP: unsafe installed engine metadata")
	}
	// Streaming fingerprint, not execution or copying the installed payload.
	fingerprint := sha256.New()
	n, err := io.Copy(fingerprint, io.LimitReader(f, (512<<20)+1))
	if err != nil || n > 512<<20 || n != info.Size() {
		return RouterMCPEngine{}, errors.New("router MCP: bounded installed engine fingerprint unavailable")
	}
	if err := VerifyPackagedMaatEngine(path); err != nil {
		return RouterMCPEngine{}, err
	}
	after, err := f.Stat()
	named, nerr := os.Lstat(path)
	if err != nil || nerr != nil || !sameMCPPayloadInfo(info, after) || !sameMCPPayloadInfo(info, named) {
		return RouterMCPEngine{}, errors.New("router MCP: installed engine changed during qualification")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return RouterMCPEngine{}, errors.New("router MCP: engine identity unavailable")
	}
	return RouterMCPEngine{Path: path, SHA256: hex.EncodeToString(fingerprint.Sum(nil)), Device: uint64(st.Dev), Inode: st.Ino}, nil
}

func legacyRouterMCPEngine(path string) (RouterMCPEngine, error) {
	f, info, err := openCanonicalPath(path, false, false)
	if err != nil {
		return RouterMCPEngine{}, errors.New("router MCP: exact existing owner-local legacy executable unavailable")
	}
	defer func() { _ = f.Close() }()
	if !info.Mode().IsRegular() || !ownedByCurrentUser(info) || linkCount(info) != 1 || info.Mode().Perm()&0111 == 0 || info.Mode().Perm()&0022 != 0 || info.Size() <= 0 || info.Size() > 512<<20 {
		return RouterMCPEngine{}, errors.New("router MCP: unsafe owner-local legacy executable")
	}
	metadata, err := buildinfo.Read(f)
	if err != nil || metadata.Path != "github.com/SirsiMaster/sirsi-pantheon/cmd/sirsi-router-mcp" {
		return RouterMCPEngine{}, errors.New("router MCP: selected legacy command is not the known standalone Go payload")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return RouterMCPEngine{}, err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(f, (512<<20)+1))
	if err != nil || n > 512<<20 || n != info.Size() {
		return RouterMCPEngine{}, errors.New("router MCP: legacy fingerprint unavailable")
	}
	after, err := f.Stat()
	named, nerr := os.Lstat(path)
	if err != nil || nerr != nil || !sameMCPPayloadInfo(info, after) || !sameMCPPayloadInfo(info, named) {
		return RouterMCPEngine{}, errors.New("router MCP: legacy payload changed during inspection")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return RouterMCPEngine{}, errors.New("router MCP: legacy identity unavailable")
	}
	return RouterMCPEngine{Path: path, SHA256: hex.EncodeToString(hash.Sum(nil)), Device: uint64(st.Dev), Inode: st.Ino}, nil
}

type routerMCPConfigSnapshot struct {
	publication *horusFilePublication
	info        os.FileInfo
	bytes       []byte
}

func validRouterMCPConfigInfo(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && ownedByCurrentUser(info) && linkCount(info) == 1 && info.Mode().Perm()&0022 == 0 && info.Size() <= routerMCPConfigLimit
}

func inspectRouterMCPConfig(path string) (*routerMCPConfigSnapshot, error) {
	dir, err := openHorusParent(path)
	if err != nil {
		return nil, errors.New("router MCP: existing nofollow config parent unavailable")
	}
	p := &horusFilePublication{dir: dir, parent: filepath.Dir(path), leaf: filepath.Base(path)}
	fail := func(err error) (*routerMCPConfigSnapshot, error) { return nil, errors.Join(err, p.Close()) }
	if err := p.parentStable(); err != nil {
		return fail(errors.New("router MCP: config parent must remain canonical, owned and not writable by others"))
	}
	fd, err := unix.Openat(dir, p.leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return fail(errors.New("router MCP: selected existing config unavailable"))
	}
	p.original = os.NewFile(uintptr(fd), path)
	info, err := p.original.Stat()
	if err != nil || !validRouterMCPConfigInfo(info) {
		return fail(errors.New("router MCP: config must be bounded owned single-link regular file without foreign write access"))
	}
	data, err := io.ReadAll(io.LimitReader(p.original, routerMCPConfigLimit+1))
	if err != nil || len(data) > routerMCPConfigLimit {
		return fail(errors.New("router MCP: config read failed or exceeds bound"))
	}
	if err := p.verify(p.original, p.leaf, data, false); err != nil {
		return fail(errors.New("router MCP: config identity/bytes changed during inspection"))
	}
	return &routerMCPConfigSnapshot{publication: p, info: info, bytes: data}, nil
}

func configIDs(s *routerMCPConfigSnapshot) (uint64, uint64, uint64, uint64, error) {
	st, ok := s.info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, 0, errors.New("config identity unavailable")
	}
	var parent unix.Stat_t
	if err := unix.Fstat(s.publication.dir, &parent); err != nil {
		return 0, 0, 0, 0, err
	}
	return uint64(st.Dev), st.Ino, uint64(parent.Dev), parent.Ino, nil
}

func (m *RouterMCPMigration) Plan(s RouterMCPSelection) (plan RouterMCPMigrationPlan, result error) {
	if m == nil || m.engine == nil || m.legacy == nil || !filepath.IsAbs(m.home) || filepath.Clean(m.home) != m.home {
		return plan, errors.New("router MCP: use the initialized owner-local migration helper")
	}
	if err := routerMCPConfigPath(m.home, s); err != nil {
		return plan, err
	}
	snapshot, err := inspectRouterMCPConfig(s.Config)
	if err != nil {
		return plan, err
	}
	defer func() { result = errors.Join(result, snapshot.publication.Close()) }()
	engine, err := m.engine()
	if err != nil {
		return plan, errors.New("router MCP: signed managed engine qualification failed")
	}
	legacy := filepath.Join(m.home, ".local", "bin", "sirsi-router-mcp")
	after, preserved, noop, err := migrateMCPBytes(snapshot.bytes, s, legacy, engine.Path)
	if err != nil {
		return plan, err
	}
	// A legacy path must name the exact existing owner-local executable, not a
	// basename, wrapper or a symlink whose destination happens to match today.
	var oldEngine RouterMCPEngine
	if !noop {
		oldEngine, err = m.legacy(legacy)
		if err != nil {
			return plan, err
		}
	}
	dev, ino, pdev, pino, err := configIDs(snapshot)
	if err != nil {
		return plan, err
	}
	plan = RouterMCPMigrationPlan{Schema: "sirsi.router-mcp-migration.v1", Selection: s, OldCommand: legacy, NewArguments: []string{engine.Path, "router-mcp"}, Engine: engine, ConfigSHA256: routerMCPHash(snapshot.bytes), AfterSHA256: routerMCPHash(after), ConfigDevice: dev, ConfigInode: ino, ParentDevice: pdev, ParentInode: pino, PreservedFields: preserved, Status: "planned_client_reconnect_unobserved"}
	plan.Legacy = oldEngine
	plan.ConfigMode = uint32(snapshot.info.Mode().Perm())
	var parentStat unix.Stat_t
	if err := unix.Fstat(snapshot.publication.dir, &parentStat); err != nil {
		return plan, err
	}
	plan.ParentMode = uint32(parentStat.Mode & 07777)
	if noop {
		plan.OldCommand = engine.Path
		plan.Status = "already_migrated_client_reconnect_unobserved"
	} else {
		plan.Backup = filepath.Join(filepath.Dir(s.Config), "."+filepath.Base(s.Config)+".router-mcp."+plan.ConfigSHA256+".backup.json")
	}
	plan.Digest = routerMCPPlanDigest(plan)
	return plan, nil
}

func (m *RouterMCPMigration) Apply(s RouterMCPSelection, confirmed string) (result RouterMCPMigrationResult, err error) {
	defer func() {
		if err != nil && result.Status == "configuration_migrated_client_reconnect_pending" {
			result.Status = "migration_not_verified_recovery_required"
		}
	}()
	if len(confirmed) != 64 {
		return result, errors.New("router MCP: apply requires the exact preview digest")
	}
	plan, err := m.Plan(s)
	if err != nil {
		return result, err
	}
	if confirmed != plan.Digest {
		return result, errors.New("router MCP: preview changed; inspect a fresh plan before applying")
	}
	result.Plan = plan
	if plan.Status == "already_migrated_client_reconnect_unobserved" {
		result.Status = plan.Status
		return result, nil
	}
	snapshot, err := inspectRouterMCPConfig(s.Config)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, snapshot.publication.Close()) }()
	dev, ino, pdev, pino, err := configIDs(snapshot)
	if err != nil {
		return result, err
	}
	if dev != plan.ConfigDevice || ino != plan.ConfigInode || pdev != plan.ParentDevice || pino != plan.ParentInode || uint32(snapshot.info.Mode().Perm()) != plan.ConfigMode || routerMCPHash(snapshot.bytes) != plan.ConfigSHA256 {
		return result, errors.New("router MCP: config changed before staging; unchanged")
	}
	after, _, _, err := migrateMCPBytes(snapshot.bytes, s, plan.OldCommand, plan.Engine.Path)
	if err != nil {
		return result, err
	}
	if routerMCPHash(after) != plan.AfterSHA256 {
		return result, errors.New("router MCP: prepared bytes differ from confirmation")
	}
	publication, err := prepareOwnerFilePublication(s.Config, snapshot.bytes, after, filepath.Base(plan.Backup), "."+filepath.Base(s.Config)+".router-mcp", ".stage.json", m.writePrivate)
	result.Backup = plan.Backup
	if err != nil {
		result.Status = "not_migrated_possible_private_staging_debt"
		return result, fmt.Errorf("router MCP: staging failed; inspect retained private backup/stage, no automatic deletion: %w", err)
	}
	result.RetainedStage = filepath.Join(filepath.Dir(s.Config), publication.stage)
	defer func() {
		err = errors.Join(err, publication.Close())
		if err != nil {
			result.Status = "migration_not_verified_recovery_required"
		}
	}()
	if m.beforePublish != nil {
		if err = m.beforePublish(); err != nil {
			return result, err
		}
	}
	engine, err := m.engine()
	if err != nil || engine != plan.Engine {
		return result, errors.New("router MCP: signed engine changed before publication; private backup retained")
	}
	oldEngine, err := m.legacy(plan.OldCommand)
	if err != nil || oldEngine != plan.Legacy {
		return result, errors.New("router MCP: legacy payload changed before publication; private backup retained")
	}
	current, err := publication.original.Stat()
	if err != nil || !os.SameFile(current, snapshot.info) {
		return result, errors.New("router MCP: original config identity changed; private backup retained")
	}
	var parentStat unix.Stat_t
	if err := unix.Fstat(publication.dir, &parentStat); err != nil || uint64(parentStat.Dev) != plan.ParentDevice || parentStat.Ino != plan.ParentInode || uint32(parentStat.Mode&07777) != plan.ParentMode || uint32(current.Mode().Perm()) != plan.ConfigMode {
		return result, errors.New("router MCP: parent/config metadata changed; private backup retained")
	}
	if err = publication.Publish(); err != nil {
		return result, fmt.Errorf("router MCP: publication outcome not verified; backup/stage retained; do not blindly restore: %w", err)
	}
	if m.afterPublish != nil {
		if err = m.afterPublish(); err != nil {
			return result, err
		}
	}
	if err = publication.verify(publication.prepared, publication.leaf, after, true); err != nil {
		return result, err
	}
	result.Status = "configuration_migrated_client_reconnect_pending"
	return result, nil
}
