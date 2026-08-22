#!/bin/zsh
set -euo pipefail

repo=${0:A:h:h}
catalog=$repo/configs/sne/releases/sne-gemma4-v2-portable.json
signature=$catalog.sig
public_key=$repo/configs/sne/runtime-catalog-ed25519.pub
before=$(shasum -a 256 $catalog | awk '{print $1}')
fixture=$(mktemp -d ${TMPDIR:-/tmp}/sne-catalog-candidate.XXXXXX)
trap 'rm -rf $fixture' EXIT INT TERM
mkdir -p $fixture/package

(cd $repo && go build -trimpath -o $fixture/sirsi-sne-catalog-candidate ./cmd/sirsi-sne-catalog-candidate)
set +e
$fixture/sirsi-sne-catalog-candidate \
  -current-catalog $catalog -current-signature $signature -public-key $public_key \
  -package $fixture/package -admission-receipt $fixture/missing-receipt.json \
  -promotion-pointer $fixture/missing-pointer.json -runtime-id rejected-fixture \
  -catalog-id rejected-fixture -output $fixture/forbidden.json \
  >$fixture/stdout 2>$fixture/stderr
exit_status=$?
set -e

[[ $exit_status == 67 && ! -e $fixture/forbidden.json ]] || {
  cat $fixture/stderr >&2
  print -u2 "catalog candidate missing-admission gate failed: status=$exit_status output_exists=$([[ -e $fixture/forbidden.json ]] && print true || print false)"
  exit 65
}
after=$(shasum -a 256 $catalog | awk '{print $1}')
[[ $before == $after ]] || { print -u2 'authoritative signed catalog changed'; exit 66; }
! rg -q 'sign|install|activate' $fixture/stdout || { print -u2 'rejected generator claimed a release action'; exit 67; }

print "sne_api4096_catalog_candidate_admission accepted=true missing_admission_exit=67 output_created=false authoritative_catalog_unchanged=true signing_capability=false install_capability=false activation_capability=false"
