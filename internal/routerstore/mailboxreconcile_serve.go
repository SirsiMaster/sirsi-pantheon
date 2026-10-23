package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

func (s *server) mailboxReconcileContext(ctx context.Context, sess Session, host string) (context.Context, error) {
	// Mandatory even when an old embedding sets the general audience gate to
	// log/off. A restricted token does not obtain a foreign mailbox capability.
	if err := s.ruleOfRa(sess, "ReconcileMailboxNotice"); err != nil {
		return nil, err
	}
	if s.opts.RecipientRegistry == "" {
		return nil, errors.New("mailbox reconciliation: canonical actor-only registry required")
	}
	f, err := os.Open(s.opts.RecipientRegistry)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("mailbox reconciliation: invalid registry size/read")
	}
	var reg struct {
		Agents map[string]struct {
			Type, Description string
			Wake              struct{ Mechanism string }
			Contract          *RecipientDeliveryContract `json:"delivery_contract"`
		}
	}
	if err := json.Unmarshal(b, &reg); err != nil {
		return nil, err
	}
	a, exists := reg.Agents["horus"]
	actorOnly := exists && a.Type == "service" && a.Wake.Mechanism == "none"
	if a.Contract != nil {
		actorOnly = actorOnly && a.Contract.Version == 1 && a.Contract.Mode == "actor-only"
	} else {
		description := strings.ToLower(a.Description)
		actorOnly = actorOnly && strings.Contains(description, "actor-only") && strings.Contains(description, "never route inbox")
	}
	if !actorOnly {
		return nil, errors.New("mailbox reconciliation: Horus must remain an actor-only service")
	}
	principal := mailboxActor{Agent: sess.Agent, Host: sess.Host, SessionID: sess.ID, ThreadID: sess.ThreadID, Runtime: sess.RuntimeHash, Bootstrap: host == bootstrapHost, ServiceRecipient: true}
	principal.FreshFor = s.opts.RuleOfRaStale
	return context.WithValue(ctx, mailboxActorKey{}, principal), nil
}
