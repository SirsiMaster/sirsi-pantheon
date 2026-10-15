package main

// The installed Apollo path deliberately has no HTTP client, bearer token,
// endpoint, model-path, or worker-path flag.  Its signed application profile
// fixes the native fleet; the current macOS account supplies the principal;
// Ma'at supplies live resource fences; and Apollo owns every native process.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-inference/apollo/fleetservice"
	"github.com/SirsiMaster/sirsi-pantheon/internal/apollo"
)

const apolloNativeProfileSchema = "sirsi.apollo.native-profile.v2"

// The native stage currently proves only a live prefix through token 1024.
// Keep the installed client inside that exact boundary until Apollo qualifies
// per-layer masking beyond it; neither a profile nor a user request may widen
// it by silently taking a sliding-window path.
const apolloNativeQualifiedContext = 1024

type apolloNativeModel struct {
	ID               string `json:"id"`
	CheckpointSHA256 string `json:"checkpoint_sha256"`
	TokenizerSHA256  string `json:"tokenizer_sha256"`
	TemplateSHA256   string `json:"template_sha256"`
	Precision        string `json:"precision"`
	TokenizerPath    string `json:"tokenizer_path"`
	Capacity         int    `json:"context_capacity"`
}

type apolloNativeProfile struct {
	Schema                     string             `json:"schema"`
	Model                      apolloNativeModel  `json:"model"`
	Fleet                      fleetservice.Fleet `json:"fleet"`
	Plan                       fleetservice.Plan  `json:"plan"`
	Resources                  map[string]string  `json:"maat_resources"`
	RawRankBinarySHA256        map[string]string  `json:"raw_rank_binary_sha256"`
	CoordinatorTokenizerSHA256 string             `json:"coordinator_tokenizer_sha256"`
}

// The default is inside the signed application bundle. Tests replace this
// function with a temporary fixture; users cannot redirect native execution to
// an arbitrary profile.
var installedApolloNativeProfilePath = func() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate installed sirsi executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("canonicalize installed sirsi executable: %w", err)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "Resources", "Apollo", "native-profile.json")), nil
}

// apolloNativeMetricsRoot is deliberately outside the signed profile: it is
// runtime evidence owned by the current macOS account, never input that can
// redirect a run. Tests replace it with a temporary directory.
var apolloNativeMetricsRoot = func() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate Apollo metrics home: %w", err)
	}
	return filepath.Join(home, ".sirsi", "apollo", "runs"), nil
}

const apolloNativeMetricsSchema = "sirsi.apollo.run-metrics.v1"

// apolloNativeRunMetrics intentionally contains measurements and bound
// execution identity only. Prompt text, decoded token text, session content,
// and user identity never enter the durable telemetry surface.
type apolloNativeRunMetrics struct {
	Schema    string                  `json:"schema"`
	RequestID string                  `json:"request_id"`
	ModelID   string                  `json:"model_id"`
	Mode      string                  `json:"mode"`
	Precision string                  `json:"precision"`
	HostRoute []string                `json:"host_route"`
	Metrics   fleetservice.RunMetrics `json:"metrics"`
}

func loadInstalledApolloNativeProfile() (apolloNativeProfile, error) {
	path, err := installedApolloNativeProfilePath()
	if err != nil {
		return apolloNativeProfile{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return apolloNativeProfile{}, fmt.Errorf("installed Apollo native profile unavailable at %s: %w", path, err)
	}
	var profile apolloNativeProfile
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return apolloNativeProfile{}, fmt.Errorf("decode installed Apollo native profile: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return apolloNativeProfile{}, fmt.Errorf("installed Apollo native profile contains trailing data")
	}
	if err := profile.validate(); err != nil {
		return apolloNativeProfile{}, err
	}
	return profile, nil
}

func (p apolloNativeProfile) validate() error {
	if p.Schema != apolloNativeProfileSchema || p.Model.ID == "" || p.Model.Precision == "" || p.Model.Capacity < 1 || p.Model.Capacity > apolloNativeQualifiedContext || !filepath.IsAbs(p.Model.TokenizerPath) || len(p.Plan.Ranks) < 1 || len(p.Resources) != len(p.Plan.Ranks) {
		return fmt.Errorf("installed Apollo native profile is incomplete")
	}
	for _, digest := range []string{p.Model.CheckpointSHA256, p.Model.TokenizerSHA256, p.Model.TemplateSHA256, p.Plan.Model, p.Plan.Tokenizer} {
		if !apolloNativeDigest(digest) {
			return fmt.Errorf("installed Apollo native profile has an invalid identity digest")
		}
	}
	if p.Plan.Model != p.Model.CheckpointSHA256 || p.Plan.Tokenizer != p.Model.TokenizerSHA256 || p.Plan.Precision != p.Model.Precision || p.Plan.Context != p.Model.Capacity || p.Plan.Epoch == 0 {
		return fmt.Errorf("installed Apollo native profile model and plan identities differ")
	}
	hosts := make(map[string]fleetservice.Host, len(p.Fleet.Hosts))
	localRanks := 0
	for _, host := range p.Fleet.Hosts {
		if host.ID == "" || hosts[host.ID].ID != "" {
			return fmt.Errorf("installed Apollo native profile contains an unnamed or duplicate fleet host")
		}
		hosts[host.ID] = host
		if host.Local {
			localRanks++
		}
	}
	if len(hosts) != len(p.Plan.Ranks) {
		return fmt.Errorf("installed Apollo native profile fleet and rank counts differ")
	}
	for i, rank := range p.Plan.Ranks {
		if rank.Index != i || hosts[rank.Host].ID == "" || p.Resources[rank.Host] == "" || rank.Budget.Admitted == 0 || !apolloNativeDigest(rank.Engine) || !apolloNativeDigest(rank.Runtime) {
			return fmt.Errorf("installed Apollo native profile rank %d is not bound to a fleet host and Ma'at resource", i)
		}
	}
	if len(p.Plan.Ranks) == 1 {
		// A single local rank is the same Apollo backend and Ma'at admission
		// path used by the fleet. It intentionally has no coordinator or
		// transport edge: adding either would turn a local placement into an
		// unbound alternate route.
		rank := p.Plan.Ranks[0]
		if localRanks != 1 || !hosts[rank.Host].Local || p.Fleet.Coordinator != nil || p.CoordinatorTokenizerSHA256 != "" || len(p.Plan.Links) != 0 || len(p.RawRankBinarySHA256) != 0 {
			return fmt.Errorf("installed local Apollo profile must bind one local rank without coordinator or transport")
		}
		return nil
	}
	coordinator := p.Fleet.Coordinator
	if coordinator == nil || coordinator.HostID == "" || coordinator.Target == "" || !filepath.IsAbs(coordinator.Binary) || !filepath.IsAbs(coordinator.TokenizerPath) || !apolloNativeDigest(coordinator.BinarySHA256) || !apolloNativeDigest(p.CoordinatorTokenizerSHA256) || p.CoordinatorTokenizerSHA256 != p.Model.TokenizerSHA256 || localRanks != 1 || !hosts[coordinator.HostID].Local {
		return fmt.Errorf("installed Apollo fleet profile must bind one exact local coordinator")
	}
	if len(p.Plan.Links) != len(p.Plan.Ranks)-1 {
		return fmt.Errorf("installed Apollo native profile link topology is incomplete")
	}
	raw := false
	for i, link := range p.Plan.Links {
		if link.From != i || link.To != i+1 || link.Transport != "tcp" || !apolloNativeDigest(link.Profile) || (link.Codec != "mercury-cpu" && link.Codec != "raw") {
			return fmt.Errorf("installed Apollo native profile link %d is not qualified for installed execution", i)
		}
		raw = raw || link.Codec == "raw"
	}
	if raw {
		// Raw activation rows are enabled only when the signed installed profile
		// binds every participating rank to its exact raw-capable binary.  A
		// user flag, host probe, or older staged binary can never enable raw.
		if len(p.RawRankBinarySHA256) != len(p.Plan.Ranks) {
			return fmt.Errorf("installed Apollo raw transport lacks complete rank-binary identities")
		}
		for _, rank := range p.Plan.Ranks {
			artifact := p.RawRankBinarySHA256[rank.Host]
			if !apolloNativeDigest(artifact) || artifact != rank.Engine {
				return fmt.Errorf("installed Apollo raw transport rank %s lacks its bound binary artifact", rank.Host)
			}
		}
	} else if len(p.RawRankBinarySHA256) != 0 {
		return fmt.Errorf("installed Apollo profile declares raw rank artifacts without a raw link")
	}
	return nil
}

func apolloNativeDigest(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size && hex.EncodeToString(b) == value
}

type fleetMaatAuthority struct {
	maat      *apollo.MaatAuthority
	profile   apolloNativeProfile
	principal fleetservice.Principal
	admitted  map[string]apollo.NativeAdmission
	jobs      map[string]string
}

func newFleetMaatAuthority(profile apolloNativeProfile, principal fleetservice.Principal) (*fleetMaatAuthority, error) {
	ledger, err := maatLedger()
	if err != nil {
		return nil, err
	}
	maatAuthority, err := apollo.NewMaatAuthority(ledger)
	if err != nil {
		return nil, err
	}
	return &fleetMaatAuthority{maat: maatAuthority, profile: profile, principal: principal, admitted: make(map[string]apollo.NativeAdmission), jobs: make(map[string]string)}, nil
}

func (a *fleetMaatAuthority) Admit(ctx context.Context, principal fleetservice.Principal, prepared fleetservice.Prepared) (fleetservice.Admission, error) {
	if principal != a.principal || prepared.Plan.Model != a.profile.Model.CheckpointSHA256 || prepared.Plan.Tokenizer != a.profile.Model.TokenizerSHA256 || prepared.Plan.Precision != a.profile.Model.Precision || len(prepared.Plan.Ranks) != len(a.profile.Plan.Ranks) {
		return fleetservice.Admission{}, fmt.Errorf("Apollo native admission does not match the installed profile")
	}
	ranks := make([]apollo.NativeRankRequest, 0, len(prepared.Plan.Ranks))
	for i, rank := range prepared.Plan.Ranks {
		if rank.Index != i || rank.Host != a.profile.Plan.Ranks[i].Host || rank.Budget.Admitted != a.profile.Plan.Ranks[i].Budget.Admitted {
			return fleetservice.Admission{}, fmt.Errorf("Apollo native plan rank %d differs from the installed profile", i)
		}
		ranks = append(ranks, apollo.NativeRankRequest{Rank: i, HostID: rank.Host, Resource: a.profile.Resources[rank.Host], MemoryBytes: int64(rank.Budget.Admitted)})
	}
	admission, err := a.maat.Admit(ctx, apollo.NativeAdmissionRequest{
		ExecutionID: prepared.PreparationID, PlanSHA256: prepared.PlanSHA256, Epoch: prepared.Plan.Epoch,
		Holder: nativeHolder(principal), ExpiresAt: prepared.ExpiresAt, Ranks: ranks,
	})
	if err != nil {
		return fleetservice.Admission{}, err
	}
	a.admitted[admission.ExecutionID] = admission
	a.jobs[admission.ExecutionID] = prepared.PreparationID
	return fleetAdmission(admission, prepared.PreparationID), nil
}

func (a *fleetMaatAuthority) Verify(ctx context.Context, admission fleetservice.Admission) error {
	owned, ok := a.admitted[admission.ExecutionID]
	if !ok || a.jobs[admission.ExecutionID] != admission.JobID || !matchesFleetAdmission(owned, admission) {
		return fmt.Errorf("Apollo native admission is unknown or altered")
	}
	return a.maat.Verify(ctx, owned)
}

func (a *fleetMaatAuthority) Release(ctx context.Context, admission fleetservice.Admission) error {
	owned, ok := a.admitted[admission.ExecutionID]
	if !ok || a.jobs[admission.ExecutionID] != admission.JobID || !matchesFleetAdmission(owned, admission) {
		return fmt.Errorf("Apollo native admission is unknown or altered")
	}
	delete(a.admitted, admission.ExecutionID)
	delete(a.jobs, admission.ExecutionID)
	return a.maat.Release(ctx, owned)
}

func fleetAdmission(in apollo.NativeAdmission, jobID string) fleetservice.Admission {
	out := fleetservice.Admission{JobID: jobID, ExecutionID: in.ExecutionID, Epoch: in.Epoch, Ranks: make([]fleetservice.RankFence, 0, len(in.Ranks))}
	for _, fence := range in.SortedFences() {
		out.Ranks = append(out.Ranks, fleetservice.RankFence{Rank: fence.Rank, HostID: fence.HostID, Fence: fence.Fence, Generation: fence.Generation, ExpiresAt: fence.ExpiresAt, MemoryAdmissionBytes: fence.MemoryAdmissionBytes})
	}
	return out
}

func matchesFleetAdmission(in apollo.NativeAdmission, got fleetservice.Admission) bool {
	want := fleetAdmission(in, got.JobID)
	if want.ExecutionID != got.ExecutionID || want.Epoch != got.Epoch || len(want.Ranks) != len(got.Ranks) {
		return false
	}
	for i := range want.Ranks {
		if want.Ranks[i] != got.Ranks[i] {
			return false
		}
	}
	return true
}

func nativeHolder(principal fleetservice.Principal) string {
	return principal.Tenant + ":" + principal.Subject
}

func currentNativePrincipal() (fleetservice.Principal, error) {
	current, err := user.Current()
	if err != nil || strings.TrimSpace(current.Uid) == "" || strings.TrimSpace(current.Username) == "" {
		return fleetservice.Principal{}, fmt.Errorf("Apollo native execution requires the current macOS user identity")
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return fleetservice.Principal{}, fmt.Errorf("Apollo native execution requires the local host identity")
	}
	return fleetservice.Principal{Tenant: strings.TrimSpace(host), Subject: strings.TrimSpace(current.Uid) + ":" + strings.TrimSpace(current.Username)}, nil
}

func runInstalledApolloNativeChat(cmdContext context.Context, messages []apollo.ChatMessage, requestID, sessionID string, output io.Writer) (fleetservice.RunMetrics, error) {
	if output == nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("Apollo native output writer is required")
	}
	profile, err := loadInstalledApolloNativeProfile()
	if err != nil {
		return fleetservice.RunMetrics{}, err
	}
	if apolloModelID != profile.Model.ID || apolloMode != "plain" || apolloPrecision != profile.Model.Precision || apolloMaxOutput == 0 || apolloMaxOutput > 4096 {
		return fleetservice.RunMetrics{}, fmt.Errorf("Apollo request does not match the installed native profile")
	}
	principal, err := currentNativePrincipal()
	if err != nil {
		return fleetservice.RunMetrics{}, err
	}
	tokenizer, err := fleetservice.LoadTokenizer(profile.Model.TokenizerPath)
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("load installed Apollo tokenizer: %w", err)
	}
	nativeMessages := make([]fleetservice.ChatMessage, len(messages))
	for i, message := range messages {
		nativeMessages[i] = fleetservice.ChatMessage{Role: message.Role, Content: message.Content}
	}
	prompt, err := tokenizer.ChatPrompt(nativeMessages)
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("prepare installed Apollo chat: %w", err)
	}
	ids := tokenizer.Encode(prompt, true)
	if len(ids) == 0 || len(ids)+int(apolloMaxOutput)-1 > profile.Model.Capacity {
		return fleetservice.RunMetrics{}, fmt.Errorf("Apollo prompt exceeds the installed profile context capacity")
	}
	plan := profile.Plan
	now := time.Now().UTC()
	plan.ID, plan.Issued, plan.Expires = "native-"+requestID, now.Unix(), now.Add(110*time.Second).Unix()
	plan.Signature = nil // the installed application signature binds profile bytes; live Ma'at fences replace static construction leases.
	planBytes, err := plan.Bytes()
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("encode installed Apollo plan: %w", err)
	}
	planHash := sha256.Sum256(planBytes)
	prepared := fleetservice.Prepared{
		Contract: fleetservice.Contract, PreparationID: requestID, RequestID: requestID, SessionID: sessionID,
		ExpiresAt: now.Add(110 * time.Second), ModelID: profile.Model.ID, CheckpointSHA256: profile.Model.CheckpointSHA256,
		TokenizerSHA256: profile.Model.TokenizerSHA256, TemplateSHA256: profile.Model.TemplateSHA256,
		PlanSHA256: hex.EncodeToString(planHash[:]), Mode: apolloMode, Precision: profile.Model.Precision,
		StopTokens: tokenizer.StopTokenIDs(), MaxOutputTokens: apolloMaxOutput, RankCount: len(plan.Ranks), Plan: plan,
	}
	for _, id := range ids {
		prepared.InputTokens = append(prepared.InputTokens, uint32(id))
	}
	authority, err := newFleetMaatAuthority(profile, principal)
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("open Ma'at Apollo authority: %w", err)
	}
	backend, err := fleetservice.NewNativeBackend(authority, profile.Fleet, tokenizer)
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("open installed Apollo native backend: %w", err)
	}
	admission, err := backend.Admit(cmdContext, principal, prepared)
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("Ma'at refused Apollo native admission: %w", err)
	}
	for i := range prepared.Plan.Ranks {
		prepared.Plan.Ranks[i].Lease = admission.Ranks[i].Fence
	}
	planBytes, err = prepared.Plan.Bytes()
	if err != nil {
		return fleetservice.RunMetrics{}, fmt.Errorf("encode live Apollo fenced plan: %w", err)
	}
	finalPlanHash := sha256.Sum256(planBytes)
	prepared.PlanSHA256 = hex.EncodeToString(finalPlanHash[:])
	runContext, cancel := context.WithDeadline(cmdContext, prepared.ExpiresAt)
	defer cancel()
	var runErr error
	if err := backend.Verify(runContext, admission); err != nil {
		runErr = fmt.Errorf("Ma'at Apollo fence verification failed: %w", err)
	} else {
		runErr = backend.Run(runContext, admission, prepared, func(_ uint32, _ uint32, text string) error {
			_, err := fmt.Fprint(output, text)
			return err
		})
	}
	metrics, metricsOK := fleetservice.ReadRunMetrics(backend, admission.ExecutionID)
	if !metricsOK {
		runErr = errors.Join(runErr, fmt.Errorf("Apollo native run metrics were not recorded before resource release"))
	}
	cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cleanupCancel()
	cleanupErr := backend.Release(cleanupContext, admission)
	if runErr != nil || cleanupErr != nil {
		return metrics, errors.Join(runErr, cleanupErr)
	}
	if err := persistApolloNativeRunMetrics(requestID, profile, metrics); err != nil {
		return metrics, err
	}
	return metrics, nil
}

func persistApolloNativeRunMetrics(requestID string, profile apolloNativeProfile, metrics fleetservice.RunMetrics) error {
	if !apolloSafeID(requestID) {
		return fmt.Errorf("Apollo run metrics require a safe request id")
	}
	root, err := apolloNativeMetricsRoot()
	if err != nil {
		return err
	}
	directory := filepath.Join(root, requestID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create Apollo run metrics directory: %w", err)
	}
	route := make([]string, len(profile.Plan.Ranks))
	for i, rank := range profile.Plan.Ranks {
		route[i] = rank.Host
	}
	record := apolloNativeRunMetrics{
		Schema: apolloNativeMetricsSchema, RequestID: requestID, ModelID: profile.Model.ID,
		Mode: apolloMode, Precision: profile.Model.Precision, HostRoute: route, Metrics: metrics,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode Apollo run metrics: %w", err)
	}
	encoded = append(encoded, '\n')
	temporary, err := os.CreateTemp(directory, ".metrics-*")
	if err != nil {
		return fmt.Errorf("create Apollo metrics evidence: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("protect Apollo metrics evidence: %w", err)
	}
	if written, err := temporary.Write(encoded); err != nil || written != len(encoded) {
		cleanup()
		if err != nil {
			return fmt.Errorf("write Apollo metrics evidence: %w", err)
		}
		return fmt.Errorf("write Apollo metrics evidence: short write")
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync Apollo metrics evidence: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("close Apollo metrics evidence: %w", err)
	}
	final := filepath.Join(directory, "metrics.json")
	if err := os.Link(temporaryName, final); err != nil {
		_ = os.Remove(temporaryName)
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("Apollo run metrics already exist for request %q", requestID)
		}
		return fmt.Errorf("publish Apollo metrics evidence: %w", err)
	}
	if err := os.Remove(temporaryName); err != nil {
		return fmt.Errorf("remove Apollo metrics staging evidence: %w", err)
	}
	directoryFD, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open Apollo metrics directory for sync: %w", err)
	}
	defer directoryFD.Close()
	if err := directoryFD.Sync(); err != nil {
		return fmt.Errorf("sync Apollo metrics directory: %w", err)
	}
	return nil
}
