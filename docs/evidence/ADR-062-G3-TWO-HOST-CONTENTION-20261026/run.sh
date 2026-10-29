#!/bin/bash
# G3 two-host contention run. Disposable test router on the M1 (scratch SQLite, Tailscale IP only).
# Never touches the production router: own HOME per host, own client agent ids, explicit test URL.
set -u
S="$(cd "$(dirname "$0")" && pwd)"
M5=thekryptodragon@100.92.193.26
IP=100.88.242.95
OWNER=ra
REPO=/Users/sirsimasterdev/Development/sirsi-pantheon
M5REPO=/Users/thekryptodragon/Development/sirsi-pantheon
M5SIRSI=/Users/thekryptodragon/.local/bin/sirsi
TOK=$(openssl rand -hex 24)
rm -rf "$S"/run && mkdir -p "$S/run/home-m1"
R="$S/run"
cd $REPO
seed() { # db prefix n
  python3 - "$1" "$2" "$3" <<'PY'
import subprocess, sys, os
db, pre, n = sys.argv[1], sys.argv[2], int(sys.argv[3])
env = dict(os.environ, SIRSI_ROUTER_URL="", SIRSI_ROUTER_DB=db, HOME=os.environ["G3HOME"])
for i in range(n):
    subprocess.run(["sirsi","router","task","add","ra",f"{pre}{i:04d}","--subject",f"{pre}{i:04d}","--quiet"], env=env, check=True, stdout=subprocess.DEVNULL)
PY
}
export G3HOME="$R/home-m1"
serve() { # db port delay
  (cd "$R" && HOME="$R/home-m1" SIRSI_ROUTER_SERVE_TOKEN=$TOK SIRSI_ROUTER_RULE_OF_RA=off SIRSI_ROUTER_URL= \
     SIRSI_ROUTER_SERVE_TEST_DELAY=$3 exec sirsi router serve --store "$1" --listen $IP:$2) > "$1.serve.log" 2>&1 &
  echo $!
}
m5run() { # url prefix n t0(M1 clock) period out — shifted onto the M5 clock
  local t; t=$(python3 -c "print($4+$OFF)")
  printf '%s\n' "$TOK" | ssh -o BatchMode=yes $M5 "mkdir -p /tmp/g3home && cd $M5REPO && HOME=/tmp/g3home python3 /tmp/g3drv.py $M5SIRSI $1 g3-m5 $OWNER $2 $3 $t $5 $6"
}
m1run() { # url prefix n t0 period out — fires LAG later so its (local) request lands with the M5's
  local t; t=$(python3 -c "print($4+$LAG)")
  printf '%s\n' "$TOK" | HOME="$R/home-m1" python3 "$S/drv.py" "$(command -v sirsi)" "$1" g3-m1 $OWNER $2 $3 $t $5 "$6"
}
ssh -o BatchMode=yes $M5 'cat > /tmp/g3drv.py' < "$S/drv.py"

echo "== clock offset (M5 - M1, s), 5 samples"
for k in 1 2 3 4 5; do
  a=$(python3 -c 'import time;print(time.time())'); b=$(ssh -o BatchMode=yes $M5 "python3 -c 'import time;print(time.time())'"); c=$(python3 -c 'import time;print(time.time())')
  python3 -c "a,b,c=$a,$b,$c;print(f'offset {b-(a+c)/2:+.4f}  rtt {c-a:.3f}')"
done | tee "$R/clock.txt"
OFF=$(python3 -c "import statistics as s;print(round(s.median(float(l.split()[1]) for l in open('$R/clock.txt')),4))")
LAG=$(python3 -c "import statistics as s;print(round(s.median(float(l.split()[3]) for l in open('$R/clock.txt'))/2,4))")
echo "M5 clock offset median ${OFF}s; M1 fire lag ${LAG}s (half median RTT)" | tee -a "$R/clock.txt"

echo "== phase A: baseline p99 (100 uncontended claims from the M5, no delay)"
seed "$R/base.db" b 100
SP=$(serve "$R/base.db" 18762 0s); sleep 2
T0=$(python3 -c 'import time;print(time.time()+3)')
m5run http://$IP:18762 b 100 $T0 0.25 /tmp/g3-base.tsv
[ -n "$(ssh -o BatchMode=yes $M5 'cat /tmp/g3-base.tsv 2>/dev/null | head -1')" ] || { echo "ABORT: phase A produced nothing"; kill $SP; exit 1; }
kill $SP; wait $SP 2>/dev/null
scp -q -o BatchMode=yes $M5:/tmp/g3-base.tsv "$R/base.tsv"
P99=$(python3 -c "
import sys
ms=sorted(float(l.split('\t')[2]) for l in open('$R/base.tsv'))
wins=sum(1 for l in open('$R/base.tsv') if l.rstrip().endswith('win'))
p=ms[int(0.99*len(ms))-1]; print(f'{p:.0f}')
print(f'baseline: n={len(ms)} wins={wins} p50={ms[len(ms)//2]:.0f}ms p99={p:.0f}ms', file=sys.stderr)")
DELAY=$((P99*2))ms
echo "TEST DELAY = 2 x p99 = $DELAY" | tee "$R/delay.txt"

echo "== phase B: 1000 contended rounds, both Macs, shared ledger, delay $DELAY"
seed "$R/g3.db" r 1000
SP=$(serve "$R/g3.db" 18762 $DELAY); sleep 2
PER=$(python3 -c "print(round(($P99*2+3*$P99)/1000+0.15,3))")
echo "period ${PER}s" | tee -a "$R/delay.txt"
T0=$(python3 -c 'import time;print(time.time()+8)')
m5run http://$IP:18762 r 1000 $T0 $PER /tmp/g3-m5.tsv &
M5P=$!
m1run http://$IP:18762 r 1000 $T0 $PER "$R/m1.tsv"
wait $M5P
kill $SP; wait $SP 2>/dev/null
scp -q -o BatchMode=yes $M5:/tmp/g3-m5.tsv "$R/m5.tsv"

echo "== phase C: negative control, 50 rounds, SEPARATE ledgers per host"
seed "$R/left.db" c 50; seed "$R/right.db" c 50
SL=$(serve "$R/left.db" 18762 $DELAY); SR=$(serve "$R/right.db" 18763 $DELAY); sleep 2
T0=$(python3 -c 'import time;print(time.time()+6)')
m5run http://$IP:18763 c 50 $T0 $PER /tmp/g3-ctl-m5.tsv &
M5P=$!
m1run http://$IP:18762 c 50 $T0 $PER "$R/ctl-m1.tsv"
wait $M5P
kill $SL $SR; wait $SL $SR 2>/dev/null
scp -q -o BatchMode=yes $M5:/tmp/g3-ctl-m5.tsv "$R/ctl-m5.tsv"
# serve logs print the absolute store path; keep the host's scratch layout out of committed evidence
sed -i '' "s|$R|<run>|g" "$R"/*.serve.log

echo "== tally"
python3 "$S/tally.py" "$R"
echo "== cleanup M5 scratch"
ssh -o BatchMode=yes $M5 'rm -rf /tmp/g3home /tmp/g3drv.py /tmp/g3-base.tsv /tmp/g3-m5.tsv /tmp/g3-ctl-m5.tsv' && echo "M5 scratch removed"
rm -rf "$R/home-m1" && echo "M1 test HOME removed"
