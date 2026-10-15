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
// intentionally delegated to the caller's installed HTTPS transport (for
// example mTLS/session transport); no bearer token or identity is accepted in
// a command flag or in a request body.
type ClusterClient struct {
	base *url.URL
	http *http.Client
}

// NewClusterClient accepts only an HTTPS service URL. The endpoint is a
// service location, never a shell/SSH target, and redirects are refused so a
// caller cannot silently send private prompt tokens elsewhere.
func NewClusterClient(endpoint string, client *http.Client) (*ClusterClient, error) {
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
		client = &http.Client{Timeout: 90 * time.Second}
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

// JobRequest is the frozen input accepted by POST /v1/apollo/jobs. It
// intentionally carries pre-tokenized input: this client never starts a local
// model or silently substitutes tokenizers/templates/checkpoints.
type JobRequest struct {
	Contract         string   `json:"contract"`
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
	if r.Contract != ClusterContractV1 {
		return fmt.Errorf("%w: expected contract %q", ErrClusterProtocol, ClusterContractV1)
	}
	for _, field := range []struct{ name, value string }{
		{"request_id", r.RequestID}, {"idempotency_key", r.IdempotencyKey}, {"session_id", r.SessionID},
		{"plan_sha256", r.PlanSHA256}, {"model_id", r.ModelID}, {"checkpoint_sha256", r.CheckpointSHA256},
		{"tokenizer_sha256", r.TokenizerSHA256}, {"template_sha256", r.TemplateSHA256}, {"precision", r.Precision},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%w: %s is required", ErrClusterProtocol, field.name)
		}
	}
	if len(r.InputTokens) == 0 || r.MaxOutputTokens == 0 {
		return fmt.Errorf("%w: input_tokens and max_output_tokens are required", ErrClusterProtocol)
	}
	if r.Mode != "plain" && r.Mode != "mtp" {
		return fmt.Errorf("%w: mode must be plain or mtp; no fallback is permitted", ErrClusterProtocol)
	}
	for _, hash := range []string{r.PlanSHA256, r.CheckpointSHA256, r.TokenizerSHA256, r.TemplateSHA256} {
		if len(hash) != 64 {
			return fmt.Errorf("%w: identity digest must be lowercase SHA-256", ErrClusterProtocol)
		}
		for _, ch := range hash {
			if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
				return fmt.Errorf("%w: identity digest must be lowercase SHA-256", ErrClusterProtocol)
			}
		}
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

func (j Job) validate(expectedRequest, expectedPlan string, requireFences bool) error {
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
	seenRank := map[int]bool{}
	now := time.Now().UTC()
	for _, rank := range j.RankFences {
		if rank.Rank < 0 || seenRank[rank.Rank] || strings.TrimSpace(rank.HostID) == "" || strings.TrimSpace(rank.Fence) == "" || strings.TrimSpace(rank.Generation) == "" || rank.MemoryAdmissionBytes <= 0 {
			return fmt.Errorf("%w: incomplete or duplicate rank admission", ErrClusterAdmission)
		}
		seenRank[rank.Rank] = true
		expires, err := time.Parse(time.RFC3339, rank.ExpiresAt)
		if err != nil || !expires.After(now) {
			return fmt.Errorf("%w: rank %d has no live execution fence", ErrClusterAdmission, rank.Rank)
		}
	}
	return nil
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

// CreateJob verifies the service's active capabilities first, then submits the
// frozen request. It refuses success unless the returned job has a complete,
// live all-rank memory admission and execution fence set.
func (c *ClusterClient) CreateJob(ctx context.Context, in JobRequest) (Job, error) {
	if err := in.validate(); err != nil {
		return Job{}, err
	}
	capabilities, err := c.Capabilities(ctx)
	if err != nil {
		return Job{}, err
	}
	if err := capabilities.validateFor(in.Mode); err != nil {
		return Job{}, err
	}
	var out Job
	if err := c.doJSON(ctx, http.MethodPost, "/v1/apollo/jobs", in, &out); err != nil {
		return Job{}, err
	}
	if err := out.validate(in.RequestID, in.PlanSHA256, true); err != nil {
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
	if err := out.validate("", "", out.State != "released" && out.State != "orphan_unknown"); err != nil {
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
	if err := out.validate("", "", out.State != "released" && out.State != "orphan_unknown"); err != nil {
		return Job{}, err
	}
	return out, nil
}

// Event is a replay-safe server-sent event. The caller provides the last
// persisted sequence and this method rejects regressions or an event for a
// different job; it does not print or persist private tokens itself.
type Event struct {
	Contract    string          `json:"contract"`
	JobID       string          `json:"job_id"`
	ExecutionID string          `json:"execution_id"`
	RequestID   string          `json:"request_id"`
	PlanSHA256  string          `json:"plan_sha256"`
	Epoch       uint64          `json:"epoch"`
	Sequence    uint64          `json:"sequence"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"-"`
}

func (c *ClusterClient) Events(ctx context.Context, jobID string, after uint64, emit func(Event) error) error {
	if strings.TrimSpace(jobID) == "" || emit == nil {
		return fmt.Errorf("%w: job_id and event consumer are required", ErrClusterProtocol)
	}
	path := "/v1/apollo/jobs/" + url.PathEscape(jobID) + "/events?after=" + strconv.FormatUint(after, 10)
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
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, maxClusterBody))
	scanner.Buffer(make([]byte, 0, 64<<10), maxEventLine)
	last := after
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
		if event.Contract != ClusterContractV1 || event.JobID != jobID || event.Sequence <= last || event.Epoch == 0 || strings.TrimSpace(event.Type) == "" {
			return fmt.Errorf("%w: non-monotonic or foreign event", ErrClusterProtocol)
		}
		last = event.Sequence
		if err := emit(event); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("apollo events: %w", err)
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
