package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

var routerSecretResourcePattern = regexp.MustCompile(`^projects/([a-z][a-z0-9-]{4,61}[a-z0-9]|[0-9]{6,})/secrets/[A-Za-z0-9_-]{1,255}$`)

// Cloud mint jobs must not put bearer credentials in Cloud Logging. Publishing
// fails closed: revoke the fresh token if its secure delivery is not confirmed.
func publishMintedRouterToken(ctx context.Context, out io.Writer, plain string, rec routerstore.HostToken, resource string,
	publish func(context.Context, string, string) (string, error), revoke func(string) error) error {
	return publishMintedToken(ctx, out, plain, rec, resource, publish, revoke, false)
}

// publishMintedHostWideToken is the same secure delivery for an UNSCOPED host token (a Mac's token, whose lanes are
// not enumerable and which must keep its host-wide reach). It is only reachable through the explicit --host-wide flag;
// the default path above still refuses an unscoped token, so a cloud sandbox can never be given one by accident.
func publishMintedHostWideToken(ctx context.Context, out io.Writer, plain string, rec routerstore.HostToken, resource string,
	publish func(context.Context, string, string) (string, error), revoke func(string) error) error {
	if len(rec.Agents) != 0 {
		if err := revoke(rec.ID); err != nil {
			return fmt.Errorf("host-wide publication of a scoped token; CLEANUP_DEBT: token %s revocation failed", rec.ID)
		}
		return errors.New("host-wide publication refused for a scoped token; fresh token revoked")
	}
	return publishMintedToken(ctx, out, plain, rec, resource, publish, revoke, true)
}

func publishMintedToken(ctx context.Context, out io.Writer, plain string, rec routerstore.HostToken, resource string,
	publish func(context.Context, string, string) (string, error), revoke func(string) error, allowHostWide bool) error {
	if (len(rec.Agents) == 0 && !allowHostWide) || !routerSecretResourcePattern.MatchString(resource) {
		if err := revoke(rec.ID); err != nil {
			return fmt.Errorf("invalid secure scope; CLEANUP_DEBT: token %s revocation failed", rec.ID)
		}
		return errors.New("invalid secure scope; fresh token revoked")
	}
	version, err := publish(ctx, resource, plain)
	if err != nil {
		if revokeErr := revoke(rec.ID); revokeErr != nil {
			return fmt.Errorf("secure token publication failed; CLEANUP_DEBT: token %s revocation failed", rec.ID)
		}
		return errors.New("secure token publication failed; fresh token revoked")
	}
	scope := strings.Join(rec.Agents, ",")
	if len(rec.Agents) == 0 {
		scope = "host-wide (unscoped)"
	}
	_, err = fmt.Fprintf(out, "token id: %s  host: %s\nscope: %s\nsecret version: %s\n", rec.ID, rec.Host, scope, version)
	if err != nil {
		if revokeErr := revoke(rec.ID); revokeErr != nil {
			return fmt.Errorf("receipt write failed; CLEANUP_DEBT: token %s revocation failed", rec.ID)
		}
		return errors.New("receipt write failed; fresh token revoked")
	}
	return err
}

func publishScopedRouterSecret(ctx context.Context, resource, plain string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("credential publication redirects are forbidden")
	}}
	return publishScopedRouterSecretWithClient(ctx, resource, plain, client)
}

func publishScopedRouterSecretWithClient(ctx context.Context, resource, plain string, client *http.Client) (string, error) {
	if !routerSecretResourcePattern.MatchString(resource) || plain == "" {
		return "", errors.New("invalid scoped secret publication")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("cloud identity unavailable")
	}
	var identity struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&identity)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Metadata-Flavor") != "Google" || decodeErr != nil || identity.AccessToken == "" || identity.TokenType != "Bearer" || identity.ExpiresIn <= 0 {
		return "", errors.New("cloud identity unavailable")
	}
	body, err := json.Marshal(map[string]any{"payload": map[string]string{"data": base64.StdEncoding.EncodeToString([]byte(plain))}})
	if err != nil {
		return "", errors.New("secret payload encoding failed")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://secretmanager.googleapis.com/v1/"+resource+":addVersion", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+identity.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return "", errors.New("secret publication unavailable")
	}
	var result struct {
		Name string `json:"name"`
	}
	decodeErr = json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&result)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || decodeErr != nil || !regexp.MustCompile(`^`+regexp.QuoteMeta(resource)+`/versions/[1-9][0-9]*$`).MatchString(result.Name) {
		return "", errors.New("secret publication not confirmed")
	}
	return result.Name, nil
}
