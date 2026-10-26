package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

const FabricTaskPageSize = 64

var ErrFabricTasksChanged = errors.New("fabric tasks changed; refresh the task list from its first page")

type FabricTask struct {
	Agent       string           `json:"agent"`
	ID          string           `json:"id"`
	Subject     string           `json:"subject"`
	Status      string           `json:"status"`
	Liveness    string           `json:"liveness"`
	Role        string           `json:"role"`
	Responsible string           `json:"responsible"`
	Stage       string           `json:"stage"`
	TestState   string           `json:"test_state"`
	Blocked     bool             `json:"blocker_recorded"`
	BlockerID   string           `json:"blocker_task_id,omitempty"`
	Updated     string           `json:"updated"`
	Provenance  FabricTaskOrigin `json:"provenance"`
	TextLimited bool             `json:"text_limited"`
}

type FabricPublicLink struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type FabricTaskOrigin struct {
	Source         string             `json:"source"`
	Created        string             `json:"created"`
	CommissionedAt string             `json:"commissioned_at"`
	CommissionedBy string             `json:"commissioned_by"`
	HasCharter     bool               `json:"charter_recorded"`
	HasOutline     bool               `json:"outline_recorded"`
	HasResult      bool               `json:"result_recorded"`
	Links          []FabricPublicLink `json:"public_links"`
	WithheldLinks  int                `json:"withheld_links"`
}

type FabricTaskPage struct {
	State      string       `json:"state"`
	Rows       []FabricTask `json:"rows"`
	Offset     int          `json:"offset"`
	Limit      int          `json:"limit"`
	Total      int          `json:"total"`
	Snapshot   string       `json:"snapshot"`
	NextOffset *int         `json:"next_offset"`
}

func fabricText(value string, limit int) (string, bool) {
	limited := false
	runes := make([]rune, 0, limit)
	for _, r := range value {
		if len(runes) == limit {
			limited = true
			break
		}
		if unicode.IsControl(r) {
			r = ' '
			limited = true
		}
		runes = append(runes, r)
	}
	return string(runes), limited
}

func fabricID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && len(value) <= 1024 && !strings.ContainsFunc(value, unicode.IsControl)
}

func fabricTime(value string) string {
	if len(value) > 64 {
		return ""
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return ""
	}
	return value
}

// Deliberately narrow public provenance: no credentials/query/fragment, local
// paths, journal URIs or private hosts. Withheld references remain in the task
// authority; this projection never reads their contents or follows a URL.
func fabricPublicURL(raw string) string {
	if len(raw) > 2048 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	return u.String()
}

// FabricTasksFrom projects already-read canonical rows. It owns no store or
// session. Continuations bind the ordered safe projection, not collection time;
// changed data refuses rather than silently skipping/mixing records.
func FabricTasksFrom(s Snapshot, offset int, expected string) (FabricTaskPage, error) {
	page := FabricTaskPage{State: "observed", Rows: []FabricTask{}, Limit: FabricTaskPageSize, Offset: offset}
	if offset < 0 || offset%FabricTaskPageSize != 0 {
		return page, errors.New("invalid fabric task page offset")
	}
	var rows []FabricTask
	seen := map[string]bool{}
	for _, lane := range s.Agents {
		for _, task := range lane.Tasks {
			if task.Status == "done" {
				continue
			}
			if !fabricID(lane.AgentID) || !fabricID(task.TaskID) || task.Agent != "" && task.Agent != lane.AgentID {
				return page, errors.New("fabric task identity unavailable")
			}
			key := lane.AgentID + "\x00" + task.TaskID
			if seen[key] {
				return page, errors.New("duplicate fabric task identity")
			}
			seen[key] = true
			rows = append(rows, fabricTask(lane.AgentID, task))
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Agent == rows[j].Agent {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].Agent < rows[j].Agent
	})
	// A blocker ID is exposed only as an exact reference within this same public
	// task set. Arbitrary blocker prose/commands are not a second message stream.
	for i := range rows {
		if !seen[rows[i].Agent+"\x00"+rows[i].BlockerID] {
			rows[i].BlockerID = ""
		}
	}
	h := sha256.New()
	_, _ = h.Write([]byte("fabric-safe-task-page-v1\n"))
	for _, row := range rows {
		if err := json.NewEncoder(h).Encode(row); err != nil {
			return page, err
		}
	}
	page.Snapshot = hex.EncodeToString(h.Sum(nil))
	if offset > 0 && expected == "" || expected != "" && expected != page.Snapshot {
		return page, ErrFabricTasksChanged
	}
	page.Total = len(rows)
	if offset > 0 && offset >= len(rows) {
		return page, errors.New("fabric task page offset outside observation")
	}
	end := min(offset+FabricTaskPageSize, len(rows))
	page.Rows = append(page.Rows, rows[offset:end]...)
	if end < len(rows) {
		page.NextOffset = &end
	}
	return page, nil
}

func fabricTask(agent string, task routerstore.Task) FabricTask {
	subject, cut := fabricText(task.Subject, 512)
	responsible, cutOwner := fabricText(task.ResponsibleParty, 128)
	commissioner, cutCommissioner := fabricText(task.CommissionedBy, 128)
	row := FabricTask{Agent: agent, ID: task.TaskID, Subject: subject, Responsible: responsible,
		Updated: fabricTime(task.Updated), Blocked: task.BlockedBy != "" || task.Status == "blocked", BlockerID: task.BlockedBy,
		TextLimited: cut || cutOwner || cutCommissioner,
		Provenance: FabricTaskOrigin{Source: "canonical-task-registry", Created: fabricTime(task.Created),
			CommissionedAt: fabricTime(task.CommissionedAt), CommissionedBy: commissioner,
			HasCharter: task.Charter != nil, HasOutline: task.Outline != nil, HasResult: task.ResultRef != "",
			Links: []FabricPublicLink{}}, Role: brand.Dim.Name(), Status: "unknown", Liveness: "unknown", Stage: "unknown", TestState: "unknown"}
	if task.Status == "pending" || task.Status == "in-progress" || task.Status == "blocked" {
		row.Status = task.Status
		row.Role = brand.Gold.Name()
	}
	if task.Liveness == "active" || task.Liveness == "stalled" {
		row.Liveness = task.Liveness
	}
	if task.Status == "blocked" || task.Liveness == "stalled" {
		row.Role = brand.Warn.Name()
	}
	for _, value := range []string{"spec", "build", "review", "verify", "shipped"} {
		if task.Stage == value {
			row.Stage = value
		}
	}
	for _, value := range []string{"untested", "tested", "passed", "failed"} {
		if task.TestState == value {
			row.TestState = value
		}
	}
	for _, link := range task.Links {
		if len(row.Provenance.Links) >= 8 || (link.Kind != "canon" && link.Kind != "repo" && link.Kind != "pr" && link.Kind != "owner-instruction" && link.Kind != "evidence") {
			row.Provenance.WithheldLinks++
			continue
		}
		public := fabricPublicURL(link.URL)
		if public == "" {
			row.Provenance.WithheldLinks++
			continue
		}
		row.Provenance.Links = append(row.Provenance.Links, FabricPublicLink{Kind: link.Kind, URL: public})
	}
	return row
}
