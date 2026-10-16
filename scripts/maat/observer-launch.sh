#!/bin/sh
# Refuse unsigned/replaced observers before bind(), never ask for privileges.
set -eu
verify_only=0
if [ "${1:-}" = --verify-only ]; then
  verify_only=1
  shift
else
  case "${1:-}" in
    "$HOME/Development/Apollo/artifacts/development/apollo-dashboard/apollo-observer-agent"|"$HOME/.local/share/apollo-observer/apollo-observer-agent") ;;
    *) echo 'Apollo observer: unexpected executable path; refusing' >&2; exit 78 ;;
  esac
fi
if [ "$#" -eq 0 ]; then exit 78; fi
if [ ! -f "$1" ] || [ -L "$1" ]; then
  echo 'Apollo observer: regular non-symlink executable required; refusing' >&2
  exit 78
fi
requirement='=identifier "ai.sirsi.apollo.observer" and anchor apple generic and certificate leaf[subject.OU] = "9D382WV988"'
if ! /usr/bin/codesign --verify --strict -R "$requirement" "$1"; then
  echo 'Apollo observer: approved Sirsi signature missing; refusing before listener startup' >&2
  exit 78
fi
cdhash=$(/usr/bin/codesign -dv --verbose=4 "$1" 2>&1 | /usr/bin/sed -n 's/^CDHash=//p')
if [ "$cdhash" != ec22477554eacc88cac63f43504b7ee6252be00c ]; then
  echo 'Apollo observer: release identity not approved; refusing before listener startup' >&2
  exit 78
fi
if [ "$verify_only" -eq 1 ]; then exit 0; fi
exec "$@"
