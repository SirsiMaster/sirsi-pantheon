//go:build darwin || linux

package setup

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

const (
	maatPodServiceLabelPrefix = "ai.sirsi.maat.pod."
	maatPodServiceStatus      = "installed_activation_unobserved"
	maxPodTLSFileBytes        = 1 << 20
	maxCodesignOutputBytes    = 16 << 10
)

// MaatPodServiceSpec contains explicit existing inputs for a dormant launchd
// recipe. Delegation must be freshly read through routerstore.Resolve by the
// caller; this package revalidates the finite snapshot but never resolves or
// mutates canonical state itself.
type MaatPodServiceSpec struct {
	Pod              string
	JournalPath      string
	Listen           string
	CertificatePath  string
	PrivateKeyPath   string
	ClientCAPath     string
	ClientBindings   []string // SHA256=registered-agent
	WorkingDirectory string   // canonical router registry root
	RecipeDirectory  string   // existing, private, owner-controlled directory
	HomeDirectory    string   // actual user home, for the explicit login opt-in destination
	Executable       string   // current executable, resolved to an installed package path
	MachineID        string   // read from router.MachineID by the caller
	Delegation       schedule.Delegation
	Now              time.Time // injectable clock for deterministic refusal fixtures
}

// MaatPodServicePlan is reviewable output only. InstallMaatPodServiceRecipe
// writes PlistPath without loading launchd or changing login/start state.
type MaatPodServicePlan struct {
	Label                   string
	PlistPath               string
	RecipeDirectory         string
	HomeDirectory           string
	LoginOptInPath          string
	WorkingDirectory        string
	Plist                   []byte
	ProgramArguments        []string
	StartArgv               []string
	LoginOptInArgv          []string
	Status                  string
	ActionsPerformed        []string
	GlobalState             string
	EngineSignatureVerified bool
	RuntimeQualification    string
}

// ValidateMaatPodDelegation checks the canonical finite grant snapshot against
// this host. Callers must obtain d from a fresh authenticated Resolve read.
func ValidateMaatPodDelegation(d schedule.Delegation, pod, machine string, now time.Time) error {
	if d.Pod != pod || pod == "" || d.Epoch < 1 || d.AuthorityMachine == "" || machine == "" || d.AuthorityMachine != machine {
		return errors.New("maat pod service: delegation identity does not match this pod and native machine")
	}
	if d.Revoked {
		return errors.New("maat pod service: delegation is revoked")
	}
	issued, err := time.Parse(time.RFC3339, d.IssuedAt)
	if err != nil {
		return errors.New("maat pod service: delegation issue time is invalid")
	}
	expires, err := time.Parse(time.RFC3339, d.NotAfter)
	if err != nil || !expires.After(issued) || expires.Sub(issued) > 12*time.Hour || !now.Before(expires) || now.Before(issued) {
		return errors.New("maat pod service: delegation is not currently finite and active")
	}
	if len(d.Resources) == 0 {
		return errors.New("maat pod service: delegation contains no resources")
	}
	return nil
}

// BuildMaatPodServicePlan validates an already-resolved delegation and builds
// direct ProgramArguments for the installed sirsi engine. It never opens the
// pod journal or creates/locks any runtime state.
func BuildMaatPodServicePlan(spec MaatPodServiceSpec, verifyEngine func(string) error) (MaatPodServicePlan, error) {
	if spec.Now.IsZero() {
		spec.Now = time.Now()
	}
	if err := ValidateMaatPodDelegation(spec.Delegation, spec.Pod, spec.MachineID, spec.Now); err != nil {
		return MaatPodServicePlan{}, err
	}
	if err := validatePodServiceSlug(spec.Pod); err != nil {
		return MaatPodServicePlan{}, err
	}
	listenIP, listenPort, err := parsePodServiceListen(spec.Listen)
	if err != nil {
		return MaatPodServicePlan{}, err
	}
	if !delegationApprovesListener(spec.Delegation, listenIP, listenPort) {
		return MaatPodServicePlan{}, errors.New("maat pod service: listener is not an approved literal delegation address")
	}
	if err := validateCanonicalDirectory(spec.WorkingDirectory); err != nil {
		return MaatPodServicePlan{}, fmt.Errorf("maat pod service: canonical working directory: %w", err)
	}
	if err := validatePrivateRecipeDirectory(spec.RecipeDirectory); err != nil {
		return MaatPodServicePlan{}, fmt.Errorf("maat pod service: private recipe directory: %w", err)
	}
	if err := validateCanonicalDirectory(spec.HomeDirectory); err != nil {
		return MaatPodServicePlan{}, fmt.Errorf("maat pod service: user home: %w", err)
	}
	if err := rejectAutoloadingPodRecipeDirectory(spec.RecipeDirectory, spec.HomeDirectory); err != nil {
		return MaatPodServicePlan{}, err
	}
	if err := validateCanonicalLeafPath(spec.JournalPath, true, true); err != nil {
		return MaatPodServicePlan{}, fmt.Errorf("maat pod service: journal path: %w", err)
	}
	if !isKnownMaatPodEngine(spec.Executable) {
		return MaatPodServicePlan{}, errors.New("maat pod service: executable is not an accepted installed sirsi engine")
	}
	if verifyEngine == nil || verifyEngine(spec.Executable) != nil {
		return MaatPodServicePlan{}, errors.New("maat pod service: installed engine signature verification failed")
	}
	if err := validateTLSMaterial(spec.CertificatePath, spec.PrivateKeyPath, spec.ClientCAPath, listenIP.String(), spec.Now); err != nil {
		return MaatPodServicePlan{}, err
	}
	if err := validateClientBindings(spec.ClientBindings); err != nil {
		return MaatPodServicePlan{}, err
	}

	args := []string{spec.Executable, "maat", "authority-serve",
		"--pod", spec.Pod,
		"--journal", spec.JournalPath,
		"--listen", spec.Listen,
		"--cert", spec.CertificatePath,
		"--key", spec.PrivateKeyPath,
		"--client-ca", spec.ClientCAPath,
	}
	for _, binding := range spec.ClientBindings {
		args = append(args, "--client", binding)
	}
	label := maatPodServiceLabelPrefix + spec.Pod
	plistPath := filepath.Join(spec.RecipeDirectory, label+".plist")
	plist, err := renderMaatPodServicePlist(label, args, spec.WorkingDirectory)
	if err != nil {
		return MaatPodServicePlan{}, err
	}
	loginPath := filepath.Join(spec.HomeDirectory, "Library", "LaunchAgents", label+".plist")
	return MaatPodServicePlan{
		Label: label, PlistPath: plistPath, RecipeDirectory: spec.RecipeDirectory,
		HomeDirectory: spec.HomeDirectory, LoginOptInPath: loginPath,
		WorkingDirectory: spec.WorkingDirectory,
		Plist:            plist, ProgramArguments: args,
		StartArgv:      []string{"/bin/launchctl", "bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), plistPath},
		LoginOptInArgv: []string{"/bin/cp", "-p", "-n", plistPath, loginPath},
		Status:         "planned_activation_unobserved", GlobalState: "unknown",
		ActionsPerformed: []string{"none"}, EngineSignatureVerified: true,
		RuntimeQualification: "Preparation validates a finite delegation and writes a dormant recipe; it does not prove runtime self-fencing, renewal, or reconciliation. " +
			"The current authority-serve path must be separately qualified before starting it.",
	}, nil
}

func validatePodServiceSlug(pod string) error {
	if pod == "" || len(pod) > 64 {
		return errors.New("maat pod service: invalid pod label")
	}
	for i, r := range pod {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || i > 0 && (r == '.' || r == '_' || r == '-')
		if !valid {
			return errors.New("maat pod service: invalid pod label")
		}
	}
	return nil
}

func parsePodServiceListen(address string) (net.IP, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.Contains(host, "%") {
		return nil, "", errors.New("maat pod service: listen must be a literal private IP and port")
	}
	ip := net.ParseIP(host)
	portNum, portErr := strconv.Atoi(port)
	if ip == nil || portErr != nil || portNum < 1 || portNum > 65535 || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() || !isPodPrivateIP(ip) {
		return nil, "", errors.New("maat pod service: listen must be a literal private, Tailscale, or LAN IP and port")
	}
	return ip, strconv.Itoa(portNum), nil
}

func isPodPrivateIP(ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	// Tailscale IPv4 uses CGNAT 100.64.0.0/10, which net.IP.IsPrivate does not classify as RFC1918.
	_, tailscale, _ := net.ParseCIDR("100.64.0.0/10")
	return tailscale.Contains(ip)
}

func delegationApprovesListener(d schedule.Delegation, ip net.IP, port string) bool {
	for _, raw := range d.AuthorityAddrs {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			continue
		}
		host, delegatedPort, err := net.SplitHostPort(u.Host)
		if err == nil && net.ParseIP(host).Equal(ip) && delegatedPort == port {
			return true
		}
	}
	return false
}

func validateClientBindings(bindings []string) error {
	if len(bindings) == 0 {
		return errors.New("maat pod service: at least one registered client binding is required")
	}
	seen := map[string]bool{}
	for _, binding := range bindings {
		hash, agent, ok := strings.Cut(binding, "=")
		if !ok || len(hash) != 64 || strings.TrimSpace(agent) != agent || agent == "" {
			return errors.New("maat pod service: invalid SHA256=registered-agent binding")
		}
		for _, r := range hash {
			if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
				return errors.New("maat pod service: client fingerprint must be lowercase SHA256 hex")
			}
		}
		if seen[hash] {
			return errors.New("maat pod service: duplicate client fingerprint")
		}
		seen[hash] = true
	}
	return nil
}

func validateTLSMaterial(certPath, keyPath, caPath, host string, now time.Time) error {
	certPEM, _, err := readCanonicalRegular(certPath, false, false)
	if err != nil {
		return fmt.Errorf("maat pod service: selected server certificate unavailable: %w", err)
	}
	keyPEM, keyInfo, err := readCanonicalRegular(keyPath, false, true)
	if err != nil {
		return errors.New("maat pod service: selected private key unavailable or not a private owned regular file")
	}
	if keyInfo.Mode().Perm() != 0o600 || !ownedByCurrentUser(keyInfo) {
		return errors.New("maat pod service: private key must be owned by this user with mode 0600")
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return errors.New("maat pod service: selected certificate and private key do not form a valid pair")
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("maat pod service: selected server certificate is invalid")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) || leaf.VerifyHostname(host) != nil {
		return errors.New("maat pod service: server certificate must be current and cover the approved listener IP")
	}
	if len(leaf.ExtKeyUsage) > 0 {
		serverAuth := false
		for _, usage := range leaf.ExtKeyUsage {
			if usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny {
				serverAuth = true
			}
		}
		if !serverAuth {
			return errors.New("maat pod service: server certificate lacks server-auth usage")
		}
	}
	caPEM, _, err := readCanonicalRegular(caPath, false, false)
	if err != nil || !validClientCABundle(caPEM, now) {
		return errors.New("maat pod service: selected client CA bundle is invalid or unavailable")
	}
	return nil
}

func validClientCABundle(data []byte, now time.Time) bool {
	count := 0
	for len(data) > 0 {
		if len(bytes.TrimSpace(data)) == 0 {
			break
		}
		block, rest := pem.Decode(data)
		if block == nil {
			return false
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
			return false
		}
		count++
	}
	return count > 0
}

func isKnownMaatPodEngine(path string) bool {
	clean, err := filepath.Abs(path)
	if err != nil || clean != path || filepath.Clean(path) != path {
		return false
	}
	return path == "/Applications/Pantheon.app/Contents/MacOS/sirsi" || path == "/usr/local/libexec/sirsi-pantheon/sirsi"
}

// CurrentPackagedMaatEngine returns the running executable only when its
// resolved path is one of the two canonical package locations. It deliberately
// has no PATH, symlink-to-dev-build, or loose binary fallback.
func CurrentPackagedMaatEngine() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", errors.New("maat pod service: current executable path unavailable")
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil || !isKnownMaatPodEngine(resolved) {
		return "", errors.New("maat pod service: run prepare from the installed Pantheon app or canonical libexec sirsi")
	}
	if err := validateInstalledPodEngine(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// VerifyPackagedMaatEngine checks the installed package signature without
// executing the candidate payload. Output and runtime are bounded.
func VerifyPackagedMaatEngine(path string) error {
	if !isKnownMaatPodEngine(path) {
		return errors.New("maat pod service: executable is not an accepted installed sirsi engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := maatPodCodesignCommand(ctx, path)
	output := &boundedPodOutput{limit: maxCodesignOutputBytes}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil || output.overflow {
		return errors.New("maat pod service: installed engine signature did not meet the Sirsi requirement")
	}
	return nil
}

func maatPodCodesignCommand(ctx context.Context, path string) *exec.Cmd {
	// codesign treats a bare requirement as a filename. '=' selects an inline
	// expression; the exact argv is covered without executing a candidate.
	requirement := `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`
	return exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", requirement, path)
}

type boundedPodOutput struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedPodOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.Buffer.Write(p[:remaining])
			b.overflow = true
		} else {
			_, _ = b.Buffer.Write(p)
		}
	} else if n > 0 {
		b.overflow = true
	}
	return n, nil
}

func validateInstalledPodEngine(path string) error {
	if !isKnownMaatPodEngine(path) {
		return errors.New("maat pod service: current executable is not an accepted installed sirsi engine")
	}
	f, info, err := openCanonicalPath(path, false, false)
	if f != nil {
		defer f.Close()
	}
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("maat pod service: accepted installed sirsi engine is unavailable or not executable")
	}
	nameInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, nameInfo) || !nameInfo.Mode().IsRegular() {
		return errors.New("maat pod service: accepted installed sirsi engine path changed during inspection")
	}
	return nil
}

func validateCanonicalDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("must be canonical absolute path")
	}
	f, info, err := openCanonicalPath(path, true, false)
	if f != nil {
		defer f.Close()
	}
	if err != nil || info == nil || !info.IsDir() {
		return errors.New("must be an existing non-symlink directory")
	}
	return nil
}

func validatePrivateRecipeDirectory(path string) error {
	if err := validateCanonicalDirectory(path); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o700 || !ownedByCurrentUser(info) {
		return errors.New("must be an existing directory owned by this user with mode 0700")
	}
	return nil
}

func rejectAutoloadingPodRecipeDirectory(path, home string) error {
	// APFS commonly treats case variants as the same directory. Conservatively
	// reject the component pair even on case-sensitive volumes.
	if strings.Contains(strings.ToLower(filepath.Clean(path))+"/", "/library/launchagents/") {
		return errors.New("maat pod service: dormant recipe must remain outside LaunchAgents, including case aliases")
	}
	login, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("maat pod service: cannot inspect login directory alias: %w", err)
	}
	// A pre-existing LaunchAgents symlink may point at an otherwise innocuous
	// canonical recipe directory. Compare object identity along its ancestry.
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err != nil {
			return fmt.Errorf("maat pod service: cannot inspect recipe ancestry: %w", err)
		}
		if os.SameFile(login, info) {
			return errors.New("maat pod service: dormant recipe directory aliases LaunchAgents")
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return nil
}

func validateCanonicalLeafPath(path string, allowMissing, private bool) error {
	f, info, err := openCanonicalPath(path, false, allowMissing)
	if err != nil {
		return err
	}
	if info == nil {
		return nil
	}
	defer f.Close()
	if !info.Mode().IsRegular() || private && (info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) || linkCount(info) != 1) {
		return errors.New("must be a regular file with required private ownership and permissions")
	}
	return nil
}

func readCanonicalRegular(path string, allowMissing, private bool) ([]byte, os.FileInfo, error) {
	f, info, err := openCanonicalPath(path, false, allowMissing)
	if err != nil || info == nil {
		return nil, info, err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxPodTLSFileBytes || private && (info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) || linkCount(info) != 1) {
		f.Close()
		return nil, nil, errors.New("not a suitable regular file")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPodTLSFileBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > maxPodTLSFileBytes {
		return nil, nil, errors.New("file exceeds bounded read size")
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(info, after) {
		return nil, nil, errors.New("file identity changed while reading")
	}
	nameInfo, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, nameInfo) || !nameInfo.Mode().IsRegular() {
		return nil, nil, errors.New("file name no longer identifies the opened regular file")
	}
	return data, info, nil
}

func openCanonicalPath(path string, directory, allowMissing bool) (*os.File, os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return nil, nil, errors.New("path must be canonical and absolute")
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	parent, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	for i, component := range parts {
		leaf := i == len(parts)-1
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if !leaf || directory {
			flags |= unix.O_DIRECTORY
		}
		fd, openErr := unix.Openat(parent, component, flags, 0)
		if openErr != nil {
			unix.Close(parent)
			if leaf && allowMissing && errors.Is(openErr, unix.ENOENT) {
				return nil, nil, nil
			}
			return nil, nil, openErr
		}
		unix.Close(parent)
		parent = fd
	}
	file := os.NewFile(uintptr(parent), path)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return file, info, nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

func linkCount(info os.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(stat.Nlink)
}

func renderMaatPodServicePlist(label string, args []string, workingDirectory string) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n" + `<plist version="1.0"><dict>` + "\n")
	write := func(key, value string) {
		b.WriteString("<key>" + key + "</key><string>")
		_ = xmlEscape(&b, value)
		b.WriteString("</string>\n")
	}
	write("Label", label)
	write("WorkingDirectory", workingDirectory)
	b.WriteString("<key>ProgramArguments</key><array>\n")
	for _, arg := range args {
		b.WriteString("<string>")
		if err := xmlEscape(&b, arg); err != nil {
			return nil, err
		}
		b.WriteString("</string>\n")
	}
	b.WriteString("</array>\n<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><true/>\n<key>ProcessType</key><string>Background</string>\n</dict></plist>\n")
	return []byte(b.String()), nil
}

func xmlEscape(b *strings.Builder, value string) error {
	var encoded strings.Builder
	if err := xml.EscapeText(&encoded, []byte(value)); err != nil {
		return err
	}
	b.WriteString(encoded.String())
	return nil
}
