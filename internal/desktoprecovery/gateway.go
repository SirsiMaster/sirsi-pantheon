// Package desktoprecovery provides Pantheon's narrow browser-to-RFB recovery
// bridge. It is deliberately not a general TCP proxy: every destination is a
// configured private node, every browser session is short lived, and the
// bridge accepts only a configured signed admission; it does not trust proxy
// identity headers as operator authority.
package desktoprecovery

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
	"golang.org/x/sys/unix"
)

const (
	cookieName        = "__Host-pantheon-recovery"
	defaultSessionTTL = 10 * time.Minute
	maxSessionTTL     = 30 * time.Minute
	maxCapabilityTTL  = 30 * time.Minute
)

//go:embed novnc
var noVNC embed.FS

// Node is one explicitly approved Screen Sharing target. Address must be a
// literal RFC1918 or Tailscale IP on port 5900; hostnames and public addresses
// are rejected so browser input can never turn this service into a proxy.
type Node struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Origins []string `json:"origins"`
}

// Principal identifies the already-authenticated operator. The admission
// identity and expiry are verified authority, not request-text conveniences:
// the gateway uses them to enforce one use through the signed expiry.
type Principal struct {
	Login              string
	admissionID        [sha256.Size]byte
	admissionExpiresAt time.Time
}

var ErrAdmissionAlreadyUsed = errors.New("desktop recovery admission already used")

// AdmissionStore makes a verified admission single-use through its signed
// expiry. Production requires durable storage so a bridge restart cannot make
// an accepted signed admission usable again.
type AdmissionStore interface {
	Claim([sha256.Size]byte, time.Time) error
}

// MemoryAdmissionStore is deliberately test-only: it does not survive a
// process restart and must never be used by recovery serve.
type MemoryAdmissionStore struct {
	mu      sync.Mutex
	claimed map[[sha256.Size]byte]struct{}
}

func NewMemoryAdmissionStore() *MemoryAdmissionStore {
	return &MemoryAdmissionStore{claimed: make(map[[sha256.Size]byte]struct{})}
}

func (s *MemoryAdmissionStore) Claim(id [sha256.Size]byte, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.claimed[id]; exists {
		return ErrAdmissionAlreadyUsed
	}
	s.claimed[id] = struct{}{}
	return nil
}

// FileAdmissionStore uses a retained, no-follow directory descriptor and a
// create-only leaf per signed admission. It intentionally never prunes claim
// leaves: an expired capability is rejected before this store is called, while
// retaining a claim is safer than making a previously accepted admission live
// after a crash or cleanup ambiguity.
type FileAdmissionStore struct{ rootFD int }

func NewFileAdmissionStore(root string) (*FileAdmissionStore, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("desktop recovery: admission claim directory must be absolute")
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("desktop recovery: open admission claim directory: %w", err)
	}
	if err := validateClaimDirectory(fd, unix.Fstat, unix.Geteuid()); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &FileAdmissionStore{rootFD: fd}, nil
}

// Check the retained descriptor rather than re-resolving a mutable path. The
// injectable stat operation also lets tests prove failure closes admission.
func validateClaimDirectory(fd int, statFn func(int, *unix.Stat_t) error, effectiveUID int) error {
	var stat unix.Stat_t
	if err := statFn(fd, &stat); err != nil {
		return fmt.Errorf("desktop recovery: stat admission claim directory: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("desktop recovery: admission claim path is not a directory")
	}
	if stat.Uid != uint32(effectiveUID) || stat.Mode&0o7777 != 0o700 {
		return errors.New("desktop recovery: admission claim directory must be owned by the effective operator and have mode 0700")
	}
	return nil
}

func (s *FileAdmissionStore) Claim(id [sha256.Size]byte, expiresAt time.Time) error {
	if s == nil || s.rootFD < 0 {
		return errors.New("desktop recovery: admission claim store is unavailable")
	}
	if err := validateClaimDirectory(s.rootFD, unix.Fstat, unix.Geteuid()); err != nil {
		return err
	}
	name := hex.EncodeToString(id[:]) + ".claim"
	fd, err := unix.Openat(s.rootFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return ErrAdmissionAlreadyUsed
		}
		return fmt.Errorf("desktop recovery: create admission claim: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = unix.Close(fd)
		}
	}()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		if err != nil {
			return fmt.Errorf("desktop recovery: stat admission claim: %w", err)
		}
		return errors.New("desktop recovery: admission claim identity is invalid")
	}
	content := []byte(fmt.Sprintf("pantheon-recovery-claim-v1\nexpires_at_unix=%d\n", expiresAt.UTC().Unix()))
	for written := 0; written < len(content); {
		n, writeErr := unix.Write(fd, content[written:])
		if writeErr != nil {
			return fmt.Errorf("desktop recovery: write admission claim: %w", writeErr)
		}
		if n <= 0 {
			return errors.New("desktop recovery: short admission claim write")
		}
		written += n
	}
	if err := unix.Fsync(fd); err != nil {
		return fmt.Errorf("desktop recovery: fsync admission claim: %w", err)
	}
	if err := unix.Close(fd); err != nil {
		return fmt.Errorf("desktop recovery: close admission claim: %w", err)
	}
	closed = true
	if err := unix.Fsync(s.rootFD); err != nil {
		return fmt.Errorf("desktop recovery: fsync admission claim directory: %w", err)
	}
	return nil
}

func (s *FileAdmissionStore) Close() error {
	if s == nil || s.rootFD < 0 {
		return nil
	}
	err := unix.Close(s.rootFD)
	s.rootFD = -1
	return err
}

// Authorizer is the Pantheon admission seam. Production uses a signed
// capability; tests inject a deterministic authorizer. There is intentionally
// no anonymous, static-token, or identity-header fallback.
type Authorizer interface {
	AuthorizeRecovery(context.Context, *http.Request, Node) (Principal, error)
}

// CapabilityClaims is a signed, short-lived, single-node operator admission.
// Its opaque compact encoding is allowed only in a POST body or Authorization
// header: it is never a route, query, or cookie value.
type CapabilityClaims struct {
	KeyID     string `json:"key_id"`
	Login     string `json:"login"`
	NodeID    string `json:"node_id"`
	Origin    string `json:"origin"`
	Purpose   string `json:"purpose"`
	ExpiresAt int64  `json:"expires_at_unix"`
	Nonce     string `json:"nonce"`
}

// CapabilityAuthorizer stores public keys only. An ordinary same-host process
// cannot turn a forged Tailscale identity header into an allowlisted operator.
type CapabilityAuthorizer struct {
	PublicKeys map[string]ed25519.PublicKey
	Now        func() time.Time
}

func (a CapabilityAuthorizer) AuthorizeRecovery(_ context.Context, r *http.Request, node Node) (Principal, error) {
	if a.Now == nil {
		a.Now = time.Now
	}
	claims, err := a.claims(r)
	if err != nil {
		return Principal{}, err
	}
	now := a.Now().UTC()
	expiresAt := time.Unix(claims.ExpiresAt, 0).UTC()
	if claims.Purpose != "pantheon.desktop-recovery" || claims.NodeID != node.ID || claims.Origin != r.Header.Get("Origin") || !now.Before(expiresAt) || expiresAt.After(now.Add(maxCapabilityTTL)) {
		return Principal{}, errors.New("desktop recovery capability is not valid for this admission")
	}
	return Principal{Login: claims.Login, admissionID: sha256.Sum256([]byte(claims.KeyID + "\x00" + claims.Nonce)), admissionExpiresAt: expiresAt}, nil
}

func (a CapabilityAuthorizer) claims(r *http.Request) (CapabilityClaims, error) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return CapabilityClaims{}, errors.New("desktop recovery requires one signed admission capability")
	}
	compact := strings.TrimPrefix(values[0], "Bearer ")
	parts := strings.Split(compact, ".")
	if len(parts) != 2 || len(compact) > 4096 {
		return CapabilityClaims{}, errors.New("desktop recovery capability is malformed")
	}
	payload, err := canonicalBase64URL(parts[0])
	if err != nil || len(payload) == 0 || len(payload) > 2048 {
		return CapabilityClaims{}, errors.New("desktop recovery capability payload is malformed")
	}
	signature, err := canonicalBase64URL(parts[1])
	if err != nil || len(signature) != ed25519.SignatureSize {
		return CapabilityClaims{}, errors.New("desktop recovery capability signature is malformed")
	}
	var claims CapabilityClaims
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&claims); err != nil || claims.KeyID == "" || claims.Login == "" || claims.NodeID == "" || claims.Origin == "" || claims.Nonce == "" || claims.ExpiresAt <= 0 {
		return CapabilityClaims{}, errors.New("desktop recovery capability claims are malformed")
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return CapabilityClaims{}, errors.New("desktop recovery capability has trailing JSON")
	}
	key, ok := a.PublicKeys[claims.KeyID]
	if !ok || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, payload, signature) {
		return CapabilityClaims{}, errors.New("desktop recovery capability signature is not accepted")
	}
	return claims, nil
}

func canonicalBase64URL(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("desktop recovery base64url value is not canonical")
	}
	return decoded, nil
}

// Config supplies the sole source of node and operator authority. A caller
// cannot add destinations through a request or query string.
type Config struct {
	Nodes       []Node
	Authorizer  Authorizer
	Claims      AdmissionStore
	SessionTTL  time.Duration
	Now         func() time.Time
	Rand        func([]byte) (int, error)
	DialContext func(context.Context, string, string) (net.Conn, error)
}

type session struct {
	nodeID    string
	principal string
	expiresAt time.Time
	active    bool
	cancel    context.CancelFunc
}

// Gateway serves an embedded, pinned noVNC client and a narrow authenticated
// WebSocket-to-RFB bridge. It does not execute desktop payloads or invoke a
// shell; it only carries the existing Screen Sharing protocol after admission.
type Gateway struct {
	nodes  map[string]Node
	auth   Authorizer
	ttl    time.Duration
	now    func() time.Time
	rand   func([]byte) (int, error)
	dial   func(context.Context, string, string) (net.Conn, error)
	claims AdmissionStore

	mu       sync.Mutex
	sessions map[[sha256.Size]byte]session
}

func New(cfg Config) (*Gateway, error) {
	if cfg.Authorizer == nil {
		return nil, errors.New("desktop recovery: authorizer is required")
	}
	if cfg.Claims == nil {
		return nil, errors.New("desktop recovery: durable admission claim store is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Read
	}
	if cfg.DialContext == nil {
		d := &net.Dialer{Timeout: 5 * time.Second}
		cfg.DialContext = d.DialContext
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.SessionTTL <= 0 || cfg.SessionTTL > maxSessionTTL {
		return nil, fmt.Errorf("desktop recovery: session TTL must be between 1ns and %s", maxSessionTTL)
	}
	nodes := make(map[string]Node, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		if err := validateNode(node); err != nil {
			return nil, err
		}
		if _, exists := nodes[node.ID]; exists {
			return nil, fmt.Errorf("desktop recovery: duplicate node %q", node.ID)
		}
		node.Origins = canonicalOrigins(node.Origins)
		nodes[node.ID] = node
	}
	if len(nodes) == 0 {
		return nil, errors.New("desktop recovery: at least one approved node is required")
	}
	return &Gateway{nodes: nodes, auth: cfg.Authorizer, claims: cfg.Claims, ttl: cfg.SessionTTL, now: cfg.Now, rand: cfg.Rand, dial: cfg.DialContext, sessions: make(map[[sha256.Size]byte]session)}, nil
}

func validateNode(node Node) error {
	if !validNodeID(node.ID) {
		return fmt.Errorf("desktop recovery: invalid node id %q", node.ID)
	}
	host, port, err := net.SplitHostPort(node.Address)
	if err != nil || port != "5900" {
		return fmt.Errorf("desktop recovery: node %q must use a literal private RFB address on port 5900", node.ID)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !isPrivateRFBAddress(ip) {
		return fmt.Errorf("desktop recovery: node %q address must be private or Tailscale", node.ID)
	}
	if len(node.Origins) == 0 {
		return fmt.Errorf("desktop recovery: node %q requires an HTTPS origin allowlist", node.ID)
	}
	for _, origin := range node.Origins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("desktop recovery: node %q has invalid HTTPS origin %q", node.ID, origin)
		}
	}
	return nil
}

func validNodeID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := range id {
		c := id[i]
		alphanumeric := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if !alphanumeric && c != '-' {
			return false
		}
		if (i == 0 || i == len(id)-1) && !alphanumeric {
			return false
		}
	}
	return true
}

func isPrivateRFBAddress(ip netip.Addr) bool {
	if ip.Is4() && ip.As4()[0] == 100 && ip.As4()[1]&0xc0 == 0x40 { // 100.64.0.0/10 Tailscale CGNAT range
		return true
	}
	return ip.IsPrivate()
}

func canonicalOrigins(origins []string) []string {
	set := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		set[origin] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for origin := range set {
		out = append(out, origin)
	}
	sort.Strings(out)
	return out
}

// Handler exposes only noVNC assets plus the bounded recovery API.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/recovery/novnc/", http.StripPrefix("/recovery/novnc/", http.FileServerFS(mustSub(noVNC, "novnc"))))
	mux.HandleFunc("/recovery/v1/nodes/", g.handleNode)
	return mux
}

func mustSub(root fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func (g *Gateway) handleNode(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/recovery/v1/nodes/")
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	node, ok := g.nodes[parts[0]]
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "sessions":
		g.admit(w, r, node)
	case "entry":
		g.entry(w, r, node)
	case "client":
		g.client(w, r, node)
	case "ws":
		g.websocket(node).ServeHTTP(w, r)
	case "disconnect":
		g.disconnect(w, r, node)
	default:
		http.NotFound(w, r)
	}
}

// Close releases all bridge sessions, including hijacked WebSockets which an
// HTTP server shutdown does not close itself.
func (g *Gateway) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for hash, s := range g.sessions {
		if s.cancel != nil {
			s.cancel()
		}
		delete(g.sessions, hash)
	}
}

func (g *Gateway) disconnect(w http.ResponseWriter, r *http.Request, node Node) {
	if r.Method != http.MethodPost || !originAllowed(r.Header.Get("Origin"), node) || r.URL.RawQuery != "" {
		http.Error(w, "same-origin POST required", http.StatusForbidden)
		return
	}
	hash, ok := sessionHash(r)
	g.mu.Lock()
	s, exists := g.sessions[hash]
	if !ok || !exists || s.nodeID != node.ID || !g.now().Before(s.expiresAt) {
		g.mu.Unlock()
		http.Error(w, "active recovery admission required", http.StatusUnauthorized)
		return
	}
	delete(g.sessions, hash)
	if s.cancel != nil {
		s.cancel()
	}
	g.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (g *Gateway) admit(w http.ResponseWriter, r *http.Request, node Node) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !originAllowed(r.Header.Get("Origin"), node) {
		g.admissionError(w, r, node, http.StatusForbidden, "Open recovery from the approved Pantheon address for this Mac.")
		return
	}
	admission, err := admissionRequest(r)
	if err != nil {
		g.admissionError(w, r, node, http.StatusUnauthorized, "This admission is incomplete. Request a fresh recovery admission, then try again.")
		return
	}
	principal, err := g.auth.AuthorizeRecovery(r.Context(), admission, node)
	if err != nil {
		g.admissionError(w, r, node, http.StatusUnauthorized, "This admission was not accepted. Request a fresh recovery admission and try again.")
		return
	}
	raw := make([]byte, 32)
	if n, err := g.rand(raw); err != nil || n != len(raw) {
		http.Error(w, "recovery session unavailable", http.StatusServiceUnavailable)
		return
	}
	now := g.now().UTC()
	if principal.admissionExpiresAt.IsZero() || !now.Before(principal.admissionExpiresAt) {
		g.admissionError(w, r, node, http.StatusUnauthorized, "This admission has expired. Request a fresh recovery admission and try again.")
		return
	}
	expires := now.Add(g.ttl)
	if expires.After(principal.admissionExpiresAt) {
		expires = principal.admissionExpiresAt
	}
	if err := g.claims.Claim(principal.admissionID, principal.admissionExpiresAt); err != nil {
		if errors.Is(err, ErrAdmissionAlreadyUsed) {
			http.Error(w, "recovery admission already used", http.StatusConflict)
			return
		}
		http.Error(w, "recovery admission unavailable", http.StatusServiceUnavailable)
		return
	}
	hash := sha256.Sum256(raw)
	g.mu.Lock()
	g.gcLocked(now)
	g.sessions[hash] = session{nodeID: node.ID, principal: principal.Login, expiresAt: expires}
	g.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: hex.EncodeToString(raw), Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, Expires: expires})
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Redirect(w, r, "/recovery/v1/nodes/"+node.ID+"/client", http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"node": node.ID, "expires_at": expires.Format(time.RFC3339), "client_path": "/recovery/v1/nodes/" + node.ID + "/client"})
}

func (g *Gateway) entry(w http.ResponseWriter, r *http.Request, node Node) {
	if r.Method != http.MethodGet || r.URL.RawQuery != "" {
		http.Error(w, "recovery entry requires GET without query", http.StatusMethodNotAllowed)
		return
	}
	g.renderEntry(w, node, http.StatusOK, "")
}

func (g *Gateway) admissionError(w http.ResponseWriter, r *http.Request, node Node, status int, message string) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") || strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		g.renderEntry(w, node, status, message)
		return
	}
	http.Error(w, "recovery admission denied", status)
}

func (g *Gateway) renderEntry(w http.ResponseWriter, node Node, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	state := ""
	if message != "" {
		state = `<div class="notice" role="alert"><strong>Recovery needs a fresh admission.</strong><span>` + message + `</span></div>`
	}
	_, _ = fmt.Fprintf(w, entryPage, node.ID, state, node.ID)
}

const entryPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="color-scheme" content="dark"><title>Pantheon recovery</title><style>
:root{color-scheme:dark;--bg:#111513;--panel:#1a201c;--line:#36443a;--ink:#f2f4ee;--muted:#b9c1b9;--green:#67c783;--gold:#d7b757;--danger:#ff8585}*{box-sizing:border-box}body{margin:0;min-width:320px;background:var(--bg);color:var(--ink);font:16px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}main{width:min(100%%,42rem);min-height:100svh;margin:auto;padding:clamp(2rem,8vw,5rem)1.25rem;display:grid;align-content:center;gap:2rem}.brand{font-weight:700;color:var(--gold);letter-spacing:.01em}.brand span{color:var(--green)}h1{max-width:16ch;margin:0;font-size:clamp(2rem,7vw,3.25rem);line-height:1.08;letter-spacing:-.035em}p{max-width:62ch;margin:0;color:var(--muted)}form,.notice{border:1px solid var(--line);border-radius:14px;background:var(--panel);padding:1.25rem}.notice{display:grid;gap:.25rem;border-color:#7e4545;color:#ffe6e6}.notice strong{color:var(--danger)}label{display:grid;gap:.5rem;font-weight:650}input{width:100%%;min-height:3rem;border:1px solid #607064;border-radius:8px;background:#0c100e;color:var(--ink);padding:.6rem .75rem;font:inherit}input:focus-visible,button:focus-visible{outline:3px solid var(--gold);outline-offset:3px}button{width:100%%;min-height:3rem;margin-top:1rem;border:0;border-radius:8px;background:var(--green);color:#092311;font:700 1rem/1 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;cursor:pointer;transition:filter 180ms ease}button:hover{filter:brightness(1.08)}button:active{filter:brightness(.92)}.help{display:grid;gap:.65rem;font-size:.9375rem}.help strong{color:var(--ink)}.help ul{margin:0;padding-left:1.25rem;color:var(--muted)}.privacy{font-size:.8125rem;color:var(--muted)}::selection{background:var(--gold);color:#1f1904}@media (prefers-reduced-motion:reduce){button{transition:none}}</style></head>
<body><main><div class="brand">Sirsi <span>Pantheon</span></div><div><h1>Open approved desktop recovery</h1><p>Use a short-lived recovery admission for <strong>%s</strong>. It can be used once, is never added to a URL, and is not retained by Pantheon.</p></div>%s<form method="post" action="/recovery/v1/nodes/%s/sessions"><label for="capability">Recovery admission</label><input id="capability" name="capability" type="password" autocomplete="off" autocapitalize="off" spellcheck="false" required aria-describedby="admission-help"><button type="submit">Continue to desktop</button><p id="admission-help" class="privacy">After admission, Apple Screen Sharing may ask for that Mac’s credentials. Those credentials stay in the browser-to-Mac connection; Pantheon does not store or log them.</p></form><section class="help" aria-label="Recovery help"><strong>Need a resolution?</strong><ul><li>Request a fresh recovery admission from your Pantheon operator.</li><li>Open this page from the approved Pantheon address for this Mac.</li><li>Use the newest admission once; a used or expired admission cannot be reused.</li></ul></section></main></body></html>`

func (g *Gateway) client(w http.ResponseWriter, r *http.Request, node Node) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if !g.validSession(r, node.ID, false) {
		http.Redirect(w, r, "/recovery/v1/nodes/"+node.ID+"/entry", http.StatusFound)
		return
	}
	q := url.Values{"autoconnect": {"true"}, "reconnect": {"false"}, "view_only": {"true"}, "path": {"recovery/v1/nodes/" + node.ID + "/ws"}}
	http.Redirect(w, r, "/recovery/novnc/vnc.html?"+q.Encode(), http.StatusFound)
}

func admissionRequest(r *http.Request) (*http.Request, error) {
	if r.URL.RawQuery != "" {
		return nil, errors.New("recovery admission capability must not be in a URL")
	}
	if len(r.Header.Values("Authorization")) != 0 {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") || r.ContentLength > 0 {
			return nil, errors.New("recovery admission must use exactly one credential transport")
		}
		return r, nil
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		return nil, errors.New("recovery admission capability is required")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("recovery admission form is invalid")
	}
	form, err := url.ParseQuery(string(raw))
	if err != nil || len(form) != 1 || len(form["capability"]) != 1 || form.Get("capability") == "" {
		return nil, errors.New("recovery admission form is invalid")
	}
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+form.Get("capability"))
	return clone, nil
}

func (g *Gateway) websocket(node Node) websocket.Server {
	return websocket.Server{
		Handshake: func(_ *websocket.Config, r *http.Request) error {
			if !originAllowed(r.Header.Get("Origin"), node) {
				return errors.New("recovery origin denied")
			}
			if !g.validSession(r, node.ID, true) {
				return errors.New("active recovery admission required")
			}
			return nil
		},
		Handler: func(ws *websocket.Conn) { g.proxy(node, ws) },
	}
}

func (g *Gateway) proxy(node Node, ws *websocket.Conn) {
	// RFB is an opaque byte stream; noVNC requires binary WebSocket frames.
	ws.PayloadType = websocket.BinaryFrame
	defer ws.Close()
	request := ws.Request()
	hash, ok := sessionHash(request)
	if !ok {
		return
	}
	now := g.now().UTC()
	g.mu.Lock()
	g.gcLocked(now)
	s, exists := g.sessions[hash]
	if !exists || s.nodeID != node.ID || s.active || !now.Before(s.expiresAt) {
		g.mu.Unlock()
		return
	}
	ctx, cancel := context.WithDeadline(request.Context(), s.expiresAt)
	defer cancel()
	s.active = true
	s.cancel = cancel
	g.sessions[hash] = s
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.sessions, hash)
		g.mu.Unlock()
	}()

	rfb, err := g.dial(ctx, "tcp", node.Address)
	if err != nil {
		return
	}
	defer rfb.Close()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(rfb, ws); done <- struct{}{} }()
	go func() { _, _ = io.Copy(ws, rfb); done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (g *Gateway) validSession(r *http.Request, nodeID string, requireIdle bool) bool {
	hash, ok := sessionHash(r)
	if !ok {
		return false
	}
	now := g.now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gcLocked(now)
	s, exists := g.sessions[hash]
	return exists && s.nodeID == nodeID && now.Before(s.expiresAt) && (!requireIdle || !s.active)
}

func sessionHash(r *http.Request) ([sha256.Size]byte, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || len(cookie.Value) != 64 {
		return [sha256.Size]byte{}, false
	}
	raw, err := hex.DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		return [sha256.Size]byte{}, false
	}
	return sha256.Sum256(raw), true
}

func (g *Gateway) gcLocked(now time.Time) {
	for key, s := range g.sessions {
		if !now.Before(s.expiresAt) {
			delete(g.sessions, key)
		}
	}
}

func originAllowed(origin string, node Node) bool {
	for _, allowed := range node.Origins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
