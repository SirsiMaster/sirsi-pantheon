#!/bin/zsh
set -euo pipefail

repo=${0:A:h:h}
fixture=$(mktemp -d "${TMPDIR:-/tmp}/sne-live-checkout-contract.XXXXXX")
trap 'rm -rf "$fixture"' EXIT INT TERM
catalog=$fixture/catalog.json
destination=$fixture/model
evidence=$fixture/evidence
cat >"$catalog" <<'JSON'
{"schema":"pantheon.sne-model-source.v1","catalog_id":"fixture","entries":[]}
JSON
cat >"$fixture/fake-acquire" <<'SH'
#!/bin/zsh
set -euo pipefail
destination=
while (( $# )); do
  if [[ $1 == -destination ]]; then destination=$2; shift 2; else shift; fi
done
[[ -n $destination ]]
mkdir -p "$destination"
print verified >"$destination/model.bin"
print '{"type":"progress","progress":{"files_done":1,"files_total":1}}'
print '{"type":"result","catalog_id":"fixture","result":{"files":1,"bytes":9}}'
SH
chmod +x "$fixture/fake-acquire"
catalog_sha=$(shasum -a 256 "$catalog" | awk '{print $1}')

SNE_EXPECTED_SOURCE_CATALOG_SHA256=$catalog_sha SNE_ACQUIRE_BIN=$fixture/fake-acquire \
  "$repo/scripts/run-sne-live-model-checkout.zsh" "$catalog" fixture-entry "$destination" "$evidence" >/dev/null
grep -q '^status=accepted$' "$evidence/receipt.env"
grep -q "^catalog_sha256=$catalog_sha$" "$evidence/receipt.env"
[[ -f $destination/model.bin && ! -e $destination/model.bin.partial ]]

set +e
SNE_EXPECTED_SOURCE_CATALOG_SHA256=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
  SNE_ACQUIRE_BIN=$fixture/fake-acquire "$repo/scripts/run-sne-live-model-checkout.zsh" \
  "$catalog" fixture-entry "$fixture/forbidden-model" "$fixture/forbidden-evidence" >/dev/null 2>&1
exit_status=$?
set -e
[[ $exit_status == 67 && ! -e $fixture/forbidden-model && ! -e $fixture/forbidden-evidence ]] || exit 66

print "sne_live_model_checkout_contract accepted=true success_receipt=true wrong_hash_exit=67 wrong_hash_residue=false"
