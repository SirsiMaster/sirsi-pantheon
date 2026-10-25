(function (root, factory) {
 'use strict';
 const api=factory(); if(typeof module==='object'&&module.exports)module.exports=api;
 else root.MaatHistoryUI=api;
})(typeof window==='undefined'?globalThis:window,function(){
 'use strict';
 function key(p){return JSON.stringify(Object.keys(p).filter(k=>k!=='cursor').sort().map(k=>[k,p[k]]))}
 function cursor(d){return d&&d.history?d.history.next_cursor||'':d&&d.next_cursor||''}
 // One current request owns commits, including failures and cancellation.
 // Pages replace, never accumulate an unbounded history or mix snapshots.
 function createRead(fetcher,path,changed,validate){
  let generation=0,disposed=false,abort=null;
  const state={data:null,error:'',busy:false,params:null,key:'',requestCursor:''};
  function notify(){if(!disposed)changed(state)}
  async function start(parameters,next){
   if(disposed)return;
   const p=Object.assign({},parameters);delete p.cursor;const k=key(p),prior=state.data;
   if(next&&(state.key!==k||cursor(prior)!==next)){state.error='Continuation no longer belongs to this query. Restart.';notify();return}
   const snapshot=next&&prior&&prior.history&&prior.history.snapshot;
   const queryID=next&&prior&&prior.history&&prior.history.query_id;
   const mine=++generation;if(abort)abort.abort();abort=new AbortController();const signal=abort.signal;
   if(state.key!==k)state.data=null;
   state.key=k;state.params=p;state.requestCursor=next||'';state.busy=true;state.error='';notify();
   const wire=Object.assign({},p);if(next)wire.cursor=next;
   try{
    const response=await fetcher(path+'?'+new URLSearchParams(wire).toString(),{signal:signal});
    const value=await response.json();
    if(!response.ok)throw new Error(value.code||value.error||'Read unavailable');
    validate(value,p);
    if(snapshot&&(value.history.snapshot!==snapshot||value.history.query_id!==queryID))throw new Error('Snapshot changed; restart the query');
    if(disposed||mine!==generation||signal.aborted)return;
    state.data=value;state.error='';
   }catch(e){if(disposed||mine!==generation||signal.aborted)return;state.error=e.message||'Read unavailable'}
   finally{if(!disposed&&mine===generation){state.busy=false;notify()}}
  }
  function invalidate(){++generation;if(abort)abort.abort();state.busy=false;state.data=null;state.params=null;state.key='';state.requestCursor='';state.error='Query changed. Submit Search.';notify()}
  function dispose(){disposed=true;++generation;if(abort)abort.abort()}
  return {state,start,invalidate,dispose};
 }
 function validateCases(v){
  if(!v||!Array.isArray(v.cases)||!v.summary||!v.history||!['complete','partial','unavailable'].includes(v.history.coverage))throw new Error('Invalid history projection');
  if(v.history.coverage==='partial'&&v.cases.length)throw new Error('Provisional history cases refused');
 }
 function validateKnowledge(v,p){
  if(p.mode==='detail'){if(!v||typeof v.id!=='string'||!Number.isInteger(v.revision))throw new Error('Invalid retained detail');return}
  if(!v||!Array.isArray(v.items)||!Number.isInteger(v.total)||v.total<0)throw new Error('Invalid archive page');
 }
 function element(tag,text,parent){const e=document.createElement(tag);if(text!==undefined)e.textContent=text;if(parent)parent.appendChild(e);return e}
 function button(parent,text,action,disabled){const b=element('button',text,parent);b.type='button';b.disabled=!!disabled;b.onclick=action;return b}
 function input(form,label,name,values){const l=element('label',label+' ',form);let e;
  if(values){e=element('select',undefined,l);values.forEach(v=>{const o=element('option',v||'All',e);o.value=v})}
  else {e=element('input',undefined,l);e.type='text';e.maxLength=512}
  e.name=name;return e;
 }
 function section(parent,title){const e=element('section',undefined,parent);e.className='t-line';element('h3',title,e);return e}
 function status(parent,text){const e=element('p',text,parent);e.setAttribute('role','status');return e}
 function caseBasis(d){
 let bits=['status: '+(d.status||'open'),'decision: '+(d.kind||'unknown')+' → '+(d.determination||'unknown'),'assessment: '+(d.assessed||'unknown')];
   if(d.affected)bits.push('affected: '+d.affected);
   if(d.evidence)bits.push('evidence: '+d.evidence);
   if(d.next_action){
    bits.push('next: '+(d.next_action.title||'review the retained evidence')+(d.next_action.detail?' — '+d.next_action.detail:''));
    if(Array.isArray(d.next_action.steps))d.next_action.steps.forEach(function(step){bits.push('recovery level '+(step.level||'?')+': '+(step.title||'unnamed')+(step.detail?' — '+step.detail:''));});
   }
   if(d.system_one){
    const screen=d.system_one, confidence=Math.round((screen.confidence||0)*100);
    bits.push('system one: '+(screen.gate||'unknown')+' · '+confidence+'% confidence · feather '+(screen.feather_weight??'unknown')+'/100');
   if(screen.subject)bits.push('screen subject: '+(screen.subject.kind||'unknown')+' '+(screen.subject.ref||'unknown')+' · '+(screen.subject.head_sha||'unknown'));
    if(screen.model)bits.push('screen model: '+(screen.model.provider||'unknown')+' '+(screen.model.version||'unknown')+' · '+(screen.model.local ? 'local' : 'external')+' · '+(screen.model.latency_ms??'unknown')+'ms');
    if(screen.floor){
     bits.push('deterministic floor: '+(screen.floor.passed?'passed':'failed'));
     if(Array.isArray(screen.floor.checks))screen.floor.checks.forEach(function(check){bits.push('floor check: '+(check.name||'unnamed')+' · '+(check.passed?'passed':'failed')+(check.detail?' · '+check.detail:''));});
    }
    if(screen.escalation&&screen.escalation.reason)bits.push('required review: '+screen.escalation.reason);
    if(Array.isArray(screen.findings))screen.findings.forEach(function(finding){
     bits.push('finding: '+(finding.severity||'information')+' · '+(finding.category||'finding')+' · '+(finding.claim||'no claim recorded'));
     if(finding.fix_hint)bits.push('prescribed next step: '+finding.fix_hint);
    });
   }
   if(d.system_one_calibration){
    const calibration=d.system_one_calibration;
    bits.push('system one calibration: '+(calibration.screen_gate||'unknown')+' → '+(calibration.frontier_gate||'unknown'));
    if(calibration.screen_evidence)bits.push('screen evidence: '+calibration.screen_evidence);
    if(calibration.frontier_evidence)bits.push('independent evidence: '+calibration.frontier_evidence);
   }

 if(d.resolution)bits.push('owner acceptance: '+d.resolution);
 for(const k of ['verified_repair_evidence','repair_outcome_evidence','pending_repair_attempt_evidence'])if(d[k])bits.push(k+': '+d[k]);
 return bits.join(" · ");
 }
 function mount(parent){
  const root=element('div',undefined,parent);let disposed=false;
  const cases=section(root,"Ma’at recorded history — read only");
  const cf=element('form',undefined,cases),cq=input(cf,'Search','q'),ck=input(cf,'Kind/category','kind');
  const cs=input(cf,'Status','status',['','open','resolved']),scope=input(cf,'Scope','scope',['all','system-one']);
  const submit=element('button','Search',cf);submit.type='submit';
  const cb=element('div',undefined,cases);let caseRead;
  const caseParams=()=>({q:cq.value,kind:ck.value,status:cs.value,scope:scope.value,limit:50});
  function caseRender(s){
   cb.replaceChildren();if(s.error){status(cb,'Unavailable: '+s.error);button(cb,'Retry',()=>caseRead.start(s.params||caseParams(),s.requestCursor));button(cb,'Restart query',()=>caseRead.start(caseParams()));return}
   if(s.busy)status(cb,'Reading bounded history…');if(!s.data)return;
   const v=s.data,h=v.history,invalid=v.journal_integrity&&v.journal_integrity.invalid_count||0;
   status(cb,'Coverage: '+h.coverage+' · counts: '+(h.count_scope||'unavailable')+' · this page '+v.cases.length+' cases');
   if(h.total_complete&&Number.isInteger(h.total))status(cb,'Complete query total: '+h.total);
   else status(cb,'Whole-query total not established on this response.');
   if(invalid)status(cb,invalid+' invalid journal records prevent an absence conclusion.');
   if(h.coverage==='partial')status(cb,'Scan incomplete; no provisional cases are shown. Continue this same snapshot.');
   else if(h.coverage!=='complete')status(cb,'This backend cannot establish historical coverage. No empty-history conclusion.');
   else{
    if(!v.cases.length)status(cb,h.count_scope==='after-cursor'?'No matches remain after this cursor.':invalid?'No valid matches; history integrity is incomplete.':'No matches in this complete query.');
    v.cases.forEach(c=>{const row=element('article',undefined,cb);element('h4',(c.priority||'normal')+' · '+c.status+' · '+c.category,row);
     element('p',c.why||c.determination,row);element('p',caseBasis(c),row);element('p','Evidence: '+(c.evidence||'none recorded'),row);
     const d=element('details',undefined,row);element('summary','Recorded case and same-snapshot links',d);element('pre',JSON.stringify(c,null,2),d);
    });
   }
   const next=cursor(v);if(next)button(cb,h.coverage==='partial'?'Continue scan':'Load older',()=>caseRead.start(s.params,next),s.busy);
   button(cb,'Restart query',()=>caseRead.start(caseParams()),s.busy);
  }
  caseRead=createRead(fetch.bind(globalThis),'/api/maat/casebook',caseRender,validateCases);
  cf.onsubmit=e=>{e.preventDefault();caseRead.start(caseParams())};cf.oninput=()=>caseRead.invalidate();cf.onchange=()=>caseRead.invalidate();

  const knowledge=section(root,"Ma’at retained knowledge — read only");
  const kf=element('form',undefined,knowledge),kq=input(kf,'Search','q'),source=input(kf,'Source','source');
  const classification=input(kf,'Classification','classification',['','conversation','search','note','document','unclassified']);
  const resolution=input(kf,'Owner state','resolution',['','open','reviewed','accepted','rejected']);
  const ks=element('button','Search',kf);ks.type='submit';const kb=element('div',undefined,knowledge);let knowledgeRead;
  const knowledgeParams=()=>({mode:'search',q:kq.value,source:source.value,classification:classification.value,resolution:resolution.value,limit:50});
  function knowledgeRender(s){
   kb.replaceChildren();if(s.error){status(kb,'Unavailable or changed: '+s.error);button(kb,'Retry',()=>knowledgeRead.start(s.params||knowledgeParams(),s.requestCursor));button(kb,'Restart search',()=>knowledgeRead.start(knowledgeParams()));return}
   if(s.busy)status(kb,'Reading canonical archive…');if(!s.data)return;
   const v=s.data,mode=s.params.mode;
   if(mode==='detail'){
    element('h4',v.title,kb);element('p','Revision '+v.revision+' · digest '+(v.content_digest||'withheld'),kb);
    element('pre',v.content||v.withheld_reason||v.summary||'',kb);
    element('pre',JSON.stringify({origins:v.origins,references:v.references,owner_annotations:v.owner_annotations},null,2),kb);
    button(kb,'Version history',()=>knowledgeRead.start({mode:'history',id:v.id,limit:50}));
   }else{
    status(kb,v.items.length+' on this page · '+v.total+' query-wide retained matches'+(Number.isInteger(v.withheld)?' · '+v.withheld+' archive-wide withheld':''));
    if(v.state&&v.state!=='complete'&&v.state!=='no_new_items')status(kb,'Source coverage: '+v.state+'; available records do not imply complete ingestion.');
    if(v.coverage)element('pre',JSON.stringify(v.coverage,null,2),kb);
    if(!v.items.length)status(kb,'No matching records in this archive query; inspect source coverage separately.');
    v.items.forEach(item=>{const row=element('article',undefined,kb);element('h4',item.title||item.id,row);element('p',item.excerpt||item.summary||'',row);
     element('p','r'+item.revision+' · '+item.classification+' · '+item.resolution,row);
     button(row,'Detail / provenance',()=>knowledgeRead.start({mode:'detail',id:item.id,revision:item.revision}));
     button(row,'History',()=>knowledgeRead.start({mode:'history',id:item.id,limit:50}));
    });
    const next=cursor(v);if(next)button(kb,'Load older',()=>knowledgeRead.start(s.params,next),s.busy);
   }
   button(kb,'Restart search',()=>knowledgeRead.start(knowledgeParams()),s.busy);
  }
  knowledgeRead=createRead(fetch.bind(globalThis),'/api/maat/knowledge',knowledgeRender,validateKnowledge);
  kf.onsubmit=e=>{e.preventDefault();knowledgeRead.start(knowledgeParams())};kf.oninput=()=>knowledgeRead.invalidate();kf.onchange=()=>knowledgeRead.invalidate();
  caseRead.start(caseParams());knowledgeRead.start(knowledgeParams());
  return function(){if(disposed)return;disposed=true;caseRead.dispose();knowledgeRead.dispose();root.remove()};
 }
 return {createRead,validateCases,validateKnowledge,caseBasis,mount};
});
