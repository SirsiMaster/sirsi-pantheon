#!/bin/zsh
set -euo pipefail

repo=${0:A:h:h}
v1=$repo/configs/sne/releases/sne-gemma4-v1-portable.json
v1_sig=$repo/configs/sne/releases/sne-gemma4-v1-portable.json.sig
v2=$repo/configs/sne/releases/sne-gemma4-v2-portable.json
v2_sig=$repo/configs/sne/releases/sne-gemma4-v2-portable.json.sig
v1_admission=$repo/configs/supervisor/sne-gemma4-v1-admission.json
v2_admission=$repo/configs/supervisor/sne-gemma4-v2-admission.json
public_key=$repo/configs/sne/runtime-catalog-ed25519.pub
packages_root=${1:-$HOME/Library/Application Support/Sirsi/SNE/packages}

[[ $(shasum -a 256 "$v1" | awk '{print $1}') == bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d ]]
[[ $(shasum -a 256 "$v1_sig" | awk '{print $1}') == 6842cee0dbcc6d36be5f59e6eedb819027e378d001afb208a4e9fe24f2314351 ]]
[[ $(shasum -a 256 "$v1_admission" | awk '{print $1}') == 1f0f8d5aebe9b7a4c92f8fb5858ae925a4f8fd3eb9e1cbda48f3d36bd676c013 ]]

jq -n -e --slurpfile old "$v1" --slurpfile next "$v2" '
  $old[0].catalog_id == "sne-gemma4-v1" and
  $next[0].catalog_id == "sne-gemma4-v2" and
  ($next[0].entries | length) == ($old[0].entries | length) + 1 and
  ($old[0].entries | all(. as $entry | $next[0].entries | any(. == $entry))) and
  ($next[0].entries | any(.model_id == "gemma-4-12b-it-affine8-sne-v26-capacity-readiness-candidate"))
' >/dev/null

jq -e '
  ([.entries[] | [.architecture,.parameter_class,.adapter,.execution_mode,.weight_format,.weight_bits,.weight_group_size] | @tsv] | length) ==
  ([.entries[] | [.architecture,.parameter_class,.adapter,.execution_mode,.weight_format,.weight_bits,.weight_group_size] | @tsv] | unique | length) and
  ([.entries[] | select(.catalog_entry == "12b-affine8-paged-v26")] | length) == 1 and
  ([.entries[] | select(.catalog_entry == "12b-affine8-plain")] | length) == 0
' "$v2_admission" >/dev/null

go run "$repo/cmd/sirsi-sne-catalog-verify" \
  -catalog "$v2" -signature "$v2_sig" -public-key "$public_key" \
  -packages-root "$packages_root" >/dev/null

print 'sne_signed_catalog_lineage accepted=true predecessor_immutable=true active_tuple_unique=true v26_present=true'
