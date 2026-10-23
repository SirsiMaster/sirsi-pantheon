package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type readerOnly struct{ io.Reader }

func TestCloudChildOverridesInheritedCredentialLogging(t *testing.T) {
	t.Setenv("CLOUDSDK_CORE_LOG_HTTP", "true")
	t.Setenv("CLOUDSDK_CORE_DISABLE_FILE_LOGGING", "false")
	cmd := cloudCommand(context.Background(), "secrets", "versions", "access", "latest", "--secret=sirsi-router-bootstrap-token")
	counts := map[string]int{}
	for _, entry := range cmd.Env {
		key, value, _ := strings.Cut(entry, "=")
		if key == "CLOUDSDK_CORE_LOG_HTTP" || key == "CLOUDSDK_CORE_DISABLE_FILE_LOGGING" {
			counts[key]++
			if (key == "CLOUDSDK_CORE_LOG_HTTP" && value != "false") || (key == "CLOUDSDK_CORE_DISABLE_FILE_LOGGING" && value != "true") {
				t.Fatalf("unsafe child logging setting %s", key)
			}
		}
	}
	if counts["CLOUDSDK_CORE_LOG_HTTP"] != 1 || counts["CLOUDSDK_CORE_DISABLE_FILE_LOGGING"] != 1 {
		t.Fatal("missing or ambiguous child logging overrides")
	}
	if os.Getenv("CLOUDSDK_CORE_LOG_HTTP") != "true" || os.Getenv("CLOUDSDK_CORE_DISABLE_FILE_LOGGING") != "false" {
		t.Fatal("operator changed parent logging configuration")
	}
	if cmd.Path != "/opt/homebrew/bin/gcloud" {
		t.Fatal("operator executable changed")
	}
}

func TestCommandOutputBoundCannotBypassWrite(t *testing.T) {
	var output boundedOutput
	_, err := io.Copy(&output, readerOnly{strings.NewReader(strings.Repeat("x", 128<<10))})
	if err == nil || output.Len() > 64<<10 {
		t.Fatalf("output bound bypassed: retained=%d err=%v", output.Len(), err)
	}
}

func fixture(t *testing.T) (options, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 23, 18, 0, 0, 0, time.UTC)
	req := routerstore.MailboxReconcileRequest{Action: "approve", Manifest: &routerstore.MailboxManifest{
		ID: "finite-test", Actor: "sirsi-software-admin", Recipient: "horus", ApprovalRef: "owner:test", EvidenceRef: "file:test", EvidenceSHA256: strings.Repeat("a", 64), ExpiresAt: now.Add(time.Hour),
		Notices: []routerstore.MailboxNotice{{ID: "exact-test", ExpectedSHA256: strings.Repeat("b", 64), Result: "Historical coordination only; product task remains open."}},
	}}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return options{path: path, digest: hex.EncodeToString(hash[:]), revision: "sirsi-router-00059-caz", image: strings.Repeat("c", 64)}, now
}

func TestDryRunNeverAccessesOperatorCredential(t *testing.T) {
	opts, now := fixture(t)
	var out bytes.Buffer
	err := run(context.Background(), opts, dependencies{now: func() time.Time { return now }, cloud: func(context.Context, ...string) ([]byte, error) {
		t.Fatal("cloud invoked during dry run")
		return nil, nil
	}}, &out)
	if err != nil || !strings.Contains(out.String(), `"apply":false`) {
		t.Fatalf("dry run: %s %v", out.String(), err)
	}
}

func TestPlanRejectionsPrecedeAnyCloudAccess(t *testing.T) {
	for _, name := range []string{"digest", "public", "symlink", "expired", "unknown-field", "foreign-actor", "ack-action", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			opts, now := fixture(t)
			data, err := os.ReadFile(opts.path)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "digest":
				opts.digest = strings.Repeat("0", 64)
			case "public":
				if err := os.Chmod(opts.path, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				alias := filepath.Join(filepath.Dir(opts.path), "alias.json")
				if err := os.Symlink(opts.path, alias); err != nil {
					t.Fatal(err)
				}
				opts.path = alias
			case "expired":
				now = now.Add(2 * time.Hour)
			default:
				var req routerstore.MailboxReconcileRequest
				if err := json.Unmarshal(data, &req); err != nil {
					t.Fatal(err)
				}
				if name == "foreign-actor" {
					req.Manifest.Actor = "owner"
				}
				if name == "ack-action" {
					req.Action = "ack"
				}
				if name == "duplicate" {
					req.Manifest.Notices = append(req.Manifest.Notices, req.Manifest.Notices[0])
				}
				data, err = json.Marshal(req)
				if err != nil {
					t.Fatal(err)
				}
				if name == "unknown-field" {
					data = append([]byte(`{"unknown":true,`), data[1:]...)
				}
				if err := os.WriteFile(opts.path, data, 0600); err != nil {
					t.Fatal(err)
				}
				hash := sha256.Sum256(data)
				opts.digest = hex.EncodeToString(hash[:])
			}
			opts.apply = true
			err = run(context.Background(), opts, dependencies{now: func() time.Time { return now }, cloud: func(context.Context, ...string) ([]byte, error) {
				t.Fatal("cloud invoked for invalid plan")
				return nil, nil
			}}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("invalid plan accepted")
			}
		})
	}
}

func TestApplyIsRouteImageAndCredentialBound(t *testing.T) {
	for _, variant := range []string{"mixed-route", "wrong-image", "not-ready", "credential-denied", "approve-denied", "valid", "expired-at-submit", "route-changed-during-secret"} {
		t.Run(variant, func(t *testing.T) {
			opts, now := fixture(t)
			opts.apply = true
			t.Setenv("SIRSI_AGENT_ID", "sirsi-software-admin")
			t.Setenv("SIRSI_THREAD_ID", "own-test-thread")
			secretCalls, approveCalls, clocks := 0, 0, 0
			const secret = "test-only-operator-secret-never-output"
			var out bytes.Buffer
			deps := dependencies{now: func() time.Time {
				clocks++
				if variant == "expired-at-submit" && clocks > 1 {
					return now.Add(2 * time.Hour)
				}
				return now
			}}
			deps.cloud = func(_ context.Context, args ...string) ([]byte, error) {
				switch args[0] + "/" + args[1] {
				case "run/services":
					revision, ready := opts.revision, "True"
					if variant == "route-changed-during-secret" && secretCalls > 0 {
						return []byte(`{"metadata":{"generation":62},"status":{"observedGeneration":62,"traffic":[{"revisionName":"sirsi-router-old","percent":100}],"conditions":[{"type":"Ready","status":"True"},{"type":"RoutesReady","status":"True"}]}}`), nil
					}
					if variant == "mixed-route" {
						revision = "sirsi-router-old"
					}
					if variant == "not-ready" {
						ready = "False"
					}
					return []byte(`{"metadata":{"generation":61},"status":{"observedGeneration":61,"traffic":[{"revisionName":"` + opts.revision + `","percent":100},{"revisionName":"` + revision + `","tag":"historical"}],"conditions":[{"type":"Ready","status":"` + ready + `"},{"type":"RoutesReady","status":"True"}]}}`), nil
				case "run/revisions":
					digest := opts.image
					if variant == "wrong-image" {
						digest = strings.Repeat("d", 64)
					}
					return []byte("us-central1-docker.pkg.dev/sirsi-nexus-live/cloud-run-source-deploy/sirsi-router@sha256:" + digest), nil
				case "secrets/versions":
					secretCalls++
					if variant == "credential-denied" {
						return nil, errors.New(secret)
					}
					return []byte(secret), nil
				default:
					t.Fatalf("unexpected operator command %q", args)
					return nil, nil
				}
			}
			deps.approve = func(_ context.Context, token string, req routerstore.MailboxReconcileRequest) (routerstore.MailboxReconcileResult, error) {
				approveCalls++
				if token != secret || req.Action != "approve" || req.Manifest.Recipient != "horus" {
					t.Fatal("wrong credential or action")
				}
				if variant == "approve-denied" {
					return routerstore.MailboxReconcileResult{}, errors.New(secret)
				}
				return routerstore.MailboxReconcileResult{ManifestID: req.Manifest.ID}, nil
			}
			err := run(context.Background(), opts, deps, &out)
			if variant == "valid" {
				if err != nil || approveCalls != 1 || secretCalls != 1 {
					t.Fatalf("valid: %v %d %d", err, approveCalls, secretCalls)
				}
			} else if err == nil {
				t.Fatal("unsafe application accepted")
			}
			if variant == "mixed-route" || variant == "wrong-image" || variant == "not-ready" {
				if secretCalls != 0 {
					t.Fatal("credential read before route qualification")
				}
			}
			if variant == "expired-at-submit" && approveCalls != 0 {
				t.Fatal("expired plan submitted")
			}
			if variant == "route-changed-during-secret" && approveCalls != 0 {
				t.Fatal("changed route accepted after credential fetch")
			}
			if strings.Contains(out.String(), secret) || (err != nil && strings.Contains(err.Error(), secret)) {
				t.Fatal("credential leaked")
			}
		})
	}
}
