'use strict';
const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const fabric=require('../ui/fabric.js');
const escape=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
function board(){return {work:{done:1,open:2,in_progress:1,assigned_not_started:1,stalled:0,blocked:0,total:3},messages:{open:7,stranded:1,oldest_age_seconds:0},lanes:[],health:{ram_pct:null,swap:null,git_dirty:null,total_ram:null,cpu_cores:null,engine:'unknown',engine_state:'unavailable',issues:[]},activity:[],activity_seeded:false,runtime_available:false,generated_at:'2026-10-25T12:00:00Z',build:'fixture'}}
function task(id){return {agent:'lane',id,subject:'Inert task '+id,status:'pending',liveness:'unknown',role:'gold',responsible:'fixture-owner',stage:'build',test_state:'untested',updated:'',blocker_recorded:false,text_limited:false,provenance:{source:'canonical-task-registry',created:'',commissioned_at:'',commissioned_by:'fixture-lead',charter_recorded:false,outline_recorded:false,result_recorded:false,public_links:[],withheld_links:0}}}
function tasksBoard(count){const b=board();b.work.open=count;b.work.total=count+b.work.done;b.work.in_progress=0;b.work.assigned_not_started=count;b.tasks={state:'observed',rows:Array.from({length:Math.min(count,64)},(_,i)=>task('task-'+i)),offset:0,limit:64,total:count,snapshot:'a'.repeat(64),next_offset:count>64?64:null};return b}
test('fabric streams retain exact producer values and unknown health',()=>{const html=fabric.render(board(),escape);assert.match(html,/1 \/ 2/);assert.match(html,/7/);assert.match(html,/Inbox messages/);assert.match(html,/Unavailable/);assert.match(html,/Baseline only/);assert.doesNotMatch(html,/undefined|NaN/)});
test('known empty differs from absent producer and first baseline from quiet',()=>{assert.match(fabric.render(null,escape),/unavailable/);const b=board();b.work={done:0,open:0,in_progress:0,assigned_not_started:0,stalled:0,blocked:0,total:0};b.messages={open:0,stranded:0,oldest_age_seconds:0};b.health.swap=0;b.activity_seeded=true;const html=fabric.render(b,escape);assert.match(html,/0 bytes/);assert.match(html,/No status transitions observed/);assert.doesNotMatch(html,/Baseline only/)});
test('real supplied transitions render escaped without synthesizing events',()=>{const b=board();b.activity_seeded=true;b.activity=[{at:'now',agent:'<lane>',task_id:'one',from:'pending',to:'done'}];const html=fabric.render(b,escape);assert.match(html,/pending → done/);assert.match(html,/&lt;lane&gt;/);assert.doesNotMatch(html,/<lane>/);assert.equal(b.activity.length,1)});
test('actual Go endpoint output is the web projection input',()=>{
 const path=process.env.PANTHEON_TEST_FABRIC_WIRE_OUTPUT;assert.ok(path,'Main must supply actual Go producer output; absence is not a pass');
 const b=JSON.parse(fs.readFileSync(path,'utf8'));assert.ok(fabric.valid(b));assert.equal(b.work.total,2);assert.equal(b.work.done,1);assert.equal(b.messages.open,1);assert.equal(b.activity.length,1);assert.equal(b.activity[0].from,'pending');assert.equal(b.activity[0].to,'in-progress');
 const html=fabric.render(b,escape);assert.match(html,/pending → in-progress/);assert.match(html,/0 bytes/);assert.match(html,/IDLE with work/);
 assert.equal(fabric.taskPage(b).rows[0].id,'task');assert.match(html,/data-fabric-task-id="task"/);assert.match(html,/Inert open task/);
 const detail=fabric.taskDetail(b,'lane','task',escape);assert.match(detail,/canonical-task-registry/);assert.match(detail,/fixture-lead/);
 assert.deepEqual(b.presentation.headlines,{completed:'ok',messages:'ink2',progress:'gold',stalled:'warn'});
});
test('real task rows expose detail provenance and registration without inbox substitution',()=>{
 const b=tasksBoard(2);b.lanes=[{agent:'lane',state:'UNKNOWN',state_label:'Runtime unavailable',role:'dim',open:2,registered:{router:true,thread:false,ledger:true}}];b.tasks.rows[0].subject='<unsafe task>';b.tasks.rows[0].blocker_recorded=true;b.tasks.rows[0].blocker_task_id='task-1';b.tasks.rows[0].provenance.public_links=[{kind:'evidence',url:'https://github.com/SirsiMaster/sirsi-pantheon/pull/1'}];
 const html=fabric.render(b,escape);assert.match(html,/data-fabric-task-id="task-0"/);assert.match(html,/&lt;unsafe task&gt;/);assert.doesNotMatch(html,/<unsafe task>/);
 const detail=fabric.taskDetail(b,'lane','task-0',escape);assert.match(detail,/canonical-task-registry/);assert.match(detail,/fixture-lead/);assert.match(detail,/task-1/);assert.match(detail,/<dt>Thread registration<\/dt><dd>Not registered/);assert.match(detail,/not fetched or independently verified/);
 assert.match(fabric.taskDetail(b,'other','task-0',escape),/not supplied/);
 b.tasks.rows[0].provenance.public_links=[{kind:'evidence',url:'javascript:alert(1)'}];assert.equal(fabric.taskPage(b),null);assert.doesNotMatch(fabric.taskDetail(b,'lane','task-0',escape),/href=/);
});
test('known empty missing incomplete and continuation pages retain distinct truth',()=>{
 assert.match(fabric.render(tasksBoard(0),escape),/No open ledger tasks in this observation/);
 assert.match(fabric.render(board(),escape),/Task rows unavailable/);
 const b=tasksBoard(65);assert.match(fabric.render(b,escape),/Showing 1–64 of 65 task rows/);assert.match(fabric.render(b,escape),/Next task page/);
 const buttons=['first','next'].map(value=>({dataset:{fabricTaskPage:value},addEventListener(_event,callback){this.click=callback}}));let selected='unset';
 fabric.bindTaskInspectors({querySelectorAll(selector){return selector==='[data-fabric-task-page]'?buttons:[]}},b,()=>assert.fail('unexpected detail'),page=>{selected=page});
 buttons[1].click();assert.deepEqual(selected,{offset:64,snapshot:'a'.repeat(64)});buttons[0].click();assert.equal(selected,null);
 delete b.tasks.next_offset;assert.equal(fabric.taskPage(b),null);
});
test('semantic presentation uses producer role tokens and existing CSS palette only',()=>{
 const b=board();b.presentation={headlines:{completed:'ok',progress:'gold',stalled:'warn',messages:'ink2'}};b.lanes=[{agent:'lane',state_label:'WORKING',open:1,role:'dim'}];
 const html=fabric.render(b,escape);for(const role of ['ok','gold','warn','ink2','dim'])assert.match(html,new RegExp('fabric-role-'+role));
 assert.match(html,/class="fabric-role-dim">WORKING/);assert.equal(fabric.roleClass('ok" onclick="bad'),'fabric-role-dim');
 const css=fs.readFileSync(__dirname+'/../ui/app.css','utf8');for(const role of ['ok','gold','warn','ink2','dim'])assert.ok(css.includes('.fabric-role-'+role+'{color:var(--'+role+')}'));
});
