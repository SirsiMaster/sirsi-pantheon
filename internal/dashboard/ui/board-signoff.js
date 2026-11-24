// One deliberate owner sign-off, not an execution receipt or a task read grant.
(function(root){
  'use strict';
  function parse(agent,id,note,owner){
    var valid=/^[A-Za-z0-9][A-Za-z0-9._:-]{0,199}$/;
    agent=agent.trim();id=id.trim();note=note.trim();owner=owner.trim();
    if(!valid.test(agent)||!valid.test(id)||!note||note.length>4000||!/^([^@\s]+)@sirsi\.ai$/i.test(owner))
      throw new Error('Enter one exact lane and task ID, and a completion evidence note of at most 4,000 characters.');
    return Object.freeze({agent:agent,task_id:id,note:note,expected_owner:'board:'+owner});
  }
  function attempt(agent,id,note,owner){
    var plan=parse(agent,id,note,owner),started=false;
    return Object.freeze({plan:plan,run:async function(currentOwner,post){
      if(started)return 'already-started';
      started=true;
      try{if('board:'+await currentOwner()!==plan.expected_owner)return 'refused'}catch(_){return 'unavailable'}
      try{
        if(await post('sign-off-verified',plan)!==true)return 'refused';
        if('board:'+await currentOwner()!==plan.expected_owner)return 'unknown';
        return 'completed';
      }catch(_){return 'unknown'}
    }});
  }
  var api={parse:parse,attempt:attempt};
  if(typeof module==='object'&&module.exports)module.exports=api;
  else root.PantheonBoardSignoff=api;
})(typeof window==='object'?window:this);
