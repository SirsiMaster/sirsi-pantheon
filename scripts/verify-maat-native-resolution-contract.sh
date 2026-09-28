#!/bin/sh
# verify-maat-native-resolution-contract.sh keeps native Pantheon alarms from
# regressing to a stranded warning. A finding may lack a safe automatic repair,
# but it must still lead an operator through Ma'at's evidence-bound review and
# explicit acceptance path.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
view="$root/macapp/Sources/SirsiMenubar/Views.swift"
engine="$root/macapp/Sources/SirsiMenubar/SirsiEngine.swift"
casebook="$root/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"
launchd="$root/internal/router/launchdkickstart.go"
screencli="$root/cmd/sirsi/maatscreen.go"
casebookcli="$root/cmd/sirsi/maatcasebook.go"
tui="$root/internal/tui/screen_activity.go"
dashboard="$root/internal/dashboard/pages.go"
controlcenter="$root/macapp/Sources/SirsiMenubar/ControlCenter.swift"
recipe="$root/contracts/stacklab/maat-system-one-recipe-v1.json"
catalog="$root/docs/qa/MAAT_SYSTEM_ONE_CATALOG.md"

[ -f "$view" ] || { echo "missing native views" >&2; exit 1; }
[ -f "$engine" ] || { echo "missing native engine" >&2; exit 1; }
[ -f "$casebook" ] || { echo "missing native Ma'at casebook" >&2; exit 1; }
[ -f "$launchd" ] || { echo "missing managed launchd recovery" >&2; exit 1; }
[ -f "$screencli" ] || { echo "missing Ma'at System One command" >&2; exit 1; }
[ -f "$casebookcli" ] || { echo "missing Ma'at Casebook CLI" >&2; exit 1; }
[ -f "$tui" ] || { echo "missing Ma'at Casebook TUI" >&2; exit 1; }
[ -f "$dashboard" ] || { echo "missing Ma'at Casebook dashboard" >&2; exit 1; }
[ -f "$controlcenter" ] || { echo "missing Pantheon control center" >&2; exit 1; }
[ -f "$recipe" ] || { echo "missing Ma'at System One Stack Lab recipe" >&2; exit 1; }
[ -f "$catalog" ] || { echo "missing Ma'at System One catalog" >&2; exit 1; }

if /usr/bin/grep -Fq 'This needs attention but has no one-click fix yet.' "$view"; then
  echo "native finding dead-end text remains" >&2
  exit 1
fi

# Caution-tier hygiene is deliberately not auto-selected, but it is not allowed
# to strand an operator in a Terminal-only workflow. The native route must keep
# its inspect → explicit select → scoped confirmation → trash-first resolution,
# and the Go engine must receive the opt-in scope only at that confirmed call.
for stranded in \
  'Not cleaned with one click' \
  'clean deliberately in Terminal' \
  'Held back from one-click cleaning'; do
  if /usr/bin/grep -Fq "$stranded" "$view"; then
    echo "native caution cleanup dead-end remains: $stranded" >&2
    exit 1
  fi
done

for needle in \
  'Caution items are never selected automatically.' \
  'Move selected caution items to Trash?' \
  'Move this caution item to Trash?' \
  'includeCaution: Bool = false' \
  'if includeCaution { args.append("--include-caution") }' \
  'engine.cleanSelected(paths: [finding.path], includeCaution: true)' \
  'Go intersection gate remains authoritative'; do
  target="$view"
  case "$needle" in
    'includeCaution: Bool = false'|'if includeCaution { args.append("--include-caution") }'|'Go intersection gate remains authoritative') target="$engine" ;;
  esac
  /usr/bin/grep -Fq "$needle" "$target" || {
    echo "native caution cleanup resolution missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'Record Ma'\''at review' \
  'Record a Ma'\''at owner review?' \
  'record-resolution' \
  '"--confirm"' \
  'Continue in Ma'\''at Casebook' \
  'MaatCasebookView(engine: engine)'; do
  /usr/bin/grep -Fq "$needle" "$view" || {
    echo "native Ma'at resolution path missing: $needle" >&2
    exit 1
  }
done

for target in "$casebookcli" "$tui" "$dashboard"; do
	/usr/bin/grep -Fq 'prescribed next step' "$target" || {
		echo "Ma'at recovery guidance projection missing: $target" >&2
		exit 1
	}
done

for target in "$casebookcli" "$tui" "$dashboard"; do
	/usr/bin/grep -Fq 'deterministic floor' "$target" || {
		echo "Ma'at System One floor projection missing: $target" >&2
		exit 1
	}
	done

for target in "$casebookcli" "$tui" "$dashboard"; do
	/usr/bin/grep -Fq 'recovery level' "$target" || {
		echo "Ma'at three-level recovery projection missing: $target" >&2
		exit 1
	}
done

/usr/bin/grep -Fq 'Screen model' "$casebook" || {
	echo "native Ma'at System One model provenance missing" >&2
	exit 1
}

/usr/bin/grep -Fq 'Resolution path' "$casebook" || {
	echo "native Ma'at three-level recovery path missing" >&2
	exit 1
}

/usr/bin/grep -Fq 'system_one_floor_recovery' "$casebook" || {
	echo "native Ma'at failed-floor recovery route missing" >&2
	exit 1
}

for needle in 'Resolve with Ma'\''at' 'Open Ma'\''at evidence' 'MaatWorkspaceView(engine: engine)'; do
  /usr/bin/grep -Fq "$needle" "$controlcenter" || {
    echo "Ma'at must remain the native control-center primary action: $needle" >&2
    exit 1
  }
done

for target in "$recipe" "$catalog"; do
	/usr/bin/grep -Fq 'deterministic-floor' "$target" || {
		echo "Stack Lab Ma'at floor provenance canon missing: $target" >&2
		exit 1
	}
done

for needle in \
  'confirmFix' \
  'Apply this system repair?' \
  'showConfirmedFix' \
  'will not broaden the command or touch unrelated services'; do
  /usr/bin/grep -Fq "$needle" "$view" || {
    echo "native confirmed repair contract missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'accept-resolution' \
  'Requires explicit confirmation' \
  'does not repair the system'; do
  /usr/bin/grep -Fq "$needle" "$casebook" || {
    echo "native Ma'at acceptance path missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'MaatSystemOneView' \
  'System One evidence is unavailable' \
  'No System One evidence yet' \
  'System One does not invent a screen' \
  'screens never grant execution authority' \
  'Choose System One JSON' \
  'Record this System One screen?' \
  'Validate and record' \
  'It will not execute the assessed payload' \
  '"maat", "screen", "--input"' \
  '"--confirm"'; do
  /usr/bin/grep -Fq "$needle" "$casebook" || {
    echo "native Ma'at System One surface missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'Preflight the release contract' \
  'Inspect release contract' \
  'Record this release-contract preflight?' \
  'Record in Ma'\''at Casebook' \
  'It never runs a build, package, signing, notarization, network, or release command.' \
  '"maat", "preflight", "release", "--root"' \
  'MaatReleaseContractPreflight' \
  'Fix: \(finding.fixHint)' \
  'Choose a project below before preflighting'; do
  /usr/bin/grep -Fq "$needle" "$casebook" || {
    echo "native Ma'at release-contract preflight surface missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'maatScreenConfirm' \
  'rerun with --confirm' \
  'confirm recording the validated Ma'\''at System One screen'; do
  /usr/bin/grep -Fq "$needle" "$screencli" || {
    echo "Ma'at System One confirmation contract missing: $needle" >&2
    exit 1
  }
done

for needle in \
  'RestoreDisabledManagedLaunchAgents' \
  'targets map[string]bool' \
  'targets != nil && !targets[label]' \
  'snapshots the' \
  'disabled override labels'; do
  /usr/bin/grep -Fq "$needle" "$launchd" || {
    echo "confirmed recovery scope missing: $needle" >&2
    exit 1
  }
done

echo "Ma'at native resolution contract: pass"
