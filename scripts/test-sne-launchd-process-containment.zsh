#!/bin/zsh
set -euo pipefail

[[ $# -eq 6 ]] || {
  print -u2 "usage: $0 STAGE PACKAGE CHECKPOINT SUPERVISOR_SHA SERVICE_SHA NATIVE_SHA"
  exit 64
}

stage=${1:A}
package=${2:A}
checkpoint=${3:A}
supervisor_sha=$4
service_sha=$5
native_sha=$6
supervisor=${SNE_TEST_SUPERVISOR:-$stage/sirsi-sne-supervisor}
sned=${SNE_TEST_SNED:-$package/bin/sned}
profile=${SNE_TEST_PROFILE:-$stage/profile.yaml}
admission=${SNE_TEST_ADMISSION:-$stage/model-admission.json}
readiness=${SNE_TEST_READINESS:-$stage/model-readiness.json}
catalog_entry=${SNE_TEST_CATALOG_ENTRY:-e2b-nvfp4-api-v2-compat-v6-m1}
label=ai.sirsi.sne.process-group-gate
domain=gui/$(id -u)
installed_label=${SNE_INSTALLED_LAUNCH_LABEL:-ai.sirsi.sne.supervisor}
installed_plist=${SNE_INSTALLED_LAUNCH_PLIST:-$HOME/Library/LaunchAgents/$installed_label.plist}
listen=127.0.0.1:19523
port=19523
plist=$stage/$label.plist
token_file=$stage/capability
log=$stage/launchd.log
phase_file=$stage/phase
phase() { print -r -- "$1" >$phase_file }

for required in \
	$supervisor $profile $admission $readiness $sned $package/manifests/model.json \
  $package/lib/runtime/libsirsi_native_runtime.dylib $package/lib/libmlx.dylib \
  $package/share/mlx.metallib $checkpoint/tokenizer.json; do
  [[ -e $required ]] || { print -u2 "missing $required"; exit 65; }
done
[[ $supervisor_sha =~ '^[0-9a-f]{64}$' ]] || { print -u2 "invalid supervisor SHA-256"; exit 65; }
actual_supervisor_sha=$(shasum -a 256 $supervisor | awk '{print $1}')
[[ $actual_supervisor_sha == $supervisor_sha ]] || {
  print -u2 "supervisor SHA-256 mismatch: expected $supervisor_sha got $actual_supervisor_sha at $supervisor"
  exit 65
}

cleanup_candidate() {
	launchctl bootout $domain/$label >/dev/null 2>&1 || true
  for attempt in {1..100}; do
		[[ -z $(lsof -nP -iTCP:$port -sTCP:LISTEN -t 2>/dev/null || true) ]] && break
    sleep 0.05
  done
}
restore_installed() {
	cleanup_candidate
	if [[ -f $installed_plist ]]; then
		launchctl bootstrap $domain $installed_plist >/dev/null 2>&1 || true
		launchctl enable $domain/$installed_label >/dev/null 2>&1 || true
		launchctl kickstart -k $domain/$installed_label >/dev/null 2>&1 || true
	fi
}
trap restore_installed EXIT INT TERM
cleanup_candidate
launchctl bootout $domain/$installed_label >/dev/null 2>&1 || true
for attempt in {1..120}; do
	[[ -z $(lsof -nP -iTCP:8477 -sTCP:LISTEN -t 2>/dev/null || true) ]] && break
	sleep 0.1
done
[[ -z $(lsof -nP -iTCP:8477 -sTCP:LISTEN -t 2>/dev/null || true) ]]
sleep 2

uuidgen | tr -d '\n' >$token_file
chmod 600 $token_file
cat >$plist <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>$label</string>
<key>ProgramArguments</key><array>
<string>$supervisor</string>
<string>-profile</string><string>$profile</string>
<string>-model-admission-registry</string><string>$admission</string>
<string>-model-readiness-registry</string><string>$readiness</string>
<string>-readiness-policy</string><string>identity</string>
<string>-catalog-entry</string><string>$catalog_entry</string>
<string>-sned</string><string>$sned</string>
<string>-model-manifest</string><string>$package/manifests/model.json</string>
<string>-checkpoint-dir</string><string>$checkpoint</string>
<string>-tokenizer-json</string><string>$checkpoint/tokenizer.json</string>
<string>-assistant-safetensors</string><string></string>
<string>-metallib</string><string>$package/share/mlx.metallib</string>
<string>-mlx-dylib</string><string>$package/lib/libmlx.dylib</string>
<string>-native-library-dir</string><string>$package/lib/runtime</string>
<string>-runtime-sha256</string><string>$service_sha</string>
<string>-native-runtime-sha256</string><string>$native_sha</string>
<string>-native-runtime-dylib</string><string>$package/lib/runtime/libsirsi_native_runtime.dylib</string>
<string>-service-version</string><string>1.2.2</string>
<string>-local-access-token-file</string><string>$token_file</string>
</array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
<key>ProcessType</key><string>Interactive</string>
<key>ThrottleInterval</key><integer>1</integer>
<key>StandardOutPath</key><string>$log</string>
<key>StandardErrorPath</key><string>$log</string>
</dict></plist>
EOF
plutil -lint $plist >/dev/null

wait_ready() {
  local previous=${1:-}
  for attempt in {1..180}; do
		local child=$(lsof -nP -iTCP:$port -sTCP:LISTEN -t 2>/dev/null | head -1 || true)
    if [[ -n $child && $child != $previous ]] && curl -fsS --max-time 1 -H "Authorization: Bearer $(cat $token_file)" http://$listen/health/ready >/dev/null 2>&1; then
      print -r -- $child
      return 0
    fi
    sleep 0.25
  done
  return 1
}

launchctl bootstrap $domain $plist
phase initial-readiness
first_child=$(wait_ready)
phase initial-supervisor-identity
first_supervisor=$(launchctl print $domain/$label | awk '/pid =/{print $3; exit}')
[[ -n $first_supervisor && -n $first_child ]]

phase child-crash
kill -KILL $first_child
phase child-recovery
second_child=$(wait_ready $first_child)
[[ $second_child != $first_child ]]
second_supervisor=$(launchctl print $domain/$label | awk '/pid =/{print $3; exit}')
[[ -n $second_supervisor && $second_supervisor != $first_supervisor ]]

phase supervisor-crash
kill -KILL $second_supervisor
phase supervisor-recovery
for attempt in {1..120}; do
	third_supervisor=$(launchctl print $domain/$label 2>/dev/null | awk '/pid =/{print $3; exit}' || true)
	[[ -n $third_supervisor && $third_supervisor != $second_supervisor ]] && break
	sleep 0.1
done
[[ -n ${third_supervisor:-} && $third_supervisor != $second_supervisor ]]
third_child=$(wait_ready $second_child)
[[ $third_child != $second_child ]]

phase final-stop
launchctl bootout $domain/$label
for attempt in {1..100}; do
	[[ -z $(lsof -nP -iTCP:$port -sTCP:LISTEN -t 2>/dev/null || true) ]] && break
  sleep 0.05
done
[[ -z $(lsof -nP -iTCP:$port -sTCP:LISTEN -t 2>/dev/null || true) ]]
phase accepted
restore_installed
trap - EXIT INT TERM

jq -n \
	--arg status accepted --arg supervisor_path $supervisor --arg supervisor_sha256 $supervisor_sha \
  --arg catalog_entry $catalog_entry --arg model_admission_registry $admission --arg model_readiness_registry $readiness \
  --arg service_sha256 $service_sha --arg native_runtime_sha256 $native_sha \
	--argjson first_supervisor $first_supervisor --argjson second_supervisor $second_supervisor --argjson third_supervisor $third_supervisor \
  --argjson first_child $first_child --argjson second_child $second_child --argjson third_child $third_child \
	'{status:$status,normal_launch:true,child_crash_recovered_by_fresh_supervisor:true,supervisor_crash_recovered:true,final_group_empty:true,catalog_entry:$catalog_entry,model_admission_registry:$model_admission_registry,model_readiness_registry:$model_readiness_registry,supervisor_path:$supervisor_path,supervisor_sha256:$supervisor_sha256,service_sha256:$service_sha256,native_runtime_sha256:$native_runtime_sha256,supervisor_pids:[$first_supervisor,$second_supervisor,$third_supervisor],service_pids:[$first_child,$second_child,$third_child]}' | tee $stage/receipt.json
