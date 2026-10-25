'use strict';
// Inert node:test controller/DOM controls. No browser, network, engine or model.
const test=require('node:test');const assert=require('node:assert/strict');
const ui=require('./ui/maat-history.js');
const page=(snapshot='s',next='')=>({cases:[],summary:{total:0},history:{coverage:'complete',snapshot,query_id:'q',next_cursor:next,count_scope:'query',total_complete:true,total:0}});
function pending(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b});return{promise,resolve,reject}}
function response(value){return{ok:true,json:async()=>value}}
test('late success and failure cannot overwrite latest committed page',async()=>{
 for(const failure of [false,true]){
  const a=pending(),b=pending();let calls=0;const c=ui.createRead(()=>[a,b][calls++].promise,'/read',()=>{},ui.validateCases);
  const first=c.start({q:'old'}),second=c.start({q:'new'});b.resolve(response(page('new')));await second;
  if(failure)a.reject(Error('late old failure'));else a.resolve(response(page('old')));await first;
  assert.equal(c.state.data.history.snapshot,'new');assert.equal(c.state.error,'');assert.equal(c.state.params.q,'new');c.dispose();
 }
});
test('continuation retains filters and rejects changed snapshot without committing',async()=>{
 const urls=[];const c=ui.createRead(async url=>{urls.push(url);return response(page(urls.length===1?'s':'changed','opaque'))},'/read',()=>{},ui.validateCases);
 await c.start({q:'needle',scope:'system-one',limit:50});await c.start(c.state.params,'opaque');
 assert.match(urls[1],/cursor=opaque/);assert.match(urls[1],/q=needle/);assert.equal(c.state.data.history.snapshot,'s');assert.match(c.state.error,/Snapshot changed/);
 const count=urls.length;await c.start({q:'other'},'opaque');assert.equal(urls.length,count);c.dispose();
});
test('cancelled draft and disposed view own no late commits',async()=>{
 for(const mode of ['invalidate','dispose']){const p=pending();let commits=0;const c=ui.createRead(()=>p.promise,'/read',()=>{commits++},ui.validateCases);
  const read=c.start({q:'old'});c[mode]();const before=commits;p.resolve(response(page('late')));await read;assert.equal(commits,before);assert.equal(c.state.data,null);
 }
});
test('partial is not complete empty and never exposes provisional cases',()=>{
 const partial=page();partial.history.coverage='partial';partial.history.total_complete=false;delete partial.history.total;
 ui.validateCases(partial);partial.cases=[{id:'provisional'}];assert.throws(()=>ui.validateCases(partial),/Provisional/);
 assert.throws(()=>ui.validateCases({}),/Invalid/);const d=page();d.history.count_scope='after-cursor';d.history.total_complete=false;delete d.history.total;ui.validateCases(d);
});
test('existing System One basis and prescribed recovery remain visible text',()=>{
 const text=ui.caseBasis({status:'resolved',kind:'system one screen',system_one:{gate:'block',feather_weight:80,subject:{head_sha:'abc'},floor:{checks:[{name:'floor',passed:false}]},findings:[{claim:'retained claim',fix_hint:'guided action'}],escalation:{reason:'owner review'}},next_action:{steps:[{level:1,title:'Recover',detail:'safe next step'}]},system_one_calibration:{frontier_evidence:'receipt:independent'}});
 for(const value of ['retained claim','guided action','safe next step','receipt:independent','owner review','abc'])assert.ok(text.includes(value),value);
});
// Minimal browser DOM seam drives the shipped mount/control callbacks; its
// assertions inspect actual textContent, never synthesized backend records.
class Node{constructor(tag){this.tag=tag;this.children=[];this.value='';this.textContent='';this.disabled=false}appendChild(n){this.children.push(n)}replaceChildren(){this.children=[]}setAttribute(){}remove(){} }
function descendants(n){return[n,...n.children.flatMap(descendants)]}
function text(n){return[n.textContent,...n.children.map(text)].join(' ')}
test('real Continue Load older Retry and canonical archive detail/history controls',async()=>{
 const beforeDoc=global.document,beforeFetch=global.fetch;global.document={createElement:t=>new Node(t)};
 const calls=[];let caseCalls=0;
 global.fetch=async url=>{calls.push(url);const q=new URL(url,'http://inert').searchParams;
  if(url.startsWith('/api/maat/casebook')){caseCalls++;const p=page();if(caseCalls===1){p.history.coverage='partial';p.history.total_complete=false;p.history.next_cursor='scan-cursor'}else if(caseCalls===2){p.history.next_cursor='older-cursor';p.history.count_scope='after-cursor';p.history.total_complete=false;delete p.history.total}else if(caseCalls===3){return{ok:false,json:async()=>({code:'snapshot_changed'})}}return response(p)}
  if(q.get('mode')==='detail')return response({id:'a'.repeat(64),revision:2,title:'Retained',content:'Canonical detail',origins:[],references:[]});
  if(q.get('mode')==='history')return response({items:[],total:2,next_cursor:'archive-cursor'});
  return response({items:[{id:'a'.repeat(64),revision:2,title:'Retained'}],total:1,withheld:0,coverage:{}});
 };
 const root=new Node('root');let dispose;
 const flush=async()=>{for(let i=0;i<10;i++)await Promise.resolve()};
 const click=label=>{const b=descendants(root).find(n=>n.tag==='button'&&n.textContent===label);assert.ok(b,label);assert.equal(b.disabled,false,label);b.onclick()};
 try{dispose=ui.mount(root);await flush();assert.match(text(root),/Scan incomplete; no provisional/);click('Continue scan');await flush();assert.match(text(root),/Whole-query total not established/);click('Load older');await flush();assert.match(text(root),/snapshot_changed/);click('Retry');await flush();
  click('Detail / provenance');await flush();assert.match(text(root),/Canonical detail/);click('Version history');await flush();assert.ok(calls.some(u=>u.includes('mode=history')));assert.ok(calls.some(u=>u.includes('cursor=scan-cursor')));assert.ok(calls.some(u=>u.includes('cursor=older-cursor')));
 }finally{if(dispose)dispose();global.document=beforeDoc;global.fetch=beforeFetch}
});
