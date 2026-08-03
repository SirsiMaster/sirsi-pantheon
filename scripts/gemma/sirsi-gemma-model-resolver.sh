#!/bin/bash
# sirsi-gemma-model-resolver.sh — keep the local Gemma worker on the LARGEST,
# MOST-ADVANCED, MOST-RECENT model that fits this machine's RAM (owner directive
# 2026-06-12: "always make sure we have the largest most advanced and most recent").
#
# Strategy:
#   1. Query mlx-community for the newest dense instruction-tuned Gemma
#      (it / instruct / assistant), preferring QAT variants (quality-at-low-bits).
#   2. Among the newest family, pick the LARGEST param size, then the HIGHEST-bit
#      quant whose on-disk size fits available RAM with headroom.
#   3. If not already cached, download it (background-safe).
#   4. Write the chosen model id to ~/.sirsi/gemma-model.conf (the worker reads it).
#
# Run on demand or on a schedule; safe to re-run (idempotent). Never downgrades
# below a known-good fallback if the network/API is unavailable.

set -u
CONF=$HOME/.sirsi/gemma-model.conf
LOG=$HOME/.sirsi/gemma-model-resolver.log
FALLBACK="mlx-community/gemma-4-12B-it-8bit"   # proven-good gemma-4, always available locally (non-gemma-4 caches removed 2026-07-02 per owner directive)
HF_CACHE=$HOME/.cache/huggingface/hub
mkdir -p "$(dirname "$CONF")"
log(){ echo "[$(date -u +%FT%TZ)] $*" >> "$LOG"; }

# The model the running broker was STARTED on. Used two ways: as a ranking
# tiebreak (hysteresis, see the ranking block) and to skip a pointless probe when
# the pick is already what is being served. Empty when no broker is running.
resident_model() {
  local sp; sp=$(cat "$HOME/.sirsi/gemma-server.pid" 2>/dev/null) || return 1
  [ -n "$sp" ] || return 1
  ps -o command= -p "$sp" 2>/dev/null | tr ' ' '\n' | grep -A1 -x -- '--model' | tail -1
}
RESIDENT=$(resident_model || true)

# Approx runtime GB-per-billion-params by quant. This deliberately estimates
# resident process footprint, not safetensors-on-disk bytes: MLX cache, KV cache,
# tokenizer/Python overhead, and activation buffers make the 8-bit 12B server
# measure 23-31GB on a 48GB host, not ~12.6GB.
gb_per_b() { case "$1" in 8bit|mxfp8) echo 2.60;; 6bit) echo 1.90;; 5bit) echo 1.50;; 4bit|qat-4bit|nvfp4|mxfp4) echo 0.93;; bf16) echo 3.20;; *) echo 1.20;; esac; }

# Total machine RAM (we size to TOTAL with a safety margin, not momentary free —
# the worker is short-lived and macOS will page the rest; but we cap so we never
# pick a model that can't fit physical RAM at all).
total_gb() { python3 -c "import subprocess;print(round(int(subprocess.check_output(['sysctl','-n','hw.memsize']))/1e9,1))"; }

pinned_gb() {
  # Pinned non-fleet residents are load-bearing local services the resolver must
  # not assume it can page away. Colima's VirtualMachine process is the canonical
  # case on this host: it anchors the local consensus ledger under ADR-040.
  ps -axo rss=,comm= 2>/dev/null | python3 -c '
import sys
total_kb=0
for line in sys.stdin:
    parts=line.strip().split(None,1)
    if len(parts) != 2:
        continue
    rss, name = parts
    base = name.rsplit("/", 1)[-1]
    if base == "com.apple.Virtualization.VirtualMachine":
        try: total_kb += int(rss)
        except ValueError: pass
print(round(total_kb / 1024 / 1024, 1))
'
}

resolve() {
  local total pinned; total=$(total_gb); pinned=$(pinned_gb)
  # DEFAULT-pick budget = 35% of physical RAM (floor 8GB). The old `total - 16`
  # arithmetic selected the 31B (~28.8GB wired) as the ALWAYS-ON default on a
  # 51.5GB box — its base footprint + the live desktop drove the machine to the
  # jetsam wall and system-wide delays (2026-07-16/17, twice). The owner-chosen
  # hybrid policy (2026-06-14) is: fleet-safe DEFAULT + big model ON-DEMAND
  # (`MODEL: max`, RAM-gated per request by the worker). This budget sizes only
  # the default; MAX_CONF stays the on-demand ceiling. On a 128GB box the same
  # rule promotes the 31B to default automatically.
  local budget; budget=$(python3 -c "print(max(8.0, round(($total - $pinned) * 0.35, 1)))")
  log "resolve: total=${total}GB pinned=${pinned}GB default-budget=${budget}GB (35% of unpinned steady-state rule)"

  # Newest gemma dense instruction models on mlx-community, newest first.
  local json
  json=$(curl -s --max-time 20 "https://huggingface.co/api/models?author=mlx-community&search=gemma&sort=createdAt&limit=60" 2>/dev/null)
  [ -z "$json" ] && { log "API unavailable — keeping current/fallback"; return 1; }

  # Pick: newest family (highest gemma-N), largest dense param size in that family,
  # prefer QAT, then highest-bit quant that fits budget. Skip MoE (A4B), embeddings,
  # vision-only, and tiny (e2b/e4b) variants for a router-reasoning worker.
  CHOSEN=$(echo "$json" | python3 -c "
import json,sys,re,os
budget=$budget
resident='$RESIDENT'
HUB=os.path.expanduser('~/.cache/huggingface/hub')
def cached(mid): return os.path.isdir(os.path.join(HUB,'models--'+mid.replace('/','--')))
gbper={'8bit':2.60,'6bit':1.90,'5bit':1.50,'4bit':0.93,'qat':0.93,'nvfp4':0.93,'mxfp4':0.93,'mxfp8':2.60,'bf16':3.20}  # runtime footprint, not disk bytes: 12B 8bit measures 23-31GB; gemma-4-31b qat-4bit measured ~28.8GB -> 0.93 GB/B
items=[m['id'] for m in json.load(sys.stdin)]
# Seed with already-downloaded models too: the API window is the 60 NEWEST uploads,
# so a perfectly good on-disk quant can fall outside it and never be considered.
local=[d[len('models--'):].replace('--','/',1) for d in (os.listdir(HUB) if os.path.isdir(HUB) else []) if d.startswith('models--')]
items=list(dict.fromkeys(items+local))
cand=[]
for i in items:
    lo=i.lower()
    if 'gemma' not in lo: continue
    if not re.search(r'(-it|instruct|assistant)', lo): continue
    if any(x in lo for x in ['embed','vision','-a4b','e2b','e4b','tiny','assistant','uncensored','heretic','unsloth','huihui','abliterated','msq']): continue  # skip custom forks mlx_lm can't parse
    if re.search(r'(?<!\d)[123]b(?![a-z0-9])', lo): continue  # tiny dense 1b/2b/3b — must not match 31b/12b (bare substring did, excluding every real candidate)
    mv=re.search(r'gemma-(\d+)', lo); fam=int(mv.group(1)) if mv else 0
    pv=re.search(r'(\d+)b', lo); params=int(pv.group(1)) if pv else 0
    qv=re.search(r'(8bit|6bit|5bit|4bit|nvfp4|mxfp4|mxfp8|bf16)', lo)
    q=qv.group(1) if qv else '4bit'
    bits=8 if q in('8bit','mxfp8') else 6 if q=='6bit' else 5 if q=='5bit' else 16 if q=='bf16' else 4
    qat=1 if 'qat' in lo else 0
    size=params*gbper.get(q,0.6)
    if size>budget: continue
    # rank: newest family, largest params, HIGHEST BITS that fit, then ALREADY
    # RESIDENT, then QAT, then already-on-disk, then name.
    #
    # RESIDENT OUTRANKS QAT -- hysteresis, and it is what stops the churn. Among
    # models that tie on family, params AND bits, the one the broker is already
    # serving wins. Rationale: at an identical bit depth, QAT-vs-plain is a
    # marginal quality difference, and it is not worth swapping a multi-GB model
    # to collect. Without this, 12B-it-qat-mxfp8 beat the resident 12B-it-8bit on
    # the qat key every single run, so conf permanently named a model the broker
    # had not started on. Probing that model then LOADED it as a SECOND resident
    # 12B (measured 2026-07-24: free RAM 82% -> 28%, ~23GB wired), and any probe
    # reading below floor SIGTERMd the broker to swap. A genuine upgrade (more
    # params, or more bits) still outranks the resident model and still swaps --
    # hysteresis only suppresses lateral moves, never real ones.
    #
    # BITS RANKS ABOVE QAT (fixed 2026-07-24). QAT sat above bits, which made ANY
    # qat variant outrank a higher-bit plain one — qat-mxfp8 (bits=8) permanently
    # beat the proven-good 12B-it-8bit FALLBACK, and once that was addressed
    # qat-6bit (bits=6) beat it too. Both contradict this script's own stated
    # strategy -- largest param size, then the HIGHEST-bit quant that fits. QAT
    # exists to recover quality lost to aggressive quantization, so it is a real
    # win BETWEEN EQUAL-BIT variants (qat-4bit over plain 4bit) and never a reason
    # to accept fewer bits. The old order also never let the pick settle: each run
    # re-chose a non-resident model, and any probe reading below floor stepped down
    # and SIGTERM'd the load-bearing broker (~2 bounces/hr on measurement noise).
    #
    # The cached flag stays AFTER bits so a small cached model can never outrank a
    # larger remote one (avoids a ~29GB download when an equivalent local quant
    # already exists).
    #
    # !! THIS WHOLE PYTHON BLOCK LIVES INSIDE A DOUBLE-QUOTED BASH STRING !!
    # So a double quote, a backtick, or dollar-paren in these comments is NOT
    # text: a double quote ENDS the string and truncates every line after it,
    # and the other two run as shell command substitution. The failure is silent
    # and misleading -- the block dies mid-parse, cand stays empty, and the
    # resolver logs no-fitting-candidate as if the API were down. Cost three
    # broken runs on 2026-07-24. Use plain ASCII prose only. Guarded by
    # test-gemma-model-resolver.sh, which runs this block for real.
    cand.append((fam, params, bits, 1 if i==resident else 0, qat, 1 if cached(i) else 0, i, round(size,1)))
if not cand:
    print(''); sys.exit()
cand.sort(reverse=True)
fam,params,bits,_res,qat,_loc,i,size=cand[0]
print(f'{i}\t{size}')
")
  [ -z "$CHOSEN" ] && { log "no fitting candidate — keeping fallback"; return 1; }
  MODEL=$(echo "$CHOSEN" | cut -f1); SIZE=$(echo "$CHOSEN" | cut -f2)
  log "selected: $MODEL (~${SIZE}GB on disk)"
  echo "$MODEL"
}

MODEL=$(resolve) || MODEL=""
if [ -z "$MODEL" ]; then
  # keep existing conf if present, else fallback
  [ -f "$CONF" ] && { log "keeping existing $(cat "$CONF")"; exit 0; }
  echo "$FALLBACK" > "$CONF"; log "wrote fallback $FALLBACK"; exit 0
fi

# Serve what EXISTS: if the chosen model is not on disk yet, conf points at the
# cached fallback until the background prefetch completes — otherwise the warm
# server (and every client) blocks on a multi-GB download while the machine has
# a perfectly good cached model (live gap 2026-07-17: conf named an uncached
# 12B-mxfp8; the broker restore fell back to serving the 31B instead).
is_cached() { [ -d "$HF_CACHE/models--$(echo "$1" | sed 's/\//--/')" ]; }
# Three cases, not two. The old `! is_cached MODEL && is_cached FALLBACK` guard
# fell through to `echo MODEL` whenever the FALLBACK was ALSO missing — so when
# the whole HF cache is gone (fresh box, cache cleared) conf got written with an
# UNCACHED model, the exact "conf names a model nobody has" failure this block
# exists to prevent. Refuse instead: leave conf untouched and exit non-zero so a
# caller sees the failure rather than a poisoned conf. Guarded by
# model-resolver-cache.test.sh.
if is_cached "$MODEL"; then
  echo "$MODEL" > "$CONF"
elif is_cached "$FALLBACK"; then
  log "chosen $MODEL not cached yet — conf serves cached $FALLBACK until the prefetch completes"
  echo "$FALLBACK" > "$CONF"
else
  log "REFUSING: neither $MODEL nor fallback $FALLBACK is cached under $HF_CACHE — conf unchanged"
  exit 1
fi
log "conf -> $(cat "$CONF")"

# INVARIANT: from here on MODEL is whatever conf actually serves. Without this the
# probe measured the CHOSEN model while clients used the cached fallback — so a
# fresh uncached pick got logged as "fit OK" on a decode nobody would ever run,
# and (worse) naming a non-resident model in the probe makes the broker LOAD it,
# a second multi-GB model alongside the resident one. Observed live 2026-07-24:
# free RAM fell 82% -> 28% (~23GB wired) inside one conduit pass.
MODEL=$(cat "$CONF")

# Warm/download via huggingface-cli if available (background-safe; mlx will also
# fetch on first use, but pre-warming avoids a cold first request).
if command -v huggingface-cli >/dev/null 2>&1; then
  log "pre-fetching $MODEL (background)"
  huggingface-cli download "$MODEL" >/dev/null 2>&1 &
fi

# ── EMPIRICAL fit test (router item 20260715-175752) ─────────────────────────
# Nominal RAM arithmetic said the 31B "fits"; measured decode was 0.5 tok/s —
# 30-50x below floor — because residency under the box's ACTUAL steady-state
# pressure, not disk size, is the constraint. So after selecting, probe the
# warm server once and assert a floor. Below floor → step down to the proven
# fallback and record why. No warm server → nothing to measure, keep the pick
# (the RAM-gated broker still protects the cold path, ADR-031).
TOKS_FLOOR=${GEMMA_TOKS_FLOOR:-5}
PORT=$( [ -f "$HOME/.sirsi/gemma-server.port" ] && cat "$HOME/.sirsi/gemma-server.port" || echo 11434 )
probe_toks() { # $1=model → echoes measured tok/s (integer), empty on unreachable
  python3 - "$1" "$PORT" <<'PY' 2>/dev/null
import sys, json, time, urllib.request
model, port = sys.argv[1], sys.argv[2]
body = json.dumps({"model": model, "messages":[{"role":"user","content":"Count from 1 to 20, digits only."}],
                   "max_tokens": 48, "temperature": 0.0}).encode()
req = urllib.request.Request(f"http://127.0.0.1:{port}/v1/chat/completions",
                             data=body, headers={"Content-Type":"application/json"})
t0 = time.time()
try:
    with urllib.request.urlopen(req, timeout=120) as r:
        toks = json.loads(r.read())["usage"]["completion_tokens"]
except Exception:
    sys.exit(0)
dt = time.time() - t0
print(int(toks / dt) if dt > 0 else 0)
PY
}
# BEST-OF-N (2026-07-24): a single probe is not a measurement, it is a sample of a
# contended machine. Host load only ever makes decode SLOWER — it cannot make it
# faster — so the MAX across N samples is the least-contaminated estimate of the
# model's true decode rate, and a false-low needs all N to be slow instead of one.
# This matters because below-floor does not merely log: it rewrites conf AND
# SIGTERMs the load-bearing broker. Measured on this box, one model inside one
# hour: 2, 8, 25, 2, 30 tok/s — a 15x spread across a 5 tok/s floor, i.e. the old
# single sample was deciding broker restarts on noise. Bail out early on the first
# sample that clears the floor: the common healthy case still costs one call.
best_toks() { # $1=model → echoes best-of-N tok/s (empty if every sample unreachable)
  local best="" r n=${GEMMA_PROBE_SAMPLES:-3}
  for _ in $(seq "$n"); do
    r=$(probe_toks "$1")
    [ -z "$r" ] && continue
    { [ -z "$best" ] || [ "$r" -gt "$best" ]; } && best=$r
    [ -n "$best" ] && [ "$best" -ge "$TOKS_FLOOR" ] && break
  done
  echo "$best"
}
# RESIDENT-AWARE SKIP (2026-07-24). probe_toks NAMES a model, so probing anything
# the broker does not already have loaded makes it LOAD that model — a second
# multi-GB set of weights alongside the resident one, left resident afterwards.
# In steady state (chosen == what the broker is already serving) there is nothing
# to learn from a probe and a real ~12GB cost to running one, so skip it entirely.
# The probe's actual job is to catch "nominal RAM says it fits but it thrashes"
# (the 31B case, router item 20260715-175752) — that only arises when a SWAP is on
# the table, which is exactly when we still probe.
if [ -n "${RESIDENT:-}" ] && [ "$RESIDENT" = "$MODEL" ]; then
  log "steady state: broker already serving $MODEL — probe skipped (no second model load)"
elif curl -s -o /dev/null -m 3 "http://127.0.0.1:$PORT/v1/models" 2>/dev/null; then
  # WARMUP-DISCARD (2026-07-24, same class as the triage probe): this probe names
  # $MODEL explicitly, so if that model is not already resident the server loads
  # it (~45s) and `toks/dt` divides 48 tokens by the LOAD, not the decode —
  # a false 1-4 tok/s. Here that is worse than a bad log line: below-floor
  # STEPS DOWN to $FALLBACK and bounces the server, so one cold-load reading can
  # silently downgrade the fabric's model. Observed live: the same model measured
  # 8 tok/s cold and 25 tok/s warm 15 min apart. Discard the first (load-bearing)
  # call, then measure warm decode.
  probe_toks "$MODEL" >/dev/null
  rate=$(best_toks "$MODEL")
  if [ -n "$rate" ] && [ "$rate" -lt "$TOKS_FLOOR" ]; then
    if [ "$MODEL" != "$FALLBACK" ]; then
      log "EMPIRICAL FIT FAIL: $MODEL measured ${rate} tok/s < ${TOKS_FLOOR} floor — stepping down to $FALLBACK"
      MODEL=$FALLBACK
      echo "$MODEL" > "$CONF"
      # Bounce the warm server so the too-big model's WIRED weights are actually
      # RELEASED — conf only changes what clients request; the server keeps every
      # loaded model resident (JetsamEvent 2026-07-16: the 31B's ~30 GB base
      # footprint, not the bounded KV cache, is what drove the system to the
      # wall). Target the SERVER PROCESS itself: ai.sirsi.gemma is a one-shot
      # ensure-warm LAUNCHER (KeepAlive=false) — kickstarting it does NOT
      # restart the detached python server (verified live 2026-07-17: the old
      # server survived a kickstart holding ~31 GB wired). SIGTERM the server,
      # then re-warm through the RAM-gated launcher (ADR-031 resize, not kill).
      # DRAIN GUARD (broker death #4, 2026-07-23): never bounce a broker with
      # in-flight work — the 14:01Z swap SIGTERM'd mid-triage-batch and the
      # batch died at item 15/24. Established client connections to the broker
      # port ARE the in-flight truth (no log parsing, no protocol change):
      # any present → defer the swap to the next resolver run, conf already
      # points at the new model so the swap completes on the first idle pass.
      if lsof -nP -iTCP:"$PORT" -sTCP:ESTABLISHED 2>/dev/null | grep -q .; then
        log "DEFERRED model swap: broker has in-flight request(s) on :$PORT — will swap on next idle run"
        exit 0
      fi
      pkill -TERM -f "gemma-capped-server.py" 2>/dev/null \
        && log "stopped gemma-capped-server to release the oversized model's wired memory" \
        || log "no running gemma-capped-server to stop"
      sleep 3
      "$HOME/.local/bin/sirsi" gemma serve >/dev/null 2>&1 \
        && log "re-warmed broker via sirsi gemma serve (RAM-gated)" \
        || log "re-warm refused/failed — broker's RAM gate will retry on next use"
      sleep 8   # let the reloaded server come up before re-probing
      rate=$(probe_toks "$MODEL")
      log "fallback $MODEL measured ${rate:-unreachable} tok/s"
    else
      log "EMPIRICAL FIT FAIL on fallback itself: ${rate} tok/s < ${TOKS_FLOOR} — machine under memory pressure, keeping conf (nothing smaller to offer)"
    fi
  else
    log "empirical fit OK: $MODEL at ${rate:-unmeasured} tok/s (floor ${TOKS_FLOOR})"
  fi
else
  log "warm server unreachable — skipped empirical probe (cold path stays broker-gated)"
fi
echo "$MODEL"
