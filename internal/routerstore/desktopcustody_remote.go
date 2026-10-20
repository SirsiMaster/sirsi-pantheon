package routerstore

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	dc "github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
)

func (rs *RemoteStore) DesktopSleep(ctx context.Context, c dc.CustodyCall) (dc.CustodyState, error) {
	ctx, cancel := context.WithTimeout(ctx, rs.perCall)
	defer cancel()
	var out dc.CustodyState
	err := rs.callCtx(ctx, "DesktopSleep", []any{c}, &out)
	return out, err
}

// DesktopSleepAuthority requires the release's enrolled HTTPS origin. The
// CLI passes its compiled deployment origin, never a caller flag or env value.
// Local stores, spool overrides, arbitrary endpoints and redirected responses
// cannot supply production power authority.
func DesktopSleepAuthority(store Store, enrolledOrigin string) (dc.OnlineAuthority, error) {
	rs, ok := store.(*RemoteStore)
	u, err := url.Parse(enrolledOrigin)
	if !ok || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimRight(rs.base, "/") != strings.TrimRight(enrolledOrigin, "/") || rs.identityErr != nil || rs.token == "" {
		return nil, errors.New("sleep custody: release-enrolled canonical HTTPS authority required")
	}
	copyClient := *rs.client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("sleep custody: authority redirect refused")
	}
	// A distinct RemoteStore avoids mutating a shared client's transport policy.
	copyStore := NewRemoteStore(rs.base, rs.token)
	copyStore.host, copyStore.agent, copyStore.runtime, copyStore.threadID = rs.host, rs.agent, rs.runtime, rs.threadID
	copyStore.sessionDir = rs.sessionDir
	copyStore.client = &copyClient
	return copyStore, nil
}
