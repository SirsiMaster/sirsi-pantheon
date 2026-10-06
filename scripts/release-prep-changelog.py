import re,sys,datetime
ver=sys.argv[1]
s=open('CHANGELOG.md').read()
# Split into sections at "## [" headings. EVERY "## [Unreleased]" block is unreleased
# work (hand-edited branches and union merges leave several); the old code cut only the
# first, so the rest were never released in the notes. Cut them all, leave one empty block.
parts=re.split(r'(?m)^(?=## \[)',s)
preamble=parts[0]; secs=parts[1:]
unrel=[p for p in secs if p.startswith('## [Unreleased]')]
versioned=[p for p in secs if not p.startswith('## [Unreleased]')]
released='\n'.join(versioned)
def bullets(b):
    out=[];cur=[]
    for line in b.split('\n'):
        if line.startswith('- '):
            if cur: out.append('\n'.join(cur))
            cur=[line]
        elif cur and (line.startswith('  ') or line.strip()==''):
            cur.append(line)
        elif line.startswith('#'):
            if cur: out.append('\n'.join(cur)); cur=[]
    if cur: out.append('\n'.join(cur))
    return [b.rstrip() for b in out]
new=[];seen=set()
for blk in unrel:
    for b in bullets(blk):
        key=re.sub(r'\W+',' ',b[:70]).strip().lower()
        if key and key not in seen and key not in re.sub(r'\W+',' ',released).lower():
            seen.add(key); new.append(b)
assert new, "no new bullets"
sec='## [%s] — %s\n\n%s\n'%(ver,datetime.date.today().isoformat(),'\n\n'.join(new))
open('CHANGELOG.md','w').write(preamble+'## [Unreleased]\n\n'+sec+'\n'+'\n'.join(versioned) if versioned else preamble+'## [Unreleased]\n\n'+sec)
open('VERSION','w').write(ver+'\n')
# stacklab canon changelog: this release's entry titles under the newest heading
p='docs/stacklab/pantheon-pt/canon/CHANGELOG.md'; t=open(p).read()
titles=[(re.search(r'\*\*(.+?)\*\*',b).group(1) if re.search(r'\*\*(.+?)\*\*',b) else b[2:90].rstrip('.')) for b in new]
add='## %s — v%s release candidate\n\n%s\n\n'%(datetime.date.today().isoformat(),ver,'\n'.join('- '+x for x in titles))
m=re.search(r'^## ',t,re.M); assert m, "no heading in canon changelog"
open(p,'w').write(t[:m.start()]+add+t[m.start():])
print(len(new),"bullets")
