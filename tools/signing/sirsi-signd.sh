#!/bin/zsh
# sirsi-signd.sh — the Sirsi signing service (runs on the ONE signing Mac, the M5, as a LaunchAgent
# in the owner's logged-in session, where the login keychain is unlocked). Jobs arrive in
# ~/sirsi-sign/inbox/<id>/ (one file or .app + `kind` [bin|app|pkg|dmg] + `caller`). Each job is signed
# with the Team 9D382WV988 Developer ID identities, notarized with the `sirsi-notary` keychain
# profile, stapled where Apple allows, and written to ~/sirsi-sign/outbox/<id>/ with receipt.txt and
# `done` (or `failed`). Every job appends one line to ~/sirsi-sign/audit.log (append-only record of
# caller, kind, input/output sha256, notary id, result). Keys never leave this Mac.
set -u
setopt nullglob
export PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin
APP="Developer ID Application: Sirsi Technologies Inc. (9D382WV988)"
INST="Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)"
PROFILE=sirsi-notary
B=$HOME/sirsi-sign; mkdir -p "$B/inbox" "$B/outbox" "$B/work"
mkdir "$B/.lock" 2>/dev/null || exit 0; trap 'rmdir "$B/.lock"' EXIT   # one runner at a time
SAFE='^[A-Za-z0-9._-]+$'

for J in "$B"/inbox/*(/); do
  id=${J:t}
  [[ $id =~ '^[0-9a-f-]{36}$' ]] || { rm -rf -- "$J"; continue; }      # only uuid job ids
  [ -f "$J/kind" ] || continue
  kind=$(head -c 8 -- "$J/kind" | tr -dc 'a-z')
  caller=$(head -c 64 -- "$J/caller" 2>/dev/null | tr -dc 'A-Za-z0-9@._-')
  entries=("$J"/*(N)); entries=(${entries:#$J/kind}); entries=(${entries:#$J/caller})
  O=$B/outbox/$id; W=$B/work/$id; R=$O/receipt.txt
  rm -rf -- "$W"; mkdir -p "$O" "$W"
  fail() { print -r -- "REJECTED: $1" > "$R"; touch "$O/failed"; rm -rf -- "$J" "$W"
           print -r -- "$(date -u +%FT%TZ) id=$id caller=${caller:-?} kind=${kind:-?} result=rejected reason=$1" >> "$B/audit.log"; }
  case $kind in codesign|bin|app|pkg|dmg) ;; *) fail "bad kind"; continue;; esac
  (( ${#entries} == 1 )) || { fail "expected exactly one file"; continue; }
  f=${entries[1]:t}
  [[ $f =~ $SAFE ]] || { fail "unsafe file name"; continue; }
  mv -- "$J/$f" "$W/$f"; rm -rf -- "$J"
  in=$W/$f; out=$in; ok=1; nid=none
  hash() { [ -d "$1" ] && find "$1" -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256 | cut -c1-64 || shasum -a 256 -- "$1" | cut -c1-64; }
  insha=$(hash "$in")
  { print -r -- "sirsi-signd job $id  kind=$kind  file=$f  caller=${caller:-?}  $(date -u +%FT%TZ)"; print -r -- "input sha256 $insha"; } > "$R"
  notarize() {   # accepted only if THIS submission's own output says so
    local t=$(mktemp); xcrun notarytool submit "$1" --keychain-profile "$PROFILE" --wait > "$t" 2>&1
    cat "$t" >> "$R"; nid=$(awk '/^ *id: /{print $2; exit}' "$t")
    grep -qE '^[[:space:]]*status: Accepted[[:space:]]*$' "$t"; local rc=$?; rm -f "$t"; return $rc; }
  case $kind in
    codesign) # sign only, no notarization: for binaries that ship INSIDE a pkg/dmg/app whose own notarization covers them
         codesign --force --options runtime --timestamp --sign "$APP" -- "$in" >> "$R" 2>&1 || ok=0 ;;
    bin) codesign --force --options runtime --timestamp --sign "$APP" -- "$in" >> "$R" 2>&1 || ok=0
         [ $ok = 1 ] && { ditto -c -k --keepParent -- "$in" "$W/n.zip" && notarize "$W/n.zip" || ok=0; } ;;
    app) codesign --force --deep --options runtime --timestamp --sign "$APP" -- "$in" >> "$R" 2>&1 || ok=0
         [ $ok = 1 ] && { ditto -c -k --keepParent -- "$in" "$W/n.zip" && notarize "$W/n.zip" && xcrun stapler staple "$in" >> "$R" 2>&1 || ok=0; } ;;
    pkg) # productsign needs distinct paths; sign to a temp, then keep the ORIGINAL name so every kind returns signed-<name>
         productsign --sign "$INST" "$in" "$W/.signed.pkg" >> "$R" 2>&1 && mv -f "$W/.signed.pkg" "$in" || ok=0
         [ $ok = 1 ] && { notarize "$in" && xcrun stapler staple "$in" >> "$R" 2>&1 || ok=0; } ;;
    dmg) codesign --force --timestamp --sign "$APP" -- "$in" >> "$R" 2>&1 || ok=0
         [ $ok = 1 ] && { notarize "$in" && xcrun stapler staple "$in" >> "$R" 2>&1 || ok=0; } ;;
  esac
  outsha=none; [ -e "$out" ] && outsha=$(hash "$out")
  { print -r -- "--- verification"; codesign -dvv -- "$out" 2>&1 | grep -E "Authority=Developer|TeamIdentifier|Timestamp|flags"
    [ $kind = pkg ] && pkgutil --check-signature "$out" | sed -n 2,5p
    case $kind in
      pkg) spctl -a -vv -t install "$out" 2>&1 | head -3 ;;
      codesign) print -r -- "spctl: not applicable (sign-only; Gatekeeper assessment happens on the notarized container)" ;;
      *) spctl -a -vv "$out" 2>&1 | head -3 ;;
    esac
    print -r -- "notary submission $nid"; print -r -- "output sha256 $outsha"; } >> "$R" 2>&1
  # Publish ONLY on success: a failed job never delivers a file (an unsigned or half-signed artifact stays in the work dir and is deleted with it).
  if [ $ok = 1 ] && [ -e "$out" ]; then mv -- "$out" "$O/"; fi; rm -rf -- "$W"
  result=failed; [ $ok = 1 ] && result=ok
  print -r -- "$(date -u +%FT%TZ) id=$id caller=${caller:-?} kind=$kind file=$f in=$insha out=$outsha notary=$nid result=$result" >> "$B/audit.log"
  [ $ok = 1 ] && touch "$O/done" || touch "$O/failed"
done
