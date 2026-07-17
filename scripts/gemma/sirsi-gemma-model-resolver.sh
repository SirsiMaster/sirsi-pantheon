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

# Approx GB-per-billion-params by quant (safetensors on disk ≈ RAM at load).
gb_per_b() { case "$1" in 8bit) echo 1.05;; 6bit) echo 0.80;; 5bit) echo 0.66;; 4bit|qat-4bit|nvfp4|mxfp4) echo 0.55;; *) echo 0.6;; esac; }

# Total machine RAM (we size to TOTAL with a safety margin, not momentary free —
# the worker is short-lived and macOS will page the rest; but we cap so we never
# pick a model that can't fit physical RAM at all).
total_gb() { python3 -c "import subprocess;print(round(int(subprocess.check_output(['sysctl','-n','hw.memsize']))/1e9,1))"; }

resolve() {
  local total; total=$(total_gb)
  # DEFAULT-pick budget = 35% of physical RAM (floor 8GB). The old `total - 16`
  # arithmetic selected the 31B (~28.8GB wired) as the ALWAYS-ON default on a
  # 51.5GB box — its base footprint + the live desktop drove the machine to the
  # jetsam wall and system-wide delays (2026-07-16/17, twice). The owner-chosen
  # hybrid policy (2026-06-14) is: fleet-safe DEFAULT + big model ON-DEMAND
  # (`MODEL: max`, RAM-gated per request by the worker). This budget sizes only
  # the default; MAX_CONF stays the on-demand ceiling. On a 128GB box the same
  # rule promotes the 31B to default automatically.
  local budget; budget=$(python3 -c "print(max(8.0, round($total * 0.35, 1)))")
  log "resolve: total=${total}GB default-budget=${budget}GB (35% steady-state rule)"

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
HUB=os.path.expanduser('~/.cache/huggingface/hub')
def cached(mid): return os.path.isdir(os.path.join(HUB,'models--'+mid.replace('/','--')))
gbper={'8bit':1.05,'6bit':0.93,'5bit':0.85,'4bit':0.93,'qat':0.93,'nvfp4':0.6,'mxfp4':0.6,'mxfp8':1.0,'bf16':2.0}  # measured: gemma-4-31b qat-4bit = 28.8GB → ~0.93 GB/B (QAT keeps hi-precision embeds/scales)
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
    # rank: newest family, largest params, QAT preferred, highest bits that fit,
    # then already-on-disk (tiebreak ONLY among otherwise-equal models — must stay
    # after bits so a small cached model can never outrank a larger remote one;
    # avoids a ~29GB download when an equivalent local quant already exists), then name
    cand.append((fam, params, qat, bits, 1 if cached(i) else 0, i, round(size,1)))
if not cand:
    print(''); sys.exit()
cand.sort(reverse=True)
fam,params,qat,bits,_loc,i,size=cand[0]
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

# Cache check (HF cache dir uses models--org--name)
cache_dir="$HF_CACHE/models--$(echo "$MODEL" | tr '/' '-' | sed 's/-/--/')"
# (the tr/sed above is approximate; rely on mlx to no-op if already present)
echo "$MODEL" > "$CONF"
log "conf -> $MODEL"

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
if curl -s -o /dev/null -m 3 "http://127.0.0.1:$PORT/v1/models" 2>/dev/null; then
  rate=$(probe_toks "$MODEL")
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
