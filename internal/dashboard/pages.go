package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// ── Shared Layout ───────────────────────────────────────────────────────

// pageShell wraps page-specific content in the shared dashboard layout.
// activePage is the nav item to highlight (e.g., "/", "/scan", "/guard").
func pageShell(title, activePage, bodyContent string) string {
	navItems := []struct {
		Path  string
		Glyph string
		Label string
	}{
		{"/", "☥", "Overview"},
		{"/scan", "𓁢", "Scan"},
		{"/ghosts", "𓂓", "Ghosts"},
		{"/guard", "🛡", "Guard"},
		{"/notifications", "🔔", "Notifications"},
		{"/horus", "𓂀", "Horus"},
		{"/vault", "🏛", "Vault"},
	}

	var navHTML strings.Builder
	for _, n := range navItems {
		cls := "nav-item"
		if n.Path == activePage {
			cls += " active"
		}
		navHTML.WriteString(fmt.Sprintf(
			`<a href="%s" class="%s"><span class="nav-glyph">%s</span><span class="nav-label">%s</span></a>`,
			n.Path, cls, n.Glyph, n.Label,
		))
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s — Sirsi Pantheon</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
@font-face{font-family:'Cinzel';font-style:normal;font-weight:400;font-display:swap;
src:local('Cinzel Regular'),local('Cinzel-Regular')}
body{background:%s;color:%s;font-family:Inter,-apple-system,system-ui,'Segoe UI',sans-serif;
display:flex;min-height:100vh;overflow-x:hidden}
::-webkit-scrollbar{width:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:rgba(200,169,81,.2);border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:rgba(200,169,81,.4)}

/* Sidebar */
.sidebar{width:220px;min-height:100vh;background:rgba(6,6,15,.96);border-right:1px solid %s;
padding:24px 0;display:flex;flex-direction:column;position:fixed;left:0;top:0;bottom:0;z-index:10}
.sidebar-brand{padding:0 20px 24px;border-bottom:1px solid %s}
.sidebar-brand h1{font-family:Cinzel,Georgia,'Times New Roman',serif;font-size:16px;font-weight:400;
color:%s;letter-spacing:3px;text-transform:uppercase}
.sidebar-brand p{font-size:9px;color:%s;letter-spacing:1.5px;margin-top:4px;text-transform:uppercase}
.sidebar-nav{flex:1;padding:16px 0}
.nav-item{display:flex;align-items:center;padding:10px 20px;color:%s;text-decoration:none;
font-size:13px;letter-spacing:.3px;transition:all .2s;border-left:2px solid transparent}
.nav-item:hover{background:rgba(200,169,81,.06);color:%s}
.nav-item.active{background:rgba(200,169,81,.08);color:%s;border-left-color:%s}
.nav-glyph{width:24px;font-size:15px;margin-right:10px;text-align:center}
.sidebar-footer{padding:16px 20px;border-top:1px solid %s;font-size:9px;color:#333;letter-spacing:1px}

/* Main */
.main{margin-left:220px;flex:1;padding:32px 40px;min-height:100vh}
.page-title{font-family:Cinzel,Georgia,'Times New Roman',serif;font-size:22px;font-weight:400;
color:%s;letter-spacing:2px;margin-bottom:24px}
.page-subtitle{font-size:11px;color:%s;letter-spacing:1.5px;text-transform:uppercase;margin-bottom:20px}

/* Cards */
.card{background:%s;border:1px solid %s;border-radius:12px;padding:20px 24px;margin-bottom:16px;
backdrop-filter:blur(12px);box-shadow:0 4px 20px rgba(0,0,0,.3)}
.card-title{font-size:10px;color:%s;letter-spacing:2px;text-transform:uppercase;margin-bottom:12px;font-weight:600}
.card-value{font-size:28px;font-weight:300;color:%s}
.card-label{font-size:11px;color:%s;margin-top:4px}

/* Grid */
.grid{display:grid;gap:16px}
.grid-2{grid-template-columns:repeat(2,1fr)}
.grid-3{grid-template-columns:repeat(3,1fr)}
.grid-4{grid-template-columns:repeat(4,1fr)}

/* Table */
.tbl{width:100%%;border-collapse:collapse}
.tbl th{text-align:left;font-size:10px;color:%s;letter-spacing:1.5px;text-transform:uppercase;
padding:10px 14px;border-bottom:1px solid %s;font-weight:600}
.tbl td{padding:10px 14px;font-size:13px;border-bottom:1px solid rgba(200,169,81,.06);color:%s}
.tbl tr:hover td{background:rgba(200,169,81,.03)}

/* Severity badges */
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:600;letter-spacing:.5px}
.badge-success{background:rgba(68,255,136,.12);color:%s}
.badge-error{background:rgba(255,68,68,.12);color:%s}
.badge-warning{background:rgba(255,215,0,.12);color:%s}
.badge-info{background:rgba(81,169,200,.12);color:#51A9C8}

/* Pulse animation */
@keyframes pulse{0%%,100%%{opacity:.6}50%%{opacity:1}}
.pulse{animation:pulse 2s ease-in-out infinite}

/* Search */
.search-box{background:rgba(6,6,15,.6);border:1px solid %s;border-radius:8px;padding:10px 16px;
color:%s;font-size:14px;width:100%%;outline:none;transition:border-color .2s}
.search-box:focus{border-color:%s}
.search-box::placeholder{color:#444}

/* Empty state */
.empty{text-align:center;padding:60px 20px;color:#444;font-size:13px;letter-spacing:.5px}
.empty-glyph{font-size:40px;margin-bottom:12px;opacity:.3}
</style>
</head>
<body>
<div class="sidebar">
 <div class="sidebar-brand">
  <h1>☥ Pantheon</h1>
  <p>Infrastructure Dashboard</p>
 </div>
 <nav class="sidebar-nav">%s</nav>
 <div class="sidebar-footer">LOCAL ONLY • 127.0.0.1:%d</div>
</div>
<div class="main">%s</div>
</body>
</html>`,
		title,
		ColorBg, ColorWhite,
		ColorBorder, ColorBorder,
		ColorGold, ColorDim,
		ColorDim, ColorWhite, ColorGold, ColorGold,
		ColorBorder,
		ColorGold, ColorDim,
		ColorBgPanel, ColorBorder,
		ColorGold, ColorGold, ColorDim,
		ColorGold, ColorBorder, ColorWhite,
		ColorGreen, ColorRed, ColorYellow,
		ColorBorder, ColorWhite, ColorGold,
		navHTML.String(),
		DashboardPort,
		bodyContent,
	)
}

// safeTextJS is a JavaScript helper function injected into pages that need to render
// dynamic data. It escapes HTML entities to prevent XSS when inserting into the DOM.
const safeTextJS = `function esc(s){if(!s)return'';const d=document.createElement('div');d.textContent=s;return d.innerHTML}`

// ── Overview Page ───────────────────────────────────────────────────────

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	statsJSON := "null"
	if s.cfg.StatsFn != nil {
		if data, err := s.cfg.StatsFn(); err == nil {
			statsJSON = string(data)
		}
	}

	recentJSON := "[]"
	if s.cfg.NotifyDB != nil {
		if recent, err := s.cfg.NotifyDB.Recent(8); err == nil && recent != nil {
			if b, err := json.Marshal(recent); err == nil {
				recentJSON = string(b)
			}
		}
	}

	body := fmt.Sprintf(`
<h1 class="page-title">System Overview</h1>
<div id="stats-grid" class="grid grid-4" style="margin-bottom:24px">
 <div class="card"><div class="card-title">RAM Pressure</div>
  <div class="card-value" id="ram-val">—</div>
  <div class="card-label" id="ram-label"></div></div>
 <div class="card"><div class="card-title">Git Status</div>
  <div class="card-value" id="git-val">—</div>
  <div class="card-label" id="git-label"></div></div>
 <div class="card"><div class="card-title">Active Deities</div>
  <div class="card-value" id="deity-val">0</div>
  <div class="card-label" id="deity-label">None running</div></div>
 <div class="card"><div class="card-title">Accelerator</div>
  <div class="card-value" id="accel-val">—</div>
  <div class="card-label" id="accel-label"></div></div>
</div>

<div class="grid grid-2">
 <div>
  <h2 class="page-subtitle">Recent Activity</h2>
  <div class="card" style="padding:0;overflow:hidden">
   <table class="tbl" id="recent-tbl">
    <thead><tr><th>Source</th><th>Summary</th><th>Status</th><th>Time</th></tr></thead>
    <tbody id="recent-body"></tbody>
   </table>
  </div>
 </div>
 <div>
  <h2 class="page-subtitle">Ra Deployment</h2>
  <div class="card" id="ra-card">
   <div class="empty"><div class="empty-glyph">𓇶</div>No active deployment</div>
  </div>
 </div>
</div>

<h2 class="page-subtitle" style="margin-top:24px">Live Command Output</h2>
<div class="card" id="live-log" style="max-height:240px;overflow-y:auto;font-family:monospace;font-size:12px;padding:12px 16px">
 <div style="color:#444;font-size:11px">Waiting for commands…</div>
</div>

<script>
(function(){
'use strict';
%s
const S=%s,R=%s;
const sevBadge=s=>({success:'badge-success',error:'badge-error',warning:'badge-warning'}[s]||'badge-info');
const sevIcon=s=>({success:'✅',error:'❌',warning:'⚠️',info:'ℹ️'}[s]||'ℹ️');
const ago=ts=>{if(!ts)return'—';const d=Date.now()-new Date(ts).getTime();
if(d<60e3)return Math.floor(d/1e3)+'s ago';if(d<3600e3)return Math.floor(d/6e4)+'m ago';
if(d<864e5)return Math.floor(d/36e5)+'h ago';return Math.floor(d/864e5)+'d ago'};

function renderStats(s){
 if(!s)return;
 document.getElementById('ram-val').textContent=(s.ram_icon||'')+' '+Math.round(s.ram_percent||0)+'%%';
 document.getElementById('ram-label').textContent=(s.ram_pressure||'unknown')+' pressure';
 document.getElementById('git-val').textContent=(s.osiris_icon||'')+' '+(s.uncommitted_files||0);
 document.getElementById('git-label').textContent=(s.git_branch||'')+(s.time_since_commit?' • '+s.time_since_commit+' ago':'');
 document.getElementById('deity-val').textContent=s.deity_count||0;
 document.getElementById('deity-label').textContent=(s.active_deities||[]).join(', ')||'None running';
 document.getElementById('accel-val').textContent=s.accel_icon||'💻';
 document.getElementById('accel-label').textContent=s.primary_accelerator||'Unknown';
 const rc=document.getElementById('ra-card');
 if(s.ra_deployed&&s.ra_scopes&&s.ra_scopes.length){
  rc.textContent='';
  s.ra_scopes.forEach(function(sc){
   const row=document.createElement('div');
   row.style.cssText='display:flex;align-items:center;padding:8px 0;border-bottom:1px solid rgba(200,169,81,.06)';
   const icon=document.createElement('span');icon.style.cssText='font-size:18px;margin-right:12px';icon.textContent=sc.icon;
   const name=document.createElement('span');name.style.cssText='flex:1;font-size:13px';name.textContent=sc.name;
   const state=document.createElement('span');state.style.cssText='font-size:11px;color:#888';state.textContent=sc.state;
   row.appendChild(icon);row.appendChild(name);row.appendChild(state);rc.appendChild(row)});
 }
}

function renderRecent(items){
 const tb=document.getElementById('recent-body');
 tb.textContent='';
 if(!items||!items.length){const tr=document.createElement('tr');const td=document.createElement('td');
  td.colSpan=4;td.className='empty';td.textContent='No activity yet';tr.appendChild(td);tb.appendChild(tr);return}
 items.forEach(function(n){
  const tr=document.createElement('tr');
  const tdSrc=document.createElement('td');tdSrc.style.fontWeight='600';tdSrc.textContent=n.source;
  const tdSum=document.createElement('td');const summary=(n.summary||'');
  tdSum.textContent=summary.length>80?summary.substring(0,77)+'…':summary;
  const tdSev=document.createElement('td');const badge=document.createElement('span');
  badge.className='badge '+sevBadge(n.severity);badge.textContent=sevIcon(n.severity)+' '+n.severity;tdSev.appendChild(badge);
  const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';
  tdTime.textContent=ago(n.timestamp);
  tr.appendChild(tdSrc);tr.appendChild(tdSum);tr.appendChild(tdSev);tr.appendChild(tdTime);tb.appendChild(tr)});
}

renderStats(S);renderRecent(R);
setInterval(function(){
 fetch('/api/stats').then(function(r){return r.json()}).then(renderStats).catch(function(){});
 fetch('/api/notifications?limit=8').then(function(r){return r.json()}).then(renderRecent).catch(function(){});
},10000);

/* SSE live command output */
(function(){
 var logEl=document.getElementById('live-log');
 if(!logEl||typeof EventSource==='undefined')return;
 var es=new EventSource('/api/events');
 es.addEventListener('output',function(e){
  try{var d=JSON.parse(e.data);
  var line=document.createElement('div');line.textContent=(d.handler||'')+': '+(d.line||'');
  line.style.cssText='font-size:12px;font-family:monospace;color:#ccc;padding:2px 0';
  logEl.appendChild(line);
  if(logEl.children.length>200)logEl.removeChild(logEl.firstChild);
  logEl.scrollTop=logEl.scrollHeight}catch(x){}});
 es.addEventListener('complete',function(e){
  try{var d=JSON.parse(e.data);
  var line=document.createElement('div');
  line.textContent='✓ '+d.handler+' — '+d.summary;
  line.style.cssText='font-size:12px;font-family:monospace;color:#44FF88;padding:4px 0';
  logEl.appendChild(line);logEl.scrollTop=logEl.scrollHeight}catch(x){}});
 es.addEventListener('error',function(e){
  try{var d=JSON.parse(e.data);
  var line=document.createElement('div');
  line.textContent='✗ '+d.handler+' — '+d.error;
  line.style.cssText='font-size:12px;font-family:monospace;color:#FF4444;padding:4px 0';
  logEl.appendChild(line);logEl.scrollTop=logEl.scrollHeight}catch(x){}});
})();
})();
</script>`, safeTextJS, statsJSON, recentJSON)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Overview", "/", body))
}

// ── Notifications Page ──────────────────────────────────────────────────

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	initialJSON := "[]"
	if s.cfg.NotifyDB != nil {
		if items, err := s.cfg.NotifyDB.Recent(200); err == nil && items != nil {
			if b, err := json.Marshal(items); err == nil {
				initialJSON = string(b)
			}
		}
	}

	var count int64
	if s.cfg.NotifyDB != nil {
		count, _ = s.cfg.NotifyDB.Count()
	}

	body := fmt.Sprintf(`
<h1 class="page-title">Notification History</h1>
<p class="page-subtitle">%d total notifications</p>

<div style="display:flex;gap:12px;margin-bottom:20px">
 <input type="text" class="search-box" id="filter-input" placeholder="Filter by source, summary, or severity..." style="flex:1">
 <select id="sev-filter" style="background:rgba(6,6,15,.6);border:1px solid %s;border-radius:8px;
  padding:8px 12px;color:%s;font-size:13px;outline:none">
  <option value="">All Severities</option>
  <option value="success">Success</option>
  <option value="error">Error</option>
  <option value="warning">Warning</option>
  <option value="info">Info</option>
 </select>
</div>

<div class="card" style="padding:0;overflow:hidden">
 <table class="tbl">
  <thead><tr><th>Time</th><th>Source</th><th>Action</th><th>Summary</th><th>Status</th><th>Duration</th></tr></thead>
  <tbody id="ntf-body"></tbody>
 </table>
</div>

<script>
(function(){
'use strict';
%s
const D=%s;
const sevBadge=s=>({success:'badge-success',error:'badge-error',warning:'badge-warning'}[s]||'badge-info');
const sevIcon=s=>({success:'✅',error:'❌',warning:'⚠️',info:'ℹ️'}[s]||'ℹ️');
const fmtDur=ms=>{if(!ms)return'—';if(ms<1000)return ms+'ms';if(ms<60000)return(ms/1000).toFixed(1)+'s';return(ms/60000).toFixed(1)+'m'};
const fmtTime=ts=>{if(!ts)return'—';const d=new Date(ts);return d.toLocaleDateString('en-US',{month:'short',day:'numeric'})+' '+d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit'})};

let filtered=D;
function render(){
 const tb=document.getElementById('ntf-body');
 tb.textContent='';
 if(!filtered.length){const tr=document.createElement('tr');const td=document.createElement('td');
  td.colSpan=6;td.className='empty';td.textContent='No notifications match your filter';
  tr.appendChild(td);tb.appendChild(tr);return}
 filtered.forEach(function(n){
  const tr=document.createElement('tr');
  const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';
  tdTime.textContent=fmtTime(n.timestamp);
  const tdSrc=document.createElement('td');tdSrc.style.fontWeight='600';tdSrc.textContent=n.source;
  const tdAct=document.createElement('td');tdAct.textContent=n.action;
  const tdSum=document.createElement('td');tdSum.textContent=n.summary||'—';
  const tdSev=document.createElement('td');const badge=document.createElement('span');
  badge.className='badge '+sevBadge(n.severity);badge.textContent=sevIcon(n.severity)+' '+n.severity;tdSev.appendChild(badge);
  const tdDur=document.createElement('td');tdDur.style.cssText='color:#666;font-size:11px';tdDur.textContent=fmtDur(n.duration_ms);
  tr.appendChild(tdTime);tr.appendChild(tdSrc);tr.appendChild(tdAct);tr.appendChild(tdSum);tr.appendChild(tdSev);tr.appendChild(tdDur);
  tb.appendChild(tr)});
}

function applyFilter(){
 const q=document.getElementById('filter-input').value.toLowerCase();
 const sev=document.getElementById('sev-filter').value;
 filtered=D.filter(function(n){
  if(sev&&n.severity!==sev)return false;
  if(!q)return true;
  return(n.source||'').toLowerCase().includes(q)||(n.summary||'').toLowerCase().includes(q)||
   (n.action||'').toLowerCase().includes(q)});
 render();
}

document.getElementById('filter-input').addEventListener('input',applyFilter);
document.getElementById('sev-filter').addEventListener('change',applyFilter);
render();
})();
</script>`, count, ColorBorder, ColorWhite, safeTextJS, initialJSON)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Notifications", "/notifications", body))
}

// ── Guard Page ──────────────────────────────────────────────────────────

func (s *Server) handleGuard(w http.ResponseWriter, r *http.Request) {
	body := fmt.Sprintf(`
<h1 class="page-title">🛡 Guard — Watchdog Monitor</h1>
<p class="page-subtitle">Live system resource monitoring</p>

<div class="grid grid-3" style="margin-bottom:24px">
 <div class="card">
  <div class="card-title">RAM Usage</div>
  <canvas id="ram-chart" width="320" height="100" style="width:100%%;height:100px"></canvas>
 </div>
 <div class="card">
  <div class="card-title">RAM Pressure</div>
  <div class="card-value pulse" id="ram-pct" style="font-size:48px;text-align:center;padding:12px 0">—</div>
  <div class="card-label" id="ram-state" style="text-align:center"></div>
 </div>
 <div class="card">
  <div class="card-title">Active Processes</div>
  <div class="card-value" id="deity-count" style="font-size:48px;text-align:center;padding:12px 0">0</div>
  <div class="card-label" id="deity-list" style="text-align:center;font-size:11px"></div>
 </div>
</div>

<h2 class="page-subtitle">Guard Alert History</h2>
<div class="card" style="padding:0;overflow:hidden">
 <table class="tbl">
  <thead><tr><th>Time</th><th>Summary</th><th>Status</th><th>Duration</th></tr></thead>
  <tbody id="guard-body"><tr><td colspan="4" class="empty">Loading...</td></tr></tbody>
 </table>
</div>

<script>
(function(){
'use strict';
const ramHistory=[];
const maxPoints=60;
const canvas=document.getElementById('ram-chart');
const ctx=canvas.getContext('2d');

function drawChart(){
 const w=canvas.width,h=canvas.height;
 ctx.clearRect(0,0,w,h);
 if(!ramHistory.length)return;
 ctx.strokeStyle='rgba(200,169,81,.08)';ctx.lineWidth=1;
 for(let y=0;y<=100;y+=25){const py=h-y/100*h;ctx.beginPath();ctx.moveTo(0,py);ctx.lineTo(w,py);ctx.stroke()}
 ctx.strokeStyle='%s';ctx.lineWidth=2;ctx.beginPath();
 ramHistory.forEach(function(v,i){const x=i/(maxPoints-1)*w,y=h-v/100*h;i===0?ctx.moveTo(x,y):ctx.lineTo(x,y)});
 ctx.stroke();
 const grad=ctx.createLinearGradient(0,0,0,h);
 grad.addColorStop(0,'rgba(200,169,81,.15)');grad.addColorStop(1,'transparent');
 ctx.fillStyle=grad;ctx.lineTo((ramHistory.length-1)/(maxPoints-1)*w,h);ctx.lineTo(0,h);ctx.fill();
}

const sevBadge=s=>({success:'badge-success',error:'badge-error',warning:'badge-warning'}[s]||'badge-info');
const sevIcon=s=>({success:'✅',error:'❌',warning:'⚠️',info:'ℹ️'}[s]||'ℹ️');
const fmtDur=ms=>{if(!ms)return'—';if(ms<1000)return ms+'ms';return(ms/1000).toFixed(1)+'s'};
const fmtTime=ts=>{if(!ts)return'—';const d=new Date(ts);return d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit',second:'2-digit'})};

function refresh(){
 fetch('/api/stats').then(function(r){return r.json()}).then(function(s){
  ramHistory.push(s.ram_percent||0);
  if(ramHistory.length>maxPoints)ramHistory.shift();
  drawChart();
  document.getElementById('ram-pct').textContent=Math.round(s.ram_percent||0)+'%%';
  document.getElementById('ram-state').textContent=(s.ram_pressure||'unknown')+' pressure';
  document.getElementById('deity-count').textContent=s.deity_count||0;
  document.getElementById('deity-list').textContent=(s.active_deities||[]).join(', ')||'None';
 }).catch(function(){});

 fetch('/api/notifications?source=isis&limit=20').then(function(r){return r.json()}).then(function(items){
  const tb=document.getElementById('guard-body');
  tb.textContent='';
  if(!items||!items.length){const tr=document.createElement('tr');const td=document.createElement('td');
   td.colSpan=4;td.className='empty';td.textContent='No guard alerts';tr.appendChild(td);tb.appendChild(tr);return}
  items.forEach(function(n){
   const tr=document.createElement('tr');
   const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';tdTime.textContent=fmtTime(n.timestamp);
   const tdSum=document.createElement('td');tdSum.textContent=n.summary;
   const tdSev=document.createElement('td');const badge=document.createElement('span');
   badge.className='badge '+sevBadge(n.severity);badge.textContent=sevIcon(n.severity)+' '+n.severity;tdSev.appendChild(badge);
   const tdDur=document.createElement('td');tdDur.style.cssText='color:#666;font-size:11px';tdDur.textContent=fmtDur(n.duration_ms);
   tr.appendChild(tdTime);tr.appendChild(tdSum);tr.appendChild(tdSev);tr.appendChild(tdDur);tb.appendChild(tr)});
 }).catch(function(){});
}

refresh();
setInterval(refresh,20000);
})();
</script>`, ColorGold)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Guard", "/guard", body))
}

// ── Scan Page ───────────────────────────────────────────────────────────

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	entries := s.readSteleByType(stele.TypeAnubisScan, stele.TypeAnubisJudge, stele.TypeAnubisClean)

	entriesJSON := "[]"
	if b, err := json.Marshal(entries); err == nil {
		entriesJSON = string(b)
	}

	body := fmt.Sprintf(`
<h1 class="page-title">𓁢 Scan Results</h1>
<p class="page-subtitle">Findings from Anubis scan operations</p>

<div class="card" style="padding:0;overflow:hidden">
 <table class="tbl">
  <thead><tr><th>Time</th><th>Type</th><th>Scope</th><th>Details</th></tr></thead>
  <tbody id="scan-body"></tbody>
 </table>
</div>

<script>
(function(){
'use strict';
const D=%s;
const fmtTime=ts=>{if(!ts)return'—';const d=new Date(ts);return d.toLocaleDateString('en-US',{month:'short',day:'numeric'})+' '+d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit'})};
const typeBadge=t=>({anubis_scan:'badge-info',anubis_judge:'badge-warning',anubis_clean:'badge-success'}[t]||'badge-info');
const typeLabel=t=>({anubis_scan:'Scan',anubis_judge:'Judge',anubis_clean:'Clean'}[t]||t);

const tb=document.getElementById('scan-body');
if(!D.length){
 const tr=document.createElement('tr');const td=document.createElement('td');
 td.colSpan=4;td.className='empty';td.textContent='No scan results yet. Run sirsi scan to begin.';
 tr.appendChild(td);tb.appendChild(tr);
}else{D.forEach(function(e){
 const tr=document.createElement('tr');
 const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';tdTime.textContent=fmtTime(e.ts);
 const tdType=document.createElement('td');const badge=document.createElement('span');
 badge.className='badge '+typeBadge(e.type);badge.textContent=typeLabel(e.type);tdType.appendChild(badge);
 const tdScope=document.createElement('td');tdScope.textContent=e.scope||'local';
 const tdDetails=document.createElement('td');tdDetails.style.fontSize='12px';
 tdDetails.textContent=Object.entries(e.data||{}).map(function(p){return p[0]+': '+p[1]}).join(' • ');
 tr.appendChild(tdTime);tr.appendChild(tdType);tr.appendChild(tdScope);tr.appendChild(tdDetails);tb.appendChild(tr)})}
})();
</script>`, entriesJSON)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Scan", "/scan", body))
}

// ── Ghosts Page ─────────────────────────────────────────────────────────

func (s *Server) handleGhosts(w http.ResponseWriter, r *http.Request) {
	entries := s.readSteleByType(stele.TypeKaHunt, stele.TypeKaClean)

	entriesJSON := "[]"
	if b, err := json.Marshal(entries); err == nil {
		entriesJSON = string(b)
	}

	body := fmt.Sprintf(`
<h1 class="page-title">𓂓 Ghost Detection</h1>
<p class="page-subtitle">Ka spirit hunt — dead app remnants</p>

<div class="card" style="padding:0;overflow:hidden">
 <table class="tbl">
  <thead><tr><th>Time</th><th>Type</th><th>Scope</th><th>Details</th></tr></thead>
  <tbody id="ghost-body"></tbody>
 </table>
</div>

<script>
(function(){
'use strict';
const D=%s;
const fmtTime=ts=>{if(!ts)return'—';const d=new Date(ts);return d.toLocaleDateString('en-US',{month:'short',day:'numeric'})+' '+d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit'})};
const typeBadge=t=>({ka_hunt:'badge-warning',ka_clean:'badge-success'}[t]||'badge-info');
const typeLabel=t=>({ka_hunt:'Hunt',ka_clean:'Clean'}[t]||t);

const tb=document.getElementById('ghost-body');
if(!D.length){
 const tr=document.createElement('tr');const td=document.createElement('td');
 td.colSpan=4;td.className='empty';td.textContent='No ghost hunts yet. Run sirsi ghosts to begin.';
 tr.appendChild(td);tb.appendChild(tr);
}else{D.forEach(function(e){
 const tr=document.createElement('tr');
 const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';tdTime.textContent=fmtTime(e.ts);
 const tdType=document.createElement('td');const badge=document.createElement('span');
 badge.className='badge '+typeBadge(e.type);badge.textContent=typeLabel(e.type);tdType.appendChild(badge);
 const tdScope=document.createElement('td');tdScope.textContent=e.scope||'local';
 const tdDetails=document.createElement('td');tdDetails.style.fontSize='12px';
 tdDetails.textContent=Object.entries(e.data||{}).map(function(p){return p[0]+': '+p[1]}).join(' • ');
 tr.appendChild(tdTime);tr.appendChild(tdType);tr.appendChild(tdScope);tr.appendChild(tdDetails);tb.appendChild(tr)})}
})();
</script>`, entriesJSON)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Ghosts", "/ghosts", body))
}

// ── Horus Page ──────────────────────────────────────────────────────────

func (s *Server) handleHorus(w http.ResponseWriter, r *http.Request) {
	entries := s.readSteleByType(stele.TypeHorusScan, stele.TypeHorusQuery)

	entriesJSON := "[]"
	if b, err := json.Marshal(entries); err == nil {
		entriesJSON = string(b)
	}

	body := fmt.Sprintf(`
<h1 class="page-title">𓂀 Horus — Code Graph</h1>
<p class="page-subtitle">Structural code analysis and symbol browser</p>

<div style="margin-bottom:20px">
 <input type="text" class="search-box" id="horus-search" placeholder="Search symbols, files, or packages...">
</div>

<div class="grid grid-2">
 <div>
  <h2 class="page-subtitle">Recent Analysis</h2>
  <div class="card" style="padding:0;overflow:hidden">
   <table class="tbl">
    <thead><tr><th>Time</th><th>Type</th><th>Details</th></tr></thead>
    <tbody id="horus-body"></tbody>
   </table>
  </div>
 </div>
 <div>
  <h2 class="page-subtitle">Symbol Outline</h2>
  <div class="card" id="outline-card">
   <div class="empty"><div class="empty-glyph">𓂀</div>Search for a file or run sirsi horus scan</div>
  </div>
 </div>
</div>

<script>
(function(){
'use strict';
const D=%s;
const fmtTime=ts=>{if(!ts)return'—';const d=new Date(ts);return d.toLocaleDateString('en-US',{month:'short',day:'numeric'})+' '+d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit'})};

const tb=document.getElementById('horus-body');
if(!D.length){
 const tr=document.createElement('tr');const td=document.createElement('td');
 td.colSpan=3;td.className='empty';td.textContent='No Horus analysis yet';
 tr.appendChild(td);tb.appendChild(tr);
}else{D.forEach(function(e){
 const tr=document.createElement('tr');
 const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';tdTime.textContent=fmtTime(e.ts);
 const tdType=document.createElement('td');const badge=document.createElement('span');
 badge.className='badge badge-info';badge.textContent=e.type.replace('horus_','');tdType.appendChild(badge);
 const tdDetails=document.createElement('td');tdDetails.style.fontSize='12px';
 tdDetails.textContent=Object.entries(e.data||{}).map(function(p){return p[0]+': '+p[1]}).join(' • ');
 tr.appendChild(tdTime);tr.appendChild(tdType);tr.appendChild(tdDetails);tb.appendChild(tr)})}
})();
</script>`, entriesJSON)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Horus", "/horus", body))
}

// ── Vault Page ──────────────────────────────────────────────────────────

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	entries := s.readSteleByType(stele.TypeVaultStore, stele.TypeVaultSearch, stele.TypeVaultCodeSearch)

	entriesJSON := "[]"
	if b, err := json.Marshal(entries); err == nil {
		entriesJSON = string(b)
	}

	body := fmt.Sprintf(`
<h1 class="page-title">🏛 Vault — Context Sandbox</h1>
<p class="page-subtitle">SQLite FTS5 code index and search</p>

<div style="margin-bottom:20px">
 <input type="text" class="search-box" id="vault-search" placeholder="Search indexed code and context...">
</div>

<div class="grid grid-2">
 <div>
  <h2 class="page-subtitle">Search Results</h2>
  <div class="card" id="vault-results">
   <div class="empty"><div class="empty-glyph">🏛</div>Type a query above to search the vault</div>
  </div>
 </div>
 <div>
  <h2 class="page-subtitle">Vault Activity</h2>
  <div class="card" style="padding:0;overflow:hidden">
   <table class="tbl">
    <thead><tr><th>Time</th><th>Type</th><th>Details</th></tr></thead>
    <tbody id="vault-body"></tbody>
   </table>
  </div>
 </div>
</div>

<script>
(function(){
'use strict';
const D=%s;
const fmtTime=ts=>{if(!ts)return'—';const d=new Date(ts);return d.toLocaleDateString('en-US',{month:'short',day:'numeric'})+' '+d.toLocaleTimeString('en-US',{hour:'2-digit',minute:'2-digit'})};

const tb=document.getElementById('vault-body');
if(!D.length){
 const tr=document.createElement('tr');const td=document.createElement('td');
 td.colSpan=3;td.className='empty';td.textContent='No vault activity yet';
 tr.appendChild(td);tb.appendChild(tr);
}else{D.forEach(function(e){
 const tr=document.createElement('tr');
 const tdTime=document.createElement('td');tdTime.style.cssText='color:#666;font-size:11px;white-space:nowrap';tdTime.textContent=fmtTime(e.ts);
 const tdType=document.createElement('td');const badge=document.createElement('span');
 badge.className='badge badge-info';badge.textContent=e.type.replace('vault_','');tdType.appendChild(badge);
 const tdDetails=document.createElement('td');tdDetails.style.fontSize='12px';
 tdDetails.textContent=Object.entries(e.data||{}).map(function(p){return p[0]+': '+p[1]}).join(' • ');
 tr.appendChild(tdTime);tr.appendChild(tdType);tr.appendChild(tdDetails);tb.appendChild(tr)})}
})();
</script>`, entriesJSON)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Vault", "/vault", body))
}

// ── Helpers ─────────────────────────────────────────────────────────────

// readSteleByType reads the Stele JSONL file and returns entries matching any of the given types.
// Returns newest first, up to 100 entries. Read-only — does not advance any consumer offset.
func (s *Server) readSteleByType(types ...string) []stele.Entry {
	path := s.stelePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var entries []stele.Entry
	for i := len(lines) - 1; i >= 0 && len(entries) < 100; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e stele.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if typeSet[e.Type] {
			entries = append(entries, e)
		}
	}
	return entries
}
