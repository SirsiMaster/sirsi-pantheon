package seshat

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GoogleWorkspaceAdapter ingests knowledge from Google Workspace (Docs, Sheets, Drive).
// Requires an OAuth2 token — use `pantheon seshat auth google` to set up.
type GoogleWorkspaceAdapter struct {
	// TokenFile is the path to the Google OAuth2 token JSON.
	// If empty, uses ~/.config/seshat/google_token.json
	TokenFile string

	// CredentialsFile is the path to the Google OAuth2 credentials.
	// If empty, uses ~/.config/seshat/google_credentials.json
	CredentialsFile string
}

func (a *GoogleWorkspaceAdapter) Name() string { return "google-workspace" }
func (a *GoogleWorkspaceAdapter) Description() string {
	return "Google Workspace (Docs, Sheets, Drive) via OAuth2"
}

func (a *GoogleWorkspaceAdapter) configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "seshat")
}

func (a *GoogleWorkspaceAdapter) tokenFile() string {
	if a.TokenFile != "" {
		return a.TokenFile
	}
	return filepath.Join(a.configDir(), "google_token.json")
}

// googleToken represents a stored OAuth2 token.
type googleToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Expiry       string `json:"expiry"`
}

// driveFile represents a Google Drive file metadata response.
type driveFile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	ModifiedTime string `json:"modifiedTime"`
	Description  string `json:"description"`
}

type driveListResponse struct {
	Files         []driveFile `json:"files"`
	NextPageToken string      `json:"nextPageToken"`
}

func (a *GoogleWorkspaceAdapter) loadToken() (*googleToken, error) {
	data, err := os.ReadFile(a.tokenFile())
	if err != nil {
		return nil, fmt.Errorf("no Google token found at %s — run 'pantheon seshat auth google' to authenticate", a.tokenFile())
	}
	var tok googleToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("invalid token file: %w", err)
	}
	return &tok, nil
}

// Ingest fetches recent Google Docs and Sheets, extracting their content as Knowledge Items.
func (a *GoogleWorkspaceAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	token, err := a.loadToken()
	if err != nil {
		return nil, err
	}

	var items []KnowledgeItem

	// Fetch recent Google Docs
	docs, err := a.listDriveFiles(token, "application/vnd.google-apps.document", since)
	if err != nil {
		fmt.Printf("  ⚠️  Google Docs: %v\n", err)
	} else {
		for _, doc := range docs {
			content, err := a.exportFileAsText(token, doc.ID, "text/plain")
			if err != nil {
				continue
			}
			items = append(items, KnowledgeItem{
				Title:   doc.Name,
				Summary: truncate(content, 500),
				References: []KIReference{
					{Type: "source", Value: "google-docs"},
					{Type: "url", Value: fmt.Sprintf("https://docs.google.com/document/d/%s", doc.ID)},
				},
			})
		}
	}

	// Fetch recent Google Sheets (metadata only — full content is complex)
	sheets, err := a.listDriveFiles(token, "application/vnd.google-apps.spreadsheet", since)
	if err != nil {
		fmt.Printf("  ⚠️  Google Sheets: %v\n", err)
	} else {
		for _, sheet := range sheets {
			content, err := a.exportFileAsText(token, sheet.ID, "text/csv")
			if err != nil {
				content = sheet.Description
			}
			items = append(items, KnowledgeItem{
				Title:   sheet.Name,
				Summary: truncate(content, 500),
				References: []KIReference{
					{Type: "source", Value: "google-sheets"},
					{Type: "url", Value: fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", sheet.ID)},
				},
			})
		}
	}

	return items, nil
}

func (a *GoogleWorkspaceAdapter) listDriveFiles(token *googleToken, mimeType string, since time.Time) ([]driveFile, error) {
	sinceStr := since.Format(time.RFC3339)
	query := fmt.Sprintf("mimeType='%s' and modifiedTime>'%s' and trashed=false", mimeType, sinceStr)

	url := fmt.Sprintf("https://www.googleapis.com/drive/v3/files?q=%s&fields=files(id,name,mimeType,modifiedTime,description)&orderBy=modifiedTime desc&pageSize=50",
		strings.ReplaceAll(query, " ", "%20"))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Drive API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("Google token expired — run 'pantheon seshat auth google' to refresh")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Drive API error %d: %s", resp.StatusCode, string(body))
	}

	var result driveListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse Drive response: %w", err)
	}

	return result.Files, nil
}

func (a *GoogleWorkspaceAdapter) exportFileAsText(token *googleToken, fileID, exportMime string) (string, error) {
	url := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s/export?mimeType=%s", fileID, exportMime)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("export error %d", resp.StatusCode)
	}

	// Limit to 100KB of text
	limited := io.LimitReader(resp.Body, 100*1024)
	body, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
