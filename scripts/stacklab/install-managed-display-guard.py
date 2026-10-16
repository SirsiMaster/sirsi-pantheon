#!/usr/bin/env python3
"""M5 deployment recipe: preserve other hooks; refuse conflicting managed policy."""
import hashlib,pathlib,subprocess,sys
def require(ok, message):
 if not ok: raise RuntimeError(message)
expected='e08b331ed8e84b84d41cc5f9b5d4b8224af54da3a70185ecb66a84d843a87206'
source=pathlib.Path(sys.argv[1]);policy=pathlib.Path(__file__).with_name('managed-requirements.toml')
require(hashlib.sha256(source.read_bytes()).hexdigest()==expected,'guard hash mismatch')
require(hashlib.sha256(policy.read_bytes()).hexdigest()=='370956d366430011542ec6303e0c4eaa3e2290f81aab8fd91905e0d4ebcb9156','policy hash mismatch')
root=pathlib.Path('/etc/codex');hooks=root/'hooks';destination=root/'requirements.toml'
for p in [root,hooks]:
 if p.exists():
  st=p.lstat();require(not p.is_symlink() and st.st_uid==0 and not st.st_mode&0o022,f'unsafe managed directory: {p}')
for src,dst in [(source,hooks/'sirsi-display-power-guard'),(policy,destination)]:
 if dst.exists() or dst.is_symlink():
  require(not dst.is_symlink() and dst.read_bytes()==src.read_bytes(),f'conflicting installed artifact: {dst}')
  st=dst.stat();require(st.st_uid==0 and not st.st_mode&0o022,f'unsafe installed artifact: {dst}')
if destination.exists():
 require((hooks/'sirsi-display-power-guard').is_file(),'managed policy exists but guard is missing')
 print('PASS: identical root-owned managed guard and policy already installed; no change')
 sys.exit(0)
subprocess.run(['sudo','-n','install','-d','-o','root','-g','wheel','-m','755',str(hooks)],check=True)
subprocess.run(['sudo','-n','install','-o','root','-g','wheel','-m','755',str(source),str(hooks/'sirsi-display-power-guard')],check=True)
subprocess.run(['sudo','-n','install','-o','root','-g','wheel','-m','644',str(policy),str(destination)],check=True)
print('Installed managed guard and policy. Verify native runtime before admitting any workload.')
