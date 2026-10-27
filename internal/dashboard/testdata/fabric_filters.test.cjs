'use strict';
const {test}=require('node:test'),assert=require('node:assert/strict');
const fabric=require('../ui/fabric.js');
const escape=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const filters=['done','open','in_progress','assigned_not_started','stalled','blocked'];
function board(){return {work:{done:1,open:4,in_progress:1,assigned_not_started:1,stalled:1,blocked:1,total:5},messages:{open:3,stranded:1,oldest_age_seconds:0},lanes:[],health:{ram_pct:null,swap:null,total_ram:null,cpu_cores:null,git_dirty:null,engine:'unknown',engine_state:'unavailable',issues:[]},activity:[],activity_seeded:false,runtime_available:true,generated_at:'2026-10-27T12:00:00Z',build:'fixture'}}
test('each headline bucket is a distinct native keyboard control with its exact count',()=>{
 const b=board(),html=fabric.headlines(b,escape);
 for(const key of filters){assert.match(html,new RegExp('<button type="button"[^>]*data-fabric-filter="'+key+'"[^>]*>'+b.work[key]+'</button>'))}
 assert.match(html,/<button type="button"[^>]*data-fabric-messages/);
 assert.match(html,/Show 1 blocked tasks/);
});
test('production metric binder selects each exact bucket and keeps messages separate',()=>{
 const selected=[],buttons=filters.map(key=>({dataset:{fabricFilter:key},addEventListener(_event,fn){this.click=fn}}));
 const message={addEventListener(_event,fn){this.click=fn}};
 const element={querySelectorAll(selector){return selector==='[data-fabric-filter]'?buttons:[message]}};
 let queues=0;fabric.bindHeadlineControls(element,board(),filter=>selected.push(filter),()=>queues++);
 buttons.forEach(button=>button.click());message.click();
 assert.deepEqual(selected,filters);assert.equal(queues,1);
});
test('filtered page total is checked against its headline, not all open tasks',()=>{
 const b=board();b.tasks={filter:'blocked',state:'observed',rows:[],limit:64,total:0,offset:0,snapshot:'a'.repeat(64),next_offset:null};
 assert.equal(fabric.taskPage(b),null);
 b.work.blocked=0;assert.ok(fabric.taskPage(b));assert.match(fabric.render(b,escape),/Blocked tasks/);assert.match(fabric.render(b,escape),/No tasks match this selection/);
 b.tasks.filter='constructor';assert.equal(fabric.taskPage(b),null);
});
test('next and refresh controls preserve the exact filter and captured cursor',()=>{
 const b=board();b.work.blocked=65;b.tasks={filter:'blocked',state:'observed',rows:Array.from({length:64},(_,i)=>({agent:'lane',id:'t'+i,subject:'Task',status:'blocked',liveness:'unknown',role:'warn',responsible:'lead',stage:'build',test_state:'untested',updated:'',blocker_recorded:true,text_limited:false,provenance:{source:'canonical-task-registry',created:'',commissioned_at:'',commissioned_by:'lead',charter_recorded:true,outline_recorded:false,result_recorded:false,public_links:[],withheld_links:0}})),limit:64,total:65,offset:0,snapshot:'b'.repeat(64),next_offset:64};
 const seen=[],buttons=['first','next'].map(key=>({dataset:{fabricTaskPage:key},addEventListener(_event,fn){this.click=fn}}));
 fabric.bindTaskInspectors({querySelectorAll(selector){return selector==='[data-fabric-task-page]'?buttons:[]}},b,()=>assert.fail('not a task click'),p=>seen.push(p));
 buttons.forEach(button=>button.click());assert.deepEqual(seen,[{filter:'blocked'},{offset:64,snapshot:'b'.repeat(64),filter:'blocked'}]);
});
test('a matching count cannot disguise rows from the wrong bucket',()=>{
 const b=board();b.tasks={filter:'blocked',state:'observed',rows:[{agent:'lane',id:'one',subject:'Task',status:'pending',liveness:'unknown',role:'gold',responsible:'lead',stage:'build',test_state:'untested',updated:'',blocker_recorded:false,text_limited:false,provenance:{source:'canonical-task-registry',created:'',commissioned_at:'',commissioned_by:'lead',charter_recorded:true,outline_recorded:false,result_recorded:false,public_links:[],withheld_links:0}}],limit:64,total:1,offset:0,snapshot:'a'.repeat(64),next_offset:null};
 assert.equal(fabric.taskPage(b),null);b.tasks.rows[0].status='blocked';assert.ok(fabric.taskPage(b));
});
