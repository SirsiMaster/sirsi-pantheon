package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
)

// routerSurfaceSchema is intentionally separate from the internal dashboard
// JSON shape. It is the small, stable read contract shared by the standalone
// router surface and Nexus. The producer remains RouterFn, so there is one
// authority and one failure boundary.
const routerSurfaceSchema = "router-surface.v1"

const routerSurfaceHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sirsi Router</title><style>
:root{color-scheme:dark;font:15px/1.45 "Avenir Next","Helvetica Neue",sans-serif;background:#071a16;color:#f4f7f5}
body{margin:0;padding:32px;max-width:1280px;margin-inline:auto;background:radial-gradient(circle at 90% 0,#123d31,#071a16 42%);min-height:100vh}
header{display:flex;align-items:end;justify-content:space-between;gap:20px;border-bottom:1px solid #315348;padding-bottom:18px}
h1{font:600 30px/1 Cinzel,serif;letter-spacing:.02em;margin:0}.eyebrow{color:#a9d8c5;text-transform:uppercase;font-size:12px;letter-spacing:.14em;margin:0 0 8px}
#state{color:#a9d8c5;font-size:13px;text-align:right}.error{color:#ffb4ad}.grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px;margin:24px 0}
.card{background:#102b24cc;border:1px solid #315348;border-radius:12px;padding:18px}.label{color:#9cb5aa;font-size:12px;text-transform:uppercase;letter-spacing:.1em}.value{font-size:32px;margin-top:5px;font-variant-numeric:tabular-nums}
section{margin-top:22px}h2{font-size:17px;margin:0 0 12px}.row{display:flex;justify-content:space-between;gap:20px;padding:11px 0;border-bottom:1px solid #214238}.row:last-child{border-bottom:0}.muted{color:#9cb5aa}.good{color:#7de0b2}.warn{color:#ffd48a}.bad{color:#ff9e96}
@media(max-width:760px){body{padding:20px}.grid{grid-template-columns:repeat(2,minmax(0,1fr))}header{align-items:start;flex-direction:column}#state{text-align:left}}
</style></head><body>
<header><div><p class="eyebrow">Ra · canonical router surface</p><h1>Work fabric</h1></div><div id="state">Connecting to Horus…</div></header>
<main id="app"><p class="muted">Loading the canonical snapshot…</p></main>
<script>
const esc=v=>String(v??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function render(d){const c=d.lanes?.counts||{};const lanes=d.lanes?.list||[];const attention=d.attention||[];
 document.querySelector('#state').innerHTML='<span class="good">Connected</span> · '+esc(d.generated_at||'unknown')+' · '+esc(d.version||'');
 const attentionHTML=attention.length?attention.map(a=>'<div class="row"><span class="'+(a.severity==='critical'?'bad':a.severity==='warn'?'warn':'muted')+'">'+esc(a.title)+'</span><span class="muted">'+esc(a.detail)+'</span></div>').join(''):'<p class="good">No recorded attention items.</p>';
 const laneHTML=lanes.length?lanes.map(l=>'<div class="row"><span>'+esc(l.agent)+'</span><span class="muted">'+esc(l.verdict)+' · '+(l.open||0)+' open</span></div>').join(''):'<p class="muted">No lane data returned.</p>';
 document.querySelector('#app').innerHTML='<div class="grid">'+
 '<div class="card"><div class="label">Open items</div><div class="value">'+(d.queue?.reduce((n,x)=>n+(x.open||0),0)||0)+'</div></div>'+ 
 '<div class="card"><div class="label">Active lanes</div><div class="value">'+(c.active||0)+'</div></div>'+ 
 '<div class="card"><div class="label">Consumers</div><div class="value">'+(d.consumers?.running||0)+'<span class="muted"> / '+(d.consumers?.max||0)+'</span></div></div>'+ 
 '<div class="card"><div class="label">Attention</div><div class="value">'+attention.length+'</div></div></div>'+ 
 '<section><h2>Attention</h2>'+attentionHTML+'</section>'+ 
 '<section><h2>Lanes</h2>'+laneHTML+'</section>';
}
async function refresh(){try{const r=await fetch('/api/router/v1/snapshot',{cache:'no-store'});if(!r.ok)throw new Error('HTTP '+r.status);render(await r.json())}catch(e){document.querySelector('#state').innerHTML='<span class="error">Unavailable</span>';document.querySelector('#app').innerHTML='<p class="error">The canonical router producer is unavailable. No local or stale state is shown.</p>'}}
refresh();setInterval(refresh,5000);
</script></body></html>`

func (s *Server) surfaceHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Sirsi-Router-Schema", routerSurfaceSchema)
	if origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/"); origin != "" {
		if _, ok := s.surfaceOrigins[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Vary", "Origin")
		}
	}
}

func (s *Server) handleRouterSurface(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/router" && r.URL.Path != "/router/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	_, _ = w.Write([]byte(routerSurfaceHTML))
}

func (s *Server) apiRouterSurfaceManifest(w http.ResponseWriter, r *http.Request) {
	s.surfaceHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.RouterFn == nil {
		http.Error(w, `{"error":"router producer not configured"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema":    routerSurfaceSchema,
		"authority": "ra",
		"producer":  "pantheon.horus",
		"surface":   "/router",
		"endpoints": map[string]string{"snapshot": "/api/router/v1/snapshot"},
		"integrations": map[string]string{
			"pantheon": "same Horus producer",
			"nexus":    "read-only contract consumer",
		},
	})
}

func (s *Server) apiRouterSurfaceSnapshot(w http.ResponseWriter, r *http.Request) {
	s.surfaceHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.RouterFn == nil {
		http.Error(w, `{"error":"router producer not configured"}`, http.StatusServiceUnavailable)
		return
	}
	snap, err := s.cfg.RouterFn()
	if err != nil {
		http.Error(w, `{"error":"router producer unavailable"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}
