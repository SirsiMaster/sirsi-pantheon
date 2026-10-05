import re,sys,datetime
ver=sys.argv[1]
s=open('CHANGELOG.md').read()
i=s.index('## [Unreleased]'); j=s.index('\n## [',i+5)  # end of Unreleased block
block=s[i:j]
released=s[j:]
def bullets(b):
    out=[];cur=[]
    for line in b.split('\n'):
        if line.startswith('- '):
            if cur: out.append('\n'.join(cur))
            cur=[line]
        elif cur and (line.startswith('  ') or line.strip()==''):
            cur.append(line)
        elif line.startswith('## ') : 
            if cur: out.append('\n'.join(cur)); cur=[]
    if cur: out.append('\n'.join(cur))
    return [b.rstrip() for b in out]
new=[]
for b in bullets(block):
    key=re.sub(r'\W+',' ',b[:70]).strip().lower()
    if key and key not in re.sub(r'\W+',' ',released).lower(): new.append(b)
assert new, "no new bullets"
sec='## [%s] — %s\n\n%s\n\n'%(ver,datetime.date.today().isoformat(),'\n\n'.join(new))
s=s[:j+1]+sec.lstrip('\n')+s[j+1:] if False else s[:j]+'\n'+sec.rstrip('\n')+'\n'+s[j:]
open('CHANGELOG.md','w').write(s)
open('VERSION','w').write(ver+'\n')
# stacklab canon changelog
p='docs/stacklab/pantheon-pt/canon/CHANGELOG.md'; t=open(p).read()
m='## 2026-10-01 — v0.24.65 release candidate'
add='## %s — v%s release candidate\n\n- Records the lease/session-identity fixes (per-thread session cache, dispatch-contract agent id), the ps-free thread anchor, the pre-push window gate, the bind router-rejection check, the gemma status default port, who-is-on live activity, the ADR-070 revision and the doctor name-conformance report from the exact tested mainline.\n\n'%(datetime.date.today().isoformat(),ver)
assert m in t; open(p,'w').write(t.replace(m,add+m,1))
print(len(new),"bullets")
