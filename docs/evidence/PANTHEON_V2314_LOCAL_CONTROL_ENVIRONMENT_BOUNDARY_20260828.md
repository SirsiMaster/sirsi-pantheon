# Pantheon v0.23.14 Local-Control Environment Boundary

**Status:** source-complete; focused Swift verification deferred by active host hold

## Change

The visible Swift local-control action no longer forwards
`ProcessInfo.processInfo.environment` wholesale to its optional
`pantheon-engine` child. `SNELocalControlBridge.controlledEnvironment` now
allows only ordinary local process context:

```text
HOME USER LOGNAME TMPDIR LANG LC_ALL LC_CTYPE TZ __CF_USER_TEXT_ENCODING
PATH=/usr/bin:/bin:/usr/sbin:/sbin
SIRSI_HEADLESS=1
```

It drops ambient provider credentials, application tokens, debug switches, and
developer-only runtime paths. The child remains an explicit loopback-control
process; this change does not start it, start SNE, or make a permission request.

## Regression fixture

`SNEControlReadModelTests.testLocalControlEnvironmentIsAllowlisted` supplies
`SIRSI_ACCESS_TOKEN`, `AWS_SESSION_TOKEN`, and `PYTHONPATH` and asserts that
all are absent from the child environment while the required local values remain.

`SNEControlReadModelTests.testLocalControlStartIsLimitedToLoopbackTransportFailures`
accepts only URL transport failures (`cannotConnectToHost`,
`cannotFindHost`, `networkConnectionLost`, or `notConnectedToInternet`) as a
reason to show the visible start action. SNE/API failures and malformed local
responses do not offer a redundant helper launch.

## Verification disposition

`git diff --check` is required before commit. Swift test execution is deferred
while the Hardware Admin release request for the non-GPU FinalWishes CI remains
open. This record makes no runtime, package, signing, or release claim until
the focused Swift test is run in a released window.
