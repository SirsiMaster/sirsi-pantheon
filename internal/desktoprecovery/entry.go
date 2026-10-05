package desktoprecovery

const entryHTML = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Pantheon desktop recovery</title><style>body{font:17px system-ui;background:#121713;color:#eef3ed;max-width:28rem;margin:8vh auto;padding:24px}h1{color:#c8a951}label,input,button{display:block;width:100%;box-sizing:border-box;margin:12px 0}input,button{font:inherit;padding:12px;border-radius:8px}button{background:#c8a951;border:0}p{line-height:1.5}</style><h1>Recover your desktop</h1><p>Sign in with your recovery operator account. The next screen may ask for your Mac Screen Sharing credentials.</p><form id="entry"><label>Operator login<input id="login" autocomplete="username" required type="email"></label><label>Recovery password<input id="password" autocomplete="off" required type="password" maxlength="72"></label><button>Connect to desktop</button></form><p id="status" role="status"></p><script src="/recovery/entry.js"></script></html>`

const entryJS = `document.getElementById('entry').addEventListener('submit', async event => {
 event.preventDefault();
 const input = document.getElementById('password');
 const login = document.getElementById('login').value;
 const password = input.value;
 input.value = '';
 const status = document.getElementById('status');
 status.textContent = 'Connecting…';
 try {
  const bytes = new TextEncoder().encode(login + ':' + password);
  const auth = btoa(Array.from(bytes, b => String.fromCharCode(b)).join(''));
  const endpoint = location.pathname.replace(/\/client$/, '/sessions');
  const response = await fetch(endpoint, {method:'POST', credentials:'same-origin', headers:{Authorization:'Basic ' + auth}});
  if (!response.ok) { status.textContent = 'Access denied. Check your operator account and private connection, then try again.'; return; }
  location.reload();
 } catch (_) { status.textContent = 'Connection unavailable. Restore your private connection and try again.'; }
});`
