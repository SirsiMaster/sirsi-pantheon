package dashboard

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
)

// LedgerAgentProducer builds one lane's ledger. Production wiring passes a
// closure over ledger.Build, the same library call behind
// `sirsi router ledger <agent> --json`; the dashboard never shells out.
type LedgerAgentProducer func(agent string) (ledger.Snapshot, error)

// RouterLedgerItem and RouterLedgerTask are the additive v1 read shape for one
// lane ledger. Task fields use the existing public Fabric projection. Items carry
// only the blocked flag: the store's blocked_by is free text and never leaves.
type RouterLedgerItem struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	From       string  `json:"from"`
	Type       string  `json:"type"`
	Opened     string  `json:"opened"`
	AgeSeconds float64 `json:"age_seconds"`
	Blocked    bool    `json:"blocked"`
}

type RouterLedgerTask struct {
	TaskID           string `json:"task_id"`
	Subject          string `json:"subject"`
	Status           string `json:"status"`
	Phase            string `json:"phase"`
	BlockedBy        string `json:"blocked_by"`
	Updated          string `json:"updated"`
	ResponsibleParty string `json:"responsible_party"`
}

type RouterLedger struct {
	GeneratedAt string             `json:"generated_at"`
	Agent       string             `json:"agent"`
	Items       []RouterLedgerItem `json:"items"`
	Tasks       []RouterLedgerTask `json:"tasks"`
}

var ledgerAgentRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)

// readGET applies the shared surface headers and accepts only GET (and the
// CORS preflight). It reports whether the handler should continue.
func (s *Server) readGET(w http.ResponseWriter, r *http.Request) bool {
	s.surfaceHeaders(w, r)
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// apiRouterLedger serves GET /api/router/v1/ledger?agent=<id>. An unknown agent
// is a 404 and an unavailable producer a 503 with a reason; a failure is never
// rendered as an empty-but-healthy body.
func (s *Server) apiRouterLedger(w http.ResponseWriter, r *http.Request) {
	if !s.readGET(w, r) {
		return
	}
	agent := r.URL.Query().Get("agent")
	if !ledgerAgentRE.MatchString(agent) {
		writeError(w, "agent query parameter required", http.StatusBadRequest)
		return
	}
	if s.cfg.LedgerAgentFn == nil {
		writeError(w, "ledger producer unavailable: not configured", http.StatusServiceUnavailable)
		return
	}
	snap, err := s.cfg.LedgerAgentFn(agent)
	if err != nil {
		writeError(w, "ledger producer unavailable: store read failed", http.StatusServiceUnavailable)
		return
	}
	out := RouterLedger{GeneratedAt: snap.GeneratedAt, Agent: agent, Items: []RouterLedgerItem{}, Tasks: []RouterLedgerTask{}}
	found := false
	scoped := ledger.Snapshot{}
	for _, a := range snap.Agents {
		if a.AgentID != agent {
			continue
		}
		found = true
		scoped.Agents = append(scoped.Agents, a)
		for _, it := range a.Items {
			out.Items = append(out.Items, RouterLedgerItem{ID: it.ID, Title: it.Title, From: it.From, Type: it.Type, Opened: it.Opened, AgeSeconds: it.AgeSeconds, Blocked: it.Blocked})
		}
	}
	// A registered lane with nothing open has no ledger entry. Only a successful
	// lane inventory can distinguish that empty ledger from an unknown agent;
	// unavailable inventory is not evidence that the agent does not exist.
	if !found {
		known, err := s.knownLane(agent)
		if err != nil {
			writeError(w, "lane registry unavailable", http.StatusServiceUnavailable)
			return
		}
		if !known {
			writeError(w, "unknown agent", http.StatusNotFound)
			return
		}
	}
	// Project both open and completed tasks with the same public allowlist used
	// by /api/fabric. Arbitrary blocker prose never leaves; the phase is a
	// display label cleaned like the subject (owner decision 2026-11-08).
	rows, err := ledger.FabricAllTasksFrom(scoped)
	if err != nil {
		writeError(w, "ledger projection unavailable", http.StatusServiceUnavailable)
		return
	}
	phase := map[string]string{}
	for _, a := range scoped.Agents {
		for _, t := range a.Tasks {
			phase[t.TaskID] = ledger.PhaseLabel(t.Phase)
		}
	}
	for _, t := range rows {
		out.Tasks = append(out.Tasks, RouterLedgerTask{TaskID: t.ID, Subject: t.Subject, Status: t.Status, Phase: phase[t.ID], BlockedBy: t.BlockerID, Updated: t.Updated, ResponsibleParty: t.Responsible})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) knownLane(agent string) (bool, error) {
	if s.cfg.RouterFn == nil {
		return false, errors.New("router producer not configured")
	}
	snap, err := s.cfg.RouterFn()
	if err != nil {
		return false, err
	}
	for _, l := range snap.Lanes.List {
		if l.Agent == agent {
			return true, nil
		}
	}
	return false, nil
}

// apiRouterJobs is the cloud-jobs slot (spec section 4). No producer exists, so
// the honest answer is 404; the UI renders that as "no source connected".
func (s *Server) apiRouterJobs(w http.ResponseWriter, r *http.Request) {
	if !s.readGET(w, r) {
		return
	}
	writeError(w, "no source connected", http.StatusNotFound)
}
