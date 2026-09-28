package mailops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
)

// Scopes requested for every mailops account. gmail.modify allows read,
// label, and archive but NOT emptying trash; gmail.settings.basic allows
// reading filter settings. Never gmail.send, never any delete-all scope.
var Scopes = []string{gmail.GmailModifyScope, gmail.GmailSettingsBasicScope}

// Authorize runs the one-time interactive OAuth consent flow for a mailbox
// and writes the refresh token to tokenPath (mode 0600). clientSecretPath is
// the GCP OAuth Desktop client's credentials.json. exchange performs the
// actual code exchange (injected — Rule A16 — so this is testable without a
// browser).
func Authorize(ctx context.Context, clientSecretPath, tokenPath string, exchange func(*oauth2.Config) (*oauth2.Token, error)) error {
	data, err := os.ReadFile(clientSecretPath)
	if err != nil {
		return fmt.Errorf("mailops: read client secret %s: %w", clientSecretPath, err)
	}
	cfg, err := google.ConfigFromJSON(data, Scopes...)
	if err != nil {
		return fmt.Errorf("mailops: parse client secret: %w", err)
	}
	tok, err := exchange(cfg)
	if err != nil {
		return fmt.Errorf("mailops: authorize: %w", err)
	}
	st := storedToken{
		Token: tok.AccessToken, RefreshToken: tok.RefreshToken,
		TokenURI: cfg.Endpoint.TokenURL, ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret,
		Scopes: Scopes,
	}
	if !tok.Expiry.IsZero() {
		st.Expiry = tok.Expiry.Format(time.RFC3339)
	}
	buf, err := json.Marshal(st)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(tokenPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("mailops: write token %s: %w", tokenPath, err)
	}
	defer f.Close()
	_, err = f.Write(buf)
	return err
}
