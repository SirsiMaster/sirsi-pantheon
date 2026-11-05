package signing_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Transport fixtures never contact a signer, read credentials or sign anything.
// They execute the real client with fake transport/verification programs.
func signingFixture(t *testing.T, source []byte, app bool, mode string) (string, string, string, error) {
	hosts := "fixture@signer.invalid"
	return signingFixtureWithHosts(t, source, app, mode, &hosts)
}

func signingFixtureWithHosts(t *testing.T, source []byte, app bool, mode string, hosts *string) (string, string, string, error) {
	return signingFixtureWithHostsAndID(t, source, app, mode, hosts, "11111111-2222-4333-8444-555555555555")
}

func signingFixtureWithHostsAndID(t *testing.T, source []byte, app bool, mode string, hosts *string, jobID string) (string, string, string, error) {
	t.Helper()
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("signing client requires zsh")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	name := "artifact.bin"
	if app {
		name = "Demo.app"
	}
	input := filepath.Join(root, name)
	if app {
		if err := os.Mkdir(input, 0o700); err != nil {
			t.Fatal(err)
		}
	} else {
		write(input, "unsigned fixture", 0o600)
	}
	transport := `#!/bin/sh
set -eu
op=${0##*/}
printf '%s %s\n' "$op" "$*" >> "$SIGN_FIXTURE_LOG"
for required in ConnectTimeout=10 BatchMode=yes StrictHostKeyChecking=yes PasswordAuthentication=no KbdInteractiveAuthentication=no ServerAliveInterval=15 ServerAliveCountMax=4; do
  case " $* " in *"-o $required "*) ;; *) echo "UNSAFE_TRANSPORT $op $required" >> "$SIGN_FIXTURE_LOG"; exit 93;; esac
done
[ "$SIGN_FIXTURE_MODE" != deny ] || exit 255
case "$op:$*" in ssh:*"mv ~/sirsi-sign/inbox/"*) [ "$SIGN_FIXTURE_MODE" != ambiguous-enqueue ] || exit 255;; esac
# the first host's upload fails or hangs; the second host is healthy
case "$SIGN_FIXTURE_MODE:$op:$*" in
  upload-fail-first:scp:*"first@signer.invalid:sirsi-sign/inbox/"*) exit 1;;
  hang-upload-first:scp:*"first@signer.invalid:sirsi-sign/inbox/"*) exec sleep 30;;
esac
for last; do :; done
case "$op:$*" in
  ssh:*"echo EXISTS"*) if [ "$SIGN_FIXTURE_MODE" = id-exists ]; then echo EXISTS; else echo STAGED; fi;;
  ssh:*"echo QUEUED"*) [ "$SIGN_FIXTURE_MODE" != ambiguous-enqueue ] || exit 255; echo QUEUED;;
  ssh:*"ls ~/sirsi-sign/outbox/"*) printf '%s\n' "$SIGN_FIXTURE_NAME";;
  rsync:*":sirsi-sign/outbox/"*)
    case "$SIGN_FIXTURE_NAME" in *.app) mkdir "$last/$SIGN_FIXTURE_NAME";; *) printf signed > "$last/$SIGN_FIXTURE_NAME";; esac;;
  scp:*"/receipt.txt"*) printf '%s\n' '--- verification' 'fixture only' > "$last";;
esac
`
	for _, op := range []string{"ssh", "scp", "rsync"} {
		write(filepath.Join(bin, op), transport, 0o700)
	}
	write(filepath.Join(bin, "codesign"), `#!/bin/sh
if [ "$1" = -dvv ]; then
  if [ "$SIGN_FIXTURE_MODE" = bad-team ]; then echo 'TeamIdentifier=FOREIGN' >&2
  else printf '%s\n' 'TeamIdentifier=9D382WV988' 'Authority=Developer ID Application: Sirsi Technologies Inc. (9D382WV988)' >&2; fi
fi
`, 0o700)
	client := filepath.Join(root, "sirsi-sign")
	write(client, string(source), 0o700)
	log := filepath.Join(root, "transport.log")
	custody := filepath.Join(root, "custody")
	if err := os.Mkdir(custody, 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zsh, "-f", client, input)
	// A closed fixture environment prevents inherited real signing host or
	// transport configuration from crossing the test boundary.
	cmd.Env = []string{
		"PATH=" + bin + ":/usr/bin:/bin:/usr/sbin:/sbin", "TMPDIR=" + root,
		"HOME=" + root, "ZDOTDIR=" + root,
		"SIRSI_SIGN_TIMEOUT=5", "SIRSI_SIGN_TRANSFER_TIMEOUT=3",
		"SIRSI_SIGN_JOB_ID=" + jobID,
		"SIRSI_SIGN_CUSTODY_DIR=" + custody,
		"SIGN_FIXTURE_LOG=" + log, "SIGN_FIXTURE_NAME=" + name, "SIGN_FIXTURE_MODE=" + mode,
	}
	if hosts != nil {
		cmd.Env = append(cmd.Env, "SIRSI_SIGN_HOSTS="+*hosts)
	}
	out, runErr := cmd.CombinedOutput()
	logged, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) { // no transport log: the client refused before contacting any host
		t.Fatal(err)
	}
	return string(out), string(logged), filepath.Join(root, "signed-"+name), runErr
}

func TestSignerAmbiguousEnqueueRetainsExactIntent(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	_, log, signed, runErr := signingFixture(t, source, false, "ambiguous-enqueue")
	if runErr == nil || !strings.Contains(log, "mv ~/sirsi-sign/inbox/") {
		t.Fatalf("fixture did not reach ambiguous enqueue: %v %s", runErr, log)
	}
	intent := filepath.Join(filepath.Dir(signed), "custody", "11111111-2222-4333-8444-555555555555.json")
	data, err := os.ReadFile(intent)
	if err != nil || !strings.Contains(string(data), `"state": "enqueue-unknown"`) || !strings.Contains(string(data), `"input_sha256":`) {
		t.Fatalf("exact intent not retained: %v %s", err, data)
	}
	if strings.Contains(log, "rm -rf ~/sirsi-sign/outbox/") {
		t.Fatalf("uncertain outcome was deleted: %s", log)
	}
}

func TestSignerRejectsInvalidCallerJobIDBeforeEnqueue(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	hosts := "fixture@signer.invalid"
	out, log, signed, runErr := signingFixtureWithHostsAndID(t, source, false, "ok", &hosts, "../not-a-job")
	exit, ok := runErr.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 || !strings.Contains(out, "invalid canonical job UUID") {
		t.Fatalf("invalid job ID was not refused: %v %s", runErr, out)
	}
	if strings.Contains(log, "mkdir ~/sirsi-sign/inbox/") || strings.Contains(log, "scp ") || strings.Contains(log, "rsync ") {
		t.Fatalf("invalid ID reached enqueue: %s", log)
	}
	if _, err := os.Stat(signed); !os.IsNotExist(err) {
		t.Fatalf("invalid ID produced signed output: %v", err)
	}
}

func signerSelectionMatches(log string, hosts []string) bool {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != len(hosts) {
		return false
	}
	for i, host := range hosts {
		if !strings.HasPrefix(lines[i], "ssh ") || !strings.Contains(lines[i], " "+host+" test -x ~/sirsi-sign/bin/sirsi-signd.sh ") {
			return false
		}
	}
	return !strings.Contains(log, "UNSAFE_TRANSPORT")
}

func TestSignerDefaultAliasesAndExplicitOverrides(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	empty, override := "", "first@signer.invalid second@signer.invalid"
	for _, tc := range []struct {
		name  string
		hosts *string
		want  []string
	}{
		{"unset-default", nil, []string{"sirsi-m5", "sirsi-m1"}},
		{"empty-default", &empty, []string{"sirsi-m5", "sirsi-m1"}},
		{"explicit-override", &override, []string{"first@signer.invalid", "second@signer.invalid"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, log, signed, runErr := signingFixtureWithHosts(t, source, false, "deny", tc.hosts)
			exit, ok := runErr.(*exec.ExitError)
			if !ok || exit.ExitCode() != 4 || !signerSelectionMatches(log, tc.want) {
				t.Fatalf("wrong ordered strict signer selection: %v\n%s\n%s", runErr, out, log)
			}
			if strings.Contains(log, "mkdir ~/sirsi-sign/inbox/") || strings.Contains(log, "scp ") || strings.Contains(log, "rsync ") {
				t.Fatalf("unreachable signer started a job: %s", log)
			}
			if _, err := os.Stat(signed); !os.IsNotExist(err) {
				t.Fatalf("unreachable signers produced output: %v", err)
			}
		})
	}
}

func TestSignerSelectionNegativeControlRejectsDHCPDefaults(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	mutant := strings.Replace(string(source), "SIRSI_SIGN_HOSTS:-sirsi-m5 sirsi-m1", "SIRSI_SIGN_HOSTS:-thekryptodragon@192.168.1.155 sirsimasterdev@192.168.1.180", 1)
	if mutant == string(source) {
		t.Fatal("negative control did not restore the former DHCP defaults")
	}
	_, log, _, runErr := signingFixtureWithHosts(t, []byte(mutant), false, "deny", nil)
	exit, ok := runErr.(*exec.ExitError)
	if !ok || exit.ExitCode() != 4 || !signerSelectionMatches(log, []string{"thekryptodragon@192.168.1.155", "sirsimasterdev@192.168.1.180"}) {
		t.Fatalf("negative control did not actually exercise old defaults: %v %s", runErr, log)
	}
	if signerSelectionMatches(log, []string{"sirsi-m5", "sirsi-m1"}) {
		t.Fatal("former DHCP defaults accepted as enrolled aliases")
	}
}

func TestAllSigningTransportPhasesArePinnedAndNoninteractive(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "app"}[app], func(t *testing.T) {
			out, log, signed, err := signingFixture(t, source, app, "ok")
			if err != nil || strings.Contains(log, "UNSAFE_TRANSPORT") {
				t.Fatalf("transport failed: %v\n%s\n%s", err, out, log)
			}
			if _, err := os.Stat(signed); err != nil {
				t.Fatal("verified fixture result not returned", err)
			}
			for _, phase := range []string{"ssh ", "scp ", "rsync ", "mkdir ~/sirsi-sign/inbox/", "mv ~/sirsi-sign/inbox/", "ls ~/sirsi-sign/outbox/", "11111111-2222-4333-8444-555555555555"} {
				if !strings.Contains(log, phase) {
					t.Errorf("missing exercised phase %q: %s", phase, log)
				}
			}
			if strings.Contains(log, "rm -rf ~/sirsi-sign/outbox/") {
				t.Fatalf("terminal evidence was deleted: %s", log)
			}
		})
	}
}

func TestSignerRefusalDoesNotPublishOrLearnKeys(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	_, log, signed, err := signingFixture(t, source, false, "deny")
	if err == nil || strings.Contains(log, "mkdir ~/sirsi-sign/inbox/") || strings.Contains(log, "scp ") || strings.Contains(log, "rsync ") {
		t.Fatalf("refused signer reached publication: %v %s", err, log)
	}
	if _, err := os.Stat(signed); !os.IsNotExist(err) {
		t.Fatalf("refused signer returned an artifact: %v", err)
	}
}

func TestForeignSignatureDoesNotReplaceOutput(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	_, _, signed, err := signingFixture(t, source, false, "bad-team")
	if err == nil {
		t.Fatal("foreign signature accepted")
	}
	if _, err := os.Stat(signed); !os.IsNotExist(err) {
		t.Fatalf("foreign signed artifact published: %v", err)
	}
}

// Failover: a signer whose upload fails or hangs gets the job's hidden staging removed (never enqueued), and the SAME
// job ID goes to the next signer. Each attempt is recorded beside the custody record.
func TestSignerFailsOverOnlyBeforeEnqueue(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"upload-fail-first", "hang-upload-first"} {
		t.Run(mode, func(t *testing.T) {
			hosts := "first@signer.invalid second@signer.invalid"
			start := time.Now()
			out, log, signed, runErr := signingFixtureWithHosts(t, source, false, mode, &hosts)
			if runErr != nil {
				t.Fatalf("no failover: %v\n%s\n%s", runErr, out, log)
			}
			if mode == "hang-upload-first" && time.Since(start) > 20*time.Second {
				t.Fatalf("a hung upload was not bounded: %s", time.Since(start))
			}
			id := "11111111-2222-4333-8444-555555555555"
			if !strings.Contains(log, "first@signer.invalid rm -rf ~/sirsi-sign/inbox/."+id) {
				t.Errorf("the failed host's hidden staging was not removed: %s", log)
			}
			if strings.Contains(log, "first@signer.invalid mv ~/sirsi-sign/inbox/") {
				t.Errorf("the failed host was enqueued anyway: %s", log)
			}
			if !strings.Contains(log, "second@signer.invalid mv ~/sirsi-sign/inbox/."+id+" ~/sirsi-sign/inbox/"+id) {
				t.Errorf("the same job ID was not enqueued on the next signer: %s", log)
			}
			if _, err := os.Stat(signed); err != nil {
				t.Errorf("no verified result after failover: %v", err)
			}
			att, err := os.ReadFile(filepath.Join(filepath.Dir(signed), "custody", id+".attempts"))
			if err != nil || !strings.Contains(string(att), "host=first@signer.invalid result=not-enqueued") || !strings.Contains(string(att), "host=second@signer.invalid result=enqueued") {
				t.Errorf("attempts not recorded: %v %s", err, att)
			}
		})
	}
}

// An enqueue whose outcome is unknown must stop (exit 6), never fail over: the first signer may already be signing it.
func TestSignerAmbiguousEnqueueNeverFailsOver(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	hosts := "first@signer.invalid second@signer.invalid"
	_, log, _, runErr := signingFixtureWithHosts(t, source, false, "ambiguous-enqueue", &hosts)
	exit, ok := runErr.(*exec.ExitError)
	if !ok || exit.ExitCode() != 6 {
		t.Fatalf("ambiguous enqueue did not stop with 6: %v %s", runErr, log)
	}
	if strings.Contains(log, "second@signer.invalid") && strings.Contains(log, "second@signer.invalid mkdir") {
		t.Fatalf("ambiguous enqueue failed over (double-sign risk): %s", log)
	}
	for _, l := range strings.Split(log, "\n") {
		if strings.Contains(l, "second@signer.invalid") && (strings.Contains(l, "echo STAGED") || strings.HasPrefix(l, "scp ")) {
			t.Fatalf("second signer was staged after an ambiguous enqueue: %s", log)
		}
	}
}

// A job ID already present on a signer stops (exit 7) before any upload.
func TestSignerExistingJobIDStops(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	out, log, signed, runErr := signingFixture(t, source, false, "id-exists")
	exit, ok := runErr.(*exec.ExitError)
	if !ok || exit.ExitCode() != 7 || !strings.Contains(out, "already exists") {
		t.Fatalf("existing job ID not refused: %v %s", runErr, out)
	}
	if strings.Contains(log, "scp ") || strings.Contains(log, "rsync ") {
		t.Fatalf("existing job ID reached upload: %s", log)
	}
	if _, err := os.Stat(signed); !os.IsNotExist(err) {
		t.Fatal("existing job ID produced output")
	}
}

// Negative control: without ServerAlive the fixture refuses the transport, so the keep-alive requirement is real.
func TestTransportNegativeControlRequiresServerAlive(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	mutant := strings.Replace(string(source), "-o ServerAliveInterval=15 ", "", 1)
	if mutant == string(source) {
		t.Fatal("negative control did not remove ServerAlive")
	}
	_, log, _, err := signingFixture(t, []byte(mutant), false, "ok")
	if err == nil || !strings.Contains(log, "UNSAFE_TRANSPORT") {
		t.Fatalf("transport without ServerAlive escaped the fixture: %v %s", err, log)
	}
}

func TestTransportNegativeControlRejectsAcceptNew(t *testing.T) {
	source, err := os.ReadFile("sirsi-sign")
	if err != nil {
		t.Fatal(err)
	}
	mutant := strings.ReplaceAll(string(source), "StrictHostKeyChecking=yes", "StrictHostKeyChecking=accept-new")
	if mutant == string(source) {
		t.Fatal("negative control did not mutate the policy")
	}
	_, log, _, err := signingFixture(t, []byte(mutant), false, "ok")
	if err == nil || !strings.Contains(log, "UNSAFE_TRANSPORT") {
		t.Fatalf("old permissive transport escaped the fixture: %v %s", err, log)
	}
}
