package dashboard

import (
	"encoding/json"
	"net/http"
)

func (s *Server) apiMercury(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.MercuryFn == nil {
		http.Error(w, "Mercury reader unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.cfg.MercuryFn(r.Context()))
}

const mercuryCSS = `
.mercury{font-family:Saira,-apple-system,system-ui,sans-serif;font-variant-numeric:tabular-nums;max-width:1120px;margin:auto;color:var(--ink);word-break:normal}
.mercury h2{font-size:22px;font-weight:500;margin:8px 0}.mercury .mc-sub{color:var(--dim);font-size:12px;margin:6px 0 22px}
.mercury .mc-hero{display:grid;grid-template-columns:repeat(3,minmax(0,300px));justify-content:center;gap:40px;padding:24px 0}
.mercury .mc-num{font-size:clamp(24px,4.4vw,60px);font-weight:300;line-height:1.15;white-space:nowrap;letter-spacing:-.03em}
.mercury .mc-num small{font-size:12px;letter-spacing:0}.mercury .mc-label{margin:8px 0 12px;font-size:12px;font-weight:600;letter-spacing:.18em}.mercury .mc-context{font-size:12px;min-height:36px;color:var(--dim);white-space:pre-line;letter-spacing:.14em;text-transform:uppercase;font-weight:600}.mercury .mc-hctx{text-align:center;color:#c6e6d8;font-size:13px;letter-spacing:.16em}.mercury .mc-run-status{text-align:center;font-size:15px;font-weight:500}.mercury .mc-unlit{color:#9cc7b666}
.mercury .mc-out{color:#34d399}.mercury .mc-in{color:#E3C878}.mercury .mc-nodes{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:16px;margin:25px 0}
.mercury .mc-card{border:1px solid var(--line);border-radius:12px;background:var(--panel);padding:20px;min-width:0}.mercury .mc-card h3{font-size:18px;font-weight:500}.mercury .mc-card p{font-size:12px;margin:8px 0;color:var(--dim)}
.mercury .mc-hardware{height:80px;border:2px dashed #6ee7b780;border-radius:8px;display:flex;align-items:center;justify-content:center;color:#6ee7b7;margin-bottom:18px;font-size:26px;background:linear-gradient(145deg,#203832,#14261f)}
.mercury .mc-link{padding:16px 0;border-top:1px solid var(--line);font-size:13px}.mercury .mc-link p{margin:7px 0;color:var(--dim);font-size:12px}.mercury .mc-lamps{display:flex;flex-wrap:wrap;gap:12px;font-size:12px;color:#E3C878;margin:16px 0}.mercury .mc-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px}.mercury button{padding:8px 14px;background:var(--panel);border:1px solid var(--line);border-radius:7px;color:var(--ink);cursor:pointer}
@media(max-width:650px){.mercury .mc-hero{gap:12px}.mercury .mc-label{font-size:11px}.mercury .mc-nodes{grid-template-columns:1fr}.mercury .mc-num{font-size:28px}.mercury .mc-num small{display:block;font-size:10px}}
.mercury .mc-scene-scroll{overflow-x:auto;margin:20px 0}.mercury .mc-scene{display:block;min-width:1000px;width:100%;height:auto;background:#000}.mercury .mc-strips{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:18px;margin-bottom:24px}.mercury .mc-strip{border-top:1px solid #34d39948;padding-top:12px}.mercury .mc-strip strong{font-size:24px;font-weight:500}.mercury .mc-strip p{font-size:12px;color:#9cc7b6}.mercury .mc-strip span{display:block;font-size:12px;margin-top:6px}.mercury .mc-scene text{font-family:-apple-system,system-ui,sans-serif;font-variant-numeric:tabular-nums}
@media(prefers-reduced-motion:reduce){.mercury .mc-pulse{display:none}}
`

const mercuryJS = `

(function(){
 const finite=v=>typeof v==='number'&&Number.isFinite(v);
 function envelope(read,wall,elapsed){
  const d=read.snapshot;if(!['ok','partial'].includes(read.state)||!d||!finite(d.generated_at)||!finite(wall)||!finite(elapsed)||elapsed<0)return false;
  const window=Math.min(6,...d.nodes.filter(n=>n.state==='live'&&finite(n.freshness_s)&&n.freshness_s>0).map(n=>n.freshness_s));
  const age=wall-d.generated_at;return age>=-2&&age+elapsed<=window;
 }
 function node(read,id,wall,elapsed){
  const d=read.snapshot;if(!envelope(read,wall,elapsed))return false;
  const nodes=d.nodes.filter(n=>n.id===id);if(nodes.length!==1)return false;const n=nodes[0];
  if(n.state!=='live'||!finite(n.age_s)||n.age_s< -2||!finite(n.observed_at)||!finite(n.freshness_s)||n.freshness_s<=0)return false;
  const age=wall-n.observed_at, carried=n.age_s+Math.max(0,wall-d.generated_at);
  return age>=-2&&Math.max(age,carried)+elapsed<=n.freshness_s;
 }
 function direction(read,cable,flow,wall,elapsed){
  if(!envelope(read,wall,elapsed)||cable.state!=='up'||typeof flow.rate!=='string'||flow.rate==='— MB/s')return false;
  const links=read.snapshot.links.filter(l=>l.id===cable.id&&l.state==='up');if(links.length!==1)return false;
  const dirs=links[0].directions.filter(f=>f.from===flow.from&&f.to===flow.to);if(dirs.length!==1)return false;
  const d=dirs[0], age=wall-d.observed_at;
  return finite(d.observed_at)&&finite(d.wire_gbs)&&d.wire_gbs>=0&&age>=-2&&age+elapsed<=6&&node(read,d.from,wall,elapsed)&&node(read,d.to,wall,elapsed);
 }
 function aggregate(read,wall,elapsed){
  if(read.state!=='ok'||!envelope(read,wall,elapsed)||!read.snapshot.source||!node(read,read.snapshot.source.host,wall,elapsed))return false;
  return read.snapshot.links.filter(l=>l.state==='up').every(l=>{
   const c=read.scene&&read.scene.cables.find(c=>c.id===l.id);
   return c&&l.directions.length===2&&c.directions.length===2&&c.directions.every(f=>direction(read,c,f,wall,elapsed));
  });
 }
 function completion(read,wall,duration){
  const start=typeof read.read_at==='string'?Date.parse(read.read_at)/1000:NaN;
  if(!finite(start)||!finite(wall)||!finite(duration)||duration<0)return null;
  const bound=Math.max(wall,start+duration+0.001);return finite(bound)?bound:null;
 }
 function state(read,wall,elapsed){return ['ok','partial'].includes(read.state)&&!envelope(read,wall,elapsed)?'STALE':String(read.state||'unavailable').toUpperCase()}
 window.mercuryReadEligibility={envelope,node,direction,aggregate,state,completion};
})();
window.mountMercury=function(host){
 let stopped=false,timer=null,controller=null,lastRead=null,receivedWall=0,receivedMono=0;
 const shell=document.createElement('section');shell.className='mercury';shell.setAttribute('aria-label','Mercury interconnect');host.appendChild(shell);
 const el=(tag,text,cls)=>{const e=document.createElement(tag);e.textContent=text;if(cls)e.className=cls;return e};
 const rate=v=>v==null?'—':v.toLocaleString(undefined,{maximumFractionDigits:3});
 const percent=v=>v==null?'Unknown':v.toLocaleString(undefined,{maximumFractionDigits:0})+'%';
 function scene(d,projection,stale,read,wall,elapsed){
  const ns='http://www.w3.org/2000/svg';
  const svg=(tag,attrs,text)=>{const e=document.createElementNS(ns,tag);Object.entries(attrs||{}).forEach(([k,v])=>e.setAttribute(k,String(v)));if(text!=null)e.textContent=text;return e};
  const drawing=svg('svg',{viewBox:'0 0 '+projection.width+' '+projection.height,role:'img','aria-label':'Mercury Mac topology; cable endpoints are card edges, not physical ports',class:'mc-scene'});
  const ink='#eef3ee',mint='#6ee7b7',out='#34d399',incoming='#E3C878';
  const color=v=>v==='out'?out:v==='in'?incoming:'#9cc7b6';
  const label=(x,y,text,size,fill,weight)=>svg('text',{x,y,'text-anchor':'middle',fill:fill||ink,'font-size':size||12,'font-weight':weight||400},text);
  projection.cables.forEach(c=>{
   const bend=Math.min(36,(c.x2-c.x1)/3),missing=c.state==='missing',down=c.state!=='up';
   const path=missing?'M'+c.x1+','+c.y1+' L'+c.x2+','+c.y2:'M'+c.x1+','+c.y1+' C'+(c.x1+bend)+','+c.y1+' '+(c.x1+bend)+','+c.lane+' '+(c.x1+bend)+','+c.lane+' L'+(c.x2-bend)+','+c.lane+' C'+(c.x2-bend)+','+c.lane+' '+(c.x2-bend)+','+c.y2+' '+c.x2+','+c.y2;
   drawing.append(svg('path',{d:path,fill:'none',stroke:out,'stroke-opacity':down?.12:.2,'stroke-width':10,'stroke-linecap':'round','stroke-dasharray':down?'5 6':''}),svg('path',{d:path,fill:'none',stroke:down?'#9cc7b6':out,'stroke-opacity':.6,'stroke-width':1.5,'stroke-dasharray':down?'5 6':''}));
   c.directions.filter(f=>f.speed>0&&window.mercuryReadEligibility.direction(read,c,f,wall,elapsed)).forEach(f=>{for(let i=0;i<12;i++){
    const dot=svg('circle',{r:2,fill:color(f.color),class:'mc-pulse'});
    dot.append(svg('animateMotion',{path,dur:(1000/f.speed)+'s',begin:(-i/12*1000/f.speed)+'s',repeatCount:'indefinite',keyPoints:f.forward?'0;1':'1;0',keyTimes:'0;1',calcMode:'linear'}));drawing.append(dot);
   }});
   const text=c.letter,x=(c.x1+c.x2)/2;
   drawing.append(svg('rect',{x:x-12,y:c.lane-13,width:24,height:26,rx:6,fill:'#000',stroke:incoming}),label(x,c.lane+5,text,14,incoming,600));if(missing)drawing.append(label(x,c.lane+30,c.iface,10,'#9cc7b6'));
  });
  projection.cards.forEach(c=>{
   const n=d.nodes.find(n=>n.id===c.id);if(!n)return;
   const ghost=['unreachable','expired'].includes(n.state),muted=!window.mercuryReadEligibility.node(read,n.id,wall,elapsed),g=svg('g',{transform:'translate('+c.x+' '+c.y+')',opacity:muted?.65:1});
   const attrs={fill:'#132a22',stroke:ghost?'#ff6b6b':'#34d399','stroke-opacity':.5,'stroke-width':1.3,'stroke-dasharray':ghost||c.family==='generic'?'6 5':''};
   const rect=(x,y,w,h,r)=>svg('rect',Object.assign({},attrs,{x,y,width:w,height:h,rx:r}));
   if(c.family==='laptop'){
    g.append(rect(0,0,c.width,c.height*.49,15),rect(0,c.height*.5,c.width,c.height*.48,0));
    for(let row=0;row<4;row++)for(let col=0;col<13;col++)g.append(svg('rect',{x:20+col*(c.width-40)/13,y:c.height*.54+row*10,width:(c.width-40)/13-2,height:8,rx:1,fill:'#0a1914',stroke:'#34d39933'}));
    g.append(rect(c.width*.28,c.height*.77,c.width*.44,c.height*.18,4));
   }else if(c.family==='imac')g.append(rect(0,0,c.width,c.height*.85,15),rect(c.width*.43,c.height*.85,c.width*.14,c.height*.13,0));
   else{g.append(rect(0,0,c.width,c.height,['studio','mini'].includes(c.family)?24:12));if(['studio','mini'].includes(c.family))g.append(svg('path',{d:'M0,'+c.height*.87+' H'+c.width,stroke:'#34d39980'}));}
   const metrics=svg('g',{opacity:ghost?.25:1});
   metrics.append(label(c.width/2,49,'CPU',12,'#9cc7b6',600),label(c.width/2,94,percent(muted?null:n.cpu.busy_pct),44));
   if(c.family==='laptop')metrics.append(svg('rect',{x:c.width*.13,y:c.height*.64-18,width:c.width*.74,height:72,rx:6,fill:'#000',opacity:.7}));
   metrics.append(label(c.width/2,c.height*.64,n.gpu.calibration.status==='calibrated'?'GPU':'GPU · '+n.gpu.calibration.status,12,'#9cc7b6',600),label(c.width/2,c.height*.64+46,!muted&&n.gpu.calibration.status==='calibrated'?percent(n.gpu.work_pct):'—',44));g.append(metrics);
   g.append(label(c.width/2,c.height+30,n.id,22,ink,600),label(c.width/2,c.height+47,(muted?'Last reported · ':'')+n.state,11,'#9cc7b6'));
   drawing.append(g);
  });
  const scroll=el('div','','mc-scene-scroll');scroll.append(drawing);shell.append(scroll);
  const strips=el('div','','mc-strips');projection.cables.forEach(c=>{const cell=el('div','','mc-strip');const complete=c.directions.length===2&&c.directions.every(f=>window.mercuryReadEligibility.direction(read,c,f,wall,elapsed));cell.append(el('strong',c.letter+'  '+(complete?c.rate.number+' '+c.rate.unit:'Unknown')),el('p',c.state==='missing'?c.iface+' · Missing peer':c.state));c.directions.forEach(f=>cell.append(el('span',(f.forward?'▶ ':'◀ ')+(window.mercuryReadEligibility.direction(read,c,f,wall,elapsed)?f.rate:'Unknown'),f.color==='out'?'mc-out':f.color==='in'?'mc-in':'')));strips.append(cell)});shell.append(strips);
 }
 function draw(read){
  const wall=receivedWall,elapsed=Math.max(0,(performance.now()-receivedMono)/1000),eligible=window.mercuryReadEligibility;
  const envelope=eligible.envelope(read,wall,elapsed),complete=eligible.aggregate(read,wall,elapsed);
  const priorDetails=shell.querySelector('details'),detailsOpen=!!(priorDetails&&priorDetails.open),focused=document.activeElement,focusRefresh=!!focused&&focused===shell.querySelector('.mc-refresh'),focusSummary=!!focused&&!!priorDetails&&focused===priorDetails.querySelector('summary');
  shell.replaceChildren();const bar=el('div','','mc-toolbar');bar.append(el('h2','Mercury — Interconnect'));const refresh=el('button','Refresh','mc-refresh');refresh.onclick=()=>{clearTimeout(timer);if(controller)controller.abort();poll()};bar.append(refresh);shell.append(bar);
  const status=el('p',(eligible.state(read,wall,elapsed)+' · '+(read.message||'Mercury unavailable')),'mc-sub');status.setAttribute('role','status');shell.append(status);
  const d=read.snapshot?Object.assign({},read.snapshot):null;if(!d){shell.append(el('p','Connect and configure a private Mercury dashboard v1 monitor, then refresh.','mc-sub'));if(focusRefresh)refresh.focus({preventScroll:true});return}
  const h=read.headline||{wire:{number:'—',unit:'',note:''},compressed:{number:'—',unit:'',note:''},gain:{number:'—',unit:'',note:''},context:'Rates unavailable',lamps:[],cables:0,node_order:[]};
  const currentLamps=(Array.isArray(h.lamps)?h.lamps:[]).map(v=>({label:v.label,on:complete&&v.on})),lampsKnown=complete&&(Array.isArray(h.lamps)||h.lamps===null);
  const lamps=el('div','','mc-lamps');lamps.setAttribute('aria-label',lampsKnown?'Current activity':'Current activity unavailable');currentLamps.forEach(v=>lamps.append(el('span',(complete?(v.on?'● ':'○ '):'? ')+v.label,v.on?'':'mc-unlit')));if(!lampsKnown&&!Array.isArray(h.lamps))lamps.append(el('span','ACTIVITY LAMPS UNKNOWN'));lamps.append(el('span',complete?h.cables+' CABLES':'ACTIVE CABLES UNKNOWN'));shell.append(lamps);
  shell.append(el('p',complete?h.context:envelope?'Rates unavailable · current aggregate is incomplete':'Rates paused · retained snapshot is no longer current','mc-context mc-hctx'));
  const hero=el('div','','mc-hero');[[complete?h.wire:{number:'—',unit:'',note:''},'WIRE',''],[complete?h.compressed:{number:'—',unit:'',note:''},'COMPRESSED',h.compressed.number==='—'?'':'mc-in'],[complete?h.gain:{number:'—',unit:'',note:''},'GAIN','']].forEach(([v,label,color])=>{const c=el('div');const n=el('div',v.number,'mc-num '+color);n.append(el('small',' '+v.unit));c.append(el('div',label,'mc-label '+color),n,el('div',v.note,'mc-context'));hero.append(c)});shell.append(hero);
  if(h.run_status)shell.append(el('p','Source run at snapshot: '+h.run_status,'mc-run-status'));
  const ordered=(h.node_order||[]).map(id=>d.nodes.find(n=>n.id===id)).filter(Boolean);if(ordered.length===d.nodes.length)d.nodes=ordered;
  if(read.scene)scene(d,read.scene,!envelope,read,wall,elapsed);else {
  const nodes=el('div','','mc-nodes');d.nodes.forEach(n=>{const card=el('article','','mc-card');card.append(el('div',n.model||'Mac','mc-hardware'),el('h3',n.name),el('p',n.chip||'Hardware unknown'),el('p',(read.state==='stale'?'Last reported ':'')+n.state+' · '+(n.age_s==null?'Age unknown':'Age at snapshot '+n.age_s.toFixed(1)+'s')),el('p','GPU work '+percent(eligible.node(read,n.id,wall,elapsed)?n.gpu.work_pct:null)+' · '+n.gpu.calibration.status),el('p','CPU busy '+percent(eligible.node(read,n.id,wall,elapsed)?n.cpu.busy_pct:null)),el('p','Decode · '+(n.decode.mode||'Unknown')),el('p','Seal · '+(n.seal.state||'Unknown')));nodes.append(card)});shell.append(nodes);}
  const details=el('details'),summary=el('summary','Retained measurement details · source observations');details.open=detailsOpen;details.append(summary);
  d.nodes.forEach(n=>details.append(el('p',n.name+' · '+(n.model||'Unknown model')+' · '+(n.chip||'Unknown chip')+' · GPU calibration '+n.gpu.calibration.status+' · '+n.gpu.calibration.source+' · Decode '+(n.decode.mode||'Unknown')+' · Seal '+(n.seal.state||'Unknown')+' · '+(n.age_s==null?'Age unknown':'Age at snapshot '+n.age_s.toFixed(1)+'s'),'mc-sub')));
  d.links.forEach(l=>{const row=el('article','','mc-link');row.append(el('strong',l.ends.map(v=>v?v.node+' · '+v.iface:'Missing peer').join(' ↔ ')),el('p',l.state+' · Physical port position unknown'));l.directions.forEach(v=>row.append(el('p',v.from+' → '+v.to+'   '+rate(v.wire_gbs)+' GB/s wire · '+rate(v.payload_gbs)+' payload · '+rate(v.compressed_gbs)+' compressed · '+(v.provenance||'Unknown provenance')+' · observed '+(v.observed_at==null?'Unknown':new Date(v.observed_at*1000).toLocaleTimeString()))));details.append(row)});shell.append(details);
  if(!d.nodes.length)shell.append(el('p','Waiting for the first Mercury sample.','mc-sub'));
  shell.append(el('p','Monitor '+d.source.host+' · build '+d.source.build+' · generation '+d.generation+' · observed '+new Date(d.generated_at*1000).toLocaleTimeString()+' · reader timestamp '+read.read_at+' · Decimal GB/s','mc-sub'));if(focusRefresh)refresh.focus({preventScroll:true});else if(focusSummary)summary.focus({preventScroll:true});
 }
 async function poll(){
  if(stopped)return;controller=new AbortController();const active=controller;const started=performance.now(),timeout=setTimeout(()=>active.abort(),2500);
  try{const response=await fetch('/api/mercury',{signal:active.signal,cache:'no-store'});if(!response.ok)throw Error('unavailable');const read=await response.json();if(!stopped&&active===controller){lastRead=read;receivedMono=performance.now();receivedWall=window.mercuryReadEligibility.completion(read,Date.now()/1000,Math.max(0,(receivedMono-started)/1000));draw(read)}}
  catch(e){if(!stopped&&active===controller){lastRead={state:'offline',message:'Mercury reader is unavailable. Check the monitor and refresh.'};draw(lastRead)}}
  finally{clearTimeout(timeout);if(!stopped&&active===controller)timer=setTimeout(poll,1000)}
 }
 const clock=setInterval(()=>{if(!stopped&&lastRead)draw(lastRead)},1000);
 draw({state:'loading',message:'Reading Mercury…'});poll();
 const stop=()=>{stopped=true;clearTimeout(timer);clearInterval(clock);if(controller)controller.abort()};
 stop.refresh=()=>{if(stopped)return;clearTimeout(timer);if(controller)controller.abort();poll()};return stop;
};
`
