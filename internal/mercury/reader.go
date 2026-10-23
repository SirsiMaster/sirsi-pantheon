// Package mercury reads Mercury's versioned monitor contract; it never controls runs.
package mercury

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Contract types and schema subset originate in Mercury e99b021c8f247afd27c8bcd741964218aae5b696.
// Consumer compatibility permits unknown optional fields and future enum words.
//
//go:embed dashboard-v1.schema.json
var schemaJSON []byte

const (
	MaxBytes               = 262144
	MaxResponseHeaderBytes = 8192
	DefaultEndpoint        = "http://127.0.0.1:8099/api/v1/dashboard"
	DashboardURLEnv        = "SIRSI_MERCURY_DASHBOARD_URL"
)

type Cable struct {
	N          int     `json:"n"`
	Cable      string  `json:"cable"`
	PayloadGBs float64 `json:"payload_gbs"`
	WireGBs    float64 `json:"wire_gbs"`
	Recoveries int     `json:"recoveries"`
}
type Dashboard struct {
	Schema        string     `json:"schema"`
	SchemaVersion string     `json:"schema_version"`
	Status        string     `json:"status"` // ok | partial | empty
	Generation    uint64     `json:"generation"`
	GeneratedAt   float64    `json:"generated_at"`
	Source        DashSource `json:"source"`
	Poll          DashPoll   `json:"poll"`
	Units         DashUnits  `json:"units"`
	Enums         DashEnums  `json:"enums"`
	Nodes         []DashNode `json:"nodes"`
	Links         []DashLink `json:"links"`
	Run           DashRun    `json:"run"`
}

type DashSource struct {
	Host  string `json:"host"`
	Build string `json:"build"`
}

type DashPoll struct {
	RecommendedMs int `json:"recommended_interval_ms"`
	MinMs         int `json:"min_interval_ms"`
	TimeoutMs     int `json:"timeout_ms"`
	MaxBytes      int `json:"max_bytes"`
}

type DashUnits struct {
	Rate    string `json:"rate"`
	Time    string `json:"time"`
	Percent string `json:"percent"`
}

type DashEnums struct {
	DecodeModes map[string]string `json:"decode_modes"` // mode -> display label
	SealStates  map[string]string `json:"seal_states"`  // state -> display label
	Status      []string          `json:"status"`
	NodeStates  []string          `json:"node_states"`
	LinkStates  []string          `json:"link_states"`
	RunPhases   []string          `json:"run_phases"`
	Provenance  []string          `json:"provenance"`
	Calibration []string          `json:"calibration"`
}

type DashNode struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Model      *string    `json:"model"`
	Family     *string    `json:"family,omitempty"`
	Chip       *string    `json:"chip"`
	State      string     `json:"state"`  // live | stale | unreachable | expired
	Source     string     `json:"source"` // samples (this monitor holds its samples) | fleet (known by gossip only)
	ObservedAt *float64   `json:"observed_at"`
	AgeS       *float64   `json:"age_s"`
	FreshnessS float64    `json:"freshness_s"` // the age past which this node is stale
	GPU        DashGPU    `json:"gpu"`
	CPU        DashCPU    `json:"cpu"`
	Decode     DashDecode `json:"decode"`
	Seal       DashSeal   `json:"seal"`
}

type DashGPU struct {
	WorkPct     *float64   `json:"work_pct"` // null unless calibrated: never 0 for unknown
	Range       [2]float64 `json:"range"`
	Calibration DashCalib  `json:"calibration"`
	ObservedAt  *float64   `json:"observed_at"`
}

type DashCalib struct {
	Status string  `json:"status"` // calibrated | uncalibrated | unknown
	ID     *string `json:"id"`
	Source string  `json:"source"`
}

type DashCPU struct {
	BusyPct *float64 `json:"busy_pct"`
}

type DashDecode struct {
	Mode   *string `json:"mode"`
	Label  *string `json:"label"`
	Source *string `json:"source"`
}

type DashSeal struct {
	State  *string `json:"state"`
	Label  *string `json:"label"`
	Source *string `json:"source"`
}

type DashLink struct {
	ID         string      `json:"id"`
	Ends       [2]*DashEnd `json:"ends"` // ends[1] null: a cabled interface with no pair (state missing)
	State      string      `json:"state"`
	MTU        *int        `json:"mtu"`
	Directions []DashDir   `json:"directions"` // [ends0->ends1, ends1->ends0]; empty when missing
}

type DashEnd struct {
	Node  string `json:"node"`
	Iface string `json:"iface"`
}

type DashDir struct {
	From          string   `json:"from"`
	To            string   `json:"to"`
	WireGBs       *float64 `json:"wire_gbs"`
	PayloadGBs    *float64 `json:"payload_gbs"`
	CompressedGBs *float64 `json:"compressed_gbs"`
	Provenance    *string  `json:"provenance"`
	ObservedAt    *float64 `json:"observed_at"`
	IntervalS     float64  `json:"interval_s"`
}

type DashRun struct {
	Configured bool            `json:"configured"`
	Phase      string          `json:"phase"`
	JobID      string          `json:"job_id,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Trip       string          `json:"trip,omitempty"`
	Transport  string          `json:"transport,omitempty"`
	Engine     string          `json:"engine,omitempty"`
	Decode     string          `json:"decode,omitempty"`
	Combo      string          `json:"combo,omitempty"`
	Seconds    int             `json:"seconds,omitempty"`
	Leg        int             `json:"leg,omitempty"`
	Legs       []DashLeg       `json:"legs,omitempty"`
	Elapsed    float64         `json:"elapsed"`
	Updated    float64         `json:"updated"`
	Note       string          `json:"note,omitempty"`
	Error      string          `json:"error,omitempty"`
	Live       []Cable         `json:"live,omitempty"`
	Results    []DashLegResult `json:"results,omitempty"`
}

type DashLeg struct {
	Path []string `json:"path"`
}

type DashLegResult struct {
	Leg       int     `json:"leg"`
	Path      string  `json:"path"`
	Cables    []Cable `json:"cables"`
	Aggregate float64 `json:"aggregate_gbs"`
	Verdict   string  `json:"verdict,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// Read is the shared projection. Unknown totals remain null; Snapshot is never cached as live.
type Read struct {
	Headline *Headline  `json:"headline"`
	Scene    *Scene     `json:"scene"`
	State    string     `json:"state"`
	Message  string     `json:"message"`
	ReadAt   time.Time  `json:"read_at"`
	Snapshot *Dashboard `json:"snapshot"`
	Outgoing *float64   `json:"outgoing_gbs"`
	Incoming *float64   `json:"incoming_gbs"`
	Total    *float64   `json:"total_gbs"`
}

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}
type Client struct {
	Endpoint string
	HTTP     Doer
	Now      func() time.Time
}

// freshnessAssessment is computed once at response completion and shared by
// headline, scene, and totals projections. Direction eligibility is aligned
// with the immutable snapshot's link and direction indexes.
type freshnessAssessment struct {
	envelopeFresh bool
	envelopeAge   float64
	withheld      bool
	nodes         map[string]bool
	directions    [][]bool
}

func (f freshnessAssessment) nodeEligible(id string) bool { return f.nodes[id] }

func (f freshnessAssessment) directionEligible(linkIndex, directionIndex int) bool {
	return linkIndex >= 0 && linkIndex < len(f.directions) && directionIndex >= 0 && directionIndex < len(f.directions[linkIndex]) && f.directions[linkIndex][directionIndex]
}

func (f freshnessAssessment) completeForHeadline(d Dashboard) bool {
	if !f.envelopeFresh || !f.nodeEligible(d.Source.Host) {
		return false
	}
	for linkIndex, link := range d.Links {
		if link.State != "up" || link.Ends[1] == nil {
			continue
		}
		if len(link.Directions) != 2 {
			return false
		}
		forward, reverse := false, false
		for directionIndex, direction := range link.Directions {
			if !f.directionEligible(linkIndex, directionIndex) {
				return false
			}
			if direction.From == link.Ends[0].Node && direction.To == link.Ends[1].Node {
				forward = true
			}
			if direction.From == link.Ends[1].Node && direction.To == link.Ends[0].Node {
				reverse = true
			}
		}
		if !forward || !reverse {
			return false
		}
	}
	return true
}

func assessFreshness(d Dashboard, completedAt time.Time) freshnessAssessment {
	assessment := freshnessAssessment{nodes: make(map[string]bool, len(d.Nodes)), directions: make([][]bool, len(d.Links))}
	assessment.envelopeAge = unixSeconds(completedAt) - d.GeneratedAt
	freshnessWindow := 6.0
	for _, node := range d.Nodes {
		if node.State == "live" && node.FreshnessS > 0 && node.FreshnessS < freshnessWindow {
			freshnessWindow = node.FreshnessS
		}
	}
	assessment.envelopeFresh = assessment.envelopeAge <= freshnessWindow && assessment.envelopeAge >= -2
	assessment.withheld = !assessment.envelopeFresh
	carriedEnvelopeAge := math.Max(0, assessment.envelopeAge)
	for _, node := range d.Nodes {
		eligible := assessment.envelopeFresh && node.State == "live" && node.AgeS != nil && *node.AgeS+carriedEnvelopeAge <= node.FreshnessS
		assessment.nodes[node.ID] = eligible
		if node.State == "stale" || (node.State == "live" && !eligible) {
			assessment.withheld = true
		}
	}
	for linkIndex, link := range d.Links {
		assessment.directions[linkIndex] = make([]bool, len(link.Directions))
		if link.State != "up" || link.Ends[1] == nil {
			continue
		}
		forward, reverse := false, false
		for directionIndex, direction := range link.Directions {
			age := math.Inf(1)
			if direction.ObservedAt != nil {
				age = unixSeconds(completedAt) - *direction.ObservedAt
			}
			eligible := assessment.envelopeFresh && assessment.nodeEligible(direction.From) && assessment.nodeEligible(direction.To) &&
				direction.ObservedAt != nil && age >= -2 && age <= 6 && direction.WireGBs != nil
			assessment.directions[linkIndex][directionIndex] = eligible
			if direction.From == link.Ends[0].Node && direction.To == link.Ends[1].Node {
				forward = true
			}
			if direction.From == link.Ends[1].Node && direction.To == link.Ends[0].Node {
				reverse = true
			}
			if !eligible {
				assessment.withheld = true
			}
		}
		if len(link.Directions) != 2 || !forward || !reverse {
			assessment.withheld = true
		}
	}
	return assessment
}

func unixSeconds(t time.Time) float64 { return float64(t.Unix()) + float64(t.Nanosecond())/1e9 }

// NewConfigured uses the fixed local monitor only when no endpoint setting is
// present. An explicitly empty setting disables reads; it is not a reset to the
// default.
func NewConfigured() *Client {
	endpoint, ok := os.LookupEnv(DashboardURLEnv)
	if !ok {
		endpoint = DefaultEndpoint
	}
	return New(endpoint)
}

func privateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		(ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1] >= 64 && ip.To4()[1] <= 127)
}

// New uses no environment proxy and rechecks every resolved address at dial time.
func New(endpoint string) *Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	transport := &http.Transport{DisableKeepAlives: true, MaxResponseHeaderBytes: MaxResponseHeaderBytes, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no private monitor address")
		}
		for _, ip := range ips {
			if !privateIP(ip.IP) {
				return nil, fmt.Errorf("monitor address is outside private networks")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	return &Client{Endpoint: endpoint, Now: time.Now, HTTP: &http.Client{Timeout: 2 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Read(ctx context.Context) Read {
	readAt := c.Now()
	r := Read{State: "unconfigured", Message: "Configure a Mercury monitor to view interconnect activity.", ReadAt: readAt}
	if c.Endpoint == "" {
		return r
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Path != "/api/v1/dashboard" || (net.ParseIP(u.Hostname()) != nil && !privateIP(net.ParseIP(u.Hostname()))) {
		r.State = "invalid"
		r.Message = "Mercury monitor configuration is invalid; use its private dashboard v1 address."
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		r.State = "invalid"
		r.Message = "Mercury monitor configuration is invalid."
		return r
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		r.State = "offline"
		r.Message = "Mercury monitor is unavailable. Check its connection and refresh."
		if errors.Is(err, os.ErrPermission) {
			r.State = "permission_required"
			r.Message = "Network access to Mercury is not permitted. Check Pantheon's network permissions and refresh."
		}
		return r
	}
	defer resp.Body.Close()
	r.State = "invalid"
	r.Message = "Mercury monitor returned an incompatible response. Check its dashboard v1 deployment."
	if resp.StatusCode == http.StatusForbidden {
		r.State = "permission_required"
		r.Message = "Mercury refused access. Check the monitor's private network access policy and refresh."
		return r
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Mercury-Dashboard-Schema") != "1" ||
		!strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		return r
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil || len(body) > MaxBytes {
		return r
	}
	d, err := Decode(body)
	if err != nil {
		return r
	}
	r.Snapshot = &d
	// Freshness is assessed once after the full bounded response has arrived and
	// decoded. ReadAt intentionally remains the request-start timestamp.
	assessment := assessFreshness(d, c.Now())
	r.State = d.Status
	r.Message = "Mercury snapshot · " + d.Source.Host
	if !assessment.envelopeFresh {
		r.State = "stale"
		r.Message = "Mercury snapshot is stale. Last observed values are shown."
	} else if assessment.withheld {
		r.State = "partial"
		r.Message = "Mercury snapshot is partial; stale or incomplete samples are withheld."
	}
	r.Headline = projectHeadline(d, assessment)
	r.Scene = projectScene(d, r.Headline.NodeOrder, assessment)
	byID := map[string]DashNode{}
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	outgoing, incoming := 0.0, 0.0
	_, present := byID[d.Source.Host]
	outKnown := present && assessment.envelopeFresh && assessment.nodeEligible(d.Source.Host)
	inKnown := outKnown
	localLinks := 0
	for linkIndex, l := range d.Links {
		hasOut, hasIn := false, false
		local := l.Ends[0] != nil && (l.Ends[0].Node == d.Source.Host || (l.Ends[1] != nil && l.Ends[1].Node == d.Source.Host))
		if local {
			localLinks++
		}
		for directionIndex, v := range l.Directions {
			if v.From == d.Source.Host {
				hasOut = true
			}
			if v.To == d.Source.Host {
				hasIn = true
			}
			if v.From != d.Source.Host && v.To != d.Source.Host {
				continue
			}
			fresh := l.State == "up" && assessment.directionEligible(linkIndex, directionIndex)
			if v.From == d.Source.Host {
				if !fresh || v.WireGBs == nil {
					outKnown = false
				} else {
					outgoing += *v.WireGBs
				}
			}
			if v.To == d.Source.Host {
				if !fresh || v.WireGBs == nil {
					inKnown = false
				} else {
					incoming += *v.WireGBs
				}
			}
		}
		if local {
			if !hasOut {
				outKnown = false
			}
			if !hasIn {
				inKnown = false
			}
		}
	}
	if localLinks == 0 {
		outKnown = false
		inKnown = false
	}
	if outKnown {
		r.Outgoing = &outgoing
	}
	if inKnown {
		r.Incoming = &incoming
	}
	if outKnown && inKnown {
		total := outgoing + incoming
		r.Total = &total
	}
	return r
}

func Decode(body []byte) (Dashboard, error) {
	var root, mapValue map[string]any
	if err := json.Unmarshal(schemaJSON, &root); err != nil {
		return Dashboard{}, err
	}
	if err := json.Unmarshal(body, &mapValue); err != nil {
		return Dashboard{}, err
	}
	if err := schemaCheck(root, root, mapValue, "$"); err != nil {
		return Dashboard{}, err
	}
	var d Dashboard
	if err := json.Unmarshal(body, &d); err != nil {
		return d, err
	}
	if !strings.HasPrefix(d.SchemaVersion, "1.") || d.GeneratedAt <= 0 {
		return d, fmt.Errorf("unsupported dashboard version/time")
	}
	ids := map[string]bool{}
	for _, n := range d.Nodes {
		if n.ID == "" || ids[n.ID] {
			return d, fmt.Errorf("duplicate or empty node")
		}
		ids[n.ID] = true
		if (n.GPU.Calibration.Status == "calibrated") != (n.GPU.WorkPct != nil) {
			return d, fmt.Errorf("GPU calibration mismatch")
		}
	}
	links := map[string]bool{}
	for _, l := range d.Links {
		if l.Ends[0] == nil || links[l.ID] {
			return d, fmt.Errorf("missing link origin or duplicate id")
		}
		links[l.ID] = true
		id := l.Ends[0].Node + ":" + l.Ends[0].Iface + "|"
		if l.Ends[1] != nil {
			id += l.Ends[1].Node + ":" + l.Ends[1].Iface
		}
		if id != l.ID || (l.State == "missing") != (l.Ends[1] == nil) || (l.Ends[1] == nil) != (len(l.Directions) == 0) {
			return d, fmt.Errorf("link identity/state mismatch")
		}
		for _, v := range l.Directions {
			if l.Ends[1] == nil || !((v.From == l.Ends[0].Node && v.To == l.Ends[1].Node) || (v.To == l.Ends[0].Node && v.From == l.Ends[1].Node)) {
				return d, fmt.Errorf("direction endpoints mismatch")
			}
		}
	}
	return d, nil
}
func schemaCheck(root, sch map[string]any, v any, path string) error {
	if ref, ok := sch["$ref"].(string); ok {
		defs, _ := root["$defs"].(map[string]any)
		r, ok := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unresolved %s", path, ref)
		}
		sch = r
	}
	if t, ok := sch["type"]; ok {
		ts, _ := t.([]any)
		if s, ok := t.(string); ok {
			ts = []any{s}
		}
		match := false
		for _, x := range ts {
			match = match || typeIs(x.(string), v)
		}
		if !match {
			return fmt.Errorf("%s: %v is not %v", path, v, t)
		}
	}
	if e, ok := sch["enum"].([]any); ok {
		in := false
		for _, x := range e {
			in = in || reflect.DeepEqual(x, v)
		}
		if !in {
			_, futureWord := v.(string)
			if !futureWord {
				return fmt.Errorf("%s: %v not in %v", path, v, e)
			}
		}
	}
	if c, ok := sch["const"]; ok && c != v {
		return fmt.Errorf("%s: %v, want %v", path, v, c)
	}
	switch x := v.(type) {
	case map[string]any:
		props, _ := sch["properties"].(map[string]any)
		req, _ := sch["required"].([]any)
		for _, k := range req {
			if _, ok := x[k.(string)]; !ok {
				return fmt.Errorf("%s: missing %s", path, k)
			}
		}
		for k, val := range x {
			if ps, ok := props[k].(map[string]any); ok {
				if err := schemaCheck(root, ps, val, path+"."+k); err != nil {
					return err
				}
				continue
			}
			switch ap := sch["additionalProperties"].(type) {
			case bool:
				// Ignore new optional properties under the v1 minor contract.
				_ = ap
			case map[string]any:
				if err := schemaCheck(root, ap, val, path+"."+k); err != nil {
					return err
				}
			}
		}
	case []any:
		if n, ok := sch["minItems"].(float64); ok && float64(len(x)) < n {
			return fmt.Errorf("%s: %d items, want >= %v", path, len(x), n)
		}
		if n, ok := sch["maxItems"].(float64); ok && float64(len(x)) > n {
			return fmt.Errorf("%s: %d items, want <= %v", path, len(x), n)
		}
		if it, ok := sch["items"].(map[string]any); ok {
			for i, val := range x {
				if err := schemaCheck(root, it, val, path+"["+strconv.Itoa(i)+"]"); err != nil {
					return err
				}
			}
		}
	case float64:
		if n, ok := sch["minimum"].(float64); ok && x < n {
			return fmt.Errorf("%s: %v < %v", path, x, n)
		}
		if n, ok := sch["maximum"].(float64); ok && x > n {
			return fmt.Errorf("%s: %v > %v", path, x, n)
		}
	}
	return nil
}

func typeIs(t string, v any) bool {
	switch t {
	case "null":
		return v == nil
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return false
}
