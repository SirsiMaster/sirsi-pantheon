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
    button('Reassign',function(){var r=ask('Responsible party:');if(r)act('reassign-task',Object.assign({responsible:r},k))})
  ])}
  function addTask(){
    var agent=ask('Add a ledger task — agent:');if(!agent)return;
    var id=ask('Task id:');if(!id)return;
    var subject=ask('Subject:');if(!subject)return;
    act('add-task',{agent:agent,task_id:id,subject:subject,responsible:ask('Responsible party (optional):')||''})
  }
  fetch(api+'whoami',{credentials:'same-origin'}).then(function(r){return r.ok?r.json():null}).then(function(j){
    if(!j||!j.email)return;who=j.email;
    var tools=document.querySelector('.tools');if(tools)tools.insertBefore(button('Add task',addTask),tools.firstChild);
    document.addEventListener('click',function(e){
      var b=e.target.closest('[data-item],[data-fabric-task-id]');if(!b)return;
      setTimeout(function(){if(b.dataset.item)itemBar(b.dataset.item);else taskBar(b.dataset.fabricTaskAgent,b.dataset.fabricTaskId)},0)
    })
  }).catch(function(){})
})();
