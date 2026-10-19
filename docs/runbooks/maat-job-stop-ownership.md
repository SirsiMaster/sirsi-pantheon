# Managed job lifecycle ownership

Compute placement, scientific intent and lifecycle control are distinct:

| Responsibility | Apollo214312 deployment |
| --- | --- |
| Scientific intent and finite workload plan | Apollo lead |
| Lifecycle of adopted launchd instance | SSA/Ma’at |
| Canonical routing and job-control integration | Ra |

A resource reservation authorizes only its admitted workload. It does **not**
authorize killing another process, even if the process name, logical lane or
work name matches. The Oct18 incident was a `pkill -f` against an adopted
training instance before the handoff was read, not an exhausted Ma’at lease.

## Implemented accident guard

`scripts/maat/admitted-run.py` refuses direct process-stop commands (`pkill`,
`killall`, `kill`, `skill`, `fuser`), common launchctl/systemctl/brew service
stop/restart routes, and common env/shell wrappers before resource
reservation or child spawn. Supervisor teardown of its own launched child group
is unchanged. `--check-stop-command` is a pure PreToolUse policy entry point;
it reads bounded tool JSON and never registers, reserves, spawns or signals.
Registration, router messaging and task bookkeeping are not compute jobs and
remain available without a compute reservation.

The M5 existing `guard-pretool` chain adds this policy before the unchanged
legacy/admission guards. The policy is installed separately: do not replace an
active recurring owner's hash-bound helper to install it. Retain prior hook
bytes and verify both the stop negative control and coordination positive
controls before declaring a host deployed. Other hosts require their own
installation/readback; a source commit alone proves none of those installations.

## Stop or retire an adopted service

1. Route an exact request to its lifecycle controller. Include host, private job
   root, launchd label, controller and workload lane, instance start, executor
   and grant IDs, reason, preserve-data instruction, and desired boundary.
2. Controller reconciles latest intent and authority. Another lane's statement
   of owner approval is evidence, not a replacement for any actual tool approval.
3. Prefer cooperative natural completion. For legacy owner retirement, verify
   exact coordinator PID **and start**, completed iteration and no active
   descendants. Recheck after freezing only that idle coordinator; if it raced
   into a new batch, resume unchanged. Never signal the model/helper or infer
   an orphan from process name, age, missing inbox or same lane.
4. Remove only that exact service registration, preserve checkpoint/data/logs
   and publish a retirement receipt as a **new** router item. No automatic
   replacement or restart. Trash/removal is a separate operation.

## Honest enforcement boundary

This is cooperative accident prevention, not adversarial process isolation.
An unrestricted same-UID shell, opaque script or interpreter can still issue
raw signals; a classifier cannot prove arbitrary program semantics. In
particular, the guard does not claim complete shell-language interpretation,
kernel PID-race elimination, cryptographic job-control authorization or a
canonical cloud job-control API. Do not turn these limitations into blanket
blocking of normal working threads. True hostile-lane isolation requires a
separately designed executor privilege boundary; no UID, sudo, key, firewall
or account-security changes are part of this repair.

Reproduce model-free checks with native CPU-only admission and
`python3 -B -m unittest discover -s scripts/maat -p 'test*.py' -v`.
