(function(g){
'use strict';
// A Mac console is live only when GET /healthz answers 200 with the body "ok",
// read in full. An opaque, redirected or error answer is unknown, never live.
function live(res,body){
 return !!res&&res.type!=='opaque'&&!res.redirected&&res.status===200&&String(body).trim()==='ok';
}
function probe(fetchFn,url,ms){
 var t=typeof AbortSignal!=='undefined'&&AbortSignal.timeout?AbortSignal.timeout(ms||4000):undefined;
 return fetchFn(String(url).replace(/\/+$/,'')+'/healthz',{mode:'cors',redirect:'error',cache:'no-store',credentials:'omit',signal:t})
  .then(function(r){return r.text().then(function(b){return live(r,b)})},function(){return false});
}
// pick returns the first console, in list order, whose probe verified it live.
function pick(macs,oks){
 for(var i=0;i<macs.length;i++)if(oks[i])return macs[i];
 return null;
}
g.consoleProbe={live:live,probe:probe,pick:pick};
})(globalThis);
