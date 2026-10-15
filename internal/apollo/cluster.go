package apollo

// The cluster client is deliberately a client of the Apollo service contract,
// not an inference implementation.  It cannot turn a Ra lease or a planning
// result into permission to allocate a model: the service must return a fresh
// all-rank admission with an execution fence before the installed client calls
// a job launched.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ClusterContractV1 = "apollo.cluster-inference.v1"
	maxClusterBody    = 4 << 20
	maxEventLine      = 2 << 20
)

var (
	ErrClusterAdmission = errors.New("apollo cluster admission rejected")
	ErrClusterProtocol  = errors.New("apollo cluster protocol rejected")
)

// ClusterClient reaches an authenticated Apollo service. Authentication is
// intentionally delegated to an installed HTTPS transport (for example
// mTLS/session transport); no bearer token or identity is accepted in a command
// flag or in a request body. A nil client is never an authentication fallback.
type ClusterClient struct {
	base *url.URL
	http *http.Client
}

// NewClusterClient accepts only an HTTPS service URL. The endpoint is a
// service location, never a shell/SSH target, and redirects are refused so a
// caller cannot silently send private prompt tokens elsewhere.
func NewClusterClient(endpoint string, client *http.Client) (*ClusterClient, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: enrolled Apollo authentication transport is required", ErrClusterAdmission)
	}
	return newClusterClient(endpoint, client, false)
}

func newClusterClient(endpoint string, client *http.Client, allowHTTP bool) (*ClusterClient, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: endpoint must be an absolute HTTPS service URL", ErrClusterProtocol)
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return nil, fmt.Errorf("%w: endpoint must use HTTPS", ErrClusterProtocol)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if client == nil {
		return nil, fmt.Errorf("%w: test transport is required", ErrClusterAdmission)
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &ClusterClient{base: u, http: &copyClient}, nil
}

// Capability describes a server-declared, actually enabled cluster capability.
// Planned-only services must leave LaunchAvailable false and are refused by
// CreateJob even when their endpoint shape otherwise looks valid.
type Capability struct {
	Contract        string   `json:"contract"`
	LaunchAvailable bool     `json:"launch_available"`
	StreamAvailable bool     `json:"stream_available"`
	CancelAvailable bool     `json:"cancel_available"`
	Modes           []string `json:"modes"`
}

func (c Capability) validateFor(mode string) error {
	if c.Contract != ClusterContractV1 || !c.LaunchAvailable || !c.StreamAvailable || !c.CancelAvailable {
		return fmt.Errorf("%w: service does not declare an active %s launch/stream/cancel contract", ErrClusterAdmission, ClusterContractV1)
	}
	for _, supported := range c.Modes {
		if supported == mode {
			return nil
		}
	}
	return fmt.Errorf("%w: service does not admit mode %q", ErrClusterAdmission, mode)
}

// ChatMessage is user text sent only to authenticated, service-owned
// preparation. The client never tokenizes it locally or decides a template.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PrepareRequest is the normal user-facing request. Tenant and caller identity
// are derived solely by the enrolled transport; the service contract does bind
// opaque request and session identifiers so preparation and later job creation
// can be matched without accepting a caller-supplied principal.
type PrepareRequest struct {
	Contract        string        `json:"contract"`
	RequestID       string        `json:"request_id"`
	SessionID       string        `json:"session_id"`
	ModelID         string        `json:"model_id"`
	Machines        []string      `json:"machines"`
	Precision       string        `json:"precision"`
	Mode            string        `json:"mode"`
	Messages        []ChatMessage `json:"messages"`
	MaxOutputTokens uint32        `json:"max_output_tokens"`
}

func (r PrepareRequest) validate() error {
	if r.Contract != ClusterContractV1 || strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.ModelID) == "" || strings.TrimSpace(r.Precision) == "" || len(r.Messages) == 0 || r.MaxOutputTokens == 0 {
		return fmt.Errorf("%w: request_id, session_id, model, precision, messages, and max_output_tokens are required", ErrClusterProtocol)
	}
	if r.Mode != "plain" && r.Mode != "flash" {
		return fmt.Errorf("%w: mode must be plain or flash; no fallback is permitted", ErrClusterProtocol)
	}
	seenMachines := map[string]bool{}
	for _, machine := range r.Machines {
		machine = strings.TrimSpace(machine)
		if machine == "" || seenMachines[machine] {
			return fmt.Errorf("%w: machines must be unique nonempty service identities", ErrClusterProtocol)
		}
		seenMachines[machine] = true
	}
	if len(seenMachines) == 0 {
		return fmt.Errorf("%w: choose at least one admitted machine or auto", ErrClusterProtocol)
	}
	for _, message := range r.Messages {
		if (message.Role != "system" && message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("%w: messages must use supported roles with nonempty content", ErrClusterProtocol)
		}
	}
	return nil
}

// RankTopology is the service-owned expected rank placement supplied by
// preparation. Job admission must return exactly this contiguous topology;
// accepting merely some arbitrary fenced rank would not prove all-rank launch.
type RankTopology struct {
	Rank   int    `json:"rank"`
	HostID string `json:"host_id"`
}

// Preparation binds trusted service tokenization/template/checkpoint/plan
// identities to one proposed rank topology. Client text cannot manufacture any
// of these values.
type Preparation struct {
	Contract         string   `json:"contract"`
	PreparationID    string   `json:"preparation_id"`
	RequestID        string   `json:"request_id"`
	SessionID        string   `json:"session_id"`
	ExpiresAt        string   `json:"expires_at"`
	ModelID          string   `json:"model_id"`
	PlanSHA256       string   `json:"plan_sha256"`
	CheckpointSHA256 string   `json:"checkpoint_sha256"`
	TokenizerSHA256  string   `json:"tokenizer_sha256"`
	TemplateSHA256   string   `json:"template_sha256"`
	Mode             string   `json:"mode"`
	Precision        string   `json:"precision"`
	InputTokens      []uint32 `json:"input_tokens"`
	StopTokens       []uint32 `json:"stop_token_ids"`
	MaxOutputTokens  uint32   `json:"max_output_tokens"`
	RankCount        int      `json:"rank_count"`
}

func (p Preparation) validate() error {
	if p.Contract != ClusterContractV1 || strings.TrimSpace(p.PreparationID) == "" || strings.TrimSpace(p.RequestID) == "" || strings.TrimSpace(p.SessionID) == "" || strings.TrimSpace(p.ModelID) == "" || (p.Mode != "plain" && p.Mode != "flash") || strings.TrimSpace(p.Precision) == "" || len(p.InputTokens) == 0 || p.MaxOutputTokens == 0 || p.RankCount < 2 {
		return fmt.Errorf("%w: incomplete service preparation", ErrClusterProtocol)
	}
	for _, hash := range []string{p.PlanSHA256, p.CheckpointSHA256, p.TokenizerSHA256, p.TemplateSHA256} {
		if !lowerSHA256(hash) {
			return fmt.Errorf("%w: preparation has invalid identity digest", ErrClusterProtocol)
		}
	}
	expires, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil || !expires.After(time.Now().UTC()) {
		return fmt.Errorf("%w: preparation is expired or malformed", ErrClusterProtocol)
	}
	return nil
}

// JobRequest is accepted only after service-owned preparation. It carries no
// model/tokenizer/template/session values that a caller could assert.
type JobRequest struct {
	Contract         string   `json:"contract"`
	PreparationID    string   `json:"preparation_id"`
	RequestID        string   `json:"request_id"`
	IdempotencyKey   string   `json:"idempotency_key"`
	SessionID        string   `json:"session_id"`
	PlanSHA256       string   `json:"plan_sha256"`
	ModelID          string   `json:"model_id"`
	CheckpointSHA256 string   `json:"checkpoint_sha256"`
	TokenizerSHA256  string   `json:"tokenizer_sha256"`
	TemplateSHA256   string   `json:"template_sha256"`
	Mode             string   `json:"mode"`
	Precision        string   `json:"precision"`
	InputTokens      []uint32 `json:"input_tokens"`
	MaxOutputTokens  uint32   `json:"max_output_tokens"`
}

func (r JobRequest) validate() error {
	if r.Contract != ClusterContractV1 || strings.TrimSpace(r.PreparationID) == "" || strings.TrimSpace(r.RequestID) == "" || strings.TrimSpace(r.IdempotencyKey) == "" || strings.TrimSpace(r.SessionID) == "" || strings.TrimSpace(r.PlanSHA256) == "" || strings.TrimSpace(r.ModelID) == "" || strings.TrimSpace(r.CheckpointSHA256) == "" || strings.TrimSpace(r.TokenizerSHA256) == "" || strings.TrimSpace(r.TemplateSHA256) == "" || strings.TrimSpace(r.Precision) == "" || len(r.InputTokens) == 0 || r.MaxOutputTokens == 0 {
		return fmt.Errorf("%w: incomplete frozen preparation request", ErrClusterProtocol)
	}
	if r.Mode != "plain" && r.Mode != "flash" {
		return fmt.Errorf("%w: mode must be plain or flash", ErrClusterProtocol)
	}
	return nil
}

// RankFence is an actual rank resource admission, not a task lease. The
// installed client requires every field so a service cannot claim all-rank
// admission while leaving a rank or its memory/fence identity unspecified.
type RankFence struct {
	Rank                 int    `json:"rank"`
	HostID               string `json:"host_id"`
	Fence                string `json:"fence"`
	Generation           string `json:"generation"`
	ExpiresAt            string `json:"expires_at"`
	MemoryAdmissionBytes int64  `json:"memory_admission_bytes"`
}

// Job is the contract response shared by create/status/cancel and event
// envelopes. A status may omit RankFences only after a terminal released state;
// a created job always requires them.
type Job struct {
	Contract    string      `json:"contract"`
	JobID       string      `json:"job_id"`
	ExecutionID string      `json:"execution_id"`
	RequestID   string      `json:"request_id"`
	PlanSHA256  string      `json:"plan_sha256"`
	Epoch       uint64      `json:"epoch"`
	State       string      `json:"state"`
	RankFences  []RankFence `json:"rank_fences,omitempty"`
}

func (j Job) validate(expectedRequest, expectedPlan string, expectedTopology []RankTopology, requireFences bool) error {
	if j.Contract != ClusterContractV1 || strings.TrimSpace(j.JobID) == "" || strings.TrimSpace(j.ExecutionID) == "" || j.Epoch == 0 {
		return fmt.Errorf("%w: response has no authoritative job execution identity", ErrClusterProtocol)
	}
	if expectedRequest != "" && j.RequestID != expectedRequest {
		return fmt.Errorf("%w: response request identity differs", ErrClusterProtocol)
	}
	if expectedPlan != "" && j.PlanSHA256 != expectedPlan {
		return fmt.Errorf("%w: response plan identity differs", ErrClusterProtocol)
	}
	if requireFences && len(j.RankFences) == 0 {
		return fmt.Errorf("%w: service returned no all-rank resource fences", ErrClusterAdmission)
	}
	if len(j.RankFences) > 0 {
		if err := validateTopology(topologyFromFences(j.RankFences)); err != nil {
			return fmt.Errorf("%w: %v", ErrClusterAdmission, err)
		}
		if len(expectedTopology) > 0 && !sameTopology(expectedTopology, topologyFromFences(j.RankFences)) {
			return fmt.Errorf("%w: job rank topology differs from service preparation", ErrClusterAdmission)
		}
	}
	seenRank := map[int]bool{}
	seenHost := map[string]bool{}
	now := time.Now().UTC()
	for _, rank := range j.RankFences {
		if rank.Rank < 0 || seenRank[rank.Rank] || seenHost[rank.HostID] || strings.TrimSpace(rank.HostID) == "" || strings.TrimSpace(rank.Fence) == "" || strings.TrimSpace(rank.Generation) == "" || rank.MemoryAdmissionBytes <= 0 {
			return fmt.Errorf("%w: incomplete or duplicate rank admission", ErrClusterAdmission)
		}
		seenRank[rank.Rank] = true
		seenHost[rank.HostID] = true
		expires, err := time.Parse(time.RFC3339, rank.ExpiresAt)
		if err != nil || !expires.After(now) {
			return fmt.Errorf("%w: rank %d has no live execution fence", ErrClusterAdmission, rank.Rank)
		}
	}
	return nil
}

func lowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

func validateTopology(topology []RankTopology) error {
	if len(topology) == 0 {
		return errors.New("no rank topology")
	}
	seenHosts := map[string]bool{}
	for index, rank := range topology {
		if rank.Rank != index || strings.TrimSpace(rank.HostID) == "" || seenHosts[rank.HostID] {
			return errors.New("rank topology must be contiguous with unique hosts")
		}
		seenHosts[rank.HostID] = true
	}
	return nil
}

func topologyFromFences(fences []RankFence) []RankTopology {
	topology := make([]RankTopology, 0, len(fences))
	for _, fence := range fences {
		topology = append(topology, RankTopology{Rank: fence.Rank, HostID: fence.HostID})
	}
	return topology
}

func sameTopology(expected, actual []RankTopology) bool {
	if len(expected) != len(actual) {
		return false
	}
	for index := range expected {
		if expected[index] != actual[index] {
			return false
		}
	}
	return true
}

func (c *ClusterClient) Capabilities(ctx context.Context) (Capability, error) {
	var out Capability
	if err := c.doJSON(ctx, http.MethodGet, "/v1/apollo/capabilities", nil, &out); err != nil {
		return Capability{}, err
	}
	if out.Contract != ClusterContractV1 {
		return Capability{}, fmt.Errorf("%w: service contract is not %q", ErrClusterProtocol, ClusterContractV1)
	}
	return out, nil
}

// Prepare asks the authenticated service to select/tokenize the requested
// conversation and return a trusted frozen preparation. It is the only normal
// client route that carries user text.
func (c *ClusterClient) Prepare(ctx context.Context, in PrepareRequest) (Preparation, error) {
	if err := in.validate(); err != nil {
		return Preparation{}, err
	}
	capabilities, err := c.Capabilities(ctx)
	if err != nil {
		return Preparation{}, err
	}
	if err := capabilities.validateFor(in.Mode); err != nil {
		return Preparation{}, err
	}
	var out Preparation
	if err := c.doJSON(ctx, http.MethodPost, "/v1/apollo/prepare", in, &out); err != nil {
		return Preparation{}, err
	}
	if err := out.validate(); err != nil {
		return Preparation{}, err
	}
	if out.RequestID != in.RequestID || out.SessionID != in.SessionID || out.ModelID != in.ModelID || out.Mode != in.Mode || out.Precision != in.Precision || out.MaxOutputTokens != in.MaxOutputTokens {
		return Preparation{}, fmt.Errorf("%w: service preparation changed requested mode or precision", ErrClusterProtocol)
	}
	return out, nil
}

// CreateJob submits one service-owned preparation. It refuses success unless
// the returned job has the exact prepared contiguous topology plus a complete,
// live all-rank memory admission and execution fence set.
func (c *ClusterClient) CreateJob(ctx context.Context, preparation Preparation, idempotencyKey string) (Job, error) {
	if err := preparation.validate(); err != nil {
		return Job{}, err
	}
	in := JobRequest{
		Contract: ClusterContractV1, PreparationID: preparation.PreparationID, RequestID: preparation.RequestID,
		IdempotencyKey: idempotencyKey, SessionID: preparation.SessionID, PlanSHA256: preparation.PlanSHA256,
		ModelID: preparation.ModelID, CheckpointSHA256: preparation.CheckpointSHA256,
		TokenizerSHA256: preparation.TokenizerSHA256, TemplateSHA256: preparation.TemplateSHA256,
		Mode: preparation.Mode, Precision: preparation.Precision, InputTokens: preparation.InputTokens,
		MaxOutputTokens: preparation.MaxOutputTokens,
	}
	if err := in.validate(); err != nil {
		return Job{}, err
	}
	capabilities, err := c.Capabilities(ctx)
	if err != nil {
		return Job{}, err
	}
	if err := capabilities.validateFor(preparation.Mode); err != nil {
		return Job{}, err
	}
	var out Job
	if err := c.doJSON(ctx, http.MethodPost, "/v1/apollo/jobs", in, &out); err != nil {
		return Job{}, err
	}
	if len(out.RankFences) != preparation.RankCount {
		return Job{}, fmt.Errorf("%w: job did not admit every prepared rank", ErrClusterAdmission)
	}
	if err := out.validate(preparation.RequestID, preparation.PlanSHA256, nil, true); err != nil {
		return Job{}, err
	}
	return out, nil
}

func (c *ClusterClient) Status(ctx context.Context, jobID string) (Job, error) {
	if strings.TrimSpace(jobID) == "" {
		return Job{}, fmt.Errorf("%w: job_id is required", ErrClusterProtocol)
	}
	var out Job
	if err := c.doJSON(ctx, http.MethodGet, "/v1/apollo/jobs/"+url.PathEscape(jobID), nil, &out); err != nil {
		return Job{}, err
	}
	if out.JobID != jobID {
		return Job{}, fmt.Errorf("%w: status response job identity differs", ErrClusterProtocol)
	}
	if err := out.validate("", "", nil, out.State != "released" && out.State != "orphan_unknown"); err != nil {
		return Job{}, err
	}
	return out, nil
}

func (c *ClusterClient) Cancel(ctx context.Context, jobID string) (Job, error) {
	if strings.TrimSpace(jobID) == "" {
		return Job{}, fmt.Errorf("%w: job_id is required", ErrClusterProtocol)
	}
	var out Job
	if err := c.doJSON(ctx, http.MethodPost, "/v1/apollo/jobs/"+url.PathEscape(jobID)+"/cancel", map[string]string{"contract": ClusterContractV1}, &out); err != nil {
		return Job{}, err
	}
	if out.JobID != jobID {
		return Job{}, fmt.Errorf("%w: cancel response job identity differs", ErrClusterProtocol)
	}
	if out.State != "cancel_requested" && out.State != "draining" && out.State != "cancelled" && out.State != "released" && out.State != "orphan_unknown" {
		return Job{}, fmt.Errorf("%w: cancel did not return a cancellation state", ErrClusterProtocol)
	}
	if err := out.validate("", "", nil, out.State != "released" && out.State != "orphan_unknown"); err != nil {
		return Job{}, err
	}
	return out, nil
}

// Event is a replay-safe server-sent event. The caller provides the last
// persisted sequence and this method rejects regressions or an event for a
// different job; it does not print or persist private tokens itself.
type Event struct {
	Contract    string  `json:"contract"`
	JobID       string  `json:"job_id"`
	ExecutionID string  `json:"execution_id"`
	RequestID   string  `json:"request_id"`
	PlanSHA256  string  `json:"plan_sha256"`
	Epoch       uint64  `json:"epoch"`
	Sequence    uint64  `json:"sequence"`
	Type        string  `json:"type"`
	Position    *uint32 `json:"position,omitempty"`
	TokenID     *uint32 `json:"token_id,omitempty"`
	Text        string  `json:"text,omitempty"`
	ErrorCode   string  `json:"error_code,omitempty"`
	// Raw retains the complete authorized service event (including token text or
	// terminal error details) for the CLI to relay without a second, lossy
	// schema. Its identity/sequence fields are still validated before delivery.
	Raw json.RawMessage `json:"-"`
}

func (e *Event) UnmarshalJSON(data []byte) error {
	type eventFields Event
	var fields eventFields
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*e = Event(fields)
	e.Raw = append(e.Raw[:0], data...)
	return nil
}

func (e Event) MarshalJSON() ([]byte, error) {
	if len(e.Raw) != 0 {
		return append([]byte(nil), e.Raw...), nil
	}
	type eventFields Event
	return json.Marshal(eventFields(e))
}

func (c *ClusterClient) Events(ctx context.Context, job Job, after uint64, emit func(Event) error) error {
	if strings.TrimSpace(job.JobID) == "" || strings.TrimSpace(job.ExecutionID) == "" || strings.TrimSpace(job.RequestID) == "" || strings.TrimSpace(job.PlanSHA256) == "" || job.Epoch == 0 || emit == nil {
		return fmt.Errorf("%w: authoritative job identity and event consumer are required", ErrClusterProtocol)
	}
	path := "/v1/apollo/jobs/" + url.PathEscape(job.JobID) + "/events?after=" + strconv.FormatUint(after, 10)
	req, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("apollo events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return fmt.Errorf("%w: events response is not server-sent events", ErrClusterProtocol)
	}
	// A stream has no fixed aggregate byte ceiling: it can be long-lived, while
	// Scanner keeps each individual event bounded before allocating it.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxEventLine)
	last := after
	terminal := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			return fmt.Errorf("%w: unsupported SSE field", ErrClusterProtocol)
		}
		var event Event
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return fmt.Errorf("%w: decode event: %v", ErrClusterProtocol, err)
		}
		if event.Contract != ClusterContractV1 || event.JobID != job.JobID || event.ExecutionID != job.ExecutionID || event.RequestID != job.RequestID || event.PlanSHA256 != job.PlanSHA256 || event.Epoch != job.Epoch || event.Sequence <= last || strings.TrimSpace(event.Type) == "" {
			return fmt.Errorf("%w: non-monotonic or foreign event", ErrClusterProtocol)
		}
		if terminal {
			return fmt.Errorf("%w: event followed a terminal event", ErrClusterProtocol)
		}
		last = event.Sequence
		if err := emit(event); err != nil {
			return err
		}
		terminal = event.Type == "released" || event.Type == "orphan_unknown"
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("apollo events: %w", err)
	}
	if !terminal {
		return fmt.Errorf("%w: event stream ended without release or orphan evidence", ErrClusterProtocol)
	}
	return nil
}

func (c *ClusterClient) doJSON(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("%w: encode request: %v", ErrClusterProtocol, err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("apollo service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxClusterBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrClusterProtocol, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("%w: response has trailing data", ErrClusterProtocol)
	}
	return nil
}

func (c *ClusterClient) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	u := *c.base
	parsed, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(parsed.Path, "/") {
		return nil, fmt.Errorf("%w: invalid service path", ErrClusterProtocol)
	}
	u.Path += parsed.Path
	u.RawQuery = parsed.RawQuery
	return http.NewRequestWithContext(ctx, method, u.String(), body)
}

func responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("%w: service returned %s: %s", ErrClusterAdmission, resp.Status, message)
}
