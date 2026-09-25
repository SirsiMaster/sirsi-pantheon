package mailops

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"

	"golang.org/x/oauth2"
)

// RunLocalConsentFlow opens the system browser to the Google consent
// screen and captures the OAuth redirect on a local loopback listener —
// the same shape as the Python prototype's InstalledAppFlow.run_local_server.
// This is the exchange function Authorize expects.
func RunLocalConsentFlow(ctx context.Context) func(*oauth2.Config) (*oauth2.Token, error) {
	return func(cfg *oauth2.Config) (*oauth2.Token, error) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("mailops: open local listener: %w", err)
		}
		defer ln.Close()
		cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c := r.URL.Query().Get("code"); c != "" {
				fmt.Fprint(w, "Sirsi mail ops: access granted. You can close this tab.")
				codeCh <- c
				return
			}
			errCh <- fmt.Errorf("mailops: consent denied or missing code: %s", r.URL.Query().Get("error"))
		})}
		go func() {
			if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
				errCh <- fmt.Errorf("mailops: local consent server: %w", serveErr)
			}
		}()
		defer srv.Close()

		url := cfg.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
		fmt.Printf("Opening browser for consent. If it doesn't open, visit:\n%s\n", url)
		_ = exec.Command("open", url).Start() // macOS (ADR-032); best-effort, URL is printed regardless

		select {
		case code := <-codeCh:
			return cfg.Exchange(ctx, code)
		case err := <-errCh:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}
