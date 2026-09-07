package routerboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// ControlActionRequest is the closed request shape for the authenticated
// worker-control mutation endpoint. It deliberately contains values, never a
// shell command: every verb maps to one routerstore transaction primitive.
type ControlActionRequest struct {
	Verb             string `json:"verb"`
	From             string `json:"from,omitempty"`
	To               string `json:"to,omitempty"`
	Title            string `json:"title,omitempty"`
	Type             string `json:"type,omitempty"`
	Instructions     string `json:"instructions,omitempty"`
	SubjectKey       string `json:"subject_key,omitempty"`
	SourceItem       string `json:"source_item,omitempty"`
	Agent            string `json:"agent,omitempty"`
	TaskID           string `json:"task_id,omitempty"`
	Subject          string `json:"subject,omitempty"`
	Phase            string `json:"phase,omitempty"`
	ResponsibleParty string `json:"responsible_party,omitempty"`
	Worker           string `json:"worker,omitempty"`
	ThreadID         string `json:"thread_id,omitempty"`
	LeaseToken       string `json:"lease_token,omitempty"`
	TTLSeconds       int64  `json:"ttl_seconds,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ResultRef        string `json:"result_ref,omitempty"`
}

type ControlActionResponse struct {
	Schema    string                 `json:"schema"`
	Authority string                 `json:"authority"`
	Verb      string                 `json:"verb"`
	ItemID    string                 `json:"item_id,omitempty"`
	Deduped   bool                   `json:"deduped,omitempty"`
	TaskID    string                 `json:"task_id,omitempty"`
	Lease     *routerstore.TaskLease `json:"lease,omitempty"`
}

func (r ControlActionRequest) validate() error {
	r.Verb = strings.TrimSpace(r.Verb)
	switch r.Verb {
	case "message", "review_request":
		if strings.TrimSpace(r.From) == "" || strings.TrimSpace(r.To) == "" || strings.TrimSpace(r.Title) == "" {
			return fmt.Errorf("%s requires from, to, and title", r.Verb)
		}
		if r.Verb == "review_request" {
			r.Type = "review"
		}
	case "delegate":
		if strings.TrimSpace(r.Agent) == "" || strings.TrimSpace(r.TaskID) == "" || strings.TrimSpace(r.Subject) == "" {
			return fmt.Errorf("delegate requires agent, task_id, and subject")
		}
	case "claim":
		if strings.TrimSpace(r.Agent) == "" || strings.TrimSpace(r.Worker) == "" || strings.TrimSpace(r.ThreadID) == "" {
			return fmt.Errorf("claim requires agent, worker, and thread_id")
		}
	case "cancel_handback":
		if strings.TrimSpace(r.Agent) == "" || strings.TrimSpace(r.TaskID) == "" || strings.TrimSpace(r.LeaseToken) == "" {
			return fmt.Errorf("cancel_handback requires agent, task_id, and lease_token")
		}
	case "result_return":
		if strings.TrimSpace(r.Agent) == "" || strings.TrimSpace(r.TaskID) == "" || strings.TrimSpace(r.LeaseToken) == "" || strings.TrimSpace(r.ResultRef) == "" {
			return fmt.Errorf("result_return requires agent, task_id, lease_token, and result_ref")
		}
	default:
		return fmt.Errorf("unsupported control action %q", r.Verb)
	}
	if r.TTLSeconds < 0 || r.TTLSeconds > int64((24*time.Hour)/time.Second) {
		return fmt.Errorf("ttl_seconds must be between 0 and 86400")
	}
	return nil
}

// ApplyControlAction maps the closed worker-control verbs to the existing
// canonical routerstore. No second control-plane database or subprocess path
// is introduced.
func ApplyControlAction(store *routerstore.Store, req ControlActionRequest) (ControlActionResponse, error) {
	if store == nil {
		return ControlActionResponse{}, fmt.Errorf("control store is nil")
	}
	req.Verb = strings.TrimSpace(req.Verb)
	if err := req.validate(); err != nil {
		return ControlActionResponse{}, err
	}
	out := ControlActionResponse{Schema: ControlSchema, Authority: "canonical-routerstore", Verb: req.Verb}
	switch req.Verb {
	case "message", "review_request":
		msgType := strings.TrimSpace(req.Type)
		if req.Verb == "review_request" {
			msgType = "review"
		}
		id, deduped, err := store.SendGuarded(routerstore.SendReq{
			From: req.From, To: req.To, Title: req.Title, Type: msgType,
			Instructions: req.Instructions, SubjectKey: req.SubjectKey, SourceItem: req.SourceItem,
		})
		if err != nil {
			return ControlActionResponse{}, err
		}
		out.ItemID, out.Deduped = id, deduped
	case "delegate":
		if err := store.AddTask(routerstore.Task{
			Agent: req.Agent, TaskID: req.TaskID, Subject: req.Subject,
			Phase: req.Phase, ResponsibleParty: req.ResponsibleParty,
		}); err != nil {
			return ControlActionResponse{}, err
		}
		out.TaskID = req.TaskID
	case "claim":
		ttl := 10 * time.Minute
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}
		var (
			lease *routerstore.TaskLease
			err   error
		)
		if strings.TrimSpace(req.TaskID) == "" {
			lease, err = store.ClaimNextTask(req.Agent, req.Worker, req.ThreadID, ttl)
		} else {
			lease, err = store.ClaimTask(req.Agent, req.TaskID, req.Worker, req.ThreadID, ttl)
		}
		if err != nil {
			return ControlActionResponse{}, err
		}
		out.TaskID, out.Lease = lease.TaskID, lease
	case "cancel_handback":
		if err := store.ReleaseTaskLease(req.Agent, req.TaskID, req.LeaseToken, req.Reason); err != nil {
			return ControlActionResponse{}, err
		}
		out.TaskID = req.TaskID
	case "result_return":
		if err := store.CompleteTaskLease(req.Agent, req.TaskID, req.LeaseToken, req.ResultRef); err != nil {
			return ControlActionResponse{}, err
		}
		out.TaskID = req.TaskID
	}
	return out, nil
}
