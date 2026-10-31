/* Hosted router board service worker: when the cloud board stops answering a
   page load, serve the cached fallback page, which moves the viewer to the
   first live Mac console on the tailnet. Data reads are never cached: a stale
   ledger shown as live would be worse than an honest "unavailable". */
'use strict';
var CACHE='sirsi-board-v1';
self.addEventListener('install',function(e){
 e.waitUntil(caches.open(CACHE).then(function(c){return c.addAll(['/offline.html','/api/macs'])}).then(function(){return self.skipWaiting()}));
});
self.addEventListener('activate',function(e){e.waitUntil(self.clients.claim())});
self.addEventListener('fetch',function(e){
 var r=e.request,u=new URL(r.url);
 if(u.origin!==location.origin)return;
 if(u.pathname==='/api/macs'){
  // Keep the Mac list fresh for the fallback page, but always answer the page live.
  e.respondWith(fetch(r).then(function(res){if(res.ok){var copy=res.clone();caches.open(CACHE).then(function(c){c.put('/api/macs',copy)})}return res}).catch(function(){return caches.match('/api/macs')}));
  return;
 }
 if(r.mode!=='navigate')return;
 e.respondWith(fetch(r).then(function(res){
  // Cloud Run / IAP answer 5xx while the board is down or starting.
  return res.status>=500?caches.match('/offline.html'):res;
 }).catch(function(){return caches.match('/offline.html')}));
});
