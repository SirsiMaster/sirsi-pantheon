package seshat

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const googleDriveReadonlyScope = "https://www.googleapis.com/auth/drive.readonly"

type googleOAuthEndpoint struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	AuthURI      string   `json:"auth_uri"`
	TokenURI     string   `json:"token_uri"`
	RedirectURIs []string `json:"redirect_uris"`
}

type googleOAuthCredentials struct {
	Installed *googleOAuthEndpoint `json:"installed"`
	Web       *googleOAuthEndpoint `json:"web"`
}

type googleToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Expiry       string `json:"expiry"`
}

type GoogleAuthStatus struct {
	Configured  bool
	Authorized  bool
	Refreshable bool
	ExpiresAt   time.Time
}

func InspectGoogleWorkspaceAuth(credentialsFile, tokenFile string) (GoogleAuthStatus, error) {
	status := GoogleAuthStatus{}
	if _, err := loadGoogleOAuthEndpoint(credentialsFile); err == nil {
		status.Configured = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return status, err
	}
	token, err := readGoogleToken(tokenFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return status, nil
		}
		return status, err
	}
	status.Authorized = strings.TrimSpace(token.AccessToken) != ""
	status.Refreshable = strings.TrimSpace(token.RefreshToken) != ""
	status.ExpiresAt, _ = time.Parse(time.RFC3339Nano, token.Expiry)
	return status, nil
}

func AuthorizeGoogleWorkspace(ctx context.Context, credentialsFile, tokenFile string, openBrowser func(string) error) error {
	endpoint, err := loadGoogleOAuthEndpoint(credentialsFile)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start local OAuth callback: %w", err)
	}
	defer listener.Close()

	state, err := randomURLToken(32)
	if err != nil {
		return err
	}
	verifier, err := randomURLToken(64)
	if err != nil {
		return err
	}
	challengeBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
	redirectURI := "http://" + listener.Addr().String() + "/oauth2/callback"

	callback := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/callback", func(writer http.ResponseWriter, request *http.Request) {
		result := callbackResult{code: request.URL.Query().Get("code")}
		if request.URL.Query().Get("state") != state {
			result.err = errors.New("Google OAuth state mismatch")
		} else if providerErr := request.URL.Query().Get("error"); providerErr != "" {
			result.err = fmt.Errorf("Google authorization rejected: %s", providerErr)
		} else if result.code == "" {
			result.err = errors.New("Google authorization returned no code")
		}
		select {
		case callback <- result:
		default:
		}
		writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if result.err != nil {
			http.Error(writer, "Google authorization failed. Return to Pantheon for details.", http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("Google Workspace authorization completed. You may close this tab."))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	authURL, err := buildGoogleAuthURL(endpoint, redirectURI, state, challenge)
	if err != nil {
		return err
	}
	if err := openBrowser(authURL); err != nil {
		return fmt.Errorf("open Google authorization page: %w", err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("Google authorization timed out: %w", ctx.Err())
	case err := <-serveDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("Google OAuth callback server: %w", err)
		}
		return errors.New("Google OAuth callback server stopped before authorization")
	case result := <-callback:
		if result.err != nil {
			return result.err
		}
		token, err := exchangeGoogleCode(ctx, http.DefaultClient, endpoint, redirectURI, result.code, verifier)
		if err != nil {
			return err
		}
		return writeGoogleTokenAtomic(tokenFile, token)
	}
}

type callbackResult struct {
	code string
	err  error
}

func buildGoogleAuthURL(endpoint *googleOAuthEndpoint, redirectURI, state, challenge string) (string, error) {
	authURI := strings.TrimSpace(endpoint.AuthURI)
	if authURI == "" {
		authURI = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	parsed, err := url.Parse(authURI)
	if err != nil {
		return "", fmt.Errorf("invalid Google auth URI: %w", err)
	}
	query := parsed.Query()
	query.Set("client_id", endpoint.ClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", googleDriveReadonlyScope)
	query.Set("access_type", "offline")
	query.Set("prompt", "consent")
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("code_challenge_method", "S256")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func exchangeGoogleCode(ctx context.Context, client *http.Client, endpoint *googleOAuthEndpoint, redirectURI, code, verifier string) (*googleToken, error) {
	values := url.Values{
		"client_id":     {endpoint.ClientID},
		"client_secret": {endpoint.ClientSecret},
		"redirect_uri":  {redirectURI},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
	}
	return requestGoogleToken(ctx, client, endpoint.TokenURI, values, "authorization code exchange")
}

func ensureGoogleToken(ctx context.Context, client *http.Client, now time.Time, credentialsFile, tokenFile string) (*googleToken, error) {
	token, err := readGoogleToken(tokenFile)
	if err != nil {
		return nil, fmt.Errorf("no Google token found at %s — run 'sirsi seshat auth google': %w", tokenFile, err)
	}
	expiry, parseErr := time.Parse(time.RFC3339Nano, token.Expiry)
	if parseErr == nil && expiry.After(now.Add(2*time.Minute)) && token.AccessToken != "" {
		return token, nil
	}
	if token.AccessToken != "" && token.Expiry == "" {
		return token, nil
	}
	return refreshGoogleToken(ctx, client, now, credentialsFile, tokenFile, token)
}

func refreshGoogleToken(ctx context.Context, client *http.Client, now time.Time, credentialsFile, tokenFile string, token *googleToken) (*googleToken, error) {
	if strings.TrimSpace(token.RefreshToken) == "" {
		return nil, errors.New("Google authorization cannot be refreshed; run 'sirsi seshat auth google' once to restore consent")
	}
	endpoint, err := loadGoogleOAuthEndpoint(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("load Google OAuth credentials: %w", err)
	}
	values := url.Values{
		"client_id":     {endpoint.ClientID},
		"client_secret": {endpoint.ClientSecret},
		"refresh_token": {token.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	refreshed, err := requestGoogleToken(ctx, client, endpoint.TokenURI, values, "token refresh")
	if err != nil {
		return nil, err
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}
	if refreshed.Expiry == "" {
		refreshed.Expiry = now.Add(time.Hour).UTC().Format(time.RFC3339Nano)
	}
	if err := writeGoogleTokenAtomic(tokenFile, refreshed); err != nil {
		return nil, err
	}
	return refreshed, nil
}

func requestGoogleToken(ctx context.Context, client *http.Client, tokenURI string, values url.Values, operation string) (*googleToken, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(tokenURI) == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Google %s: %w", operation, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google %s rejected with HTTP %d; stored consent may have been revoked", operation, response.StatusCode)
	}
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode Google %s: %w", operation, err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return nil, fmt.Errorf("Google %s returned no access token", operation)
	}
	if payload.ExpiresIn <= 0 {
		payload.ExpiresIn = 3600
	}
	return &googleToken{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		TokenType:    payload.TokenType,
		Expiry:       time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second).UTC().Format(time.RFC3339Nano),
	}, nil
}

func loadGoogleOAuthEndpoint(path string) (*googleOAuthEndpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var credentials googleOAuthCredentials
	if err := json.Unmarshal(data, &credentials); err != nil {
		return nil, fmt.Errorf("invalid Google OAuth credentials: %w", err)
	}
	endpoint := credentials.Installed
	if endpoint == nil {
		endpoint = credentials.Web
	}
	if endpoint == nil || strings.TrimSpace(endpoint.ClientID) == "" || strings.TrimSpace(endpoint.TokenURI) == "" {
		return nil, errors.New("Google OAuth credentials lack a usable installed/web client")
	}
	return endpoint, nil
}

func readGoogleToken(path string) (*googleToken, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token googleToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("invalid Google token file: %w", err)
	}
	return &token, nil
}

func writeGoogleTokenAtomic(path string, token *googleToken) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".google-token-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func randomURLToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
