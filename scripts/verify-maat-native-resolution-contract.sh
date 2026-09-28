#!/bin/sh
# verify-maat-native-resolution-contract.sh keeps native Pantheon alarms from
# regressing to a stranded warning. A finding may lack a safe automatic repair,
# but it must still lead an operator through Ma'at's evidence-bound review and
# explicit acceptance path.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
view="$root/macapp/Sources/SirsiMenubar/Views.swift"
casebook="$root/macapp/Sources/SirsiMenubar/MaatCasebookView.swift"

[ -f "$view" ] || { echo "missing native views" >&2; exit 1; }
[ -f "$casebook" ] || { echo "missing native Ma'at casebook" >&2; exit 1; }

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

for needle in \
  'accept-resolution' \
  'Requires explicit confirmation' \
  'does not repair the system'; do
  /usr/bin/grep -Fq "$needle" "$casebook" || {
    echo "native Ma'at acceptance path missing: $needle" >&2
    exit 1
  }
done

echo "Ma'at native resolution contract: pass"
