// Ledger edits on the hosted board. Inert everywhere else: the controls appear only when
// /api/router/v1/act/whoami answers, which needs board-serve and a verified sirsi.ai sign-in.
(function(){
  var api='/api/router/v1/act/',who=null;
  function say(m){var a=document.getElementById('announcement');if(a)a.textContent=m}
  function act(verb,body){
    return fetch(api+verb,{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Sirsi-Board':'1'},body:JSON.stringify(body)})
      .then(function(r){return r.json().then(function(j){if(!r.ok)throw new Error(j.error||('HTTP '+r.status));return j})})
      .then(function(){say(verb+' done');var b=document.getElementById('refresh');if(b)b.click();var d=document.querySelector('.inspector');if(d)d.close()})
      .catch(function(e){window.alert(verb+' refused: '+e.message)})
  }
  function ask(label,def){var v=window.prompt(label,def||'');return v===null?null:v.trim()}
  function button(label,fn){var b=document.createElement('button');b.type='button';b.className='btn';b.textContent=label;b.onclick=fn;return b}
  function bar(buttons){
    var d=document.querySelector('.inspector');if(!d||d.querySelector('.board-actions'))return;
    var s=document.createElement('section');s.className='board-actions';s.setAttribute('aria-label','Ledger edits');
    var h=document.createElement('h3');h.textContent='Edit as '+who;s.appendChild(h);
    buttons.forEach(function(b){s.appendChild(b)});d.appendChild(s)
  }
  function itemBar(id){bar([
    button('Close',function(){var n=ask('Close '+id+' — result note:');if(n!==null)act('close',{id:id,note:n})}),
    button('Make task',function(){var t=ask('Ledger task id for '+id+':',id.replace(/^\d{8}-\d{6}-/,'').slice(0,60));if(t)act('convert',{id:id,task_id:t})}),
    button('Reassign',function(){var to=ask('Reassign '+id+' to agent:');if(to)act('reassign-item',{id:id,to:to,note:ask('Note (optional):')||''})})
  ])}
  function taskBar(agent,id){var k={agent:agent,task_id:id};bar([
    button('Set status',function(){var s=ask('Status for '+agent+'/'+id+' (pending or blocked; claiming and completing stay with the lane):','blocked');if(s)act('update-task',Object.assign({status:s},k))}),
    button('Blocked by',function(){var b=ask('Blocked by (task id or reason; empty clears):');if(b!==null)act('update-task',Object.assign({blocked_by:b},k))}),
    button('Rename',function(){var s=ask('New subject:');if(s)act('update-task',Object.assign({subject:s},k))}),
    button('Reassign',function(){var r=ask('Responsible party:');if(r)act('reassign-task',Object.assign({responsible:r},k))}),
    button('Sign off as done',function(){var n=ask('Sign off '+agent+'/'+id+' as done — why (recorded as your receipt):');if(n)act('sign-off',Object.assign({note:n},k))}),
    button('Cancel task',function(){var n=ask('Cancel '+agent+'/'+id+' — reason (canceled never counts as done):');if(n)act('cancel-task',Object.assign({note:n},k))})
  ])}
  function addTask(){
    var agent=ask('Add a ledger task — agent:');if(!agent)return;
    var id=ask('Task id:');if(!id)return;
    var subject=ask('Subject:');if(!subject)return;
    act('add-task',{agent:agent,task_id:id,subject:subject,responsible:ask('Responsible party (optional):')||''})
  }
  function retireTasks(){
    if(document.getElementById('board-retire'))return;
    var d=document.createElement('dialog');d.id='board-retire';d.className='inspector board-retire';
    d.setAttribute('aria-labelledby','board-retire-title');
    d.innerHTML='<h2 id="board-retire-title">Retire reviewed tasks</h2><p>Owner cancellation preserves history and never counts as completed work. Enter only reviewed IDs. Records with custody or an existing terminal outcome cannot be retired. Any unconfirmed response stops this pass for reconciliation.</p><form><label>Lane<input name="agent" required maxlength="200" autocomplete="off"></label><label>Task IDs — one per line<textarea name="ids" rows="8" required maxlength="100500" spellcheck="false"></textarea></label><label>Retirement reason<textarea name="reason" rows="3" required maxlength="2000"></textarea></label><label><input name="confirmed" type="checkbox" required> I reviewed these exact records and authorize cancellation, not completion.</label><button class="btn primary" type="submit">Retire entered tasks</button> <button class="btn" type="button" data-stop>Close</button></form><p role="status" data-progress></p><pre data-results tabindex="0" aria-label="Retirement results"></pre>';
    document.body.appendChild(d);
    var form=d.querySelector('form'),progress=d.querySelector('[data-progress]'),results=d.querySelector('[data-results]'),close=d.querySelector('[data-stop]'),busy=false,stopped=false;
    function finishClose(){if(busy){stopped=true;progress.textContent='Stopping after the current request; retain these results for reconciliation.'}else{d.close();d.remove()}}
    close.onclick=finishClose;d.addEventListener('cancel',function(e){e.preventDefault();finishClose()});
    form.onsubmit=async function(e){
      e.preventDefault();if(busy)return;
      var plan;
      try{plan=PantheonBoardRetire.parse(form.elements.agent.value,form.elements.ids.value,form.elements.reason.value)}catch(err){progress.textContent=err.message;return}
      if(!form.elements.confirmed.checked)return;
      busy=true;stopped=false;results.textContent='';
      Array.from(form.elements).forEach(function(el){if(el!==close)el.disabled=true});close.textContent='Stop after current record';
      var seen=0;
      async function post(verb,body){
        var controller=new AbortController(),timer=setTimeout(function(){controller.abort()},30000);
        try{
          var r=await fetch(api+verb,{method:'POST',credentials:'same-origin',signal:controller.signal,headers:{'Content-Type':'application/json','X-Sirsi-Board':'1'},body:JSON.stringify(body)});
          if(!r.ok)throw new Error('request unavailable');
          var reply=await r.json();if(reply.ok!==true)throw new Error('success not confirmed');return true;
        }finally{clearTimeout(timer)}
      }
      progress.textContent='Retiring '+plan.ids.length+' reviewed records…';
      try{await PantheonBoardRetire.run(plan,post,function(id,outcome){seen++;results.textContent+=id+' — '+outcome+'\n';progress.textContent=seen+' of '+plan.ids.length+' records checked.'},function(){return stopped})}
      finally{busy=false;close.textContent='Close';progress.textContent+=' Finished this pass. Canceled is not done; reconcile held, unknown and unprocessed records before another pass.'}
    };
    d.showModal();form.elements.agent.focus();
  }
  fetch(api+'whoami',{credentials:'same-origin'}).then(function(r){return r.ok?r.json():null}).then(function(j){
    if(!j||!j.email)return;who=j.email;
    var tools=document.querySelector('.tools');if(tools){tools.insertBefore(button('Add task',addTask),tools.firstChild);tools.insertBefore(button('Retire reviewed tasks',retireTasks),tools.firstChild)}
    document.addEventListener('click',function(e){
      var b=e.target.closest('[data-item],[data-fabric-task-id]');if(!b)return;
      setTimeout(function(){if(b.dataset.item)itemBar(b.dataset.item);else taskBar(b.dataset.fabricTaskAgent,b.dataset.fabricTaskId)},0)
    })
  }).catch(function(){})
})();
