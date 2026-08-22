package dashboard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
)

// ── Shared Layout ───────────────────────────────────────────────────────

// pageShell wraps page-specific content in the shared dashboard layout.
// activePage is the nav item to highlight (e.g., "/", "/scan", "/guard").
//
// port is the port the server is ACTUALLY listening on, not the package
// default. The footer renders it as this node's address, so it has to be the
// live value: rendering the DashboardPort constant made `--port 8080` serve a
// working UI that told the operator the node was on 9119. Callers pass
// s.cfg.Port, which NewServer has already normalized to DashboardPort when
// unset, so this can never render :0.
func pageShell(title, activePage, bodyContent string, port int) string {
	navItems := []struct {
		Key   string
		Glyph string
		Label string
	}{
		{"home", "☥", "Home"},
		{"fleet", "⚑", "Fleet"},
		{"scan", "𓁢", "Scan"},
		{"ghosts", "𓂓", "Ghosts"},
		{"guard", "🛡", "Guard"},
		{"notifications", "🔔", "Notifications"},
		{"horus", "𓂀", "Horus"},
		{"vault", "🏛", "Vault"},
		{"sne", "⚡", "SNE"},
		{"recovery", "↻", "Recovery"},
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
<title>%s — Horus — Sirsi</title>
<style>
:root{%s}
*{margin:0;padding:0;box-sizing:border-box}
body{background:%s;color:%s;font-family:'SF Mono',Menlo,Consolas,'Courier New',monospace;
display:flex;min-height:100vh;overflow:hidden}
::-webkit-scrollbar{width:6px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:color-mix(in srgb, var(--gold) 20%%, transparent);border-radius:3px}

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
.nav-item:hover{background:color-mix(in srgb, var(--gold) 6%%, transparent);color:%s}
.nav-item.active{background:color-mix(in srgb, var(--gold) 8%%, transparent);color:%s;border-left-color:%s}
.nav-glyph{width:20px;font-size:14px;margin-right:8px;text-align:center}
.sidebar-footer{padding:12px 16px;border-top:1px solid %s;font-size:8px;color:var(--line);letter-spacing:1px;
font-family:Inter,-apple-system,system-ui,sans-serif}

/* Main — content is capped at 1400px and centered in the space right of the
   fixed sidebar so ultra-wide viewports don't strand everything top-left. */
.main{margin-left:180px;flex:1;display:flex;flex-direction:column;align-items:center;height:100vh;overflow:hidden}
.main-inner{width:100%%;max-width:1400px;display:flex;flex-direction:column;height:100vh;overflow:hidden;
border-left:1px solid color-mix(in srgb, var(--gold) 6%%, transparent);border-right:1px solid color-mix(in srgb, var(--gold) 6%%, transparent)}

/* Stats bar */
.stats-bar{display:flex;gap:1px;background:color-mix(in srgb, var(--gold) 6%%, transparent);border-bottom:1px solid %s;flex-shrink:0}
.stat{flex:1;padding:12px 16px;background:%s}
.stat-label{font-size:9px;color:%s;letter-spacing:1.5px;text-transform:uppercase;
font-family:Inter,-apple-system,system-ui,sans-serif;margin-bottom:4px}
.stat-value{font-size:16px;color:%s;font-weight:400}
.stat-sub{font-size:10px;color:var(--dim);margin-top:2px}
/* Only tiles that actually go somewhere get a pointer and a chevron. A readout
   that looks clickable and isn't is worse than one that plainly isn't. */
.stat-go{cursor:pointer}
.stat-go:hover{background:color-mix(in srgb, var(--gold) 7%%, transparent)}
.stat-go:focus-visible{outline:1px solid var(--gold);outline-offset:-1px}
.stat-go .stat-label::after{content:' \203A';color:var(--gold);opacity:.6}

/* Clickable command rows (home screen) */
.t-cmd{display:flex;gap:14px;cursor:pointer;padding:2px 8px;margin:0 -8px;border-radius:3px}
.t-cmd:hover,.t-cmd:focus-visible{background:color-mix(in srgb, var(--gold) 8%%, transparent);outline:none}
.t-cmd-name{color:var(--ink2);min-width:96px;flex-shrink:0}
.t-cmd-desc{color:var(--dim)}
.t-cmd:hover .t-cmd-name,.t-cmd:focus-visible .t-cmd-name{color:var(--gold)}
.t-cmd:hover .t-cmd-desc,.t-cmd:focus-visible .t-cmd-desc{color:var(--ink2)}

/* Terminal */
.terminal-wrap{flex:1;display:flex;flex-direction:column;overflow:hidden}
.term-input-bar{display:flex;align-items:center;padding:0;border-bottom:1px solid %s;background:rgba(3,3,8,.9);flex-shrink:0}
.term-prompt{color:%s;padding:8px 0 8px 16px;font-size:13px;flex-shrink:0}
.term-input{flex:1;background:none;border:none;color:%s;font-size:13px;padding:8px 16px 8px 8px;
font-family:inherit;outline:none}
.term-input::placeholder{color:var(--dim)}
.term-view-label{color:var(--dim);font-size:10px;padding-right:16px;letter-spacing:1px;text-transform:uppercase;
font-family:Inter,-apple-system,system-ui,sans-serif;flex-shrink:0}
.terminal{flex:1;overflow-y:auto;padding:12px 16px;background:rgba(3,3,8,.95);line-height:1.6;font-size:12px}
.t-line{margin:0;white-space:pre-wrap;word-break:break-all}
.t-dim{color:var(--dim)}
.t-out{color:var(--ink2)}
.t-ok{color:var(--ok)}
.t-err{color:var(--danger)}
.t-gold{color:var(--gold)}
.t-head{color:var(--gold);font-weight:600;font-size:13px;margin-top:8px}
.t-row{display:flex;gap:16px;padding:2px 0}
.t-row:hover{background:color-mix(in srgb, var(--gold) 3%%, transparent)}
.t-col{color:var(--ink2)}.t-col-r{color:var(--gold);text-align:right;min-width:80px}
	.t-action{color:var(--dim);cursor:pointer;transition:color .15s;text-decoration:underline;text-decoration-color:var(--line)}
	.t-action:hover{color:var(--gold);text-decoration-color:var(--gold)}
	.t-action:focus-visible,.nav-item:focus-visible{color:var(--gold);outline:2px solid var(--gold);outline-offset:3px;text-decoration-color:var(--gold)}
	.t-sep{border-top:1px solid color-mix(in srgb, var(--gold) 6%%, transparent);margin:6px 0}
	@media (prefers-reduced-motion:reduce){*,*::before,*::after{animation-duration:.01ms!important;animation-iteration-count:1!important;scroll-behavior:auto!important;transition-duration:.01ms!important}}
	@media (prefers-contrast:more){.t-action:focus-visible,.nav-item:focus-visible,.stat-go:focus-visible{outline-width:3px}.t-dim,.t-action{color:var(--ink2)}}
</style>
</head>
<body>
<div class="sidebar">
 <div class="sidebar-brand"><h1>𓂀 Horus</h1></div>
 <nav class="sidebar-nav">%s</nav>
 <div class="sidebar-footer">LOCAL NODE • 127.0.0.1:%d</div>
</div>
<div class="main"><div class="main-inner">%s</div></div>
</body>
</html>`,
		title,
		brand.CSSVars(brand.Dark),
		ColorBg, ColorWhite,
		ColorBorder, ColorBorder,
		ColorEmerald,
		ColorDim, ColorWhite, ColorEmerald, ColorEmerald,
		ColorBorder,
		ColorBorder, ColorBg,
		ColorEmerald, ColorEmerald,
		ColorBorder, ColorEmerald, ColorWhite,
		navHTML.String(),
		port,
		bodyContent,
	)
}

// ── SPA Entry Point ───────────────────────────────────────────────────

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	if token := s.sneAccess.snapshot(); token != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     sneLocalSessionCookie,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
	if r.URL.Path != "/" && r.URL.Path != "/sne" {
		http.NotFound(w, r)
		return
	}

	body := `

<!-- Stats bar -->
<div class="stats-bar">
 <div class="stat stat-go" data-cmd="guard" tabindex="0" role="button"
  title="Open Guard — the view that explains memory pressure and can act on it"><div class="stat-label">RAM</div>
  <div class="stat-value" id="ram-val">—</div>
  <div class="stat-sub" id="ram-label"></div></div>
 <div class="stat"><div class="stat-label">Git</div>
  <div class="stat-value" id="git-val">—</div>
  <div class="stat-sub" id="git-label"></div></div>
 <div class="stat"><div class="stat-label">Deities</div>
  <div class="stat-value" id="deity-val">0</div>
  <div class="stat-sub" id="deity-label"></div></div>
 <div class="stat stat-go" data-cmd="hardware" tabindex="0" role="button"
  title="Run hardware detection — CPU / GPU / ANE"><div class="stat-label">Platform</div>
  <div class="stat-value" id="accel-val">—</div>
  <div class="stat-sub" id="accel-label"></div></div>
</div>

<!-- Terminal -->
<div class="terminal-wrap">
 <div class="term-input-bar">
  <span class="term-prompt">𓉴 </span>
  <input type="text" class="term-input" id="term-input" placeholder="Ask a question, or type a command (scan, ghosts, doctor, guard, network, hardware)" autocomplete="off">
  <span class="term-view-label" id="view-label">home</span>
 </div>
 <div class="terminal" id="terminal">
  <div class="t-line t-dim">𓂀 Horus — use sidebar or type a command</div>
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
let currentView=location.pathname==='/sne'?'sne':'home';let running=false;

function out(text,cls){const d=document.createElement('div');d.className='t-line '+(cls||'t-out');
 d.textContent=text;T.appendChild(d);if(T.children.length>800)T.removeChild(T.firstChild);T.scrollTop=T.scrollHeight}
function sep(){const d=document.createElement('div');d.className='t-sep';T.appendChild(d)}
function clear(){T.textContent=''}

/* Stats polling */
function renderStats(s){if(!s)return;
 /* Body is a Sprintf ARGUMENT, not format — a literal % is correct here. */
 document.getElementById('ram-val').textContent=(s.ram_icon||'')+' '+Math.round(s.ram_percent||0)+'%';
 /* Show real used/total when the producer supplies them; never render
    fabricated numbers when they're absent (data honesty). */
 document.getElementById('ram-label').textContent=(s.total_ram>0
  ?fmtSize(s.used_ram||0)+' / '+fmtSize(s.total_ram)+' · ':'')+(s.ram_pressure||'');
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
 var loader={home:viewHome,fleet:viewFleet,scan:viewScan,ghosts:viewGhosts,guard:viewGuard,
  notifications:viewNotifications,horus:viewHorus,vault:viewVault,sne:viewSNE,recovery:viewRecovery,ra:viewRa};
 (loader[view]||viewHome)();
};

function viewHome(){
 out('𓂀 Horus — Local Workstation Monitor','t-gold');
 out('');
 cmdRow('scan','Scan for infrastructure waste + ghost remnants');
 cmdRow('ghosts','Hunt dead application residuals');
 cmdRow('guard','System health, process slayer');
 cmdRow('doctor','Full diagnostic health check');
 cmdRow('network','Network security audit');
 cmdRow('hardware','CPU/GPU/ANE detection');
 cmdRow('quality','Code governance audit');
 cmdRow('dedup','Find duplicate files');
 out('');
 out('Click any command above, or type it. The sidebar switches views.','t-dim');
}

function viewSNE(){
 out('⚡ SNE — Local AI Engine','t-gold');
 out('Pantheon installs, verifies, admits, and supervises. SNE computes. Nexus presents.','t-dim');
 sep();
 fetch('/api/sne').then(function(r){return r.json().then(function(body){
  if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})}).then(function(data){
  out('Service      '+(data.ready?'READY':'NOT READY'),data.ready?'t-ok':'t-err');
  out('Model        '+(data.active_model||'none'),'t-out');
  out('Mac          '+(data.device_family||'unknown'),'t-out');
  out('Memory       '+fmtSize(data.unified_memory_bytes||0),'t-out');
  if(data.runtime_catalog){
   const c=data.runtime_catalog;
   out('Catalog      '+(c.state||'unknown')+(c.signed_required?' · signed':''),c.state==='verified'?'t-ok':'t-err');
   if(c.catalog_id)out('Catalog ID   '+c.catalog_id,'t-out');
   if(c.version_sha256)out('Version      '+c.version_sha256.slice(0,12)+'… · '+c.entries+' entries · '+c.versions+' retained','t-out');
   out('Rollback     '+(c.rollback_available?'available':'not available'),c.rollback_available?'t-gold':'t-dim');
   if(c.update_feed_configured){
    const check=document.createElement('div');check.className='t-line t-action';check.textContent='[Check signed catalog updates]';check.tabIndex=0;check.setAttribute('role','button');
    check.onclick=checkSNECatalogUpdates;check.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();checkSNECatalogUpdates()}};T.appendChild(check);
   }
   (c.retained_versions||[]).forEach(function(version){
    if(version===c.version_sha256)return;
    const row=document.createElement('div');row.className='t-line t-row';
    const label=document.createElement('span');label.className='t-col';label.style.flex='1';label.textContent='Retained     '+version.slice(0,12)+'…';
    const rollback=document.createElement('span');rollback.className='t-action';rollback.textContent='[Rollback]';rollback.tabIndex=0;rollback.setAttribute('role','button');
    rollback.onclick=function(){mutateSNECatalog('rollback',version)};
    rollback.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();mutateSNECatalog('rollback',version)}};
    const remove=document.createElement('span');remove.className='t-action';remove.style.marginLeft='12px';remove.textContent='[Remove]';remove.tabIndex=0;remove.setAttribute('role','button');
    remove.onclick=function(){mutateSNECatalog('remove',version)};
    remove.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();mutateSNECatalog('remove',version)}};
    row.appendChild(label);row.appendChild(rollback);row.appendChild(remove);T.appendChild(row);
   });
   if(c.error)out('Catalog error '+c.error,'t-err');
  }
	  if(data.recovery)out('Next         '+data.recovery,'t-dim');
	  out('Model tools   '+(data.lifecycle_tools_ready?'ready':'unavailable')+(data.lifecycle_tools_status?' · '+data.lifecycle_tools_status:''),data.lifecycle_tools_ready?'t-ok':'t-err');
  sep();
  out('CATALOG','t-head');
  (data.catalog||[]).forEach(function(m){
   const row=document.createElement('div');row.className='t-line t-row';
   const state=document.createElement('span');state.style.width='22px';
   state.textContent=m.active?'●':(m.installed?'◆':'○');
   state.style.color=m.active?'var(--ok)':(m.state==='incompatible-memory'?'var(--danger)':'var(--gold)');
   const name=document.createElement('span');name.className='t-col';name.style.flex='1';
   name.textContent=m.parameter_class+' · '+m.weight_format.toUpperCase()+m.weight_bits+' · '+m.execution_mode+(m.runtime_id?' · '+m.runtime_id:'');
   name.title=m.model_id+' · '+(m.support_status||'unqualified')+(m.next_gate?' · next '+m.next_gate:'');
   const support=document.createElement('span');support.className='t-col-r';support.style.minWidth='118px';
   support.textContent=m.support_status||'unqualified';
   support.style.color=m.support_status==='release-supported'?'var(--ok)':(m.support_status==='research-only'?'var(--danger)':'var(--gold)');
   const mem=document.createElement('span');mem.className='t-col-r';mem.textContent=fmtSize(m.memory_bytes);
   const action=document.createElement('span');action.className='t-action';action.style.marginLeft='12px';
   action.textContent='['+m.action_label+']';
   if(!m.action_enabled){action.style.cursor='default';action.style.opacity='.55';action.style.textDecoration='none';action.setAttribute('aria-disabled','true')}
   else{action.tabIndex=0;action.setAttribute('role','button');action.setAttribute('aria-label',m.action_label+' '+m.model_id+(m.runtime_id?' using '+m.runtime_id:''));action.onclick=function(){actSNE(m)};
    action.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();actSNE(m)}}}
	   row.appendChild(state);row.appendChild(name);row.appendChild(support);row.appendChild(mem);row.appendChild(action);
	   if(m.installed&&!m.active){const remove=document.createElement('span');remove.className='t-action';remove.style.marginLeft='12px';remove.textContent='[Remove model]';
	    if(!m.removal_enabled){remove.style.cursor='default';remove.style.opacity='.55';remove.style.textDecoration='none';remove.setAttribute('aria-disabled','true');remove.title=m.removal_reason||'Stop SNE before removal'}
	    else{remove.tabIndex=0;remove.setAttribute('role','button');remove.setAttribute('aria-label','Remove installed model '+m.model_id);remove.onclick=function(){removeSNEModel(m)};
	     remove.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();removeSNEModel(m)}}}row.appendChild(remove)}
	   T.appendChild(row);
   if(m.license_id){const license=document.createElement('div');license.className='t-line t-dim';license.style.cssText='padding-left:38px;font-size:10px';license.appendChild(document.createTextNode('License: '+m.license_id+(m.license_acceptance_required?' · acceptance required':'')));
    if(m.license_url){const terms=document.createElement('a');terms.href=m.license_url;terms.target='_blank';terms.rel='noopener noreferrer';terms.textContent=' · Review terms';terms.style.marginLeft='4px';license.appendChild(terms)}T.appendChild(license)}
   if(m.reason){const why=document.createElement('div');why.className='t-line t-dim';
    why.style.cssText='padding-left:38px;font-size:10px';why.textContent=m.reason;T.appendChild(why)}
   if(m.next_gate&&m.next_gate!=='complete'){const gate=document.createElement('div');gate.className='t-line t-dim';
    gate.style.cssText='padding-left:38px;font-size:10px';gate.textContent='Next qualification gate: '+m.next_gate;T.appendChild(gate)}
  });
  out('');out('● active · ◆ installed · ○ available · only release-supported tuples can install or start','t-dim');
  if(data.lifecycle&&data.lifecycle.state!=='stopped'&&data.lifecycle.state!=='not-configured'){
   if(data.lifecycle.state==='failed')renderSNELifecycleFailure(data.lifecycle);
   else out('Lifecycle    '+data.lifecycle.state+(data.lifecycle.runtime_id?' · '+data.lifecycle.runtime_id:''),'t-dim');
  }
  out('Installs are transactional; starts are exact-tuple, package-bound, and Pantheon-supervised.','t-dim');
  const activeSupported=(data.catalog||[]).some(function(m){return m.active&&m.support_status==='release-supported'});
  if(data.ready&&activeSupported){const nexus=document.createElement('div');nexus.className='t-line t-action';nexus.textContent='[Open Nexus Local AI]';nexus.tabIndex=0;nexus.setAttribute('role','button');nexus.setAttribute('aria-label','Open Nexus with the verified local SNE model');
   nexus.onclick=openSNENexus;nexus.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();openSNENexus()}};T.appendChild(nexus)}
  const diagnostics=document.createElement('div');diagnostics.className='t-line t-action';diagnostics.textContent='[Export privacy-safe support diagnostics]';diagnostics.tabIndex=0;diagnostics.setAttribute('role','button');
  diagnostics.onclick=exportSNEDiagnostics;diagnostics.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();exportSNEDiagnostics()}};T.appendChild(diagnostics);
  const support=document.createElement('div');support.className='t-line t-action';support.textContent='[Export complete SNE support bundle]';support.tabIndex=0;support.setAttribute('role','button');support.setAttribute('aria-label','Export complete privacy-safe SNE support bundle');
  support.onclick=exportSNESupportBundle;support.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();exportSNESupportBundle()}};T.appendChild(support);
 }).catch(function(e){out('SNE read model unavailable: '+e.message,'t-err')});
}

function viewRecovery(){
 out('↻ Recovery — Applications & Services','t-gold');
 out('Restore resumes declared durable state. Fresh deliberately clears registered transient queues or caches.','t-dim');
 sep();
 fetch('/api/recovery').then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(data){
  const targets=data.targets||[];
  if(!targets.length){out('No governed recovery targets are registered.','t-dim');return}
  targets.forEach(function(target){
   const row=document.createElement('div');row.className='t-line t-row';
   const state=document.createElement('span');state.className='t-col';state.style.flex='1';state.textContent=target.target_id+' · '+target.kind+(target.auto_resume?' · auto-resume':'')+(target.phase?' · '+target.phase:'');row.appendChild(state);
   if(target.restore_supported)row.appendChild(recoveryAction('[Restore]','Restore '+target.target_id+' from its declared durable session or checkpoint?',function(){restartRecoveryTarget(target.target_id,'restore')}));
   if(target.fresh_supported){const fresh=recoveryAction('[Fresh restart]','Fresh restart '+target.target_id+'? Pantheon will discard only its registered transient queue/cache files.',function(){restartRecoveryTarget(target.target_id,'fresh')});fresh.style.marginLeft='12px';row.appendChild(fresh)}
   if(target.phase==='captured'||target.phase==='stopped'||target.phase==='started'){const resume=recoveryAction('[Resume interrupted]','Resume the interrupted '+target.mode+' operation for '+target.target_id+'?',function(){resumeRecoveryTarget(target.target_id)});resume.style.marginLeft='12px';row.appendChild(resume)}
   T.appendChild(row);
   if(target.failure_code)out('  Failure      '+target.failure_code,'t-err');
  });
  out('');out('Every action is registry-bound, receipt-backed, and requires a verified replacement process.','t-dim');
 }).catch(function(e){out('Recovery unavailable: '+e.message,'t-err')});
}

function recoveryAction(label,question,action){
 const control=document.createElement('span');control.className='t-action';control.textContent=label;control.tabIndex=0;control.setAttribute('role','button');control.setAttribute('aria-label',label.replace(/[\[\]]/g,''));
 control.onclick=function(){if(confirm(question))action()};control.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();control.click()}};return control;
}

function restartRecoveryTarget(targetID,mode){
 out((mode==='restore'?'Restoring ':'Fresh restarting ')+targetID+'…','t-gold');
 fetch('/api/recovery/restart',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({target_id:targetID,mode:mode})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.failure_code||body.error||('HTTP '+r.status));return body})})
 .then(function(){out(targetID+' is ready.','t-ok');setTimeout(viewRecovery,300)})
 .catch(function(e){out('Recovery rejected: '+e.message,'t-err');setTimeout(viewRecovery,300)});
}

function resumeRecoveryTarget(targetID){
 out('Resuming interrupted recovery for '+targetID+'…','t-gold');
 fetch('/api/recovery/resume',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({target_id:targetID,mode:''})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.failure_code||body.error||('HTTP '+r.status));return body})})
 .then(function(){out(targetID+' is ready.','t-ok');setTimeout(viewRecovery,300)})
 .catch(function(e){out('Resume rejected: '+e.message,'t-err');setTimeout(viewRecovery,300)});
}

function exportSNEDiagnostics(){
 const link=document.createElement('a');link.href='/api/sne/diagnostics';link.download='';document.body.appendChild(link);link.click();link.remove();
 out('Exported privacy-safe SNE support diagnostics.','t-ok');
}

function openSNENexus(){
 out('Opening Nexus with this Mac\'s private SNE capability…','t-gold');
 fetch('/api/sne/nexus/open',{method:'POST'}).then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error((body.error&&body.error.message)||body.error||('HTTP '+r.status));return body})})
 .then(function(body){out('Nexus opened for '+body.model+'.','t-ok')})
 .catch(function(e){out('Nexus handoff rejected: '+e.message,'t-err')});
}

function exportSNESupportBundle(){
 if(!confirm('Create a privacy-safe SNE support bundle? It includes package identity, admission and resource state, local health counters, and signature status. It excludes conversations, model data, caches, logs, environment values, network configuration, and machine identifiers. Review the archive before sharing.'))return;
 out('Creating privacy-safe SNE support bundle…','t-gold');
 fetch('/api/sne/support-bundle',{method:'POST'}).then(function(r){if(!r.ok)return r.json().then(function(body){throw new Error(body.error||('HTTP '+r.status))});return r.blob()})
 .then(function(blob){const url=URL.createObjectURL(blob);const link=document.createElement('a');link.href=url;link.download='sirsi-sne-support.zip';document.body.appendChild(link);link.click();link.remove();setTimeout(function(){URL.revokeObjectURL(url)},1000);out('Exported privacy-verified SNE support bundle. Review it before sharing.','t-ok')})
 .catch(function(e){out('Support bundle export failed: '+e.message,'t-err')});
}

function renderSNELifecycleFailure(state){
 out('Lifecycle    failed'+(state.error_code?' · '+state.error_code:'')+(state.error?' · '+state.error:''),'t-err');
 const resource=state.resource_admission;
 if(resource){
  out('Memory       '+fmtSize(resource.required_bytes)+' required · '+fmtSize(resource.available_ram_bytes)+' available','t-dim');
  out('Headroom     '+fmtSize(resource.dynamic_reserve_bytes)+' dynamic · '+fmtSize(resource.lifecycle_reserve_bytes)+' lifecycle','t-dim');
  out('Swap         '+fmtSize(resource.swap_used_bytes)+' used · '+fmtSize(resource.swap_limit_bytes)+' limit','t-dim');
 }
 if(state.recovery)out('Recovery     '+state.recovery,'t-gold');
 if(state.model_id){
  const retry=document.createElement('div');retry.className='t-line t-action';retry.textContent='[Retry when conditions are safe]';retry.tabIndex=0;retry.setAttribute('role','button');
  retry.onclick=function(){startSNE(state.model_id,state.runtime_id||'')};
  retry.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();startSNE(state.model_id,state.runtime_id||'')}};
  T.appendChild(retry);
 }
}

function installSNE(catalogEntry,modelID,licenseID,licenseURL){
 if(!licenseID||!licenseURL){out('Install blocked: verified license terms are unavailable.','t-err');return}
 const dialog=document.createElement('dialog');dialog.setAttribute('aria-labelledby','sne-license-title');dialog.style.cssText='max-width:560px;border:1px solid var(--gold);background:var(--bg);color:var(--text);padding:22px;box-shadow:0 18px 60px rgba(0,0,0,.45)';
 const title=document.createElement('h2');title.id='sne-license-title';title.textContent='Review model terms';title.style.cssText='margin:0 0 12px;color:var(--gold);font-size:16px';dialog.appendChild(title);
 const identity=document.createElement('p');identity.textContent=modelID;identity.style.cssText='overflow-wrap:anywhere';dialog.appendChild(identity);
 const explanation=document.createElement('p');explanation.textContent='Pantheon will download and verify this exact signed model tuple. Acceptance is recorded in its checkout receipt.';dialog.appendChild(explanation);
 const terms=document.createElement('a');terms.href=licenseURL;terms.target='_blank';terms.rel='noopener noreferrer';terms.textContent='Review '+licenseID+' in a new window';terms.style.color='var(--gold)';dialog.appendChild(terms);
 const consentRow=document.createElement('label');consentRow.style.cssText='display:flex;gap:10px;align-items:flex-start;margin:20px 0';
 const consent=document.createElement('input');consent.type='checkbox';consent.setAttribute('aria-describedby','sne-license-consent-copy');
 const consentCopy=document.createElement('span');consentCopy.id='sne-license-consent-copy';consentCopy.textContent='I reviewed and accept these terms for this model installation.';consentRow.appendChild(consent);consentRow.appendChild(consentCopy);dialog.appendChild(consentRow);
 const actions=document.createElement('div');actions.style.cssText='display:flex;justify-content:flex-end;gap:10px';
 const cancel=document.createElement('button');cancel.type='button';cancel.textContent='Cancel';
 const install=document.createElement('button');install.type='button';install.textContent='Accept and install';install.disabled=true;
 consent.onchange=function(){install.disabled=!consent.checked};cancel.onclick=function(){dialog.close('cancel')};
 install.onclick=function(){if(!consent.checked)return;install.disabled=true;cancel.disabled=true;dialog.close('accepted');beginSNEInstall(catalogEntry)};
 actions.appendChild(cancel);actions.appendChild(install);dialog.appendChild(actions);dialog.addEventListener('close',function(){dialog.remove()});document.body.appendChild(dialog);dialog.showModal();consent.focus();
}

function beginSNEInstall(catalogEntry){
 out('Starting verified installation for '+catalogEntry+'…','t-gold');
 fetch('/api/sne/install',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({catalog_entry:catalogEntry,accept_license:true,allow_research:false})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(job){pollSNEInstall(job.id)}).catch(function(e){out('Install rejected: '+e.message,'t-err')});
}

function discardSNEPrepared(catalogEntry,modelID){
 if(!confirm('Discard the retained download for '+modelID+'? This removes only the failed prepared source. Installed models and shared model-store objects are not changed.'))return;
 out('Discarding retained download for '+catalogEntry+'…','t-gold');
 fetch('/api/sne/prepared/discard',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({catalog_entry:catalogEntry})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(body){const result=body.result||{};out('Discarded retained download for '+result.catalog_entry+' · revision '+result.revision+'. Installed models were not changed.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Retained download cleanup rejected: '+e.message,'t-err')});
}

function actSNE(model){
 if(model.action_kind==='install'){installSNE(model.catalog_entry,model.model_id,model.license_id,model.license_url);return}
 if(model.action_kind==='start'){startSNE(model.model_id,model.runtime_id||'');return}
 if(model.action_kind==='stop'){stopSNE();return}
}

function removeSNEModel(model){
	 if(!model.removal_enabled){out(model.removal_reason||'Stop SNE before removing this model.','t-err');return}
	 if(!confirm('Remove '+model.model_id+' from this Mac? Pantheon will remove its governed model view, retain any objects shared by another installed model, and allow the model to be installed again later.'))return;
	 out('Removing '+model.model_id+' transactionally…','t-gold');
	 fetch('/api/sne/remove',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({catalog_entry:model.catalog_entry,model_id:model.model_id})})
	 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
	 .then(function(body){const result=body.result||{};out('Removed '+model.model_id+'. Shared objects retained: '+(result.objects_retained||0)+'.','t-ok');setTimeout(viewSNE,300)})
	 .catch(function(e){out('Removal rejected: '+e.message,'t-err')});
}

function startSNE(modelID,runtimeID){
 out('Starting verified runtime for '+modelID+(runtimeID?' · '+runtimeID:'')+'…','t-gold');
 fetch('/api/sne/start',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({model_id:modelID,runtime_id:runtimeID||undefined})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){pollSNELifecycle()}).catch(function(e){out('Start rejected: '+e.message,'t-err')});
}

function stopSNE(){
 out('Stopping SNE under Pantheon supervision…','t-gold');
 fetch('/api/sne/stop',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){out('SNE stopped.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Stop rejected: '+e.message,'t-err')});
}

function mutateSNECatalog(action,version){
 const verb=action==='rollback'?'Roll back to':'Remove inactive';
 if(!confirm(verb+' signed catalog '+version.slice(0,12)+'…? SNE must be stopped.'))return;
 out((action==='rollback'?'Rolling back to ':'Removing ')+version.slice(0,12)+'…','t-gold');
 fetch('/api/sne/catalog/'+action,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({version_sha256:version})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){out('Catalog '+action+' completed.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Catalog '+action+' rejected: '+e.message,'t-err')});
}

function checkSNECatalogUpdates(){
 out('Checking the authenticated SNE catalog feed…','t-gold');
 fetch('/api/sne/catalog/updates').then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(feed){
  out('Update feed  '+feed.feed_id,'t-ok');
  const available=(feed.versions||[]).filter(function(v){return v!==feed.current_version_sha256});
  if(!available.length){out('Catalog      current','t-ok');return}
  available.forEach(function(version){
   const row=document.createElement('div');row.className='t-line t-row';
   const label=document.createElement('span');label.className='t-col';label.style.flex='1';label.textContent='Available    '+version.slice(0,12)+'…';
   const install=document.createElement('span');install.className='t-action';install.textContent='[Install]';install.tabIndex=0;install.setAttribute('role','button');
   install.onclick=function(){installSNECatalogUpdate(version)};install.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();installSNECatalogUpdate(version)}};
   row.appendChild(label);row.appendChild(install);T.appendChild(row);
  });
 }).catch(function(e){out('Update check failed: '+e.message,'t-err')});
}

function installSNECatalogUpdate(version){
 if(!confirm('Install authenticated catalog '+version.slice(0,12)+'…? SNE must be stopped and the current version will remain available for rollback.'))return;
 out('Downloading and verifying signed catalog '+version.slice(0,12)+'…','t-gold');
 fetch('/api/sne/catalog/install',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({version_sha256:version})})
 .then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(){out('Catalog update installed; prior version retained.','t-ok');setTimeout(viewSNE,300)})
 .catch(function(e){out('Catalog update rejected: '+e.message,'t-err')});
}

function pollSNELifecycle(){
 fetch('/api/sne/lifecycle').then(function(r){return r.json().then(function(body){if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})})
 .then(function(state){
  if(state.state==='ready'){out('Ready        '+state.model_id,'t-ok');setTimeout(viewSNE,300);return}
  if(state.state==='failed'){renderSNELifecycleFailure(state);return}
  out('Lifecycle    '+state.state,'t-dim');setTimeout(pollSNELifecycle,1000);
 }).catch(function(e){out('Lifecycle status unavailable: '+e.message,'t-err')});
}

function pollSNEInstall(id){
 fetch('/api/sne/install/status?id='+encodeURIComponent(id)).then(function(r){return r.json().then(function(body){
  if(!r.ok)throw new Error(body.error||('HTTP '+r.status));return body})}).then(function(job){
   if(job.progress){out('Install      '+job.progress.files_done+'/'+job.progress.files_total+' files · '+fmtSize(job.progress.bytes_done)+' / '+fmtSize(job.progress.bytes_total),'t-dim')}
   if(job.state==='installed'){out('Installed    '+job.model_id,'t-ok');setTimeout(viewSNE,300);return}
   if(job.state==='failed'){
    out('Install failed: '+job.error,'t-err');
    const discard=document.createElement('div');discard.className='t-line t-action';discard.textContent='[Discard retained download]';discard.tabIndex=0;discard.setAttribute('role','button');discard.setAttribute('aria-label','Discard retained download for '+job.model_id);
    discard.onclick=function(){discardSNEPrepared(job.catalog_entry,job.model_id)};
    discard.onkeydown=function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();discardSNEPrepared(job.catalog_entry,job.model_id)}};
    T.appendChild(discard);return
   }
   setTimeout(function(){pollSNEInstall(id)},1000);
 }).catch(function(e){out('Install status unavailable: '+e.message,'t-err')});
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
   btn.style.cssText='color:var(--gold);font-weight:600;font-size:13px';
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
    const desc=document.createElement('span');desc.className='t-col';desc.style.flex='1';
    desc.textContent=f.description;
    if(f.advisory){desc.title=f.advisory+(f.remediation?' | Fix: '+f.remediation:'')}
    const size=document.createElement('span');size.className='t-col-r';size.textContent=f.size_human||fmtSize(f.size_bytes);
    row.appendChild(sev);row.appendChild(desc);row.appendChild(size);
    if(f.can_fix){
     const act=document.createElement('span');act.className='t-action';
     act.textContent=f.remediation==='Flag for review'?'[flag]':'['+f.remediation+']';
     act.style.marginLeft='12px';
     if(f.breaking){act.style.color='var(--warn)'}
     if(f.remediation!=='Flag for review'){act.addEventListener('click',function(){cleanIdx(act,f._i)})}
     else{act.style.cursor='default';act.style.textDecoration='none';act.style.color='var(--dim)'}
     row.appendChild(act);
    }else{
     const flag=document.createElement('span');flag.style.cssText='margin-left:12px;color:var(--dim);font-size:10px';
     flag.textContent='review';row.appendChild(flag);
    }
    T.appendChild(row);
    /* Show advisory as sub-line */
    if(f.advisory){
     const adv=document.createElement('div');adv.className='t-line';
     adv.style.cssText='padding-left:24px;font-size:10px;color:var(--dim);margin-top:-2px';
     adv.textContent=f.advisory;T.appendChild(adv)}
   });
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
 btn.textContent='▸ CLEANING '+safeIdx.length+' ITEMS...';btn.style.color='var(--gold)';
 fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({indices:safeIdx,dry_run:false})
 }).then(r=>r.json()).then(function(d){
  btn.textContent='✓ FREED '+d.freed_human+' ('+d.cleaned+' items)';btn.style.color='var(--ok)';
  /* Reload after 2s to show updated state */
  setTimeout(function(){switchView('scan')},2000);
 }).catch(function(e){btn.textContent='✗ Error: '+e.message;btn.style.color='var(--danger)'});
}

function cleanIdx(el,idx){
 el.textContent='...';
 fetch('/api/clean',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({indices:[idx],dry_run:false})
 }).then(r=>r.json()).then(function(d){
  if(d.cleaned>0){el.textContent='✓ '+d.freed_human;el.style.color='var(--ok)'}
  else{el.textContent='skip';el.style.color='var(--dim)'}
 }).catch(function(){el.textContent='err';el.style.color='var(--danger)'});
}

function viewFleet(){
 out('⚑ Fleet — every lane, live','t-gold');
 fetch('/api/fleet').then(function(r){
  if(!r.ok)return r.json().then(function(e){throw new Error(e.error||('HTTP '+r.status))});
  return r.json()}).then(function(d){
  const s=d.summary||{};
  out('');
  // Summary tiles. Percent is stated WITH its numerator and denominator so a
  // number can never be read without the count it came from.
  out('  COMPLETED / IN FLIGHT    '+s.done+' / '+s.total+'   ('+s.pct_done+'% done, '+s.in_flight+' still in flight)','t-head');
  out('  IN PROGRESS / ASSIGNED   '+s.active+' / '+(s.active+s.assigned)+'   ('+s.assigned+' assigned but not started)','t-head');
  out('  STALLED / BLOCKED        '+(s.stalled+s.blocked)+'   ('+s.stalled+' stalled · '+s.blocked+' blocked · '+s.idle_lanes+' idle lanes)','t-head');
  sep();
  out('  ACTIVITY — real status changes, in order, as they happen','t-dim');
  const acts=d.activity||[];
  if(!acts.length){
   // A seeded tracker with no events means genuine quiet; an unseeded one
   // means no baseline yet. Saying "no activity" for the second is a lie.
   out(d.seeded?'  (no status changes since this board started)':'  (baseline being taken — changes appear from the next poll)','t-dim');
  } else {
   acts.forEach(function(e){
    const row=document.createElement('div');row.className='t-line t-row';
    const at=document.createElement('span');at.className='t-col';at.style.width='90px';at.style.color='var(--dim)';at.textContent=e.at;
    const ag=document.createElement('span');ag.className='t-col';ag.style.width='170px';ag.style.color='var(--dim)';ag.textContent=e.agent;
    const sub=document.createElement('span');sub.className='t-col';sub.style.flex='1';sub.textContent=e.task_id+' — '+(e.subject||'');
    const tr=document.createElement('span');tr.className='t-col-r';
    tr.style.color=(e.to==='done')?'var(--ok)':(e.to==='blocked')?'var(--warn)':'var(--gold)';
    tr.textContent=e.from+' → '+e.to;
    row.appendChild(at);row.appendChild(ag);row.appendChild(sub);row.appendChild(tr);T.appendChild(row)});
  }
  sep();
  out('  APPLICATIONS — every lane, active first','t-dim');
  out('  '+s.lanes_working+' of '+s.lanes_total+' lanes actively working','t-dim');
  (d.lanes||[]).forEach(function(l){
   const row=document.createElement('div');row.className='t-line t-row';
   const ag=document.createElement('span');ag.className='t-col';ag.style.width='210px';ag.style.whiteSpace='nowrap';ag.textContent=l.agent;
   const st=document.createElement('span');st.className='t-col';st.style.width='200px';st.style.whiteSpace='nowrap';
   // Every state maps explicitly. A benign default turned a lane with 24 open
   // items into "stopped — no open work" the moment the vocabulary grew.
   const LBL={WORKING:'WORKING',ASSIGNED:'assigned — claimed',IDLE_WITH_WORK:'IDLE — work waiting',
              BLOCKED:'blocked',UNROUTABLE:'UNROUTABLE',COMPLETE:'complete — no open work'};
   const CLR={WORKING:'var(--ok)',ASSIGNED:'var(--ok)',IDLE_WITH_WORK:'var(--warn)',
              BLOCKED:'var(--warn)',UNROUTABLE:'var(--err)',COMPLETE:'var(--dim)'};
   st.style.color=CLR[l.state]||'var(--err)';
   st.textContent=LBL[l.state]||('unknown state: '+l.state);
   const cts=document.createElement('span');cts.className='t-col';cts.style.flex='1';cts.style.color='var(--dim)';
   let parts=[l.open+' open'];
   if(l.inbox)parts.push(l.inbox+' inbox');
   if(l.active)parts.push(l.active+' active');
   if(l.stalled)parts.push(l.stalled+' stalled');
   if(l.blocked)parts.push(l.blocked+' blocked');
   if(l.touched_ago)parts.push('touched '+l.touched_ago);
   cts.textContent=parts.join(' · ');
   row.appendChild(ag);row.appendChild(st);row.appendChild(cts);T.appendChild(row)});
 }).catch(function(e){out('  fleet board unavailable: '+e.message,'t-err')});
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
    const path=document.createElement('span');path.className='t-col';path.style.flex='1';path.style.color='var(--dim)';path.textContent=r.path;
    const size=document.createElement('span');size.className='t-col-r';size.textContent=fmtSize(r.size_bytes);
    row.appendChild(type);row.appendChild(path);row.appendChild(size);T.appendChild(row)});
   const cleanRow=document.createElement('div');cleanRow.className='t-line';
   const act=document.createElement('span');act.className='t-action';act.textContent='[clean all residuals]';
   act.addEventListener('click',function(){
    act.textContent='cleaning...';
    fetch('/api/ghosts/clean',{method:'POST',headers:{'Content-Type':'application/json'},
     body:JSON.stringify({app_name:g.app_name,dry_run:false})
    }).then(r=>r.json()).then(function(d){
     act.textContent='✓ freed '+d.freed_human;act.style.color='var(--ok)'
    }).catch(function(){act.textContent='error';act.style.color='var(--danger)'})});
   cleanRow.appendChild(act);T.appendChild(cleanRow)});
 }).catch(function(e){out('Error: '+e.message,'t-err')});
}

function viewGuard(){
 out('🛡 Guard — System Monitor','t-gold');
 out('');out('Running diagnostics...','t-dim');
 /* Lowercase keys only — /api/doctor marshals guard.DoctorReport through its
    json tags (score/findings/check/severity/message). This block used to read
    the Go field names instead, so the score rendered "undefined/100" and the
    findings list fell through ||[] and silently discarded EVERY diagnostic:
    16 real findings shown as none, on the one screen whose whole job is to
    tell you something is wrong. Pinned by TestGuardView_ReadsDoctorJSONKeys… */
 fetch('/api/doctor').then(r=>r.json()).then(function(rpt){
  out('  Health Score: '+rpt.score+'/100','t-head');sep();
  const fs=rpt.findings||[];
  if(!fs.length){out('  No diagnostics returned.','t-dim')}
  fs.forEach(function(f){
   const icon=({0:'✅',1:'ℹ️',2:'⚠️',3:'🔴'}[f.severity]||'⚪');
   out('  '+icon+' '+f.check+' — '+f.message)});
  sep();out('');
  out('Process Slayer — type: kill node | kill electron | kill docker | kill lsp | kill build | kill ai','t-dim');
  out('Deprioritize — type: deprioritize (safe, reversible — lowers background process priority)','t-dim');
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

/* Ra fleet orchestration has no backend yet — say so plainly instead of
   dead-ending on fetches that can never succeed. Plain info, no alarm
   styling: nothing here is fixable by the user, so nothing may alarm. */
function viewRa(){
 out('𓇶 Ra — Fleet Orchestration','t-gold');
 out('');
 out('  Fleet orchestration — coming with the Ra backend.','t-out');
 out('');
 out('  Ra will balance work across your machines: each node reports its','t-dim');
 out('  capacity (RAM, GPU, pressure) and Ra deploys builds where they fit.','t-dim');
 out('');
 out('  This tab will light up when the backend ships. Nothing to configure','t-dim');
 out('  or fix here today.','t-dim');
}

/* ── Command input ────────────────────────────────────── */
const input=document.getElementById('term-input');

/* Typing Enter and clicking an affordance both land here. Keeping ONE dispatch
   means a clickable row can never drift from what the typed word does. */
input.addEventListener('keydown',function(e){
 if(e.key!=='Enter')return;
 const raw=this.value.trim();this.value='';if(!raw)return;
 exec(raw);
});

function exec(raw){
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

 if(raw==='renice'||raw==='renice lsp'||raw==='deprioritize'||raw==='deprioritize lsp'){
  out('▸ Deprioritize background processes (safe, reversible)','t-gold');
  fetch('/api/guard/renice?target=lsp',{method:'POST'}).then(r=>r.json()).then(function(d){
   if(d.reniced>0){out('✓ Deprioritized '+d.reniced+' background processes (safe, reversible)','t-ok');
    (d.processes||[]).forEach(function(p){out('  PID '+p.pid+' '+p.name+' — '+p.rss_human,'t-dim')})}
   else out('No background processes found to deprioritize','t-dim');
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
 /* Not a command — treat it as a question about this machine. The bar reads
    like a prompt, so operators type questions at it; answering "Unknown
    command" was the surface refusing an affordance it visibly offers. The
    answer is grounded in this workstation's live diagnostics and never leaves
    loopback. */
 if(!key){ask(raw);return}
 running=true;
 fetch('/api/run?cmd='+key,{method:'POST'}).then(function(r){
  if(!r.ok)return r.json().then(function(e){throw new Error(e.error)});
 }).catch(function(e){out('✗ '+e.message,'t-err');running=false});
}

/* Natural-language question about this machine, answered by the LOCAL engine
   from this machine's live diagnostics. Every failure is stated plainly: a
   made-up answer here would be indistinguishable from a real one. */
let asking=false;
function ask(q){
 if(asking){out('Still answering the previous question.','t-err');return}
 asking=true;
 out('');out('▸ '+q,'t-gold');
 out('  asking the local engine (nothing leaves this machine)…','t-dim');
 fetch('/api/ask',{method:'POST',headers:{'Content-Type':'application/json'},
  body:JSON.stringify({question:q})})
  .then(function(r){return r.json().then(function(d){
   if(!r.ok)throw new Error(d.error||r.statusText);return d})})
  .then(function(d){
   /* Findings are rendered by the SERVER from its own diagnostic data — the
      model only chose which ones. Nothing here was written by the model, so
      no name, path or number on screen can have been paraphrased. */
   if(d.summary)out('  '+d.summary);
   (d.findings||[]).forEach(function(l){out('  '+l)});
   if(d.dropped)out('  ('+d.dropped+')','t-dim');
   if(!d.summary&&!(d.findings||[]).length)out('  Nothing in the current diagnostics answers that. Try "doctor".','t-dim');
   out('  — findings quoted verbatim from this machine; selection by '+d.model,'t-dim');
  })
  .catch(function(e){
   out('✗ '+e.message,'t-err');
   out('  Commands that always work: scan, ghosts, doctor, guard, network, hardware, quality, dedup, kill <target>, deprioritize','t-dim');
  })
  .finally(function(){asking=false});
}

/* A terminal line that runs its own command when clicked. The home screen used
   to print these as inert text, so the only way to act on a listed capability
   was to retype it by hand — the surface named eight things it could do and
   afforded none of them. */
function cmdRow(cmd,desc){
 const row=document.createElement('div');
 row.className='t-line t-cmd';
 row.tabIndex=0;row.setAttribute('role','button');
 const name=document.createElement('span');name.className='t-cmd-name';name.textContent=cmd;
 const d=document.createElement('span');d.className='t-cmd-desc';d.textContent=desc;
 row.appendChild(name);row.appendChild(d);
 const go=function(){input.value='';exec(cmd)};
 row.addEventListener('click',go);
 row.addEventListener('keydown',function(e){
  if(e.key==='Enter'||e.key===' '){e.preventDefault();go()}});
 T.appendChild(row);T.scrollTop=T.scrollHeight;
}

/* Stat tiles that map to a real destination act like the command they stand
   for. Git and Deities have no view to open, so they stay plain readouts —
   deliberately, rather than half-wiring every tile. */
document.querySelectorAll('.stat-go').forEach(function(tile){
 const go=function(){exec(tile.dataset.cmd)};
 tile.addEventListener('click',go);
 tile.addEventListener('keydown',function(e){
  if(e.key==='Enter'||e.key===' '){e.preventDefault();go()}});
});

/* Land ready to type. Without this the first keystroke goes nowhere and the
   page reads as inert. */
input.focus();
window.addEventListener('focus',function(){input.focus()});

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
	fmt.Fprint(w, pageShell("Horus", "home", body, s.cfg.Port))
}

// ── Page Redirects (all views are SPA now) ─────────────────────────────
// These handlers exist so direct URLs like /scan still work.
// They redirect to the SPA with the view pre-selected via JS.

func spaRedirect(w http.ResponseWriter, r *http.Request, view string) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<script>location.replace('/');setTimeout(function(){switchView('%s')},100)</script>`, view)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request)   { spaRedirect(w, r, "scan") }
func (s *Server) handleGhosts(w http.ResponseWriter, r *http.Request) { spaRedirect(w, r, "ghosts") }
func (s *Server) handleGuard(w http.ResponseWriter, r *http.Request)  { spaRedirect(w, r, "guard") }
func (s *Server) handleHorus(w http.ResponseWriter, r *http.Request)  { spaRedirect(w, r, "horus") }
func (s *Server) handleVault(w http.ResponseWriter, r *http.Request)  { spaRedirect(w, r, "vault") }

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
