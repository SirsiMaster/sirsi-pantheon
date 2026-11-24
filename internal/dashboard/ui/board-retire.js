// Write-only owner retirement: no task enumeration or private read grant.
(function(root){
  'use strict';
  function parse(agent,text,note){
    agent=agent.trim();note=note.trim();
    var ids=text.trim().split(/\s+/),valid=/^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$/;
    if(!valid.test(agent)||!text.trim()||ids.length>500||ids.some(function(id){return !valid.test(id)})||new Set(ids).size!==ids.length||!note||note.length>2000)
      throw new Error('Enter one lane, 1–500 unique task IDs (one per line), and a reason of at most 2,000 characters.');
    return {agent:agent,ids:ids,note:note};
  }
  async function run(plan,post,report,stop){
    for(var id of plan.ids){
      if(stop())break;
      var key={agent:plan.agent,task_id:id};
      try{
        // CancelTask's final UPDATE atomically refuses terminal or custodial
        // records. Never reopen or relabel a task as preparation for retirement.
        var canceled=await post('cancel-task',Object.assign({note:plan.note},key));
        if(!canceled)throw new Error('success not confirmed');
        report(id,'canceled (not done)');
      }catch(_){report(id,'unknown: cancellation response unavailable; reconcile before retry');break}
    }
  }
  var api={parse:parse,run:run};
  if(typeof module==='object'&&module.exports)module.exports=api;
  else root.PantheonBoardRetire=api;
})(typeof window==='object'?window:this);
