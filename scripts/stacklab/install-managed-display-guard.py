#!/usr/bin/env python3
"""M5 deployment recipe: preserve other hooks; refuse conflicting managed policy."""
import hashlib,pathlib,subprocess,sys
def require(ok, message):
 if not ok: raise RuntimeError(message)
expected='88d7b791721af3eeeba167e18f7161b2201e90a677218da400ee193c618aa840'
source=pathlib.Path(sys.argv[1]);policy=pathlib.Path(__file__).with_name('managed-requirements.toml')
require(hashlib.sha256(source.read_bytes()).hexdigest()==expected,'guard hash mismatch')
require(hashlib.sha256(policy.read_bytes()).hexdigest()=='66ee45441852eee31946722a6ea4cdbc1c3267845bb997f153d665a9d83f14c8','policy hash mismatch')
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
