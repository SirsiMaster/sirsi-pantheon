package seshat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GoogleWorkspaceAdapter ingests knowledge from Google Workspace (Docs, Sheets, Drive).
// Requires an OAuth2 token — use `sirsi seshat auth google` to set up.
type GoogleWorkspaceAdapter struct {
	// TokenFile is the path to the Google OAuth2 token JSON.
	// If empty, uses ~/.config/seshat/google_token.json
	TokenFile string

	// CredentialsFile is the path to the Google OAuth2 credentials.
	// If empty, uses ~/.config/seshat/google_credentials.json
	CredentialsFile string

	// DriveBaseURL overrides the Google Drive v3 API root for deterministic tests.
	// Production leaves this empty and uses Google's public endpoint.
	DriveBaseURL string
}

func (a *GoogleWorkspaceAdapter) driveBaseURL() string {
	if a.DriveBaseURL != "" {
		return strings.TrimSuffix(a.DriveBaseURL, "/")
	}
	return "https://www.googleapis.com/drive/v3"
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
	return ensureGoogleToken(context.Background(), http.DefaultClient, time.Now(), a.credentialsFile(), a.tokenFile())
}

func (a *GoogleWorkspaceAdapter) credentialsFile() string {
	if a.CredentialsFile != "" {
		return a.CredentialsFile
	}
	return filepath.Join(a.configDir(), "google_credentials.json")
}

func (a *GoogleWorkspaceAdapter) get(token *googleToken, endpoint string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusUnauthorized {
		return response, nil
	}
	response.Body.Close()
	refreshed, err := refreshGoogleToken(context.Background(), http.DefaultClient, time.Now(), a.credentialsFile(), a.tokenFile(), token)
	if err != nil {
		return nil, err
	}
	retry, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	retry.Header.Set("Authorization", fmt.Sprintf("Bearer %s", refreshed.AccessToken))
	return http.DefaultClient.Do(retry)
}

// Ingest fetches recent Google Docs and Sheets, extracting their content as Knowledge Items.
func (a *GoogleWorkspaceAdapter) Ingest(since time.Time) ([]KnowledgeItem, error) {
	token, err := a.loadToken()
	if err != nil {
		return nil, err
	}

	var items []KnowledgeItem

	// Fetch recent Google Docs
	docs, docsErr := a.listDriveFiles(token, "application/vnd.google-apps.document", since)
	if docsErr != nil {
		fmt.Printf("  ⚠️  Google Docs: %v\n", docsErr)
	} else {
		for _, doc := range docs {
			content, exportErr := a.exportFileAsText(token, doc.ID, "text/plain")
			if exportErr != nil {
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
	sheets, sheetsErr := a.listDriveFiles(token, "application/vnd.google-apps.spreadsheet", since)
	if sheetsErr != nil {
		fmt.Printf("  ⚠️  Google Sheets: %v\n", sheetsErr)
	} else {
		for _, sheet := range sheets {
			content, exportErr := a.exportFileAsText(token, sheet.ID, "text/csv")
			if exportErr != nil {
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
	if docsErr != nil && sheetsErr != nil {
		return nil, errors.Join(fmt.Errorf("Google Docs listing: %w", docsErr), fmt.Errorf("Google Sheets listing: %w", sheetsErr))
	}

	return items, nil
}

func (a *GoogleWorkspaceAdapter) listDriveFiles(token *googleToken, mimeType string, since time.Time) ([]driveFile, error) {
	sinceStr := since.Format(time.RFC3339)
	query := fmt.Sprintf("mimeType='%s' and modifiedTime>'%s' and trashed=false", mimeType, sinceStr)

	endpoint, err := url.Parse(a.driveBaseURL() + "/files")
	if err != nil {
		return nil, fmt.Errorf("construct Drive API endpoint: %w", err)
	}
	parameters := endpoint.Query()
	parameters.Set("q", query)
	parameters.Set("fields", "files(id,name,mimeType,modifiedTime,description)")
	parameters.Set("orderBy", "modifiedTime desc")
	parameters.Set("pageSize", "50")
	endpoint.RawQuery = parameters.Encode()

	resp, err := a.get(token, endpoint.String())
	if err != nil {
		return nil, fmt.Errorf("Drive API request: %w", err)
	}
	defer resp.Body.Close()

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
	endpoint, err := url.Parse(fmt.Sprintf("%s/files/%s/export", a.driveBaseURL(), url.PathEscape(fileID)))
	if err != nil {
		return "", fmt.Errorf("construct Drive export endpoint: %w", err)
	}
	parameters := endpoint.Query()
	parameters.Set("mimeType", exportMime)
	endpoint.RawQuery = parameters.Encode()

	resp, err := a.get(token, endpoint.String())
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
