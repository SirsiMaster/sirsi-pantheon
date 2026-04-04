// Seba is the keeper of stars and the mapper of the celestial/digital vault.
package seba

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

// NodeType categorizes nodes in the infrastructure graph.
type NodeType string

const (
	NodeDevice    NodeType = "device"
	NodeApp       NodeType = "app"
	NodeGhost     NodeType = "ghost"
	NodeContainer NodeType = "container"
	NodeProcess   NodeType = "process"
	NodeCache     NodeType = "cache"
	NodeVolume    NodeType = "volume"
	NodeNetwork   NodeType = "network"
	NodeService   NodeType = "service"
)

// Node represents a single entity in the infrastructure graph.
type Node struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	Type     NodeType          `json:"type"`
	Size     int64             `json:"size,omitempty"` // bytes
	Color    string            `json:"color,omitempty"`
	X        float64           `json:"x"`
	Y        float64           `json:"y"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Edge represents a relationship between two nodes.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Weight int    `json:"weight,omitempty"`
}

// InfraGraph is the complete infrastructure map.
type InfraGraph struct {
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges"`
	GeneratedAt string `json:"generated_at"`
	Hostname    string `json:"hostname"`
	Platform    string `json:"platform"`
}

// NodeColors maps node types to display colors.
var NodeColors = map[NodeType]string{
	NodeDevice:    "#C8A951", // Anubis gold
	NodeApp:       "#4A90D9", // Blue
	NodeGhost:     "#E74C3C", // Red — ghosts glow red
	NodeContainer: "#2ECC71", // Green
	NodeProcess:   "#F39C12", // Orange
	NodeCache:     "#95A5A6", // Gray
	NodeVolume:    "#8E44AD", // Purple
	NodeNetwork:   "#1ABC9C", // Teal
	NodeService:   "#3498DB", // Light blue
}

// NewGraph creates an empty infrastructure graph.
func NewGraph() *InfraGraph {
	hostname, _ := os.Hostname()
	return &InfraGraph{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Hostname:    hostname,
		Platform:    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// AddNode adds a node to the graph.
func (g *InfraGraph) AddNode(id, label string, nodeType NodeType) {
	color, ok := NodeColors[nodeType]
	if !ok {
		color = "#CCCCCC"
	}
	g.Nodes = append(g.Nodes, Node{
		ID:    id,
		Label: label,
		Type:  nodeType,
		Color: color,
	})
}

// AddEdge connects two nodes.
func (g *InfraGraph) AddEdge(source, target, label string) {
	id := fmt.Sprintf("%s->%s", source, target)
	g.Edges = append(g.Edges, Edge{
		ID:     id,
		Source: source,
		Target: target,
		Label:  label,
	})
}

// ToJSON exports the graph as JSON.
func (g *InfraGraph) ToJSON() ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}

// RenderHTML produces a self-contained HTML file with a kinetic infrastructure graph.
// Pure Canvas + JavaScript — ZERO external dependencies.
// Features: data pulses, breathing nodes, ghost shimmer, bezier edges, particles.
func (g *InfraGraph) RenderHTML(outputPath string) error {
	graphJSON, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("marshal graph: %w", err)
	}

	colorsJSON := mustJSON(NodeColors)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>𓁢 Seba — %s</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{background:#06060F;color:#C8A951;font-family:-apple-system,system-ui,sans-serif;overflow:hidden;user-select:none}
canvas{display:block;cursor:crosshair}
#hdr{position:fixed;top:0;left:0;right:0;z-index:10;background:linear-gradient(180deg,rgba(6,6,15,.96) 0%%,rgba(6,6,15,.5) 60%%,transparent 100%%);padding:22px 28px 36px;pointer-events:none}
#hdr h1{font-size:18px;font-weight:300;letter-spacing:3px;text-transform:uppercase;color:#C8A951;text-shadow:0 0 20px rgba(200,169,81,.4)}
#hdr p{font-size:10px;color:#444;margin-top:5px;letter-spacing:1px}
.panel{background:rgba(6,6,15,.88);border:1px solid rgba(200,169,81,.12);border-radius:12px;backdrop-filter:blur(12px);box-shadow:0 8px 32px rgba(0,0,0,.5)}
#leg{position:fixed;bottom:20px;left:20px;z-index:10;padding:16px 20px;font-size:11px}
.lt{color:#C8A951;font-weight:600;margin-bottom:8px;font-size:10px;letter-spacing:2px;text-transform:uppercase}
.li{display:flex;align-items:center;margin:5px 0;color:#666}
.ld{width:7px;height:7px;border-radius:50%%;margin-right:9px}
#sta{position:fixed;top:20px;right:20px;z-index:10;padding:16px 20px;text-align:right}
.sl{color:#555;letter-spacing:1px;text-transform:uppercase;font-size:8px}
.sv{color:#C8A951;font-weight:600;font-size:16px}
#tip{position:fixed;display:none;z-index:20;background:rgba(6,6,15,.96);border:1px solid rgba(200,169,81,.35);border-radius:10px;padding:12px 16px;font-size:12px;pointer-events:none;max-width:300px;box-shadow:0 8px 40px rgba(200,169,81,.08);backdrop-filter:blur(16px)}
.tn{color:#C8A951;font-weight:600;font-size:13px}
.tt{color:#555;margin-top:2px;font-size:10px;text-transform:uppercase;letter-spacing:1px}
.tm{color:#444;margin-top:5px;font-size:10px;line-height:1.5}
#ctl{position:fixed;bottom:20px;right:20px;z-index:10;display:flex;gap:6px}
#ctl button{background:rgba(6,6,15,.88);border:1px solid rgba(200,169,81,.12);border-radius:10px;padding:9px 13px;color:#555;cursor:pointer;font-size:12px;transition:all .3s}
#ctl button:hover{border-color:rgba(200,169,81,.4);color:#C8A951}
#fi{position:fixed;bottom:72px;left:50%%;transform:translateX(-50%%);z-index:10;background:rgba(6,6,15,.92);border:1px solid rgba(200,169,81,.25);border-radius:10px;padding:10px 22px;font-size:11px;color:#C8A951;display:none;backdrop-filter:blur(12px);letter-spacing:.5px}
</style>
</head>
<body>
<div id="hdr"><h1>𓁢 Seba — Infrastructure Map</h1><p>%s • %s • %s</p></div>
<canvas id="c"></canvas>
<div id="leg" class="panel"></div>
<div id="sta" class="panel"></div>
<div id="tip"></div>
<div id="fi"></div>
<div id="ctl">
<button onclick="RV()" title="Reset">⟲</button>
<button onclick="TL()" title="Labels">Aa</button>
<button onclick="TP()" title="Pulse" id="bp">◉</button>
</div>
<script>
(function(){
'use strict';
const D=%s,TC=%s;
const C=document.getElementById('c'),X=C.getContext('2d'),tip=document.getElementById('tip'),fi=document.getElementById('fi');
let W,H,dp,sL=true,pE=true,t=0;

function rs(){dp=devicePixelRatio||1;W=innerWidth;H=innerHeight;C.width=W*dp;C.height=H*dp;C.style.width=W+'px';C.style.height=H+'px';X.setTransform(dp,0,0,dp,0,0)}
addEventListener('resize',rs);rs();

// Stars
const ST=Array.from({length:180},()=>({x:Math.random()*3e3-1500,y:Math.random()*3e3-1500,s:Math.random()*1.2+.3,b:Math.random(),sp:.002+Math.random()*.003}));

// Camera with easing
let cm={x:0,y:0,z:1,tx:0,ty:0,tz:1};
function lc(){cm.x+=(cm.tx-cm.x)*.06;cm.y+=(cm.ty-cm.y)*.06;cm.z+=(cm.tz-cm.z)*.06}
function w2s(x,y){return[(x-cm.x)*cm.z+W/2,(y-cm.y)*cm.z+H/2]}
function s2w(x,y){return[(x-W/2)/cm.z+cm.x,(y-H/2)/cm.z+cm.y]}

// Nodes
const N=D.nodes.map((n,i)=>{
  const a=2*Math.PI*i/D.nodes.length,r=160+Math.random()*110;
  return{id:n.id,label:n.label,type:n.type,color:n.color||TC[n.type]||'#C8A951',
    bs:Math.max(4,Math.min(22,Math.log2((n.size||4096)/512)+2)),
    x:Math.cos(a)*r+(Math.random()-.5)*60,y:Math.sin(a)*r+(Math.random()-.5)*60,
    vx:0,vy:0,ph:Math.random()*6.28,da:Math.random()*6.28,ds:.0003+Math.random()*.0008,
    meta:n.metadata||{},sz:n.size||0}
});
const NM={};N.forEach(n=>NM[n.id]=n);
const E=D.edges.filter(e=>NM[e.source]&&NM[e.target]).map(e=>({
  s:NM[e.source],t:NM[e.target],l:e.label||'',
  po:Math.random(),ps:.4+Math.random()*.8,cv:(Math.random()-.5)*.35
}));

// Particles & ripples
const PA=[],RI=[];
function sP(x,y,c){if(PA.length>200)return;PA.push({x,y,vx:(Math.random()-.5)*.7,vy:(Math.random()-.5)*.7,li:1,dc:.01+Math.random()*.015,c,s:1+Math.random()*2})}
function sR(x,y,c){RI.push({x,y,r:0,mr:50+Math.random()*40,a:.5,c})}

// Physics
function sim(){
  for(let i=0;i<N.length;i++)for(let j=i+1;j<N.length;j++){
    const a=N[i],b=N[j];let dx=b.x-a.x,dy=b.y-a.y,d2=dx*dx+dy*dy||1,d=Math.sqrt(d2),f=2800/d2,fx=dx/d*f,fy=dy/d*f;
    a.vx-=fx;a.vy-=fy;b.vx+=fx;b.vy+=fy}
  E.forEach(e=>{let dx=e.t.x-e.s.x,dy=e.t.y-e.s.y,d=Math.sqrt(dx*dx+dy*dy)||1,f=(d-95)*.005;
    e.s.vx+=dx/d*f;e.s.vy+=dy/d*f;e.t.vx-=dx/d*f;e.t.vy-=dy/d*f});
  N.forEach(n=>{n.vx-=n.x*.0007;n.vy-=n.y*.0007;n.da+=n.ds;n.vx+=Math.cos(n.da)*.025;n.vy+=Math.sin(n.da)*.025;
    n.vx*=.93;n.vy*=.93;n.x+=n.vx;n.y+=n.vy})}

// Bezier point
function bp(x1,y1,cx,cy,x2,y2,t){const u=1-t;return[u*u*x1+2*u*t*cx+t*t*x2,u*u*y1+2*u*t*cy+t*t*y2]}

let hv=null,fc=null;

function render(){
  t+=.016;sim();
  // Update particles
  for(let i=PA.length-1;i>=0;i--){const p=PA[i];p.x+=p.vx;p.y+=p.vy;p.li-=p.dc;if(p.li<=0)PA.splice(i,1)}
  for(let i=RI.length-1;i>=0;i--){const r=RI[i];r.r+=1.8;r.a-=.014;if(r.a<=0)RI.splice(i,1)}
  lc();X.clearRect(0,0,W,H);

  // Stars
  ST.forEach(s=>{const[sx,sy]=w2s(s.x,s.y);if(sx<-10||sx>W+10||sy<-10||sy>H+10)return;
    const br=.12+.12*Math.sin(t*s.sp*60+s.b*6.28);X.fillStyle='rgba(200,169,81,'+br+')';
    X.beginPath();X.arc(sx,sy,s.s*cm.z*.4,0,6.28);X.fill()});

  // Edges
  E.forEach(e=>{
    const[x1,y1]=w2s(e.s.x,e.s.y),[x2,y2]=w2s(e.t.x,e.t.y);
    const mx=(x1+x2)/2,my=(y1+y2)/2,dx=x2-x1,dy=y2-y1,ln=Math.sqrt(dx*dx+dy*dy)||1;
    const nx=-dy/ln*e.cv*ln*.5,ny=dx/ln*e.cv*ln*.5,cx=mx+nx,cy=my+ny;
    const ac=fc?(e.s===fc||e.t===fc):hv?(e.s===hv||e.t===hv):false;
    const dm=(fc||hv)&&!ac;

    // Line
    const g=X.createLinearGradient(x1,y1,x2,y2);
    g.addColorStop(0,e.s.color+(ac?'70':'20'));g.addColorStop(1,e.t.color+(ac?'70':'20'));
    X.strokeStyle=g;X.lineWidth=ac?1.8:.5;
    X.beginPath();X.moveTo(x1,y1);X.quadraticCurveTo(cx,cy,x2,y2);X.stroke();

    if(!pE||dm)return;
    // Data pulses
    const isN=e.s.type==='network'||e.t.type==='network';
    const pc=isN?3:1,sp=isN?e.ps*1.6:e.ps;
    for(let p=0;p<pc;p++){
      const pr=((t*sp+e.po+p/pc)%%1);
      const[px,py]=bp(x1,y1,cx,cy,x2,y2,pr);
      const pa=Math.sin(pr*3.14)*(ac?.9:.35);
      const pR=(isN?3:1.8)*cm.z;
      // Pulse glow
      const pg=X.createRadialGradient(px,py,0,px,py,pR*5);
      const ha=Math.min(255,Math.floor(pa*50));
      pg.addColorStop(0,e.s.color+ha.toString(16).padStart(2,'0'));pg.addColorStop(1,'transparent');
      X.fillStyle=pg;X.beginPath();X.arc(px,py,pR*5,0,6.28);X.fill();
      // Dot
      X.fillStyle=isN?'#fff':e.s.color;X.globalAlpha=pa;
      X.beginPath();X.arc(px,py,pR,0,6.28);X.fill();X.globalAlpha=1}

    // Arrowhead for network
    if(isN&&ln>30){const[ax,ay]=bp(x1,y1,cx,cy,x2,y2,.72),[bx,by]=bp(x1,y1,cx,cy,x2,y2,.74);
      const an=Math.atan2(by-ay,bx-ax),az=5*cm.z;
      X.fillStyle='rgba(26,188,156,'+(ac?'.5':'.15')+')';X.beginPath();
      X.moveTo(bx+Math.cos(an)*az,by+Math.sin(an)*az);
      X.lineTo(bx+Math.cos(an+2.5)*az*.6,by+Math.sin(an+2.5)*az*.6);
      X.lineTo(bx+Math.cos(an-2.5)*az*.6,by+Math.sin(an-2.5)*az*.6);X.fill()}
  });

  // Ripples
  RI.forEach(r=>{const[sx,sy]=w2s(r.x,r.y);X.strokeStyle=r.c+Math.floor(r.a*255).toString(16).padStart(2,'0');
    X.lineWidth=1.2;X.beginPath();X.arc(sx,sy,r.r*cm.z,0,6.28);X.stroke()});

  // Particles
  PA.forEach(p=>{const[sx,sy]=w2s(p.x,p.y);X.globalAlpha=p.li;X.fillStyle=p.c;
    X.beginPath();X.arc(sx,sy,p.s*cm.z,0,6.28);X.fill();X.globalAlpha=1});

  // Nodes
  N.forEach(n=>{
    const[sx,sy]=w2s(n.x,n.y);
    const pulse=pE?Math.sin(t*2.2+n.ph)*.15:0;
    let r=(n.bs+n.bs*pulse)*cm.z;
    if(sx<-60||sx>W+60||sy<-60||sy>H+60)return;
    const isH=n===hv,isF=n===fc;
    const con=(fc||hv)&&E.some(e=>(e.s===(fc||hv)&&e.t===n)||(e.t===(fc||hv)&&e.s===n));
    const dm=(fc||hv)&&!isH&&!isF&&!con;

    // Ghost shimmer
    if(n.type==='ghost'){X.globalAlpha=dm?.06:.3+.7*Math.abs(Math.sin(t*3.5+n.ph*5))}
    else X.globalAlpha=dm?.08:1;

    // Outer glow
    const gr=r*(isH||isF?6:3);
    const gl=X.createRadialGradient(sx,sy,r*.3,sx,sy,gr);
    gl.addColorStop(0,n.color+(isH||isF?'30':n.type==='device'?'18':'0c'));
    gl.addColorStop(.6,n.color+'05');gl.addColorStop(1,'transparent');
    X.fillStyle=gl;X.beginPath();X.arc(sx,sy,gr,0,6.28);X.fill();

    // Process heartbeat
    if(n.type==='process'&&pE){const bt=Math.pow(Math.sin(t*4.5+n.ph),8);
      X.strokeStyle=n.color+Math.floor(bt*80).toString(16).padStart(2,'0');X.lineWidth=1.2;
      X.beginPath();X.arc(sx,sy,r+5*cm.z+bt*10*cm.z,0,6.28);X.stroke()}

    // Core with gradient
    const cg=X.createRadialGradient(sx-r*.25,sy-r*.25,0,sx,sy,r);
    cg.addColorStop(0,lC(n.color,50));cg.addColorStop(1,n.color);
    X.fillStyle=cg;X.beginPath();X.arc(sx,sy,Math.max(1.5,r),0,6.28);X.fill();

    // Bright center
    X.fillStyle='rgba(255,255,255,'+(isH?.7:.25)+')';
    X.beginPath();X.arc(sx,sy,Math.max(.8,r*.25),0,6.28);X.fill();

    // Focus ring
    if(isH||isF){X.strokeStyle='rgba(255,255,255,.45)';X.lineWidth=1;X.setLineDash([3,4]);
      X.beginPath();X.arc(sx,sy,r+4*cm.z,0,6.28);X.stroke();X.setLineDash([])}

    // Device particles
    if(n.type==='device'&&pE&&Math.random()<.12)sP(n.x+(Math.random()-.5)*18,n.y+(Math.random()-.5)*18,n.color);

    // Labels
    if(sL&&!dm&&cm.z>.3){const fs=Math.max(8,Math.min(11,8.5*cm.z));
      X.font=((isH||isF)?'600 ':'300 ')+fs+'px -apple-system,system-ui,sans-serif';X.textAlign='center';
      X.fillStyle=(isH||isF)?'#fff':'rgba(200,169,81,'+(dm?'.08':'.55')+')';
      const lb=n.label.length>26?n.label.substring(0,23)+'…':n.label;
      X.fillText(lb,sx,sy-r-5*cm.z)}
    X.globalAlpha=1});

  X.font='300 9px -apple-system,system-ui,sans-serif';X.fillStyle='rgba(200,169,81,.08)';
  X.textAlign='right';X.fillText('𓁢 Seba',W-14,H-8);
  requestAnimationFrame(render)}
requestAnimationFrame(render);

function lC(h,a){h=h.replace('#','');let r=parseInt(h.substring(0,2),16),g=parseInt(h.substring(2,4),16),b=parseInt(h.substring(4,6),16);
  return'#'+[Math.min(255,r+a),Math.min(255,g+a),Math.min(255,b+a)].map(c=>c.toString(16).padStart(2,'0')).join('')}

// Mouse
let iD=false,sx0,sy0,cx0,cy0,dN=null;
function nAt(mx,my){const[wx,wy]=s2w(mx,my);let b=null,bd=1e9;
  N.forEach(n=>{const dx=n.x-wx,dy=n.y-wy,d=Math.sqrt(dx*dx+dy*dy),hr=(n.bs+6)/cm.z;
    if(d<hr&&d<bd){b=n;bd=d}});return b}
C.addEventListener('mousedown',e=>{const n=nAt(e.clientX,e.clientY);
  if(n){dN=n}else{iD=true;sx0=e.clientX;sy0=e.clientY;cx0=cm.tx;cy0=cm.ty}});
C.addEventListener('mousemove',e=>{
  if(dN){const[wx,wy]=s2w(e.clientX,e.clientY);dN.x=wx;dN.y=wy;dN.vx=0;dN.vy=0;C.style.cursor='grabbing'}
  else if(iD){cm.tx=cx0-(e.clientX-sx0)/cm.z;cm.ty=cy0-(e.clientY-sy0)/cm.z;C.style.cursor='grabbing'}
  else{const n=nAt(e.clientX,e.clientY);hv=n;
    if(n){tip.style.display='block';tip.style.left=(e.clientX+14)+'px';tip.style.top=(e.clientY+14)+'px';
      let h='<div class="tn">'+n.label+'</div><div class="tt">'+n.type+'</div>';
      if(n.sz)h+='<div class="tm">'+fS(n.sz)+'</div>';
      if(Object.keys(n.meta).length){h+='<div class="tm">';for(const[k,v]of Object.entries(n.meta))h+=k+': '+v+'<br>';h+='</div>'}
      tip.innerHTML=h;C.style.cursor='pointer'}else{tip.style.display='none';C.style.cursor='crosshair'}}});
C.addEventListener('mouseup',()=>{iD=false;dN=null;C.style.cursor='crosshair'});
C.addEventListener('dblclick',e=>{const n=nAt(e.clientX,e.clientY);
  if(n){if(fc===n){fc=null;fi.style.display='none'}else{fc=n;cm.tx=n.x;cm.ty=n.y;cm.tz=2.2;
    sR(n.x,n.y,n.color);fi.innerHTML='⬡ <strong>'+n.label+'</strong> — dblclick to release';fi.style.display='block'}}
  else{fc=null;fi.style.display='none'}});
C.addEventListener('wheel',e=>{e.preventDefault();cm.tz=Math.max(.15,Math.min(8,cm.tz*(e.deltaY>0?.9:1.12)))},{passive:false});

function fS(b){if(b<1024)return b+' B';if(b<1048576)return(b/1024).toFixed(1)+' KB';
  if(b<1073741824)return(b/1048576).toFixed(1)+' MB';return(b/1073741824).toFixed(1)+' GB'}

window.RV=()=>{cm.tx=0;cm.ty=0;cm.tz=1;fc=null;fi.style.display='none'};
window.TL=()=>{sL=!sL};
window.TP=()=>{pE=!pE;document.getElementById('bp').style.color=pE?'#C8A951':'#333'};

const tc={};N.forEach(n=>tc[n.type]=(tc[n.type]||0)+1);
let lh='<div class="lt">Infrastructure</div>';
for(const[ty,co]of Object.entries(TC))if(tc[ty])lh+='<div class="li"><span class="ld" style="background:'+co+';box-shadow:0 0 6px '+co+'"></span>'+ty+' ('+tc[ty]+')</div>';
document.getElementById('leg').innerHTML=lh;

document.getElementById('sta').innerHTML=
  '<div><div class="sl">Nodes</div><div class="sv">'+N.length+'</div></div>'+
  '<div style="margin-top:6px"><div class="sl">Edges</div><div class="sv">'+E.length+'</div></div>'+
  '<div style="margin-top:8px;color:#333;font-size:9px;letter-spacing:1px">'+D.platform+'</div>'+
  '<div style="margin-top:6px;color:#222;font-size:8px;letter-spacing:1px">SCROLL ZOOM<br>DRAG PAN<br>DBLCLICK FOCUS</div>';
})();
</script>
</body>
</html>`, g.Hostname, g.Hostname, g.Platform, g.GeneratedAt,
		string(graphJSON), colorsJSON)

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(outputPath, []byte(html), 0644); err != nil {
		return err
	}
	stele.Inscribe("seba", stele.TypeSebaRender, "", map[string]string{
		"nodes":  fmt.Sprintf("%d", len(g.Nodes)),
		"edges":  fmt.Sprintf("%d", len(g.Edges)),
		"output": outputPath,
	})
	return nil
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
