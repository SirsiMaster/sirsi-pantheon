package verify

import (
	"bytes"
	"crypto/elliptic"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHSIGHappyPaths(t *testing.T) {
	msg := goldenEnvelope().Canonical()
	for _, kc := range []struct {
		name string
		key  func(*testing.T) testKey
	}{
		{"ed25519", newEd25519},
		{"ecdsa-p256", func(t *testing.T) testKey { return newECDSA(t, elliptic.P256()) }},
	} {
		for _, h := range []string{"sha256", "sha512"} {
			t.Run(kc.name+"/"+h, func(t *testing.T) {
				k := kc.key(t)
				sig, err := ParseSignature(signSSHSIG(t, k, msg, sigOpts{hashAlg: h}))
				if err != nil {
					t.Fatal(err)
				}
				if sig.Fingerprint != k.fp || sig.HashAlg != h {
					t.Fatalf("fingerprint/hash mismatch: %s %s", sig.Fingerprint, sig.HashAlg)
				}
				if err := sig.VerifyMessage(msg); err != nil {
					t.Fatal(err)
				}
				wantReason(t, reasonOf(sig.VerifyMessage(append([]byte("x"), msg...))), ReasonSignatureInvalid)
			})
		}
	}
}

func TestParseSignatureRejects(t *testing.T) {
	msg := goldenEnvelope().Canonical()
	ed := newEd25519(t)
	good := string(signSSHSIG(t, ed, msg, sigOpts{}))
	cases := []struct {
		name string
		in   []byte
		want Reason
	}{
		{"wrong namespace", signSSHSIG(t, ed, msg, sigOpts{ns: "sirsi-other"}), ReasonWrongNamespace},
		{"one-char namespace", signSSHSIG(t, ed, msg, sigOpts{ns: "x"}), ReasonWrongNamespace},
		{"rsa key", signSSHSIG(t, newRSA(t), msg, sigOpts{}), ReasonKeyTypeNotAllowed},
		{"ecdsa p384 key", signSSHSIG(t, newECDSA(t, elliptic.P384()), msg, sigOpts{}), ReasonKeyTypeNotAllowed},
		{"sha1 digest", signSSHSIG(t, ed, msg, sigOpts{hashAlg: "sha1"}), ReasonHashNotAllowed},
		{"version 2", signSSHSIG(t, ed, msg, sigOpts{version: 2}), ReasonSignatureMalformed},
		{"bad magic", signSSHSIG(t, ed, msg, sigOpts{magic: "SSHSIX"}), ReasonSignatureMalformed},
		{"reserved not empty", signSSHSIG(t, ed, msg, sigOpts{reserved: "x"}), ReasonSignatureMalformed},
		{"trailing blob data", signSSHSIG(t, ed, msg, sigOpts{trailing: []byte{0}}), ReasonSignatureMalformed},
		{"trailing pubkey data", signSSHSIG(t, ed, msg, sigOpts{pubTrailing: []byte("x")}), ReasonSignatureMalformed},
		{"oversize", []byte(strings.Repeat("A", MaxSignatureArmorBytes+1)), ReasonSignatureOversize},
		{"empty", nil, ReasonSignatureMalformed},
		{"no header", []byte(strings.TrimPrefix(good, "-----BEGIN SSH SIGNATURE-----\n")), ReasonSignatureMalformed},
		{"no footer", []byte(strings.Replace(good, "-----END SSH SIGNATURE-----", "", 1)), ReasonSignatureMalformed},
		{"empty body", []byte("-----BEGIN SSH SIGNATURE-----\n-----END SSH SIGNATURE-----\n"), ReasonSignatureMalformed},
		{"CR in body", []byte(strings.Replace(good, "\n", "\r\n", 2)), ReasonSignatureMalformed},
		{"trailing after footer", []byte(good + "x"), ReasonSignatureMalformed},
		{"bad base64", []byte("-----BEGIN SSH SIGNATURE-----\n!!!!\n-----END SSH SIGNATURE-----\n"), ReasonSignatureMalformed},
		{"truncated blob", []byte("-----BEGIN SSH SIGNATURE-----\nU1NIU0lH\n-----END SSH SIGNATURE-----\n"), ReasonSignatureMalformed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseSignature(c.in)
			wantReason(t, reasonOf(err), c.want)
		})
	}
}

// ---- Cross-check against the real OpenSSH tooling (skipped if absent) ----

func keygen(t *testing.T, kg, dir string, stdin []byte, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(kg, args...)
	// Hermetic: no agent, no real home, so nothing outside dir is read.
	cmd.Env = []string{"HOME=" + dir, "PATH=/usr/bin:/bin"}
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	return cmd.CombinedOutput()
}

func TestSSHSIGCrossCheckRealSSHKeygen(t *testing.T) {
	kg, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not available")
	}
	msg := goldenEnvelope().Canonical()
	for _, kt := range []struct {
		name    string
		args    []string
		allowed bool
	}{
		{"ed25519", []string{"-t", "ed25519"}, true},
		{"ecdsa-p256", []string{"-t", "ecdsa", "-b", "256"}, true},
		{"rsa-2048", []string{"-t", "rsa", "-b", "2048"}, false},
		{"ecdsa-p384", []string{"-t", "ecdsa", "-b", "384"}, false},
	} {
		t.Run(kt.name, func(t *testing.T) {
			dir := t.TempDir()
			keyfile := filepath.Join(dir, "testonly_key")
			if out, err := keygen(t, kg, dir, nil, append([]string{"-q", "-N", "", "-C", "test-only", "-f", keyfile}, kt.args...)...); err != nil {
				t.Fatalf("keygen: %v: %s", err, out)
			}
			pubRaw, _ := os.ReadFile(keyfile + ".pub")
			pub, _, _, _, err := ssh.ParseAuthorizedKey(pubRaw)
			if err != nil {
				t.Fatal(err)
			}
			sign := func(name string, extra ...string) []byte {
				p := filepath.Join(dir, name)
				if err := os.WriteFile(p, msg, 0o600); err != nil {
					t.Fatal(err)
				}
				args := append([]string{"-Y", "sign", "-f", keyfile}, extra...)
				if out, err := keygen(t, kg, dir, nil, append(args, p)...); err != nil {
					t.Fatalf("sign: %v: %s", err, out)
				}
				b, err := os.ReadFile(p + ".sig")
				if err != nil {
					t.Fatal(err)
				}
				return b
			}

			armored := sign("m1", "-n", Namespace)
			sig, perr := ParseSignature(armored)
			if !kt.allowed {
				wantReason(t, reasonOf(perr), ReasonKeyTypeNotAllowed)
				return
			}
			if perr != nil {
				t.Fatal(perr)
			}
			if sig.Fingerprint != ssh.FingerprintSHA256(pub) {
				t.Fatalf("fingerprint %s != %s", sig.Fingerprint, ssh.FingerprintSHA256(pub))
			}
			if err := sig.VerifyMessage(msg); err != nil {
				t.Fatalf("real ssh-keygen signature rejected: %v", err)
			}
			if err := sig.VerifyMessage(append(append([]byte(nil), msg...), ' ')); err == nil {
				t.Fatal("tampered message verified")
			}
			t.Logf("real ssh-keygen -Y sign (%s, default digest %s) verified", kt.name, sig.HashAlg)

			// Wrong namespace from the real tool.
			_, werr := ParseSignature(sign("m2", "-n", "file"))
			wantReason(t, reasonOf(werr), ReasonWrongNamespace)

			// Explicit sha256 digest, if this OpenSSH supports selecting it.
			p3 := filepath.Join(dir, "m3")
			_ = os.WriteFile(p3, msg, 0o600)
			if out, err := keygen(t, kg, dir, nil, "-Y", "sign", "-f", keyfile, "-n", Namespace, "-O", "hashalg=sha256", p3); err != nil {
				t.Logf("this ssh-keygen cannot select hashalg=sha256 (%v: %s); sha256 covered by builder tests", err, out)
			} else {
				b, _ := os.ReadFile(p3 + ".sig")
				s256, err := ParseSignature(b)
				if err != nil || s256.HashAlg != "sha256" || s256.VerifyMessage(msg) != nil {
					t.Fatalf("real sha256 SSHSIG rejected: %v", err)
				}
			}

			// End to end: real signature through Admit.
			f := newFixture(t)
			f.setGrants([]byte(grantLine(testKey{fp: sig.Fingerprint}, f.uid, "restart-broker", false)), nil)
			env, err := ParseEnvelope(msg)
			if err != nil {
				t.Fatal(err)
			}
			if env.NodeID != defaultNode {
				t.Fatal("fixture node mismatch")
			}
			f.cfg.Verbs[VerbRestartBroker] = VerbIdentity{Target: env.Target, ConfigDigest: env.ConfigDigest}
			f.rebuild()
			d := f.v.Admit(Request{Envelope: msg, Signature: armored, PeerUID: f.uid}, f.pol)
			if !d.Allowed {
				t.Fatalf("real signature not admitted: %s (%s)", d.Reason, d.Detail)
			}
		})
	}
}

// The inverse check: the real tool must accept what the test builder emits,
// which proves the builder (and therefore the negative tests) speaks real
// SSHSIG.
func TestRealSSHKeygenAcceptsBuilderOutput(t *testing.T) {
	kg, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not available")
	}
	msg := goldenEnvelope().Canonical()
	for name, k := range map[string]testKey{"ed25519": newEd25519(t), "ecdsa-p256": newECDSA(t, elliptic.P256())} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			sigPath := filepath.Join(dir, "x.sig")
			if err := os.WriteFile(sigPath, signSSHSIG(t, k, msg, sigOpts{}), 0o600); err != nil {
				t.Fatal(err)
			}
			if out, err := keygen(t, kg, dir, msg, "-Y", "check-novalidate", "-n", Namespace, "-s", sigPath); err != nil {
				t.Fatalf("real ssh-keygen rejected builder signature: %v: %s", err, out)
			}
			if _, err := keygen(t, kg, dir, msg, "-Y", "check-novalidate", "-n", "other", "-s", sigPath); err == nil {
				t.Fatal("real ssh-keygen accepted wrong namespace")
			}
		})
	}
}
