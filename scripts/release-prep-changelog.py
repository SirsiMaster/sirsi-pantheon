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
        elif line.startswith('## '):
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
# stacklab canon changelog: this release's entry titles under the newest heading
p='docs/stacklab/pantheon-pt/canon/CHANGELOG.md'; t=open(p).read()
titles=[(re.search(r'\*\*(.+?)\*\*',b).group(1) if re.search(r'\*\*(.+?)\*\*',b) else b[2:90].rstrip('.')) for b in new]
add='## %s — v%s release candidate\n\n%s\n\n'%(datetime.date.today().isoformat(),ver,'\n'.join('- '+x for x in titles))
m=re.search(r'^## ',t,re.M); assert m, "no heading in canon changelog"
open(p,'w').write(t[:m.start()]+add+t[m.start():])
print(len(new),"bullets")
