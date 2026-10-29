#!/bin/bash
# G3 two-host contention, Postgres backend. Same rounds, driver and tally as the 2026-10-26 SQLite run
# (../ADR-062-G3-TWO-HOST-CONTENTION-20261026/), with the test router's store swapped for a disposable
# local Postgres 16 cluster (trust auth, 127.0.0.1 only, deleted at the end). Never touches the production
# router or Cloud SQL: own HOME per host, own client agent ids, explicit test URL.
set -u
S="$(cd "$(dirname "$0")" && pwd)"
G3=${G3:-$S/../ADR-062-G3-TWO-HOST-CONTENTION-20261026}   # drv.py + tally.py
M5=thekryptodragon@100.92.193.26
IP=100.88.242.95
OWNER=ra
REPO=/Users/sirsimasterdev/Development/sirsi-pantheon
M5REPO=/Users/thekryptodragon/Development/sirsi-pantheon
M5SIRSI=/Users/thekryptodragon/.local/bin/sirsi
PG=/opt/homebrew/opt/postgresql@16/bin
PGP=55433
TAG=v0.24.143   # pg/schema.sql from the tag the installed sirsi was built from
TOK=$(openssl rand -hex 24)
rm -rf "$S"/run && mkdir -p "$S/run/home-m1"
R="$S/run"
cd $REPO
export G3HOME="$R/home-m1"

echo "== disposable Postgres on 127.0.0.1:$PGP"
export LC_ALL=C   # postmaster refuses an unset/invalid locale ("became multithreaded during startup")
$PG/initdb -D "$R/pgdata" -U postgres --auth=trust -E UTF8 >"$R/initdb.log" 2>&1 || { echo "ABORT: initdb"; exit 1; }
$PG/pg_ctl -D "$R/pgdata" -l "$R/pg.log" -w -o "-p $PGP -c unix_socket_directories='' -c listen_addresses=127.0.0.1" start >/dev/null || { echo "ABORT: pg start"; exit 1; }
trap '$PG/pg_ctl -D "$R/pgdata" -m fast stop >/dev/null 2>&1' EXIT   # an abort never leaves the test postmaster up
$PG/postgres --version | tee "$R/pg-version.txt"
git -C $REPO show $TAG:internal/routerstore/pg/roles.sql >"$R/roles.sql"
git -C $REPO show $TAG:internal/routerstore/pg/schema.sql >"$R/schema.sql"
$PG/psql -q -h 127.0.0.1 -p $PGP -U postgres -d postgres -v ON_ERROR_STOP=1 -f "$R/roles.sql" || { echo "ABORT: roles"; exit 1; }
for d in base g3 left right; do
  $PG/createdb -h 127.0.0.1 -p $PGP -U postgres -O router_migrator $d
  $PG/psql -q -h 127.0.0.1 -p $PGP -U router_migrator -d $d -v ON_ERROR_STOP=1 -f "$R/schema.sql" || { echo "ABORT: schema $d"; exit 1; }
done
dsn() { echo "postgres://router_service@127.0.0.1:$PGP/$1?sslmode=disable"; }

serve() { # db port delay
  (cd "$R" && HOME="$R/home-m1" SIRSI_ROUTER_SERVE_TOKEN=$TOK SIRSI_ROUTER_RULE_OF_RA=off SIRSI_ROUTER_URL= SIRSI_ROUTER_DB= \
     SIRSI_ROUTER_SERVE_TEST_DELAY=$3 exec sirsi router serve --store "$(dsn $1)" --listen $IP:$2) >> "$R/$1.serve.log" 2>&1 &
  echo $!
}
up() { # port — wait for healthz
  for _ in $(seq 50); do curl -fsS -o /dev/null "http://$IP:$1/v1/healthz" && return 0; sleep 0.2; done
  echo "ABORT: serve on :$1 never healthy"; return 1
}
seed() { # db prefix n — through an undelayed test service, so the store's own invariants apply
  local p; p=$(serve $1 18764 0s); up 18764 || exit 1
  python3 - "$2" "$3" <<PY
import subprocess, sys, os
pre, n = sys.argv[1], int(sys.argv[2])
env = dict(os.environ, SIRSI_ROUTER_URL="http://$IP:18764", SIRSI_ROUTER_TOKEN="$TOK", SIRSI_ROUTER_DB="", SIRSI_AGENT_ID="$OWNER", HOME=os.environ["G3HOME"])
for i in range(n):
    subprocess.run(["sirsi","router","task","add","$OWNER",f"{pre}{i:04d}","--subject",f"{pre}{i:04d}","--quiet"], env=env, check=True, stdout=subprocess.DEVNULL)
PY
  kill $p; wait $p 2>/dev/null
}
m5run() { # url prefix n t0(M1 clock) period out — shifted onto the M5 clock
  local t; t=$(python3 -c "print($4+$OFF)")
  printf '%s\n' "$TOK" | ssh -o BatchMode=yes $M5 "mkdir -p /tmp/g3home && cd $M5REPO && HOME=/tmp/g3home python3 /tmp/g3drv.py $M5SIRSI $1 g3-m5 $OWNER $2 $3 $t $5 $6"
}
m1run() { # url prefix n t0 period out — fires LAG later so its (local) request lands with the M5's
  local t; t=$(python3 -c "print($4+$LAG)")
  printf '%s\n' "$TOK" | HOME="$R/home-m1" python3 "$G3/drv.py" "$(command -v sirsi)" "$1" g3-m1 $OWNER $2 $3 $t $5 "$6"
}
ssh -o BatchMode=yes $M5 'cat > /tmp/g3drv.py' < "$G3/drv.py"

echo "== clock offset (M5 - M1, s), 5 samples"
for k in 1 2 3 4 5; do
  a=$(python3 -c 'import time;print(time.time())'); b=$(ssh -o BatchMode=yes $M5 "python3 -c 'import time;print(time.time())'"); c=$(python3 -c 'import time;print(time.time())')
  python3 -c "a,b,c=$a,$b,$c;print(f'offset {b-(a+c)/2:+.4f}  rtt {c-a:.3f}')"
done | tee "$R/clock.txt"
OFF=$(python3 -c "import statistics as s;print(round(s.median(float(l.split()[1]) for l in open('$R/clock.txt')),4))")
LAG=$(python3 -c "import statistics as s;print(round(s.median(float(l.split()[3]) for l in open('$R/clock.txt'))/2,4))")
echo "M5 clock offset median ${OFF}s; M1 fire lag ${LAG}s (half median RTT)" | tee -a "$R/clock.txt"

echo "== phase A: baseline p99 (100 uncontended claims from the M5, no delay)"
seed base b 100
SP=$(serve base 18762 0s); up 18762 || exit 1
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

echo "== phase B: 1000 contended rounds, both Macs, shared Postgres ledger, delay $DELAY"
seed g3 r 1000
SP=$(serve g3 18762 $DELAY); up 18762 || exit 1
PER=$(python3 -c "print(round(($P99*2+3*$P99)/1000+0.15,3))")
echo "period ${PER}s" | tee -a "$R/delay.txt"
T0=$(python3 -c 'import time;print(time.time()+8)')
m5run http://$IP:18762 r 1000 $T0 $PER /tmp/g3-m5.tsv &
M5P=$!
m1run http://$IP:18762 r 1000 $T0 $PER "$R/m1.tsv"
wait $M5P
kill $SP; wait $SP 2>/dev/null
scp -q -o BatchMode=yes $M5:/tmp/g3-m5.tsv "$R/m5.tsv"
echo "== store readback: claimed rows in the shared ledger"
$PG/psql -At -h 127.0.0.1 -p $PGP -U postgres -d g3 -c "SELECT status, count(*) FROM router.tasks GROUP BY status ORDER BY 1" | tee "$R/g3-readback.txt"

echo "== phase C: negative control, 50 rounds, SEPARATE Postgres databases per host"
seed left c 50; seed right c 50
SL=$(serve left 18762 $DELAY); SR=$(serve right 18763 $DELAY); up 18762 && up 18763 || exit 1
T0=$(python3 -c 'import time;print(time.time()+6)')
m5run http://$IP:18763 c 50 $T0 $PER /tmp/g3-ctl-m5.tsv &
M5P=$!
m1run http://$IP:18762 c 50 $T0 $PER "$R/ctl-m1.tsv"
wait $M5P
kill $SL $SR; wait $SL $SR 2>/dev/null
scp -q -o BatchMode=yes $M5:/tmp/g3-ctl-m5.tsv "$R/ctl-m5.tsv"
# serve logs print the store; keep the host's scratch layout out of committed evidence
sed -i '' "s|$R|<run>|g" "$R"/*.serve.log

echo "== tally"
python3 "$G3/tally.py" "$R"
echo "== cleanup"
ssh -o BatchMode=yes $M5 'rm -rf /tmp/g3home /tmp/g3drv.py /tmp/g3-base.tsv /tmp/g3-m5.tsv /tmp/g3-ctl-m5.tsv' && echo "M5 scratch removed"
$PG/pg_ctl -D "$R/pgdata" -m fast stop >/dev/null && rm -rf "$R/pgdata" && echo "Postgres stopped and data dir removed"
rm -rf "$R/home-m1" && echo "M1 test HOME removed"
