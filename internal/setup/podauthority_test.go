//go:build darwin

package setup

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

func TestMaatPodCodesignUsesInlineTeamRequirement(t *testing.T) {
	path := "/usr/local/libexec/sirsi-pantheon/sirsi"
	cmd := maatPodCodesignCommand(context.Background(), path)
	want := []string{"/usr/bin/codesign", "--verify", "--strict", "-R", `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`, path}
	if cmd.Path != "/usr/bin/codesign" || !equalStrings(cmd.Args, want) {
		t.Fatalf("verification argv = %q, want %q", cmd.Args, want)
	}
}

func TestBuildMaatPodServicePlanIsDryAndUsesDirectPackagedEngineArgs(t *testing.T) {
	spec, temp := podServiceFixture(t)
	verifiedPath := ""
	plan, err := BuildMaatPodServicePlan(spec, func(path string) error {
		verifiedPath = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if verifiedPath != spec.Executable || !plan.EngineSignatureVerified {
		t.Fatalf("signature probe path/result = %q/%v", verifiedPath, plan.EngineSignatureVerified)
	}
	if got, want := plan.ProgramArguments[:3], []string{spec.Executable, "maat", "authority-serve"}; !equalStrings(got, want) {
		t.Fatalf("program arguments prefix = %q, want %q", got, want)
	}
	if strings.Contains(strings.Join(plan.ProgramArguments, "\x00"), "sh -c") || !strings.Contains(string(plan.Plist), "a&amp;b") {
		t.Fatalf("expected direct argv and XML-escaped working directory, got %q", plan.Plist)
	}
	if plan.Status != "planned_activation_unobserved" || plan.GlobalState != "unknown" || len(plan.ActionsPerformed) != 1 || plan.ActionsPerformed[0] != "none" {
		t.Fatalf("plan makes an unsupported activation claim: %#v", plan)
	}
	if _, err := os.Stat(spec.JournalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry planning touched journal path: stat err=%v", err)
	}
	if _, err := os.Stat(plan.PlistPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry planning wrote recipe: stat err=%v", err)
	}
	if plan.RecipeDirectory != temp.recipeDir {
		t.Fatalf("recipe directory changed: %q", plan.RecipeDirectory)
	}
}

func TestBuildMaatPodServicePlanRefusesInvalidInputs(t *testing.T) {
	tests := []struct {
		name        string
		rejectProbe bool
		mutate      func(*testing.T, *MaatPodServiceSpec)
	}{
		{name: "expired delegation", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			s.Delegation.NotAfter = s.Now.Add(-time.Second).Format(time.RFC3339)
		}},
		{name: "wrong native identity", mutate: func(t *testing.T, s *MaatPodServiceSpec) { s.MachineID = "other-machine" }},
		{name: "revoked delegation", mutate: func(t *testing.T, s *MaatPodServiceSpec) { s.Delegation.Revoked = true }},
		{name: "unapproved listener", mutate: func(t *testing.T, s *MaatPodServiceSpec) { s.Listen = "100.64.12.8:9443" }},
		{name: "wildcard listener", mutate: func(t *testing.T, s *MaatPodServiceSpec) { s.Listen = "0.0.0.0:9443" }},
		{name: "untrusted engine signature", mutate: func(t *testing.T, s *MaatPodServiceSpec) { s.Executable = "/tmp/sirsi" }},
		{name: "signature probe rejects", rejectProbe: true},
		{name: "private key permissions", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			if err := os.Chmod(s.PrivateKeyPath, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "private key hard link", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			if err := os.Link(s.PrivateKeyPath, s.PrivateKeyPath+".linked"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlinked certificate path", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			link := s.CertificatePath + ".link"
			if err := os.Symlink(s.CertificatePath, link); err != nil {
				t.Fatal(err)
			}
			s.CertificatePath = link
		}},
		{name: "relative journal path", mutate: func(t *testing.T, s *MaatPodServiceSpec) { s.JournalPath = "journal.json" }},
		{name: "non-private recipe parent", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			if err := os.Chmod(s.RecipeDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "case alias of LaunchAgents", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			s.RecipeDirectory = filepath.Join(s.HomeDirectory, "Library", "launchagents")
			if err := os.MkdirAll(s.RecipeDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "LaunchAgents alias to dormant parent", mutate: func(t *testing.T, s *MaatPodServiceSpec) {
			library := filepath.Join(s.HomeDirectory, "Library")
			if err := os.MkdirAll(library, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(s.RecipeDirectory, filepath.Join(library, "LaunchAgents")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, _ := podServiceFixture(t)
			if tt.mutate != nil {
				tt.mutate(t, &spec)
			}
			probe := func(string) error { return nil }
			if tt.rejectProbe {
				probe = func(string) error { return errors.New("untrusted") }
			}
			if _, err := BuildMaatPodServicePlan(spec, probe); err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

func TestInstallMaatPodServiceRecipeIsCreateOnlyAndPreservesExistingFiles(t *testing.T) {
	spec, temp := podServiceFixture(t)
	plan, err := BuildMaatPodServicePlan(spec, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(spec.RecipeDirectory, "unrelated.keep")
	if err := os.WriteFile(sentinel, []byte("preserve-me"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallMaatPodServiceRecipe(plan)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Status != "installed_activation_unobserved" || installed.GlobalState != "unknown" || len(installed.ActionsPerformed) != 1 || installed.ActionsPerformed[0] != "created_private_recipe" {
		t.Fatalf("install made an unsupported activation claim: %#v", installed)
	}
	info, err := os.Stat(plan.PlistPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("installed recipe mode/stat = %v, %v", info, err)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "preserve-me" {
		t.Fatalf("unrelated package file changed: %q, %v", got, err)
	}
	if _, err := InstallMaatPodServiceRecipe(plan); err == nil || !strings.Contains(err.Error(), "preserved") {
		t.Fatalf("occupied recipe was not refused with preservation notice: %v", err)
	}
	if got, err := os.ReadFile(plan.PlistPath); err != nil || string(got) != string(plan.Plist) {
		t.Fatalf("existing recipe was overwritten: %v", err)
	}
	if temp.recipeDir != plan.RecipeDirectory {
		t.Fatal("test fixture recipe parent unexpectedly changed")
	}
}

func TestInstallFailureLeavesExplicitCleanupDebtWithoutDeletingLeaf(t *testing.T) {
	spec, _ := podServiceFixture(t)
	plan, err := BuildMaatPodServicePlan(spec, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	installed, err := installMaatPodServiceRecipe(plan, func(file *os.File) error {
		if _, writeErr := file.Write([]byte("partial")); writeErr != nil {
			return writeErr
		}
		return errors.New("injected failure")
	})
	if err == nil || !strings.Contains(err.Error(), "cleanup debt retained") || !strings.Contains(err.Error(), "dev=") {
		t.Fatalf("expected path and identity cleanup debt, got %v", err)
	}
	if installed.Status != "planned_activation_unobserved" {
		t.Fatalf("failed install claimed success: %q", installed.Status)
	}
	got, readErr := os.ReadFile(plan.PlistPath)
	if readErr != nil || string(got) != "partial" {
		t.Fatalf("failed-create leaf was deleted or changed: %q, %v", got, readErr)
	}
}

type podServiceTemp struct{ recipeDir string }

func podServiceFixture(t *testing.T) (MaatPodServiceSpec, podServiceTemp) {
	t.Helper()
	base := canonicalTestDir(t, t.TempDir())
	working := filepath.Join(base, "a&b")
	recipe := filepath.Join(base, "private-recipes")
	if err := os.Mkdir(working, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(recipe, 0o700); err != nil {
		t.Fatal(err)
	}
	working, recipe = canonicalTestDir(t, working), canonicalTestDir(t, recipe)
	now := time.Now().UTC().Truncate(time.Second)
	certPEM, keyPEM, caPEM := makePodServiceTLSFixture(t, now)
	certPath, keyPath, caPath := filepath.Join(base, "server.pem"), filepath.Join(base, "server.key"), filepath.Join(base, "client-ca.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, caPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	delegation := schedule.Delegation{
		Pod: "studio-m5", AuthorityMachine: "native-mac", AuthorityAddrs: []string{"https://100.64.12.5:9443"},
		Resources: []string{"studio"}, Epoch: 3, IssuedAt: now.Add(-time.Minute).Format(time.RFC3339), NotAfter: now.Add(time.Hour).Format(time.RFC3339),
	}
	spec := MaatPodServiceSpec{
		Pod: delegation.Pod, JournalPath: filepath.Join(base, "journal.json"), Listen: "100.64.12.5:9443",
		CertificatePath: certPath, PrivateKeyPath: keyPath, ClientCAPath: caPath,
		ClientBindings:   []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef=agent"},
		WorkingDirectory: working, RecipeDirectory: recipe, HomeDirectory: base,
		Executable: "/Applications/Pantheon.app/Contents/MacOS/sirsi", MachineID: "native-mac", Delegation: delegation, Now: now,
	}
	return spec, podServiceTemp{recipeDir: recipe}
}

func canonicalTestDir(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func makePodServiceTLSFixture(t *testing.T, now time.Time) ([]byte, []byte, []byte) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "100.64.12.5"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IPAddresses: []net.IP{net.ParseIP("100.64.12.5")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
