#!/bin/sh
# verify-maat-native-resolution-contract.sh keeps native Pantheon alarms from
# regressing to a stranded warning. A finding may lack a safe automatic repair,
# but it must still lead an operator through Ma'at's evidence-bound review and
# explicit acceptance path.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
view="$root/macapp/Sources/SirsiMenubar/Views.swift"
casebook="$root/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"
launchd="$root/internal/router/launchdkickstart.go"
screencli="$root/cmd/sirsi/maatscreen.go"
casebookcli="$root/cmd/sirsi/maatcasebook.go"
tui="$root/internal/tui/screen_activity.go"
dashboard="$root/internal/dashboard/pages.go"

[ -f "$view" ] || { echo "missing native views" >&2; exit 1; }
[ -f "$casebook" ] || { echo "missing native Ma'at casebook" >&2; exit 1; }
[ -f "$launchd" ] || { echo "missing managed launchd recovery" >&2; exit 1; }
[ -f "$screencli" ] || { echo "missing Ma'at System One command" >&2; exit 1; }
[ -f "$casebookcli" ] || { echo "missing Ma'at Casebook CLI" >&2; exit 1; }
[ -f "$tui" ] || { echo "missing Ma'at Casebook TUI" >&2; exit 1; }
[ -f "$dashboard" ] || { echo "missing Ma'at Casebook dashboard" >&2; exit 1; }

if /usr/bin/grep -Fq 'This needs attention but has no one-click fix yet.' "$view"; then
  echo "native finding dead-end text remains" >&2
  exit 1
fi

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
