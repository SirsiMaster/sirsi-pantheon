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
	"google.golang.org/api/option"
)

// gmailClient is the real Client, backed by the Gmail API. Scopes are
// gmail.modify (read/label/archive — cannot empty trash) and
// gmail.settings.basic (read filter settings). Never requests
// gmail.send or the full-account-delete scopes (Rule: never send mail,
// never delete).
type gmailClient struct {
	svc *gmail.Service
}

// storedToken is the on-disk shape written by google-auth-oauthlib's
// Credentials.to_json() (the Python prototype's auth.py) — carrying the
// refresh token plus client id/secret so Go can refresh it too.
type storedToken struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token"`
	TokenURI     string   `json:"token_uri"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Expiry       string   `json:"expiry"` // RFC3339
}

// NewClient loads a stored OAuth token from tokenPath and returns a Client
// bound to that mailbox, with automatic refresh. Run `sirsi mail auth
// <label>` first to mint the token file (one-time interactive consent).
func NewClient(ctx context.Context, tokenPath string) (Client, error) {
	data, readErr := os.ReadFile(tokenPath)
	if readErr != nil {
		return nil, fmt.Errorf("mailops: read token %s: %w (run 'sirsi mail auth' first)", tokenPath, readErr)
	}
	var st storedToken
	if unmarshalErr := json.Unmarshal(data, &st); unmarshalErr != nil {
		return nil, fmt.Errorf("mailops: parse token %s: %w", tokenPath, unmarshalErr)
	}
	cfg := &oauth2.Config{
		ClientID:     st.ClientID,
		ClientSecret: st.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       st.Scopes,
	}
	tok := &oauth2.Token{AccessToken: st.Token, RefreshToken: st.RefreshToken}
	if expiry, parseErr := time.Parse(time.RFC3339, st.Expiry); parseErr == nil {
		tok.Expiry = expiry
	}
	svc, err := gmail.NewService(ctx, option.WithTokenSource(cfg.TokenSource(ctx, tok)))
	if err != nil {
		return nil, fmt.Errorf("mailops: gmail service: %w", err)
	}
	return &gmailClient{svc: svc}, nil
}

func (c *gmailClient) ListInboxIDs(query string) ([]string, error) {
	var out []string
	pageTok := ""
	for {
		call := c.svc.Users.Messages.List("me").Q(query).MaxResults(500)
		if pageTok != "" {
			call = call.PageToken(pageTok)
		}
		r, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("mailops: list messages: %w", err)
		}
		for _, m := range r.Messages {
			out = append(out, m.Id)
		}
		if r.NextPageToken == "" {
			return out, nil
		}
		pageTok = r.NextPageToken
	}
}

func (c *gmailClient) HasEmptyTextPart(ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		m, err := c.svc.Users.Messages.Get("me", id).Format("full").Do()
		if err != nil {
			return nil, fmt.Errorf("mailops: get message %s: %w", id, err)
		}
		if m.Payload != nil && len(m.Payload.Parts) > 0 && emptyTextPart(m.Payload) {
			out = append(out, id)
		}
	}
	return out, nil
}

func emptyTextPart(p *gmail.MessagePart) bool {
	if p == nil {
		return false
	}
	isText := len(p.MimeType) >= 5 && p.MimeType[:5] == "text/"
	if isText && p.Body != nil && p.Body.Size == 0 && len(p.Parts) == 0 {
		return true
	}
	for _, c := range p.Parts {
		if emptyTextPart(c) {
			return true
		}
	}
	return false
}

func (c *gmailClient) Headers(ids []string) (map[string]Message, error) {
	out := make(map[string]Message, len(ids))
	for _, id := range ids {
		m, err := c.svc.Users.Messages.Get("me", id).Format("metadata").
			MetadataHeaders("From", "Subject", "Date").Do()
		if err != nil {
			return nil, fmt.Errorf("mailops: get headers %s: %w", id, err)
		}
		msg := Message{ID: id}
		if m.Payload != nil {
			for _, h := range m.Payload.Headers {
				switch h.Name {
				case "From":
					msg.From = h.Value
				case "Subject":
					msg.Subject = h.Value
				case "Date":
					msg.Date = h.Value
				}
			}
		}
		out[id] = msg
	}
	return out, nil
}

func (c *gmailClient) SendersLastYear() (map[string]int, map[string]bool, error) {
	ids, err := c.ListInboxIDs("in:inbox newer_than:365d")
	if err != nil {
		return nil, nil, err
	}
	counts := map[string]int{}
	lists := map[string]bool{}
	for _, id := range ids {
		m, err := c.svc.Users.Messages.Get("me", id).Format("metadata").
			MetadataHeaders("From", "List-Unsubscribe").Do()
		if err != nil {
			return nil, nil, fmt.Errorf("mailops: get sender headers %s: %w", id, err)
		}
		var from string
		hasUnsub := false
		if m.Payload != nil {
			for _, h := range m.Payload.Headers {
				switch h.Name {
				case "From":
					from = normalizeFrom(h.Value)
				case "List-Unsubscribe":
					hasUnsub = true
				}
			}
		}
		if from == "" {
			from = "?"
		}
		counts[from]++
		if hasUnsub {
			lists[from] = true
		}
	}
	return counts, lists, nil
}

func normalizeFrom(header string) string {
	// "Name <addr@host>" -> "addr@host"; bare addresses pass through.
	start := -1
	for i, r := range header {
		if r == '<' {
			start = i + 1
		}
	}
	if start < 0 {
		return header
	}
	end := len(header)
	for i := len(header) - 1; i >= start; i-- {
		if header[i] == '>' {
			end = i
			break
		}
	}
	return header[start:end]
}

func (c *gmailClient) Archive(ids []string) error {
	for i := 0; i < len(ids); i += 1000 {
		end := i + 1000
		if end > len(ids) {
			end = len(ids)
		}
		req := &gmail.BatchModifyMessagesRequest{Ids: ids[i:end], RemoveLabelIds: []string{"INBOX"}}
		if err := c.svc.Users.Messages.BatchModify("me", req).Do(); err != nil {
			return fmt.Errorf("mailops: archive batch: %w", err)
		}
	}
	return nil
}
