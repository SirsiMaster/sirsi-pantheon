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
		Key   string
		Glyph string
		Label string
	}{
		{"home", "☥", "Home"},
		{"scan", "𓁢", "Scan"},
		{"ghosts", "𓂓", "Ghosts"},
		{"guard", "🛡", "Guard"},
		{"notifications", "🔔", "Notifications"},
		{"horus", "𓂀", "Horus"},
		{"vault", "🏛", "Vault"},
		{"ra", "𓇶", "Ra"},
	}

	var navHTML strings.Builder
	for _, n := range navItems {
		cls := "nav-item"
		if n.Key == activePage {
			cls += " active"
		}
		navHTML.WriteString(fmt.Sprintf(
			`<a href="#" class="%s" data-view="%s" onclick="switchView('%s');return false"><span class="nav-glyph">%s</span><span class="nav-label">%s</span></a>`,
			cls, n.Key, n.Key, n.Glyph, n.Label,
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
body{background:%s;color:%s;font-family:'SF Mono',Menlo,Consolas,'Courier New',monospace;
display:flex;min-height:100vh;overflow:hidden}
::-webkit-scrollbar{width:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:rgba(200,169,81,.2);border-radius:3px}

/* Sidebar */
.sidebar{width:180px;min-height:100vh;background:rgba(6,6,15,.96);border-right:1px solid %s;
display:flex;flex-direction:column;position:fixed;left:0;top:0;bottom:0;z-index:10}
.sidebar-brand{padding:16px 16px 12px;border-bottom:1px solid %s}
.sidebar-brand h1{font-family:Inter,-apple-system,system-ui,sans-serif;font-size:13px;font-weight:600;
color:%s;letter-spacing:2px;text-transform:uppercase}
.sidebar-nav{flex:1;padding:8px 0}
.nav-item{display:flex;align-items:center;padding:8px 16px;color:%s;text-decoration:none;
font-size:12px;letter-spacing:.3px;transition:all .15s;border-left:2px solid transparent;cursor:pointer;
font-family:Inter,-apple-system,system-ui,sans-serif}
.nav-item:hover{background:rgba(200,169,81,.06);color:%s}
.nav-item.active{background:rgba(200,169,81,.08);color:%s;border-left-color:%s}
.nav-glyph{width:20px;font-size:14px;margin-right:8px;text-align:center}
.sidebar-footer{padding:12px 16px;border-top:1px solid %s;font-size:8px;color:#333;letter-spacing:1px;
font-family:Inter,-apple-system,system-ui,sans-serif}

/* Main */
.main{margin-left:180px;flex:1;display:flex;flex-direction:column;height:100vh;overflow:hidden}

/* Stats bar */
.stats-bar{display:flex;gap:1px;background:rgba(200,169,81,.06);border-bottom:1px solid %s;flex-shrink:0}
.stat{flex:1;padding:12px 16px;background:%s}
.stat-label{font-size:9px;color:%s;letter-spacing:1.5px;text-transform:uppercase;
font-family:Inter,-apple-system,system-ui,sans-serif;margin-bottom:4px}
.stat-value{font-size:16px;color:%s;font-weight:400}
.stat-sub{font-size:10px;color:#555;margin-top:2px}

/* Terminal */
.terminal-wrap{flex:1;display:flex;flex-direction:column;overflow:hidden}
.term-input-bar{display:flex;align-items:center;padding:0;border-bottom:1px solid %s;background:rgba(3,3,8,.9);flex-shrink:0}
.term-prompt{color:%s;padding:8px 0 8px 16px;font-size:13px;flex-shrink:0}
.term-input{flex:1;background:none;border:none;color:%s;font-size:13px;padding:8px 16px 8px 8px;
font-family:inherit;outline:none}
.term-input::placeholder{color:#444}
.term-view-label{color:#555;font-size:10px;padding-right:16px;letter-spacing:1px;text-transform:uppercase;
font-family:Inter,-apple-system,system-ui,sans-serif;flex-shrink:0}
.terminal{flex:1;overflow-y:auto;padding:12px 16px;background:rgba(3,3,8,.95);line-height:1.6;font-size:12px}
.t-line{margin:0;white-space:pre-wrap;word-break:break-all}
.t-dim{color:#555}
.t-out{color:#ccc}
.t-ok{color:#44FF88}
.t-err{color:#FF4444}
.t-gold{color:#C8A951}
.t-head{color:#C8A951;font-weight:600;font-size:13px;margin-top:8px}
.t-row{display:flex;gap:16px;padding:2px 0}
.t-row:hover{background:rgba(200,169,81,.03)}
.t-col{color:#aaa}.t-col-r{color:#C8A951;text-align:right;min-width:80px}
.t-action{color:#555;cursor:pointer;transition:color .15s;text-decoration:underline;text-decoration-color:#333}
.t-action:hover{color:#C8A951;text-decoration-color:#C8A951}
.t-sep{border-top:1px solid rgba(200,169,81,.06);margin:6px 0}
</style>
</head>
<body>
<div class="sidebar">
 <div class="sidebar-brand"><h1>☥ Pantheon</h1></div>
 <nav class="sidebar-nav">%s</nav>
 <div class="sidebar-footer">LOCAL • 127.0.0.1:%d</div>
</div>
<div class="main">%s</div>
</body>
</html>`,
		title,
		ColorBg, ColorWhite,
		ColorBorder, ColorBorder,
		ColorGold,
		ColorDim, ColorWhite, ColorGold, ColorGold,
		ColorBorder,
		ColorBorder, ColorBg,
		ColorGold, ColorGold,
		ColorBorder, ColorGold, ColorWhite,
		navHTML.String(),
		DashboardPort,
		bodyContent,
	)
}

// safeTextJS is a JavaScript helper function injected into pages that need to render
// dynamic data. It escapes HTML entities to prevent XSS when inserting into the DOM.
const safeTextJS = `function esc(s){if(!s)return'';const d=document.createElement('div');d.textContent=s;return d.innerHTML}`

// ── SPA Entry Point ───────────────────────────────────────────────────

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	body := `

<!-- Stats bar -->
<div class="stats-bar">
 <div class="stat"><div class="stat-label">RAM</div>
  <div class="stat-value" id="ram-val">—</div>
  <div class="stat-sub" id="ram-label"></div></div>
 <div class="stat"><div class="stat-label">Git</div>
  <div class="stat-value" id="git-val">—</div>
  <div class="stat-sub" id="git-label"></div></div>
 <div class="stat"><div class="stat-label">Deities</div>
  <div class="stat-value" id="deity-val">0</div>
  <div class="stat-sub" id="deity-label"></div></div>
 <div class="stat"><div class="stat-label">Platform</div>
  <div class="stat-value" id="accel-val">—</div>
  <div class="stat-sub" id="accel-label"></div></div>
</div>

<!-- Terminal -->
<div class="terminal-wrap">
 <div class="term-input-bar">
  <span class="term-prompt">𓉴 </span>
  <input type="text" class="term-input" id="term-input" placeholder="Type a command... (scan, ghosts, doctor, guard, network, hardware)" autocomplete="off">
  <span class="term-view-label" id="view-label">home</span>
 </div>
 <div class="terminal" id="terminal">
  <div class="t-line t-dim">☥ Pantheon — use sidebar or type a command</div>
 </div>
</div>

<script>
(function(){
'use strict';
const T=document.getElementById('terminal');
const fmtSize=b=>{if(b>=1073741824)return(b/1073741824).toFixed(1)+' GB';
 if(b>=1048576)return(b/1048576).toFixed(1)+' MB';if(b>=1024)return(b/1024).toFixed(1)+' KB';return b+' B'};
const ago=ts=>{if(!ts)return'';const d=Date.now()-new Date(ts).getTime();
 if(d<60e3)return Math.floor(d/1e3)+'s ago';if(d<3600e3)return Math.floor(d/6e4)+'m ago';
 if(d<864e5)return Math.floor(d/36e5)+'h ago';return Math.floor(d/864e5)+'d ago'};
let currentView='home';let running=false;

function out(text,cls){const d=document.createElement('div');d.className='t-line '+(cls||'t-out');
 d.textContent=text;T.appendChild(d);if(T.children.length>800)T.removeChild(T.firstChild);T.scrollTop=T.scrollHeight}
function sep(){const d=document.createElement('div');d.className='t-sep';T.appendChild(d)}
function clear(){T.textContent=''}

/* Stats polling */
function renderStats(s){if(!s)return;
 document.getElementById('ram-val').textContent=(s.ram_icon||'')+' '+Math.round(s.ram_percent||0)+'%%';
 document.getElementById('ram-label').textContent=s.ram_pressure||'';
 document.getElementById('git-val').textContent=(s.uncommitted_files||0)+' dirty';
 document.getElementById('git-label').textContent=s.git_branch||'';
 document.getElementById('deity-val').textContent=s.deity_count||0;
 document.getElementById('deity-label').textContent=(s.active_deities||[]).join(', ')||'';
 document.getElementById('accel-val').textContent=s.accel_icon||'';
 document.getElementById('accel-label').textContent=s.primary_accelerator||''}
function pollStats(){fetch('/api/stats').then(r=>r.json()).then(renderStats).catch(function(){})}
pollStats();setInterval(pollStats,10000);

/* ── View system ──────────────────────────────────────── */
window.switchView=function(view){
 currentView=view;
 document.getElementById('view-label').textContent=view;
 document.querySelectorAll('.nav-item').forEach(function(n){
  n.classList.toggle('active',n.dataset.view===view)});
 clear();
 var loader={home:viewHome,scan:viewScan,ghosts:viewGhosts,guard:viewGuard,
  notifications:viewNotifications,horus:viewHorus,vault:viewVault,ra:viewRa};
 (loader[view]||viewHome)();
};

function viewHome(){
 out('☥ Pantheon Command Center','t-gold');
 out('');
 out('  scan        Scan for infrastructure waste + ghost remnants','t-dim');
 out('  ghosts      Hunt dead application residuals','t-dim');
 out('  guard       System health, process slayer','t-dim');
 out('  doctor      Full diagnostic health check','t-dim');
 out('  network     Network security audit','t-dim');
 out('  hardware    CPU/GPU/ANE detection','t-dim');
 out('  quality     Code governance audit','t-dim');
 out('  dedup       Find duplicate files','t-dim');
 out('');
 out('Type a command or click a sidebar item.','t-dim');
}

function viewScan(){
 out('𓁢 Scan Results','t-gold');
 fetch('/api/findings').then(r=>r.json()).then(function(data){
  if(!data.findings||!data.findings.length){
   out('');out('No scan results. Type "scan" to run one.','t-dim');return}

  /* Count actionable items */
  let safeCount=0,safeSize=0,cautionCount=0,cautionSize=0;
  data.findings.forEach(function(f){
   if(f.severity==='safe'){safeCount++;safeSize+=f.size_bytes}
   if(f.severity==='caution'){cautionCount++;cautionSize+=f.size_bytes}
  });

  out('  '+data.findings.length+' findings · '+fmtSize(data.total_size)+' total waste','t-dim');
  out('  🟢 '+safeCount+' safe to clean ('+fmtSize(safeSize)+') · 🟡 '+cautionCount+' caution ('+fmtSize(cautionSize)+')','t-dim');
  sep();

  /* Bulk actions */
  if(safeCount>0){
   out('','t-dim');
   const bulk=document.createElement('div');bulk.className='t-line';
   const btn=document.createElement('span');btn.className='t-action';
   btn.style.cssText='color:#C8A951;font-weight:600;font-size:13px';
   btn.textContent='▸ CLEAN ALL '+safeCount+' SAFE ITEMS ('+fmtSize(safeSize)+')';
   btn.addEventListener('click',function(){cleanAllSafe(btn,data.findings)});
   bulk.appendChild(btn);T.appendChild(bulk);
   out('','t-dim');
  }
  sep();

  /* Group by category */
  const cats={};data.findings.forEach(function(f,i){f._i=i;
   if(!cats[f.category])cats[f.category]={items:[],size:0};
   cats[f.category].items.push(f);cats[f.category].size+=f.size_bytes});

  Object.keys(cats).sort(function(a,b){return cats[b].size-cats[a].size}).forEach(function(cat){
   const c=cats[cat];
   out('');out('  '+cat.toUpperCase()+' ('+c.items.length+' · '+fmtSize(c.size)+')','t-head');
   c.items.forEach(function(f){
    const row=document.createElement('div');row.className='t-line t-row';
    const sev=document.createElement('span');sev.textContent=({safe:'🟢',caution:'🟡',warning:'🟠'}[f.severity]||'⚪');
    sev.style.width='20px';
    const desc=document.createElement('span');desc.className='t-col';desc.style.flex='1';desc.textContent=f.description;
    const size=document.createElement('span');size.className='t-col-r';size.textContent=f.size_human||fmtSize(f.size_bytes);
    const act=document.createElement('span');act.className='t-action';act.textContent='[clean]';act.style.marginLeft='12px';
    act.addEventListener('click',function(){cleanIdx(act,f._i)});
    row.appendChild(sev);row.appendChild(desc);row.appendChild(size);
    if(f.severity==='safe'||f.severity==='caution')row.appendChild(act);
    T.appendChild(row)});
  });
  sep();out('');
  out('  🟢 safe = always safe to delete (caches, logs, temp files)','t-dim');
  out('  🟡 caution = review first (build artifacts, old venvs)','t-dim');
  out('  🟠 warning = may affect running services (shown but not cleanable)','t-dim');
  out('');out('  Type "scan" to re-scan · "clean all" for bulk cleanup · click [clean] per item','t-dim');
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function cleanAllSafe(btn,findings){
 const safeIdx=[];
 findings.forEach(function(f,i){if(f.severity==='safe')safeIdx.push(i)});
 if(!safeIdx.length)return;
 btn.textContent='▸ CLEANING '+safeIdx.length+' ITEMS...';btn.style.color='#C8A951';
 fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({indices:safeIdx,dry_run:false})
 }).then(r=>r.json()).then(function(d){
  btn.textContent='✓ FREED '+d.freed_human+' ('+d.cleaned+' items)';btn.style.color='#44FF88';
  /* Reload after 2s to show updated state */
  setTimeout(function(){switchView('scan')},2000);
 }).catch(function(e){btn.textContent='✗ Error: '+e.message;btn.style.color='#FF4444'});
}

function cleanIdx(el,idx){
 el.textContent='...';
 fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({indices:[idx],dry_run:false})
 }).then(r=>r.json()).then(function(d){
  if(d.cleaned>0){el.textContent='✓ '+d.freed_human;el.style.color='#44FF88'}
  else{el.textContent='skip';el.style.color='#666'}
 }).catch(function(){el.textContent='err';el.style.color='#FF4444'});
}

function viewGhosts(){
 out('𓂓 Ghost Hunt — Scanning...','t-gold');
 fetch('/api/ghosts').then(r=>r.json()).then(function(ghosts){
  if(!ghosts.length){out('');out('No ghost remnants found. System is clean.','t-ok');return}
  let total=0;ghosts.forEach(function(g){total+=g.total_size});
  out('  '+ghosts.length+' ghosts · '+fmtSize(total)+' waste','t-dim');sep();
  ghosts.sort(function(a,b){return b.total_size-a.total_size}).forEach(function(g){
   out('');out('  👻 '+g.app_name+' — '+fmtSize(g.total_size)+' ('+g.total_files+' files)','t-head');
   g.residuals.forEach(function(r){
    const row=document.createElement('div');row.className='t-line t-row';
    const type=document.createElement('span');type.className='t-col';type.style.width='140px';type.textContent=r.type;
    const path=document.createElement('span');path.className='t-col';path.style.flex='1';path.style.color='#666';path.textContent=r.path;
    const size=document.createElement('span');size.className='t-col-r';size.textContent=fmtSize(r.size_bytes);
    row.appendChild(type);row.appendChild(path);row.appendChild(size);T.appendChild(row)});
   const cleanRow=document.createElement('div');cleanRow.className='t-line';
   const act=document.createElement('span');act.className='t-action';act.textContent='[clean all residuals]';
   act.addEventListener('click',function(){
    act.textContent='cleaning...';
    fetch('/api/ghosts/clean',{method:'POST',headers:{'Content-Type':'application/json'},
     body:JSON.stringify({app_name:g.app_name,dry_run:false})
    }).then(r=>r.json()).then(function(d){
     act.textContent='✓ freed '+d.freed_human;act.style.color='#44FF88'
    }).catch(function(){act.textContent='error';act.style.color='#FF4444'})});
   cleanRow.appendChild(act);T.appendChild(cleanRow)});
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function viewGuard(){
 out('🛡 Guard — System Monitor','t-gold');
 out('');out('Running diagnostics...','t-dim');
 fetch('/api/doctor').then(r=>r.json()).then(function(rpt){
  out('  Health Score: '+rpt.Score+'/100','t-head');sep();
  (rpt.Findings||[]).forEach(function(f){
   const icon=({0:'✅',1:'ℹ️',2:'⚠️',3:'🔴'}[f.Severity]||'⚪');
   out('  '+icon+' '+f.Check+' — '+f.Message)});
  sep();out('');
  out('Process Slayer — type: kill node | kill electron | kill docker | kill lsp | kill build | kill ai','t-dim');
  out('Renice LSPs  — type: renice (deprioritize Language Servers, nice +10)','t-dim');
 }).catch(function(e){out('Doctor failed: '+e.message,'t-err')});
}

function viewNotifications(){
 out('🔔 Notifications','t-gold');
 fetch('/api/notifications?limit=30').then(r=>r.json()).then(function(items){
  if(!items.length){out('');out('No notifications yet.','t-dim');return}
  out('  '+items.length+' recent notifications','t-dim');sep();
  items.forEach(function(n){
   const icon=({success:'✅',error:'❌',warning:'⚠️',info:'ℹ️'}[n.severity]||'ℹ️');
   out('  '+icon+' '+n.source+' — '+n.summary+'  '+ago(n.timestamp))});
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function viewHorus(){
 out('𓂀 Horus — Code Graph','t-gold');
 out('');out('Type a symbol name to search, or "horus scan" to analyze the project.','t-dim');
}

function viewVault(){
 out('🏛 Vault — Context Sandbox','t-gold');
 fetch('/api/vault/stats').then(r=>r.json()).then(function(s){
  out('  '+s.totalEntries+' entries · '+fmtSize(s.totalBytes||0)+' · '+
   Object.keys(s.tagCounts||{}).length+' tags','t-dim');
  sep();out('');out('Type a search query to find content in the vault.','t-dim');
 }).catch(function(){out('Vault not available.','t-dim')});
}

function viewRa(){
 out('𓇶 Ra — Orchestrator','t-gold');
 out('');out('Loading deployment status...','t-dim');

 fetch('/api/ra/scopes').then(r=>r.json()).then(function(scopes){
  if(scopes.length){
   out('');out('  Available Scopes:','t-head');
   scopes.forEach(function(s){
    const deadline=s.deadline?' · deadline '+s.deadline:'';
    out('  ['+s.priority+'] '+s.display_name+' — '+s.repo_path+deadline)});
  }
 }).catch(function(){});

 fetch('/api/ra/status').then(r=>r.json()).then(function(d){
  sep();
  if(!d.deployed){out('');out('  No active deployment.','t-dim');
   out('');out('  Commands: deploy, ra status, ra collect, ra kill','t-dim');return}
  out('');out('  Deployment Status (started '+d.started_at+')','t-head');
  (d.windows||[]).forEach(function(w){
   const icon=({running:'⟳',completed:'✅',failed:'❌',crashed:'💀'}[w.state]||'⚪');
   out('  '+icon+' '+w.name+' — '+w.state+' ('+w.duration+')');
   if(w.log_tail)out('    '+w.log_tail.split('\\n').pop(),'t-dim')});
  if(d.all_done)out('');out('  All windows completed. Run "ra collect" to gather results.','t-dim');
 }).catch(function(){out('  Ra not available.','t-dim')});
}

/* ── Command input ────────────────────────────────────── */
const input=document.getElementById('term-input');
input.addEventListener('keydown',function(e){
 if(e.key!=='Enter')return;
 const raw=this.value.trim();this.value='';if(!raw)return;

 /* Built-in commands */
 if(raw==='clear'){clear();return}
 if(raw==='home'){switchView('home');return}

 /* View switches */
 const viewMap={scan:'scan',ghosts:'ghosts',guard:'guard',doctor:'guard',
  notifications:'notifications',horus:'horus',vault:'vault',ra:'ra',deploy:'ra'};
 if(viewMap[raw]){switchView(viewMap[raw]);return}

 /* Kill commands */
 if(raw.startsWith('kill ')){
  const target=raw.split(' ')[1];
  out('▸ kill '+target,'t-gold');
  fetch('/api/slay?target='+target+'&dry_run=false',{method:'POST'}).then(r=>r.json()).then(function(d){
   if(d.killed>0)out('✓ Killed '+d.killed+' '+target+' processes','t-ok');
   else out('No '+target+' processes found','t-dim');
  }).catch(function(e){out('✗ '+e.message,'t-err')});
  return}

 if(raw==='clean all'||raw==='clean safe'){
  out('▸ Cleaning all safe findings...','t-gold');
  fetch('/api/findings').then(r=>r.json()).then(function(data){
   const idx=[];(data.findings||[]).forEach(function(f,i){if(f.severity==='safe')idx.push(i)});
   if(!idx.length){out('No safe findings to clean.','t-dim');return}
   out('  Cleaning '+idx.length+' items...','t-dim');
   return fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({indices:idx,dry_run:false})}).then(r=>r.json()).then(function(d){
    out('✓ Freed '+d.freed_human+' ('+d.cleaned+' items cleaned)','t-ok');
    setTimeout(function(){switchView('scan')},1500)})
  }).catch(function(e){out('✗ '+e.message,'t-err')});
  return}

 if(raw==='judge'){
  out('▸ Loading findings for judgment...','t-gold');
  switchView('scan');return}

 if(raw==='renice'||raw==='renice lsp'){
  out('▸ renice lsp','t-gold');
  fetch('/api/guard/renice?target=lsp',{method:'POST'}).then(r=>r.json()).then(function(d){
   if(d.reniced>0){out('✓ Reniced '+d.reniced+' LSP processes (nice +10, Background QoS)','t-ok');
    (d.processes||[]).forEach(function(p){out('  PID '+p.pid+' '+p.name+' — '+p.rss_human,'t-dim')})}
   else out('No LSP processes found to renice','t-dim');
  }).catch(function(e){out('✗ '+e.message,'t-err')});
  return}

 /* Horus search */
 if(currentView==='horus'||raw.startsWith('horus ')){
  const q=raw.replace(/^horus\s*/,'');
  if(q==='scan'){out('▸ Scanning project...','t-gold');
   fetch('/api/horus/scan?path=.').then(r=>r.json()).then(function(g){
    const s=g.stats||g.Stats||{};
    out('  '+s.files+' files · '+s.packages+' packages · '+
     s.types+' types · '+s.functions+' functions · '+s.methods+' methods','t-dim')
   }).catch(function(e){out('Error: '+e.message,'t-err')});return}
  out('▸ search: '+q,'t-gold');
  fetch('/api/horus/query?path=.&filter='+encodeURIComponent('*'+q+'*')).then(r=>r.json()).then(function(syms){
   if(!syms||!syms.length){out('No symbols match "'+q+'"','t-dim');return}
   syms.slice(0,30).forEach(function(s){
    out('  '+s.kind+' '+(s.parent?s.parent+'.':'')+s.name+'  '+s.file+':'+s.line)})
  }).catch(function(e){out('Error: '+e.message,'t-err')});
  return}

 /* Vault search */
 if(currentView==='vault'){
  out('▸ search: '+raw,'t-gold');
  fetch('/api/vault/search?q='+encodeURIComponent(raw)+'&limit=10').then(r=>r.json()).then(function(res){
   if(!res.entries||!res.entries.length){out('No results.','t-dim');return}
   out('  '+res.totalHits+' hits','t-dim');sep();
   res.entries.forEach(function(e){
    out('  '+e.source+' ['+e.tag+']  '+e.createdAt,'t-head');
    out('  '+(e.snippet||'').substring(0,200),'t-dim');out('')})
  }).catch(function(e){out('Error: '+e.message,'t-err')});
  return}

 /* CLI command execution */
 if(running){out('A command is already running.','t-err');return}
 out('');out('▸ '+raw,'t-gold');
 const cmdMap={scan:'scan',ghosts:'ghosts',doctor:'doctor',guard:'guard',
  network:'network',hardware:'hardware',quality:'quality',dedup:'dedup'};
 const key=cmdMap[raw];
 if(!key){out('Unknown command: '+raw,'t-err');out('Available: scan, ghosts, doctor, guard, network, hardware, quality, dedup, kill <target>, renice','t-dim');return}
 running=true;
 fetch('/api/run?cmd='+key,{method:'POST'}).then(function(r){
  if(!r.ok)return r.json().then(function(e){throw new Error(e.error)});
 }).catch(function(e){out('✗ '+e.message,'t-err');running=false});
});

/* ── SSE ──────────────────────────────────────────────── */
if(typeof EventSource!=='undefined'){
 const es=new EventSource('/api/events');
 es.addEventListener('run_output',function(e){try{out(JSON.parse(e.data).line)}catch(x){}});
 es.addEventListener('run_complete',function(e){
  try{const d=JSON.parse(e.data);
   if(d.status==='success')out('✓ '+d.label+' ('+d.duration_ms+'ms)','t-ok');
   else out('✗ '+d.label+': '+(d.error||'failed'),'t-err');
   running=false;
   /* Auto-switch to actionable view after scan/ghost commands */
   if(d.key==='scan'){out('');out('Loading findings...','t-dim');
    setTimeout(function(){switchView('scan')},800)}
   else if(d.key==='ghosts'){setTimeout(function(){switchView('ghosts')},800)}
   else if(currentView==='scan'){setTimeout(function(){viewScan()},500)}
  }catch(x){running=false}});
}

viewHome();
})();
</script>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, pageShell("Pantheon", "home", body))
}

// ── Page Redirects (all views are SPA now) ─────────────────────────────
// These handlers exist so direct URLs like /scan still work.
// They redirect to the SPA with the view pre-selected via JS.

func spaRedirect(w http.ResponseWriter, r *http.Request, view string) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<script>location.replace('/');setTimeout(function(){switchView('%s')},100)</script>`, view)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request)          { spaRedirect(w, r, "scan") }
func (s *Server) handleGhosts(w http.ResponseWriter, r *http.Request)        { spaRedirect(w, r, "ghosts") }
func (s *Server) handleGuard(w http.ResponseWriter, r *http.Request)         { spaRedirect(w, r, "guard") }
func (s *Server) handleHorus(w http.ResponseWriter, r *http.Request)         { spaRedirect(w, r, "horus") }
func (s *Server) handleVault(w http.ResponseWriter, r *http.Request)         { spaRedirect(w, r, "vault") }

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	spaRedirect(w, r, "notifications")
}

// ── Legacy page code removed — all views are now rendered client-side
// in the terminal pane via the SPA entry point (handleOverview).

var _ = "legacy page handlers removed"

// Old multi-page handler bodies were here — removed in SPA rewrite.
// All rendering now happens in the terminal pane via JavaScript views.
// API endpoints in api.go, modules.go, findings.go serve the data.
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
