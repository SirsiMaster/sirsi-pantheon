package routerstore

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	dc "github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
)

func (s *server) desktopSleepContext(ctx context.Context, r *http.Request, sess Session, host string) (context.Context, error) {
	ms, err := strconv.ParseInt(strings.SplitN(r.Header.Get("X-Sirsi-Nonce"), ".", 2)[0], 10, 64)
	skew := s.opts.now().Sub(time.UnixMilli(ms))
	if err != nil || skew > 5*time.Second || skew < -5*time.Second {
		return nil, errors.New("sleep custody: native clock exceeds five-second bound")
	}
	if host == bootstrapHost || sess.ID == "" || sess.ThreadID == "" {
		return nil, errors.New("sleep custody: exact enrolled host token/session required; bootstrap is not a participant")
	}
	t, err := s.store.LookupHostToken(strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
	if err != nil {
		return nil, err
	}
	if t.Host != sess.Host {
		return nil, errors.New("sleep custody: exact enrolled host binding required")
	}
	if err := s.ruleOfRa(sess, "DesktopSleep"); err != nil {
		return nil, err
	}
	p := dc.Participant{TokenID: t.ID, SessionID: sess.ID, MachineID: t.MachineID, Host: sess.Host, Agent: sess.Agent, ThreadID: sess.ThreadID, RuntimeSHA256: sess.RuntimeHash}
	return context.WithValue(ctx, desktopSleepActorKey{}, desktopSleepActor{p}), nil
}
