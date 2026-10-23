// router-notice-operator provisions only an exact reviewed Horus notice plan.
// It is an operator maintenance tool, not a lane consumer or token exporter.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"golang.org/x/sys/unix"
)

const canonicalURL = "https://sirsi-router-6kdf4or4qq-uc.a.run.app"
const deployAccount = "claude-agent@sirsi-nexus-live.iam.gserviceaccount.com"

type options struct {
	path, digest, revision, image string
	apply                         bool
}

type dependencies struct {
	cloud   func(context.Context, ...string) ([]byte, error)
	approve func(context.Context, string, routerstore.MailboxReconcileRequest) (routerstore.MailboxReconcileResult, error)
	now     func() time.Time
}

func main() {
	var opts options
	flag.StringVar(&opts.path, "request", "", "Absolute private regular request JSON path")
	flag.StringVar(&opts.digest, "sha256", "", "Exact independently reviewed request SHA256")
	flag.StringVar(&opts.revision, "revision", "", "Exact qualified canonical revision")
	flag.StringVar(&opts.image, "image-sha256", "", "Exact qualified image digest")
	flag.BoolVar(&opts.apply, "apply", false, "Approve this finite plan through the existing operator authority")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	err := run(ctx, opts, dependencies{cloud: runCloud, now: time.Now, approve: func(ctx context.Context, token string, req routerstore.MailboxReconcileRequest) (routerstore.MailboxReconcileResult, error) {
		store := routerstore.NewMemoryOnlyRemoteStore(canonicalURL, token)
		defer func() { _ = store.Close() }()
		return store.ReconcileMailboxNotice(ctx, req)
	}}, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readPlan(path, expected string, now time.Time) (routerstore.MailboxReconcileRequest, error) {
	var req routerstore.MailboxReconcileRequest
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !isDigest(expected) {
		return req, errors.New("operator: canonical request path and exact reviewed digest required")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return req, errors.New("operator: private regular request could not be opened")
	}
	f := os.NewFile(uintptr(fd), path)
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return req, errors.New("operator: request must be a private regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return req, errors.New("operator: request ownership or link count refused")
	}
	data, err := io.ReadAll(io.LimitReader(f, (128<<10)+1))
	if err != nil || len(data) > 128<<10 {
		return req, errors.New("operator: bounded request could not be read")
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != expected {
		return req, errors.New("operator: reviewed request digest mismatch")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&req) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return req, errors.New("operator: closed approval request schema required")
	}
	m := req.Manifest
	if req.Action != "approve" || req.ManifestID != "" || req.NoticeID != "" || m == nil || m.ID == "" || len(m.ID) > 100 || m.Actor != "sirsi-software-admin" || m.Recipient != "horus" || m.ApprovalRef == "" || m.EvidenceRef == "" || !isDigest(m.EvidenceSHA256) || len(m.Notices) < 1 || len(m.Notices) > 64 || !m.ExpiresAt.After(now) || m.ExpiresAt.After(now.Add(24*time.Hour)) {
		return req, errors.New("operator: finite unexpired SSA/Horus approval plan required")
	}
	seen := map[string]bool{}
	for _, notice := range m.Notices {
		if notice.ID == "" || seen[notice.ID] || !isDigest(notice.ExpectedSHA256) || strings.TrimSpace(notice.Result) == "" || len(notice.Result) > 8192 {
			return req, errors.New("operator: unique exact coordination notices required")
		}
		seen[notice.ID] = true
	}
	return req, nil
}

func isDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && strings.ToLower(value) == value
}

func run(ctx context.Context, opts options, deps dependencies, out io.Writer) error {
	req, err := readPlan(opts.path, opts.digest, deps.now())
	if err != nil {
		return err
	}
	if !opts.apply {
		return json.NewEncoder(out).Encode(map[string]any{"apply": false, "request_sha256": opts.digest, "notice_count": len(req.Manifest.Notices)})
	}
	if os.Getenv("SIRSI_AGENT_ID") != "sirsi-software-admin" || os.Getenv("SIRSI_THREAD_ID") == "" {
		return errors.New("operator: actual SSA runtime and own thread required; no owner or service impersonation")
	}
	if !regexp.MustCompile(`^sirsi-router-[a-z0-9-]+$`).MatchString(opts.revision) || !isDigest(opts.image) {
		return errors.New("operator: exact qualified revision and image required")
	}
	validateRoutes := func() (int, error) {
		service, err := deps.cloud(ctx, "run", "services", "describe", "sirsi-router", "--region=us-central1", "--format=json(metadata.generation,status.observedGeneration,status.traffic,status.conditions)")
		if err != nil {
			return 0, errors.New("operator: serving-route readback failed")
		}
		var state struct {
			Metadata struct{ Generation int } `json:"metadata"`
			Status   struct {
				ObservedGeneration int `json:"observedGeneration"`
				Traffic            []struct {
					Revision string `json:"revisionName"`
					Percent  int    `json:"percent"`
				} `json:"traffic"`
				Conditions []struct{ Type, Status string } `json:"conditions"`
			} `json:"status"`
		}
		if json.Unmarshal(service, &state) != nil || state.Metadata.Generation < 1 || state.Metadata.Generation != state.Status.ObservedGeneration || len(state.Status.Traffic) == 0 || !ready(state.Status.Conditions) {
			return 0, errors.New("operator: current ready serving configuration required")
		}
		percent := 0
		for _, route := range state.Status.Traffic {
			if route.Revision != opts.revision || route.Percent < 0 {
				return 0, errors.New("operator: an authoritative route still selects another revision")
			}
			percent += route.Percent
		}
		if percent != 100 {
			return 0, errors.New("operator: sole qualified revision must serve all traffic")
		}
		return state.Metadata.Generation, nil
	}
	generation, err := validateRoutes()
	if err != nil {
		return err
	}
	image, err := deps.cloud(ctx, "run", "revisions", "describe", opts.revision, "--region=us-central1", "--format=value(status.imageDigest)")
	if err != nil || strings.TrimSpace(string(image)) != "us-central1-docker.pkg.dev/sirsi-nexus-live/cloud-run-source-deploy/sirsi-router@sha256:"+opts.image {
		return errors.New("operator: qualified image readback mismatch before credential access")
	}
	// The existing operator credential remains only in this process and the
	// authenticated SDK call. Never export it to a lane's env, argv or file.
	secret, err := deps.cloud(ctx, "secrets", "versions", "access", "latest", "--secret=sirsi-router-bootstrap-token")
	if err != nil || len(secret) == 0 || len(secret) > 4096 {
		return errors.New("operator: existing operator credential unavailable; no fallback or permission change")
	}
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()
	if !req.Manifest.ExpiresAt.After(deps.now()) || ctx.Err() != nil {
		return errors.New("operator: approval expired or canceled before submission")
	}
	currentGeneration, err := validateRoutes()
	if err != nil || currentGeneration != generation {
		return errors.New("operator: authoritative route configuration changed before submission")
	}
	result, err := deps.approve(ctx, strings.TrimSpace(string(secret)), req)
	if err != nil {
		return errors.New("operator: exact finite approval refused; credential was not exported or persisted")
	}
	return json.NewEncoder(out).Encode(result)
}

func ready(conditions []struct{ Type, Status string }) bool {
	ready, routes := false, false
	for _, condition := range conditions {
		if condition.Type == "Ready" && condition.Status == "True" {
			ready = true
		}
		if condition.Type == "RoutesReady" && condition.Status == "True" {
			routes = true
		}
	}
	return ready && routes
}

// Do not embed bytes.Buffer: its promoted ReadFrom would bypass Write's cap
// when os/exec copies a child's stdout through io.Copy.
type boundedOutput struct{ data bytes.Buffer }

func (b *boundedOutput) Len() int      { return b.data.Len() }
func (b *boundedOutput) Bytes() []byte { return b.data.Bytes() }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > 64<<10-b.Len() {
		return 0, errors.New("operator output bound exceeded")
	}
	return b.data.Write(p)
}

func runCloud(ctx context.Context, args ...string) ([]byte, error) {
	cmd := cloudCommand(ctx, args...)
	var out boundedOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, errors.New("existing operator command failed")
	}
	return out.Bytes(), nil
}

func cloudCommand(ctx context.Context, args ...string) *exec.Cmd {
	args = append(args, "--project=sirsi-nexus-live", "--account="+deployAccount, "--quiet")
	cmd := exec.CommandContext(ctx, "/opt/homebrew/bin/gcloud", args...)
	// Secret Manager HTTP responses contain the encoded secret. Override both
	// inherited settings and persistent gcloud property defaults in this child
	// only; discarding stderr alone cannot prevent SDK file/HTTP logging.
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CLOUDSDK_CORE_LOG_HTTP=") || strings.HasPrefix(entry, "CLOUDSDK_CORE_DISABLE_FILE_LOGGING=") {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "CLOUDSDK_CORE_LOG_HTTP=false", "CLOUDSDK_CORE_DISABLE_FILE_LOGGING=true")
	return cmd
}
